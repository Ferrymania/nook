package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
	"go.uber.org/zap"
)

func TestConservativeAutomaticTitleRequiresWholeNameOrAlias(t *testing.T) {
	for _, tc := range []struct {
		query, title string
		aliases      []string
		want         bool
	}{
		{"Yi Yi", "Xiang yi nian nian", nil, false},
		{"Legend of the Dragon", "Legend of the Lost Dragon", nil, false},
		{"Yi Yi", "一一", []string{"Yi Yi"}, true},
		{"Legend of the Dragon", "龙的传人", []string{"Legend of the Dragon"}, true},
		{"The Pitt", "匹兹堡", []string{"The Pitt"}, true},
		{"The One", "The One", nil, true},
		{"Royal Tramp II", "Royal Tramp 2", nil, true},
	} {
		m := &Match{Title: tc.title, Aliases: tc.aliases, TMDbID: 1}
		if got := automaticMetadataTitleTrusted(tc.query, m); got != tc.want {
			t.Errorf("%q -> %q: %v, want %v", tc.query, tc.title, got, tc.want)
		}
	}
}

func TestConservativeAutomaticCandidatesRequireUniqueIdentity(t *testing.T) {
	a := &Match{Title: "Same Title", Year: 2000, TMDbID: 1, MediaType: "movie"}
	b := &Match{Title: "Same Title", Year: 2000, TMDbID: 2, MediaType: "movie"}
	failure := scrapeLookupFailure{}
	if got := uniqueAutomaticMetadataMatch("Same Title", 2000, "movie", []*Match{a, b}, &failure); got != nil || !failure.ambiguous {
		t.Fatalf("ambiguous candidates accepted: %+v, %+v", got, failure)
	}
	if !strings.Contains(failure.message(), "手动匹配") {
		t.Fatalf("missing manual-match instruction: %s", failure.message())
	}
	if got := uniqueAutomaticMetadataMatch("Same Title", 2000, "movie", []*Match{a, a}, &scrapeLookupFailure{}); got == nil {
		t.Fatal("duplicate response for one ID is not ambiguity")
	}
	if got := uniqueAutomaticMetadataMatch("Different", 2000, "movie", []*Match{a}, &scrapeLookupFailure{}); got != nil {
		t.Fatal("single unrelated result accepted")
	}
}

func TestConservativeEnglishMatchDoesNotOverrideChineseSourceConflict(t *testing.T) {
	match := &Match{Title: "另一部电影", OriginalName: "Yi Yi", Year: 2000, TMDbID: 1}
	if automaticSourceNamesTrusted([]string{"一一", "Yi Yi"}, match) {
		t.Fatal("English match concealed Chinese source conflict")
	}
	match.Aliases = []string{"一一"}
	if !automaticSourceNamesTrusted([]string{"一一", "Yi Yi"}, match) {
		t.Fatal("trusted Chinese alias should resolve translation difference")
	}
	if !automaticSourceNamesTrusted([]string{"Yi Yi"}, match) {
		t.Fatal("English-only sources do not require a Chinese translation")
	}
}

func TestConservativeQueriesPreferMovieDirectoryAndKeepEnglishTVName(t *testing.T) {
	lib := &model.Library{Path: "/media/华语电影", Type: "movie"}
	m := &model.Media{Path: "/media/华语电影/龙的传人.Legend.of.the.Dragon.1990/Legend.of.the.Dragon.1990.mkv", Title: "Legend of the Dragon", Year: 1990}
	queries := scrapeQueryCandidates(m, lib)
	if len(queries) == 0 || queries[0] != "龙的传人" {
		t.Fatalf("Chinese directory must be first: %v", queries)
	}
	lib = &model.Library{Path: "/media/外语电视剧", Type: "tv"}
	m = &model.Media{Path: "/media/外语电视剧/匹兹堡医护前线.The.Pitt.S01.2025/匹兹堡医护前线.The.Pitt.S01E02.2025.mkv", Title: "匹兹堡医护前线 The Pitt", SeasonNum: 1, EpisodeNum: 2}
	queries = scrapeQueryCandidates(m, lib)
	if firstIndexFunc(queries, func(q string) bool { return q == "The Pitt" }) < 0 {
		t.Fatalf("English name dropped: %v", queries)
	}
	if firstIndexFunc(queries, func(q string) bool { return q == "外语电视剧" }) >= 0 {
		t.Fatalf("category used as title: %v", queries)
	}
}

