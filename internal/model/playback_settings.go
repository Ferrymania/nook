package model

import "encoding/json"

type PlaybackSettings struct {
	ContentRatingLimit    string   `json:"content_rating_limit"`
	AllowAdult            bool     `json:"allow_adult"`
	PreferredSubtitleLang string   `json:"preferred_subtitle_lang"`
	PreferredAudioLang    string   `json:"preferred_audio_lang"`
	AutoplayNext          bool     `json:"autoplay_next"`
	SkipIntro             bool     `json:"skip_intro"`
	AllowedLibraryIDs     []string `json:"allowed_library_ids"`
}

func (u *User) DecodePlaybackSettings() PlaybackSettings {
	settings := PlaybackSettings{AutoplayNext: true, AllowedLibraryIDs: []string{}}
	if u != nil {
		_ = json.Unmarshal([]byte(u.PlaybackSettings), &settings)
		settings.AllowAdult = !u.HideAdult
	}
	if settings.AllowedLibraryIDs == nil {
		settings.AllowedLibraryIDs = []string{}
	}
	return settings
}
