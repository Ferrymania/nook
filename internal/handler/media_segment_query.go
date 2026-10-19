package handler

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/service"
)

func queryMediaSegmentsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if svc == nil || svc.Repo == nil || svc.Segments == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": service.ErrSegmentQueryUnavailable.Error()})
			return
		}
		// This operation only applies to a concrete local media record. In
		// particular, do not contact a remote Emby mount or probe a video file.
		m, err := svc.Repo.Media.FindByID(c.Request.Context(), c.Param("id"))
		if err != nil || m == nil || !mediaVisibleForRequest(c, svc, m) {
			c.JSON(http.StatusNotFound, gin.H{"error": "媒体不存在或不可访问，请进入具体影片或单集后查询"})
			return
		}
		result, err := svc.Segments.QueryForMedia(c.Request.Context(), m)
		if err != nil {
			status, message := http.StatusBadGateway, "片头片尾查询失败，请稍后重试；已有时间点保留"
			var upstream *service.IntroDBHTTPError
			var timeout net.Error
			switch {
			case errors.Is(err, service.ErrSegmentMetadataMissing):
				status, message = http.StatusBadRequest, err.Error()
			case errors.Is(err, service.ErrSegmentQueryUnavailable):
				status, message = http.StatusServiceUnavailable, err.Error()
			case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeout) && timeout.Timeout()):
				message = "TheIntroDB 查询超时，请稍后重试；已有时间点保留"
			case errors.As(err, &upstream) && upstream.StatusCode == http.StatusTooManyRequests:
				status, message = http.StatusServiceUnavailable, "TheIntroDB 暂时限制了查询请求，请稍后重试；已有时间点保留"
			}
			if svc.Log != nil {
				svc.Log.Warn("manual introdb lookup failed", zap.String("media_id", m.ID), zap.Error(err))
			}
			c.JSON(status, gin.H{"error": message})
			return
		}
		c.JSON(http.StatusOK, result)
	}
}
