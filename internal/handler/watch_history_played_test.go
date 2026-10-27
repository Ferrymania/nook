package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/middleware"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
	"github.com/Ferrymania/nook/internal/service"
	"github.com/Ferrymania/nook/internal/testutil"
)

func TestHistorySetPlayedValidatesRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, user, body string
		status           int
	}{
		{"no user", "", `{"played":true}`, http.StatusUnauthorized},
		{"missing played", "alice", `{}`, http.StatusBadRequest},
		{"null played", "alice", `{"played":null}`, http.StatusBadRequest},
		{"string played", "alice", `{"played":"true"}`, http.StatusBadRequest},
		{"invalid JSON", "alice", `{`, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) { c.Set(middleware.CtxUserID, tc.user) })
			router.PATCH("/watch-history/:id/played", historySetPlayedHandler(&service.Container{}))
			req := httptest.NewRequest(http.MethodPatch, "/watch-history/history-1/played", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", response.Code, tc.status, response.Body.String())
			}
		})
	}
}

func TestHistorySetPlayedIsolatesUsersAndUpdatesResume(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := testutil.Open(t, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Media{}, &model.PlaybackHistory{}); err != nil {
		t.Fatal(err)
	}
	media := model.Media{Base: model.Base{ID: "movie"}, Title: "电影", Path: "/media/movie.mkv", DurationSec: 120}
	if err := db.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	for _, user := range []string{"alice", "bob"} {
		if err := db.Create(&model.PlaybackHistory{
			Base: model.Base{ID: user + "-history"}, UserID: user, MediaID: media.ID,
			PositionMs: 30_000, DurationMs: 120_000, WatchedAt: time.Now(),
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	repos := repository.New(db)
	svc := &service.Container{Repo: repos, Emby: service.NewEmbyService(&config.Config{}, zap.NewNop(), repos)}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(middleware.CtxUserID, "alice") })
	router.PATCH("/watch-history/:id/played", historySetPlayedHandler(svc))
	request := func(id, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPatch, "/watch-history/"+id+"/played", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	for _, id := range []string{"bob-history", "missing-history"} {
		for _, body := range []string{`{"played":true}`, `{"played":false}`} {
			if response := request(id, body); response.Code != http.StatusNotFound {
				t.Fatalf("changing %s = %d, want 404: %s", id, response.Code, response.Body.String())
			}
		}
	}
	if response := request("alice-history", `{"played":true,"user_id":"bob"}`); response.Code != http.StatusOK {
		t.Fatalf("mark played = %d: %s", response.Code, response.Body.String())
	}
	var alice, bob model.PlaybackHistory
	if err := db.First(&alice, "user_id = ?", "alice").Error; err != nil {
		t.Fatal(err)
	}
	if !alice.Completed || alice.PositionMs != 120_000 || alice.DurationMs != 120_000 {
		t.Fatalf("watched state = %+v", alice)
	}
	if response := request(alice.ID, `{"played":false}`); response.Code != http.StatusOK {
		t.Fatalf("mark unplayed = %d: %s", response.Code, response.Body.String())
	}
	var aliceCount int64
	if err := db.Model(&model.PlaybackHistory{}).Where("user_id = ?", "alice").Count(&aliceCount).Error; err != nil {
		t.Fatal(err)
	}
	if aliceCount != 0 {
		t.Fatalf("unwatched should clear resume, remaining = %d", aliceCount)
	}
	if err := db.First(&bob, "user_id = ?", "bob").Error; err != nil {
		t.Fatal(err)
	}
	if bob.Completed || bob.PositionMs != 30_000 {
		t.Fatalf("another user's history changed: %+v", bob)
	}
	var mediaCount int64
	if err := db.Model(&model.Media{}).Count(&mediaCount).Error; err != nil || mediaCount != 1 {
		t.Fatalf("media should be retained, count = %d, error = %v", mediaCount, err)
	}
}
