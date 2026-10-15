package service

import (
	"context"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
)

// Store metadata under a UUID without changing the public virtual Series ID or
// attaching a different series_id to existing episodes and playback histories.
func seriesMetadataStorageID(publicID string) string {
	if !strings.HasPrefix(publicID, embyVirtualSeriesPrefix) {
		return publicID
	}
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("nook:series-metadata:"+publicID)).String()
}

func scrapedSeriesPublicID(m *model.Media, title string) string {
	if strings.TrimSpace(m.SeriesID) != "" {
		return m.SeriesID
	}
	name := strings.TrimSpace(embyEpisodeTitleRE.ReplaceAllString(strings.TrimSpace(title), ""))
	name = strings.TrimSpace(embyYearSuffixRE.ReplaceAllString(name, ""))
	return stableEmbyID(embyVirtualSeriesPrefix, m.LibraryID, name)
}

func (s *ScraperService) saveScrapedSeriesMetadata(ctx context.Context, m *model.Media, match *Match, poster, backdrop string) error {
	id := seriesMetadataStorageID(scrapedSeriesPublicID(m, match.Title))
	existing, err := s.repo.Series.FindByID(ctx, id)
	if err != nil {
		return err
	}
	row := model.Series{Base: model.Base{ID: id}, LibraryID: m.LibraryID}
	if existing != nil {
		row = *existing
	}
	row.Title, row.Year, row.Rating = match.Title, match.Year, match.Rating
	row.TMDbID, row.BangumiID = match.TMDbID, match.BangumiID
	row.DoubanID, row.TheTVDBID = match.DoubanID, match.TheTVDBID
	if strings.TrimSpace(match.Overview) != "" {
		row.Overview = match.Overview
	}
	if poster != "" {
		row.PosterURL = poster
	}
	// A failed/empty series image lookup must not promote a legacy episode still.
	if backdrop != "" && backdrop != m.EpisodeStillURL && !legacyTMDbEpisodeStill(m, backdrop) {
		row.BackdropURL = backdrop
	}
	if existing != nil && reflect.DeepEqual(*existing, row) {
		return nil
	}
	row.UpdatedAt = time.Now()
	if err := s.repo.Series.Upsert(ctx, &row); err != nil {
		return err
	}
	if s.onSeriesMetadataChanged != nil {
		s.onSeriesMetadataChanged()
	}
	return nil
}

func legacyTMDbEpisodeStill(m *model.Media, raw string) bool {
	return m != nil && m.TMDbID > 0 && (m.SeasonNum > 0 || m.EpisodeNum > 0) && strings.Contains(raw, "/w500/")
}

func seriesMetadataForGroups(ctx context.Context, repo *repository.Container, groups []embySeriesGroup) (map[string]model.Series, error) {
	ids := make([]string, 0, len(groups))
	for _, group := range groups {
		ids = append(ids, seriesMetadataStorageID(group.ID))
	}
	rows, err := repo.Series.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]model.Series, len(rows))
	for _, row := range rows {
		out[row.ID] = row
	}
	return out, nil
}

// Backfill only missing series records using already-confirmed provider IDs.
// It never re-identifies episodes, rewrites STRM files, or runs a media probe.
func (s *ScraperService) backfillSeriesMetadata(ctx context.Context) {
	if s.tmdb == nil || !s.tmdb.Enabled() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	var candidates []model.Media
	if err := s.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Select("library_id, series_id, title, tm_db_id").Distinct().
		Where("scrape_status = ? AND tm_db_id > 0 AND (season_num > 0 OR episode_num > 0)", "matched").
		Where("library_id IN (SELECT id FROM libraries WHERE deleted_at IS NULL AND LOWER(type) IN ('tv', 'anime', 'variety'))").
		Order("library_id, title, tm_db_id").Find(&candidates).Error; err != nil {
		s.log.Warn("series metadata backfill query failed", zap.Error(err))
		return
	}
	completed := 0
	for _, m := range candidates {
		if ctx.Err() != nil {
			break
		}
		id := seriesMetadataStorageID(scrapedSeriesPublicID(&m, m.Title))
		existing, err := s.repo.Series.FindByID(ctx, id)
		if err != nil {
			s.log.Warn("series metadata backfill lookup failed", zap.Error(err))
			continue
		}
		if existing != nil {
			continue
		}
		lookupCtx, lookupCancel := context.WithTimeout(ctx, tmdbDetailsTimeout)
		match := s.manualTMDbMatchByIDForType(lookupCtx, m.TMDbID, "tv")
		lookupCancel()
		if match == nil {
			s.log.Warn("series metadata backfill provider lookup failed", zap.String("title", m.Title), zap.Int("tmdb_id", m.TMDbID))
			continue
		}
		// Keep the established grouping key/localized name, hence public IDs.
		match.Title = m.Title
		if err := s.saveScrapedSeriesMetadata(ctx, &m, match, match.PosterURL, match.BackdropURL); err != nil {
			s.log.Warn("series metadata backfill save failed", zap.Int("tmdb_id", m.TMDbID), zap.Error(err))
			continue
		}
		completed++
		s.invalidateMediaCache(ctx)
		select {
		case <-ctx.Done():
		case <-time.After(250 * time.Millisecond):
		}
	}
	if completed > 0 {
		s.log.Info("series metadata backfill completed", zap.Int("series_count", completed))
	}
}

func (s *MediaService) attachSeriesMetadataToCards(ctx context.Context, cards []SeriesCard) {
	ids := make([]string, 0, len(cards))
	for _, card := range cards {
		if card.IsSeries || card.Rep.SeasonNum > 0 || card.Rep.EpisodeNum > 0 || card.Rep.SeriesID != "" {
			ids = append(ids, seriesMetadataStorageID(scrapedSeriesPublicID(&card.Rep, card.Rep.Title)))
		}
	}
	rows, err := s.repo.Series.FindByIDs(ctx, ids)
	if err != nil {
		s.log.Warn("load series card metadata failed", zap.Error(err))
		return
	}
	byID := make(map[string]model.Series, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}
	for i := range cards {
		if !cards[i].IsSeries && cards[i].Rep.SeasonNum == 0 && cards[i].Rep.EpisodeNum == 0 && cards[i].Rep.SeriesID == "" {
			continue
		}
		m := &cards[i].Rep
		if row, ok := byID[seriesMetadataStorageID(scrapedSeriesPublicID(m, m.Title))]; ok {
			m.PosterURL, m.BackdropURL, m.Overview = row.PosterURL, row.BackdropURL, row.Overview
			m.Rating, m.Year = row.Rating, row.Year
		}
	}
}
