package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestLibraryPathChangeHTTPRequiresPreviewAndAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := testutil.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	media := service.NewMediaService(&config.Config{}, zap.NewNop(), repos)
	oldDir, newDir := t.TempDir(), t.TempDir()
	disabled := false
	lib, err := media.CreateLibraryWithRoots(t.Context(), "测试", "movie", []service.LibraryRootInput{{Path: oldDir, Enabled: &disabled}})
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{oldDir, newDir} {
		if err := os.WriteFile(filepath.Join(dir, "movie.mkv"), []byte("same"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := model.Media{LibraryID: lib.ID, LibraryRootID: lib.Roots[0].ID, Path: filepath.Join(oldDir, "movie.mkv"), SizeBytes: 4, Title: "Movie"}
	if err := db.Create(&m).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	group := router.Group("/api")
	group.Use(func(c *gin.Context) { c.Set(middleware.CtxUserRole, c.GetHeader("X-Test-Role")); c.Next() })
	registerAuthedLibraryRoutes(group, &service.Container{Repo: repos, Media: media})
	endpoint := "/api/libraries/" + lib.ID + "/roots/" + lib.Roots[0].ID
	call := func(method, suffix, role string, input any) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, endpoint+suffix, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-Role", role)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	in := service.LibraryRootInput{Path: newDir, ChangeMode: service.LibraryPathMigrate}
	if w := call(http.MethodPost, "/preview-path", "user", in); w.Code != http.StatusForbidden {
		t.Fatalf("non-admin preview code=%d", w.Code)
	}
	if w := call(http.MethodPatch, "", "admin", in); w.Code != http.StatusConflict {
		t.Fatalf("unconfirmed update code=%d body=%s", w.Code, w.Body)
	}
	w := call(http.MethodPost, "/preview-path", "admin", in)
	var preview service.LibraryPathPreview
	if err := json.Unmarshal(w.Body.Bytes(), &preview); w.Code != http.StatusOK || err != nil || !preview.CanApply {
		t.Fatalf("preview failed: code=%d body=%s err=%v", w.Code, w.Body, err)
	}
	root, _ := repos.Library.FindRootByID(t.Context(), lib.ID, lib.Roots[0].ID)
	if root.Path != oldDir {
		t.Fatal("preview modified root")
	}
	in.PreviewToken = preview.PreviewToken
	if w := call(http.MethodPatch, "", "admin", in); w.Code != http.StatusOK {
		t.Fatalf("confirmed update code=%d body=%s", w.Code, w.Body)
	}
	root, _ = repos.Library.FindRootByID(t.Context(), lib.ID, lib.Roots[0].ID)
	if root.Path != newDir || root.Enabled {
		t.Fatalf("saved root=%+v", root)
	}
}
