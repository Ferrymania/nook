package handler

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Ferrymania/nook/internal/middleware"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/service"
)

func mediaVisibilityForRequest(c *gin.Context, svc *service.Container) service.MediaVisibility {
	return service.UserDefaultMediaVisibility(c.Request.Context(), svc.Repo, currentUserID(c))
}

func mediaVisibleForRequest(c *gin.Context, svc *service.Container, media *model.Media) bool {
	return mediaVisibilityForRequest(c, svc).Allows(media)
}

func settingBool(c *gin.Context, svc *service.Container, key string, fallback bool) bool {
	if svc == nil || svc.Repo == nil || svc.Repo.Setting == nil {
		return fallback
	}
	value, err := svc.Repo.Setting.Get(c.Request.Context(), key)
	if err != nil {
		return fallback
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on", "enabled", "启用", "开启":
		return true
	case "0", "false", "no", "off", "disabled", "禁用", "关闭", "":
		return false
	default:
		return fallback
	}
}

func currentUserID(c *gin.Context) string {
	uid, _ := c.Get(middleware.CtxUserID)
	return toString(uid)
}
