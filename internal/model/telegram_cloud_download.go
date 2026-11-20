package model

import "time"

const (
	CloudDownloadDraft      = "draft"
	CloudDownloadQueued     = "queued"
	CloudDownloadSubmitting = "submitting"
	CloudDownloadTracking   = "tracking"
	CloudDownloadDone       = "done"
	CloudDownloadFailed     = "failed"
)

// TelegramCloudDownload tracks one Telegram confirmation and its 115 downloads.
type TelegramCloudDownload struct {
	Base
	BotKey           string `gorm:"size:64;index:idx_telegram_cloud_download_poll"`
	ChatID           int64  `gorm:"index:idx_telegram_cloud_download_poll"`
	MessageID        int64
	Status           string    `gorm:"size:20;index"`
	CheckAfter       time.Time `gorm:"index:idx_telegram_cloud_download_poll"`
	PollDelaySeconds int       `gorm:"default:0"`
	EncryptedLink    string    `gorm:"type:text"`
	AccountID        string    `gorm:"size:36"`
	StagingFolderID  string    `gorm:"size:32"`
	Directory        string
	Items            []TelegramCloudDownloadItem `gorm:"serializer:json;type:text"`
	DetailPage       int                         `gorm:"default:0"`
	RemoteTaskID     string                      `gorm:"size:128"`
	SubmittedAt      *time.Time
	MissingSince     *time.Time
	Progress         int
	Detail           string
	Reported         bool
}

// TelegramCloudDownloadItem tracks one link within a confirmed download.
type TelegramCloudDownloadItem struct {
	Name          string     `json:"name,omitempty"`
	EncryptedLink string     `json:"encrypted_link,omitempty"`
	Status        string     `json:"status"`
	RemoteTaskID  string     `json:"remote_task_id,omitempty"`
	MissingSince  *time.Time `json:"missing_since,omitempty"`
	Progress      int        `json:"progress"`
	Detail        string     `json:"detail,omitempty"`
}
