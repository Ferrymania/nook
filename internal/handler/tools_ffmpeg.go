// Package handler — FFmpeg/FFprobe 工具安装端点。
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Ferrymania/nook/internal/service"
)

// ffToolsStatusHandler 返回 ffmpeg/ffprobe 当前安装状态
// （GET /api/admin/tools/ffmpeg/status）。
func ffToolsStatusHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil || svc.FFTools == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "FFmpeg 工具服务不可用"})
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, svc.FFTools.Status(c.Request.Context()))
	}
}

// ffToolsDetectHandler 只返回候选路径，不改变当前配置。
func ffToolsDetectHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil || svc.FFTools == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "FFmpeg 工具服务不可用"})
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, svc.FFTools.Detect(c.Request.Context()))
	}
}

// ffToolsInstallHandler 只准备工具，保存设置后才切换使用。
func ffToolsInstallHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil || svc.FFTools == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "FFmpeg 工具服务不可用"})
			return
		}
		if err := svc.FFTools.StartInstall(c.Request.Context()); err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, svc.FFTools.Status(c.Request.Context()))
	}
}
