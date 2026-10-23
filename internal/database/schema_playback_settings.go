package database

import (
	"encoding/json"

	"github.com/Ferrymania/nook/internal/model"
	"gorm.io/gorm"
)

// Only the previously effective default is migrated. The retired table remains
// untouched for recovery; fresh installations no longer create it. A non-empty
// account setting is never overwritten on subsequent starts.
func migratePlaybackSettings(db *gorm.DB) error {
	if !db.Migrator().HasTable("play_profiles") {
		return nil
	}
	var legacy []struct {
		UserID, ContentRatingLimit, PreferredSubtitleLang, PreferredAudioLang string
		AllowAdult, AutoplayNext, SkipIntro                                   bool
		AllowedLibraryIDs                                                     string
	}
	if err := db.Table("play_profiles").
		Where("is_default = ? AND deleted_at IS NULL", true).
		Order("updated_at DESC, created_at DESC, id DESC").Scan(&legacy).Error; err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, row := range legacy {
			settings := model.PlaybackSettings{
				ContentRatingLimit: row.ContentRatingLimit, AllowAdult: row.AllowAdult,
				PreferredSubtitleLang: row.PreferredSubtitleLang, PreferredAudioLang: row.PreferredAudioLang,
				AutoplayNext: row.AutoplayNext, SkipIntro: row.SkipIntro, AllowedLibraryIDs: []string{},
			}
			if row.AllowedLibraryIDs != "" {
				if err := json.Unmarshal([]byte(row.AllowedLibraryIDs), &settings.AllowedLibraryIDs); err != nil {
					return err
				}
			}
			raw, err := json.Marshal(settings)
			if err != nil {
				return err
			}
			if err := tx.Model(&model.User{}).Where("id = ?", row.UserID).
				Where("playback_settings IS NULL OR playback_settings = ''").
				Updates(map[string]any{
					"playback_settings": string(raw),
					"hide_adult":        gorm.Expr("hide_adult OR ?", !row.AllowAdult),
				}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
