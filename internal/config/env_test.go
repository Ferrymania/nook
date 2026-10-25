package config

import "testing"

func TestLoadUsesOnlyNookEnvironment(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("NOOK_APP_DATA_DIR", t.TempDir())
	t.Setenv("NOOK_APP_PORT", "")
	t.Setenv("NOOK_DATABASE_DSN", "")
	t.Setenv("MEBOX_APP_PORT", "9091")
	t.Setenv("MEBOX_DATABASE_DSN", "postgres://legacy@localhost/legacy")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.App.Port != 8080 || cfg.Database.DSN != "" {
		t.Fatalf("retired environment aliases were loaded: port=%d dsn=%q", cfg.App.Port, cfg.Database.DSN)
	}
	t.Setenv("NOOK_APP_PORT", "9092")
	const dsn = "postgres://nook@localhost/current"
	t.Setenv("NOOK_DATABASE_DSN", dsn)
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.App.Port != 9092 || cfg.Database.DSN != dsn {
		t.Fatalf("Nook environment was not loaded: port=%d dsn=%q", cfg.App.Port, cfg.Database.DSN)
	}
}
