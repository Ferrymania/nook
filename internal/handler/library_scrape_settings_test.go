package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/middleware"
	"github.com/Ferrymania/nook/internal/service"
	"github.com/gin-gonic/gin"
)

func TestLibraryScrapeSettingsAdminOnlyAndValidated(t *testing.T) {
	router, svc, _, id := newLibraryFilterEnv(t)
	svc.Scraper = service.NewScraperService(&config.Config{}, svc.Log, svc.Repo, nil, nil, nil, nil, service.NewHub(svc.Log))
	role := func(c *gin.Context) { c.Set(middleware.CtxUserRole, c.GetHeader("X-Test-Role")); c.Next() }
	router.GET("/api/libraries/:id/scrape-settings", role, middleware.AdminRequired(), libraryScrapeSettingsHandler(svc))
	router.PUT("/api/libraries/:id/scrape-settings", role, middleware.AdminRequired(), saveLibraryScrapeSettingsHandler(svc))
	request := func(method, role, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/libraries/"+id+"/scrape-settings", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-Role", role)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	for _, method := range []string{"GET", "PUT"} {
		if w := request(method, "user", `{}`); w.Code != http.StatusForbidden {
			t.Fatalf("viewer accessed admin settings: %d", w.Code)
		}
	}
	body := `{"auto_probe":false,"settings":{"auto_on_scan":false,"providers":["tmdb","bangumi"],"language":"en-US","delay_min_ms":0,"delay_max_ms":0}}`
	if w := request("PUT", "admin", body); w.Code != http.StatusOK {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	view := service.LibraryScrapeSettingsView{}
	w := request("GET", "admin", "")
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &view) != nil || view.Settings.Language != "en-US" || view.Settings.AutoOnScan || view.Settings.DelayMaxMS != 0 {
		t.Fatalf("read saved settings: %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "api_key") || strings.Contains(w.Body.String(), "token") {
		t.Fatal("settings exposed credentials")
	}
	for _, invalid := range []string{strings.Replace(body, "en-US", "invalid", 1), strings.Replace(body, "\"delay_min_ms\":0", "\"delay_min_ms\":100", 1), strings.Replace(body, "\"tmdb\",\"bangumi\"", "\"unknown\"", 1)} {
		if w := request("PUT", "admin", invalid); w.Code != http.StatusBadRequest {
			t.Fatalf("invalid settings accepted: %d %s", w.Code, w.Body.String())
		}
	}
}

func TestLibraryScrapeSettingsPreserveGlobalEpisodeImagesScope(t *testing.T) {
	router, svc, _, id := newLibraryFilterEnv(t)
	svc.Scraper = service.NewScraperService(&config.Config{}, svc.Log, svc.Repo, nil, nil, nil, nil, service.NewHub(svc.Log))
	router.PUT("/api/libraries/:id/scrape-settings", saveLibraryScrapeSettingsHandler(svc))
	request := func(extra string) *httptest.ResponseRecorder {
		body := `{"auto_probe":false,"settings":{"providers":["tmdb"],"language":"zh-CN","delay_min_ms":0,"delay_max_ms":0}` + extra + `}`
		req := httptest.NewRequest("PUT", "/api/libraries/"+id+"/scrape-settings", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	for _, step := range []struct {
		extra string
		want  bool
	}{{`,"episode_images":true`, true}, {``, true}, {`,"episode_images":false`, false}} {
		w := request(step.extra)
		var result service.LibraryScrapeSettingsView
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.EpisodeImages != step.want {
			t.Fatalf("save: %d %s", w.Code, w.Body.String())
		}
		stored, err := svc.Repo.Setting.Get(t.Context(), "scrape.episode_images")
		if err != nil || stored != map[bool]string{true: "true", false: "false"}[step.want] {
			t.Fatalf("global preference: %q %v", stored, err)
		}
	}
}
