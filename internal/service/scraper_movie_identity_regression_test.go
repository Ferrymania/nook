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

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
)

func TestMovieScrapeFailedReleaseNames(t *testing.T) {
	batman := "[蝙蝠侠：黑暗骑士系列1~3部] The Dark Knight Collection 2005~2012 V4 2160p Remux"
	potter := "哈利波特8部合集/英国台粤DTS XMA7.1/导评音轨&视觉描述/英简繁&双语特效SUP字幕/章节/杜比p8.Harry.Potter.8-Film.Collection.2001-2011.UHD.BluRay.2160p"
	cases := []struct {
		folder, file, query, title, english, original string
		year, id                                      int
	}{
		{batman, "蝙蝠侠：黑暗骑士.The.Dark.Knight.2008.V2.2160p.Remux.strm", "The Dark Knight", "黑暗骑士", "The Dark Knight", "The Dark Knight", 2008, 155},
		{batman, "蝙蝠侠：黑暗骑士崛起.The.Dark.Knight.Rises.2012.v2.2160p.Remux.strm", "The Dark Knight Rises", "黑暗骑士崛起", "The Dark Knight Rises", "The Dark Knight Rises", 2012, 49026},
		{potter, "Harry.Potter.and.the.Goblet.of.Fire.2005.UHD.BluRay.strm", "Harry Potter and the Goblet of Fire", "哈利·波特与火焰杯", "Harry Potter and the Goblet of Fire", "Harry Potter and the Goblet of Fire", 2005, 674},
		{potter, "Harry.Potter.and.the.Prisoner.of.Azkaban.2004.UHD.BluRay.strm", "Harry Potter and the Prisoner of Azkaban", "哈利·波特与阿兹卡班的囚徒", "Harry Potter and the Prisoner of Azkaban", "Harry Potter and the Prisoner of Azkaban", 2004, 673},
		{potter, "Harry.Potter.and.the.Order.of.the.Phoenix.2007.UHD.BluRay.strm", "Harry Potter and the Order of the Phoenix", "哈利·波特与凤凰社", "Harry Potter and the Order of the Phoenix", "Harry Potter and the Order of the Phoenix", 2007, 675},
		{potter, "Harry.Potter.and.the.Half-Blood.Prince.2009.UHD.BluRay.strm", "Harry Potter and the Half Blood Prince", "哈利·波特与混血王子", "Harry Potter and the Half-Blood Prince", "Harry Potter and the Half-Blood Prince", 2009, 767},
		{potter, "Harry.Potter.and.the.Deathly.Hallows-Part.2.2011.UHD.BluRay.strm", "Harry Potter and the Deathly Hallows Part 2", "哈利·波特与死亡圣器（下）", "Harry Potter and the Deathly Hallows: Part 2", "Harry Potter and the Deathly Hallows: Part 2", 2011, 12445},
		{"5 浪客剑心：回忆篇 最终篇", "浪客剑心 最终章 追忆篇.strm", "浪客剑心 最终章 追忆篇", "浪客剑心最终章：追忆篇", "Rurouni Kenshin: The Beginning", "るろうに剣心 最終章 The Beginning", 2021, 672322},
		{"还有明天 蓝光原盘REMUX [中英双字 简繁中字]", "Theres.Still.Tomorrow.2023.ITA.1080p.BluRay.REMUX.strm", "Theres Still Tomorrow", "还有明天", "There's Still Tomorrow", "C'è ancora domani", 2023, 1026227},
		{"A计划 (1983)", "Project.A.1983.H.265.strm", "Project A", "A计划", "Project A", "A計劃", 1983, 21519},
		{"古墓荒斋 (1991)", "古墓荒斋.1991.HD1080p.国语对白.strm", "古墓荒斋", "古墓荒斋", "An Old Manor", "古墓荒斋", 1991, 663923},
		{"一一 (2000)", "Yi.Yi.2000.USA.CC.BluRay.strm", "一一", "一一", "Yi Yi", "一一", 2000, 25538},
		{"蛇鹤八步 (1978)", "Snake.and.Crane.Arts.of.Shao.lin.1978.strm", "蛇鹤八步", "蛇鹤八步", "Snake and Crane Arts of Shaolin", "蛇鶴八步", 1978, 35170},
		{"少林门 (1976)", "Shao.Lin.men.1976.strm", "少林门", "少林门", "Hand of Death", "少林門", 1976, 38099},
		{"我老婆不是人 (1991)", "我的老婆不是人.strm", "我老婆不是人", "我老婆不是人", "Pretty Ghost", "我老婆不是人", 1991, 320529},
		{"寒战1994 (2026)", "寒战1994.2026.2160p.strm", "寒战1994", "寒战1994", "Cold War 1994", "寒戰1994", 2026, 1499071},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				item := map[string]any{"id": tc.id, "title": tc.title, "original_title": tc.original, "release_date": fmt.Sprintf("%d-01-01", tc.year)}
				if r.URL.Path == "/search/movie" {
					if r.URL.Query().Get("query") != tc.query || (r.URL.Query().Get("year") != "" && r.URL.Query().Get("year") != fmt.Sprint(tc.year)) {
						_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
						return
					}
					if r.URL.Query().Get("language") == "en-US" {
						item["title"] = tc.english
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{item}})
					return
				}
				_ = json.NewEncoder(w).Encode(item)
			}))
			defer upstream.Close()
			repos := newOrganizerTestRepo(t)
			cfg := &config.Config{}
			cfg.Secrets.TMDbAPIKey, cfg.Secrets.TMDbAPIProxy = "test-key", upstream.URL
			log := zap.NewNop()
			scraper := NewScraperService(cfg, log, repos, NewTMDbProvider(cfg, log, nil), nil, nil, nil, NewHub(log))
			root := t.TempDir()
			path := filepath.Join(root, tc.folder, tc.file)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("https://example.invalid/movie"), 0o644); err != nil {
				t.Fatal(err)
			}
			lib := model.Library{Name: "Movies", Type: "movie", Path: root, Enabled: true}
			if err := repos.DB.Create(&lib).Error; err != nil {
				t.Fatal(err)
			}
			year := tc.year
			if tc.id == 672322 {
				year = 0
			}
			media := model.Media{LibraryID: lib.ID, Title: tc.query, Year: year, Path: path, ScrapeStatus: "no_match"}
			if err := repos.DB.Create(&media).Error; err != nil {
				t.Fatal(err)
			}
			if err := scraper.EnrichOne(t.Context(), &media); err != nil {
				t.Fatal(err)
			}
			got, err := repos.Media.FindByID(t.Context(), media.ID)
			if err != nil || got == nil || got.ScrapeStatus != "matched" || got.TMDbID != tc.id || got.Year != tc.year || got.Title != tc.title || got.SeasonNum != 0 || got.EpisodeNum != 0 {
				t.Fatalf("movie identity=%+v error=%v; want %q (%d), tmdb=%d", got, err, tc.title, tc.year, tc.id)
			}
		})
	}
}

