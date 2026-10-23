package service

import (
	"reflect"
	"testing"

	"github.com/Ferrymania/nook/internal/testutil"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/Ferrymania/nook/internal/model"
)

// newServiceTestDB opens an isolated PostgreSQL schema for service tests.
//
// The full model set is always migrated, on top of any explicitly requested
// models. Service code probes tables that a given test may not care about
// (settings for adult visibility, users for playback preferences, media for library
// counts); with a partial schema those probes fail with "no such table" and the
// default GORM logger floods the test output, burying real failures. Migrating
// everything keeps fixtures faithful to production and the output quiet.
func newServiceTestDB(t *testing.T, models ...any) *gorm.DB {
	t.Helper()
	db, err := testutil.Open(t, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	toMigrate := model.AllModels()
	seen := make(map[string]struct{}, len(toMigrate)+len(models))
	for _, m := range toMigrate {
		seen[reflect.TypeOf(m).String()] = struct{}{}
	}
	for _, m := range models {
		key := reflect.TypeOf(m).String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		toMigrate = append(toMigrate, m)
	}
	if err := db.AutoMigrate(toMigrate...); err != nil {
		t.Fatal(err)
	}
	return db
}
