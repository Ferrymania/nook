package handler

import (
	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/middleware"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestContentRatingBlockedMediaRejectsWebAndEmbyPlaybackEndpoints(t *testing.T) {
	router, svc := newDeviceTestEnv(t)
	seedDeviceUsers(t, svc)
	if err := svc.Repo.DB.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatal(err)
	}
	svc.Media = service.NewMediaService(&config.Config{}, zap.NewNop(), svc.Repo)
	svc.Emby = service.NewEmbyService(&config.Config{}, zap.NewNop(), svc.Repo)
	if err := svc.Repo.User.UpdateFields(t.Context(), "user-a", map[string]any{"playback_settings": `{"content_rating_limit":"PG-13"}`}); err != nil {
		t.Fatal(err)
	}
	m := model.Media{Base: model.Base{ID: "restricted-film"}, LibraryID: "movies", Title: "Restricted film", Path: "/media/restricted.mkv", ContentRating: model.NewContentRating("R", "US", "manual")}
	if err := svc.Repo.DB.Create(&m).Error; err != nil {
		t.Fatal(err)
	}
	endpoints := []struct {
		path    string
		handler gin.HandlerFunc
	}{
		{"/test/detail/:id", getMediaHandler(svc)},
		{"/test/stream/:id", streamHandler(svc)},
		{"/test/hls/:id", hlsPlaylistHandler(svc)},
		{"/test/external/:id", externalPlayersHandler(svc)},
		{"/test/emby/:id", embyVideoStreamHandler(svc, "")},
	}
	for _, ep := range endpoints {
		router.GET(ep.path, func(c *gin.Context) { c.Set(middleware.CtxUserID, "user-a"); ep.handler(c) })
	}
	for _, path := range []string{"/test/detail/", "/test/stream/", "/test/hls/", "/test/external/", "/test/emby/"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path+m.ID, nil)
		router.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("%s status=%d body=%s", path, w.Code, w.Body.String())
		}
	}
}
