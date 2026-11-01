package handler

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
	"github.com/Ferrymania/nook/internal/service"
	"github.com/Ferrymania/nook/internal/testutil"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Existing route tests must now supply a real account as well as a signed
// administrator token: the route group checks the database account state.
func addAdminRouteTestUser(t *testing.T, svc *service.Container, id string) {
	t.Helper()
	if svc.Repo == nil {
		db, err := testutil.Open(t)
		if err != nil {
			t.Fatal(err)
		}
		svc.Repo = repository.New(db)
	}
	if err := svc.Repo.DB.AutoMigrate(&model.User{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Repo.User.Create(t.Context(), &model.User{Base: model.Base{ID: id}, Username: id, Role: "admin", PasswordHash: "test-hash"}); err != nil {
		t.Fatal(err)
	}
}

func TestPasswordResetEnforcedAcrossWebAdminAndEmby(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("ADMIN_INITIAL_PASSWORD", "")
	db, err := testutil.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserPermission{}, &model.RefreshToken{}, &model.Setting{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	cfg := &config.Config{Secrets: config.SecretsConfig{JWTSecret: "password-reset-test"}}
	log := zap.NewNop()
	tokens := service.NewTokenService(cfg, log, repos)
	auth := service.NewAuthService(cfg, log, repos, tokens, service.NewPermissionService(log, repos))
	svc := &service.Container{Cfg: cfg, Log: log, Repo: repos, Auth: auth, Audit: service.NewAuditService(log, repos), Emby: service.NewEmbyService(cfg, log, repos)}
	u, pair, err := auth.Register(t.Context(), "owner", "custom-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := repos.User.UpdateFields(t.Context(), u.ID, map[string]any{"force_password_reset": true}); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	Register(router, cfg, log, svc)
	request := func(method, path, token, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	for _, path := range []string{"/api/libraries", "/api/admin/users", "/api/admin/settings", "/api/me/temporary-password", "/api/stream/movie", "/emby/Users/Me", "/emby/Items"} {
		w := request("GET", path, pair.AccessToken, "")
		if w.Code != 403 || !strings.Contains(w.Body.String(), "40305") {
			t.Fatalf("old token %s: %d %s", path, w.Code, w.Body.String())
		}
	}
	if w := request("GET", "/api/me", pair.AccessToken, ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"force_password_reset":true`) {
		t.Fatalf("me: %d %s", w.Code, w.Body.String())
	}
	if w := request("PATCH", "/api/me", pair.AccessToken, `{}`); w.Code != 403 {
		t.Fatalf("profile change bypass: %d %s", w.Code, w.Body.String())
	}
	if w := request("POST", "/emby/Users/AuthenticateByName", "", `{"Username":"owner","Pw":"custom-password"}`); w.Code != 403 {
		t.Fatalf("Emby login: %d %s", w.Code, w.Body.String())
	}
	if w := request("POST", "/api/auth/refresh", "", `{"refresh_token":"`+pair.RefreshToken+`"}`); w.Code != 403 {
		t.Fatalf("refresh: %d %s", w.Code, w.Body.String())
	}
	login := request("POST", "/api/auth/login", "", `{"username":"owner","password":"custom-password"}`)
	var response struct {
		User   model.User        `json:"user"`
		Tokens service.TokenPair `json:"tokens"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &response); err != nil || login.Code != 200 || !response.User.ForcePasswordReset || response.Tokens.RefreshToken != "" {
		t.Fatalf("reset login: %d %s %v", login.Code, login.Body.String(), err)
	}
	resetToken := response.Tokens.AccessToken
	for _, path := range []string{"/api/admin/users", "/api/libraries", "/api/stream/movie", "/emby/Items"} {
		if w := request("GET", path, resetToken, ""); w.Code != 403 && w.Code != 401 {
			t.Fatalf("reset token %s: %d %s", path, w.Code, w.Body.String())
		}
	}
	if w := request("POST", "/api/me/password", resetToken, `{"old_password":"custom-password","new_password":"custom-password"}`); w.Code != 400 {
		t.Fatalf("same password: %d %s", w.Code, w.Body.String())
	}
	changed := request("POST", "/api/me/password", resetToken, `{"old_password":"custom-password","new_password":"changed-password"}`)
	if changed.Code != 204 {
		t.Fatalf("change: %d %s", changed.Code, changed.Body.String())
	}
	for _, path := range []string{"/api/admin/users", "/api/me"} {
		if w := request("GET", path, resetToken, ""); w.Code != 401 && w.Code != 403 {
			t.Fatalf("reset token upgraded after change %s: %d %s", path, w.Code, w.Body.String())
		}
	}
	if w := request("POST", "/api/auth/refresh", "", `{"refresh_token":"`+pair.RefreshToken+`"}`); w.Code != 401 {
		t.Fatalf("old refresh after change: %d %s", w.Code, w.Body.String())
	}
	login = request("POST", "/api/auth/login", "", `{"username":"owner","password":"changed-password"}`)
	if err := json.Unmarshal(login.Body.Bytes(), &response); err != nil || login.Code != 200 || response.User.ForcePasswordReset || response.Tokens.RefreshToken == "" {
		t.Fatalf("normal login: %d %s %v", login.Code, login.Body.String(), err)
	}
	if w := request("GET", "/api/admin/users/limit", response.Tokens.AccessToken, ""); w.Code != 200 {
		t.Fatalf("admin after reset: %d %s", w.Code, w.Body.String())
	}
}
