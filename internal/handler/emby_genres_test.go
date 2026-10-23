package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/Ferrymania/nook/internal/model"
)

// Decode actual HTTP responses using Emby's NameLongIdPair shape: a string ID
// here can fail a native client's whole metadata decode, not just its genre row.
func TestEmbyGenresFieldReturnsOfficialPairsAcrossHierarchy(t *testing.T) {
	router, svc, _ := newEmbyDiscoveryEnv(t)
	movieLib := seedEmbyLibrary(t, svc, "movie")
	tvLib := seedEmbyLibrary(t, svc, "tv")
	rows := []model.Media{
		{LibraryID: movieLib, Title: "类型电影", Path: "/media/movie/genre.mkv", Genres: "动作，惊悚"},
		{LibraryID: tvLib, SeriesID: "genre-http-show", Title: "类型剧", Path: "/media/tv/genre/S01E01.mkv", SeasonNum: 1, EpisodeNum: 1, Genres: "剧情"},
		{LibraryID: tvLib, SeriesID: "genre-http-show", Title: "类型剧", Path: "/media/tv/genre/S02E01.mkv", SeasonNum: 2, EpisodeNum: 1},
	}
	for i := range rows {
		if err := svc.Repo.DB.Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	token := signedTestToken(t, "test-secret")
	type genreItem struct {
		Name string
		ID   int64 `json:"Id"`
	}
	type item struct {
		ID         string `json:"Id"`
		Genres     []string
		GenreItems []genreItem
	}
	catalogue := embyGet(t, router, "/emby/Genres", token)
	var genres struct {
		Items []struct{ ID, Name string }
	}
	if catalogue.Code != http.StatusOK || json.Unmarshal(catalogue.Body.Bytes(), &genres) != nil || len(genres.Items) != 3 {
		t.Fatalf("genre catalogue: %s", catalogue.Body.String())
	}
	ids := map[string]int64{}
	for _, genre := range genres.Items {
		id, err := strconv.ParseInt(genre.ID, 10, 64)
		if err != nil || id <= 0 {
			t.Fatalf("catalogue ID must match numeric GenreItems: %#v", genre)
		}
		ids[genre.Name] = id
	}
	assertGenres := func(dto item, names ...string) {
		t.Helper()
		if dto.Genres == nil || dto.GenreItems == nil || len(dto.Genres) != len(names) || len(dto.GenreItems) != len(names) {
			t.Fatalf("genre fields missing for %s: %#v", dto.ID, dto)
		}
		for i, name := range names {
			if dto.Genres[i] != name || dto.GenreItems[i].Name != name || dto.GenreItems[i].ID != ids[name] {
				t.Fatalf("genre fields disagree for %s: %#v", dto.ID, dto)
			}
		}
	}
	for _, check := range []struct {
		id     string
		genres []string
	}{
		{rows[0].ID, []string{"动作", "惊悚"}},
		{"genre-http-show", []string{"剧情"}},
		{rows[2].ID, []string{"剧情"}},
	} {
		for _, prefix := range []string{"/emby/Items/", "/emby/Users/user-1/Items/"} {
			response := embyGet(t, router, prefix+check.id+"?Fields=Genres", token)
			var dto item
			if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &dto) != nil {
				t.Fatalf("item response: %s", response.Body.String())
			}
			assertGenres(dto, check.genres...)
		}
	}
	response := embyGet(t, router, "/emby/Shows/genre-http-show/Seasons?UserId=user-1&Fields=Genres", token)
	var seasons struct{ Items []item }
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &seasons) != nil || len(seasons.Items) != 2 {
		t.Fatalf("season response: %s", response.Body.String())
	}
	for _, season := range seasons.Items {
		assertGenres(season, "剧情")
	}
}
