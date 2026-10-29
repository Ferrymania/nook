package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
	"github.com/Ferrymania/nook/internal/testutil"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func newLibraryScrapeEnv(t *testing.T) (*ScraperService, *APIConfigService) {
	t.Helper()
	db, err := testutil.Open(t, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.LibraryRoot{}, &model.Setting{}, &model.APIConfig{}, &model.Media{}, &model.Series{}, &model.ScrapeTask{}); err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	repo := repository.New(db)
	cfg := &config.Config{}
	log := zap.NewNop()
	api := NewAPIConfigService(log, repo, NewCryptoService("test-library-settings", log))
	if err := api.SeedDefaults(t.Context()); err != nil {
		t.Fatal(err)
	}
	s := NewScraperService(cfg, log, repo, NewTMDbProvider(cfg, log, api), NewBangumiProvider(cfg, log), NewTheTVDBProvider(cfg, log), NewFanartProvider(cfg, log), NewHub(log))
	s.bangumi.apiConfig = api
	s.thetvdb.apiConfig = api
	s.fanart.apiConfig = api
	return s, api
}
func setTestProvider(t *testing.T, api *APIConfigService, provider, key, base string, enabled bool) {
	t.Helper()
	if _, err := api.Update(t.Context(), provider, APIConfigPatch{APIKey: &key, BaseURL: &base, Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}
}

func TestLibraryScrapeSettingsExcludeRemovedProviders(t *testing.T) {
	prefs := &model.LibraryScrapeSettings{
		Providers: []string{"tmdb", "douban", "bangumi"}, Language: "ja-JP",
		AutoOnScan: true, DelayMinMS: 1200, DelayMaxMS: 1500,
	}
	lib := &model.Library{ScrapeSettings: prefs}
	got := libraryScrapeSettings(t.Context(), nil, lib)
	if strings.Join(got.Providers, ",") != "tmdb,bangumi" || got.Language != "ja-JP" || !got.AutoOnScan || got.DelayMinMS != 1200 || got.DelayMaxMS != 1500 {
		t.Fatalf("effective preferences = %#v", got)
	}
	if strings.Join(prefs.Providers, ",") != "tmdb,douban,bangumi" {
		t.Fatal("reading preferences must not mutate stored library settings")
	}
	if err := validateLibraryScrapeSettings(got); err != nil {
		t.Fatalf("effective preferences must remain editable: %v", err)
	}
	prefs.Providers = []string{"douban"}
	if got := libraryScrapeSettings(t.Context(), nil, lib); len(got.Providers) != 0 {
		t.Fatalf("removed-only selection must not silently enable other providers: %v", got.Providers)
	}
	if err := validateLibraryScrapeSettings(*prefs); err == nil {
		t.Fatal("removed provider must not be accepted on save")
	}
	ctx := withLibraryScrapeSettings(t.Context(), *prefs)
	if got := scrapeProviders(ctx); len(got) != 0 {
		t.Fatalf("runtime provider chain still includes removed providers: %v", got)
	}
	for _, provider := range defaultScrapeProviders {
		if provider == "douban" {
			t.Fatal("removed provider must not be a default")
		}
	}
}
func TestLibraryScrapeSettingsExcludeRemovedGlobalProvider(t *testing.T) {
	s, _ := newLibraryScrapeEnv(t)
	for _, tc := range []struct{ stored, want string }{
		{"douban", ""},
		{"douban,tmdb", "tmdb"},
		{"tmdb,douban,bangumi", "tmdb,bangumi"},
	} {
		if err := s.repo.Setting.Set(t.Context(), "scrape.providers", tc.stored); err != nil {
			t.Fatal(err)
		}
		got := libraryScrapeSettings(t.Context(), s.repo, nil)
		if strings.Join(got.Providers, ",") != tc.want {
			t.Fatalf("stored=%q effective providers=%v, want %q", tc.stored, got.Providers, tc.want)
		}
		stored, err := s.repo.Setting.Get(t.Context(), "scrape.providers")
		if err != nil || stored != tc.stored {
			t.Fatalf("reading preferences changed stored providers: %q, %v", stored, err)
		}
	}
}

func TestLibraryScrapeSettingsPreserveLegacyAndIsolateLibraries(t *testing.T) {
	s, _ := newLibraryScrapeEnv(t)
	for k, v := range map[string]string{"scrape.auto_on_scan": "true", "scrape.language": "ja-JP", "scrape.providers": "bangumi,tmdb", "scrape.delay_min_ms": "1200", "scrape.delay_max_ms": "1500"} {
		if err := s.repo.Setting.Set(t.Context(), k, v); err != nil {
			t.Fatal(err)
		}
	}
	a := model.Library{Name: "A", Path: t.TempDir(), Type: "anime"}
	b := model.Library{Name: "B", Path: t.TempDir(), Type: "movie"}
	for _, lib := range []*model.Library{&a, &b} {
		if err := s.repo.Library.Create(t.Context(), lib); err != nil {
			t.Fatal(err)
		}
	}
	initial, err := s.GetLibraryScrapeSettings(t.Context(), a.ID)
	if err != nil || !initial.Settings.AutoOnScan || initial.Settings.Language != "ja-JP" || initial.Settings.DelayMinMS != 1200 || initial.Settings.Providers[0] != "bangumi" {
		t.Fatalf("legacy preferences lost: %#v, %v", initial, err)
	}
	settings := model.LibraryScrapeSettings{AutoOnScan: false, Providers: []string{"tmdb"}, Language: "en-US", DelayMinMS: 0, DelayMaxMS: 0}
	if _, err := s.SaveLibraryScrapeSettings(t.Context(), a.ID, settings, true, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveLibraryScrapeSettings(t.Context(), a.ID, settings, false, nil); err != nil {
		t.Fatal(err)
	}
	stored, err := s.repo.Library.FindByID(t.Context(), a.ID)
	if err != nil || stored.ScrapeSettings == nil || stored.ScrapeSettings.AutoOnScan || stored.AutoProbe || stored.ScrapeSettings.DelayMaxMS != 0 {
		t.Fatalf("false/zero not persisted: %#v %v", stored, err)
	}
	other, err := s.GetLibraryScrapeSettings(t.Context(), b.ID)
	if err != nil || !other.Settings.AutoOnScan || other.Settings.Language != "ja-JP" || other.AutoProbe {
		t.Fatalf("other library changed: %#v %v", other, err)
	}
	scanner := &ScannerService{repo: s.repo}
	if scanner.autoScrapeEnabled(t.Context(), stored) || !scanner.autoScrapeEnabled(t.Context(), &b) {
		t.Fatal("scanner did not use per-library preference")
	}
	invalid := settings
	invalid.DelayMaxMS = -1
	if _, err := s.SaveLibraryScrapeSettings(t.Context(), a.ID, invalid, true, nil); err == nil {
		t.Fatal("invalid delay accepted")
	}
	invalid = settings
	invalid.Providers = []string{"fanart"}
	if _, err := s.SaveLibraryScrapeSettings(t.Context(), a.ID, invalid, true, nil); err == nil {
		t.Fatal("artwork-only metadata chain accepted")
	}
	view, _ := s.GetLibraryScrapeSettings(t.Context(), a.ID)
	for _, p := range view.Providers {
		if p.ID == "bangumi" && (!p.Available || p.Status != "可匿名使用") {
			t.Fatalf("anonymous Bangumi hidden: %#v", p)
		}
	}
}

func TestLibrarySegmentPrewarmPreferencePreservesLegacyAndIsolatesLibraries(t *testing.T) {
	s, _ := newLibraryScrapeEnv(t)
	ctx := t.Context()
	prefs := model.LibraryScrapeSettings{Providers: []string{"tmdb"}, Language: "zh-CN"}
	a := model.Library{Name: "Existing preferences", Path: t.TempDir(), ScrapeSettings: &prefs}
	b := model.Library{Name: "Legacy preferences", Path: t.TempDir()}
	for _, lib := range []*model.Library{&a, &b} {
		if err := s.repo.Library.Create(ctx, lib); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.repo.Setting.Set(ctx, legacySegmentPrewarmEnabledKey, "true"); err != nil {
		t.Fatal(err)
	}
	check := func(id string, want bool) *LibraryScrapeSettingsView {
		t.Helper()
		view, err := s.GetLibraryScrapeSettings(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if view.Settings.SegmentPrewarmEnabled == nil || *view.Settings.SegmentPrewarmEnabled != want {
			t.Fatalf("library %s prewarm=%v, want %v", id, view.Settings.SegmentPrewarmEnabled, want)
		}
		return view
	}
	view := check(a.ID, true)
	check(b.ID, true)
	off := false
	view.Settings.SegmentPrewarmEnabled = &off
	if _, err := s.SaveLibraryScrapeSettings(ctx, a.ID, view.Settings, false, nil); err != nil {
		t.Fatal(err)
	}
	check(a.ID, false)
	check(b.ID, true)
	if value, err := s.repo.Setting.Get(ctx, legacySegmentPrewarmEnabledKey); err != nil || value != "true" {
		t.Fatalf("library save rewrote global preference: %q err=%v", value, err)
	}
	on := true
	view.Settings.SegmentPrewarmEnabled = &on
	if _, err := s.SaveLibraryScrapeSettings(ctx, a.ID, view.Settings, false, nil); err != nil {
		t.Fatal(err)
	}
	// Old clients can save other fields without resetting this library's choice.
	view.Settings.SegmentPrewarmEnabled = nil
	if _, err := s.SaveLibraryScrapeSettings(ctx, a.ID, view.Settings, false, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.repo.Setting.Set(ctx, legacySegmentPrewarmEnabledKey, "false"); err != nil {
		t.Fatal(err)
	}
	check(a.ID, true)
	check(b.ID, false)
}

func TestMetadataProvidersUseSavedCredentialsAndGlobalSwitch(t *testing.T) {
	s, api := newLibraryScrapeEnv(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/login":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["apikey"] != "saved-key" {
				t.Errorf("TheTVDB ignored saved key")
			}
			_, _ = w.Write([]byte(`{"data":{"token":"session-token"}}`))
		case strings.HasPrefix(r.URL.Path, "/search/subject/"):
			if r.Header.Get("Authorization") != "Bearer saved-key" {
				t.Errorf("Bangumi ignored saved token")
			}
			_, _ = w.Write([]byte(`{"results":1,"list":[{"id":7,"name":"Clock"}]}`))
		case r.URL.Path == "/search":
			if r.Header.Get("Authorization") != "Bearer session-token" {
				t.Errorf("TheTVDB ignored session token")
			}
			_, _ = w.Write([]byte(`{"data":[]}`))
		case strings.HasPrefix(r.URL.Path, "/movies/"):
			if r.URL.Query().Get("api_key") != "saved-key" {
				t.Errorf("Fanart ignored saved key")
			}
			_, _ = w.Write([]byte(`{}`))
		default:
			if r.URL.Query().Get("api_key") != "saved-key" {
				t.Errorf("TMDb ignored saved key")
			}
			_, _ = w.Write([]byte(`{"results":[]}`))
		}
	}))
	defer server.Close()
	for _, p := range []string{"tmdb", "bangumi", "thetvdb", "fanart"} {
		setTestProvider(t, api, p, "saved-key", server.URL, true)
	}
	if _, err := s.tmdb.SearchMovie(t.Context(), "Clock", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.bangumi.Search(t.Context(), "Clock"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.thetvdb.SearchSeries(t.Context(), "Clock"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.fanart.MovieArtwork(t.Context(), 7); err != nil {
		t.Fatal(err)
	}
	before := calls.Load()
	for _, p := range []string{"tmdb", "bangumi", "thetvdb", "fanart"} {
		enabled := false
		if _, err := api.Update(t.Context(), p, APIConfigPatch{Enabled: &enabled}); err != nil {
			t.Fatal(err)
		}
	}
	// File-provided keys must not bypass a persisted global disable.
	s.cfg.Secrets.TMDbAPIKey = "file-key"
	s.cfg.Secrets.TheTVDBAPIKey = "file-key"
	s.cfg.Secrets.FanartAPIKey = "file-key"
	if s.tmdb.Enabled() || s.bangumi.Enabled() || s.thetvdb.Enabled() || s.fanart.Enabled() {
		t.Fatal("global switch ignored")
	}
	_, _ = s.tmdb.SearchMovie(t.Context(), "Clock", 0)
	_, _ = s.bangumi.GetSubject(t.Context(), 7)
	_, _ = s.thetvdb.GetSeriesMatchByID(t.Context(), "7")
	_, _ = s.fanart.MovieArtwork(t.Context(), 7)
	if calls.Load() != before {
		t.Fatal("disabled provider made a network request")
	}
}

func TestLibraryScrapePriorityLanguageAndCacheIsolation(t *testing.T) {
	s, api := newLibraryScrapeEnv(t)
	var tmdbCalls, bangumiCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/search/subject/") {
			if r.Header.Get("Authorization") != "" {
				t.Error("anonymous search unexpectedly sent credentials")
			}
			bangumiCalls.Add(1)
			_, _ = w.Write([]byte(`{"results":1,"list":[{"id":7,"name":"Clock","name_cn":"时钟","air_date":"2024-01-01"}]}`))
			return
		}
		tmdbCalls.Add(1)
		lang := r.URL.Query().Get("language")
		title := "Clock"
		if lang == "zh-CN" {
			title = "时钟"
		}
		switch r.URL.Path {
		case "/search/tv":
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{{"id": 8, "name": title, "original_name": "Clock", "first_air_date": "2024-01-01"}}})
		case "/movie/8":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 8, "title": title, "original_title": "Clock", "release_date": "2024-01-01", "alternative_titles": map[string]any{"titles": []map[string]string{{"iso_3166_1": "CN", "title": "时钟"}}}})
		case "/tv/8/season/1/episode/1":
			if lang != "ja-JP" {
				t.Errorf("episode language=%s", lang)
			}
			_, _ = w.Write([]byte(`{"name":"エピソード"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	setTestProvider(t, api, "tmdb", "key", server.URL, true)
	setTestProvider(t, api, "bangumi", "", server.URL, true)
	prefs := model.LibraryScrapeSettings{Providers: []string{"tmdb", "bangumi"}, Language: "en-US"}
	lib := model.Library{Name: "TV", Type: "tv", ScrapeSettings: &prefs}
	match := s.lookup(t.Context(), &lib, nil, "Clock", 2024, false, &scrapeLookupFailure{})
	if match == nil || match.Provider != "tmdb" || match.Title != "Clock" || bangumiCalls.Load() != 0 {
		t.Fatalf("TMDb priority/language ignored: %#v", match)
	}
	before := tmdbCalls.Load()
	prefs.Providers = []string{"bangumi", "tmdb"}
	match = s.lookup(t.Context(), &lib, nil, "Clock", 2024, false, &scrapeLookupFailure{})
	if match == nil || match.Provider != "bangumi" || tmdbCalls.Load() != before {
		t.Fatalf("changed priority reused another library's cache: %#v", match)
	}
	prefs.Providers = []string{"tmdb"}
	prefs.Language = "zh-CN"
	match = s.lookup(t.Context(), &lib, nil, "Clock", 2024, false, &scrapeLookupFailure{})
	if match == nil || match.Title != "时钟" {
		t.Fatalf("changed language reused old cache: %#v", match)
	}
	prefs.Language = "en-US"
	ctx := withLibraryScrapeSettings(t.Context(), prefs)
	match, err := s.tmdb.GetMovieMatch(ctx, 8)
	if err != nil || match.Title != "Clock" {
		t.Fatalf("English detail forced to Chinese: %#v %v", match, err)
	}
	prefs.Language = "ja-JP"
	_, err = s.tmdb.GetTVEpisodeDetails(withLibraryScrapeSettings(t.Context(), prefs), 8, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
}

func TestLibraryScrapePacingSharedAndCancelable(t *testing.T) {
	s, _ := newLibraryScrapeEnv(t)
	ctx := withLibraryScrapeSettings(t.Context(), model.LibraryScrapeSettings{DelayMinMS: 60, DelayMaxMS: 60})
	if err := s.waitScrapeTurn(ctx); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := s.waitScrapeTurn(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel not honored: %v", err)
	}
	start := time.Now()
	if err := s.waitScrapeTurn(ctx); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 40*time.Millisecond {
		t.Fatal("consecutive task starts were not paced")
	}
}

func TestLibraryScrapeUnavailableProviderDoesNotReportOldMatchAsSuccess(t *testing.T) {
	s, api := newLibraryScrapeEnv(t)
	setTestProvider(t, api, "tmdb", "", "", false)
	settings := model.LibraryScrapeSettings{Providers: []string{"tmdb"}, Language: "zh-CN"}
	lib := model.Library{Name: "Movies", Path: t.TempDir(), Type: "movie", ScrapeSettings: &settings}
	if err := s.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	media := model.Media{LibraryID: lib.ID, Title: "Clock", Path: lib.Path + "/Clock.2024.mkv", Year: 2024, ScrapeStatus: "matched"}
	if err := s.repo.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	task := model.ScrapeTask{MediaID: media.ID, LibraryID: lib.ID, Status: model.ScrapeTaskRunning}
	if err := s.repo.ScrapeTask.Create(t.Context(), &task); err != nil {
		t.Fatal(err)
	}
	s.processScrapeTask(t.Context(), &task)
	if task.Status != model.ScrapeTaskFailed || !strings.Contains(task.Error, "均不可用") {
		t.Fatalf("unavailable provider reported success: %#v", task)
	}
}

func TestLibraryScrapeIncrementalIngestUsesLibrarySwitchAndDeduplicates(t *testing.T) {
	s, _ := newLibraryScrapeEnv(t)
	scanner := NewScannerService(s.cfg, s.log, s.repo, s.hub, nil, s)
	for _, enabled := range []bool{false, true} {
		settings := model.LibraryScrapeSettings{AutoOnScan: enabled, Providers: []string{"bangumi"}, Language: "zh-CN"}
		lib := model.Library{Name: "Library", Path: t.TempDir(), Type: "movie", Enabled: true, ScrapeSettings: &settings}
		if err := s.repo.Library.Create(t.Context(), &lib); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(lib.Path, "Clock.2024.mkv")
		if err := os.WriteFile(path, []byte("test media"), 0600); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 2; i++ {
			if _, err := scanner.IngestPath(t.Context(), lib.ID, path); err != nil {
				t.Fatal(err)
			}
		}
		var count int64
		if err := s.repo.DB.Model(&model.ScrapeTask{}).Where("library_id = ?", lib.ID).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		want := int64(0)
		if enabled {
			want = 1
		}
		if count != want {
			t.Fatalf("auto_on_scan=%v: queued %d tasks, want %d", enabled, count, want)
		}
	}
}
