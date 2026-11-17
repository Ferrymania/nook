package service

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/Ferrymania/nook/internal/model"
)

func assertEmbyPlayedState(t *testing.T, item map[string]any, played bool, position int64, percentage float64) {
	t.Helper()
	if item == nil {
		t.Fatal("missing item")
	}
	data, ok := item["UserData"].(map[string]any)
	count := int64(0)
	if played {
		count = 1
	}
	if !ok || data["Played"] != played || remoteItemInt64(data, "PlayCount") != count ||
		remoteItemInt64(data, "PlaybackPositionTicks") != position*10_000 || data["PlayedPercentage"] != percentage {
		t.Fatalf("want played=%t position=%d percentage=%v, got %#v", played, position, percentage, item["UserData"])
	}
}

func TestEmbyPlayedStateAcrossEndpointsAndWarmCaches(t *testing.T) {
	t.Parallel()

	e := newTestEmbyService(t)
	e.cache = newRuntimeCacheForTest(t, 8)
	ctx := t.Context()
	for _, user := range []string{"alice", "bob"} {
		if err := e.repo.User.Create(ctx, &model.User{Base: model.Base{ID: user}, Username: user, Role: "user", IsActive: true}); err != nil {
			t.Fatal(err)
		}
	}
	for _, fixture := range []struct {
		id      string
		percent int
	}{
		{"strict", 95}, {"lenient", 80},
	} {
		lib := model.Library{Base: model.Base{ID: fixture.id}, Name: fixture.id, Path: "/" + fixture.id, Type: "movie", Enabled: true, PlayedThresholdPct: fixture.percent}
		if err := e.repo.Library.Create(ctx, &lib); err != nil {
			t.Fatal(err)
		}
		m := model.Media{Base: model.Base{ID: fixture.id + "-movie"}, LibraryID: lib.ID, Title: fixture.id + " Movie", Genres: "Drama", DurationSec: 1000, Year: 2024, Path: lib.Path + "/movie.mkv"}
		if err := e.repo.DB.Create(&m).Error; err != nil {
			t.Fatal(err)
		}
		if err := e.SetFavorite(ctx, "alice", m.ID, true); err != nil {
			t.Fatal(err)
		}
	}
	source := model.Media{Base: model.Base{ID: "source"}, LibraryID: "strict", Title: "Source Movie", Genres: "Drama", DurationSec: 1000, Year: 2024, Path: "/strict/source.mkv"}
	if err := e.repo.DB.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	getters := map[string]func(string) ([]map[string]any, error){
		"details": func(user string) ([]map[string]any, error) {
			items := []map[string]any{}
			for _, id := range []string{"strict-movie", "lenient-movie"} {
				item, err := e.Item(ctx, id, user)
				if err != nil {
					return nil, err
				}
				items = append(items, item)
			}
			return items, nil
		},
		"items": func(user string) ([]map[string]any, error) {
			out, err := e.Items(ctx, ItemsParams{UserID: user, IncludeItemTypes: []string{"Movie"}, Recursive: true, Limit: 50})
			return similarResponseItems(t, out), err
		},
		"explicit IDs": func(user string) ([]map[string]any, error) {
			out, err := e.Items(ctx, ItemsParams{UserID: user, IDs: []string{"strict-movie", "lenient-movie"}, Limit: 50})
			return similarResponseItems(t, out), err
		},
		"latest": func(user string) ([]map[string]any, error) {
			return e.LatestItems(ctx, user, "", 20)
		},
		"similar": func(user string) ([]map[string]any, error) {
			out, err := e.SimilarItems(ctx, source.ID, user, 20)
			return similarResponseItems(t, out), err
		},
		"favorites": func(user string) ([]map[string]any, error) {
			out, err := e.Items(ctx, ItemsParams{UserID: user, Filters: []string{"IsFavorite"}, Limit: 50})
			return similarResponseItems(t, out), err
		},
	}
	// Populate metadata caches before web progress reports, which do not evict
	// Emby caches. Both thresholds must still be reflected on the very next read.
	for name, get := range getters {
		for _, user := range []string{"alice", "bob"} {
			if _, err := get(user); err != nil {
				t.Fatalf("warm %s for %s: %v", name, user, err)
			}
		}
	}
	web := NewPlaybackService(e.log, e.repo)
	if err := web.RecordProgress(ctx, "alice", "strict-movie", 920_000, 1_000_000); err != nil {
		t.Fatal(err)
	}
	if err := web.RecordProgress(ctx, "alice", "lenient-movie", 850_000, 1_000_000); err != nil {
		t.Fatal(err)
	}
	for name, get := range getters {
		t.Run(name, func(t *testing.T) {
			for range 2 {
				for _, user := range []string{"alice", "bob"} {
					items, err := get(user)
					if err != nil {
						t.Fatal(err)
					}
					if name == "favorites" && user == "bob" {
						if len(items) != 0 {
							t.Fatalf("favorites leaked to bob: %#v", items)
						}
						continue
					}
					found := map[string]bool{}
					for _, item := range items {
						id, _ := item["Id"].(string)
						if id != "strict-movie" && id != "lenient-movie" {
							continue
						}
						found[id] = true
						position, percentage, played := int64(0), float64(0), false
						if user == "alice" {
							position, percentage = 920_000, 92
							if id == "lenient-movie" {
								position, percentage, played = 850_000, 85, true
							}
						}
						assertEmbyPlayedState(t, item, played, position, percentage)
						if item["UserData"].(map[string]any)["IsFavorite"] != (user == "alice") {
							t.Fatalf("favorite state lost: %#v", item)
						}
					}
					if len(found) != 2 {
						t.Fatalf("missing movie from %s: %#v", name, items)
					}
				}
			}
		})
	}
	resume, err := e.ResumeItems(ctx, "alice", "", 20, 0)
	ids := continueWatchingIDs(t, resume, err)
	if len(ids) != 1 || ids[0] != "strict-movie" {
		t.Fatalf("resume and played state disagree: %v", ids)
	}
}