func TestMovieScrapeKeepsAmbiguousTitlesUnmatched(t *testing.T) {
	for _, tc := range []struct{ query, title string }{
		{"浪客剑心 最终章 追忆篇", "浪客剑心最终章：人诛篇"},
		{"Harry Potter and the Deathly Hallows Part 2", "Harry Potter and the Deathly Hallows Part 1"},
		{"Rocky 2", "Rocky"},
		{"无间道", "无间道2"},
		{"电影 AB C", "电影 A BC"},
	} {
		if organizeMetadataMatchTrusted(tc.query, 0, &Match{Title: tc.title, TMDbID: 1}) {
			t.Errorf("accepted distinct movie %q for %q", tc.title, tc.query)
		}
	}
	if organizeMetadataMatchTrusted("The Dark Knight", 2005, &Match{Title: "The Dark Knight", Year: 2008, TMDbID: 155}) {
		t.Fatal("year mismatch guard was weakened")
	}
	for input, want := range map[string]string{
		"ITA.Movie.2023.1080p.mkv":           "ITA Movie",
		"V2.Movie.2023.1080p.mkv":            "V2 Movie",
		"Mad.Max.1979.1080p.mkv":             "Mad Max",
		"Dune.Part.2.2024.2160p.mkv":         "Dune Part 2",
		"Theres.Still.Tomorrow.2023.ITA.mkv": "Theres Still Tomorrow",
		"蝙蝠侠.The.Dark.Knight.2008.V2.mkv":    "蝙蝠侠 The Dark Knight",
	} {
		if got, _ := CleanQuery(input); got != want {
			t.Errorf("CleanQuery(%q)=%q, want %q", input, got, want)
		}
	}
}

