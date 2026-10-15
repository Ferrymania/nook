package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Ferrymania/nook/internal/model"
)

// SeriesRepository persists model.Series records.
type SeriesRepository struct{ db *gorm.DB }

// FindByID returns the series or (nil, nil).
func (r *SeriesRepository) FindByID(ctx context.Context, id string) (*model.Series, error) {
	var s model.Series
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// List returns all series (ordered by title).
func (r *SeriesRepository) List(ctx context.Context) ([]model.Series, error) {
	var s []model.Series
	err := r.db.WithContext(ctx).Order("title asc").Find(&s).Error
	return s, err
}

func (r *SeriesRepository) FindByIDs(ctx context.Context, ids []string) ([]model.Series, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []model.Series
	err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&rows).Error
	return rows, err
}

func (r *SeriesRepository) Upsert(ctx context.Context, series *model.Series) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"title", "poster_url", "backdrop_url", "overview", "rating", "year",
			"tm_db_id", "bangumi_id", "douban_id", "thetvdb_id", "updated_at", "deleted_at",
		}),
	}).Create(series).Error
}