func TestEmbyManualPlayedStateWithUnknownDurationAndWarmCache(t *testing.T) {
	t.Parallel()

	e := newTestEmbyService(t)
	e.cache = newRuntimeCacheForTest(t, 8)
	ctx := t.Context()
	libID := seedDiscoveryLibrary(t, e.repo, "movie")
	m := model.Media{Base: model.Base{ID: "unknown-runtime"}, LibraryID: libID, Title: "Unknown Movie", Path: "/media/unknown.strm", Container: "strm"}
	seedDiscoveryMedia(t, e.repo, &m)
	if _, err := e.LatestItems(ctx, "viewer", libID, 20); err != nil {
		t.Fatal(err)
	}
	for _, played := range []bool{true, false} {
		if err := e.MarkPlayed(ctx, "viewer", m.ID, played); err != nil {
			t.Fatal(err)
		}
		item, err := e.Item(ctx, m.ID, "viewer")
		if err != nil {
			t.Fatal(err)
		}
		position, percentage := int64(0), float64(0)
		if played {
			position, percentage = 1, 100
		}
		assertEmbyPlayedState(t, item, played, position, percentage)
		items, err := e.LatestItems(ctx, "viewer", libID, 20)
		if err != nil || len(items) != 1 {
			t.Fatalf("latest: %#v %v", items, err)
		}
		assertEmbyPlayedState(t, items[0], played, position, percentage)
	}
}

