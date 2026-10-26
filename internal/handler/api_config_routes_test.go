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

func TestRetiredAPIConfigRoutesAreUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	Register(router, &config.Config{
		Secrets: config.SecretsConfig{JWTSecret: "test-secret"},
	}, zap.NewNop(), &service.Container{Log: zap.NewNop()})

	for _, route := range router.Routes() {
		if route.Path == "/api/api-config" || strings.HasPrefix(route.Path, "/api/api-config/") {
			t.Fatalf("retired API config route still registered: %s %s", route.Method, route.Path)
		}
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, middleware.Claims{
		UserID: "admin", Role: "admin",
	}).SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []struct{ method, path string }{
		{http.MethodGet, "/api/api-config"},
		{http.MethodGet, "/api/api-config/providers/list"},
		{http.MethodGet, "/api/api-config/tmdb"},
		{http.MethodGet, "/api/api-config/tmdb/effective"},
		{http.MethodPost, "/api/api-config/tmdb"},
		{http.MethodDelete, "/api/api-config/tmdb"},
		{http.MethodPost, "/api/api-config/tmdb/test"},
		{http.MethodHead, "/api/api-config/tmdb"},
	} {
		for _, suffix := range []string{"", "/"} {
			req := httptest.NewRequest(request.method, request.path+suffix, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != http.StatusNotFound {
				t.Errorf("%s %s = %d, want 404", request.method, request.path+suffix, response.Code)
			}
		}
	}
}

func TestCurrentAPIConfigRoutesRequireAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	Register(router, &config.Config{
		Secrets: config.SecretsConfig{JWTSecret: "test-secret"},
	}, zap.NewNop(), &service.Container{Log: zap.NewNop()})

	userToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, middleware.Claims{
		UserID: "viewer", Role: "user",
	}).SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []struct{ method, path string }{
		{http.MethodGet, "/api/admin/api-configs"},
		{http.MethodGet, "/api/admin/api-configs/tmdb"},
		{http.MethodPut, "/api/admin/api-configs/tmdb"},
		{http.MethodDelete, "/api/admin/api-configs/tmdb"},
		{http.MethodPost, "/api/admin/api-configs/tmdb/test"},
	} {
		for _, auth := range []struct {
			token string
			want  int
		}{
			{"", http.StatusUnauthorized},
			{userToken, http.StatusForbidden},
		} {
			req := httptest.NewRequest(request.method, request.path, nil)
			if auth.token != "" {
				req.Header.Set("Authorization", "Bearer "+auth.token)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != auth.want {
				t.Errorf("%s %s with token=%t = %d, want %d", request.method, request.path, auth.token != "", response.Code, auth.want)
			}
		}
	}
}
