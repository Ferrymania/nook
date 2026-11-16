package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

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

func TestGlobalRemotePlaybackProxyAcrossEntrypoints(t *testing.T) {
	t.Parallel()
	db, err := testutil.Open(t, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	cfg, log := &config.Config{}, zap.NewNop()
	strm := service.NewStrmService(cfg, log, repos, service.NewCryptoService("proxy-test", log))
	stream := service.NewStreamService(cfg, log, repos, nil).
		SetStrmPlayTargetResolver(strm.ResolvePlayTargetWithUA).SetStrmMediaProxy(strm.ProxyMediaDirect)
	svc := &service.Container{Cfg: cfg, Repo: repos, Strm: strm, Stream: stream,
		Media: service.NewMediaService(cfg, log, repos), Emby: service.NewEmbyService(cfg, log, repos)}
	var videoRequests atomic.Int32
	var upstream *httptest.Server
	upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/fs/get":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"raw_url": upstream.URL + "/redirect"}})
		case "/redirect":
			http.Redirect(w, r, "/video", http.StatusFound)
		case "/video":
			videoRequests.Add(1)
			if r.Header.Get("Range") != "bytes=2-4" || r.Header.Get("If-Range") != `"etag"` || r.UserAgent() != "ProxyPlayer/1" {
				t.Errorf("lost playback headers: %v", r.Header)
			}
			if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("X-Emby-Token") != "" || r.URL.Query().Get("token") != "" {
				t.Errorf("Nook credentials leaked upstream: %v %s", r.Header, r.URL)
			}
			w.Header().Set("Content-Type", "video/mp4")
			w.Header().Set("Content-Length", "3")
			w.Header().Set("Content-Range", "bytes 2-4/10")
			w.Header().Set("Accept-Ranges", "bytes")
			w.WriteHeader(http.StatusPartialContent)
			if r.Method != http.MethodHead {
				_, _ = w.Write([]byte("234"))
			}
		case "/unresolved":
			w.WriteHeader(http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	acct, err := strm.CreateStrmAccount(t.Context(), "test", model.StrmProviderOpenList, map[string]string{"server": upstream.URL, "token": "account-token"})
	if err != nil {
		t.Fatal(err)
	}
	if err := repos.User.Create(t.Context(), &model.User{Base: model.Base{ID: "proxy-admin"}, Username: "proxy-admin", Role: "admin", PasswordHash: "test", IsActive: true}); err != nil {
		t.Fatal(err)
	}
	lib := model.Library{Name: "Movies", Path: t.TempDir(), Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	playPath := "/api/strm/play/openlist/video.mp4?acct=" + acct.ID + "&ref=" + url.QueryEscape("/movie.mp4")
	media := model.Media{Base: model.Base{ID: "proxy-media"}, LibraryID: lib.ID, Title: "Movie", Path: lib.Path + "/Movie.strm", STRMURL: playPath, Container: "strm", ScrapeStatus: "matched"}
	if err := repos.Media.Upsert(t.Context(), &media); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(middleware.CtxUserID, "proxy-admin"); c.Next() })
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		router.Handle(method, "/api/stream/:id", streamHandler(svc))
		router.Handle(method, "/api/strm/play/:provider/:file", strmPlayHandler(svc))
	}
	registerEmbyVideoStreamRoutes(router.Group(""), svc, "/Videos")
	paths := []string{"/api/stream/proxy-media", "/Videos/proxy-media/stream", "/Videos/proxy-media/stream.mp4", playPath}
	request := func(method, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("User-Agent", "ProxyPlayer/1")
		req.Header.Set("Range", "bytes=2-4")
		req.Header.Set("If-Range", `"etag"`)
		req.Header.Set("Authorization", "Bearer nook-token")
		req.Header.Set("Cookie", "nook=session")
		req.Header.Set("X-Emby-Token", "nook-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	// Unset defaults to 302; saving the setting affects every next request.
	for _, path := range paths {
		if rec := request(http.MethodGet, path); rec.Code != http.StatusFound {
			t.Fatalf("default %s: %d %s", path, rec.Code, rec.Body.String())
		}
	}
	if videoRequests.Load() != 0 {
		t.Fatal("default playback unexpectedly proxied video bytes")
	}
	if err := repos.Setting.Set(t.Context(), service.PlaybackProxyRemoteSettingKey, "true"); err != nil {
		t.Fatal(err)
	}
	// The old STRM preference must not add an intermediate 302 in proxy mode.
	if err := repos.Setting.Set(t.Context(), service.CloudPlaybackModeSettingKey, "strm"); err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			rec := request(method, path)
			if rec.Code != http.StatusPartialContent || rec.Header().Get("Location") != "" || rec.Header().Get("Content-Range") != "bytes 2-4/10" || rec.Header().Get("Content-Length") != "3" {
				t.Fatalf("proxy %s %s: %d %v %s", method, path, rec.Code, rec.Header(), rec.Body.String())
			}
			if method == http.MethodGet && rec.Body.String() != "234" {
				t.Fatal("proxy changed original bytes")
			}
			if method == http.MethodHead && rec.Body.Len() != 0 {
				t.Fatal("HEAD returned video bytes")
			}
		}
	}
	info, err := svc.Emby.PlaybackInfo(t.Context(), media.ID, "proxy-admin")
	if err != nil {
		t.Fatal(err)
	}
	sources := info["MediaSources"].([]map[string]any)
	if direct, _ := sources[0]["DirectStreamUrl"].(string); !strings.HasPrefix(direct, "/Videos/") {
		t.Fatalf("proxy playback URL = %q", direct)
	}
	if err := db.Model(&model.Media{}).Where("id = ?", media.ID).Update("strm_url", upstream.URL+"/unresolved").Error; err != nil {
		t.Fatal(err)
	}
	for _, path := range paths[:3] {
		rec := request(http.MethodGet, path)
		if rec.Code != http.StatusBadGateway || rec.Header().Get("Location") != "" {
			t.Fatalf("proxy failure fell back: %d %v", rec.Code, rec.Header())
		}
	}
	if err := repos.Setting.Set(t.Context(), service.PlaybackProxyRemoteSettingKey, "false"); err != nil {
		t.Fatal(err)
	}
	if rec := request(http.MethodGet, paths[0]); rec.Code != http.StatusFound {
		t.Fatalf("disabled proxy = %d", rec.Code)
	}
}
