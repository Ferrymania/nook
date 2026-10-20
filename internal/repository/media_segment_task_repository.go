package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/Ferrymania/nook/internal/model"
)

type MediaSegmentTaskRepository struct{ db *gorm.DB }

func (r *MediaSegmentTaskRepository) Create(ctx context.Context, task *model.MediaSegmentTask) error {
	return r.db.WithContext(ctx).Create(task).Error
}

func (r *MediaSegmentTaskRepository) FindByID(ctx context.Context, id string) (*model.MediaSegmentTask, error) {
	var task model.MediaSegmentTask
	err := r.db.WithContext(ctx).First(&task, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &task, err
}

func (r *MediaSegmentTaskRepository) Finish(ctx context.Context, task *model.MediaSegmentTask) error {
	result := r.db.WithContext(ctx).Model(&model.MediaSegmentTask{}).
		Where("id = ? AND status = ?", task.ID, model.SegmentTaskRunning).
		Select("status", "error", "segments", "finished_at").Updates(task)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("片头片尾任务不存在或已结束")
	}
	return nil
}

// Queries have a bounded ten-second provider budget. A two-minute running row
// therefore represents an interrupted process, not a task waiting for work.
func (r *MediaSegmentTaskRepository) InterruptStale(ctx context.Context, before time.Time) error {
	return r.db.WithContext(ctx).Model(&model.MediaSegmentTask{}).
		Where("status = ? AND started_at < ?", model.SegmentTaskRunning, before).
		Updates(map[string]any{"status": model.SegmentTaskFailed, "error": "服务重启或查询中断，请重新查询；已有时间点保留", "finished_at": time.Now()}).Error
}

type SegmentQueueSnapshot struct {
	Tasks    []model.MediaSegmentTask `json:"tasks"`
	Counts   map[string]int64         `json:"counts"`
	Total    int64                    `json:"total"`
	Page     int                      `json:"page"`
	PageSize int                      `json:"page_size"`
}

func (r *MediaSegmentTaskRepository) List(ctx context.Context, status, search string, page, size int) (*SegmentQueueSnapshot, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 50
	}
	base := func() *gorm.DB {
		q := r.db.WithContext(ctx).Model(&model.MediaSegmentTask{})
		if search = strings.TrimSpace(search); search != "" {
			pattern := "%" + search + "%"
			q = q.Where("media_title ILIKE ? OR library_name ILIKE ? OR media_path ILIKE ? OR error ILIKE ?", pattern, pattern, pattern, pattern)
		}
		return q
	}
	out := &SegmentQueueSnapshot{Tasks: []model.MediaSegmentTask{}, Counts: map[string]int64{}, Page: page, PageSize: size}
	var counts []struct {
		Status string
		Count  int64
	}
	if err := base().Select("status, count(*) AS count").Group("status").Scan(&counts).Error; err != nil {
		return nil, err
	}
	for _, count := range counts {
		out.Counts[count.Status] = count.Count
	}
	q := base()
	if status != "" && status != "all" {
		q = q.Where("status = ?", status)
	}
	if err := q.Count(&out.Total).Error; err != nil {
		return nil, err
	}
	if err := q.Order("created_at DESC, id DESC").Offset((page - 1) * size).Limit(size).Find(&out.Tasks).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// Removing a record never touches saved timestamps or the negative cache.
// Soft deletion also prevents the upgrade backfill from recreating old records.
func (r *MediaSegmentTaskRepository) DeleteFinished(ctx context.Context, id string) (int64, error) {
	q := r.db.WithContext(ctx).Where("status <> ?", model.SegmentTaskRunning)
	if id != "" {
		q = q.Where("id = ?", id)
	}
	result := q.Delete(&model.MediaSegmentTask{})
	return result.RowsAffected, result.Error
}
