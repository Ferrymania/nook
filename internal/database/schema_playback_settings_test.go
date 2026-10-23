package database

import (
	"testing"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/testutil"
	"gorm.io/gorm"
)

func TestPlaybackSettingsMigrationPreservesDefaultAndAccountLock(t *testing.T) {
	db, err := testutil.Open(t, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"restricted", "locked", "no-default"} {
		if err := db.Create(&model.User{Base: model.Base{ID: id}, Username: id, PasswordHash: "x", HideAdult: true}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Model(&model.User{}).Where("id = ?", "restricted").Update("hide_adult", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE play_profiles (
		id text PRIMARY KEY, user_id text, is_default boolean, deleted_at timestamptz,
		updated_at timestamptz DEFAULT now(), created_at timestamptz DEFAULT now(),
		content_rating_limit text, allow_adult boolean, preferred_subtitle_lang text,
		preferred_audio_lang text, autoplay_next boolean, skip_intro boolean, allowed_library_ids text
	)`).Error; err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		id         string
		def, adult bool
	}{
		{"restricted", true, false}, {"locked", true, true}, {"no-default", false, false},
	} {
		if err := db.Exec(`INSERT INTO play_profiles
			(id, user_id, is_default, content_rating_limit, allow_adult, preferred_subtitle_lang, preferred_audio_lang, autoplay_next, skip_intro, allowed_library_ids)
			VALUES (?, ?, ?, 'PG-13', ?, 'zh-CN', 'ja', false, true, '["lib-a"]')`, row.id, row.id, row.def, row.adult).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := AutoMigrateWithConfig(db, &config.Config{}, nil); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"restricted", "locked"} {
		var user model.User
		if err := db.First(&user, "id = ?", id).Error; err != nil {
			t.Fatal(err)
		}
		settings := user.DecodePlaybackSettings()
		if !user.HideAdult || settings.AllowAdult || settings.AutoplayNext || !settings.SkipIntro || settings.ContentRatingLimit != "PG-13" || settings.PreferredSubtitleLang != "zh-CN" || settings.PreferredAudioLang != "ja" || len(settings.AllowedLibraryIDs) != 1 || settings.AllowedLibraryIDs[0] != "lib-a" {
			t.Fatalf("migration changed effective settings for %s: %+v", id, settings)
		}
	}
	var noDefault model.User
	if err := db.First(&noDefault, "id = ?", "no-default").Error; err != nil {
		t.Fatal(err)
	}
	if noDefault.PlaybackSettings != "" {
		t.Fatal("inactive non-default settings must not replace account defaults")
	}
	if err := db.Model(&model.User{}).Where("id = ?", "restricted").Update("playback_settings", `{"autoplay_next":true,"skip_intro":false}`).Error; err != nil {
		t.Fatal(err)
	}
	if err := migratePlaybackSettings(db); err != nil {
		t.Fatal(err)
	}
	var edited model.User
	if err := db.First(&edited, "id = ?", "restricted").Error; err != nil {
		t.Fatal(err)
	}
	if !edited.DecodePlaybackSettings().AutoplayNext || edited.DecodePlaybackSettings().SkipIntro {
		t.Fatal("restart overwrote edited account settings")
	}
}

func TestFreshInstallDoesNotCreatePlayProfiles(t *testing.T) {
	db, err := testutil.Open(t, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrateWithConfig(db, &config.Config{}, nil); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasTable("play_profiles") {
		t.Fatal("retired profile table was created")
	}
}
