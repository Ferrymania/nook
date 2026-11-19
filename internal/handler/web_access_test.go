package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/middleware"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
	"github.com/Ferrymania/nook/internal/service"
	"github.com/Ferrymania/nook/internal/testutil"
)

func TestWebAccessPermissionPreservesEmbyPlayback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := testutil.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatal(err)
	}
	const secret = "web-access-test"
	cfg := &config.Config{Secrets: config.SecretsConfig{JWTSecret: secret}}
	log := zap.NewNop()
	repos := repository.New(db)
	tokens := service.NewTokenService(cfg, log, repos)
	auth := service.NewAuthService(cfg, log, repos, tokens, service.NewPermissionService(log, repos))
	svc := &service.Container{
		Cfg: cfg, Log: log, Repo: repos, Auth: auth, Audit: service.NewAuditService(log, repos),
		Emby:  service.NewEmbyService(cfg, log, repos),
		Media: service.NewMediaService(cfg, log, repos), Stream: service.NewStreamService(cfg, log, repos, nil),
	}
	// Seed a viewer with the old schema, then check the new column defaults to enabled.
	if err := db.Migrator().DropColumn(&model.User{}, "web_access_disabled"); err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	user := &model.User{Base: model.Base{ID: "viewer"}, Username: "viewer", Role: "user", PasswordHash: string(hash)}
	if err := db.Omit("WebAccessDisabled").Create(user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatal(err)
	}
	admin := &model.User{Base: model.Base{ID: "owner"}, Username: "owner", Role: "admin", PasswordHash: string(hash)}
	if err := repos.User.Create(t.Context(), admin); err != nil {
		t.Fatal(err)
	}
	adminToken, err := auth.IssueToken(admin)
	if err != nil {
		t.Fatal(err)
	}
	lib := &model.Library{Name: "Movies", Path: t.TempDir(), Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), lib); err != nil {
		t.Fatal(err)
	}
	media := &model.Media{Base: model.Base{ID: "movie"}, LibraryID: lib.ID, Title: "Movie", Path: filepath.Join(lib.Path, "Movie.mp4")}
	if err := os.WriteFile(media.Path, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := repos.Media.Upsert(t.Context(), media); err != nil {
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
		if strings.HasPrefix(path, "/api/stream/") {
			req.Header.Set("Range", "bytes=0-1")
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	requireStatus := func(w *httptest.ResponseRecorder, want int) {
		t.Helper()
		if w.Code != want {
			t.Fatalf("status = %d, want %d: %s", w.Code, want, w.Body)
		}
	}
	webLogin := func() *httptest.ResponseRecorder {
		return request(http.MethodPost, "/api/auth/login", "", `{"username":"viewer","password":"test-password"}`)
	}
	w := webLogin()
	requireStatus(w, http.StatusOK)
	var login service.LoginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &login); err != nil || login.Tokens == nil {
		t.Fatalf("web login response: %v %s", err, w.Body)
	}
	refreshBody := `{"refresh_token":"` + login.Tokens.RefreshToken + `"}`
	w = request(http.MethodPost, "/api/auth/refresh", "", refreshBody)
	requireStatus(w, http.StatusOK)
	var rotated struct {
		Data struct {
			RefreshToken string `json:"refresh_token"`
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &rotated); err != nil || rotated.Data.RefreshToken == "" {
		t.Fatalf("refresh response: %v %s", err, w.Body)
	}
	accessPath := "/api/admin/users/" + user.ID + "/libraries"
	w = request(http.MethodPatch, accessPath, adminToken, `{"web_access_disabled":true,"allowed_library_ids":null}`)
	requireStatus(w, http.StatusOK)
	if !strings.Contains(w.Body.String(), `"web_access_disabled":true`) {
		t.Fatalf("permission was not saved: %s", w.Body)
	}
	for _, denied := range []*httptest.ResponseRecorder{
		webLogin(),
		request(http.MethodGet, "/api/me", login.Tokens.AccessToken, ""),
		request(http.MethodPost, "/api/auth/refresh", "", refreshBody),
		request(http.MethodPost, "/api/auth/refresh", "", `{"refresh_token":"`+rotated.Data.RefreshToken+`"}`),
	} {
		requireStatus(denied, http.StatusForbidden)
		if !strings.Contains(denied.Body.String(), `"code":40306`) || !strings.Contains(denied.Body.String(), service.ErrWebAccessDisabled.Error()) {
			t.Fatalf("missing web access error: %s", denied.Body)
		}
		cookies := denied.Result().Cookies()
		if len(cookies) == 0 || cookies[len(cookies)-1].Name != middleware.AccessTokenCookieName || cookies[len(cookies)-1].MaxAge >= 0 {
			t.Fatalf("web access cookie was not cleared: %v", cookies)
		}
	}

	w = request(http.MethodPost, "/emby/Users/AuthenticateByName", "", `{"Username":"viewer","Pw":"test-password"}`)
	requireStatus(w, http.StatusOK)
	var embyLogin struct{ AccessToken string }
	if err := json.Unmarshal(w.Body.Bytes(), &embyLogin); err != nil {
		t.Fatal(err)
	}
	claims, err := tokens.ValidateAccessToken(embyLogin.AccessToken)
	if err != nil || claims.Purpose != model.EmbyTokenPurpose {
		t.Fatalf("Emby login token = %#v, error %v", claims, err)
	}
	now := time.Now()
	legacyToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, middleware.Claims{
		UserID: user.ID, Role: user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: "nook", Subject: user.ID, IssuedAt: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(model.EmbyTokenDuration)),
		},
	}).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{embyLogin.AccessToken, legacyToken} {
		requireStatus(request(http.MethodGet, "/emby/Users/Me", token, ""), http.StatusOK)
		requireStatus(request(http.MethodGet, "/emby/Items", token, ""), http.StatusOK)
		requireStatus(request(http.MethodGet, "/api/me", token, ""), http.StatusForbidden)
		w := request(http.MethodGet, "/api/stream/"+media.ID+"?api_key="+url.QueryEscape(token), "", "")
		requireStatus(w, http.StatusPartialContent)
		if w.Body.String() != "vi" || len(w.Result().Cookies()) != 0 {
			t.Fatalf("shared playback body or cookies: %q %v", w.Body, w.Result().Cookies())
		}
	}

	requireStatus(request(http.MethodPatch, "/api/admin/users/"+admin.ID+"/libraries", adminToken, `{"web_access_disabled":true}`), http.StatusOK)
	requireStatus(request(http.MethodGet, "/api/me", adminToken, ""), http.StatusOK)
	requireStatus(request(http.MethodPatch, accessPath, adminToken, `{"web_access_disabled":false}`), http.StatusOK)
	requireStatus(webLogin(), http.StatusOK)
}
