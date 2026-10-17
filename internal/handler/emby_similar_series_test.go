package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/Ferrymania/nook/internal/model"
)

func TestEmbySimilarSeriesCardsOpenAllSeasons(t *testing.T) {
	router, svc, userID := newEmbyDiscoveryEnv(t)
	if err := svc.Repo.DB.AutoMigrate(&model.Series{}); err != nil {
		t.Fatal(err)
	}
	libID := seedEmbyLibrary(t, svc, "tv")
	for _, id := range []string{"source-series", "recommended-series"} {
		series := model.Series{Base: model.Base{ID: id}, LibraryID: libID, Title: id, PosterURL: "https://art.example/" + id + ".jpg"}
		if err := svc.Repo.DB.Create(&series).Error; err != nil {
			t.Fatal(err)
		}
		for season := 1; season <= 2; season++ {
			for episode := 1; episode <= 2; episode++ {
				m := model.Media{LibraryID: libID, SeriesID: id, Title: id, SeasonNum: season, EpisodeNum: episode, Genres: "Drama", Rating: 8,
					Path: fmt.Sprintf("/media/%s/S%02dE%02d.strm", id, season, episode), EpisodeStillURL: "https://art.example/still.jpg"}
				if err := svc.Repo.DB.Create(&m).Error; err != nil {
					t.Fatal(err)
				}
				if id == "recommended-series" && season == 1 && episode == 1 {
					if err := svc.Repo.History.Upsert(t.Context(), &model.PlaybackHistory{UserID: userID, MediaID: m.ID, Completed: true}); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
	}
	token := signedTestToken(t, "test-secret")
	for _, path := range []string{"/emby/Items/source-series/Similar?Limit=1", "/emby/items/source-series/similar?limit=1"} {
		w := embyGet(t, router, path, token)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		items := decodeItemsEnvelope(t, w.Body.Bytes())
		if len(items) != 1 || items[0]["Id"] != "recommended-series" || items[0]["Type"] != "Series" {
			t.Fatalf("wrong similar cards: %s", w.Body.String())
		}
		if items[0]["IsFolder"] != true || items[0]["ChildCount"] != float64(2) || items[0]["RecursiveItemCount"] != float64(4) {
			t.Fatalf("incomplete show: %#v", items[0])
		}
		if data, _ := items[0]["UserData"].(map[string]any); data["UnplayedItemCount"] != float64(3) {
			t.Fatalf("similar card must include the current user's unplayed count: %#v", items[0])
		}
		if _, ok := items[0]["SeasonName"]; ok {
			t.Fatalf("series card must not have an episode season label: %#v", items[0])
		}
		var detail map[string]any
		w = embyGet(t, router, "/emby/Items/recommended-series", token)
		if err := json.Unmarshal(w.Body.Bytes(), &detail); err != nil {
			t.Fatal(err)
		}
		if w.Code != http.StatusOK || detail["Type"] != "Series" {
			t.Fatalf("wrong detail: %d %s", w.Code, w.Body.String())
		}
		if data, _ := detail["UserData"].(map[string]any); data["UnplayedItemCount"] != float64(3) {
			t.Fatalf("detail lost unplayed count: %#v", detail)
		}
		for _, pair := range []struct {
			path  string
			count int
		}{{"/emby/Shows/recommended-series/Seasons", 2}, {"/emby/Shows/recommended-series/Episodes", 4}} {
			w = embyGet(t, router, pair.path, token)
			if w.Code != http.StatusOK || len(decodeItemsEnvelope(t, w.Body.Bytes())) != pair.count {
				t.Fatalf("series navigation incomplete: %s %d %s", pair.path, w.Code, w.Body.String())
			}
			for _, item := range decodeItemsEnvelope(t, w.Body.Bytes()) {
				if item["Type"] != "Season" {
					continue
				}
				want := float64(2)
				if item["IndexNumber"] == float64(1) {
					want = 1
				}
				if data, _ := item["UserData"].(map[string]any); data["UnplayedItemCount"] != want {
					t.Fatalf("season lost unplayed count: %#v", item)
				}
			}
		}
	}
}
