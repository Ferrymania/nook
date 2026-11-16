package service

import (
	"context"
	"fmt"
	"strconv"

	"github.com/Ferrymania/nook/internal/model"
)

type telegramSiteStatus struct {
	telegramMediaStatus
	Libraries    int64
	Users        int64
	PlayingUsers int64
}

func (s *TelegramService) siteStatus(ctx context.Context) (telegramSiteStatus, error) {
	var status telegramSiteStatus
	// Count logical libraries, including disabled ones, rather than their roots.
	var libraries []model.Library
	if err := s.repo.DB.WithContext(ctx).Select("id", "type", "path").Find(&libraries).Error; err != nil {
		return status, fmt.Errorf("list libraries: %w", err)
	}
	status.Libraries = int64(len(libraries))
	var err error
	status.Users, err = s.repo.User.Count(ctx)
	if err != nil {
		return status, fmt.Errorf("count users: %w", err)
	}
	status.telegramMediaStatus, err = s.mediaStatus(ctx, libraries)
	if err != nil {
		return status, fmt.Errorf("count media: %w", err)
	}

	// ListPlaying expires stale playback heartbeats. A user with multiple
	// playing devices counts once; paused sessions do not count as playing.
	userIDs := make(map[string]struct{})
	for _, session := range s.sessions.ListPlaying(ctx) {
		if !session.IsPaused && session.UserID != "" {
			userIDs[session.UserID] = struct{}{}
		}
	}
	if len(userIDs) > 0 {
		ids := make([]string, 0, len(userIDs))
		for id := range userIDs {
			ids = append(ids, id)
		}
		// Deleted accounts can leave transient sessions until their heartbeat
		// expires. Only count users that still exist.
		if err := s.repo.DB.WithContext(ctx).Model(&model.User{}).
			Where("id IN ?", ids).Count(&status.PlayingUsers).Error; err != nil {
			return status, fmt.Errorf("count playing users: %w", err)
		}
	}
	return status, nil
}

func (s *TelegramService) sendSiteStatus(ctx context.Context, token, chatID string) {
	status, err := s.siteStatus(ctx)
	if err != nil {
		s.warn("telegram site status failed", "admin", err)
		s.controlReply(ctx, token, chatID, "读取站点状态失败，请稍后重试", nil)
		return
	}
	s.controlReply(ctx, token, chatID, fmt.Sprintf(
		"📊 Nook 站点状态\n\n媒体库数量：%s 个\n用户数：%s 人\n当前正在播放用户数：%s 人\n\n电影：%s 部\n剧集：%s 部\n总集数：%s 集",
		formatTelegramCount(status.Libraries), formatTelegramCount(status.Users), formatTelegramCount(status.PlayingUsers),
		formatTelegramCount(status.Movies), formatTelegramCount(status.Series), formatTelegramCount(status.Episodes),
	), nil)
}

func formatTelegramCount(count int64) string {
	value := strconv.FormatInt(count, 10)
	for i := len(value) - 3; i > 0; i -= 3 {
		value = value[:i] + "," + value[i:]
	}
	return value
}
