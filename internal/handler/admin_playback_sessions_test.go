package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Ferrymania/nook/internal/testutil"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/middleware"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
	"github.com/Ferrymania/nook/internal/service"
)

func newPlaybackSessionsTestService(t *testing.T) *service.Container {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := testutil.Open(t, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	log := zap.NewNop()
	tracker := service.NewSessionTrackerService(log)
	device := service.NewDeviceService(log, repos)
	device.SetSessionTracker(tracker)
	return &service.Container{Repo: repos, Sessions: tracker, Device: device,
		Emby: service.NewEmbyService(&config.Config{}, log, repos), Playback: service.NewPlaybackService(log, repos)}
}

func TestAdminPlaybackSessionsPermissionsAndDisplayMetadata(t *testing.T) {
	svc := newPlaybackSessionsTestService(t)
	user := model.User{Base: model.Base{ID: "u1"}, Username: "viewer", PasswordHash: "x", Role: "user", IsActive: true}
	if err := svc.Repo.DB.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	series := model.Series{Base: model.Base{ID: "series1"}, Title: "秘密森林"}
	media := model.Media{Base: model.Base{ID: "m1"}, Title: "Stranger", EpisodeTitle: "第三集", SeriesID: series.ID, SeasonNum: 2, EpisodeNum: 3, DurationSec: 3600, Path: "/private/video.strm", STRMURL: "https://secret.example/video?token=hidden"}
	if err := svc.Repo.DB.Create(&series).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.Repo.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	svc.Sessions.RecordPlaybackState(t.Context(), "u1", "", "d1", "Mac", "SenPlayer", "private-ip", service.PlaybackState{ItemID: "m1", PositionTicks: 1200000000, IsPaused: true})
	svc.Sessions.RecordPlayback(t.Context(), "u1", "", "d2", "iPhone", "Lenna", "", "m1", 0, 0, false)
	svc.Sessions.RecordLogin(t.Context(), "u2", "idle", "d3", "Web", "Web", "")
	router := gin.New()
	registerAdminRoutes(router.Group("/api"), &config.Config{Secrets: config.SecretsConfig{JWTSecret: "test-secret"}}, svc)
	for _, role := range []string{"", "user", "admin"} {
		req := httptest.NewRequest(http.MethodGet, "/api/admin/playback-sessions", nil)
		if role != "" {
			token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, middleware.Claims{UserID: "operator", Role: role, RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}).SignedString([]byte("test-secret"))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		want := map[string]int{"": 401, "user": 403, "admin": 200}[role]
		if w.Code != want {
			t.Fatalf("role %q status=%d body=%s", role, w.Code, w.Body.String())
		}
		if role != "admin" {
			continue
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("live response must not be cached")
		}
		var data struct {
			Items []adminPlaybackSession `json:"items"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
			t.Fatal(err)
		}
		if len(data.Items) != 2 {
			t.Fatalf("include all devices even without viewing history: %#v", data)
		}
		paused := false
		for _, row := range data.Items {
			if row.Username != "viewer" || row.SeriesName != "秘密森林" || row.SeasonNum != 2 || row.EpisodeNum != 3 || row.DurationMs != 3600000 {
				t.Fatalf("wrong metadata: %#v", row)
			}
			if row.IsPaused {
				paused = true
				if row.PositionMs != 120000 {
					t.Fatalf("wrong tick conversion: %#v", row)
				}
			}
		}
		if !paused {
			t.Fatal("paused session missing")
		}
		for _, secret := range []string{"private-ip", "/private/video", "secret.example", "hidden"} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatalf("response leaked %s", secret)
			}
		}
	}
}

func TestPlaybackReportsPreservePauseAndStopAcrossClients(t *testing.T) {
	svc := newPlaybackSessionsTestService(t)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(middleware.CtxUserID, "u1"); c.Next() })
	for _, route := range []string{"/Sessions/Playing", "/Sessions/Playing/Progress", "/Sessions/Playing/Stopped"} {
		router.POST(route, embyPlayingProgressHandler(svc))
	}
	router.POST("/history", recordProgressHandler(svc))
	post := func(path, body string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Emby-Authorization", `MediaBrowser Client="SenPlayer", Device="Mac", DeviceId="d1"`)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusNoContent {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
	}
	post("/Sessions/Playing", `{"ItemId":"m1","PositionTicks":0,"RunTimeTicks":600000000,"IsPaused":false}`)
	post("/Sessions/Playing/Progress?itemId=m1&positionTicks=10000000&runTimeTicks=600000000&isPaused=true", `{}`)
	if got := svc.Sessions.ListPlaying(t.Context()); len(got) != 1 || !got[0].IsPaused {
		t.Fatalf("query pause overwritten by device bookkeeping: %#v", got)
	}
	post("/Sessions/Playing/Progress", `{"ItemId":"m1","IsPaused":false,"RunTimeTicks":600000000}`)
	if got := svc.Sessions.ListPlaying(t.Context()); len(got) != 1 || got[0].IsPaused {
		t.Fatalf("resume failed: %#v", got)
	}
	post("/Sessions/Playing/Stopped", `{"ItemId":"m1","RunTimeTicks":600000000}`)
	if got := svc.Sessions.ListPlaying(t.Context()); len(got) != 0 {
		t.Fatalf("stop failed: %#v", got)
	}
	post("/history", `{"media_id":"m2","duration_ms":60000,"session_id":"web1","sequence":2,"is_paused":true}`)
	post("/history", `{"media_id":"m2","duration_ms":60000,"session_id":"web1","sequence":1,"is_paused":false}`)
	if got := svc.Sessions.ListPlaying(t.Context()); len(got) != 1 || !got[0].IsPaused || got[0].Client != "Nook Web" {
		t.Fatalf("web pause/stale ordering failed: %#v", got)
	}
	post("/history", `{"media_id":"m2","duration_ms":60000,"session_id":"web1","sequence":3,"is_paused":false,"stopped":true}`)
	post("/history", `{"media_id":"m2","duration_ms":60000,"session_id":"web1","sequence":2,"is_paused":false}`)
	if got := svc.Sessions.ListPlaying(t.Context()); len(got) != 0 {
		t.Fatalf("stale web report resurrected playback: %#v", got)
	}
}

func TestAdminPlaybackSessionsEmptyAndFailureAreDistinct(t *testing.T) {
	svc := newPlaybackSessionsTestService(t)
	router := gin.New()
	router.GET("/sessions", adminPlaybackSessionsHandler(svc))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/sessions", nil))
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"items":[]}` {
		t.Fatalf("wrong empty response: %d %s", w.Code, w.Body.String())
	}
	svc.Sessions.RecordPlayback(t.Context(), "u1", "viewer", "d1", "Mac", "SenPlayer", "", "m1", 1, 2, false)
	if err := svc.Repo.DB.Migrator().DropTable(&model.Media{}); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", "/sessions", nil))
	if w.Code != 500 {
		t.Fatalf("database failure must not look empty: %d %s", w.Code, w.Body.String())
	}
}
