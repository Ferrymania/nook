package service

import (
	"context"
	"errors"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
)

// LibraryPlaybackSettings separates resume eligibility from the saved watched
// state. Keeping the original history allows settings changes without rewriting
// a user's progress or watch statistics.
type LibraryPlaybackSettings struct {
	PlayedThresholdPct       int `json:"played_threshold_pct"`
	MinResumePct             int `json:"min_resume_pct"`
	MinResumeDurationSeconds int `json:"min_resume_duration_seconds"`
}

func defaultLibraryPlaybackSettings() LibraryPlaybackSettings {
	return LibraryPlaybackSettings{model.DefaultPlayedThresholdPct, model.DefaultMinResumePct, model.DefaultMinResumeDurationSeconds}
}

func playbackSettingsFromLibrary(lib *model.Library) LibraryPlaybackSettings {
	if lib == nil {
		return defaultLibraryPlaybackSettings()
	}
	return LibraryPlaybackSettings{lib.PlayedThresholdPct, lib.MinResumePct, lib.MinResumeDurationSeconds}
}

func ResolveLibraryPlaybackSettings(lib *model.Library, played, minimum, duration *int) (LibraryPlaybackSettings, error) {
	settings := playbackSettingsFromLibrary(lib)
	if played != nil {
		settings.PlayedThresholdPct = *played
	}
	if minimum != nil {
		settings.MinResumePct = *minimum
	}
	if duration != nil {
		settings.MinResumeDurationSeconds = *duration
	}
	return settings, settings.Validate()
}

func (settings LibraryPlaybackSettings) Validate() error {
	if err := ValidatePlayedThresholdPct(settings.PlayedThresholdPct); err != nil {
		return err
	}
	if settings.MinResumePct < 0 || settings.MinResumePct >= settings.PlayedThresholdPct {
		return errors.New("最小续播百分比须为不小于 0 且小于播放完成阈值的整数")
	}
	if settings.MinResumeDurationSeconds < 0 || settings.MinResumeDurationSeconds > 2147483647 {
		return errors.New("最短可续播视频时长须为 0–2147483647 的整数秒数")
	}
	return nil
}

func (settings LibraryPlaybackSettings) resumable(h *model.PlaybackHistory) bool {
	if h == nil || h.Completed || h.PositionMs <= 0 {
		return false
	}
	if h.DurationMs <= 0 {
		return settings.MinResumePct == 0 && settings.MinResumeDurationSeconds == 0
	}
	if h.DurationMs/1000 < int64(settings.MinResumeDurationSeconds) {
		return false
	}
	threshold := h.DurationMs/100*int64(settings.MinResumePct) + (h.DurationMs%100*int64(settings.MinResumePct)+99)/100
	return h.PositionMs >= threshold
}

// resumeHistory changes only the response, never the persisted progress. A
// completed item keeps its saved state even if it is too short to resume.
func (settings LibraryPlaybackSettings) resumeHistory(h *model.PlaybackHistory) *model.PlaybackHistory {
	if h == nil || h.Completed || settings.resumable(h) {
		return h
	}
	copy := *h
	copy.PositionMs = 0
	return &copy
}

type mediaPlaybackPolicy struct {
	MediaID string
	LibraryPlaybackSettings
	DurationSec int64
}

// Load policies once per page, including the metadata duration fallback. Remote
// mounted items have no editable local library and use the same defaults.
func mediaPlaybackPolicies(ctx context.Context, repo *repository.Container, ids []string) (map[string]mediaPlaybackPolicy, error) {
	policies := make(map[string]mediaPlaybackPolicy, len(ids))
	localIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		policies[id] = mediaPlaybackPolicy{MediaID: id, LibraryPlaybackSettings: defaultLibraryPlaybackSettings()}
		if !IsEmbyRemoteID(id) {
			localIDs = append(localIDs, id)
		}
	}
	if len(localIDs) == 0 {
		return policies, nil
	}
	var rows []mediaPlaybackPolicy
	err := repo.DB.WithContext(ctx).Model(&model.Media{}).
		Select(`media.id AS media_id, media.duration_sec,
			COALESCE(l.played_threshold_pct, 90) AS played_threshold_pct,
			COALESCE(l.min_resume_pct, 5) AS min_resume_pct,
			COALESCE(l.min_resume_duration_seconds, 180) AS min_resume_duration_seconds`).
		Joins("LEFT JOIN libraries l ON l.id = media.library_id AND l.deleted_at IS NULL").
		Where("media.id IN ?", localIDs).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		policies[row.MediaID] = row
	}
	return policies, nil
}

func (policy mediaPlaybackPolicy) historyWithDuration(h *model.PlaybackHistory) *model.PlaybackHistory {
	if h == nil || h.DurationMs > 0 || policy.DurationSec <= 0 {
		return h
	}
	copy := *h
	copy.DurationMs = policy.DurationSec * 1000
	return &copy
}

func filterResumeHistories(ctx context.Context, repo *repository.Container, histories []model.PlaybackHistory, includeCompleted bool) ([]model.PlaybackHistory, error) {
	ids := make([]string, 0, len(histories))
	for _, h := range histories {
		ids = append(ids, h.MediaID)
	}
	policies, err := mediaPlaybackPolicies(ctx, repo, ids)
	if err != nil {
		return nil, err
	}
	out := make([]model.PlaybackHistory, 0, len(histories))
	for i := range histories {
		policy := policies[histories[i].MediaID]
		h := policy.historyWithDuration(&histories[i])
		if (includeCompleted && h.Completed) || policy.resumable(h) {
			out = append(out, *h)
		}
	}
	return out, nil
}

func (p *PlaybackService) FilterResumableHistories(ctx context.Context, histories []model.PlaybackHistory) ([]model.PlaybackHistory, error) {
	return filterResumeHistories(ctx, p.repo, histories, false)
}

// GetResumeProgress is the browser player's resume view; GetProgress remains a
// raw history lookup for statistics and explicit history management.
func (p *PlaybackService) GetResumeProgress(ctx context.Context, userID, mediaID string) (*model.PlaybackHistory, error) {
	h, err := p.GetProgress(ctx, userID, mediaID)
	if err != nil || h == nil {
		return h, err
	}
	policies, err := mediaPlaybackPolicies(ctx, p.repo, []string{mediaID})
	if err != nil {
		return nil, err
	}
	policy := policies[mediaID]
	return policy.resumeHistory(policy.historyWithDuration(h)), nil
}
