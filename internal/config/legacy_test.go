package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLegacyEnvironmentAndNookPrecedence(t *testing.T) {
	t.Setenv("MEBOX_APP_PORT", "9091")
	t.Setenv("MEBOX_DATABASE_DSN", "postgres://legacy:secret@localhost/legacy")
	t.Setenv("NOOK_APP_DATA_DIR", t.TempDir())
	cfg, err := Load()
	if err != nil || cfg.App.Port != 9091 || cfg.Database.DSN != Env("NOOK_DATABASE_DSN") {
		t.Fatalf("legacy configuration was lost: cfg=%+v err=%v", cfg, err)
	}
	t.Setenv("NOOK_APP_PORT", "9092")
	t.Setenv("NOOK_DATABASE_DSN", "postgres://current:secret@localhost/current")
	cfg, err = Load()
	if err != nil || cfg.App.Port != 9092 || cfg.Database.DSN != Env("NOOK_DATABASE_DSN") {
		t.Fatalf("Nook configuration did not take precedence: cfg=%+v err=%v", cfg, err)
	}
}

func TestNookSQLiteReusesLegacyDatabase(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "mebox.db")
	current := filepath.Join(dir, "nook.db")
	if err := os.WriteFile(legacy, []byte("existing database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := existingSQLitePath(current); got != legacy {
		t.Fatalf("existing database ignored: %q", got)
	}
	if err := os.WriteFile(current, []byte("current database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := existingSQLitePath(current); got != current {
		t.Fatalf("current database must take precedence: %q", got)
	}
}
