package service

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/Ferrymania/nook/internal/model"
)

func assertEmbyGenres(t *testing.T, item map[string]any, names ...string) {
	t.Helper()
	raw, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	var dto struct {
		Genres     []string
		GenreItems []struct {
			Name string
			ID   int64 `json:"Id"`
		}
	}
	if err := json.Unmarshal(raw, &dto); err != nil {
		t.Fatalf("official NameLongIdPair DTO cannot decode: %v", err)
	}
	if dto.Genres == nil || dto.GenreItems == nil || len(dto.Genres) != len(names) || len(dto.GenreItems) != len(names) {
		t.Fatalf("want non-null genres %v, got %s", names, raw)
	}
	for i, name := range names {
		if dto.Genres[i] != name || dto.GenreItems[i].Name != name || dto.GenreItems[i].ID != embyGenreNumber(name) || dto.GenreItems[i].ID <= 0 {
			t.Fatalf("inconsistent genre metadata, want %v: %s", names, raw)
		}
	}
}

func TestEmbyGenreOfficialShapeAndNormalization(t *testing.T) {
	item := map[string]any{}
	setEmbyGenres(item, []string{"28, Action，剧情;18;action ", "剧情、喜剧；35", ""})
	assertEmbyGenres(t, item, "动作", "剧情", "喜剧")
	if embyGenreNumber(" ACTION ") != embyGenreNumber("动作") || embyGenreNumber("28") != embyGenreNumber("动作") {
		t.Fatal("aliases/case/whitespace changed stable genre ID")
	}
	for _, genre := range []string{"剧情", "Action", "Science Fiction"} {
		id, err := strconv.ParseInt(embyGenreID(genre), 10, 64)
		if err != nil || id != embyGenreNumber(genre) || id > (1<<53)-1 {
			t.Fatalf("catalogue and GenreItems IDs differ: %q", genre)
		}
	}
	setEmbyGenres(item, nil)
	assertEmbyGenres(t, item)
	var svc *EmbyService
	empty := []map[string]any{}
	out, err := svc.withMediaGenres(t.Context(), "", empty)
	if err != nil || out == nil || len(out) != 0 {
		t.Fatal("empty response became null")
	}
}

func TestEmbyGenreMetadataAcrossMovieEndpointsAndCache(t *testing.T) {
	svc := newTestEmbyService(t)
	svc.cache = newRuntimeCacheForTest(t, 8)
	libID := seedDiscoveryLibrary(t, svc.repo, "movie")
	m := model.Media{LibraryID: libID, Title: "类型测试电影", Genres: "28,Action，惊悚;动作,Thriller,53", Path: "/media/genres/movie.mkv"}
	seedDiscoveryMedia(t, svc.repo, &m)
	for range 2 {
		item, err := svc.Item(t.Context(), m.ID, "")
		if err != nil {
			t.Fatal(err)
		}
		assertEmbyGenres(t, item, "动作", "惊悚")
		out, err := svc.Items(t.Context(), ItemsParams{ParentID: libID})
		if err != nil {
			t.Fatal(err)
		}
		assertEmbyGenres(t, similarResponseItems(t, out)[0], "动作", "惊悚")
		latest, err := svc.LatestItems(t.Context(), "", libID, 10)
		if err != nil || len(latest) != 1 {
			t.Fatalf("latest: %v %v", latest, err)
		}
		assertEmbyGenres(t, latest[0], "动作", "惊悚")
	}
	catalogue, err := svc.Genres(t.Context(), "", libID)
	if err != nil {
		t.Fatal(err)
	}
	for _, genre := range similarResponseItems(t, catalogue) {
		if genre["Id"] != embyGenreID(genre["Name"].(string)) {
			t.Fatalf("catalogue uses a different ID: %#v", genre)
		}
	}
}

