package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Ferrymania/nook/internal/config"
)

func ffDetectionTool(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf '%s\\n' '"+name+" version fixture'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFFmpegToolsUnavailable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	svc := NewFFmpegToolsService(&config.Config{})
	for _, result := range []map[string]ffToolInfo{svc.Status(t.Context()), svc.Detect(t.Context())} {
		if len(result) != 2 || result["ffmpeg"].Installed || result["ffprobe"].Installed {
			t.Fatalf("missing tools should be unavailable: %+v", result)
		}
	}
}

func TestFFToolsDetectDoesNotApplyPaths(t *testing.T) {
	dir := t.TempDir()
	configured := ffDetectionTool(t, dir, "ffmpeg")
	bin := t.TempDir()
	foundProbe := ffDetectionTool(t, bin, "ffprobe")
	// An unrelated executable in the configured path must not mask the PATH tool.
	badProbe := filepath.Join(dir, "bad-probe")
	if err := os.WriteFile(badProbe, []byte("#!/bin/sh\nprintf 'unrelated program\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{App: config.AppConfig{FFmpegPath: configured, FFprobePath: badProbe}}
	t.Setenv("PATH", bin)
	svc := NewFFmpegToolsService(cfg)
	got := svc.Detect(t.Context())
	if got["ffmpeg"].Path != configured || got["ffprobe"].Path != foundProbe || !got["ffprobe"].Installed {
		t.Fatalf("wrong discovery: %+v", got)
	}
	if cfg.App.FFmpegPath != configured || cfg.App.FFprobePath != badProbe {
		t.Fatal("detection changed runtime configuration")
	}
	if svc.Status(t.Context())["ffprobe"].Installed {
		t.Fatal("detection activated a different executable without saving")
	}
}

func TestFFToolsConfiguredPathsAndPartialDetection(t *testing.T) {
	bin := t.TempDir()
	ffmpeg := ffDetectionTool(t, bin, "ffmpeg")
	t.Setenv("PATH", bin)
	cfg := &config.Config{}
	svc := NewFFmpegToolsService(cfg)
	got := svc.Detect(t.Context())
	if got["ffmpeg"].Path != ffmpeg || got["ffprobe"].Installed {
		t.Fatalf("wrong partial detection: %+v", got)
	}
	// Explicit paths outside PATH continue to work, regardless of installation source.
	probe := ffDetectionTool(t, t.TempDir(), "ffprobe")
	cfg.App.FFprobePath = probe
	for _, result := range []map[string]ffToolInfo{svc.Status(t.Context()), svc.Detect(t.Context())} {
		if result["ffprobe"].Path != probe || !result["ffprobe"].Installed || result["ffmpeg"].Path != ffmpeg {
			t.Fatalf("configured tools not retained: %+v", result)
		}
	}
}
