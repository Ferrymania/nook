package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Ferrymania/nook/internal/middleware"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/service"
)

func TestStatsTopUsersActivity(t *testing.T) {
	router, svc := newDeviceTestEnv(t)
	seedDeviceUsers(t, svc)
	if err := svc.Repo.DB.AutoMigrate(&model.PlaybackHistory{}); err != nil {
		t.Fatal(err)
	}
	router.GET("/api/stats/top-users", func(c *gin.Context) {
		c.Set(middleware.CtxUserRole, c.GetHeader("X-Test-Role"))
		c.Next()
	}, statsTopUsersHandler(svc))

	// Device activity is admin-only, even if the caller is on the leaderboard.
	w := doJSON(t, router, http.MethodGet, "/api/stats/top-users", "user-a", "user")
	if w.Code != http.StatusForbidden {
		t.Fatalf("non-admin status = %d", w.Code)
	}
	w = doJSON(t, router, http.MethodGet, "/api/stats/top-users", "admin", "admin")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Fatalf("empty leaderboard = %d %s", w.Code, w.Body.String())
	}

	if err := svc.Repo.DB.Create(&model.User{
		Base: model.Base{ID: "legacy"}, Username: "legacy", PasswordHash: "x", Role: "user",
	}).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	latest := now.Add(-time.Hour)
	devices := []model.UserDevice{
		{UserID: "user-a", DeviceID: "old-a", DeviceName: "Old TV", Client: "Infuse", LastSeenAt: now.Add(-2 * time.Hour)},
		{UserID: "user-a", DeviceID: "new-a", DeviceName: "iPad", Client: "SenPlayer", LastSeenAt: latest, LastIP: "192.0.2.1", Fingerprint: "private-fingerprint"},
		{UserID: "user-b", DeviceID: "old-b", DeviceName: "Old Phone", Client: "Infuse", LastSeenAt: now.Add(-24 * time.Hour)},
	}
	if err := svc.Repo.DB.Create(&devices).Error; err != nil {
		t.Fatal(err)
	}
	for i, userID := range []string{"user-a", "user-a", "user-a", "user-b", "user-b", "legacy"} {
		if err := svc.Repo.DB.Create(&model.PlaybackHistory{
			UserID: userID, MediaID: string(rune('a' + i)), WatchedAt: now.Add(-48 * time.Hour),
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	tracker := service.NewSessionTrackerService(svc.Log)
	svc.Device.SetSessionTracker(tracker)
	tracker.RecordLogin(t.Context(), "user-b", "user-b", "web-session", "Web", "Web", "192.0.2.2")
	liveTime := tracker.ListByUser(t.Context(), "user-b")[0].LastActivityAt

	w = doJSON(t, router, http.MethodGet, "/api/stats/top-users?limit=3", "admin", "admin")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var payload struct {
		Items []struct {
			UserID       string     `json:"user_id"`
			Username     string     `json:"username"`
			Plays        int64      `json:"plays"`
			LastActiveAt *time.Time `json:"last_active_at"`
			DeviceName   string     `json:"device_name"`
			Client       string     `json:"client"`
		} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Items) != 3 {
		t.Fatalf("items = %+v", payload.Items)
	}
	a, b, legacy := payload.Items[0], payload.Items[1], payload.Items[2]
	if a.UserID != "user-a" || a.Username != "user-a" || a.Plays != 3 || a.LastActiveAt == nil || !a.LastActiveAt.Equal(latest) || a.DeviceName != "iPad" || a.Client != "SenPlayer" {
		t.Fatalf("latest durable device = %+v", a)
	}
	if b.UserID != "user-b" || b.Plays != 2 || b.LastActiveAt == nil || !b.LastActiveAt.Equal(liveTime) || b.DeviceName != "Web" || b.Client != "Web" {
		t.Fatalf("latest realtime device = %+v", b)
	}
	if legacy.UserID != "legacy" || legacy.LastActiveAt != nil || legacy.DeviceName != "" || legacy.Client != "" {
		t.Fatalf("unreported activity must remain unknown: %+v", legacy)
	}
	for _, field := range []string{"last_ip", "device_id", "fingerprint", "remote_end_point"} {
		if strings.Contains(w.Body.String(), field) {
			t.Fatalf("leaderboard exposes %s: %s", field, w.Body.String())
		}
	}
}
