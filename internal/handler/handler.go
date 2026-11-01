// Package handler wires the HTTP routes to the service container.
//
// All routes are mounted under /api/* so the frontend dev-server can proxy a
// single prefix.
package handler

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/middleware"
	"github.com/Ferrymania/nook/internal/service"
)

// Register attaches every API route to the engine.
func Register(r *gin.Engine, cfg *config.Config, log *zap.Logger, svc *service.Container) {
	api := r.Group("/api")
	api.Use(middleware.GzipAPI())
	{
		api.GET("/health", healthCheck)
		api.HEAD("/health", healthCheck)
		api.GET("/version", versionInfo)

		// STRM 播放端点：strm 文件内容指向这里，Emby/Infuse 直接请求（无 JWT）。
		api.GET("/strm/play/:provider/:file", strmPlayHandler(svc))
		api.HEAD("/strm/play/:provider/:file", strmPlayHandler(svc))

		registerPublicAuthRoutes(api, svc, log)

		registerAuthenticatedRoutes(api, cfg, svc)

		registerAdminRoutes(api, cfg, svc)

		// Emby/Jellyfin compatibility shim — routes mounted at /emby/* AND
		// the root path so Infuse / Yamby / Hills / Senplayer 都能自动连接。
	}

	registerEmbyRoutes(r, cfg.Secrets.JWTSecret, svc)
}

func healthCheck(c *gin.Context) {
	c.JSON(200, gin.H{"status": "ok"})
}

func versionInfo(c *gin.Context) {
	c.JSON(200, gin.H{"name": "Nook", "version": "0.1.0"})
}

// ─── 权限 Handler 包装 ────────────────────────────────────────────────────────

func getMyPermissionsHandler(svc *service.Container) gin.HandlerFunc {
	h := NewPermissionHandler(svc, svc.Log)
	return h.GetMyPermissions
}
