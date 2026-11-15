package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
	"go.uber.org/zap"
)

func TestContentRatingNFORoundTripAndEpisodeOverride(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "Show.S01E01.strm")
	if err := os.WriteFile(path, []byte("https://example.test/video"), 0600); err != nil {
		t.Fatal(err)
	}
	m := model.Media{Path: path, Title: "Show", SeasonNum: 1, EpisodeNum: 1, ContentRating: model.NewContentRating("TV-14", "US", "manual")}
	if _, err := WriteMediaNFO(&m); err != nil {
		t.Fatal(err)
	}
	local, err := ReadLocalMetadata(path, root, true)
	if err != nil || local == nil || local.OfficialRating != "TV-14" || local.ContentRatingLevel != 35 {
		t.Fatalf("round trip: %#v %v", local, err)
	}
	dst := &LocalMetadata{ContentRating: model.NewContentRating("TV-PG", "US", "nfo")}
	mergeEpisodeMetadata(dst, local, &nfoDocument{})
	if dst.OfficialRating != "TV-14" {
		t.Fatal("episode certificate did not override show")
	}
}

func TestContentRatingFiltersCountsCachesAndDirectPlayback(t *testing.T) {
	e := newTestEmbyService(t)
	ctx := t.Context()
	repo := e.repo
	lib := model.Library{Name: "Movies", Path: "/media", Type: "movie"}
	if err := repo.Library.Create(ctx, &lib); err != nil {
		t.Fatal(err)
	}
	u := model.User{Username: "viewer", PasswordHash: "x", IsActive: true, PlaybackSettings: `{"content_rating_limit":"PG-13"}`}
	if err := repo.User.Create(ctx, &u); err != nil {
		t.Fatal(err)
	}
	for i, rating := range []string{"G", "PG-13", "R", "TV-14", ""} {
		m := model.Media{Base: model.Base{ID: fmt.Sprintf("rating-%d", i)}, LibraryID: lib.ID, Title: fmt.Sprintf("Movie %d", i), Path: fmt.Sprintf("/media/%d.mkv", i)}
		if rating != "" {
			m.ContentRating = model.NewContentRating(rating, "US", "manual")
		}
		if err := repo.DB.Create(&m).Error; err != nil {
			t.Fatal(err)
		}
	}
	media := NewMediaService(&config.Config{}, zap.NewNop(), repo).SetRuntimeCache(NewRuntimeCacheService(&config.Config{}))
	for _, limit := range []string{"PG-13", "R", "PG-13"} {
		if err := repo.DB.Model(&u).Update("playback_settings", fmt.Sprintf(`{"content_rating_limit":%q,"hide_unrated":true}`, limit)).Error; err != nil {
			t.Fatal(err)
		}
		e.InvalidateUserVisibility(ctx, u.ID)
		v := UserDefaultMediaVisibility(ctx, repo, u.ID)
		want := int64(2)
		if limit == "R" {
			want = 4
		}
		rows, total, err := media.ListMediaVisible(ctx, lib.ID, 1, 1, v)
		if err != nil || total != want || len(rows) != 1 {
			t.Fatalf("list %s count=%d rows=%d err=%v", limit, total, len(rows), err)
		}
		rows, total, err = media.SearchMediaVisiblePage(ctx, "Movie", 1, 1, v)
		if err != nil || total != want || len(rows) != 1 {
			t.Fatalf("search count=%d err=%v", total, err)
		}
		counts, err := media.CountLibrariesCached(ctx, []string{lib.ID}, mediaFilterFromVisibility(v))
		if err != nil || counts[lib.ID] != want {
			t.Fatalf("counts=%v err=%v", counts, err)
		}
		if limit == "PG-13" {
			item, err := e.Item(ctx, "rating-2", u.ID)
			if err != nil || item != nil {
				t.Fatalf("blocked detail=%v err=%v", item, err)
			}
			playback, err := e.PlaybackInfo(ctx, "rating-2", u.ID)
			if err != nil || playback != nil {
				t.Fatalf("blocked playback=%v err=%v", playback, err)
			}
		}
	}
}

