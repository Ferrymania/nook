package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/middleware"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
	"github.com/Ferrymania/nook/internal/service"
	"github.com/Ferrymania/nook/internal/testutil"
)

func ffToolsRequest(t *testing.T, router http.Handler, method, path, role string, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	if role != "" {
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, middleware.Claims{UserID: "operator", Role: role, RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}).SignedString([]byte("ff-tools-test"))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestFFToolsRoutesRequireAdminAndHandleUnavailableService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	cfg := &config.Config{Secrets: config.SecretsConfig{JWTSecret: "ff-tools-test"}}
	registerAdminRoutes(router.Group("/api"), cfg, &service.Container{Cfg: cfg, Log: zap.NewNop()})
	for _, endpoint := range []struct{ method, path string }{{"GET", "/api/admin/tools/ffmpeg/detect"}, {"GET", "/api/admin/tools/ffmpeg/status"}, {"POST", "/api/admin/tools/ffmpeg/install"}} {
		for _, role := range []string{"", "user", "admin"} {
			w := ffToolsRequest(t, router, endpoint.method, endpoint.path, role, nil)
			want := map[string]int{"": 401, "user": 403, "admin": 503}[role]
			if w.Code != want {
				t.Fatalf("%s role=%q status=%d body=%s", endpoint.path, role, w.Code, w.Body.String())
			}
		}
	}
}

func TestFFToolsOnlySettingsSavePersistsAndAppliesDownloadedPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses local shell fixtures")
	}
	gin.SetMode(gin.TestMode)
	db, err := testutil.Open(t, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Setting{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	dataDir, oldDir := t.TempDir(), t.TempDir()
	paths := map[string]string{}
	for _, name := range []string{"ffmpeg", "ffprobe"} {
		for _, dir := range []string{oldDir, filepath.Join(dataDir, "tools", "ffmpeg", "download-retained")} {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, name)
			if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf '%s\\n' '"+name+" version fixture'\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			if dir != oldDir {
				paths[name] = path
			}
		}
		if err := repos.Setting.Set(t.Context(), name+".path", filepath.Join(oldDir, name)); err != nil {
			t.Fatal(err)
		}
	}
	cfg := &config.Config{App: config.AppConfig{DataDir: dataDir, FFmpegPath: filepath.Join(oldDir, "ffmpeg"), FFprobePath: filepath.Join(oldDir, "ffprobe")}, Secrets: config.SecretsConfig{JWTSecret: "ff-tools-test"}}
	svc := &service.Container{Cfg: cfg, Repo: repos, Log: zap.NewNop(), FFTools: service.NewFFmpegToolsService(cfg, zap.NewNop())}
	router := gin.New()
	registerAdminRoutes(router.Group("/api"), cfg, svc)
	for _, endpoint := range []struct{ method, path string }{{"GET", "/api/admin/tools/ffmpeg/detect"}, {"POST", "/api/admin/tools/ffmpeg/install"}} {
		w := ffToolsRequest(t, router, endpoint.method, endpoint.path, "admin", nil)
		if w.Code != 200 {
			t.Fatalf("%s status=%d body=%s", endpoint.path, w.Code, w.Body.String())
		}
		for _, name := range []string{"ffmpeg", "ffprobe"} {
			value, err := repos.Setting.Get(t.Context(), name+".path")
			if err != nil || value != filepath.Join(oldDir, name) {
				t.Fatalf("detection/install persisted %s=%s: %v", name, value, err)
			}
		}
		if cfg.App.FFmpegPath != filepath.Join(oldDir, "ffmpeg") || cfg.App.FFprobePath != filepath.Join(oldDir, "ffprobe") {
			t.Fatal("detection/install activated new paths")
		}
	}
	// Reopening the page/service sees retained downloads without activating them.
	svc.FFTools = service.NewFFmpegToolsService(cfg, zap.NewNop())
	w := ffToolsRequest(t, router, "GET", "/api/admin/tools/ffmpeg/status", "admin", nil)
	var payload struct {
		FFmpeg     struct{ Path string } `json:"ffmpeg"`
		Downloaded map[string]struct {
			Path      string
			Installed bool
		} `json:"downloaded"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.FFmpeg.Path != filepath.Join(oldDir, "ffmpeg") || payload.Downloaded["ffmpeg"].Path != paths["ffmpeg"] || !payload.Downloaded["ffprobe"].Installed {
		t.Fatalf("wrong reopened state: %s", w.Body.String())
	}
	for _, name := range []string{"ffmpeg", "ffprobe"} {
		w = ffToolsRequest(t, router, "PUT", "/api/admin/settings", "admin", map[string]string{"key": name + ".path", "value": paths[name]})
		if w.Code != http.StatusNoContent {
			t.Fatalf("save failed: %d %s", w.Code, w.Body.String())
		}
		value, err := repos.Setting.Get(t.Context(), name+".path")
		if err != nil || value != paths[name] {
			t.Fatalf("save did not persist %s: %s %v", name, value, err)
		}
	}
	if cfg.App.FFmpegPath != paths["ffmpeg"] || cfg.App.FFprobePath != paths["ffprobe"] {
		t.Fatal("saving did not activate downloaded tools")
	}
}
