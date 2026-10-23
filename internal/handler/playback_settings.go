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
		var req struct {
			model.PlaybackSettings
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		userID := currentUserID(c)
		current, err := svc.Profile.GetPlaybackSettings(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if current.AllowAdult != req.AllowAdult {
			if err := svc.Auth.VerifyPassword(c.Request.Context(), userID, req.Password); err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "需要输入当前账号密码确认"})
				return
			}
		}
		settings, err := svc.Profile.UpdatePlaybackSettings(c.Request.Context(), userID, req.PlaybackSettings)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, settings)
	}
}
