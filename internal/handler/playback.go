// Package handler — playback history / favourites / playlists endpoints.
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Ferrymania/nook/internal/middleware"
	"github.com/Ferrymania/nook/internal/service"
)

// ─── History ────────────────────────────────────────────────────────────────

type progressReq struct {
	MediaID            string `json:"media_id" binding:"required"`
	PositionMs         int64  `json:"position_ms"`
	DurationMs         int64  `json:"duration_ms"`
	SessionID          string `json:"session_id"`
	SessionStartedAtMs int64  `json:"session_started_at_ms"`
	Sequence           int64  `json:"sequence"`
	IsPaused           *bool  `json:"is_paused"`
	Stopped            *bool  `json:"stopped"`
}

func recordProgressHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req progressReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		uid, _ := c.Get(middleware.CtxUserID)
		if err := svc.Playback.RecordProgressUpdate(c.Request.Context(), service.ProgressUpdate{
			UserID:             toString(uid),
			MediaID:            req.MediaID,
			PositionMs:         req.PositionMs,
			DurationMs:         req.DurationMs,
			SessionID:          req.SessionID,
			SessionStartedAtMs: req.SessionStartedAtMs,
			Sequence:           req.Sequence,
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if svc.Sessions != nil && req.SessionID != "" && req.IsPaused != nil {
			svc.Sessions.RecordPlaybackState(c.Request.Context(), toString(uid), "",
				"web:"+req.SessionID, "", "Nook Web", c.ClientIP(), service.PlaybackState{
					ItemID: req.MediaID, PositionTicks: req.PositionMs * 10000, RuntimeTicks: req.DurationMs * 10000,
					IsPaused: *req.IsPaused, Stopped: req.Stopped != nil && *req.Stopped, Sequence: req.Sequence,
				})
		}
		c.Status(http.StatusNoContent)
	}
}

func recentHistoryHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, _ := c.Get(middleware.CtxUserID)
		items, err := svc.Playback.RecentHistory(c.Request.Context(), uid.(string), 30)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		visibility := mediaVisibilityForRequest(c, svc)
		filtered := make([]service.HistoryItem, 0, len(items))
		for _, item := range items {
			if item.Media == nil || visibility.Allows(item.Media) {
				filtered = append(filtered, item)
			}
		}
		c.JSON(http.StatusOK, gin.H{"items": filtered})
	}
}

// ─── Favourites ─────────────────────────────────────────────────────────────

func toggleFavouriteHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, _ := c.Get(middleware.CtxUserID)
		state, err := svc.Playback.ToggleFavourite(
			c.Request.Context(), uid.(string), c.Param("id"),
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"favourite": state})
	}
}

func listFavouritesHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, _ := c.Get(middleware.CtxUserID)
		if c.Query("ids") == "1" {
			ids, err := svc.Playback.ListFavouriteIDs(c.Request.Context(), uid.(string))
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			if ids == nil {
				ids = []string{}
			}
			c.JSON(http.StatusOK, gin.H{"ids": ids})
			return
		}
		items, err := svc.Playback.ListFavourites(c.Request.Context(), uid.(string))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		visibility := mediaVisibilityForRequest(c, svc)
		filtered := make([]any, 0, len(items))
		for i := range items {
			if visibility.Allows(&items[i]) {
				filtered = append(filtered, items[i])
			}
		}
		c.JSON(http.StatusOK, gin.H{"items": filtered})
	}
}
