package service

import (
	"encoding/json"
	"fmt"
	"testing"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
)

func similarResponseItems(t *testing.T, out map[string]any) []map[string]any {
	t.Helper()
	data, err := json.Marshal(out["Items"])
	if err != nil {
		t.Fatal(err)
	}
	var items []map[string]any
	if err := json.Unmarshal(data, &items); err != nil {
		t.Fatal(err)
	}
	return items
}

func TestSimilarSeriesReturnsWholeShowsWithPostersAndSeasons(t *testing.T) {
	svc := newTestEmbyService(t)
	libID := seedDiscoveryLibrary(t, svc.repo, "tv")
	for _, id := range []string{"source", "widows-bay", "hostage"} {
		series := model.Series{Base: model.Base{ID: id}, LibraryID: libID, Title: id, PosterURL: "https://art.example/" + id + "-poster.jpg", Rating: 8, Year: 2026}
		if err := svc.repo.DB.Create(&series).Error; err != nil {
			t.Fatal(err)
		}
		for season := 1; season <= 2; season++ {
			for episode := 1; episode <= 3; episode++ {
				m := model.Media{LibraryID: libID, SeriesID: id, Title: id, EpisodeTitle: fmt.Sprintf("Episode %d", episode),
					SeasonNum: season, EpisodeNum: episode, Genres: "Drama,Crime", Year: 2026, Rating: 8,
					Path: fmt.Sprintf("/media/%s/S%02dE%02d.strm", id, season, episode), PosterURL: "https://art.example/episode-poster.jpg", EpisodeStillURL: "https://art.example/episode-still.jpg"}
				if err := svc.repo.DB.Create(&m).Error; err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	out, err := svc.SimilarItems(t.Context(), "source", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	items := similarResponseItems(t, out)
	if len(items) != 2 || totalRecordCount(t, out) != 2 {
		t.Fatalf("want two distinct shows, got %#v", out)
	}
	seen := map[string]bool{}
	for _, item := range items {
		id, _ := item["Id"].(string)
		if id == "source" || seen[id] || (id != "widows-bay" && id != "hostage") {
			t.Fatalf("wrong series recommendation: %#v", item)
		}
		seen[id] = true
		if item["Type"] != "Series" || item["IsFolder"] != true || item["Name"] != id {
			t.Fatalf("must return a series detail entry: %#v", item)
		}
		if item["ChildCount"] != float64(2) || item["RecursiveItemCount"] != float64(6) {
			t.Fatalf("recommendation lost seasons/episodes: %#v", item)
		}
		for _, key := range []string{"SeasonName", "IndexNumber", "MediaSources"} {
			if _, ok := item[key]; ok {
				t.Fatalf("series card contains episode field %s", key)
			}
		}
		image, err := svc.ImageURL(t.Context(), id, "Primary")
		if err != nil || image != "https://art.example/"+id+"-poster.jpg" {
			t.Fatalf("wrong series artwork %q: %v", image, err)
		}
		detail, err := svc.Item(t.Context(), id, "")
		if err != nil || detail["Type"] != "Series" {
			t.Fatalf("series card cannot open series detail: %#v %v", detail, err)
		}
	}
}

func TestSimilarSeriesExpandsAfterDeduplication(t *testing.T) {
	svc := newTestEmbyService(t)
	local := seedDiscoveryLibrary(t, svc.repo, "tv")
	other := seedDiscoveryLibrary(t, svc.repo, "tv")
	var sourceID string
	for i := 0; i < 8; i++ {
		seriesID, libID := "local-show", local
		if i == 0 {
			seriesID = "source"
		}
		if i == 7 {
			seriesID, libID = "remote-library-show", other
		}
		m := model.Media{LibraryID: libID, SeriesID: seriesID, Title: seriesID, SeasonNum: 1, EpisodeNum: i + 1, Genres: "Drama", Rating: 8, Path: fmt.Sprintf("/media/%s/%d.strm", seriesID, i)}
		if err := svc.repo.DB.Create(&m).Error; err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			sourceID = m.ID
		}
	}
	for _, visibility := range []MediaVisibility{{IncludeNSFW: true}, {IncludeNSFW: true, AllowedLibraryIDs: []string{local}}} {
		rows, err := svc.discoveryService().SimilarCandidates(t.Context(), sourceID, 2, visibility)
		if err != nil {
			t.Fatal(err)
		}
		want := 2
		if len(visibility.AllowedLibraryIDs) > 0 {
			want = 1
		}
		if len(rows) != want {
			t.Fatalf("deduplication must precede limit/expansion and respect permissions: got %d want %d", len(rows), want)
		}
		if len(rows) == 2 && rows[0].SeriesID == rows[1].SeriesID {
			t.Fatalf("duplicate show consumes recommendation slots: %#v", rows)
		}
	}
}

func TestSimilarVirtualSeriesCacheRetainsWholeShowArtwork(t *testing.T) {
	svc := newTestEmbyService(t)
	svc.SetRuntimeCache(NewRuntimeCacheService(&config.Config{}, zap.NewNop()))
	libID := seedDiscoveryLibrary(t, svc.repo, "tv")
	var sourceID string
	for _, title := range []string{"源剧", "候选剧"} {
		for season := 1; season <= 2; season++ {
			m := model.Media{LibraryID: libID, Title: title, ScrapeStatus: "matched", SeasonNum: season, EpisodeNum: 1, Genres: "Drama", Rating: 8,
				Path: fmt.Sprintf("/media/%s/Season %d/01.strm", title, season), EpisodeStillURL: "https://art.example/still.jpg"}
			if err := svc.repo.DB.Create(&m).Error; err != nil {
				t.Fatal(err)
			}
			if title == "源剧" && season == 1 {
				sourceID = m.ID
			}
		}
	}
	virtualID := stableEmbyID(embyVirtualSeriesPrefix, libID, "候选剧")
	metadata := model.Series{Base: model.Base{ID: seriesMetadataStorageID(virtualID)}, LibraryID: libID, Title: "候选剧", PosterURL: "https://art.example/show-poster.jpg"}
	if err := svc.repo.DB.Create(&metadata).Error; err != nil {
		t.Fatal(err)
	}
	out, err := svc.SimilarItems(t.Context(), sourceID, "", 12)
	if err != nil {
		t.Fatal(err)
	}
	items := similarResponseItems(t, out)
	if len(items) != 1 || items[0]["Id"] != virtualID || items[0]["Type"] != "Series" || items[0]["ChildCount"] != float64(2) {
		t.Fatalf("wrong virtual series recommendation: %#v", items)
	}
	if group, ok := svc.cachedSeriesGroup(virtualID); !ok || len(group.Episodes) != 2 {
		t.Fatalf("virtual series cache must contain both seasons: %#v", group)
	}
	if err := svc.repo.DB.Where("title = ?", "候选剧").Delete(&model.Media{}).Error; err != nil {
		t.Fatal(err)
	}
	svc.virtualMu.Lock()
	svc.virtualArtwork = nil
	svc.virtualSeries = nil
	svc.virtualMu.Unlock()
	cached, err := svc.SimilarItems(t.Context(), sourceID, "", 12)
	if err != nil || totalRecordCount(t, cached) != 1 {
		t.Fatalf("cached result missing: %#v %v", cached, err)
	}
	image, err := svc.ImageURL(t.Context(), virtualID, "Primary")
	if err != nil || image != metadata.PosterURL {
		t.Fatalf("cached series poster was lost: %q %v", image, err)
	}
}

func TestSimilarSeriesHydrationRespectsLibraryPermissions(t *testing.T) {
	svc := newTestEmbyService(t)
	visible := seedDiscoveryLibrary(t, svc.repo, "tv")
	hidden := seedDiscoveryLibrary(t, svc.repo, "tv")
	user := model.User{Base: model.Base{ID: "restricted-user"}, Username: "viewer", PasswordHash: "x", Role: "user", IsActive: true, AllowedLibraryIDs: fmt.Sprintf("[%q]", visible)}
	if err := svc.repo.DB.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	var sourceID string
	for i, libID := range []string{visible, visible, hidden} {
		seriesID, season := "other-show", 1
		if i == 0 {
			seriesID = "source"
		}
		if i == 2 {
			season = 99
		}
		m := model.Media{LibraryID: libID, SeriesID: seriesID, Title: seriesID, SeasonNum: season, EpisodeNum: 1, Genres: "Drama", Path: fmt.Sprintf("/media/%d.strm", i)}
		if err := svc.repo.DB.Create(&m).Error; err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			sourceID = m.ID
		}
	}
	out, err := svc.SimilarItems(t.Context(), sourceID, user.ID, 12)
	if err != nil {
		t.Fatal(err)
	}
	items := similarResponseItems(t, out)
	if len(items) != 1 || items[0]["Id"] != "other-show" || items[0]["ChildCount"] != float64(1) || items[0]["RecursiveItemCount"] != float64(1) {
		t.Fatalf("hidden library episodes leaked into recommended series: %#v", items)
	}
}