func TestEmbyEpisodePlayedStateMatchesNextUpAndSeasonCounts(t *testing.T) {
	t.Parallel()

	e := newTestEmbyService(t)
	ctx := t.Context()
	lib := model.Library{Name: "TV", Type: "tv", Path: "/tv", Enabled: true, PlayedThresholdPct: 95}
	if err := e.repo.Library.Create(ctx, &lib); err != nil {
		t.Fatal(err)
	}
	if err := e.repo.DB.Create(&model.Series{Base: model.Base{ID: "strict-show"}, LibraryID: lib.ID, Title: "Strict Show"}).Error; err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{"strict-e1", "strict-e2"} {
		m := model.Media{Base: model.Base{ID: id}, LibraryID: lib.ID, SeriesID: "strict-show", Title: "Strict Show", SeasonNum: 1, EpisodeNum: i + 1, DurationSec: 1000, Path: "/tv/Strict Show/" + id + ".mkv"}
		seedDiscoveryMedia(t, e.repo, &m)
	}
	if err := e.RecordProgress(ctx, "viewer", "strict-e1", 9_200_000_000, 10_000_000_000); err != nil {
		t.Fatal(err)
	}
	item, err := e.Item(ctx, "strict-e1", "viewer")
	if err != nil {
		t.Fatal(err)
	}
	assertEmbyPlayedState(t, item, false, 920_000, 92)
	episodes, err := e.Items(ctx, ItemsParams{UserID: "viewer", ParentID: "strict-show", IncludeItemTypes: []string{"Episode"}, Recursive: true, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	for _, episode := range similarResponseItems(t, episodes) {
		if episode["Id"] == "strict-e1" {
			assertEmbyPlayedState(t, episode, false, 920_000, 92)
		}
	}
	next, err := e.NextUp(ctx, "viewer", "strict-show", 1)
	if err != nil {
		t.Fatal(err)
	}
	items := similarResponseItems(t, next)
	if len(items) != 1 || items[0]["Id"] != "strict-e1" {
		t.Fatalf("unfinished episode must remain next up: %#v", next)
	}
	assertEmbyPlayedState(t, items[0], false, 920_000, 92)
	series, err := e.Item(ctx, "strict-show", "viewer")
	if err != nil {
		t.Fatal(err)
	}
	assertEmbyFolderCounts(t, series, 2, 2)
	if err := e.MarkPlayed(ctx, "viewer", "strict-e1", true); err != nil {
		t.Fatal(err)
	}
	item, err = e.Item(ctx, "strict-e1", "viewer")
	if err != nil {
		t.Fatal(err)
	}
	assertEmbyPlayedState(t, item, true, 1_000_000, 100)
	next, err = e.NextUp(ctx, "viewer", "strict-show", 1)
	if err != nil {
		t.Fatal(err)
	}
	items = similarResponseItems(t, next)
	if len(items) != 1 || items[0]["Id"] != "strict-e2" {
		t.Fatalf("completed episode must advance next up: %#v", next)
	}
	assertEmbyPlayedState(t, items[0], false, 0, 0)
	series, err = e.Item(ctx, "strict-show", "viewer")
	if err != nil {
		t.Fatal(err)
	}
	assertEmbyFolderCounts(t, series, 2, 1)
}

func TestEmbyMediaUserDataDoesNotMutateSharedMetadata(t *testing.T) {
	t.Parallel()

	e := newTestEmbyService(t)
	for _, h := range []model.PlaybackHistory{
		{UserID: "alice", MediaID: "shared", PositionMs: 850_000, Completed: true},
		{UserID: "bob", MediaID: "shared", PositionMs: 920_000, DurationMs: 1_000_000},
	} {
		if err := e.repo.History.Upsert(t.Context(), &h); err != nil {
			t.Fatal(err)
		}
	}
	shared := []map[string]any{
		{"Id": "shared", "Type": "Movie", "RunTimeTicks": float64(10_000_000_000), "UserData": map[string]any{"Played": false, "IsFavorite": true, "Custom": "keep"}},
		{"Id": EncodeEmbyRemoteID("mount", "episode"), "Type": "Episode", "UserData": map[string]any{"Played": true, "PlayCount": 42}},
		{"Id": "series", "Type": "Series", "UserData": map[string]any{"UnplayedItemCount": 12}},
	}
	before, _ := json.Marshal(shared)
	users := []string{"alice", "bob", "alice", "bob", ""}
	results := make([][]map[string]any, len(users))
	errors := make([]error, len(users))
	var wg sync.WaitGroup
	for i, user := range users {
		wg.Go(func() { results[i], errors[i] = e.withMediaUserData(t.Context(), user, shared) })
	}
	wg.Wait()
	for i, user := range users {
		if errors[i] != nil {
			t.Fatal(errors[i])
		}
		position, percentage, played := int64(0), float64(0), false
		if user == "alice" {
			position, percentage, played = 850_000, 85, true
		} else if user == "bob" {
			position, percentage = 920_000, 92
		}
		assertEmbyPlayedState(t, results[i][0], played, position, percentage)
		if results[i][0]["UserData"].(map[string]any)["Custom"] != "keep" {
			t.Fatal("unrelated UserData field lost")
		}
		rest, _ := json.Marshal(results[i][1:])
		want, _ := json.Marshal(shared[1:])
		if string(rest) != string(want) {
			t.Fatal("remote or folder payload changed")
		}
	}
	after, _ := json.Marshal(shared)
	if string(before) != string(after) {
		t.Fatal("shared metadata was mutated")
	}
	var history model.PlaybackHistory
	if err := e.repo.DB.Where("user_id = ?", "alice").First(&history).Error; err != nil || history.DurationMs != 0 {
		t.Fatalf("reading user state rewrote history: %#v %v", history, err)
	}
}
