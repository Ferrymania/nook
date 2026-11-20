package handler

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/middleware"
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

func TestChangePasswordOptionRevokesExistingSessions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := testutil.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserPermission{}, &model.RefreshToken{}, &model.Setting{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	cfg := &config.Config{Secrets: config.SecretsConfig{JWTSecret: "password-change-sessions-test"}}
	log := zap.NewNop()
	auth := service.NewAuthService(cfg, log, repos, service.NewTokenService(cfg, log, repos), service.NewPermissionService(log, repos))
	svc := &service.Container{Cfg: cfg, Log: log, Repo: repos, Auth: auth, Audit: service.NewAuditService(log, repos), Emby: service.NewEmbyService(cfg, log, repos)}
	ctx := t.Context()
	user, pair, err := auth.Register(ctx, "owner", "original-password")
	if err != nil {
		t.Fatal(err)
	}
	_, otherPair, err := auth.Register(ctx, "other", "another-password")
	if err != nil {
		t.Fatal(err)
	}
	_, embyToken, err := auth.LoginEmby(ctx, user.Username, "original-password")
	if err != nil {
		t.Fatal(err)
	}
	code, _, err := auth.CreateTemporaryPassword(ctx, user.ID, user.SessionVersion)
	if err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	Register(router, cfg, log, svc)
	request := func(method, path, token, body string, status int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != status {
			t.Fatalf("%s %s: got %d, want %d: %s", method, path, w.Code, status, w.Body.String())
		}
		return w
	}
	request("POST", "/api/me/password", pair.AccessToken, `{"old_password":"original-password","new_password":"updated-password"}`, 204)
	request("POST", "/api/me/password", pair.AccessToken, `{"old_password":"wrong-password","new_password":"final-password","sign_out_devices":true}`, 400)
	request("GET", "/api/me", pair.AccessToken, "", 200)
	request("GET", "/emby/Users/Me", embyToken, "", 200)

	rotated, err := auth.RefreshTokens(ctx, pair.RefreshToken)
	if err != nil {
		t.Fatalf("unchecked change invalidated refresh token: %v", err)
	}
	changed := request("POST", "/api/me/password", pair.AccessToken, `{"old_password":"updated-password","new_password":"final-password","sign_out_devices":true}`, 204)
	cookies := changed.Result().Cookies()
	if len(cookies) == 0 || cookies[len(cookies)-1].Name != middleware.AccessTokenCookieName || cookies[len(cookies)-1].MaxAge >= 0 {
		t.Fatal("password change did not clear the access token cookie")
	}
	request("GET", "/api/me", pair.AccessToken, "", 401)
	request("GET", "/api/admin/users/limit", rotated.AccessToken, "", 401)
	request("GET", "/emby/Users/Me", embyToken, "", 401)
	for _, refreshToken := range []string{pair.RefreshToken, rotated.RefreshToken} {
		request("POST", "/api/auth/refresh", "", `{"refresh_token":"`+refreshToken+`"}`, 401)
	}
	request("GET", "/api/me", otherPair.AccessToken, "", 200)
	if _, ok := auth.VerifyAndConsumeTemporaryPassword(ctx, user.Username, code); ok {
		t.Fatal("temporary password survived session revocation")
	}

	request("POST", "/api/auth/login", "", `{"username":"owner","password":"updated-password"}`, 401)
	login, err := auth.Login(ctx, user.Username, "final-password")
	if err != nil {
		t.Fatal(err)
	}
	_, newEmbyToken, err := auth.LoginEmby(ctx, user.Username, "final-password")
	if err != nil {
		t.Fatal(err)
	}
	request("GET", "/api/me", login.Tokens.AccessToken, "", 200)
	request("GET", "/emby/Users/Me", newEmbyToken, "", 200)
	request("POST", "/api/auth/refresh", "", `{"refresh_token":"`+login.Tokens.RefreshToken+`"}`, 200)
}
