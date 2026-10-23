package service

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Ferrymania/nook/internal/model"
)

func assertEmbyFolderCounts(t *testing.T, item map[string]any, total, unplayed int) {
	t.Helper()
	encoded, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	var dto struct {
		RecursiveItemCount int
		UserData           struct {
			UnplayedItemCount *int
			Played            bool
			PlayedPercentage  float64
		}
	}
	if err := json.Unmarshal(encoded, &dto); err != nil {
		t.Fatal(err)
	}
	percentage := float64(0)
	if total > 0 {
		percentage = float64(total-unplayed) * 100 / float64(total)
	}
	if dto.RecursiveItemCount != total || dto.UserData.UnplayedItemCount == nil || *dto.UserData.UnplayedItemCount != unplayed ||
		dto.UserData.Played != (total > 0 && unplayed == 0) || dto.UserData.PlayedPercentage != percentage {
		t.Fatalf("want total=%d unplayed=%d, got %s", total, unplayed, encoded)
	}
}

func TestEmbyFolderCountsAcrossEndpointsAndWarmCaches(t *testing.T) {
	svc := newTestEmbyService(t)
	svc.cache = newRuntimeCacheForTest(t, 8)
	libID := seedDiscoveryLibrary(t, svc.repo, "tv")
	for _, id := range []string{"badge-show", "badge-source"} {
		if err := svc.repo.DB.Create(&model.Series{Base: model.Base{ID: id}, LibraryID: libID, Title: id}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, m := range []model.Media{
		{Base: model.Base{ID: "badge-e1"}, SeriesID: "badge-show", SeasonNum: 1, EpisodeNum: 1},
		{Base: model.Base{ID: "badge-e1-4k"}, SeriesID: "badge-show", SeasonNum: 1, EpisodeNum: 1},
		{Base: model.Base{ID: "badge-e2"}, SeriesID: "badge-show", SeasonNum: 1, EpisodeNum: 2},
		{Base: model.Base{ID: "badge-s2e1"}, SeriesID: "badge-show", SeasonNum: 2, EpisodeNum: 1},
		{Base: model.Base{ID: "badge-source-e1"}, SeriesID: "badge-source", SeasonNum: 1, EpisodeNum: 1},
	} {
		m.LibraryID, m.Title, m.Genres = libID, m.SeriesID, "Drama,Crime"
		m.Path = "/media/" + m.SeriesID + "/" + m.ID + ".strm"
		m.DurationSec = 1000
		seedDiscoveryMedia(t, svc.repo, &m)
	}
	for _, h := range []model.PlaybackHistory{
		{UserID: "alice", MediaID: "badge-e1-4k", Completed: true},
		{UserID: "alice", MediaID: "badge-s2e1", Completed: true},
		{UserID: "alice", MediaID: "badge-e2", Completed: false, PositionMs: 500000, DurationMs: 1000000},
	} {
		if err := svc.repo.History.Upsert(t.Context(), &h); err != nil {
			t.Fatal(err)
		}
	}
	getters := map[string]func(string) ([]map[string]any, error){
		"library": func(user string) ([]map[string]any, error) {
			out, err := svc.Items(t.Context(), ItemsParams{UserID: user, ParentID: libID})
			return similarResponseItems(t, out), err
		},
		"item": func(user string) ([]map[string]any, error) {
			out, err := svc.Item(t.Context(), "badge-show", user)
			return []map[string]any{out}, err
		},
		"latest": func(user string) ([]map[string]any, error) {
			return svc.LatestItems(t.Context(), user, libID, 10)
		},
		"similar": func(user string) ([]map[string]any, error) {
			out, err := svc.SimilarItems(t.Context(), "badge-source", user, 10)
			return similarResponseItems(t, out), err
		},
		"mixed types": func(user string) ([]map[string]any, error) {
			out, err := svc.Items(t.Context(), ItemsParams{UserID: user, ParentID: libID, IncludeItemTypes: []string{"Movie", "Series"}})
			return similarResponseItems(t, out), err
		},
		"explicit IDs": func(user string) ([]map[string]any, error) {
			out, err := svc.Items(t.Context(), ItemsParams{UserID: user, IDs: []string{"badge-show"}})
			return similarResponseItems(t, out), err
		},
	}
	for name, get := range getters {
		t.Run(name, func(t *testing.T) {
			for range 2 { // Cold and warm DTO caches must both receive fresh UserData.
				for _, user := range []string{"alice", "bob"} {
					items, err := get(user)
					if err != nil {
						t.Fatal(err)
					}
					found := false
					for _, item := range items {
						if item["Id"] == "badge-show" {
							unplayed := 3
							if user == "alice" {
								unplayed = 1
							}
							assertEmbyFolderCounts(t, item, 3, unplayed)
							if item["ChildCount"] != 2 && item["ChildCount"] != float64(2) {
								t.Fatalf("want two seasons: %#v", item)
							}
							found = true
						}
					}
					if !found {
						t.Fatalf("missing badge-show: %#v", items)
					}
				}
			}
		})
	}
	seasons, err := svc.Items(t.Context(), ItemsParams{UserID: "alice", ParentID: "badge-show"})
	if err != nil {
		t.Fatal(err)
	}
	for _, season := range similarResponseItems(t, seasons) {
		total, unplayed := 2, 1
		if season["IndexNumber"] == float64(2) {
			total, unplayed = 1, 0
		}
		assertEmbyFolderCounts(t, season, total, unplayed)
		detail, err := svc.Item(t.Context(), season["Id"].(string), "alice")
		if err != nil {
			t.Fatal(err)
		}
		assertEmbyFolderCounts(t, detail, total, unplayed)
	}

	// The second write occurs inside the 30s invalidation throttle. Its new
	// count must still reach a cached homepage immediately.
	if err := svc.MarkPlayed(t.Context(), "alice", "badge-e2", true); err != nil {
		t.Fatal(err)
	}
	latest, err := getters["latest"]("alice")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range latest {
		if item["Id"] == "badge-show" {
			assertEmbyFolderCounts(t, item, 3, 0)
		}
	}
	if err := svc.MarkPlayed(t.Context(), "alice", "badge-e2", false); err != nil {
		t.Fatal(err)
	}
	if _, ok := svc.cachedLatestItems(t.Context(), svc.embyLatestCacheKey("alice", libID, 10)); !ok {
		t.Fatal("fixture must keep a warm metadata cache during the throttled write")
	}
	latest, err = getters["latest"]("alice")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range latest {
		if item["Id"] == "badge-show" {
			assertEmbyFolderCounts(t, item, 3, 1)
		}
	}
}

func TestEmbyFolderCountsUseFullVirtualSeriesOutsideLatestWindow(t *testing.T) {
	svc := newTestEmbyService(t)
	svc.cache = newRuntimeCacheForTest(t, 8)
	libID := seedDiscoveryLibrary(t, svc.repo, "tv")
	rows := make([]model.Media, 0, 300)
	for i := 1; i <= 300; i++ {
		rows = append(rows, model.Media{Base: model.Base{ID: fmt.Sprintf("long-%d", i), CreatedAt: time.Now().Add(time.Duration(i) * time.Second)},
			LibraryID: libID, Title: "Long Show", ScrapeStatus: "matched", SeasonNum: 1, EpisodeNum: i,
			Path: fmt.Sprintf("/media/Long Show/Season 01/S01E%03d.strm", i)})
	}
	if err := svc.repo.DB.CreateInBatches(rows, 100).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.History.Upsert(t.Context(), &model.PlaybackHistory{UserID: "alice", MediaID: "long-1", Completed: true}); err != nil {
		t.Fatal(err)
	}
	latest, err := svc.LatestItems(t.Context(), "alice", libID, 1)
	if err != nil || len(latest) != 1 {
		t.Fatalf("latest: %#v %v", latest, err)
	}
	assertEmbyFolderCounts(t, latest[0], 300, 299)
	detail, err := svc.Item(t.Context(), latest[0]["Id"].(string), "alice")
	if err != nil {
		t.Fatal(err)
	}
	assertEmbyFolderCounts(t, detail, 300, 299)
	season, err := svc.Item(t.Context(), seasonID(latest[0]["Id"].(string), 1), "alice")
	if err != nil {
		t.Fatal(err)
	}
	assertEmbyFolderCounts(t, season, 300, 299)
}

func TestEmbyFolderCountsRespectVisibilitySpecialsAndDeletedMedia(t *testing.T) {
	svc := newTestEmbyService(t)
	libID := seedDiscoveryLibrary(t, svc.repo, "anime")
	hidden := seedDiscoveryLibrary(t, svc.repo, "anime")
	user := model.User{Base: model.Base{ID: "restricted"}, Username: "restricted", PasswordHash: "x", AllowedLibraryIDs: fmt.Sprintf("[%q]", libID)}
	if err := svc.repo.DB.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	for _, m := range []model.Media{
		{Base: model.Base{ID: "integer"}, SeasonNum: 1, EpisodeNum: 1},
		{Base: model.Base{ID: "fraction"}, SeasonNum: 1, EpisodeNum: 1, EpisodeFraction: .5},
		{Base: model.Base{ID: "special"}, SeasonNum: 0, EpisodeNum: 1},
		{Base: model.Base{ID: "ova"}, SeasonNum: 0, EpisodeNum: 1, Path: "/media/show/OVA/OVA01.strm"},
		{Base: model.Base{ID: "unknown-a"}, SeasonNum: 1},
		{Base: model.Base{ID: "unknown-b"}, SeasonNum: 1},
		{Base: model.Base{ID: "deleted"}, SeasonNum: 1, EpisodeNum: 2},
		{Base: model.Base{ID: "adult"}, SeasonNum: 1, EpisodeNum: 3, NSFW: true},
		{Base: model.Base{ID: "hidden"}, LibraryID: hidden, SeasonNum: 1, EpisodeNum: 4},
	} {
		if m.LibraryID == "" {
			m.LibraryID = libID
		}
		m.SeriesID, m.Title = "special-show", "Special Show"
		if m.Path == "" {
			m.Path = "/media/show/" + m.ID + ".strm"
		}
		seedDiscoveryMedia(t, svc.repo, &m)
	}
	if err := svc.repo.DB.Delete(&model.Media{}, "id = ?", "deleted").Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"integer", "fraction"} {
		if err := svc.repo.History.Upsert(t.Context(), &model.PlaybackHistory{UserID: user.ID, MediaID: id, Completed: true}); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.repo.DB.Where("media_id = ?", "fraction").Delete(&model.PlaybackHistory{}).Error; err != nil {
		t.Fatal(err)
	}
	item, err := svc.Item(t.Context(), "special-show", user.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertEmbyFolderCounts(t, item, 6, 5)
	if item["ChildCount"] != 3 {
		t.Fatalf("normal, specials and OVA must stay separate: %#v", item)
	}
}

func TestEmbyFolderCountsDoNotMutateSharedPayloadsOrRemoteItems(t *testing.T) {
	svc := newTestEmbyService(t)
	libID := seedDiscoveryLibrary(t, svc.repo, "tv")
	seedDiscoveryMedia(t, svc.repo, &model.Media{Base: model.Base{ID: "shared-ep"}, LibraryID: libID, SeriesID: "shared-show", Title: "Shared", SeasonNum: 1, EpisodeNum: 1, Path: "/media/shared/S01E01.strm"})
	if err := svc.repo.History.Upsert(t.Context(), &model.PlaybackHistory{UserID: "alice", MediaID: "shared-ep", Completed: true}); err != nil {
		t.Fatal(err)
	}
	shared := []map[string]any{
		{"Id": "shared-show", "Type": "Series", "ParentId": libID, "UserData": map[string]any{"IsFavorite": true}},
		{"Id": EncodeEmbyRemoteID("mount", "show"), "Type": "Series", "UserData": map[string]any{"UnplayedItemCount": 42}},
		{"Id": "movie", "Type": "Movie", "UserData": emptyUserData()},
		{"Id": "shared-ep", "Type": "Episode", "UserData": emptyUserData()},
	}
	for _, item := range shared[2:] {
		setEmbyGenres(item, nil)
	}
	before, _ := json.Marshal(shared)
	var wg sync.WaitGroup
	users := []string{"alice", "bob", "alice", "bob"}
	results := make([][]map[string]any, len(users))
	errors := make([]error, len(users))
	for i, user := range users {
		wg.Go(func() {
			results[i], errors[i] = svc.withFolderUserData(t.Context(), user, shared)
		})
	}
	wg.Wait()
	for i, items := range results {
		if errors[i] != nil {
			t.Fatal(errors[i])
		}
		unplayed := 1
		if users[i] == "alice" {
			unplayed = 0
		}
		assertEmbyFolderCounts(t, items[0], 1, unplayed)
		if items[0]["UserData"].(map[string]any)["IsFavorite"] != true {
			t.Error("overlay lost favorite state")
		}
		otherBefore, _ := json.Marshal(shared[1:])
		otherAfter, _ := json.Marshal(items[1:])
		if string(otherBefore) != string(otherAfter) {
			t.Error("remote, movie or episode payload changed")
		}
	}
	after, _ := json.Marshal(shared)
	if string(before) != string(after) {
		t.Fatal("shared metadata payload was mutated")
	}
}
