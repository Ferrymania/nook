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

func TestPlaylistWebAndEmbyShareDataAndEnforcePermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := testutil.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatal(err)
	}
	repo := repository.New(db)
	cfg := &config.Config{}
	cfg.Secrets.JWTSecret = "playlist-test-secret"
	svc := &service.Container{Repo: repo, Log: zap.NewNop()}
	svc.Playback = service.NewPlaybackService(svc.Log, repo)
	svc.Emby = service.NewEmbyService(cfg, svc.Log, repo)
	auth := service.NewAuthService(cfg, svc.Log, repo, nil, nil)
	tokens := map[string]string{}
	for _, uid := range []string{"one", "two"} {
		u := &model.User{Base: model.Base{ID: uid}, Username: uid, PasswordHash: "unused", Role: "user", IsActive: true}
		if err = repo.User.Create(t.Context(), u); err != nil {
			t.Fatal(err)
		}
		tokens[uid], err = auth.IssueToken(u)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, lib := range []string{"visible", "hidden"} {
		if err = repo.Library.Create(t.Context(), &model.Library{Base: model.Base{ID: lib}, Name: lib, Type: "movie", Path: "/" + lib, Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"a", "b", "secret"} {
		lib := "visible"
		if id == "secret" {
			lib = "hidden"
		}
		if err = db.Create(&model.Media{Base: model.Base{ID: id}, LibraryID: lib, Title: id, Path: "/" + id + ".mp4"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err = repo.User.UpdateFields(t.Context(), "one", map[string]any{"allowed_library_ids": `["visible"]`}); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	api := router.Group("/api", middleware.AuthRequired(cfg.Secrets.JWTSecret))
	api.GET("/playlists", listPlaylistsHandler(svc))
	api.POST("/playlists", createPlaylistHandler(svc))
	api.GET("/playlists/:id", getPlaylistHandler(svc))
	api.PUT("/playlists/:id", renamePlaylistHandler(svc))
	api.POST("/playlists/:id/items", addPlaylistItemHandler(svc))
	api.POST("/playlists/:id/reorder", reorderPlaylistHandler(svc))
	registerEmbyRoutes(router, cfg.Secrets.JWTSecret, svc)
	// Direct browser navigation must reach the SPA rather than Emby auth.
	router.GET("/playlists", func(c *gin.Context) { c.String(200, "playlist SPA") })
	request := func(method, path, body, user string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if user != "" {
			req.Header.Set("Authorization", "Bearer "+tokens[user])
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s %s as %s: got %d want %d: %s", method, path, user, w.Code, want, w.Body.String())
		}
		return w
	}
	request("GET", "/playlists", "", "", 200)
	request("GET", "/emby/playlists", "", "", 401)
	request("POST", "/api/playlists", `{"name":"Invalid","media_ids":["a","secret"]}`, "one", 404)
	request("POST", "/api/playlists", `{"name":"  "}`, "one", 400)
	w := request("POST", "/api/playlists", `{"name":"周末","media_ids":["a","b"]}`, "one", 201)
	var native model.Playlist
	if err = json.Unmarshal(w.Body.Bytes(), &native); err != nil {
		t.Fatal(err)
	}
	pid := service.EmbyPlaylistID(native.ID)
	w = request("GET", "/emby/Users/two/Items?IncludeItemTypes=Playlist&Recursive=true", "", "one", 200)
	if !strings.Contains(w.Body.String(), pid) {
		t.Fatal("spoofed path user prevented own playlist query")
	}
	w = request("GET", "/emby/Users/one/Items?IncludeItemTypes=Playlist&Recursive=true", "", "two", 200)
	if strings.Contains(w.Body.String(), pid) {
		t.Fatal("private playlist leaked to another account")
	}
	request("GET", "/api/playlists/"+native.ID, "", "two", 403)
	request("GET", "/emby/Playlists/"+pid+"/Items", "", "two", 403)
	request("GET", "/emby/Items?ParentId="+pid, "", "two", 403)
	request("POST", "/emby/Playlists/"+pid+"/Items?Ids=a&UserId=one", "", "two", 403)
	request("GET", "/emby/Playlists/"+pid+"/Items", "", "", 401)
	w = request("GET", "/emby/Users/one/Views", "", "one", 200)
	if !strings.Contains(w.Body.String(), `"CollectionType":"playlists"`) {
		t.Fatal(w.Body.String())
	}
	w = request("GET", "/emby/Playlists/"+pid+"/Items?Limit=1&StartIndex=1", "", "one", 200)
	var page struct {
		Items []struct {
			ID      string `json:"Id"`
			EntryID string `json:"PlaylistItemId"`
		}
		Total int `json:"TotalRecordCount"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Items) != 1 || page.Items[0].ID != "b" || page.Items[0].EntryID == "" {
		t.Fatalf("wrong page %s", w.Body.String())
	}
	entry := page.Items[0].EntryID
	request("POST", "/Emby/PLAYLISTS/"+pid+"/ITEMS/"+entry+"/MOVE/0", "", "one", 200)
	w = request("GET", "/api/playlists/"+native.ID, "", "one", 200)
	var detail service.PlaylistDetail
	json.Unmarshal(w.Body.Bytes(), &detail)
	if detail.Items[0].ID != "b" {
		t.Fatal("Emby move not reflected in web")
	}
	request("POST", "/emby/Playlists/"+pid+"/Items?Ids=a", "", "one", 200)
	w = request("GET", "/emby/Playlists/"+pid+"/AddToPlaylistInfo?Ids=a", "", "one", 200)
	if !strings.Contains(w.Body.String(), `"ContainsDuplicates":true`) {
		t.Fatal(w.Body.String())
	}
	request("POST", "/emby/Playlists/"+pid+"/Items?Ids=secret", "", "one", 404)
	request("POST", "/api/playlists/"+native.ID+"/reorder", `{"order":["missing"]}`, "one", 400)
	request("PUT", "/api/playlists/"+native.ID, `{"name":"重命名"}`, "one", 204)
	w = request("GET", "/emby/Items/"+pid, "", "one", 200)
	if !strings.Contains(w.Body.String(), "重命名") {
		t.Fatal(w.Body.String())
	}
	request("DELETE", "/emby/Playlists/"+pid+"/Items?EntryIds="+entry, "", "one", 200)
	w = request("GET", "/api/playlists/"+native.ID, "", "one", 200)
	json.Unmarshal(w.Body.Bytes(), &detail)
	if len(detail.Items) != 1 || detail.Items[0].ID != "a" {
		t.Fatal(w.Body.String())
	}
	request("DELETE", "/emby/Items/"+pid, "", "two", 403)
	request("DELETE", "/emby/Items/"+pid, "", "one", 204)
	request("GET", "/api/playlists/"+native.ID, "", "one", 404)
	w = request("POST", "/emby/Playlists?Name=Emby-created&Ids=a,b&UserId=two", "", "one", 200)
	if !strings.Contains(w.Body.String(), `"ItemAddedCount":2`) {
		t.Fatal(w.Body.String())
	}
	w = request("GET", "/api/playlists?media_id=a", "", "one", 200)
	if !strings.Contains(w.Body.String(), "Emby-created") || !strings.Contains(w.Body.String(), `"contains_media":true`) {
		t.Fatal(w.Body.String())
	}
	var count int64
	db.Model(&model.Playlist{}).Where("user_id = ?", "two").Count(&count)
	if count != 0 {
		t.Fatal("query UserId overrode authenticated identity")
	}
	// Denied media and missing entries must not be returned after ACL changes.
	repo.User.UpdateFields(t.Context(), "one", map[string]any{"allowed_library_ids": `["hidden"]`})
	w = request("GET", "/api/playlists", "", "one", 200)
	if !strings.Contains(w.Body.String(), `"item_count":0`) {
		t.Fatal(w.Body.String())
	}
}
