package handler

import (
	"github.com/Ferrymania/nook/internal/service"
	"github.com/gin-gonic/gin"
)

func reorderPlaylistHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Order []string `json:"order"`
		}
		if c.ShouldBindJSON(&req) != nil {
			playlistError(c, service.ErrPlaylistInvalid)
			return
		}
		if err := svc.Playback.ReorderPlaylist(c.Request.Context(), c.Param("id"), currentUserID(c), req.Order, playlistAdmin(c)); err != nil {
			playlistError(c, err)
			return
		}
		c.Status(204)
	}
}
func deletePlaylistItemByIDHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.Playback.RemovePlaylistEntries(c.Request.Context(), c.Param("id"), currentUserID(c), []string{c.Param("item_id")}, playlistAdmin(c)); err != nil {
			playlistError(c, err)
			return
		}
		c.Status(204)
	}
}
