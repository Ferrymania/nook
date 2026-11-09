package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Ferrymania/nook/internal/service"
)

func libraryRatingStatusHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		status, err := svc.ExternalRatings.LibraryStatus(c.Request.Context(), c.Param("id"))
		if err != nil {
			writeRatingError(c, err)
			return
		}
		c.JSON(http.StatusOK, status)
	}
}

func enqueueLibraryRatingsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		count, err := svc.ExternalRatings.EnqueueLibrary(c.Request.Context(), c.Param("id"))
		if err != nil {
			writeRatingError(c, err)
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"queued": count})
	}
}

func mediaExternalRatingsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		media, err := svc.Repo.Media.FindByID(ctx, c.Param("id"))
		if err != nil {
			writeRatingError(c, err)
			return
		}
		if media == nil || !mediaVisibleForRequest(c, svc, media) {
			c.JSON(http.StatusNotFound, gin.H{"error": "媒体不存在"})
			return
		}
		ratings, err := svc.ExternalRatings.ForMedia(ctx, media)
		if err != nil {
			writeRatingError(c, err)
			return
		}
		c.JSON(http.StatusOK, ratings)
	}
}

func writeRatingError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrRatingLibraryNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrOMDbNotConfigured):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法读取或提交评分任务，请稍后重试"})
	}
}
