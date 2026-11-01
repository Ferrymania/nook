package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/database"
	"github.com/Ferrymania/nook/internal/middleware"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
	"github.com/Ferrymania/nook/internal/service"
	"github.com/Ferrymania/nook/internal/testutil"
	"gorm.io/gorm"
)

func TestGetDatabaseStatusHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	cfg.Database.Type = "postgres"
	svc := &service.Container{Database: service.NewDatabaseAdminService(cfg, nil)}
	r := gin.New()
	registerAdminDatabaseRoutes(r.Group("/api/admin"), svc)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/database/status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp database.DatabaseStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Type != "postgres" || resp.Connected || resp.Error == "" || len(resp.TableCounts) != 0 {
		t.Fatalf("expected explicit unavailable status without counts, got %+v", resp)
	}
}

func TestDatabaseConfigurationRoutesRemoved(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	registerAdminDatabaseRoutes(r.Group("/api/admin"), &service.Container{})
	for _, path := range []string{"/api/admin/database/test", "/api/admin/database/save-config"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("removed route %s returned %d", path, rec.Code)
		}
	}
}

func TestGetDatabaseStatusServiceUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/status", getDatabaseStatusHandler(nil))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/status", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestDatabaseStatusAccountLookupFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := testutil.Open(t, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatal(err)
	}
	svc := &service.Container{Repo: repository.New(db)}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set(middleware.CtxUserID, "00000000-0000-0000-0000-000000000001") })
	r.Use(activeUserRequired(svc))
	r.GET("/status", func(c *gin.Context) { t.Error("account checks must not be bypassed") })

	// A genuinely absent user still rejects the credentials.
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/status", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing user should return 401, got %d", rec.Code)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.Close(); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/status", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("database failure should return 503 without invalidating the browser session, got %d", rec.Code)
	}
}
