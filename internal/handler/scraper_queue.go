package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Ferrymania/nook/internal/service"
)

func listScrapeQueueHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
		snap, err := svc.Scraper.ScrapeQueueSnapshot(c.Request.Context(), c.Query("status"), page, pageSize)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, snap)
	}
}

func cancelScrapeTaskHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.Scraper.CancelScrapeTask(c.Request.Context(), c.Param("id")); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

func retryScrapeTaskHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.Scraper.RetryScrapeTask(c.Request.Context(), c.Param("id")); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

func deleteScrapeTaskHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.Scraper.DeleteScrapeTask(c.Request.Context(), c.Param("id")); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

func batchActionScrapeTasksHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req queueBatchReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		n, err := svc.Scraper.BatchActionScrapeTasks(c.Request.Context(), req.Action, req.IDs)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"affected": n, "action": req.Action})
	}
}

func clearDoneScrapeTasksHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		n, err := svc.Scraper.ClearDoneScrapeTasks(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"deleted": n})
	}
}

func clearFinishedScrapeTasksHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		n, err := svc.Scraper.ClearFinishedScrapeTasks(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"deleted": n})
	}
}

func clearCanceledScrapeTasksHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		n, err := svc.Scraper.ClearCanceledScrapeTasks(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"deleted": n})
	}
}

func clearFailedScrapeTasksHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		n, err := svc.Scraper.ClearFailedScrapeTasks(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"deleted": n})
	}
}

func retryAllFailedScrapeTasksHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		n, err := svc.Scraper.RetryAllFailedScrapeTasks(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"retried": n})
	}
}

func cancelPendingScrapeTasksHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		n, err := svc.Scraper.CancelPendingScrapeTasks(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"canceled": n})
	}
}

func enqueueLibraryScrapeHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		options, err := scrapeOptionsFromRequest(c, true)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scrape options"})
			return
		}
		libID := c.Param("id")
		n, err := svc.Scraper.EnqueueLibrary(c.Request.Context(), libID, options)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"enqueued": n})
	}
}

func enqueueAllScrapeHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		options, err := scrapeOptionsFromRequest(c, true)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scrape options"})
			return
		}
		n, err := svc.Scraper.EnqueueAll(c.Request.Context(), options)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"enqueued": n})
	}
}
