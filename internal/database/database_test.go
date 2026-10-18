package database

import (
	"os"
	"strings"
	"testing"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/testutil"
	"gorm.io/gorm"
)

func TestOpenRequiresConfig(t *testing.T) {
	db, err := Open(nil, nil)
	if err == nil {
		t.Fatal("expected nil config to return an error")
	}
	if db != nil {
		t.Fatal("db should be nil when config is missing")
	}
	if !strings.Contains(err.Error(), "database config") {
		t.Fatalf("error = %v, want database config message", err)
	}
}

func TestEnsurePerformanceIndexesCreatesHotPathIndexes(t *testing.T) {
	db, err := testutil.Open(t, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Media{}, &model.Favorite{}, &model.PlaybackHistory{}, &model.PlayProfile{}); err != nil {
		t.Fatal(err)
	}
	if err := ensurePerformanceIndexes(db); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"idx_media_library_created_active",
		"idx_media_library_episode_active",
		"idx_favorites_user_media_active",
		"idx_playback_histories_user_media_active",
		"idx_play_profiles_user_created_active",
	} {
		var count int
		if err := db.Raw(`SELECT COUNT(1) FROM pg_indexes WHERE schemaname = current_schema() AND indexname = ?`, name).Scan(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("index %s count = %d, want 1", name, count)
		}
	}
}

// Missing credentials must fail explicitly instead of creating a separate local database.
func TestOpenRequiresPostgresDSN(t *testing.T) {
	for _, kind := range []string{"", "auto", "postgres"} {
		t.Run(kind, func(t *testing.T) {
			cfg := &config.Config{Database: config.DatabaseConfig{Type: kind}}
			db, err := Open(cfg, nil)
			if err == nil || db != nil || !strings.Contains(err.Error(), "database.dsn is required") {
				t.Fatalf("Open() = %v, %v", db, err)
			}
		})
	}
}

func TestOpenRejectsUnsupportedDatabaseEngine(t *testing.T) {
	cfg := &config.Config{Database: config.DatabaseConfig{Type: "unsupported"}}
	db, err := Open(cfg, nil)
	if err == nil || db != nil || !strings.Contains(err.Error(), "supported: postgres") {
		t.Fatalf("Open() = %v, %v", db, err)
	}
}

func TestOpenPostgresConfiguresPool(t *testing.T) {
	dsn := os.Getenv("NOOK_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("requires NOOK_TEST_POSTGRES_DSN")
	}
	cfg := &config.Config{Database: config.DatabaseConfig{Type: "postgres", DSN: dsn, MaxOpenConns: 3, MaxIdleConns: 2}}
	db, err := Open(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if !isPostgres(db) || pool.Stats().MaxOpenConnections != 3 {
		t.Fatalf("incorrect PostgreSQL pool: %s %+v", db.Name(), pool.Stats())
	}
}
