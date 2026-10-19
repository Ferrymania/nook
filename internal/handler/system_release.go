package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Ferrymania/nook/internal/service"
)

func systemReleaseCheckHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc.ReleaseCheck == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "版本检查暂不可用，请稍后重试"})
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, svc.ReleaseCheck.Check(c.Request.Context(), c.Query("refresh") == "1"))
	}
}
