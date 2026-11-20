package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Ferrymania/nook/internal/model"
)

type TelegramCloudDownloadRepository struct{ db *gorm.DB }

func (r *TelegramCloudDownloadRepository) Create(ctx context.Context, task *model.TelegramCloudDownload) (bool, error) {
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(task)
	return result.RowsAffected == 1, result.Error
}

func (r *TelegramCloudDownloadRepository) Find(ctx context.Context, id string) (*model.TelegramCloudDownload, error) {
	var task model.TelegramCloudDownload
	err := r.db.WithContext(ctx).First(&task, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &task, err
}

func (r *TelegramCloudDownloadRepository) Update(ctx context.Context, task *model.TelegramCloudDownload, fields map[string]any) (bool, error) {
	result := r.db.WithContext(ctx).Model(&model.TelegramCloudDownload{}).
		Where("id = ? AND status = ?", task.ID, task.Status).Updates(fields)
	return result.RowsAffected == 1, result.Error
}

func (r *TelegramCloudDownloadRepository) Due(ctx context.Context, botKey string, chatID int64, now time.Time) ([]model.TelegramCloudDownload, error) {
	var tasks []model.TelegramCloudDownload
	active := []string{model.CloudDownloadQueued, model.CloudDownloadSubmitting, model.CloudDownloadTracking}
	terminal := []string{model.CloudDownloadDone, model.CloudDownloadFailed}
	err := r.db.WithContext(ctx).Where("bot_key = ? AND chat_id = ? AND check_after <= ?", botKey, chatID, now).
		Where("(status IN ? OR (status IN ? AND reported = false))", active, terminal).
		Order("check_after, id").Limit(20).Find(&tasks).Error
	return tasks, err
}

func (r *TelegramCloudDownloadRepository) DeleteExpiredDrafts(ctx context.Context, before time.Time) error {
	return r.db.WithContext(ctx).Unscoped().Where("status = ? AND created_at < ?", model.CloudDownloadDraft, before).
		Delete(&model.TelegramCloudDownload{}).Error
}