func TestConservativeTVUsesSeasonYearAndSeparatesSeasonCache(t *testing.T) {
	seasonCalls := map[string]int{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/search/tv":
			if r.URL.Query().Get("first_air_date_year") != "" {
				t.Error("season year used as series premiere filter")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{map[string]any{"id": 70626, "name": "秘密森林", "original_name": "비밀의 숲", "first_air_date": "2017-06-10"}}})
		case "/tv/70626/season/2":
			seasonCalls["2"]++
			_, _ = w.Write([]byte(`{"air_date":"2020-08-15"}`))
		case "/tv/70626/season/3":
			seasonCalls["3"]++
			_, _ = w.Write([]byte(`{"air_date":"2021-08-15"}`))
		case "/tv/70626/season/4":
			_, _ = w.Write([]byte(`{}`))
		case "/tv/70626/season/5":
			http.Error(w, "unavailable", http.StatusNotFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey, cfg.Secrets.TMDbAPIProxy = "test-key", upstream.URL
	log := zap.NewNop()
	svc := NewScraperService(cfg, log, newOrganizerTestRepo(t), NewTMDbProvider(cfg, log, nil), nil, nil, nil, NewHub(log))
	lib := &model.Library{Type: "tv", Path: "/media/tv"}
	m := &model.Media{Title: "秘密森林", Path: "/media/tv/秘密森林/S02/E02.mkv", SeasonNum: 2, EpisodeNum: 2}
	match := svc.lookup(t.Context(), lib, m, "秘密森林", 2020, false, &scrapeLookupFailure{})
	if match == nil || match.Year != 2017 || match.seasonYear != 2020 || !organizeMetadataMatchTrusted("秘密森林", 2020, match) {
		t.Fatalf("season year not preserved independently: %+v", match)
	}
	if got := svc.lookup(t.Context(), lib, m, "秘密森林", 2020, false, &scrapeLookupFailure{}); got == nil || seasonCalls["2"] != 1 {
		t.Fatal("same-season match not cached")
	}
	for _, season := range []int{3, 4, 5} {
		m.SeasonNum = season
		if got := svc.lookup(t.Context(), lib, m, "秘密森林", 2020, false, &scrapeLookupFailure{}); got != nil {
			t.Fatalf("unverified season %d accepted: %+v", season, got)
		}
	}
	if seasonCalls["3"] != 1 {
		t.Fatal("season 3 reused season 2 cache")
	}
}

func TestConservativeAmbiguityStopsBeforeOtherQueriesOrLocalFallback(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/search/movie" {
			http.NotFound(w, r)
			return
		}
		calls++
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{
			map[string]any{"id": 1, "title": "Same Title", "release_date": "2000-01-01"},
			map[string]any{"id": 2, "title": "Same Title", "release_date": "2000-02-01"},
		}})
	}))
	defer upstream.Close()
	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey, cfg.Secrets.TMDbAPIProxy = "test-key", upstream.URL
	log := zap.NewNop()
	repo := newOrganizerTestRepo(t)
	svc := NewScraperService(cfg, log, repo, NewTMDbProvider(cfg, log, nil), nil, nil, nil, NewHub(log))
	lib := model.Library{Path: t.TempDir(), Type: "movie", Name: "test"}
	if err := repo.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	m := model.Media{LibraryID: lib.ID, Title: "Same Title", Path: lib.Path + "/Same.Title.2000.mkv", Year: 2000, ScrapeStatus: "pending"}
	writeOrgFile(t, filepath.Join(lib.Path, "Same.Title.2000.nfo"), `<movie><title>Same Title</title><year>2000</year></movie>`)
	if err := repo.DB.Create(&m).Error; err != nil {
		t.Fatal(err)
	}
	reason := ""
	if err := svc.EnrichOneWithOptions(t.Context(), &m, ScrapeOptions{resultFailure: &reason}); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Media.FindByID(t.Context(), m.ID)
	if err != nil || got.ScrapeStatus != "no_match" || got.TMDbID != 0 || !strings.Contains(reason, "手动匹配") || calls != 1 {
		t.Fatalf("ambiguity not retained: media=%+v reason=%q calls=%d err=%v", got, reason, calls, err)
	}
}

func TestConservativeOrganizerCacheStillChecksChineseSource(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		results := []any{}
		if r.URL.Path == "/search/movie" && r.URL.Query().Get("query") == "Same Title" {
			results = append(results, map[string]any{"id": 1, "title": "电影甲", "original_title": "Same Title", "release_date": "2000-01-01"})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
	}))
	defer upstream.Close()
	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey, cfg.Secrets.TMDbAPIProxy = "test-key", upstream.URL
	log := zap.NewNop()
	repo := newOrganizerTestRepo(t)
	svc := NewScraperService(cfg, log, repo, NewTMDbProvider(cfg, log, nil), nil, nil, nil, NewHub(log))
	organizer := NewOrganizerService(cfg, log, repo)
	organizer.SetScraper(svc)
	root := t.TempDir()
	cache := map[string]*Match{}
	for _, title := range []string{"电影甲", "电影乙"} {
		path := filepath.Join(root, title+".Same.Title.2000", "Same.Title.2000.mkv")
		got := organizer.lookupOrganizeMetadata(t.Context(), path, root, "movie", "Same Title", 2000, 0, 0, cache)
		if title == "电影甲" && (got == nil || got.TMDbID != 1) {
			t.Fatalf("verified bilingual source rejected: %+v", got)
		}
		if title == "电影乙" && got != nil {
			t.Fatalf("cached English name concealed conflicting Chinese source: %+v", got)
		}
	}
}
