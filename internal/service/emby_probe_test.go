package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/service/cloud"
)

func TestManualProbeResolvesSTRMAndExposesMetadata(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a POSIX shell")
	}
	svc := newTestEmbyService(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "movie.strm")
	raw := "/api/strm/play/cloud115/video.mkv?acct=test&pickcode=test"
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	m := model.Media{Base: model.Base{ID: "strm-probe"}, Title: "Movie", Path: path, Container: "strm", SizeBytes: 248}
	if err := svc.repo.DB.Create(&m).Error; err != nil {
		t.Fatal(err)
	}
	argsPath := filepath.Join(dir, "args")
	t.Setenv("NOOK_PROBE_TEST_ARGS", argsPath)
	bin := filepath.Join(dir, "ffprobe")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$NOOK_PROBE_TEST_ARGS\"\ncat <<'JSON'\n" + fullProbeFixture + "\nJSON\n"
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	svc.cfg.App.FFprobePath = bin
	stream := NewStreamService(svc.cfg, svc.log, svc.repo, nil).SetStrmPlayTargetResolver(func(_ context.Context, target, _ string) (*StrmPlayResult, error) {
		if target != raw {
			t.Fatalf("unexpected target %q", target)
		}
		return &StrmPlayResult{Link: &cloud.DirectLink{URL: "https://cdn.example.test/video.mkv", Headers: map[string]string{"User-Agent": "provider-agent"}}}, nil
	})
	if err := stream.Probe(t.Context(), m.ID, NewFFprobeService(svc.cfg, svc.log)); err != nil {
		t.Fatal(err)
	}
	refreshed, _ := svc.repo.Media.FindByID(t.Context(), m.ID)
	if refreshed.SizeBytes != 1451000000 {
		t.Fatal("manual probe did not refresh the media's video size")
	}
	args, _ := os.ReadFile(argsPath)
	if !strings.Contains(string(args), "https://cdn.example.test/video.mkv") || !strings.Contains(string(args), "User-Agent: provider-agent") || strings.Contains(string(args), path) {
		t.Fatalf("unresolved source or missing provider headers: %s", args)
	}
	item, err := svc.Item(t.Context(), m.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if item["Width"] != 3840 || item["Height"] != 2160 || item["RunTimeTicks"] != int64(1451)*10_000_000 {
		t.Fatalf("probe missing from detail: %#v", item)
	}
	if len(item["MediaStreams"].([]map[string]any)) != 3 {
		t.Fatal("detail is missing its top-level media streams")
	}
	info, err := svc.PlaybackInfo(t.Context(), m.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	source := info["MediaSources"].([]map[string]any)[0]
	if source["Size"] != int64(1451000000) || source["Container"] != "mkv" || len(source["MediaStreams"].([]map[string]any)) != 3 {
		t.Fatalf("probe missing from playback info: %#v", source)
	}
	// A failed explicit retry must leave the last successful data usable.
	stream.SetStrmPlayTargetResolver(func(context.Context, string, string) (*StrmPlayResult, error) {
		return nil, errors.New("provider unavailable")
	})
	if err := stream.Probe(t.Context(), m.ID, NewFFprobeService(svc.cfg, svc.log)); err == nil {
		t.Fatal("expected resolution error")
	}
	probe, _ := svc.repo.MediaProbe.Get(t.Context(), m.ID)
	if probe == nil || probe.DurationSec != 1451 {
		t.Fatal("failed retry lost the saved metadata")
	}
}

func TestEmbySTRMUsesKnownHistoryDurationWithoutInventingMetadata(t *testing.T) {
	svc := newTestEmbyService(t)
	m := model.Media{Base: model.Base{ID: "unknown-strm"}, Title: "Movie", Path: "/media/movie.strm", Container: "strm", SizeBytes: 248}
	if err := svc.repo.DB.Create(&m).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.History.Upsert(t.Context(), &model.PlaybackHistory{UserID: "viewer", MediaID: m.ID, PositionMs: 137000}); err != nil {
		t.Fatal(err)
	}
	item, err := svc.Item(t.Context(), m.ID, "viewer")
	if err != nil {
		t.Fatal(err)
	}
	if item["RunTimeTicks"] != int64(0) {
		t.Fatal("unknown duration was fabricated")
	}
	source := item["MediaSources"].([]map[string]any)[0]
	if _, exists := source["Size"]; exists {
		t.Fatal("STRM text size exposed as video size")
	}
	if err := svc.repo.History.Upsert(t.Context(), &model.PlaybackHistory{UserID: "viewer", MediaID: m.ID, PositionMs: 137000, DurationMs: 6491072}); err != nil {
		t.Fatal(err)
	}
	item, err = svc.Item(t.Context(), m.ID, "viewer")
	if err != nil || item["RunTimeTicks"] != int64(6491)*10_000_000 {
		t.Fatalf("known history duration missing: item=%#v err=%v", item, err)
	}
}

func TestEmbyCachedTracksKeepExternalSubtitleRouting(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "Movies", Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	id := writeTempVideoWithSubtitle(t, svc, &lib, ".mkv", ".srt")
	svc.SetSubtitleService(newTestSubtitleService(t, svc))
	m, _ := svc.repo.Media.FindByID(t.Context(), id)
	result, _ := parseFullProbeJSON([]byte(fullProbeFixture))
	row, _ := mediaProbeRow(m, result)
	if err := svc.repo.MediaProbe.Upsert(t.Context(), row); err != nil {
		t.Fatal(err)
	}
	streams := svc.mediaStreams(t.Context(), m)
	if len(streams) != 4 || streams[3]["Index"] != 3 || streams[3]["IsExternal"] != true {
		t.Fatalf("subtitle stream index collision: %#v", streams)
	}
	var body strings.Builder
	if err := svc.ServeSubtitleStream(t.Context(), &body, id, "3", ""); err != nil || !strings.Contains(body.String(), "hello") {
		t.Fatalf("external subtitle no longer served: %v", err)
	}
}
