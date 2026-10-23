package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/Ferrymania/nook/internal/model"
)

func (p *ProfileService) GetPlaybackSettings(ctx context.Context, userID string) (model.PlaybackSettings, error) {
	user, err := p.repo.User.FindByID(ctx, userID)
	if err != nil {
		return model.PlaybackSettings{}, err
	}
	if user == nil {
		return model.PlaybackSettings{}, errors.New("user not found")
	}
	return user.DecodePlaybackSettings(), nil
}

func (p *ProfileService) UpdatePlaybackSettings(ctx context.Context, userID string, settings model.PlaybackSettings) (model.PlaybackSettings, error) {
	settings.ContentRatingLimit = strings.TrimSpace(settings.ContentRatingLimit)
	switch settings.ContentRatingLimit {
	case "", "G", "PG", "PG-13", "R", "NC-17":
	default:
		return model.PlaybackSettings{}, errors.New("invalid content rating")
	}
	settings.PreferredSubtitleLang = strings.TrimSpace(settings.PreferredSubtitleLang)
	settings.PreferredAudioLang = strings.TrimSpace(settings.PreferredAudioLang)
	for _, lang := range []string{settings.PreferredSubtitleLang, settings.PreferredAudioLang} {
		switch lang {
		case "", "zh", "zh-CN", "en", "ja":
		default:
			return model.PlaybackSettings{}, errors.New("invalid preferred language")
		}
	}
	ids := make([]string, 0, len(settings.AllowedLibraryIDs))
	seen := map[string]bool{}
	for _, id := range settings.AllowedLibraryIDs {
		id = strings.TrimSpace(id)
		if id != "" && !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	settings.AllowedLibraryIDs = ids
	user, err := p.repo.User.FindByID(ctx, userID)
	if err != nil {
		return model.PlaybackSettings{}, err
	}
	if user == nil {
		return model.PlaybackSettings{}, errors.New("user not found")
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		return model.PlaybackSettings{}, err
	}
	if err := p.repo.User.UpdateFields(ctx, userID, map[string]any{
		"playback_settings": string(raw), "hide_adult": !settings.AllowAdult,
	}); err != nil {
		return model.PlaybackSettings{}, err
	}
	return settings, nil
}
