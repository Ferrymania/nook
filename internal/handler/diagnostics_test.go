package handler

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/middleware"
	"github.com/Ferrymania/nook/internal/service"
)

func diagnosticsRouter(t *testing.T) (*gin.Engine, *service.DiagnosticsService) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{Secrets: config.SecretsConfig{JWTSecret: "diagnostics-test"}}
	s := service.NewDiagnosticsService()
	router := gin.New()
	api := router.Group("/api", middleware.GzipAPI())
	registerAdminRoutes(api, cfg, &service.Container{Cfg: cfg, Log: zap.NewNop(), Diagnostics: s})
	t.Cleanup(func() { s.Disable() })
	return router, s
}

func diagnosticsRequest(t *testing.T, method, path, role string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(method, "/api/admin/diagnostics/pprof/"+path, nil)
	if role != "" {
		raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, middleware.Claims{
			UserID: "operator", Role: role,
			RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
		}).SignedString([]byte("diagnostics-test"))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Authorization", "Bearer "+raw)
	}
	return r
}

func TestDiagnosticsRoutesRequireAdminAndOptIn(t *testing.T) {
	router, s := diagnosticsRouter(t)
	for _, endpoint := range []struct{ method, path string }{
		{"GET", "status"}, {"POST", "enable"}, {"POST", "disable"},
		{"GET", "heap"}, {"GET", "allocs"}, {"GET", "goroutine"}, {"GET", "profile"},
	} {
		for _, role := range []string{"", "user"} {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, diagnosticsRequest(t, endpoint.method, endpoint.path, role))
			want := http.StatusUnauthorized
			if role == "user" {
				want = http.StatusForbidden
			}
			if w.Code != want {
				t.Fatalf("%s %s role=%s: %d %s", endpoint.method, endpoint.path, role, w.Code, w.Body.String())
			}
		}
	}
	for _, name := range []string{"heap", "allocs", "goroutine", "profile"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, diagnosticsRequest(t, "GET", name, "admin"))
		if w.Code != 404 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("disabled %s: %d %s", name, w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, diagnosticsRequest(t, "POST", "enable", "admin"))
	var status service.DiagnosticsStatus
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil || !status.Enabled || status.ExpiresAt == nil {
		t.Fatalf("enable failed: %d %s %v", w.Code, w.Body.String(), err)
	}
	if !s.Status().Enabled {
		t.Fatal("enable did not update shared service")
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, diagnosticsRequest(t, "POST", "disable", "admin"))
	if w.Code != 200 || s.Status().Enabled {
		t.Fatal("disable failed")
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, diagnosticsRequest(t, "GET", "status", "admin"))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"enabled":false`) {
		t.Fatalf("status failed: %s", w.Body.String())
	}
}

func TestDiagnosticsBinaryDownloadsAndQueryIsolation(t *testing.T) {
	router, s := diagnosticsRouter(t)
	s.Enable()
	for _, name := range []string{"heap", "allocs", "goroutine"} {
		// A huge delta duration or debug=2 must not change binary snapshots
		// into long-running captures or plaintext goroutine dumps.
		r := diagnosticsRequest(t, "GET", name+"?seconds=999999&gc=1&debug=2", "admin")
		r.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		assertDiagnosticsProfile(t, w, name)
	}
	for _, name := range []string{"trace", "cmdline", "symbol", "block", "mutex"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, diagnosticsRequest(t, "GET", name, "admin"))
		if w.Code != 404 {
			t.Fatalf("unexpected profile exposed: %s %d", name, w.Code)
		}
	}
}

func assertDiagnosticsProfile(t *testing.T, w *httptest.ResponseRecorder, name string) {
	t.Helper()
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/octet-stream" || w.Header().Get("Content-Encoding") != "" {
		t.Fatalf("invalid download %s: %d %v", name, w.Code, w.Header())
	}
	if !strings.Contains(w.Header().Get("Content-Disposition"), "nook-"+name+"-") {
		t.Fatalf("missing filename: %v", w.Header())
	}
	zr, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatalf("invalid pprof gzip: %v", err)
	}
	defer zr.Close()
	data, err := io.ReadAll(zr)
	if err != nil || len(data) == 0 || (len(data) > 1 && data[0] == 0x1f && data[1] == 0x8b) {
		t.Fatalf("empty or double-compressed profile: %d bytes %v", len(data), err)
	}
}

func TestDiagnosticsCPUValidationAndCancellation(t *testing.T) {
	router, s := diagnosticsRouter(t)
	s.Enable()
	for _, seconds := range []string{"", "0", "-1", "31", "abc", "1.5", "99999999999999999", "1&seconds=2"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, diagnosticsRequest(t, "GET", "profile?seconds="+seconds, "admin"))
		if w.Code != 400 || s.Status().Capturing {
			t.Fatalf("invalid seconds=%s: %d", seconds, w.Code)
		}
	}
	// The actual standard CPU profiler must stop promptly on disable, release
	// its process-wide resource, and allow a subsequent snapshot.
	r := diagnosticsRequest(t, "GET", "profile", "admin")
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { router.ServeHTTP(w, r); close(done) }()
	deadline := time.After(2 * time.Second)
	for !s.Status().Capturing {
		select {
		case <-deadline:
			t.Fatal("CPU capture did not start")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	conflict := httptest.NewRecorder()
	router.ServeHTTP(conflict, diagnosticsRequest(t, "GET", "heap", "admin"))
	if conflict.Code != 409 {
		t.Fatalf("concurrent snapshot was accepted: %d", conflict.Code)
	}
	s.Disable()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("CPU profiler did not stop after disable")
	}
	assertDiagnosticsProfile(t, w, "profile")
	if s.Status().Capturing {
		t.Fatal("CPU capture lease leaked")
	}
	s.Enable()
	w = httptest.NewRecorder()
	router.ServeHTTP(w, diagnosticsRequest(t, "GET", "heap", "admin"))
	assertDiagnosticsProfile(t, w, "heap")
}

func TestDiagnosticsControlsRejectCrossOrigin(t *testing.T) {
	router, s := diagnosticsRouter(t)
	for _, path := range []string{"enable", "disable"} {
		r := diagnosticsRequest(t, "POST", path, "admin")
		r.Header.Set("Origin", "https://another.example")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("cross-origin %s: %d", path, w.Code)
		}
	}
	if s.Status().Enabled {
		t.Fatal("cross-origin enable changed state")
	}
	// The real browser download flow uses the administrator's existing cookie.
	r := diagnosticsRequest(t, "POST", "enable", "admin")
	r.AddCookie(&http.Cookie{Name: middleware.AccessTokenCookieName, Value: strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")})
	r.Header.Del("Authorization")
	r.Header.Set("Origin", "http://example.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != 200 || !s.Status().Enabled {
		t.Fatalf("same-origin administrator session was rejected: %d %s", w.Code, w.Body.String())
	}
}
