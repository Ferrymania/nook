package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/service"
)

func TestScrapeQueueRespectsEpisodeImagePreference(t *testing.T) {
	router, svc, _, libraryID := newLibraryFilterEnv(t)
	if err := svc.Repo.DB.AutoMigrate(&model.ScrapeTask{}, &model.Series{}); err != nil {
		t.Fatal(err)
	}
	svc.Scraper = service.NewScraperService(&config.Config{}, svc.Log, svc.Repo, nil, nil, nil, nil, service.NewHub(svc.Log))
	seedLibraryMedia(t, svc, &model.Media{Base: model.Base{ID: "episode-options"}, LibraryID: libraryID, Path: "/media/movies/test.mkv", Title: "fixture", ScrapeStatus: "matched"})
	router.POST("/api/enqueue-all", enqueueAllScrapeHandler(svc))
	router.POST("/api/enqueue-library/:id", enqueueLibraryScrapeHandler(svc))
	router.POST("/api/enqueue-media/:id", scrapeOneHandler(svc))
	for _, endpoint := range []string{"/api/enqueue-all", "/api/enqueue-library/" + libraryID, "/api/enqueue-media/episode-options"} {
		for _, tc := range []struct {
			name, setting, extra string
			want                 bool
		}{
			{"stored-off", "false", "", false},
			{"stored-on", "true", "", true},
			{"explicit-off", "true", `,"episode_images":false`, false},
			{"explicit-on", "false", `,"episode_images":true`, true},
			{"legacy-option", "false", `,"episode_artwork":true`, true},
			{"modern-option-wins", "true", `,"episode_images":false,"episode_artwork":true`, false},
		} {
			t.Run(endpoint+"/"+tc.name, func(t *testing.T) {
				if err := svc.Repo.DB.Unscoped().Where("1=1").Delete(&model.ScrapeTask{}).Error; err != nil {
					t.Fatal(err)
				}
				if err := svc.Repo.Setting.Set(t.Context(), "scrape.episode_images", tc.setting); err != nil {
					t.Fatal(err)
				}
				w := httptest.NewRecorder()
				req := httptest.NewRequest("POST", endpoint, strings.NewReader(`{"include_matched":true`+tc.extra+`}`))
				req.Header.Set("Content-Type", "application/json")
				router.ServeHTTP(w, req)
				if w.Code != http.StatusOK {
					t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
				}
				var task model.ScrapeTask
				if err := svc.Repo.DB.First(&task).Error; err != nil {
					t.Fatal(err)
				}
				if task.EpisodeImages != tc.want {
					t.Fatalf("queued episode_images=%v, want %v", task.EpisodeImages, tc.want)
				}
				// Changing the setting later must not rewrite a queued job's options.
				if err := svc.Repo.Setting.Set(t.Context(), "scrape.episode_images", map[bool]string{true: "false", false: "true"}[tc.want]); err != nil {
					t.Fatal(err)
				}
				if err := svc.Repo.DB.First(&task, "id = ?", task.ID).Error; err != nil {
					t.Fatal(err)
				}
				if task.EpisodeImages != tc.want {
					t.Fatal("queued option changed after saving settings")
				}
			})
		}
	}
	if err := svc.Repo.DB.Unscoped().Where("1=1").Delete(&model.ScrapeTask{}).Error; err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/enqueue-all", strings.NewReader(`{"episode_images":`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("malformed options accepted: %d %s", w.Code, w.Body.String())
	}
	var count int64
	if err := svc.Repo.DB.Model(&model.ScrapeTask{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("malformed request enqueued work: %d %v", count, err)
	}
}
