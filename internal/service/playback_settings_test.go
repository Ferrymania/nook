package service

import (
	"testing"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
	"go.uber.org/zap"
)

func TestPlaybackSettingsPersistPerUserAndNarrowAdminAccess(t *testing.T) {
	db := newServiceTestDB(t)
	repos := repository.New(db)
	svc := NewProfileService(zap.NewNop(), repos)
	for _, id := range []string{"viewer-a", "viewer-b"} {
		if err := repos.User.Create(t.Context(), &model.User{Base: model.Base{ID: id}, Username: id, PasswordHash: "x", Role: "user", HideAdult: true, AllowedLibraryIDs: `["lib-a"]`}); err != nil {
			t.Fatal(err)
		}
	}
	settings := model.PlaybackSettings{AutoplayNext: false, SkipIntro: true, PreferredAudioLang: "ja", PreferredSubtitleLang: "zh", AllowedLibraryIDs: []string{"lib-b"}}
	if _, err := svc.UpdatePlaybackSettings(t.Context(), "viewer-a", settings); err != nil {
		t.Fatal(err)
	}
	reloaded, err := svc.GetPlaybackSettings(t.Context(), "viewer-a")
	if err != nil || reloaded.AutoplayNext || !reloaded.SkipIntro || reloaded.PreferredAudioLang != "ja" {
		t.Fatalf("settings = %+v, err = %v", reloaded, err)
	}
	other, err := svc.GetPlaybackSettings(t.Context(), "viewer-b")
	if err != nil || !other.AutoplayNext || other.SkipIntro {
		t.Fatalf("another user's settings changed: %+v %v", other, err)
	}
	vis := UserDefaultMediaVisibility(t.Context(), repos, "viewer-a")
	for _, id := range []string{"lib-a", "lib-b"} {
		if LibraryIDAllowed(vis, id) {
			t.Fatalf("disjoint user selection bypassed administrator access: %+v", vis)
		}
	}
	account := UserAccountMediaVisibility(t.Context(), repos, "viewer-a")
	if !LibraryIDAllowed(account, "lib-a") || LibraryIDAllowed(account, "lib-b") {
		t.Fatalf("settings picker ignored account access: %+v", account)
	}
	settings.AllowedLibraryIDs = []string{"lib-a"}
	if _, err := svc.UpdatePlaybackSettings(t.Context(), "viewer-a", settings); err != nil {
		t.Fatal(err)
	}
	if !LibraryIDAllowed(UserDefaultMediaVisibility(t.Context(), repos, "viewer-a"), "lib-a") {
		t.Fatal("could not restore an account-authorized library")
	}
	settings.PreferredAudioLang = "invalid"
	if _, err := svc.UpdatePlaybackSettings(t.Context(), "viewer-a", settings); err == nil {
		t.Fatal("invalid language accepted")
	}
}

func TestEmbyUserConfigurationUsesAccountPlaybackSettings(t *testing.T) {
	user := &model.User{PlaybackSettings: `{"preferred_audio_lang":"ja","preferred_subtitle_lang":"zh-CN","autoplay_next":false}`}
	configuration := (&EmbyService{}).userPayload(user)["Configuration"].(map[string]any)
	if configuration["EnableNextEpisodeAutoPlay"] != false || configuration["AudioLanguagePreference"] != "ja" || configuration["SubtitleLanguagePreference"] != "zh-CN" {
		t.Fatalf("preferences were not forwarded: %+v", configuration)
	}
}
