package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/service"
)

type adminPlaybackSession struct {
	ID           string     `json:"id"`
	UserID       string     `json:"user_id"`
	Username     string     `json:"username"`
	DeviceName   string     `json:"device_name"`
	Client       string     `json:"client"`
	MediaID      string     `json:"media_id"`
	Title        string     `json:"title"`
	SeriesName   string     `json:"series_name,omitempty"`
	SeasonNum    int        `json:"season_num"`
	EpisodeNum   int        `json:"episode_num"`
	PositionMs   int64      `json:"position_ms"`
	DurationMs   int64      `json:"duration_ms"`
	IsPaused     bool       `json:"is_paused"`
	LastReportAt *time.Time `json:"last_report_at"`
}

func adminPlaybackSessionsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		out := make([]adminPlaybackSession, 0)
		if svc.Sessions == nil {
			c.JSON(http.StatusOK, gin.H{"items": out})
			return
		}
		sessions := svc.Sessions.ListPlaying(c.Request.Context())
		if len(sessions) == 0 {
			c.JSON(http.StatusOK, gin.H{"items": out})
			return
		}
		userIDs, mediaIDs := make([]string, 0, len(sessions)), make([]string, 0, len(sessions))
		for _, sess := range sessions {
			userIDs = append(userIDs, sess.UserID)
			mediaIDs = append(mediaIDs, sess.ItemID)
		}
		// Hydrate only display metadata in batches. No probing, scraping or history writes.
		var users []model.User
		var media []model.Media
		var series []model.Series
		db := svc.Repo.DB.WithContext(c.Request.Context())
		if err := db.Select("id", "username").Where("id IN ?", userIDs).Find(&users).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "load playback users failed"})
			return
		}
		if err := db.Select("id", "title", "episode_title", "series_id", "season_num", "episode_num", "duration_sec").Where("id IN ?", mediaIDs).Find(&media).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "load playback media failed"})
			return
		}
		names := make(map[string]string, len(users))
		for _, user := range users {
			names[user.ID] = user.Username
		}
		mediaByID := make(map[string]model.Media, len(media))
		seriesIDs := make([]string, 0, len(media))
		for _, m := range media {
			mediaByID[m.ID] = m
			if m.SeriesID != "" {
				seriesIDs = append(seriesIDs, m.SeriesID)
			}
		}
		if len(seriesIDs) > 0 {
			if err := db.Select("id", "title").Where("id IN ?", seriesIDs).Find(&series).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "load playback series failed"})
				return
			}
		}
		seriesNames := make(map[string]string, len(series))
		for _, s := range series {
			seriesNames[s.ID] = s.Title
		}
		// Remote metadata is optional; an unavailable mount must not hide a live session.
		remoteCtx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		for _, sess := range sessions {
			row := adminPlaybackSession{
				ID: sess.ID, UserID: sess.UserID, Username: names[sess.UserID],
				DeviceName: sess.DeviceName, Client: sess.Client, MediaID: sess.ItemID,
				Title: "未知影片", PositionMs: sess.PositionTicks / 10000, DurationMs: sess.RuntimeTicks / 10000,
				IsPaused: sess.IsPaused, LastReportAt: sess.LastPlaybackAt,
			}
			if row.Username == "" {
				row.Username = sess.UserName
			}
			if m, ok := mediaByID[sess.ItemID]; ok {
				row.Title = m.Title
				row.SeriesName = seriesNames[m.SeriesID]
				row.SeasonNum, row.EpisodeNum = m.SeasonNum, m.EpisodeNum
				if row.EpisodeNum > 0 && row.SeriesName == "" {
					row.SeriesName = m.Title
				}
				if m.EpisodeTitle != "" {
					row.Title = m.EpisodeTitle
				}
				if row.DurationMs <= 0 {
					row.DurationMs = int64(m.DurationSec) * 1000
				}
			} else if svc.Emby != nil && service.IsEmbyRemoteID(sess.ItemID) {
				if item, err := svc.Emby.Item(remoteCtx, sess.ItemID, sess.UserID); err == nil && item != nil {
					if title, ok := item["Name"].(string); ok && title != "" {
						row.Title = title
					}
					row.SeriesName, _ = item["SeriesName"].(string)
					row.SeasonNum = playbackItemNumber(item["ParentIndexNumber"])
					row.EpisodeNum = playbackItemNumber(item["IndexNumber"])
				}
			}
			out = append(out, row)
		}
		c.JSON(http.StatusOK, gin.H{"items": out})
	}
}

func playbackItemNumber(value any) int {
	switch n := value.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}
