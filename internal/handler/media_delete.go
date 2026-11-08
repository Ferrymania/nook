// Package handler — media delete endpoint.
package handler

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Ferrymania/nook/internal/service"
)

func remoteMediaDeleteHandler(svc *service.Container, preview bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			IDs      []string          `json:"media_ids" binding:"required,min=1,max=1000,dive,required,max=36"`
			Approved map[string]string `json:"approved" binding:"max=1000"`
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256<<10)
		if err := c.ShouldBindJSON(&req); err != nil || (!preview && len(req.Approved) == 0) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "缺少有效的媒体列表或删除确认"})
			return
		}
		ids := slices.Clone(req.IDs)
		slices.Sort(ids)
		ids = slices.Compact(ids)
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
		defer cancel()
		var items []service.RemoteDeleteItem
		var err error
		if preview {
			items, err = svc.Strm.PreviewRemoteMediaDelete(ctx, ids)
		} else {
			items, err = svc.Strm.DeleteRemoteMedia(ctx, svc.Media, ids, req.Approved)
		}
		if err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}

func deleteMediaHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		deleteFiles := parseDeleteFilesQuery(c.Query("delete_files"))
		if err := svc.Media.DeleteMedia(c.Request.Context(), c.Param("id"), deleteFiles); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func parseDeleteFilesQuery(raw string) bool {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return false
	}
	if raw == "1" || raw == "true" || raw == "yes" {
		return true
	}
	v, err := strconv.ParseBool(raw)
	return err == nil && v
}
