// Package service — playback history / favourites / playlists.
//
// These three concerns are intentionally co-located: they all sit between
// "the user" and "a media item" and share the same join-table flavour. A
// dedicated PlaybackService keeps the wiring simple and lets handlers
// dispatch by feature instead of by repository.
package service

import (
	"context"
	"errors"
	"sort"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
)

// PlaybackService bundles history / favourite / playlist business logic.
type PlaybackService struct {
	log    *zap.Logger
	repo   *repository.Container
	remote *EmbyRemoteService
}

// NewPlaybackService is the constructor.
func NewPlaybackService(log *zap.Logger, repo *repository.Container) *PlaybackService {
	return &PlaybackService{log: log, repo: repo}
}

// SetEmbyRemote wires the remote Emby service for hydrating mounted remote items.
func (p *PlaybackService) SetEmbyRemote(remote *EmbyRemoteService) *PlaybackService {
	if p != nil {
		p.remote = remote
	}
	return p
}

// ─── History ────────────────────────────────────────────────────────────────

// ProgressUpdate is a playback-progress report. SessionID, SessionStartedAtMs
// and Sequence are optional for compatibility with legacy callers.
type ProgressUpdate struct {
	UserID             string
	MediaID            string
	PositionMs         int64
	DurationMs         int64
	SessionID          string
	SessionStartedAtMs int64
	Sequence           int64
}

// RecordProgress records an unversioned progress report. It is retained for
// older callers that do not provide playback-session metadata.
func (p *PlaybackService) RecordProgress(ctx context.Context, userID, mediaID string, position, duration int64) error {
	return p.RecordProgressUpdate(ctx, ProgressUpdate{
		UserID:     userID,
		MediaID:    mediaID,
		PositionMs: position,
		DurationMs: duration,
	})
}

// RecordProgressUpdate upserts the resume position for a (user, media) pair.
// When session metadata is present, stale reports are ignored so a delayed
// request cannot overwrite a newer position or completion state.
func (p *PlaybackService) RecordProgressUpdate(ctx context.Context, update ProgressUpdate) error {
	if update.UserID == "" || update.MediaID == "" {
		return errors.New("missing user or media")
	}
	dur := p.resolvePlaybackDuration(ctx, update.UserID, update.MediaID, update.DurationMs)
	percent, err := libraryPlayedThresholdPct(ctx, p.repo, update.MediaID)
	if err != nil {
		return err
	}
	h := &model.PlaybackHistory{
		UserID:             update.UserID,
		MediaID:            update.MediaID,
		PositionMs:         update.PositionMs,
		DurationMs:         dur,
		WatchedAt:          time.Now(),
		Completed:          playbackProgressCompleted(update.PositionMs, dur, percent),
		SessionID:          update.SessionID,
		SessionStartedAtMs: update.SessionStartedAtMs,
		Sequence:           update.Sequence,
	}
	return p.repo.History.UpsertProgress(ctx, h)
}

// playbackProgressCompleted uses the media library's completion percentage.
// A positive position is required so an empty report never marks an item played.
func playbackProgressCompleted(position, duration int64, percent int) bool {
	if position <= 0 || duration <= 0 || percent < 1 || percent > 100 {
		return false
	}
	// Round the threshold up to a whole millisecond without overflowing int64.
	threshold := duration/100*int64(percent) + (duration%100*int64(percent)+99)/100
	return position >= threshold
}

func libraryPlayedThresholdPct(ctx context.Context, repo *repository.Container, mediaID string) (int, error) {
	// Mounted remote libraries are read-only and use the default threshold.
	if IsEmbyRemoteID(mediaID) {
		return model.DefaultPlayedThresholdPct, nil
	}
	var percent int
	libraryID := repo.DB.Model(&model.Media{}).Select("library_id").Where("id = ?", mediaID)
	if err := repo.DB.WithContext(ctx).Model(&model.Library{}).Select("played_threshold_pct").
		Where("id = (?)", libraryID).Scan(&percent).Error; err != nil {
		return 0, err
	}
	if percent < 1 || percent > 100 {
		percent = model.DefaultPlayedThresholdPct
	}
	return percent, nil
}

func ValidatePlayedThresholdPct(percent int) error {
	if percent < 1 || percent > 100 {
		return errors.New("播放完成阈值须为 1–100 的整数百分比")
	}
	return nil
}

