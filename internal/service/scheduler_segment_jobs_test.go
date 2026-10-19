package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
)

func TestSegmentPrewarmDefaultsOff(t *testing.T) {
	repos := repository.New(newServiceTestDB(t, &model.Setting{}))
	enabled, err := librarySegmentPrewarmEnabled(t.Context(), repos, &model.Library{})
	if err != nil || enabled {
		t.Fatalf("prewarm must default to off: enabled=%v err=%v", enabled, err)
	}
}

func TestJobSegmentPrewarmSkippedWhenDisabled(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	repos := repository.New(newServiceTestDB(t))
	lib := model.Library{Base: model.Base{ID: "lib-1"}, Name: "Movies", Path: "/movies"}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	if err := repos.DB.Create(&model.Media{
		Base: model.Base{ID: "mv-1"}, LibraryID: lib.ID, Path: "/a.mkv", TMDbID: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}
	segments := NewMediaSegmentService(zap.NewNop(), repos).
		SetIntroDB(NewIntroDBService(zap.NewNop()).SetBaseURL(server.URL)).
		SetPrewarmGap(0)
	scheduler := NewSchedulerService(zap.NewNop(), repos, nil, nil, nil, nil, "")
	scheduler.SetSegments(segments)

	if err := scheduler.jobSegmentPrewarm(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("provider calls = %d, want 0 when prewarm is disabled", got)
	}

	// Upgrades retain the previous global preference until a library saves its own.
	if err := repos.Setting.Set(t.Context(), legacySegmentPrewarmEnabledKey, "true"); err != nil {
		t.Fatal(err)
	}
	if err := scheduler.jobSegmentPrewarm(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("provider calls = %d, want 1 after enabling prewarm", got)
	}
}

func TestJobSegmentPrewarmRespectsLibraryScopeAndKeepsPlaybackQueries(t *testing.T) {
	segments, repos, calls := newSegmentServiceFixture(t, writeJSONBody(introDBMoviePayload))
	segments.SetPrewarmGap(0)
	on, off := true, false
	for _, lib := range []*model.Library{
		{Base: model.Base{ID: "on"}, Name: "Enabled", Path: "/on", ScrapeSettings: &model.LibraryScrapeSettings{SegmentPrewarmEnabled: &on}},
		{Base: model.Base{ID: "off"}, Name: "Opted out", Path: "/off", ScrapeSettings: &model.LibraryScrapeSettings{SegmentPrewarmEnabled: &off}},
		{Base: model.Base{ID: "legacy"}, Name: "Legacy", Path: "/legacy"},
		{Base: model.Base{ID: "disabled"}, Name: "Disabled", Path: "/disabled", ScrapeSettings: &model.LibraryScrapeSettings{SegmentPrewarmEnabled: &on}},
		{Base: model.Base{ID: "deleted"}, Name: "Deleted", Path: "/deleted", ScrapeSettings: &model.LibraryScrapeSettings{SegmentPrewarmEnabled: &on}},
	} {
		if err := repos.Library.Create(t.Context(), lib); err != nil {
			t.Fatal(err)
		}
		if err := repos.DB.Create(&model.Media{Base: model.Base{ID: "m-" + lib.ID}, LibraryID: lib.ID, Path: lib.Path + "/a.mkv", TMDbID: 27205}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := repos.DB.Model(&model.Library{}).Where("id = ?", "disabled").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := repos.DB.Delete(&model.Library{}, "id = ?", "deleted").Error; err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), legacySegmentPrewarmEnabledKey, "true"); err != nil {
		t.Fatal(err)
	}
	scheduler := NewSchedulerService(zap.NewNop(), repos, nil, nil, nil, nil, "")
	scheduler.SetSegments(segments)
	// Manual execution must not bypass the library preference.
	ctx := context.WithValue(t.Context(), schedulerManualRunKey{}, true)
	if err := scheduler.jobSegmentPrewarm(ctx); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(calls); got != 2 {
		t.Fatalf("provider calls=%d, want enabled and inherited libraries only", got)
	}
	for _, id := range []string{"on", "legacy", "off", "disabled", "deleted"} {
		ledger, err := repos.MediaSegment.GetFetch(t.Context(), "m-"+id, IntroDBSource)
		if err != nil || (ledger != nil) != (id == "on" || id == "legacy") {
			t.Fatalf("library %s: unexpected ledger=%#v err=%v", id, ledger, err)
		}
	}
	// An opted-out library still resolves timestamps when its player asks for them.
	m, err := repos.Media.FindByID(t.Context(), "m-off")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := segments.ListForPlayback(t.Context(), m)
	if err != nil || len(rows) == 0 || atomic.LoadInt32(calls) != 3 {
		t.Fatalf("playback lookup was blocked: rows=%#v calls=%d err=%v", rows, atomic.LoadInt32(calls), err)
	}
	if _, err := segments.ListForPlayback(t.Context(), m); err != nil || atomic.LoadInt32(calls) != 3 {
		t.Fatalf("playback did not reuse its cache: calls=%d err=%v", atomic.LoadInt32(calls), err)
	}
}
