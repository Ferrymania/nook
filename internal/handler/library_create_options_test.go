package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/middleware"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
	"github.com/Ferrymania/nook/internal/service"
	"github.com/Ferrymania/nook/internal/testutil"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestLibraryCreateHTTPSettingsAndConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := testutil.Open(t, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.LibraryRoot{}, &model.Setting{}, &model.AccessLog{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	log := zap.NewNop()
	svc := &service.Container{Repo: repos, Log: log, Media: service.NewMediaService(&config.Config{}, log, repos), Audit: service.NewAuditService(log, repos)}
	svc.Scraper = service.NewScraperService(&config.Config{}, log, repos, nil, nil, nil, nil, service.NewHub(log))
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(middleware.CtxUserRole, c.GetHeader("X-Test-Role")); c.Next() })
	registerAuthedLibraryRoutes(router.Group("/api"), svc)
	request := func(method, path, role string, payload any) *httptest.ResponseRecorder {
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-Role", role)
		result := httptest.NewRecorder()
		router.ServeHTTP(result, req)
		return result
	}
	for _, role := range []string{"user", "admin"} {
		result := request("GET", "/api/libraries/scrape-defaults", role, nil)
		want := http.StatusOK
		if role == "user" {
			want = http.StatusForbidden
		}
		if result.Code != want {
			t.Fatalf("defaults access: %d %s", result.Code, result.Body.String())
		}
		if role == "admin" {
			var defaults service.LibraryScrapeSettingsView
			if err := json.Unmarshal(result.Body.Bytes(), &defaults); err != nil || defaults.Settings.Language == "" || len(defaults.Providers) == 0 {
				t.Fatalf("default settings = %#v %v", defaults, err)
			}
		}
	}
	prewarm := false
	payload := createLibraryReq{Name: "新建电影", Type: "movie", Roots: []service.LibraryRootInput{{Path: t.TempDir()}}, LibraryCreateOptions: service.LibraryCreateOptions{CreateOnly: true, AutoProbe: true, ScrapeSettings: &model.LibraryScrapeSettings{Providers: []string{"tmdb"}, Language: "en-US", DelayMinMS: 0, DelayMaxMS: 0, SegmentPrewarmEnabled: &prewarm}}}
	result := request("POST", "/api/libraries", "admin", payload)
	var created model.Library
	if result.Code != http.StatusCreated || json.Unmarshal(result.Body.Bytes(), &created) != nil {
		t.Fatalf("create: %d %s", result.Code, result.Body.String())
	}
	stored, err := repos.Library.FindByID(t.Context(), created.ID)
	if err != nil || stored == nil || !stored.AutoProbe || stored.ScrapeSettings == nil || stored.ScrapeSettings.Language != "en-US" || stored.ScrapeSettings.SegmentPrewarmEnabled == nil || *stored.ScrapeSettings.SegmentPrewarmEnabled {
		t.Fatalf("creation preferences missing: %#v %v", stored, err)
	}
	payload.Roots = []service.LibraryRootInput{{Path: t.TempDir()}}
	result = request("POST", "/api/libraries", "admin", payload)
	if result.Code != http.StatusConflict {
		t.Fatalf("duplicate should conflict: %d %s", result.Code, result.Body.String())
	}
	stored, err = repos.Library.FindByID(t.Context(), created.ID)
	if err != nil || len(stored.Roots) != 1 {
		t.Fatalf("duplicate request appended roots: %#v %v", stored, err)
	}
	payload.Name = "无效设置"
	payload.ScrapeSettings.Providers = []string{"fanart"}
	result = request("POST", "/api/libraries", "admin", payload)
	if result.Code != http.StatusBadRequest {
		t.Fatalf("invalid creation settings: %d %s", result.Code, result.Body.String())
	}
}
