package service

import (
	"reflect"
	"testing"
	"time"

	"github.com/Ferrymania/nook/internal/model"
)

func continueWatchingIDs(t *testing.T, out map[string]any, err error) []string {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	items, ok := out["Items"].([]map[string]any)
	if !ok {
		t.Fatalf("invalid Items envelope: %#v", out)
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item["Id"].(string))
	}
	return ids
}

func TestContinueWatchingMergesEpisodesAndPreservesResumeSemantics(t *testing.T) {
	e := newTestEmbyService(t)
	ctx := t.Context()
	for _, uid := range []string{"viewer", "other"} {
		if err := e.repo.User.Create(ctx, &model.User{Base: model.Base{ID: uid}, Username: uid, PasswordHash: "x", Role: "user", IsActive: true}); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"allowed", "hidden"} {
		if err := e.repo.Library.Create(ctx, &model.Library{Base: model.Base{ID: id}, Name: id, Type: "tv", Path: "/" + id, Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.repo.User.UpdateFields(ctx, "viewer", map[string]any{"allowed_library_ids": `["allowed"]`}); err != nil {
		t.Fatal(err)
	}
	for _, m := range []model.Media{
		{Base: model.Base{ID: "movie"}, Title: "Movie", LibraryID: "allowed", Path: "/allowed/movie.mp4"},
		{Base: model.Base{ID: "a1"}, Title: "Show A", LibraryID: "allowed", SeriesID: "show-a", SeasonNum: 1, EpisodeNum: 1, Path: "/allowed/Show A/Season 01/S01E01.mp4"},
		{Base: model.Base{ID: "a2"}, Title: "Show A", LibraryID: "allowed", SeriesID: "show-a", SeasonNum: 1, EpisodeNum: 2, Path: "/allowed/Show A/Season 01/S01E02.mp4"},
		{Base: model.Base{ID: "a3"}, Title: "Show A", LibraryID: "allowed", SeriesID: "show-a", SeasonNum: 2, EpisodeNum: 1, Path: "/allowed/Show A/Season 02/S02E01.mp4"},
		{Base: model.Base{ID: "b1"}, Title: "Show B", LibraryID: "allowed", SeasonNum: 1, EpisodeNum: 1, Path: "/allowed/Show B/Season 01/S01E01.mp4"},
		{Base: model.Base{ID: "b2"}, Title: "Show B", LibraryID: "allowed", SeasonNum: 1, EpisodeNum: 2, Path: "/allowed/Show B/Season 01/S01E02.mp4"},
		{Base: model.Base{ID: "secret"}, Title: "Secret", LibraryID: "hidden", Path: "/hidden/secret.mp4"},
		{Base: model.Base{ID: "deleted"}, Title: "Deleted", LibraryID: "allowed", Path: "/allowed/deleted.mp4"},
	} {
		if err := e.repo.DB.Create(&m).Error; err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	history := func(uid, mid string, position int64, completed bool, minutes int) {
		t.Helper()
		if err := e.repo.History.Upsert(ctx, &model.PlaybackHistory{UserID: uid, MediaID: mid, PositionMs: position, DurationMs: 600000, Completed: completed, WatchedAt: now.Add(-time.Duration(minutes) * time.Minute)}); err != nil {
			t.Fatal(err)
		}
	}
	history("viewer", "movie", 120000, false, 1)
	history("viewer", "a1", 30000, false, 20)
	history("viewer", "a2", 600000, true, 2)
	history("viewer", "b1", 60000, false, 10)
	history("viewer", "b2", 240000, false, 3)
	history("viewer", "secret", 30000, false, 0)
	history("viewer", "deleted", 30000, false, 0)
	history("other", "secret", 50000, false, 0)
	if err := e.repo.DB.Delete(&model.Media{}, "id = ?", "deleted").Error; err != nil {
		t.Fatal(err)
	}
	out, err := e.ResumeItems(ctx, "viewer", "", 20, 0)
	if ids := continueWatchingIDs(t, out, err); !reflect.DeepEqual(ids, []string{"movie", "a3", "b2"}) {
		t.Fatalf("combined order = %v", ids)
	}
	items := out["Items"].([]map[string]any)
	for i, want := range []int64{1200000000, 0, 2400000000} {
		if got := items[i]["UserData"].(map[string]any)["PlaybackPositionTicks"]; got != want {
			t.Fatalf("item %d position = %v, want %d", i, got, want)
		}
	}
	if out["TotalRecordCount"] != int64(3) {
		t.Fatal(out)
	}
	for start, want := range [][]string{{"movie"}, {"a3"}, {"b2"}, {}} {
		page, err := e.ResumeItems(ctx, "viewer", "", 1, start)
		if got := continueWatchingIDs(t, page, err); !reflect.DeepEqual(got, want) || page["TotalRecordCount"] != int64(3) {
			t.Fatalf("page %d = %v, total %v", start, got, page["TotalRecordCount"])
		}
	}
	page, err := e.ResumeItems(ctx, "viewer", "", int(^uint(0)>>1), 1)
	if got := continueWatchingIDs(t, page, err); !reflect.DeepEqual(got, []string{"a3", "b2"}) {
		t.Fatal(got)
	}
	// The raw filter still exposes real resume points, without adding A3.
	raw, err := e.Items(ctx, ItemsParams{UserID: "viewer", Filters: []string{"IsResumable"}, Limit: 100})
	if got := continueWatchingIDs(t, raw, err); !reflect.DeepEqual(got, []string{"movie", "b2", "b1", "a1"}) {
		t.Fatalf("raw IsResumable changed: %v", got)
	}
	scoped, err := e.ResumeItems(ctx, "viewer", "show-a", 20, 0)
	if got := continueWatchingIDs(t, scoped, err); !reflect.DeepEqual(got, []string{"a3"}) {
		t.Fatal(got)
	}
	b, err := e.Item(ctx, "b2", "viewer")
	if err != nil {
		t.Fatal(err)
	}
	scoped, err = e.ResumeItems(ctx, "viewer", b["SeriesId"].(string), 20, 0)
	if got := continueWatchingIDs(t, scoped, err); !reflect.DeepEqual(got, []string{"b2"}) {
		t.Fatalf("virtual series scope: %v", got)
	}
	// Details retain the current, partially watched episode.
	detail, err := e.NextUp(ctx, "viewer", b["SeriesId"].(string), 1)
	if got := continueWatchingIDs(t, detail, err); !reflect.DeepEqual(got, []string{"b2"}) {
		t.Fatal(got)
	}
	// Finishing A must not resurrect the old partial A1. Finishing the movie
	// removes it, while a rewound/resumed episode keeps its actual position.
	history("viewer", "a3", 0, true, 0)
	history("viewer", "movie", 600000, true, 0)
	history("viewer", "b2", 600000, true, 0)
	finished, err := e.ResumeItems(ctx, "viewer", "", 20, 0)
	if got := continueWatchingIDs(t, finished, err); len(got) != 0 {
		t.Fatalf("old partial histories resurrected: %v", got)
	}
	ended, err := e.NextUp(ctx, "viewer", "show-a", 1)
	if got := continueWatchingIDs(t, ended, err); len(got) != 0 {
		t.Fatalf("detail resurrected an old partial: %v", got)
	}
	other, err := e.ResumeItems(ctx, "other", "", 20, 0)
	if got := continueWatchingIDs(t, other, err); !reflect.DeepEqual(got, []string{"secret"}) {
		t.Fatalf("user isolation: %v", got)
	}
	anonymous, err := e.ResumeItems(ctx, "", "", 20, 0)
	if got := continueWatchingIDs(t, anonymous, err); len(got) != 0 {
		t.Fatal(got)
	}
}
