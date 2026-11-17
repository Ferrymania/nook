package database

import (
	"testing"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/testutil"
	"gorm.io/gorm"
)

func TestLibraryPlaybackThresholdMigration(t *testing.T) {
	db, err := testutil.Open(t, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE libraries (id varchar(36) PRIMARY KEY, name varchar(128) NOT NULL, path varchar(1024) NOT NULL)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO libraries (id, name, path) VALUES ('existing', 'Movies', '/movies')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	var library model.Library
	if err := db.First(&library, "id = ?", "existing").Error; err != nil || library.PlayedThresholdPct != 90 {
		t.Fatalf("migration default: %#v %v", library, err)
	}
	if err := db.Model(&library).Update("played_threshold_pct", 95).Error; err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&library, "id = ?", "existing").Error; err != nil || library.PlayedThresholdPct != 95 {
		t.Fatalf("migration replaced saved threshold: %#v %v", library, err)
	}
}
