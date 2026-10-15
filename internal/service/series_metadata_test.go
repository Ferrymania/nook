package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
)

func TestSeriesMetadataSeparatesEpisodeArtworkAndPreservesIdentity(t *testing.T) {
	scraper, repo, closeServer := newTestScraper(t)
	defer closeServer()
	lib := model.Library{Name: "番剧", Type: "tv", Path: t.TempDir(), Enabled: true}
	if err := repo.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	m := model.Media{LibraryID: lib.ID, Title: "间谍过家家", Path: filepath.Join(lib.Path, "间谍过家家 - S02E01.strm"), SeasonNum: 2, EpisodeNum: 1, TMDbID: 12345, ScrapeStatus: "matched"}
	if err := repo.DB.Create(&m).Error; err != nil {
		t.Fatal(err)
	}
	emby := NewEmbyService(scraper.cfg, zap.NewNop(), repo)
	publicID := emby.seriesIDForMedia(t.Context(), &m)
	scraper.onSeriesMetadataChanged = emby.invalidateSeriesMetadata
	if err := scraper.EnrichOne(t.Context(), &m); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Media.FindByID(t.Context(), m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got.EpisodeStillURL, "/w500/still.jpg") || !strings.HasSuffix(got.BackdropURL, "/w1280/backdrop.jpg") || got.Overview != "单集剧情" {
		t.Fatalf("series and episode fields mixed: %+v", got)
	}
	groups := emby.seriesGroupsFromMedia(t.Context(), []model.Media{*got})
	if len(groups) != 1 || groups[0].ID != publicID || groups[0].Overview != "测试简介" || groups[0].BackdropURL != got.BackdropURL {
		t.Fatalf("series metadata/identity not preserved: %+v", groups)
	}
	if got.SeriesID != "" {
		t.Fatalf("legacy public identity was relinked: %q", got.SeriesID)
	}
	if image, _ := emby.ImageURL(t.Context(), got.ID, "Primary"); image != got.EpisodeStillURL {
		t.Fatalf("episode Primary=%q", image)
	}
	beforeTag := emby.seriesPayload(groups[0])["BackdropImageTags"].([]string)[0]
	updated := &Match{Title: got.Title, TMDbID: got.TMDbID, Overview: "Updated series plot", Year: 2022, Rating: 8.6}
	if err := scraper.saveScrapedSeriesMetadata(t.Context(), got, updated, got.PosterURL, "https://image.example/new-series-backdrop.jpg"); err != nil {
		t.Fatal(err)
	}
	after, err := emby.Item(t.Context(), publicID, "")
	if err != nil {
		t.Fatal(err)
	}
	if after["Id"] != publicID || after["BackdropImageTags"].([]string)[0] == beforeTag {
		t.Fatal("artwork changes must refresh the image tag without changing the item ID")
	}
	if source, _ := emby.ImageURL(t.Context(), publicID, "Backdrop"); source != "https://image.example/new-series-backdrop.jpg" {
		t.Fatalf("stale series artwork cache: %q", source)
	}
	off := false
	if err := scraper.EnrichOneWithOptions(t.Context(), got, ScrapeOptions{EpisodeArtwork: &off}); err != nil {
		t.Fatal(err)
	}
	retained, _ := repo.Media.FindByID(t.Context(), m.ID)
	if retained.EpisodeStillURL != got.EpisodeStillURL {
		t.Fatal("disabling episode images removed an existing still")
	}
	var count int64
	if err := repo.DB.Model(&model.Series{}).Where("library_id = ?", lib.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("duplicate series metadata records: %d", count)
	}
}