func TestMovieScrapeFailureReasonsReachQueue(t *testing.T) {
	for _, tc := range []struct {
		name, title, want string
		year, status      int
	}{
		{"empty", "", "搜索结果为空", 0, 200},
		{"title", "Different Film", "标题或续集编号不符", 2023, 200},
		{"year", "Example Film", "上映年份不符", 2000, 200},
		{"request", "", "接口请求失败", 0, 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				results := []any{}
				if tc.title != "" {
					results = append(results, map[string]any{"id": 99, "title": tc.title, "release_date": fmt.Sprintf("%d-01-01", tc.year)})
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
			}))
			defer upstream.Close()
			repos := newOrganizerTestRepo(t)
			if err := repos.DB.AutoMigrate(&model.ScrapeTask{}); err != nil {
				t.Fatal(err)
			}
			cfg := &config.Config{}
			cfg.Secrets.TMDbAPIKey, cfg.Secrets.TMDbAPIProxy = "secret-test-key", upstream.URL
			log := zap.NewNop()
			scraper := NewScraperService(cfg, log, repos, NewTMDbProvider(cfg, log, nil), nil, nil, nil, NewHub(log))
			lib := model.Library{Name: "Movies", Type: "movie", Path: "cloud://test/movies", Enabled: true}
			if err := repos.DB.Create(&lib).Error; err != nil {
				t.Fatal(err)
			}
			media := model.Media{LibraryID: lib.ID, Title: "Example Film", Path: "cloud://test/movies/Example.Film.2023.strm", Year: 2023, ScrapeStatus: "pending"}
			if err := repos.DB.Create(&media).Error; err != nil {
				t.Fatal(err)
			}
			task := model.ScrapeTask{MediaID: media.ID, MediaTitle: media.Title, MediaPath: media.Path, Status: model.ScrapeTaskRunning}
			if err := repos.DB.Create(&task).Error; err != nil {
				t.Fatal(err)
			}
			scraper.processScrapeTask(t.Context(), &task)
			var saved model.ScrapeTask
			if err := repos.DB.First(&saved, "id = ?", task.ID).Error; err != nil {
				t.Fatal(err)
			}
			if saved.Status != model.ScrapeTaskFailed || !strings.Contains(saved.Error, tc.want) || strings.Contains(saved.Error, cfg.Secrets.TMDbAPIKey) || strings.Contains(saved.Error, upstream.URL) {
				t.Fatalf("unexpected or unsafe queue failure: %q, status=%s", saved.Error, saved.Status)
			}
			// A cached miss must retain the same explanation without another lookup.
			failure := scrapeLookupFailure{}
			if _, ok := scraper.lookupCache.get(scrapeLookupCacheKey("movie", "Example Film", 2023, false), &failure); !ok || !strings.Contains(failure.message(), tc.want) {
				t.Fatalf("cached failure lost its explanation: %s", failure.message())
			}
		})
	}
}

func TestMovieCollectionDoesNotBorrowOtherMoviesNFO(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "The Dark Knight Collection 2005-2012")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Batman.Begins.2005.nfo"), []byte("<movie><title>Batman Begins</title><year>2005</year></movie>"), 0o644); err != nil {
		t.Fatal(err)
	}
	meta, err := ReadLocalMetadata(filepath.Join(dir, "The.Dark.Knight.2008.strm"), filepath.Dir(dir), false)
	if err != nil || (meta != nil && meta.HasNFO) {
		t.Fatalf("collection borrowed another movie's NFO: %+v, %v", meta, err)
	}
	media := model.Media{Title: "Confirmed Title", Year: 2010, TMDbID: 1, ScrapeStatus: "matched"}
	applyLocalMetadata(&media, &LocalMetadata{PathHint: true, Title: "Guessed Title", Year: 2001})
	if media.Title != "Confirmed Title" || media.Year != 2010 || media.TMDbID != 1 {
		t.Fatalf("confirmed identity overwritten: %+v", media)
	}
}
