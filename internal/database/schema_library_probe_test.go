package database

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/Ferrymania/nook/internal/model"
)

func TestLibraryAutoProbeMigrationDefaultsExistingLibrariesToOff(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE libraries (
		id varchar(36) PRIMARY KEY,
		name varchar(128) NOT NULL,
		path varchar(1024) NOT NULL,
		type varchar(16) NOT NULL DEFAULT 'movie'
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO libraries (id, name, path) VALUES ('old', 'Movies', '/media/movies')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	var library model.Library
	if err := db.First(&library, "id = ?", "old").Error; err != nil {
		t.Fatal(err)
	}
	if library.AutoProbe {
		t.Fatal("upgrading must not enable probing for existing libraries")
	}
	var nulls int64
	if err := db.Model(&model.Library{}).Where("auto_probe IS NULL").Count(&nulls).Error; err != nil || nulls != 0 {
		t.Fatalf("migration left an unset probe preference: nulls=%d, error=%v", nulls, err)
	}
}
