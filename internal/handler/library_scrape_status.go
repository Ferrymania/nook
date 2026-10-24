package handler

import (
	"net/http"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/service"
	"github.com/gin-gonic/gin"
)

func libraryScrapeStatusHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		lib, err := svc.Repo.Library.FindByID(ctx, c.Param("id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取媒体库失败"})
			return
		}
		if lib == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "library not found"})
			return
		}
		counts, err := svc.Repo.ScrapeTask.CountActiveForLibraries(ctx, []string{lib.ID})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取刮削状态失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"pending": counts[model.ScrapeTaskPending], "running": counts[model.ScrapeTaskRunning]})
	}
}