func TestContentRatingCollectedByNormalScrapeQueue(t *testing.T) {
	s, api := newLibraryScrapeEnv(t)
	ctx := t.Context()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/tv/123" && !strings.Contains(r.URL.Query().Get("append_to_response"), "content_ratings") {
			t.Error("normal scrape did not request content ratings")
		}
		fmt.Fprint(w, `{"id":123,"name":"Original","overview":"Keep this","content_ratings":{"results":[{"iso_3166_1":"US","rating":"TV-14"}]}}`)
	}))
	defer server.Close()
	setTestProvider(t, api, "tmdb", "key", server.URL, true)
	prefs := libraryScrapeSettings(nil)
	prefs.DelayMinMS = 0
	prefs.DelayMaxMS = 0
	lib := model.Library{Name: "Show", Path: t.TempDir(), Type: "tv", ScrapeSettings: &prefs}
	if err := s.repo.Library.Create(ctx, &lib); err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for i := 1; i <= 3; i++ {
		m := model.Media{LibraryID: lib.ID, Title: "Original", Overview: "Keep this", Path: fmt.Sprintf("cloud://test/Original {tmdb-123}/Season 1/Original.S01E%02d.strm", i), SeasonNum: 1, EpisodeNum: i, TMDbID: 123, ScrapeStatus: "matched", DurationSec: 1234, SizeBytes: 5678}
		if i == 3 {
			m.ContentRating = model.NewContentRating("TV-MA", "US", "manual")
		}
		if err := s.repo.DB.Create(&m).Error; err != nil {
			t.Fatal(err)
		}
		ids = append(ids, m.ID)
	}
	n, err := s.EnqueueLibrary(ctx, lib.ID, ScrapeOptions{IncludeMatched: true})
	if err != nil || n != 3 {
		t.Fatalf("normal scrape enqueued=%d err=%v", n, err)
	}
	var tasks []model.ScrapeTask
	if err = s.repo.DB.Find(&tasks).Error; err != nil {
		t.Fatal(err)
	}
	for i := range tasks {
		s.processScrapeTask(ctx, &tasks[i])
		if tasks[i].Status != model.ScrapeTaskDone {
			t.Fatalf("ordinary scrape failed: %+v", tasks[i])
		}
	}
	for i, id := range ids {
		m, err := s.repo.Media.FindByID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		want := "TV-14"
		if i == 2 {
			want = "TV-MA"
		}
		if m.OfficialRating != want || m.Title != "Original" || m.Overview != "Keep this" || m.DurationSec != 1234 || m.SizeBytes != 5678 || m.ScrapeStatus != "matched" {
			t.Fatalf("normal scrape did not preserve fields or collect certificate: %+v", m)
		}
	}
}

func TestContentRatingTMDbAppendsAndPreservesCountry(t *testing.T) {
	s, api := newLibraryScrapeEnv(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !strings.Contains(r.URL.Query().Get("append_to_response"), "release_dates") {
			t.Error("certificate not requested")
		}
		fmt.Fprint(w, `{"id":5,"title":"Movie","release_dates":{"results":[{"iso_3166_1":"GB","release_dates":[{"certification":"15"}]},{"iso_3166_1":"US","release_dates":[{"certification":"PG-13"}]}]}}`)
	}))
	defer server.Close()
	setTestProvider(t, api, "tmdb", "key", server.URL, true)
	m, err := s.tmdb.GetMovieMatch(t.Context(), 5)
	if err != nil || m == nil || m.OfficialRating != "PG-13" || m.RatingCountry != "US" || m.ContentRatingLevel != 30 {
		t.Fatalf("match=%+v err=%v", m, err)
	}
	raw, _ := json.Marshal(m)
	if !strings.Contains(string(raw), "official_rating") {
		t.Fatal("certificate missing from JSON")
	}
}

