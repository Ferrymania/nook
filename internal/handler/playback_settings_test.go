package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/middleware"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

func TestPlaybackSettingsRoutesAreAccountScopedAndProtectAdultAccess(t *testing.T) {
	router, svc := newDeviceTestEnv(t)
	seedDeviceUsers(t, svc)
	svc.Profile = service.NewProfileService(zap.NewNop(), svc.Repo)
	svc.Auth = service.NewAuthService(&config.Config{}, zap.NewNop(), svc.Repo, nil, nil)
	hash, err := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Repo.User.UpdateFields(t.Context(), "user-a", map[string]any{"password_hash": string(hash)}); err != nil {
		t.Fatal(err)
	}
	request := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPut, "/api/me/playback-settings", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-User", "user-a")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	w := request(`{"user_id":"user-b","allow_adult":false,"autoplay_next":false,"skip_intro":true,"preferred_audio_lang":"ja","allowed_library_ids":[]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	got := doJSON(t, router, http.MethodGet, "/api/me/playback-settings", "user-a", "user")
	var settings model.PlaybackSettings
	if err := json.Unmarshal(got.Body.Bytes(), &settings); err != nil {
		t.Fatal(err)
	}
	if got.Code != http.StatusOK || !settings.SkipIntro || settings.AutoplayNext || settings.PreferredAudioLang != "ja" {
		t.Fatalf("settings = %+v response = %s", settings, got.Body.String())
	}
	other, err := svc.Profile.GetPlaybackSettings(t.Context(), "user-b")
	if err != nil || other.SkipIntro || !other.AutoplayNext {
		t.Fatalf("another account changed: %+v %v", other, err)
	}
	if w := request(`{"allow_adult":true,"autoplay_next":true}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("adult access changed without password: %d %s", w.Code, w.Body.String())
	}
	if w := request(`{"allow_adult":true,"autoplay_next":true,"password":"test-password"}`); w.Code != http.StatusOK {
		t.Fatalf("confirmed change failed: %d %s", w.Code, w.Body.String())
	}
	user, err := svc.Repo.User.FindByID(t.Context(), "user-a")
	if err != nil || user.HideAdult {
		t.Fatalf("account visibility did not update: %+v %v", user, err)
	}
}

func TestPlaybackSettingsLibraryPickerKeepsAccountAccess(t *testing.T) {
	router, svc := newDeviceTestEnv(t)
	seedDeviceUsers(t, svc)
	if err := svc.Repo.DB.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatal(err)
	}
	svc.Media = service.NewMediaService(&config.Config{}, zap.NewNop(), svc.Repo)
	for _, id := range []string{"lib-a", "lib-b", "lib-denied"} {
		if err := svc.Repo.Library.Create(t.Context(), &model.Library{Base: model.Base{ID: id}, Name: id, Path: "/media/" + id, Type: "movie", Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.Repo.User.UpdateFields(t.Context(), "user-a", map[string]any{
		"allowed_library_ids": `["lib-a","lib-b"]`,
		"playback_settings":   `{"allowed_library_ids":["lib-a"]}`,
	}); err != nil {
		t.Fatal(err)
	}
	router.GET("/api/libraries", func(c *gin.Context) {
		c.Set(middleware.CtxUserID, "user-a")
		c.Set(middleware.CtxUserRole, "user")
		listLibrariesHandler(svc)(c)
	})
	for _, tt := range []struct {
		path string
		want int
	}{
		{"/api/me/playback-settings/libraries", 2},
		{"/api/libraries", 1},
	} {
		w := doJSON(t, router, http.MethodGet, tt.path, "user-a", "user")
		var rows []webLibraryPayload
		if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
			t.Fatal(err)
		}
		if w.Code != http.StatusOK || len(rows) != tt.want {
			t.Fatalf("%s returned %d %s", tt.path, w.Code, w.Body.String())
		}
		for _, row := range rows {
			if row.ID == "lib-denied" {
				t.Fatal("settings picker exposed an administrator-restricted library")
			}
		}
	}
}

func TestPlayProfileRoutesRemoved(t *testing.T) {
	router, svc := newDeviceTestEnv(t)
	registerAuthedUISurfaceRoutes(router.Group("/api"), svc)
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/play-profiles"}, {http.MethodPost, "/api/play-profiles"},
		{http.MethodPut, "/api/play-profiles/old"}, {http.MethodDelete, "/api/play-profiles/old"},
		{http.MethodPost, "/api/play-profiles/old/verify-pin"},
	} {
		w := doJSON(t, router, route.method, route.path, "user-a", "user")
		if w.Code != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 404", route.method, route.path, w.Code)
		}
	}
}
