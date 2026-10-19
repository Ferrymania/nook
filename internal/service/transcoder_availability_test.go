package service

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/config"
)

// Available() gates the local HLS renditions advertised to the player, so it must
// be false both when transcoding is disabled by configuration and when no usable
// ffmpeg can be resolved.
func TestTranscoderAvailableGatesOnConfigAndBinary(t *testing.T) {
	cases := []struct {
		name       string
		enabled    bool
		ffmpegPath string
	}{
		{name: "disabled by config", enabled: false, ffmpegPath: filepath.Join(t.TempDir(), "missing-ffmpeg")},
		{name: "no ffmpeg binary", enabled: true, ffmpegPath: filepath.Join(t.TempDir(), "missing-ffmpeg")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Transcoder.Enabled = tc.enabled
			cfg.App.FFmpegPath = tc.ffmpegPath
			svc := NewTranscoderService(cfg, zap.NewNop(), nil, NewHub(zap.NewNop()))
			svc.findExecutables = func(configured, name string) []string {
				if !tc.enabled {
					t.Fatal("disabled transcoding must not discover or execute ffmpeg")
				}
				return nil
			}

			if svc.Available() {
				t.Fatalf("Available() = true, want false (enabled=%v path=%q)", tc.enabled, tc.ffmpegPath)
			}
		})
	}
}

func transcodeAvailabilityTool(t *testing.T, name, encoder, muxer string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses local shell fixtures")
	}
	path := filepath.Join(t.TempDir(), name)
	script := "#!/bin/sh\ncase \"$2\" in\n" +
		"  -encoders) printf '%s\\n' ' V..... " + encoder + "' ;;\n" +
		"  -muxers|-formats) printf '%s\\n' ' E " + muxer + "' ;;\n" +
		"  *) exit 1 ;;\nesac\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTranscoderAvailableValidatesCandidates(t *testing.T) {
	usable := transcodeAvailabilityTool(t, "ffmpeg", "libx264", "hls")
	noEncoder := transcodeAvailabilityTool(t, "ffmpeg", "mpeg4", "hls")
	noHLS := transcodeAvailabilityTool(t, "ffmpeg", "libx264", "mp4")
	missing := filepath.Join(t.TempDir(), "missing-ffmpeg")
	for _, tc := range []struct {
		name       string
		candidates []string
		wantPath   string
	}{
		{name: "missing executable", candidates: []string{missing}},
		{name: "missing encoder", candidates: []string{noEncoder}},
		{name: "missing HLS muxer", candidates: []string{noHLS}},
		{name: "usable executable", candidates: []string{usable}, wantPath: usable},
		{name: "fallback after unusable candidates", candidates: []string{missing, noEncoder, noHLS, usable}, wantPath: usable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Transcoder.Enabled = true
			cfg.App.FFmpegPath = missing
			svc := NewTranscoderService(cfg, zap.NewNop(), nil, nil)
			svc.findExecutables = func(configured, name string) []string { return tc.candidates }
			if got := svc.Available(); got != (tc.wantPath != "") {
				t.Fatalf("Available() = %v, want %v", got, tc.wantPath != "")
			}
			if tc.wantPath != "" && cfg.App.FFmpegPath != tc.wantPath {
				t.Fatalf("resolved path = %q, want %q", cfg.App.FFmpegPath, tc.wantPath)
			}
		})
	}
}

func TestTranscoderResolveFFmpegPathPrefersConfiguredOrPATH(t *testing.T) {
	configured := transcodeAvailabilityTool(t, "configured-ffmpeg", "libx264", "hls")
	fallback := transcodeAvailabilityTool(t, "ffmpeg", "libx264", "hls")
	t.Setenv("PATH", filepath.Dir(fallback))
	for _, tc := range []struct {
		name       string
		configured string
		want       string
	}{
		{name: "configured executable first", configured: configured, want: configured},
		{name: "missing configured executable falls back to PATH", configured: filepath.Join(t.TempDir(), "missing-ffmpeg"), want: fallback},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.App.FFmpegPath = tc.configured
			svc := NewTranscoderService(cfg, zap.NewNop(), nil, nil)
			got, err := svc.resolveFFmpegPath()
			if err != nil || got != tc.want {
				t.Fatalf("resolveFFmpegPath() = %q, %v, want %q", got, err, tc.want)
			}
		})
	}
}

// A nil service or nil config must not panic: playback info is built on paths
// where wiring may be partial.
func TestTranscoderAvailableNilSafe(t *testing.T) {
	var nilSvc *TranscoderService
	if nilSvc.Available() {
		t.Fatal("nil transcoder should report unavailable")
	}
	svc := &TranscoderService{}
	if svc.Available() {
		t.Fatal("transcoder without config should report unavailable")
	}
}

// Playback info must not advertise local HLS renditions when the playback service
// has no transcoder wired in.
func TestCloud115PlaybackServiceWithoutTranscoderMarksLocalQualitiesUnavailable(t *testing.T) {
	svc := NewCloud115PlaybackService(&config.Config{}, zap.NewNop(), nil, nil)
	if svc.localTranscodeAvailable() {
		t.Fatal("playback service without a transcoder must report local HLS unavailable")
	}
	options := LocalQualityOptions(nil, svc.localTranscodeAvailable())
	if len(options) == 0 {
		t.Fatal("local quality presets should still be listed for the UI")
	}
	for _, option := range options {
		if option.Available {
			t.Fatalf("quality %s advertised as available without a transcoder", option.ID)
		}
	}
}
