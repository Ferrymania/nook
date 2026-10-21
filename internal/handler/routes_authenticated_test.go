package handler

import (
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
		"GET /api/playback/:id/info",
		"GET /api/playback/:id/segments",
	} {
		if !routes[want] {
			t.Fatalf("%s route is not registered", want)
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
