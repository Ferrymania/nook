package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Ferrymania/nook/internal/service"
)

// testTelegramHandler 向管理员会话发送一条测试消息，用于验证 Token/会话 ID。
func testTelegramHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc.Telegram == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "telegram service unavailable"})
			return
		}
		if !svc.Telegram.Configured(c.Request.Context()) {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "请先启用 Telegram 通知并填写 Bot Token 与管理员 Chat ID",
			})
			return
		}
		if err := svc.Telegram.SendToAdminChecked(c.Request.Context(), "✅ Nook 测试消息：通知通道工作正常。"); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}
