package database

import (
	"strings"
	"testing"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/testutil"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestMediaIngestCursorIndex(t *testing.T) {
	db, err := testutil.Open(t, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	var definition string
	if err := db.Raw(`SELECT indexdef FROM pg_indexes WHERE schemaname = current_schema() AND indexname = 'idx_media_ingest_created_active'`).Scan(&definition).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(definition, "(created_at, id)") || !strings.Contains(definition, "deleted_at IS NULL") {
		t.Fatalf("ingest cursor index = %q", definition)
	}
	// The added index must not change the normal media insert behavior.
	if err := db.Create(&model.Media{Title: "新影片", Path: "/new-film.strm"}).Error; err != nil {
		t.Fatal(err)
	}
}
