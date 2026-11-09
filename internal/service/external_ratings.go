package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
)

const (
	ratingPending  = "pending"
	ratingRetry    = "retry"
	ratingFound    = "found"
	ratingPartial  = "partial"
	ratingMissing  = "missing"
	omdbDailyLimit = 1000
)

var ErrRatingLibraryNotFound = errors.New("媒体库不存在")
var ErrOMDbNotConfigured = errors.New("请先在「系统设置 → 外部 API」启用 OMDb 并填写 API Key")

type ratingWork struct {
	Kind   string
	TMDbID int
}

type ExternalRatingService struct {
	repo   *repository.Container
	config *APIConfigService
	tmdb   *TMDbProvider
	log    *zap.Logger
	client *http.Client
	wake   chan struct{}
}

func newExternalRatingService(repo *repository.Container, config *APIConfigService, tmdb *TMDbProvider, log *zap.Logger) *ExternalRatingService {
	client := NewExternalHTTPClient(8 * time.Second)
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return &ExternalRatingService{repo: repo, config: config, tmdb: tmdb, log: log, client: client, wake: make(chan struct{}, 1)}
}

type RatingLibraryStatus struct {
	Total       int    `json:"total"`
	Unqueried   int    `json:"unqueried"`
	Pending     int    `json:"pending"`
	Found       int    `json:"found"`
	Partial     int    `json:"partial"`
	Missing     int    `json:"missing"`
	Failed      int    `json:"failed"`
	PauseReason string `json:"pause_reason,omitempty"`
}

func (s *ExternalRatingService) configForRatings(ctx context.Context) (Resolved, int, error) {
	cfg, err := s.config.Resolve(ctx, "omdb")
	if err != nil {
		return cfg, 0, err
	}
	if !cfg.Enabled || cfg.APIKey == "" {
		return cfg, 0, ErrOMDbNotConfigured
	}
	limit := omdbDailyLimit
	if n, err := strconv.Atoi(strings.TrimSpace(cfg.Extra)); err == nil && n > 0 {
		limit = n
	}
	return cfg, limit, nil
}

func (s *ExternalRatingService) LibraryStatus(ctx context.Context, libraryID string) (RatingLibraryStatus, error) {
	var status RatingLibraryStatus
	works, err := s.libraryWorks(ctx, libraryID)
	if err != nil {
		return status, err
	}
	status.Total = len(works)
	now := time.Now()
	for batch := range slices.Chunk(works, 200) {
		rows, err := s.ratingRows(ctx, batch)
		if err != nil {
			return status, err
		}
		unqueried := len(batch) - len(rows)
		status.Unqueried += unqueried
		for _, row := range rows {
			switch row.Status {
			case ratingPending:
				status.Pending++
			case ratingRetry:
				status.Failed++
			case ratingFound:
				status.Found++
			case ratingPartial:
				status.Partial++
			case ratingMissing:
				status.Missing++
			}
		}
	}
	cfg, limit, err := s.configForRatings(ctx)
	switch {
	case errors.Is(err, ErrOMDbNotConfigured):
		status.PauseReason = "unconfigured"
	case err != nil:
		return status, err
	default:
		status.PauseReason, err = s.pauseReason(ctx, cfg.APIKey, limit, now)
	}
	return status, err
}

func (s *ExternalRatingService) EnqueueLibrary(ctx context.Context, libraryID string) (int, error) {
	if _, _, err := s.configForRatings(ctx); err != nil {
		return 0, err
	}
	works, err := s.libraryWorks(ctx, libraryID)
	if err != nil {
		return 0, err
	}
	return s.enqueue(ctx, works)
}

func (s *ExternalRatingService) enqueue(ctx context.Context, works []ratingWork) (int, error) {
	now := time.Now()
	count := 0
	for batch := range slices.Chunk(works, 200) {
		rows := make([]model.ExternalRating, 0, len(batch))
		for _, work := range batch {
			rows = append(rows, model.ExternalRating{Kind: work.Kind, TMDbID: work.TMDbID, Status: ratingPending, NextAttemptAt: now})
		}
		result := s.repo.DB.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "kind"}, {Name: "tm_db_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"status", "next_attempt_at"}),
			Where: clause.Where{Exprs: []clause.Expression{clause.Expr{
				SQL:  "external_ratings.status IN ? AND external_ratings.next_attempt_at <= ?",
				Vars: []any{[]string{ratingMissing, ratingPartial}, now},
			}}},
		}).Create(&rows)
		if result.Error != nil {
			return count, result.Error
		}
		count += int(result.RowsAffected)
	}
	if count > 0 {
		select {
		case s.wake <- struct{}{}:
		default:
		}
	}
	return count, nil
}

func (s *ExternalRatingService) ratingRows(ctx context.Context, works []ratingWork) ([]model.ExternalRating, error) {
	keys := make([][]any, 0, len(works))
	for _, work := range works {
		keys = append(keys, []any{work.Kind, work.TMDbID})
	}
	var rows []model.ExternalRating
	err := s.repo.DB.WithContext(ctx).Where("(kind, tm_db_id) IN ?", keys).Find(&rows).Error
	return rows, err
}

func (s *ExternalRatingService) ForMedia(ctx context.Context, media *model.Media) (model.ExternalRating, error) {
	var empty model.ExternalRating
	if media.TMDbMediaType == "" {
		lib, err := s.repo.Library.FindByID(ctx, media.LibraryID)
		if err != nil {
			return empty, err
		}
		works, err := s.resolveWorks(ctx, lib, []model.Media{*media}, func(m model.Media) bool {
			return s.repo.Media.ExistsSiblingWithTMDbID(ctx, &m)
		})
		if err != nil || len(works) == 0 {
			return empty, err
		}
		return s.getRating(ctx, works[0])
	}
	if media.TMDbID <= 0 {
		return empty, nil
	}
	return s.getRating(ctx, ratingWork{media.TMDbMediaType, media.TMDbID})
}

