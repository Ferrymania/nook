package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/service"
)

func TestAuthenticatedRouteSurfacesAreRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	Register(router, &config.Config{
		Secrets: config.SecretsConfig{JWTSecret: "test-secret"},
	}, zap.NewNop(), &service.Container{Log: zap.NewNop()})

	routes := map[string]bool{}
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}

	for _, want := range []string{
		"GET /api/me",
		"GET /api/me/pinned-libraries",
		"PUT /api/me/pinned-libraries",
		"GET /api/me/library-tags",
		"PUT /api/me/library-tags",
		"GET /api/auth/permissions",
		"GET /api/libraries",
		"GET /api/media",
		"GET /api/stream/:id",
		"GET /api/storage",
		"GET /api/watch-history",
		"PATCH /api/watch-history/:id/played",
		"GET /api/events",
		"GET /api/ws",
		"GET /api/playback/:id/info",
		"GET /api/playback/:id/segments",
	} {
		if !routes[want] {
			t.Fatalf("%s route is not registered", want)
		}
	}
}

func TestRetiredSystemCompatibilityAPIsAreUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	Register(router, &config.Config{
		Secrets: config.SecretsConfig{JWTSecret: "test-secret"},
	}, zap.NewNop(), &service.Container{Log: zap.NewNop()})

	retiredPaths := []string{
		"/api/system/config",
		"/api/settings/schema",
		"/api/system/events/ticket",
	}
	for _, route := range router.Routes() {
		for _, path := range retiredPaths {
			if strings.TrimSuffix(route.Path, "/") == path {
				t.Fatalf("retired system route still registered: %s %s", route.Method, route.Path)
			}
		}
	}
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		for _, path := range retiredPaths {
			for _, suffix := range []string{"", "/"} {
				response := httptest.NewRecorder()
				router.ServeHTTP(response, httptest.NewRequest(method, path+suffix, nil))
				if response.Code != http.StatusNotFound {
					t.Errorf("%s %s = %d, want 404", method, path+suffix, response.Code)
				}
			}
		}
	}
}

func TestRealtimeAPIsRequireAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	Register(router, &config.Config{
		Secrets: config.SecretsConfig{JWTSecret: "test-secret"},
	}, zap.NewNop(), &service.Container{Log: zap.NewNop()})

	for _, path := range []string{"/api/events", "/api/ws"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusUnauthorized {
			t.Errorf("GET %s without authentication = %d, want 401", path, response.Code)
		}
	}
}

func TestRetiredReadingAPIsAreUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	Register(router, &config.Config{
		Secrets: config.SecretsConfig{JWTSecret: "test-secret"},
	}, zap.NewNop(), &service.Container{Log: zap.NewNop()})

	// Cover authenticated APIs and the former signed public media proxies.
	for _, route := range router.Routes() {
		if route.Path == "/api/reader" || strings.HasPrefix(route.Path, "/api/reader/") {
			t.Errorf("retired route still registered: %s %s", route.Method, route.Path)
		}
	}
	for _, request := range []struct{ method, path string }{
		{"GET", "/api/reader/books"},
		{"POST", "/api/reader/sources"},
		{"GET", "/api/reader/profile"},
		{"GET", "/api/reader/media"},
		{"GET", "/api/reader/local/asset"},
		{"HEAD", "/api/reader/local/audio"},
		{"GET", "/api/reader/audio/transcode"},
		{"GET", "/api/reader/browser/page"},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(request.method, request.path, nil))
		if response.Code != 404 {
			t.Errorf("%s %s = %d, want 404", request.method, request.path, response.Code)
		}
	}
}

func TestRetiredDLNAAPIsAreUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	Register(router, &config.Config{
		Secrets: config.SecretsConfig{JWTSecret: "test-secret"},
	}, zap.NewNop(), &service.Container{Log: zap.NewNop()})

	for _, route := range router.Routes() {
		if route.Path == "/api/dlna" || strings.HasPrefix(route.Path, "/api/dlna/") {
			t.Errorf("retired route still registered: %s %s", route.Method, route.Path)
		}
	}
	for _, request := range []struct{ method, path string }{
		{http.MethodGet, "/api/dlna/devices"},
		{http.MethodPost, "/api/dlna/cast"},
		{http.MethodPost, "/api/dlna/device-1/play"},
		{http.MethodPost, "/api/dlna/device-1/pause"},
		{http.MethodPost, "/api/dlna/device-1/stop"},
		{http.MethodGet, "/api/dlna/device-1/status"},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(request.method, request.path, nil))
		if response.Code != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 404", request.method, request.path, response.Code)
		}
	}
}
