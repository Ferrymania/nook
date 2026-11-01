package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Ferrymania/nook/internal/service"
)

func getDatabaseStatusHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil || svc.Database == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "数据库状态服务暂不可用"})
			return
		}
		status := svc.Database.GetStatus(c.Request.Context())
		c.JSON(http.StatusOK, status)
	}
}
