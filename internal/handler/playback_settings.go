package handler

import (
	"net/http"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/service"
	"github.com/gin-gonic/gin"
)

func getPlaybackSettingsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		settings, err := svc.Profile.GetPlaybackSettings(c.Request.Context(), currentUserID(c))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, settings)
	}
}

func updatePlaybackSettingsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req model.PlaybackSettings
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		userID := currentUserID(c)
		settings, err := svc.Profile.UpdatePlaybackSettings(c.Request.Context(), userID, req)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, settings)
	}
}
