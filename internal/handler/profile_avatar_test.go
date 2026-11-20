package handler

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/middleware"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
	"github.com/Ferrymania/nook/internal/service"
	"github.com/Ferrymania/nook/internal/testutil"
)

func TestProfileAvatarUploadReplaceAndRemove(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := testutil.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserAvatar{}); err != nil {
		t.Fatal(err)
	}
	repo := repository.New(db)
	log := zap.NewNop()
	dir := t.TempDir()
	cfg := &config.Config{
		Secrets: config.SecretsConfig{JWTSecret: "avatar-test"},
		App:     config.AppConfig{DataDir: dir},
		Cache:   config.CacheConfig{CacheDir: filepath.Join(dir, "cache")},
	}
	legacyPath := filepath.Join(dir, "original-avatar.png")
	if err := os.WriteFile(legacyPath, avatarTestPNG(t, 16, 16, color.NRGBA{G: 245, A: 255}), 0o600); err != nil {
		t.Fatal(err)
	}
	owner := &model.User{Username: "owner", Role: "user", PasswordHash: "test-hash"}
	other := &model.User{Username: "other", Role: "user", PasswordHash: "test-hash", AvatarURL: legacyPath}
	for _, user := range []*model.User{owner, other} {
		if err := repo.User.Create(t.Context(), user); err != nil {
			t.Fatal(err)
		}
	}
	auth := service.NewAuthService(cfg, log, repo, nil, nil)
	ownerToken, err := auth.IssueToken(owner)
	if err != nil {
		t.Fatal(err)
	}
	otherToken, err := auth.IssueToken(other)
	if err != nil {
		t.Fatal(err)
	}
	svc := &service.Container{
		Repo: repo, Profile: service.NewProfileService(log, repo), Emby: service.NewEmbyService(cfg, log, repo),
		ImageProxy: service.NewImageProxy(cfg, log),
	}
	router := gin.New()
	registerAuthedUserAndLicenseRoutes(router.Group("/api", middleware.AuthRequired(cfg.Secrets.JWTSecret), activeUserRequired(svc)), svc)
	registerEmbyRoutes(router, cfg.Secrets.JWTSecret, svc)
	const web = "/api/me/avatar"
	emby := "/emby/Users/" + owner.ID + "/Images/Primary"
	request := func(method, path, token string, body []byte, status int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/octet-stream")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != status {
			t.Fatalf("%s %s status = %d, want %d: %.200s", method, path, w.Code, status, w.Body.Bytes())
		}
		return w
	}
	first := avatarTestPNG(t, 800, 400, color.NRGBA{R: 200, A: 180})
	request("POST", web, "", first, http.StatusUnauthorized)
	request("POST", web, ownerToken, nil, http.StatusBadRequest)
	request("POST", web, ownerToken, []byte("<svg></svg>"), http.StatusBadRequest)
	request("POST", web, ownerToken, make([]byte, service.AvatarMaxBytes+1), http.StatusRequestEntityTooLarge)
	request("GET", web, ownerToken, nil, http.StatusNotFound)
	legacy := request("GET", web, otherToken, nil, http.StatusOK)
	if !strings.Contains(legacy.Header().Get("Cache-Control"), "private") {
		t.Fatalf("legacy web avatar must be private, got %s", legacy.Header().Get("Cache-Control"))
	}
	readUser := func(w *httptest.ResponseRecorder) model.User {
		t.Helper()
		var user model.User
		if err := json.Unmarshal(w.Body.Bytes(), &user); err != nil || user.ID != owner.ID {
			t.Fatalf("invalid owner response: %v %v", user.ID, err)
		}
		return user
	}
	updated := readUser(request("POST", web+"?userId="+other.ID, ownerToken, first, http.StatusOK))
	if !strings.HasPrefix(updated.AvatarURL, model.UploadedAvatarURLPrefix) {
		t.Fatalf("expected versioned avatar URL, got %q", updated.AvatarURL)
	}
	webImage := request("GET", updated.AvatarURL, ownerToken, nil, http.StatusOK)
	config, _, err := image.DecodeConfig(webImage.Body)
	if err != nil || config.Width != 512 || config.Height != 256 || webImage.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("expected a 512×256 PNG, got %+v (%v)", config, err)
	}
	embyImage := request("GET", emby, "", nil, http.StatusOK)
	if webImage.Header().Get("ETag") != embyImage.Header().Get("ETag") {
		t.Fatal("the website and Emby should serve the same avatar version")
	}
	if head := request("HEAD", emby, "", nil, http.StatusOK); head.Body.Len() != 0 || head.Header().Get("ETag") != embyImage.Header().Get("ETag") {
		t.Fatal("Emby HEAD should return the image headers with an empty body")
	}
	payload, err := svc.Emby.FindUser(t.Context(), owner.ID)
	if err != nil || payload["PrimaryImageTag"] != updated.AvatarImageTag() {
		t.Fatalf("unexpected Emby avatar tag: %v (%v)", payload["PrimaryImageTag"], err)
	}
	second := avatarTestPNG(t, 32, 32, color.NRGBA{B: 242, A: 255})
	replacement := readUser(request("POST", web, ownerToken, second, http.StatusOK))
	if replacement.AvatarURL == updated.AvatarURL || request("GET", emby, "", nil, http.StatusOK).Header().Get("ETag") == embyImage.Header().Get("ETag") {
		t.Fatal("replacing the image should change its URL and ETag")
	}
	if user := readUser(request("DELETE", web, ownerToken, nil, http.StatusOK)); user.AvatarURL != "" {
		t.Fatalf("removing the avatar left an address: %s", user.AvatarURL)
	}
	request("GET", web, ownerToken, nil, http.StatusNotFound)
	request("GET", emby, "", nil, http.StatusNotFound)
	request("GET", web, otherToken, nil, http.StatusOK)
}

func avatarTestPNG(t *testing.T, width, height int, fill color.Color) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), image.NewUniform(fill), image.Point{}, draw.Src)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
