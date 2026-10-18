package config

import (
	"os"
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
