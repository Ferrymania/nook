package service

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/config"
)

func ffWorkflowTool(t *testing.T, dir, name, version string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses local shell fixtures")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf '%s\\n' '"+name+" version "+version+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func ffWorkflowConfig(t *testing.T) *config.Config {
	t.Helper()
	dir := t.TempDir()
	return &config.Config{App: config.AppConfig{
		DataDir: t.TempDir(), FFmpegPath: ffWorkflowTool(t, dir, "ffmpeg", "old"),
		FFprobePath: ffWorkflowTool(t, dir, "ffprobe", "old"),
	}}
}

func awaitFFWorkflow(t *testing.T, svc *FFmpegToolsService) map[string]any {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		svc.mu.Lock()
		running := svc.running
		svc.mu.Unlock()
		if !running {
			return svc.Status(t.Context())
		}
		select {
		case <-tick.C:
		case <-deadline.C:
			t.Fatal("tool installation did not finish")
		}
	}
}

func TestFFToolsDetectDoesNotApplyPaths(t *testing.T) {
	cfg := ffWorkflowConfig(t)
	oldFFmpeg := cfg.App.FFmpegPath
	bin := t.TempDir()
	foundProbe := ffWorkflowTool(t, bin, "ffprobe", "system")
	// Invalid configured ffprobe must not prevent discovery of the system tool.
	badProbe := filepath.Join(t.TempDir(), "bad-probe")
	if err := os.WriteFile(badProbe, []byte("#!/bin/sh\nprintf 'unrelated program\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg.App.FFprobePath = badProbe
	t.Setenv("PATH", bin)
	svc := NewFFmpegToolsService(cfg, zap.NewNop())
	got := svc.Detect(t.Context())
	if got["ffmpeg"].Path != oldFFmpeg || got["ffprobe"].Path != foundProbe || !got["ffprobe"].Installed {
		t.Fatalf("wrong discovery: %+v", got)
	}
	if cfg.App.FFmpegPath != oldFFmpeg || cfg.App.FFprobePath != badProbe {
		t.Fatal("detection changed runtime configuration")
	}
}

func TestFFToolsPartialDetectionAndDownloadedFallback(t *testing.T) {
	bin := t.TempDir()
	ffmpeg := ffWorkflowTool(t, bin, "ffmpeg", "system")
	t.Setenv("PATH", bin)
	cfg := &config.Config{App: config.AppConfig{DataDir: t.TempDir()}}
	svc := NewFFmpegToolsService(cfg, zap.NewNop())
	got := svc.Detect(t.Context())
	if got["ffmpeg"].Path != ffmpeg || got["ffprobe"].Installed {
		t.Fatalf("wrong partial detection: %+v", got)
	}
	downloadDir := filepath.Join(cfg.App.DataDir, "tools", "ffmpeg", "download-recovered")
	ffWorkflowTool(t, downloadDir, "ffmpeg", "downloaded")
	probe := ffWorkflowTool(t, downloadDir, "ffprobe", "downloaded")
	got = svc.Detect(t.Context())
	if got["ffprobe"].Path != probe || got["ffmpeg"].Path != ffmpeg {
		t.Fatalf("missing download fallback: %+v", got)
	}
	if cfg.App.FFmpegPath != "" || cfg.App.FFprobePath != "" {
		t.Fatal("discovery enabled downloaded tools")
	}
}

func TestFFToolsReuseDoesNotApplyPaths(t *testing.T) {
	cfg := ffWorkflowConfig(t)
	oldFFmpeg, oldFFprobe := cfg.App.FFmpegPath, cfg.App.FFprobePath
	dir := filepath.Join(cfg.App.DataDir, "tools", "ffmpeg")
	downloaded := ffWorkflowTool(t, dir, "ffmpeg", "downloaded")
	ffWorkflowTool(t, dir, "ffprobe", "downloaded")
	svc := NewFFmpegToolsService(cfg, zap.NewNop())
	svc.errMsg = "previous failure"
	svc.installTools = func(context.Context, *zap.Logger, *config.Config, func(string)) (string, string, error) {
		t.Error("cached tools must not download again")
		return "", "", nil
	}
	if err := svc.StartInstall(t.Context()); err != nil {
		t.Fatal(err)
	}
	status := awaitFFWorkflow(t, svc)
	if status["error"] != "" || status["downloaded"].(map[string]ffToolInfo)["ffmpeg"].Path != downloaded {
		t.Fatalf("wrong cached result: %+v", status)
	}
	if status["ffmpeg"].(ffToolInfo).Path != oldFFmpeg || cfg.App.FFmpegPath != oldFFmpeg || cfg.App.FFprobePath != oldFFprobe {
		t.Fatal("reuse switched active tool paths")
	}
}

func ffWorkflowArchive(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, name := range []string{"ffmpeg", "ffprobe"} {
		header := &zip.FileHeader{Name: "bin/" + name, Method: zip.Deflate}
		header.SetMode(0o755)
		file, err := w.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte("#!/bin/sh\nprintf '%s\\n' '" + name + " version downloaded'\n")); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestFFToolsDownloadKeepsActiveFilesAndConfigUntilSave(t *testing.T) {
	cfg := ffWorkflowConfig(t)
	// A partially installed legacy directory may already contain an active tool.
	legacy := filepath.Join(cfg.App.DataDir, "tools", "ffmpeg")
	cfg.App.FFmpegPath = ffWorkflowTool(t, legacy, "ffmpeg", "active")
	oldFFmpeg, oldFFprobe := cfg.App.FFmpegPath, cfg.App.FFprobePath
	before, err := os.ReadFile(oldFFmpeg)
	if err != nil {
		t.Fatal(err)
	}
	archive := ffWorkflowArchive(t)
	var downloads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloads.Add(1)
		_, _ = w.Write(archive)
	}))
	defer server.Close()
	svc := NewFFmpegToolsService(cfg, zap.NewNop())
	svc.installTools = func(ctx context.Context, log *zap.Logger, cfg *config.Config, progress func(string)) (string, string, error) {
		return installFFmpegTarget(ctx, log, cfg, &ffmpegDownloadTarget{Label: "test", Kind: "zip", Archives: []string{server.URL}}, progress)
	}
	if err := svc.StartInstall(t.Context()); err != nil {
		t.Fatal(err)
	}
	status := awaitFFWorkflow(t, svc)
	if status["error"] != "" {
		t.Fatal(status["error"])
	}
	ready := status["downloaded"].(map[string]ffToolInfo)
	if !ready["ffmpeg"].Installed || !ready["ffprobe"].Installed || filepath.Dir(ready["ffmpeg"].Path) == legacy {
		t.Fatalf("download not isolated: %+v", ready)
	}
	if cfg.App.FFmpegPath != oldFFmpeg || cfg.App.FFprobePath != oldFFprobe {
		t.Fatal("download changed runtime paths")
	}
	after, _ := os.ReadFile(oldFFmpeg)
	if !bytes.Equal(before, after) {
		t.Fatal("download overwrote an active executable")
	}
	if !strings.Contains(status["message"].(string), "尚未启用") {
		t.Fatal("result did not explain pending activation")
	}
	// Closing the page/restarting the service preserves files and permits reuse.
	restarted := NewFFmpegToolsService(cfg, zap.NewNop())
	if err := restarted.StartInstall(t.Context()); err != nil {
		t.Fatal(err)
	}
	if restarted.Status(t.Context())["downloaded"].(map[string]ffToolInfo)["ffmpeg"].Path != ready["ffmpeg"].Path || downloads.Load() != 1 {
		t.Fatal("download was not recovered and reused")
	}
	if cfg.App.FFmpegPath != oldFFmpeg {
		t.Fatal("reusing after restart activated the download")
	}
}

func TestFFToolsDownloadFailurePreservesConfigAndCleansStaging(t *testing.T) {
	cfg := ffWorkflowConfig(t)
	oldFFmpeg, oldFFprobe := cfg.App.FFmpegPath, cfg.App.FFprobePath
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("not a zip")) }))
	defer server.Close()
	svc := NewFFmpegToolsService(cfg, zap.NewNop())
	svc.installTools = func(ctx context.Context, log *zap.Logger, cfg *config.Config, progress func(string)) (string, string, error) {
		return installFFmpegTarget(ctx, log, cfg, &ffmpegDownloadTarget{Label: "test", Kind: "zip", Archives: []string{server.URL}}, progress)
	}
	if err := svc.StartInstall(t.Context()); err != nil {
		t.Fatal(err)
	}
	status := awaitFFWorkflow(t, svc)
	if status["error"] == "" || cfg.App.FFmpegPath != oldFFmpeg || cfg.App.FFprobePath != oldFFprobe {
		t.Fatalf("failure changed config or was hidden: %+v", status)
	}
	entries, err := os.ReadDir(svc.ffmpegInstallDir())
	if err != nil || len(entries) != 0 {
		t.Fatalf("incomplete download directory remains: %v %v", entries, err)
	}
}
