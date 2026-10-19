package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/middleware"
	"github.com/Ferrymania/nook/internal/service"
)

func TestSystemReleaseCheckRequiresAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	svc := &service.Container{Log: zap.NewNop()}
	registerAdminRoutes(router.Group("/api"), &config.Config{Secrets: config.SecretsConfig{JWTSecret: "release-test"}}, svc)
	for _, tc := range []struct {
		role string
		want int
	}{{"", 401}, {"user", 403}, {"admin", 503}} {
		req := httptest.NewRequest(http.MethodGet, "/api/admin/system/release?refresh=1", nil)
		if tc.role != "" {
			token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, middleware.Claims{UserID: "release-user", Role: tc.role}).SignedString([]byte("release-test"))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Fatalf("role=%q code=%d body=%s", tc.role, w.Code, w.Body)
		}
	}
}

func TestSystemReleaseCheckReturnsFailureStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/release", systemReleaseCheckHandler(&service.Container{ReleaseCheck: service.NewReleaseCheckService("nook-v0.0.26")}))
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // Exercise the handler without making an external request.
	req := httptest.NewRequest(http.MethodGet, "/release", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var status service.ReleaseCheckStatus
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK || status.Status != "unavailable" || status.CurrentVersion != "nook-v0.0.26" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected release response: code=%d body=%s", w.Code, w.Body)
	}
}
