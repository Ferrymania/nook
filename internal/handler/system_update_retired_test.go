package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/middleware"
	"github.com/Ferrymania/nook/internal/service"
)

func TestRetiredSystemUpdateAPIsAreUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	Register(router, &config.Config{
		Secrets: config.SecretsConfig{JWTSecret: "test-secret"},
	}, zap.NewNop(), &service.Container{Log: zap.NewNop()})

	for _, route := range router.Routes() {
		if route.Path == "/api/admin/system/update" || strings.HasPrefix(route.Path, "/api/admin/system/update/") {
			t.Fatalf("retired updater route still registered: %s %s", route.Method, route.Path)
		}
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, middleware.Claims{
		UserID: "admin", Role: "admin",
	}).SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		"/api/admin/system/update", "/api/admin/system/update/check", "/api/admin/system/update/apply",
	} {
		for _, suffix := range []string{"", "/"} {
			for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPost} {
				req := httptest.NewRequest(method, path+suffix, nil)
				req.Header.Set("Authorization", "Bearer "+token)
				response := httptest.NewRecorder()
				router.ServeHTTP(response, req)
				if response.Code != http.StatusNotFound {
					t.Errorf("%s %s = %d, want 404", method, path+suffix, response.Code)
				}
			}
		}
	}
}