func TestContentRatingUnknownVersusFailureAndManualPrecedence(t *testing.T) {
	s, api := newLibraryScrapeEnv(t)
	ctx := t.Context()
	fail := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if fail {
			fmt.Fprint(w, `{"id":123,"title":"Movie","release_dates":{"success":false,"status_code":11}}`)
		} else {
			fmt.Fprint(w, `{"id":123,"title":"Movie","release_dates":{"results":[]}}`)
		}
	}))
	defer server.Close()
	setTestProvider(t, api, "tmdb", "key", server.URL, true)
	prefs := libraryScrapeSettings(nil)
	prefs.DelayMinMS = 0
	prefs.DelayMaxMS = 0
	lib := model.Library{Name: "Movie", Path: t.TempDir(), Type: "movie", ScrapeSettings: &prefs}
	if err := s.repo.Library.Create(ctx, &lib); err != nil {
		t.Fatal(err)
	}
	m := model.Media{LibraryID: lib.ID, Title: "Movie", Path: "cloud://test/Movie/movie.strm", TMDbID: 123, ContentRating: model.NewContentRating("R", "US", "tmdb")}
	if err := s.repo.DB.Create(&m).Error; err != nil {
		t.Fatal(err)
	}
	opts := ScrapeOptions{DeferEpisodeDetails: true}
	fail = true
	if err := s.EnrichOneWithOptions(ctx, &m, opts); err != nil {
		t.Fatal(err)
	}
	got, _ := s.repo.Media.FindByID(ctx, m.ID)
	if got.OfficialRating != "R" {
		t.Fatal("failure cleared old certificate")
	}
	fail = false
	if err := s.EnrichOneWithOptions(ctx, &m, opts); err != nil {
		t.Fatal(err)
	}
	got, _ = s.repo.Media.FindByID(ctx, m.ID)
	if got.OfficialRating != "" || got.ContentRatingCheckedAt == nil {
		t.Fatal("successful unknown must be marked as queried")
	}
	editor := NewMediaService(&config.Config{}, zap.NewNop(), s.repo)
	manual := "PG"
	country := "US"
	got, err := editor.UpdateMetadata(ctx, m.ID, MediaMetadataUpdate{OfficialRating: &manual, RatingCountry: &country})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.EnrichOneWithOptions(ctx, got, opts); err != nil {
		t.Fatal(err)
	}
	got, _ = s.repo.Media.FindByID(ctx, m.ID)
	if got.OfficialRating != "PG" || got.ContentRatingSource != "manual" || got.ContentRatingLevel != 20 {
		t.Fatalf("manual certificate overwritten: %+v", got)
	}
	// A partial API edit that changes the country and identity together is still
	// an explicit manual certificate edit, even when the raw label is unchanged.
	if err := s.repo.DB.Model(&model.Media{}).Where("id = ?", m.ID).Updates(contentRatingUpdates(model.NewContentRating("PG", "US", "tmdb"))).Error; err != nil {
		t.Fatal(err)
	}
	changedID, changedCountry := 456, "GB"
	got, err = editor.UpdateMetadata(ctx, m.ID, MediaMetadataUpdate{TMDbID: &changedID, RatingCountry: &changedCountry})
	if err != nil || got == nil || got.OfficialRating != "PG" || got.RatingCountry != "GB" || got.ContentRatingSource != "manual" {
		t.Fatalf("explicit country edit lost during rematch: %+v, %v", got, err)
	}
}

func TestContentRatingEmbyCacheSeparatesInflightPreferenceChanges(t *testing.T) {
	e := newTestEmbyService(t).SetRuntimeCache(NewRuntimeCacheService(nil))
	lib := model.Library{Name: "Cache", Path: "/media/cache", Type: "movie"}
	if err := e.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	for i, raw := range []string{"G", "R"} {
		media := model.Media{LibraryID: lib.ID, Title: fmt.Sprintf("Cached movie %d", i), Path: fmt.Sprintf("/media/cache/%d.mkv", i), ContentRating: model.NewContentRating(raw, "US", "tmdb")}
		if err := e.repo.DB.Create(&media).Error; err != nil {
			t.Fatal(err)
		}
	}
	oldCtx := WithContentRatingPolicy(t.Context(), model.ContentRatingPolicy{})
	newCtx := WithContentRatingPolicy(t.Context(), model.ContentRatingPolicy{MaxContentRatingLevel: 30, HideUnrated: true})
	params := ItemsParams{UserID: "viewer", ParentID: lib.ID, Limit: 20, IncludeItemTypes: []string{"Movie"}}
	// The same authenticated user can have an old request in flight while a new
	// request already uses the saved stricter preference. Neither cache can win
	// over the other request's policy, in either order.
	for _, test := range []struct {
		restricted bool
		count      int
	}{{false, 2}, {true, 1}, {false, 2}, {true, 1}} {
		ctx := oldCtx
		if test.restricted {
			ctx = newCtx
		}
		body, err := e.Items(ctx, params)
		if err != nil || remoteItemInt(body, "TotalRecordCount") != test.count {
			t.Fatalf("restricted=%v response=%v error=%v", test.restricted, body, err)
		}
	}
}

func TestContentRatingVirtualSeriesCacheDoesNotLoseOtherUsersEpisodes(t *testing.T) {
	e := newTestEmbyService(t)
	lib := model.Library{Name: "Series", Path: "/media/series", Type: "tv"}
	if err := e.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	for i, raw := range []string{"TV-PG", "TV-MA"} {
		m := model.Media{LibraryID: lib.ID, Title: "Show", Path: fmt.Sprintf("/media/series/Show/S01E%02d.strm", i+1), SeasonNum: 1, EpisodeNum: i + 1, ContentRating: model.NewContentRating(raw, "US", "tmdb")}
		if err := e.repo.DB.Create(&m).Error; err != nil {
			t.Fatal(err)
		}
	}
	id := stableEmbyID(embyVirtualSeriesPrefix, lib.ID, "Show")
	for _, restricted := range []bool{false, true, false} {
		policy := model.ContentRatingPolicy{}
		want := 2
		if restricted {
			policy.MaxContentRatingLevel = 30
			want = 1
		}
		body, err := e.Item(WithContentRatingPolicy(t.Context(), policy), id, "viewer")
		if err != nil || body == nil || remoteItemInt(body, "RecursiveItemCount") != want {
			t.Fatalf("restricted=%v body=%v err=%v", restricted, body, err)
		}
	}
}
