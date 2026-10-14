package config

import (
	"os"
	"path/filepath"
	"strings"
)

// Env prefers Nook configuration while allowing existing installations to
// retain their old environment variables during an image-only upgrade.
func Env(name string) string {
	if value, exists := os.LookupEnv(name); exists {
		return value
	}
	return os.Getenv(legacyEnvName(name))
}

func legacyEnvName(name string) string {
	if strings.HasPrefix(name, "NOOK_") {
		return "MEBOX_" + strings.TrimPrefix(name, "NOOK_")
	}
	return name
}

// Reuse existing data without renaming a SQLite database or its WAL files.
// A new database always takes precedence when both names exist.
func existingSQLitePath(path string) string {
	if filepath.Base(path) != "nook.db" {
		return path
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return path
	}
	for _, name := range []string{"mebox.db", "mmtl.db"} {
		legacy := filepath.Join(filepath.Dir(path), name)
		if info, err := os.Stat(legacy); err == nil && !info.IsDir() {
			return legacy
		}
	}
	return path
}
