package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Ferrymania/nook/internal/middleware"
	"github.com/Ferrymania/nook/internal/service"
)

const embyCtxUserName = "emby_user_name"

func activeUserRequired(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, _ := c.Get(middleware.CtxUserID)
		userID, _ := uid.(string)
		if userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 40101, "message": "missing user"})
			return
		}
		if svc == nil || svc.Repo == nil || svc.Repo.User == nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"code": 50301, "message": "unable to verify account state"})
			return
		}
		u, err := svc.Repo.User.FindByID(c.Request.Context(), userID)
		if err != nil {
			if service.IsTransientDatabaseLock(err) {
				c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"code": 50301, "message": "unable to verify account state"})
				return
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 40101, "message": "user not found"})
			return
		}
		if u == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 40101, "message": "user not found"})
			return
		}
		if !u.IsActive {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 40302, "message": "user account is disabled"})
			return
		}
		if u.ExpiredAt != nil && time.Now().After(*u.ExpiredAt) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 40303, "message": "user account has expired"})
			return
		}
		if u.ForcePasswordReset && !middleware.PasswordResetRequestAllowed(c) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 40305, "message": service.ErrPasswordResetRequired.Error()})
			return
		}
		if !u.ForcePasswordReset && c.GetString(middleware.CtxTokenPurpose) == service.PasswordResetTokenPurpose {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 40101, "message": "password changed; sign in again"})
			return
		}
		c.Next()
	}
}

func activeEmbyUserRequired(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, _ := c.Get(middleware.CtxUserID)
		userID, _ := uid.(string)
		u, err := svc.Repo.User.FindByID(c.Request.Context(), userID)
		if userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"Code": 40101, "Message": "User not found"})
			return
		}
		if err != nil {
			if service.IsTransientDatabaseLock(err) {
				c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"Code": 50301, "Message": "Unable to verify account state"})
				return
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"Code": 40101, "Message": "User not found"})
			return
		}
		if u == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"Code": 40101, "Message": "User not found"})
			return
		}
		if !u.IsActive {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"Code": 40302, "Message": "User account is disabled"})
			return
		}
		if u.ExpiredAt != nil && time.Now().After(*u.ExpiredAt) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"Code": 40303, "Message": "User account has expired"})
			return
		}
		if u.ForcePasswordReset {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"Code": 40305, "Message": service.ErrPasswordResetRequired.Error()})
			return
		}
		c.Set(embyCtxUserName, u.Username)
		c.Next()
	}
}
