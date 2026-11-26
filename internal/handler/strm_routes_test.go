package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/Ferrymania/nook/internal/testutil"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/middleware"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
	"github.com/Ferrymania/nook/internal/service"
)

// TestStrmAdminRoutes 冒烟测试 STRM 管理端点注册与 401 拦截。
func TestStrmAdminRoutesAreRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	Register(router, &config.Config{
		Secrets: config.SecretsConfig{JWTSecret: "test-secret"},
	}, zap.NewNop(), &service.Container{Log: zap.NewNop()})

	routes := map[string]bool{}
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for _, want := range []string{
		"GET /api/admin/strm/accounts",
		"POST /api/admin/strm/accounts",
		"PUT /api/admin/strm/accounts/:id",
		"DELETE /api/admin/strm/accounts/:id",
		"POST /api/admin/strm/accounts/:id/test",
		"GET /api/admin/strm/accounts/:id/list",
		"GET /api/admin/strm/accounts/:id/files",
		"POST /api/admin/strm/accounts/:id/folders",
		"DELETE /api/admin/strm/accounts/:id/files",
		"DELETE /api/admin/strm/accounts/:id/files/:fileID",
		"GET /api/admin/strm/accounts/:id/request-stats",
		"POST /api/admin/strm/accounts/:id/oauth/start",
		"POST /api/admin/strm/accounts/:id/oauth/poll",
		"GET /api/admin/strm/settings",
		"PUT /api/admin/strm/settings",
		"GET /api/admin/strm/paths",
		"POST /api/admin/strm/paths",
		"PUT /api/admin/strm/paths/:id",
		"DELETE /api/admin/strm/paths/:id",
		"POST /api/admin/strm/paths/:id/sync",
		"POST /api/admin/strm/paths/:id/cancel",
		"GET /api/admin/strm/records",
		"POST /api/admin/strm/local-dirs",
		"GET /api/admin/strm/downloads",
		"POST /api/admin/strm/downloads/:id/cancel",
		"POST /api/admin/strm/downloads/:id/retry",
		"POST /api/admin/strm/downloads/clear-finished",
		"POST /api/admin/strm/downloads/clear-canceled",
		"POST /api/admin/strm/downloads/retry-failed",
		"POST /api/admin/strm/downloads/cancel-pending",
		"GET /api/admin/strm/uploads",
		"POST /api/admin/strm/uploads/:id/cancel",
		"POST /api/admin/strm/uploads/:id/retry",
		"POST /api/admin/strm/uploads/clear-done",
		"POST /api/admin/strm/uploads/clear-finished",
		"POST /api/admin/strm/uploads/clear-canceled",
		"POST /api/admin/strm/uploads/retry-failed",
		"POST /api/admin/strm/uploads/cancel-pending",
		"GET /api/strm/play/:provider/:file",
		"HEAD /api/strm/play/:provider/:file",
	} {
		if !routes[want] {
			t.Fatalf("%s route is not registered", want)
		}
	}
	for _, removed := range []string{"GET /api/strm/oauth/callback", "POST /api/strm/oauth/callback"} {
		if routes[removed] {
			t.Fatalf("removed authorization callback is still registered: %s", removed)
		}
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/admin/strm/accounts/account-a/request-stats", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated request stats request = %d, want 401", w.Code)
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/admin/strm/local-dirs", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated output folder request = %d, want 401", w.Code)
	}
	userToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, middleware.Claims{UserID: "viewer", Role: "user"}).SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatal(err)
	}
	const prefix = "/api/admin/strm/accounts/account-a"
	for _, endpoint := range []struct{ method, path string }{
		{http.MethodGet, "/files"},
		{http.MethodPost, "/folders"},
		{http.MethodDelete, "/files?parent_id=%2F&file_id=movie"},
	} {
		for _, tc := range []struct {
			token string
			want  int
		}{{"", http.StatusUnauthorized}, {userToken, http.StatusForbidden}} {
			req := httptest.NewRequest(endpoint.method, prefix+endpoint.path, nil)
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("%s %s = %d, want %d: %s", endpoint.method, endpoint.path, w.Code, tc.want, w.Body)
			}
		}
	}
}

