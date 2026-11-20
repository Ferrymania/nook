package handler

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Ferrymania/nook/internal/middleware"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/service"
)

func saveProfileAvatarHandler(svc *service.Container, remove bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var file []byte
		if !remove {
			if c.ContentType() != "application/octet-stream" {
				c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": service.ErrAvatarInvalid.Error()})
				return
			}
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.AvatarMaxBytes)
			var err error
			file, err = io.ReadAll(c.Request.Body)
			if err != nil {
				var tooLarge *http.MaxBytesError
				if errors.As(err, &tooLarge) {
					c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": service.ErrAvatarTooLarge.Error()})
				} else {
					c.JSON(http.StatusBadRequest, gin.H{"error": "无法读取头像图片"})
				}
				return
			}
			if len(file) == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": service.ErrAvatarInvalid.Error()})
				return
			}
		}

		user, err := svc.Profile.SetAvatar(c.Request.Context(), c.GetString(middleware.CtxUserID), file)
		switch {
		case err == nil:
			c.JSON(http.StatusOK, user)
		case errors.Is(err, service.ErrAvatarInvalid), errors.Is(err, service.ErrAvatarPixelBudget):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存头像失败，请稍后重试"})
		}
	}
}

func profileAvatarHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.GetString(middleware.CtxUserID)
		user, err := svc.Repo.User.FindByID(c.Request.Context(), uid)
		c.Writer.Header().Add("Vary", "Authorization, Cookie")
		c.Header("Cache-Control", "private, no-cache")
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		if user == nil || !serveUserAvatar(privateAvatarResponse{c.Writer}, c.Request, svc, uid, user.AvatarURL) {
			c.Status(http.StatusNotFound)
		}
	}
}

// The legacy image proxy uses public caching. The account-specific web route
// must keep its response private even when serving one of those images.
type privateAvatarResponse struct{ http.ResponseWriter }

func (w privateAvatarResponse) WriteHeader(status int) {
	w.Header().Set("Cache-Control", "private, no-cache")
	w.ResponseWriter.WriteHeader(status)
}

func serveUserAvatar(w http.ResponseWriter, r *http.Request, svc *service.Container, userID, source string) bool {
	source = strings.TrimSpace(source)
	if source == "" || svc == nil {
		return false
	}
	if !strings.HasPrefix(source, model.UploadedAvatarURLPrefix) {
		return svc.ImageProxy != nil && svc.ImageProxy.Serve(r.Context(), w, r, source) == nil
	}
	if svc.Profile == nil {
		return false
	}
	data, err := svc.Profile.UploadedAvatar(r.Context(), userID)
	if err != nil || len(data) == 0 {
		return false
	}
	w.Header().Set("Content-Type", http.DetectContentType(data))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("ETag", fmt.Sprintf(`"%x"`, sha256.Sum256(data)))
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
	return true
}