func TestEmbySeriesSeasonEpisodeGenresAndVisibility(t *testing.T) {
	svc := newTestEmbyService(t)
	svc.cache = newRuntimeCacheForTest(t, 8)
	libID := seedDiscoveryLibrary(t, svc.repo, "tv")
	hidden := seedDiscoveryLibrary(t, svc.repo, "tv")
	user := model.User{Base: model.Base{ID: "genre-viewer"}, Username: "genre-viewer", Role: "user", PasswordHash: "x", HideAdult: true, AllowedLibraryIDs: fmt.Sprintf("[%q]", libID)}
	if err := svc.repo.DB.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.DB.Create(&model.Series{Base: model.Base{ID: "genre-show"}, LibraryID: libID, Title: "类型测试剧"}).Error; err != nil {
		t.Fatal(err)
	}
	for i, row := range []model.Media{
		{Base: model.Base{ID: "genre-s1e1"}, SeasonNum: 1, EpisodeNum: 1, Genres: "18,剧情，Crime;80;剧情"},
		{Base: model.Base{ID: "genre-s1e2"}, SeasonNum: 1, EpisodeNum: 2},
		{Base: model.Base{ID: "genre-s2e1"}, SeasonNum: 2, EpisodeNum: 1, Genres: "喜剧"},
		{Base: model.Base{ID: "genre-s3e1"}, SeasonNum: 3, EpisodeNum: 1},
		{Base: model.Base{ID: "genre-adult"}, SeasonNum: 4, EpisodeNum: 1, Genres: "成人类型", NSFW: true},
		{Base: model.Base{ID: "genre-hidden"}, LibraryID: hidden, SeasonNum: 5, EpisodeNum: 1, Genres: "未授权类型"},
		{Base: model.Base{ID: "genre-deleted"}, SeasonNum: 6, EpisodeNum: 1, Genres: "已删除类型"},
	} {
		if row.LibraryID == "" {
			row.LibraryID = libID
		}
		row.SeriesID, row.Title = "genre-show", "类型测试剧"
		row.Path = "/media/genres/" + row.ID + ".strm"
		row.CreatedAt = time.Now().Add(time.Duration(i) * time.Minute)
		seedDiscoveryMedia(t, svc.repo, &row)
	}
	if err := svc.repo.DB.Delete(&model.Media{}, "id = ?", "genre-deleted").Error; err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		id     string
		genres []string
	}{
		{"genre-show", []string{"剧情", "犯罪", "喜剧"}},
		{seasonID("genre-show", 1), []string{"剧情", "犯罪"}},
		{seasonID("genre-show", 2), []string{"喜剧"}},
		{seasonID("genre-show", 3), []string{"剧情", "犯罪", "喜剧"}},
		{"genre-s1e1", []string{"剧情", "犯罪"}},
		{"genre-s1e2", []string{"剧情", "犯罪", "喜剧"}},
	}
	for range 2 {
		for _, check := range checks {
			item, err := svc.Item(t.Context(), check.id, user.ID)
			if err != nil || item == nil {
				t.Fatalf("%s: %v %v", check.id, item, err)
			}
			assertEmbyGenres(t, item, check.genres...)
		}
		latest, err := svc.LatestItems(t.Context(), user.ID, libID, 1)
		if err != nil || len(latest) != 1 {
			t.Fatalf("latest: %v %v", latest, err)
		}
		assertEmbyGenres(t, latest[0], "剧情", "犯罪", "喜剧")
		out, err := svc.Items(t.Context(), ItemsParams{UserID: user.ID, ParentID: "genre-show", IncludeItemTypes: []string{"Episode"}, Recursive: true})
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range similarResponseItems(t, out) {
			if item["Id"] == "genre-s1e2" {
				assertEmbyGenres(t, item, "剧情", "犯罪", "喜剧")
			}
		}
	}
}

func TestEmbyGenreFallbackVirtualSeriesAndSharedPayloads(t *testing.T) {
	svc := newTestEmbyService(t)
	libID := seedDiscoveryLibrary(t, svc.repo, "tv")
	rows := []model.Media{
		{Base: model.Base{ID: "virtual-genre-1"}, Title: "虚拟类型剧", SeasonNum: 1, EpisodeNum: 1, Genres: "剧情"},
		{Base: model.Base{ID: "virtual-genre-2"}, Title: "虚拟类型剧", SeasonNum: 1, EpisodeNum: 2},
		{Base: model.Base{ID: "virtual-other"}, Title: "其他剧", SeasonNum: 1, EpisodeNum: 1, Genres: "不相关类型"},
	}
	for i := range rows {
		rows[i].LibraryID, rows[i].ScrapeStatus = libID, "matched"
		rows[i].Path = fmt.Sprintf("/media/%s/S01E%02d.strm", rows[i].Title, rows[i].EpisodeNum)
		seedDiscoveryMedia(t, svc.repo, &rows[i])
	}
	item, err := svc.Item(t.Context(), rows[1].ID, "")
	if err != nil {
		t.Fatal(err)
	}
	assertEmbyGenres(t, item, "剧情")
	shared := []map[string]any{
		{"Id": rows[1].ID, "Type": "Episode", "SeriesId": item["SeriesId"], "Genres": []any{}},
		{"Id": EncodeEmbyRemoteID("mount", "remote"), "Type": "Episode", "GenreItems": []any{map[string]any{"Name": "远端", "Id": 123}}},
	}
	before, _ := json.Marshal(shared)
	out, err := svc.withMediaGenres(t.Context(), "", shared)
	if err != nil {
		t.Fatal(err)
	}
	assertEmbyGenres(t, out[0], "剧情")
	if !reflect.DeepEqual(out[1], shared[1]) {
		t.Fatal("remote genre IDs were replaced")
	}
	after, _ := json.Marshal(shared)
	if string(before) != string(after) {
		t.Fatal("shared cache payload was mutated")
	}
}

func TestRemoteSeriesFallbackRetainsGenreItems(t *testing.T) {
	entries := []any{map[string]any{"Name": "剧情", "Id": 123}}
	item := remoteSeriesPayloadFromEpisode(nil, map[string]any{"SeriesName": "远端剧", "GenreItems": entries}, "remote-series")
	if !reflect.DeepEqual(item["GenreItems"], entries) {
		t.Fatal("remote series fallback dropped official genre items")
	}
}
