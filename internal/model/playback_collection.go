package model

import "time"

// PlaybackHistory 记录当前播放位置以支持续播。
// (user_id, media_id) 唯一：播放进度每几秒上报一次，唯一索引保证并发上报
// 不会插入重复行（否则续播列表会出现重复卡片），也让 upsert 单语句完成。
type PlaybackHistory struct {
	Base
	UserID     string    `gorm:"index;size:36;not null;uniqueIndex:uniq_user_history" json:"user_id"`
	MediaID    string    `gorm:"index;size:128;not null;uniqueIndex:uniq_user_history" json:"media_id"`
	PositionMs int64     `json:"position_ms"`
	DurationMs int64     `json:"duration_ms"`
	WatchedAt  time.Time `json:"watched_at"`
	Completed  bool      `json:"completed"`

	// 播放会话信息用于丢弃乱序到达的旧进度，避免旧请求把新的续播状态覆盖。
	// 旧客户端不提供这些字段时保持 0，继续沿用无条件 upsert 语义。
	SessionID          string `gorm:"size:128;index" json:"-"`
	SessionStartedAtMs int64  `gorm:"not null;default:0" json:"-"`
	Sequence           int64  `gorm:"not null;default:0" json:"-"`
}

// Favorite 将媒体项标记为给定用户的收藏。
type Favorite struct {
	Base
	UserID  string `gorm:"index;size:36;not null;uniqueIndex:uniq_user_media" json:"user_id"`
	MediaID string `gorm:"index;size:128;not null;uniqueIndex:uniq_user_media" json:"media_id"`
}

// Playlist 是用户策划的、有序的媒体列表。
type Playlist struct {
	Base
	UserID   string `gorm:"index;size:36;not null" json:"user_id"`
	Name     string `gorm:"size:128;not null" json:"name"`
	IsPublic bool   `gorm:"default:false" json:"is_public"`
}

// PlaylistItem 是 Playlist 和 Media 的连接表，带有排序。
type PlaylistItem struct {
	Base
	PlaylistID string `gorm:"index;size:36;not null" json:"playlist_id"`
	MediaID    string `gorm:"index;size:128;not null" json:"media_id"`
	Position   int    `json:"position"`
}
