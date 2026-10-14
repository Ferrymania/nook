package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
)

func TestScraperCompletesMetadataIndependentlyOfProbe(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "success"
		if fail {
			name = "failure"
		}
		t.Run(name, func(t *testing.T) {
			probe, repos, m := newProbeFixture(t)
			lib := model.Library{Base: model.Base{ID: m.LibraryID}, Name: "Movies", Path: filepath.Dir(m.Path), Type: "movie"}
			if err := repos.DB.Create(&lib).Error; err != nil {
				t.Fatal(err)
			}
			m.Path = filepath.Join(lib.Path, "Movie.2026.mkv")
			m.SeasonNum, m.EpisodeNum, m.SeriesID = 0, 0, ""
			if err := os.WriteFile(m.Path, []byte("fixture"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(strings.TrimSuffix(m.Path, ".mkv")+".nfo", []byte(`<movie><title>Probe Movie</title><year>2026</year></movie>`), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := repos.DB.Save(m).Error; err != nil {
				t.Fatal(err)
			}
			result := chapterProbeResult()
			result.Streams = result.Streams[:1] // A silent movie still has valid media information.
			stub := &stubProber{result: result}
			if fail {
				stub.err = errors.New("source refused https://cdn.example.test/video?token=private-value")
			}
			probe.probe = stub
			scraper := NewScraperService(&config.Config{}, zap.NewNop(), repos, nil, nil, nil, nil, NewHub(zap.NewNop())).SetMediaProbe(probe)
			if err := scraper.EnrichOne(t.Context(), m); err != nil {
				t.Fatal(err)
			}
			refreshed, _ := repos.Media.FindByID(t.Context(), m.ID)
			if refreshed == nil || refreshed.ScrapeStatus != "matched" || refreshed.Title != "Probe Movie" {
				t.Fatalf("metadata scrape did not complete: %#v", refreshed)
			}
			row, err := repos.MediaProbe.Get(t.Context(), m.ID)
			if err != nil || row == nil {
				t.Fatalf("probe outcome missing: %v", err)
			}
			if fail {
				if row.LastError == "" || strings.Contains(row.LastError, "private-value") || strings.Contains(row.LastError, "cdn.example") {
					t.Fatalf("failure was not safely recorded: %s", row.LastError)
				}
			} else if row.LastError != "" || refreshed.Width != 3840 || refreshed.DurationSec != 1451 {
				t.Fatalf("technical data not persisted: %#v", row)
			}
			if err := scraper.EnrichOne(t.Context(), refreshed); err != nil {
				t.Fatal(err)
			}
			if stub.callCount() != 1 {
				t.Fatal("another scrape ignored the successful cache or failure cooldown")
			}
		})
	}
}

func TestMediaProbeRemoteConcurrencyIncludesManualRequests(t *testing.T) {
	svc, repos, first := newProbeFixture(t)
	first.Path, first.STRMURL, first.Container = "first.strm", "https://example.test/first.mkv", "strm"
	second := *first
	second.ID, second.Path, second.STRMURL = "second", "second.strm", "https://example.test/second.mkv"
	if err := repos.DB.Save(first).Error; err != nil {
		t.Fatal(err)
	}
	if err := repos.DB.Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	gate := make(chan struct{})
	stub := &stubProber{result: chapterProbeResult(), gate: gate}
	svc.probe = stub
	done := make(chan error, 1)
	go func() { done <- svc.Ensure(t.Context(), first) }()
	waitForCondition(t, time.Second, func() bool { return stub.callCount() == 1 })
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	err := svc.Probe(ctx, &second)
	cancel()
	close(gate)
	if !errors.Is(err, context.DeadlineExceeded) || stub.callCount() != 1 {
		t.Fatalf("remote probes ran concurrently: calls=%d, error=%v", stub.callCount(), err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	row, _ := repos.MediaProbe.Get(t.Context(), second.ID)
	if row != nil {
		t.Fatal("cancellation while waiting for a slot is not a failed probe")
	}
}

func TestMediaProbeTimeoutPersistsFailureAndChangedFileRetries(t *testing.T) {
	svc, repos, m := newProbeFixture(t)
	stub := &stubProber{result: chapterProbeResult(), gate: make(chan struct{})}
	svc.probe = stub
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	err := svc.Ensure(ctx, m)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want a deadline", err)
	}
	row, err := repos.MediaProbe.Get(t.Context(), m.ID)
	if err != nil || row == nil || row.LastError == "" || row.Signature == "" {
		t.Fatalf("expired context prevented failure persistence: row=%#v, error=%v", row, err)
	}
	if err := svc.Ensure(t.Context(), m); err != nil || stub.callCount() != 1 {
		t.Fatal("failed source should stay in cooldown")
	}
	if err := os.WriteFile(m.Path, []byte("changed-file-with-different-size"), 0o600); err != nil {
		t.Fatal(err)
	}
	close(stub.gate)
	if err := svc.Ensure(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	row, _ = repos.MediaProbe.Get(t.Context(), m.ID)
	if stub.callCount() != 2 || row.LastError != "" || row.Payload == "" {
		t.Fatal("changed source must invalidate the old failure cooldown")
	}
}

func TestMediaProbeRejectsEmptySuccessfulResult(t *testing.T) {
	svc, repos, m := newProbeFixture(t)
	svc.probe = &stubProber{result: &FullProbeResult{Container: "matroska"}}
	if err := svc.Ensure(t.Context(), m); err == nil {
		t.Fatal("an empty result is not a successful probe")
	}
	row, _ := repos.MediaProbe.Get(t.Context(), m.ID)
	if row == nil || row.LastError == "" || row.Payload != "" {
		t.Fatal("empty probe result must be recorded as a failure")
	}
}
