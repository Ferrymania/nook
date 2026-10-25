package database

import (
	"testing"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/testutil"
	"gorm.io/gorm"
)

func TestStartupPreservesCurrentSettingsAndLeavesLegacyProfilesUntouched(t *testing.T) {
	db, err := testutil.Open(t, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatal(err)
	}
	currentSettings := `{"preferred_audio_lang":"ja","autoplay_next":false,"skip_intro":true}`
	for _, user := range []model.User{
		{Base: model.Base{ID: "configured"}, Username: "configured", PasswordHash: "x", HideAdult: true, PlaybackSettings: currentSettings},
		{Base: model.Base{ID: "unconfigured"}, Username: "unconfigured", PasswordHash: "x", HideAdult: true},
	} {
		if err := db.Create(&user).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec(`CREATE TABLE play_profiles (
		id text PRIMARY KEY, user_id text, is_default boolean, deleted_at timestamptz,
		updated_at timestamptz DEFAULT now(), created_at timestamptz DEFAULT now(),
		content_rating_limit text, allow_adult boolean, preferred_subtitle_lang text,
		preferred_audio_lang text, autoplay_next boolean, skip_intro boolean, allowed_library_ids text
	)`).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"configured", "unconfigured"} {
		if err := db.Exec(`INSERT INTO play_profiles (id, user_id, is_default, allow_adult, preferred_audio_lang, autoplay_next)
			VALUES (?, ?, true, true, 'fr', true)`, id, id).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := AutoMigrateWithConfig(db, &config.Config{}, nil); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{"configured": currentSettings, "unconfigured": ""} {
		var user model.User
		if err := db.First(&user, "id = ?", id).Error; err != nil {
			t.Fatal(err)
		}
		if user.PlaybackSettings != want || !user.HideAdult {
			t.Fatalf("startup changed account %s: settings=%q hide_adult=%t", id, user.PlaybackSettings, user.HideAdult)
		}
	}
	var count int64
	if err := db.Table("play_profiles").Where("preferred_audio_lang = 'fr' AND is_default = true").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("legacy profile data was altered: remaining rows=%d", count)
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
