package handler

import (
	"errors"
	"net/http"

	"github.com/Ferrymania/nook/internal/middleware"
	"github.com/Ferrymania/nook/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type createPlaylistReq struct {
	Name     string   `json:"name" binding:"required"`
	IsPublic bool     `json:"is_public"`
	MediaIDs []string `json:"media_ids"`
}

func playlistError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, service.ErrPlaylistForbidden):
		status = http.StatusForbidden
	case errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, service.ErrPlaylistMediaUnavailable):
		status = http.StatusNotFound
	case errors.Is(err, service.ErrPlaylistInvalid):
		status = http.StatusBadRequest
	}
	c.JSON(status, gin.H{"error": err.Error()})
}
func playlistAdmin(c *gin.Context) bool { return c.GetString(middleware.CtxUserRole) == "admin" }
func createPlaylistHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req createPlaylistReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "请输入播放列表名称"})
			return
		}
		pl, err := svc.Playback.CreatePlaylistWithItems(c.Request.Context(), currentUserID(c), req.Name, req.IsPublic, req.MediaIDs)
		if err != nil {
			playlistError(c, err)
			return
		}
		c.JSON(http.StatusCreated, pl)
	}
}
func listPlaylistsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := svc.Playback.PlaylistSummaries(c.Request.Context(), currentUserID(c), c.Query("media_id"))
		if err != nil {
			playlistError(c, err)
			return
		}
		c.JSON(200, gin.H{"items": items})
	}
}
func getPlaylistHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		detail, err := svc.Playback.GetPlaylistForUser(c.Request.Context(), c.Param("id"), currentUserID(c), playlistAdmin(c))
		if err != nil {
			playlistError(c, err)
			return
		}
		c.JSON(200, detail)
	}
}
func renamePlaylistHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Name string `json:"name" binding:"required"`
		}
		if c.ShouldBindJSON(&req) != nil {
			playlistError(c, service.ErrPlaylistInvalid)
			return
		}
		if err := svc.Playback.RenamePlaylist(c.Request.Context(), c.Param("id"), currentUserID(c), req.Name, playlistAdmin(c)); err != nil {
			playlistError(c, err)
			return
		}
		c.Status(204)
	}
}
func addPlaylistItemHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			MediaID  string   `json:"media_id"`
			MediaIDs []string `json:"media_ids"`
		}
		if c.ShouldBindJSON(&req) != nil {
			playlistError(c, service.ErrPlaylistInvalid)
			return
		}
		ids := req.MediaIDs
		if req.MediaID != "" {
			ids = append(ids, req.MediaID)
		}
		added, err := svc.Playback.AddPlaylistItems(c.Request.Context(), c.Param("id"), currentUserID(c), ids, playlistAdmin(c))
		if err != nil {
			playlistError(c, err)
			return
		}
		c.JSON(200, gin.H{"added": added})
	}
}
func removePlaylistItemHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.Playback.RemoveFromPlaylist(c.Request.Context(), c.Param("id"), currentUserID(c), c.Param("media_id"), playlistAdmin(c)); err != nil {
			playlistError(c, err)
			return
		}
		c.Status(204)
	}
}
func deletePlaylistHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.Playback.DeletePlaylist(c.Request.Context(), c.Param("id"), currentUserID(c), playlistAdmin(c)); err != nil {
			playlistError(c, err)
			return
		}
		c.Status(204)
	}
}