func TestStrmOutputFolderCreation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	cfg.App.DataDir = t.TempDir()
	t.Setenv("NOOK_MEDIA_CONTAINER_DIR", cfg.App.DataDir)
	outside := t.TempDir()
	router := gin.New()
	router.POST("/local-dirs", createStrmOutputFolderHandler(&service.Container{
		FileManager: service.NewFileManagerService(cfg, zap.NewNop(), nil),
	}))

	for _, tc := range []struct {
		path, name string
		status     int
	}{
		{cfg.App.DataDir, "影视 STRM", http.StatusOK},
		{cfg.App.DataDir, "影视 STRM", http.StatusConflict},
		{outside, "影视 STRM", http.StatusForbidden},
		{cfg.App.DataDir, "父目录/子目录", http.StatusBadRequest},
	} {
		body, err := json.Marshal(fileFolderReq{Path: tc.path, Name: tc.name})
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/local-dirs", bytes.NewReader(body)))
		if w.Code != tc.status {
			t.Fatalf("creating %q in %q = %d, want %d: %s", tc.name, tc.path, w.Code, tc.status, w.Body)
		}
	}
	created := filepath.Join(cfg.App.DataDir, "影视 STRM")
	if info, err := os.Stat(created); err != nil || !info.IsDir() {
		t.Fatalf("expected a folder at %q: %v", created, err)
	}
}

func TestStrm115OAuthStartValidation(t *testing.T) {
	for _, tc := range []struct {
		body  string
		valid bool
	}{
		{`{"auth_source":"built_in_appid","app_id":"100195125"}`, true},
		{`{"auth_source":"custom_appid","app_id":"123456789"}`, true},
		{`{"auth_source":"custom_appid"}`, false},
		{`{"auth_source":"built_in_relay","provider":"qmediasync","app_id":"100195125"}`, false},
		{`{"auth_source":"third_party_service","provider":"moviepilot","app_id":"100195125"}`, false},
	} {
		t.Run(tc.body, func(t *testing.T) {
			var req strm115OAuthStartReq
			err := binding.JSON.BindBody([]byte(tc.body), &req)
			if (err == nil) != tc.valid {
				t.Fatalf("request accepted = %v, want %v: %v", err == nil, tc.valid, err)
			}
		})
	}
}

// TestStrmAccountsCRUD 用内存库走一遍账号/设置/同步目录接口。
func TestStrmAccountsCRUD(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := testutil.Open(t,
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(4)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&model.StrmAccount{}, &model.StrmSyncPath{}, &model.StrmSyncRecord{},
		&model.StrmDownloadTask{}, &model.StrmUploadTask{}, &model.Setting{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	svc := service.NewWithVersion(&config.Config{}, zap.NewNop(), repos, "test")
	router := gin.New()
	Register(router, &config.Config{Secrets: config.SecretsConfig{JWTSecret: "test-secret"}}, zap.NewNop(), svc)

	// 未登录访问应 401
	req := httptest.NewRequest(http.MethodGet, "/api/admin/strm/accounts", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET /admin/strm/accounts = %d, want 401", w.Code)
	}

	// 公开播放端点（本地提供方路径校验失败 → 404/400，不 panic）
	req = httptest.NewRequest(http.MethodGet, "/api/strm/play/local/video.mkv?path=%2Ftmp%2Fnope.mkv", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code == http.StatusInternalServerError {
		t.Fatalf("strm play endpoint errored: %d", w.Code)
	}
	videoPath := filepath.Join(t.TempDir(), "sample.mkv")
	if err := os.WriteFile(videoPath, []byte("fake-video-bytes"), 0o644); err != nil {
		t.Fatalf("write test video: %v", err)
	}
	if err := repos.StrmSyncPath.Create(t.Context(), &model.StrmSyncPath{
		Name:       "local-test",
		Provider:   model.StrmProviderLocal,
		RemotePath: filepath.Dir(videoPath),
		Enabled:    true,
	}); err != nil {
		t.Fatalf("create local sync path: %v", err)
	}

	// VidHub 等客户端会先用 HEAD 探测 STRM 播放源，不能返回 405/404。
	req = httptest.NewRequest(http.MethodHead, "/api/strm/play/local/video.mkv?path="+url.QueryEscape(videoPath), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("HEAD strm play endpoint = %d, want 200", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Fatalf("HEAD strm play endpoint returned body: %q", w.Body.String())
	}
}
