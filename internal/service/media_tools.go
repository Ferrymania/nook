package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func executableCandidates(configured, name string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 12)
	add := func(path string) {
		path = strings.Trim(path, `" `)
		if path == "" {
			return
		}

		if resolved, err := exec.LookPath(path); err == nil {
			path = resolved
		}
		if stat, err := os.Stat(path); err != nil || stat.IsDir() {
			return
		}
		clean, err := filepath.Abs(path)
		if err == nil {
			path = clean
		}
		key := strings.ToLower(path)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, path)
	}

	if configured != "" {
		add(configured)
	}
	add(name)

	return out
}

func resolveLocalExecutable(configured, name string) (string, error) {
	for _, path := range executableCandidates(configured, name) {
		return path, nil
	}
	return "", fmt.Errorf("%s not found at the configured path or in PATH", name)
}

func commandOutput(ctx context.Context, timeout time.Duration, name string, args ...string) ([]byte, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, name, args...) // #nosec G204 -- callers pass paths resolved by resolveLocalExecutable.
	out, err := cmd.CombinedOutput()
	if cmdCtx.Err() != nil {
		return out, cmdCtx.Err()
	}
	return out, err
}