// GetProgress returns the saved resume row for one media item, or nil when absent.
func (p *PlaybackService) GetProgress(ctx context.Context, userID, mediaID string) (*model.PlaybackHistory, error) {
	if userID == "" || mediaID == "" {
		return nil, errors.New("missing user or media")
	}
	var row model.PlaybackHistory
	err := p.repo.DB.WithContext(ctx).
		Where("user_id = ? AND media_id = ?", userID, mediaID).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (p *PlaybackService) resolvePlaybackDuration(ctx context.Context, userID, mediaID string, duration int64) int64 {
	if duration > 0 {
		return duration
	}
	var existing model.PlaybackHistory
	if err := p.repo.DB.WithContext(ctx).
		Where("user_id = ? AND media_id = ?", userID, mediaID).
		First(&existing).Error; err == nil && existing.DurationMs > 0 {
		return existing.DurationMs
	}
	if m, _ := p.repo.Media.FindByID(ctx, mediaID); m != nil && m.DurationSec > 0 {
		return int64(m.DurationSec) * 1000
	}
	if p.remote != nil && IsEmbyRemoteID(mediaID) {
		mountID, remoteID, _ := DecodeEmbyRemoteID(mediaID)
		if mount, acct, _ := p.remote.ResolveMount(ctx, mountID); mount != nil && acct != nil {
			if rm, err := p.remote.RemoteMediaDetail(ctx, mount, acct, remoteID); err == nil && rm != nil && rm.DurationSec > 0 {
				return int64(rm.DurationSec) * 1000
			}
		}
	}
	return duration
}

// HistoryItem joins the playback row with its media so the API consumer
// gets a fully-populated card without a second round-trip.
type HistoryItem struct {
	model.PlaybackHistory
	Media *model.Media `json:"media,omitempty"`
}

// RecentHistory returns the most recently-watched items for a user. We
// fetch the history rows first then attach each Media row in a single
// follow-up query.
func (p *PlaybackService) RecentHistory(ctx context.Context, userID string, limit int) ([]HistoryItem, error) {
	rows, err := p.repo.History.ListByUser(ctx, userID, limit)
	if err != nil {
		return nil, err
	}
	mediaIDs := make([]string, 0, len(rows))
	for i := range rows {
		if rows[i].MediaID != "" {
			mediaIDs = append(mediaIDs, rows[i].MediaID)
		}
	}
	mediaByID := map[string]model.Media{}
	if len(mediaIDs) > 0 {
		var mediaRows []model.Media
		if err := p.repo.DB.WithContext(ctx).Where("id IN ?", mediaIDs).Find(&mediaRows).Error; err == nil {
			for _, media := range mediaRows {
				mediaByID[media.ID] = media
			}
		}
	}
	items := make([]HistoryItem, 0, len(rows))
	for i := range rows {
		if m, ok := mediaByID[rows[i].MediaID]; ok {
			media := m
			items = append(items, HistoryItem{PlaybackHistory: rows[i], Media: &media})
			continue
		}
		if p.remote != nil && IsEmbyRemoteID(rows[i].MediaID) {
			mountID, remoteID, _ := DecodeEmbyRemoteID(rows[i].MediaID)
			if mount, acct, _ := p.remote.ResolveMount(ctx, mountID); mount != nil && acct != nil {
				if rm, err := p.remote.RemoteMediaDetail(ctx, mount, acct, remoteID); err == nil && rm != nil {
					media := *rm
					items = append(items, HistoryItem{PlaybackHistory: rows[i], Media: &media})
					continue
				}
			}
		}
		items = append(items, HistoryItem{PlaybackHistory: rows[i]})
	}
	return items, nil
}

// ─── Favourites ─────────────────────────────────────────────────────────────

// ToggleFavourite flips the favourite flag and reports the new state.
func (p *PlaybackService) ToggleFavourite(ctx context.Context, userID, mediaID string) (bool, error) {
	current, err := IsUserFavorite(ctx, p.repo, userID, mediaID)
	if err != nil {
		return false, err
	}
	next := !current
	if err := p.SetFavourite(ctx, userID, mediaID, next); err != nil {
		return false, err
	}
	return next, nil
}

// SetFavourite sets favourite state for a media item.
func (p *PlaybackService) SetFavourite(ctx context.Context, userID, mediaID string, favorite bool) error {
	return SyncUserFavorite(ctx, p.repo, p.remote, userID, mediaID, favorite)
}

// ListFavourites returns every favourited media for a user.
func (p *PlaybackService) ListFavourites(ctx context.Context, userID string) ([]model.Media, error) {
	favs, err := p.repo.Favorite.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(favs) == 0 {
		return nil, nil
	}
	sort.Slice(favs, func(i, j int) bool {
		return favs[i].CreatedAt.After(favs[j].CreatedAt)
	})

	localIDs := make([]string, 0, len(favs))
	for _, fav := range favs {
		if !IsEmbyRemoteID(fav.MediaID) {
			localIDs = append(localIDs, fav.MediaID)
		}
	}
	mediaByID := map[string]model.Media{}
	if len(localIDs) > 0 {
		var mediaRows []model.Media
		if err := p.repo.DB.WithContext(ctx).Where("id IN ?", localIDs).Find(&mediaRows).Error; err != nil {
			return nil, err
		}
		for _, media := range mediaRows {
			mediaByID[media.ID] = media
		}
	}

	out := make([]model.Media, 0, len(favs))
	for _, fav := range favs {
		if media, ok := mediaByID[fav.MediaID]; ok {
			out = append(out, media)
			continue
		}
		if p.remote != nil && IsEmbyRemoteID(fav.MediaID) {
			mountID, remoteID, _ := DecodeEmbyRemoteID(fav.MediaID)
			mount, acct, _ := p.remote.ResolveMount(ctx, mountID)
			if mount == nil || acct == nil {
				continue
			}
			remoteMedia, err := p.remote.RemoteMediaDetail(ctx, mount, acct, remoteID)
			if err != nil || remoteMedia == nil {
				continue
			}
			out = append(out, *remoteMedia)
		}
	}
	return out, nil
}

// ListFavouriteIDs returns the user's favourite media IDs without hydrating
// remote Emby items. Heart-icon state only needs the IDs; fetching each remote
// detail on every library open adds a few hundred milliseconds.
func (p *PlaybackService) ListFavouriteIDs(ctx context.Context, userID string) ([]string, error) {
	favs, err := p.repo.Favorite.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(favs))
	for _, fav := range favs {
		if fav.MediaID != "" {
			ids = append(ids, fav.MediaID)
		}
	}
	return ids, nil
}