func (s *ExternalRatingService) getRating(ctx context.Context, work ratingWork) (model.ExternalRating, error) {
	var row model.ExternalRating
	err := s.repo.DB.WithContext(ctx).Take(&row, "kind = ? AND tm_db_id = ?", work.Kind, work.TMDbID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = nil
	}
	return row, err
}

func (s *ExternalRatingService) libraryWorks(ctx context.Context, libraryID string) ([]ratingWork, error) {
	lib, err := s.repo.Library.FindByID(ctx, libraryID)
	if err != nil {
		return nil, err
	}
	if lib == nil {
		return nil, ErrRatingLibraryNotFound
	}
	var rows []model.Media
	err = s.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Select("id", "library_id", "series_id", "title", "path", "season_num", "episode_num", "tm_db_id", "tm_db_media_type").
		Where("library_id = ? AND scrape_status = ? AND (tm_db_id > 0 OR COALESCE(series_id, '') <> '')", libraryID, "matched").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	type legacySeriesKey struct {
		group string
		id    int
	}
	legacyKey := func(m model.Media) legacySeriesKey {
		if m.SeriesID != "" {
			return legacySeriesKey{"series:" + m.SeriesID, m.TMDbID}
		}
		return legacySeriesKey{"title:" + m.Title, m.TMDbID}
	}
	counts := make(map[legacySeriesKey]int)
	for _, m := range rows {
		if m.TMDbID > 0 && m.TMDbMediaType != "movie" {
			counts[legacyKey(m)]++
		}
	}
	return s.resolveWorks(ctx, lib, rows, func(m model.Media) bool { return counts[legacyKey(m)] > 1 })
}

func (s *ExternalRatingService) resolveWorks(ctx context.Context, lib *model.Library, media []model.Media, sharedLegacyID func(model.Media) bool) ([]ratingWork, error) {
	seriesIDs := make(map[string]int)
	for _, m := range media {
		if m.TMDbMediaType == "" && mediaIsEpisodic(&m, lib) {
			seriesIDs[seriesMetadataStorageID(scrapedSeriesPublicID(&m, m.Title))] = 0
		}
	}
	ids := make([]string, 0, len(seriesIDs))
	for id := range seriesIDs {
		ids = append(ids, id)
	}
	for batch := range slices.Chunk(ids, 200) {
		rows, err := s.repo.Series.FindByIDs(ctx, batch)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			seriesIDs[row.ID] = row.TMDbID
		}
	}
	seen := make(map[ratingWork]bool)
	works := make([]ratingWork, 0)
	for _, m := range media {
		kind, id := m.TMDbMediaType, m.TMDbID
		if kind == "" {
			kind = "movie"
			if mediaIsEpisodic(&m, lib) {
				kind = "tv"
				if seriesID := seriesIDs[seriesMetadataStorageID(scrapedSeriesPublicID(&m, m.Title))]; seriesID > 0 {
					id = seriesID
				} else if id <= 0 || !sharedLegacyID(m) {
					continue // An isolated legacy episode ID is not a reliable series ID.
				}
			}
		}
		work := ratingWork{kind, id}
		if id > 0 && !seen[work] {
			works = append(works, work)
			seen[work] = true
		}
	}
	slices.SortFunc(works, func(a, b ratingWork) int {
		if a.Kind != b.Kind {
			return strings.Compare(a.Kind, b.Kind)
		}
		return a.TMDbID - b.TMDbID
	})
	return works, nil
}

func (s *ExternalRatingService) run(ctx context.Context) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.wake:
		case <-timer.C:
		}
		processed, err := s.processNext(ctx)
		if err != nil && ctx.Err() == nil {
			s.log.Warn("external rating worker", zap.Error(err))
		}
		delay := 15 * time.Second
		if processed {
			delay = time.Second
			// Bursts of new work should not bypass the delay between requests.
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			delay = 0
		}
		timer.Reset(delay)
	}
}

func (s *ExternalRatingService) processNext(ctx context.Context) (bool, error) {
	now := time.Now()
	var row model.ExternalRating
	db := s.repo.DB.WithContext(ctx)
	err := db.Where("status IN ? AND next_attempt_at <= ?", []string{ratingPending, ratingRetry}, now).
		Order("next_attempt_at, kind, tm_db_id").Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	cfg, limit, err := s.configForRatings(ctx)
	if errors.Is(err, ErrOMDbNotConfigured) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	paused, err := s.pauseReason(ctx, cfg.APIKey, limit, now)
	if err != nil || paused != "" {
		return false, err
	}
	lease := now.Truncate(time.Millisecond).Add(time.Minute)
	work := db.Model(&model.ExternalRating{}).Where("kind = ? AND tm_db_id = ?", row.Kind, row.TMDbID)
	claim := work.Session(&gorm.Session{}).Where("status = ? AND next_attempt_at = ?", row.Status, row.NextAttemptAt).
		Update("next_attempt_at", lease)
	if claim.Error != nil || claim.RowsAffected == 0 {
		return false, claim.Error
	}
	updates, err := s.fetchRatings(ctx, cfg, limit, row, now)
	if err != nil {
		return true, fmt.Errorf("update rating lookup: %w", err)
	}
	return true, work.Where("next_attempt_at = ?", lease).Updates(updates).Error
}
