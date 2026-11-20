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

// TelegramCloudDownload tracks a single 115 cloud download to a staging directory.
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
	RemoteTaskID     string `gorm:"size:128"`
	SubmittedAt      *time.Time
	MissingSince     *time.Time
	Progress         int
	Detail           string
	Reported         bool
}
