package handler

import (
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/service"
	"github.com/gin-gonic/gin"
	"net/http"
)

func defaultLibraryScrapeSettingsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		view, err := svc.Scraper.DefaultLibraryScrapeSettings(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, view)
	}
}

func libraryScrapeSettingsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		view, err := svc.Scraper.GetLibraryScrapeSettings(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, view)
	}
}
func saveLibraryScrapeSettingsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Settings      model.LibraryScrapeSettings `json:"settings"`
			EpisodeImages *bool                       `json:"episode_images"`
			AutoProbe     bool                        `json:"auto_probe"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		view, err := svc.Scraper.SaveLibraryScrapeSettings(c.Request.Context(), c.Param("id"), req.Settings, req.AutoProbe, req.EpisodeImages)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, view)
	}
}
