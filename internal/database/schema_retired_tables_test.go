package database

import (
	"testing"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/testutil"
	"gorm.io/gorm"
)

func TestMigrationLeavesRetiredReadingTablesUntouched(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "fresh install"
		if existing {
			name = "upgrade"
		}
		t.Run(name, func(t *testing.T) {
			db, err := testutil.Open(t, &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			tables := []string{
				"reader_book_sources", "reader_source_states", "reader_books", "reader_chapters",
				"reader_content_caches", "reader_replace_rules", "reader_profiles", "reader_book_groups",
			}
			if existing {
				// Opaque legacy data: startup must neither migrate nor delete it.
				for _, table := range tables {
					if err := db.Exec("CREATE TABLE " + quoteIdent(table) + " (id text PRIMARY KEY, payload text NOT NULL)").Error; err != nil {
						t.Fatal(err)
					}
					if err := db.Exec("INSERT INTO "+quoteIdent(table)+" (id, payload) VALUES (?, ?)", "legacy", "preserve "+table).Error; err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := AutoMigrateWithConfig(db, &config.Config{}, nil); err != nil {
				t.Fatal(err)
			}
			if !db.Migrator().HasTable("media") {
				t.Fatal("media schema was not initialized")
			}
			for _, table := range tables {
				if got := db.Migrator().HasTable(table); got != existing {
					t.Errorf("table %s exists = %v, want %v", table, got, existing)
				}
				if !existing {
					continue
				}
				var rows []struct{ ID, Payload string }
				if err := db.Table(table).Find(&rows).Error; err != nil {
					t.Fatal(err)
				}
				if len(rows) != 1 || rows[0].ID != "legacy" || rows[0].Payload != "preserve "+table {
					t.Errorf("legacy data in %s changed: %+v", table, rows)
				}
				columns, err := db.Migrator().ColumnTypes(table)
				if err != nil {
					t.Fatal(err)
				}
				if len(columns) != 2 {
					t.Errorf("legacy schema %s was modified: %d columns", table, len(columns))
				}
			}
		})
	}
}
