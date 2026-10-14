package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
	"go.uber.org/zap"
)

func TestMovieReleaseCleanupAndTitleNumbers(t *testing.T) {
	for _, tc := range []struct {
		file, title string
		year        int
	}{
		{"Project.A.1983.H.265.strm", "Project A", 1983},
		{"X.2022.1080p.strm", "X", 2022},
		{"Project.X.2012.1080p.strm", "Project X", 2012},
		{"Spy.x.Family.S01E01.2022.1080p.mkv", "Spy Family", 2022},
		{"I.Am.Number.Four.2011.strm", "I Am Number Four", 2011},
		{"Yi.Yi.2000.USA.CC.BluRay.strm", "Yi Yi", 2000},
		{"USA.2026.1080p.strm", "USA", 2026},
		{"古墓荒斋.1991.HD1080p.国语对白.strm", "古墓荒斋", 1991},
		{"寒战1994.2026.2160p.strm", "寒战1994", 2026},
		{"寒战1994.strm", "寒战1994", 0},
		{"Rush.Hour.2.2001BluRay.1080p.strm", "Rush Hour 2", 2001},
	} {
		t.Run(tc.file, func(t *testing.T) {
			if title, year := CleanQuery(tc.file); title != tc.title || year != tc.year {
				t.Fatalf("got (%q, %d), want (%q, %d)", title, year, tc.title, tc.year)
			}
		})
	}
	media := &model.Media{Title: "寒战2026", Path: "/media/华语电影/寒战1994.2026.strm", Year: 1994, ScrapeStatus: "no_match"}
	if got := mediaYearHint(media); got != 2026 {
		t.Fatalf("stale parsed year = %d, want 2026", got)
	}
	media.TMDbID, media.ScrapeStatus = 1, "matched"
	if got := mediaYearHint(media); got != 1994 {
		t.Fatalf("confirmed year overwritten: %d", got)
	}
}

func TestMovieSequelNormalizationPreservesIdentity(t *testing.T) {
	for _, tc := range []struct {
		query, title string
		want         bool
	}{
		{"The Fearless Hyena Part II", "Fearless Hyena 2", true},
		{"Royal Tramp II", "Royal Tramp 2", true},
		{"Police Story III Super Cop", "Police Story 3 Super Cop", true},
		{"A Chinese Odyssey Part One", "Chinese Odyssey Part 1", true},
		{"Fearless Hyena Part II", "Fearless Hyena", false},
		{"Royal Tramp II", "Royal Tramp 3", false},
		{"Chinese Odyssey Part 1", "Chinese Odyssey Part 2", false},
		{"The One", "The 1", false},
		{"Project A", "Project B", false},
		{"寒战1994", "寒战1995", false},
	} {
		if got := organizeMetadataMatchTrusted(tc.query, 2026, &Match{Title: tc.title, Year: 2026, TMDbID: 1}); got != tc.want {
			t.Errorf("trust(%q, %q)=%v, want %v", tc.query, tc.title, got, tc.want)
		}
	}
}

func TestMovieFolderFallbackExcludesCollectionsAndYearConflicts(t *testing.T) {
	lib := &model.Library{Path: "/media/movies", Type: "movie"}
	for _, folder := range []string{"华语电影", "成龙电影合集", "新精武门 (1976)"} {
		m := &model.Media{Path: "/media/movies/" + folder + "/New.Fist.of.Fury.1980.strm", Year: 1980}
		for _, query := range scrapeQueryCandidates(m, lib) {
			if containsCJK(query) {
				t.Errorf("unsafe parent fallback %q from %q", query, folder)
			}
		}
	}
}

func TestAutomaticMovieYearFallbackAndProviderAliases(t *testing.T) {
	for _, tc := range []struct {
		name, query, title, alias string
		year, sourceYear, wantID  int
	}{
		{"adjacent year", "Mr Nice Guy", "一个好人", "Mr. Nice Guy", 1997, 1996, 10622},
		{"trusted alias", "Shao Lin men", "少林门", "Shao Lin men", 1976, 1976, 38099},
		{"wrong year", "New Fist of Fury", "新精武门", "New Fist of Fury", 1976, 1980, 0},
		{"wrong sequel", "Royal Tramp II", "鹿鼎记", "Royal Tramp", 1992, 1992, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			unfiltered, details := 0, 0
			id := tc.wantID
			if id == 0 {
				id = 42
			}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.URL.Path == "/search/movie":
					item := map[string]any{"id": id, "title": tc.title, "original_title": tc.title, "release_date": strconv.Itoa(tc.year) + "-01-01"}
					if tc.name == "adjacent year" && r.URL.Query().Get("year") != "" {
						item = map[string]any{"id": 9404, "title": "警察故事4：简单任务", "original_title": "警察故事4", "release_date": "1996-01-01"}
					}
					if r.URL.Query().Get("year") == "" {
						unfiltered++
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{item}})
				case strings.HasPrefix(r.URL.Path, "/movie/"):
					details++
					if r.URL.Path == "/movie/9404" {
						_ = json.NewEncoder(w).Encode(map[string]any{"id": 9404, "title": "警察故事4", "release_date": "1996-01-01"})
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "title": tc.title, "release_date": strconv.Itoa(tc.year) + "-01-01",
						"alternative_titles": map[string]any{"titles": []any{map[string]any{"title": tc.alias}}}})
				default:
					http.NotFound(w, r)
				}
			}))
			defer upstream.Close()
			cfg := &config.Config{}
			cfg.Secrets.TMDbAPIKey, cfg.Secrets.TMDbAPIProxy = "test-key", upstream.URL
			log := zap.NewNop()
			svc := NewScraperService(cfg, log, newOrganizerTestRepo(t), NewTMDbProvider(cfg, log, nil), nil, nil, nil, NewHub(log))
			match := svc.lookupAutomaticTMDb(t.Context(), "movie", tc.query, tc.sourceYear, &scrapeLookupFailure{})
			if tc.wantID == 0 {
				if match != nil {
					t.Fatalf("unsafe match accepted: %+v", match)
				}
			} else if match == nil || match.TMDbID != tc.wantID || match.Year != tc.year {
				t.Fatalf("wanted verified movie, got %+v", match)
			}
			if tc.name == "adjacent year" && unfiltered == 0 {
				t.Fatal("nonempty rejected year results prevented fallback")
			}
			if tc.name == "wrong year" && details != 0 {
				t.Fatal("hydrated details for candidates outside year tolerance")
			}
		})
	}
}