func TestSeriesMetadataBackfillFetchesEachShowOnceWithoutTouchingEpisodes(t *testing.T) {
	requests := []string{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path)
		if r.URL.Path != "/tv/12345" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 12345, "name": "Show", "overview": "Series plot", "backdrop_path": "/series.jpg", "poster_path": "/poster.jpg", "first_air_date": "2020-01-01"})
	}))
	defer upstream.Close()
	db := newServiceTestDB(t, &model.Library{}, &model.Series{}, &model.Media{})
	repo := repository.New(db)
	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey, cfg.Secrets.TMDbAPIProxy = "test", upstream.URL
	log := zap.NewNop()
	scraper := NewScraperService(cfg, log, repo, NewTMDbProvider(cfg, log, nil), nil, nil, nil, NewHub(log))
	lib := model.Library{Name: "TV", Type: "tv", Path: "/media/TV", Enabled: true}
	if err := db.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{LibraryID: lib.ID, Title: "Show", Path: "/media/TV/Show/S01E01.strm", SeasonNum: 1, EpisodeNum: 1, TMDbID: 12345, ScrapeStatus: "matched", EpisodeStillURL: "https://image.example/still-1.jpg", Overview: "Episode 1"},
		{LibraryID: lib.ID, Title: "Show", Path: "/media/TV/Show/S01E02.strm", SeasonNum: 1, EpisodeNum: 2, TMDbID: 12345, ScrapeStatus: "matched", EpisodeStillURL: "https://image.example/still-2.jpg", Overview: "Episode 2"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	scraper.backfillSeriesMetadata(t.Context())
	scraper.backfillSeriesMetadata(t.Context())
	if !reflect.DeepEqual(requests, []string{"/tv/12345"}) {
		t.Fatalf("unexpected backfill requests: %v", requests)
	}
	var after []model.Media
	if err := db.Order("episode_num").Find(&after).Error; err != nil {
		t.Fatal(err)
	}
	for i := range rows {
		if rows[i].ID != after[i].ID || rows[i].Path != after[i].Path || rows[i].SeriesID != after[i].SeriesID || rows[i].EpisodeStillURL != after[i].EpisodeStillURL || rows[i].Overview != after[i].Overview || rows[i].DurationSec != after[i].DurationSec {
			t.Fatal("backfill modified episode records")
		}
	}
	var parent model.Series
	if err := db.First(&parent).Error; err != nil {
		t.Fatal(err)
	}
	if parent.Overview != "Series plot" || !strings.Contains(parent.BackdropURL, "/w1280/series.jpg") {
		t.Fatalf("bad parent metadata: %+v", parent)
	}
}

func TestEmbySenPlayerMixedTypesIncludeMoviesAndSeries(t *testing.T) {
	e := newTestEmbyService(t)
	movies := model.Library{Name: "Movies", Type: "movie", Path: "/media/movies", Enabled: true}
	tv := model.Library{Name: "TV", Type: "tv", Path: "/media/tv", Enabled: true}
	if err := e.repo.DB.Create(&movies).Error; err != nil {
		t.Fatal(err)
	}
	if err := e.repo.DB.Create(&tv).Error; err != nil {
		t.Fatal(err)
	}
	u := model.User{Username: "viewer", PasswordHash: "unused", HideAdult: true}
	if err := e.repo.DB.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{LibraryID: movies.ID, Title: "A Movie", Path: "/media/movies/a.strm"},
		{LibraryID: movies.ID, Title: "Hidden", Path: "/media/movies/hidden.strm", NSFW: true},
		{LibraryID: tv.ID, Title: "B Show", Path: "/media/tv/Show/S01E01.strm", SeasonNum: 1, EpisodeNum: 1},
		{LibraryID: tv.ID, Title: "B Show", Path: "/media/tv/Show/S01E02.strm", SeasonNum: 1, EpisodeNum: 2},
	}
	if err := e.repo.DB.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	p := ItemsParams{UserID: u.ID, Recursive: true, IncludeItemTypes: []string{"Series", "Movie", "Video", "MusicVideo", "MusicAlbum"}, SortBy: "SortName", SortOrder: "Ascending", Limit: 30}
	for _, tc := range []struct {
		parent, wantType string
		want             int
	}{{movies.ID, "Movie", 1}, {tv.ID, "Series", 1}, {"", "", 2}} {
		p.ParentID = tc.parent
		out, err := e.Items(t.Context(), p)
		if err != nil {
			t.Fatal(err)
		}
		items := out["Items"].([]map[string]any)
		if len(items) != tc.want {
			t.Fatalf("parent %q: got %d items: %+v", tc.parent, len(items), out)
		}
		if tc.wantType != "" && items[0]["Type"] != tc.wantType {
			t.Fatalf("wrong item type: %+v", items[0])
		}
	}
	p.ParentID, p.Limit, p.StartIndex = "", 1, 1
	out, err := e.Items(t.Context(), p)
	if err != nil {
		t.Fatal(err)
	}
	if out["TotalRecordCount"] != 2 || len(out["Items"].([]map[string]any)) != 1 || out["Items"].([]map[string]any)[0]["Type"] != "Series" {
		t.Fatalf("mixed pagination/sorting failed: %+v", out)
	}
}
