package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/service"
)

func registerAdminSegmentRoutes(admin *gin.RouterGroup, svc *service.Container) {
	admin.GET("/segments/queue", listSegmentQueueHandler(svc))
	admin.POST("/segments/queue/:id/retry", retrySegmentTaskHandler(svc))
	admin.DELETE("/segments/queue/:id", deleteSegmentTaskHandler(svc))
	admin.POST("/segments/queue/clear-finished", clearSegmentTasksHandler(svc))
}

func listSegmentQueueHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		status := c.DefaultQuery("status", "all")
		switch status {
		case "all", model.SegmentTaskRunning, model.SegmentTaskFound, model.SegmentTaskMissing, model.SegmentTaskFailed:
		default:
			c.JSON(400, gin.H{"error": "未知的任务状态"})
			return
		}
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		size, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
		if page < 1 || page > 1000000 {
			c.JSON(400, gin.H{"error": "页码无效"})
			return
		}
		search := strings.TrimSpace(c.Query("q"))
		if len([]rune(search)) > 200 {
			c.JSON(400, gin.H{"error": "搜索内容不能超过 200 字"})
			return
		}
		if err := svc.Repo.SegmentTask.InterruptStale(c.Request.Context(), time.Now().Add(-2*time.Minute)); err != nil {
			c.JSON(500, gin.H{"error": "无法恢复中断任务，请重试"})
			return
		}
		result, err := svc.Repo.SegmentTask.List(c.Request.Context(), status, search, page, size)
		if err != nil {
			c.JSON(500, gin.H{"error": "无法读取片头片尾队列，请重试"})
			return
		}
		c.JSON(200, result)
	}
}

func retrySegmentTaskHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		task, err := svc.Repo.SegmentTask.FindByID(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(500, gin.H{"error": "无法读取任务，请重试"})
			return
		}
		if task == nil {
			c.JSON(404, gin.H{"error": "查询记录不存在"})
			return
		}
		if task.Status == model.SegmentTaskRunning {
			c.JSON(409, gin.H{"error": "任务仍在查询中，请稍候"})
			return
		}
		m, err := svc.Repo.Media.FindByID(c.Request.Context(), task.MediaID)
		if err != nil || m == nil || !mediaVisibleForRequest(c, svc, m) {
			c.JSON(404, gin.H{"error": "媒体已移除或不可访问"})
			return
		}
		if svc.Segments == nil {
			c.JSON(503, gin.H{"error": service.ErrSegmentQueryUnavailable.Error()})
			return
		}
		result, err := svc.Segments.RetryForMedia(c.Request.Context(), m)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": service.SegmentQueryErrorMessage(err)})
			return
		}
		c.JSON(200, result)
	}
}

func deleteSegmentTaskHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		n, err := svc.Repo.SegmentTask.DeleteFinished(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(500, gin.H{"error": "无法删除查询记录，请重试"})
			return
		}
		if n == 0 {
			c.JSON(409, gin.H{"error": "记录不存在或仍在查询中"})
			return
		}
		c.JSON(200, gin.H{"deleted": n})
	}
}

func clearSegmentTasksHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		n, err := svc.Repo.SegmentTask.DeleteFinished(c.Request.Context(), "")
		if err != nil {
			c.JSON(500, gin.H{"error": "无法清理查询记录，请重试"})
			return
		}
		c.JSON(200, gin.H{"deleted": n})
	}
}
