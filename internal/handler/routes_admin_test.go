package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/service"
)

func TestRetiredOrganizerAPIsAreUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	Register(router, &config.Config{
		Secrets: config.SecretsConfig{JWTSecret: "test-secret"},
	}, zap.NewNop(), &service.Container{Log: zap.NewNop()})

	retired := map[string]string{
		"/api/admin/media/:id/organize":     "/api/admin/media/media-id/organize",
		"/api/admin/libraries/:id/organize": "/api/admin/libraries/library-id/organize",
		"/api/admin/organize/source":        "/api/admin/organize/source",
		"/api/media/organize":               "/api/media/organize",
	}
	for _, route := range router.Routes() {
		if _, exists := retired[route.Path]; exists {
			t.Fatalf("retired organizer route still registered: %s %s", route.Method, route.Path)
		}
	}
	for _, path := range retired {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, nil))
		if response.Code != http.StatusNotFound {
			t.Errorf("POST %s = %d, want 404", path, response.Code)
		}
	}
}

func TestAdminRouteSurfacesAreRegistered(t *testing.T) {
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
		"GET /api/admin/settings",
		"PUT /api/admin/settings",
		"GET /api/admin/system/release",
		"GET /api/admin/users",
		"GET /api/admin/users/:id/permissions",
		"POST /api/admin/backups",
		"GET /api/admin/api-configs",
		"GET /api/admin/api-configs/:provider",
		"PUT /api/admin/api-configs/:provider",
		"DELETE /api/admin/api-configs/:provider",
		"POST /api/admin/api-configs/:provider/test",
		"GET /api/admin/tools/ffmpeg/detect",
		"GET /api/admin/tools/ffmpeg/status",
		"POST /api/media/:id/segments/query",
	} {
		if !routes[want] {
			t.Fatalf("%s route is not registered", want)
		}
	}
}
