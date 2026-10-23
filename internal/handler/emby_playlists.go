package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Ferrymania/nook/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func registerEmbyPlaylistRoutes(auth *gin.RouterGroup, svc *service.Container) {
	for _, prefix := range []string{"/Playlists", "/playlists"} {
		items, move, deleteSuffix, info := "/Items", "/Move", "/Delete", "/AddToPlaylistInfo"
		if prefix == "/playlists" {
			items, move, deleteSuffix, info = "/items", "/move", "/delete", "/addtoplaylistinfo"
		}
		// The lower-case root /playlists belongs to the web app. Emby clients
		// discover lists through Views/Items or the prefixed /emby/playlists.
		if prefix != "/playlists" || auth.BasePath() != "/" {
			auth.GET(prefix, func(c *gin.Context) {
				params := parseEmbyItemsParams(c)
				params.IncludeItemTypes = []string{"Playlist"}
				out, err := svc.Emby.Items(c.Request.Context(), params)
				if err != nil {
					embyPlaylistError(c, err)
					return
				}
				c.JSON(200, out)
			})
		}
		auth.POST(prefix, embyCreatePlaylistHandler(svc))
		auth.GET(prefix+"/:id"+items, embyPlaylistItemsHandler(svc))
		auth.POST(prefix+"/:id"+items, embyAddPlaylistItemsHandler(svc))
		auth.DELETE(prefix+"/:id"+items, embyRemovePlaylistItemsHandler(svc))
		auth.POST(prefix+"/:id"+items+deleteSuffix, embyRemovePlaylistItemsHandler(svc))
		auth.POST(prefix+"/:id"+items+"/:itemId"+move+"/:newIndex", embyMovePlaylistItemHandler(svc))
		auth.GET(prefix+"/:id"+info, embyAddToPlaylistInfoHandler(svc))
	}
	// Normalization maps lower-case Emby paths onto these canonical routes.
	auth.DELETE("/items/:id", embyDeletePlaylistHandler(svc))
	auth.POST("/items/:id", embyRenamePlaylistHandler(svc))
	auth.DELETE("/Items/:id", embyDeletePlaylistHandler(svc))
	auth.POST("/Items/:id", embyRenamePlaylistHandler(svc))
}
func embyPlaylistError(c *gin.Context, err error) {
	status := 500
	switch {
	case errors.Is(err, service.ErrPlaylistForbidden):
		status = 403
	case errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, service.ErrPlaylistMediaUnavailable):
		status = 404
	case errors.Is(err, service.ErrPlaylistInvalid):
		status = 400
	}
	embyError(c, status, err.Error())
}
func embyPlaylistNativeID(id string) string {
	if decoded, ok := service.DecodeEmbyPlaylistID(id); ok {
		return decoded
	}
	return id
}
func playlistQueryIDs(c *gin.Context, name string) []string {
	raw := firstQueryValue(c, name, strings.ToLower(name))
	if raw == "" {
		raw = c.PostForm(name)
	}
	if raw == "" {
		return nil
	}
	return strings.Split(raw, ",")
}
func embyCreatePlaylistHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := embyEffectiveUserID(c)
		ids, err := svc.Emby.PlaylistMediaIDs(c.Request.Context(), uid, playlistQueryIDs(c, "Ids"))
		if err != nil {
			embyPlaylistError(c, err)
			return
		}
		name := firstQueryValue(c, "Name", "name")
		if name == "" {
			name = c.PostForm("Name")
		}
		mediaType := firstQueryValue(c, "MediaType", "mediaType")
		if mediaType != "" && !strings.EqualFold(mediaType, "Video") {
			embyPlaylistError(c, service.ErrPlaylistInvalid)
			return
		}
		pl, err := svc.Playback.CreatePlaylistWithItems(c.Request.Context(), uid, name, false, ids)
		if err != nil {
			embyPlaylistError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"Id": service.EmbyPlaylistID(pl.ID), "Name": pl.Name, "ItemAddedCount": len(ids)})
	}
}
func embyPlaylistItemsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		out, err := svc.Emby.PlaylistItems(c.Request.Context(), c.Param("id"), parseEmbyItemsParams(c))
		if err != nil {
			embyPlaylistError(c, err)
			return
		}
		embyAttachRequestTokenToMediaSources(c, out)
		c.JSON(200, out)
	}
}
func embyAddPlaylistItemsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := embyEffectiveUserID(c)
		pid := embyPlaylistNativeID(c.Param("id"))
		if err := svc.Playback.EnsurePlaylistOwner(c.Request.Context(), pid, uid, playlistAdmin(c)); err != nil {
			embyPlaylistError(c, err)
			return
		}
		ids, err := svc.Emby.PlaylistMediaIDs(c.Request.Context(), uid, playlistQueryIDs(c, "Ids"))
		if err != nil {
			embyPlaylistError(c, err)
			return
		}
		added, err := svc.Playback.AddPlaylistItems(c.Request.Context(), pid, uid, ids, playlistAdmin(c))
		if err != nil {
			embyPlaylistError(c, err)
			return
		}
		c.JSON(200, gin.H{"Id": service.EmbyPlaylistID(pid), "ItemAddedCount": added})
	}
}
func embyRemovePlaylistItemsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		err := svc.Playback.RemovePlaylistEntries(c.Request.Context(), embyPlaylistNativeID(c.Param("id")), embyEffectiveUserID(c), playlistQueryIDs(c, "EntryIds"), playlistAdmin(c))
		if err != nil {
			embyPlaylistError(c, err)
			return
		}
		c.Status(200)
	}
}
func embyMovePlaylistItemHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		index, err := strconv.Atoi(c.Param("newIndex"))
		if err != nil {
			embyPlaylistError(c, service.ErrPlaylistInvalid)
			return
		}
		err = svc.Playback.MovePlaylistEntry(c.Request.Context(), embyPlaylistNativeID(c.Param("id")), embyEffectiveUserID(c), c.Param("itemId"), index, playlistAdmin(c))
		if err != nil {
			embyPlaylistError(c, err)
			return
		}
		c.Status(200)
	}
}
func embyAddToPlaylistInfoHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := embyEffectiveUserID(c)
		detail, err := svc.Playback.GetPlaylistForUser(c.Request.Context(), embyPlaylistNativeID(c.Param("id")), uid, playlistAdmin(c))
		if err != nil {
			embyPlaylistError(c, err)
			return
		}
		ids, err := svc.Emby.PlaylistMediaIDs(c.Request.Context(), uid, playlistQueryIDs(c, "Ids"))
		if err != nil {
			embyPlaylistError(c, err)
			return
		}
		existing := map[string]bool{}
		for _, entry := range detail.Entries {
			existing[entry.MediaID] = true
		}
		duplicate := false
		for _, id := range ids {
			if existing[id] {
				duplicate = true
			}
		}
		c.JSON(200, gin.H{"ItemCount": len(ids), "ContainsDuplicates": duplicate})
	}
}
func embyDeletePlaylistHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := service.DecodeEmbyPlaylistID(c.Param("id"))
		if !ok {
			embyError(c, 404, "playlist not found")
			return
		}
		if err := svc.Playback.DeletePlaylist(c.Request.Context(), id, embyEffectiveUserID(c), playlistAdmin(c)); err != nil {
			embyPlaylistError(c, err)
			return
		}
		c.Status(204)
	}
}
func embyRenamePlaylistHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := service.DecodeEmbyPlaylistID(c.Param("id"))
		if !ok {
			embyError(c, 404, "playlist not found")
			return
		}
		var req struct {
			Name string `json:"Name"`
		}
		if c.ShouldBindJSON(&req) != nil {
			embyPlaylistError(c, service.ErrPlaylistInvalid)
			return
		}
		if err := svc.Playback.RenamePlaylist(c.Request.Context(), id, embyEffectiveUserID(c), req.Name, playlistAdmin(c)); err != nil {
			embyPlaylistError(c, err)
			return
		}
		c.Status(204)
	}
}
