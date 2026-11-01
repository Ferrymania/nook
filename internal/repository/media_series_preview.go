package repository

import (
	"context"
	"time"

	"github.com/Ferrymania/nook/internal/model"
)

// MediaSeriesPreviewRow contains only the fields used by series identity,
// ordering and representative selection. Large display/playback fields are
// loaded only for the few records selected for the preview response.
type MediaSeriesPreviewRow struct {
	ID           string
	LibraryID    string
	SeriesID     string
	Title        string
	OriginalName string
	Path         string
	PosterURL    string
	BackdropURL  string
	SeasonNum    int
	EpisodeNum   int
	ScrapeStatus string
	TMDbID       int
	BangumiID    int
	DoubanID     string `gorm:"column:douban_id"`
	TheTVDBID    string `gorm:"column:thetvdb_id"`
	Year         int
	ReleaseDate  string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (r *MediaRepository) ListSeriesPreviewRows(ctx context.Context, libraryIDs []string, filter MediaQueryFilter) ([]MediaSeriesPreviewRow, error) {
	items := make([]MediaSeriesPreviewRow, 0)
	if len(libraryIDs) == 0 {
		return items, nil
	}
	q := r.db.WithContext(ctx).Model(&model.Media{}).
		Select("id", "library_id", "series_id", "title", "original_name", "path",
			"poster_url", "backdrop_url", "season_num", "episode_num", "scrape_status",
			"tm_db_id", "bangumi_id", "douban_id", "thetvdb_id", "year", "release_date", "created_at", "updated_at").
		Where("library_id IN ?", libraryIDs)
	q = applyMediaQueryFilter(q, filter)
	err := q.Order("release_date DESC, year DESC, updated_at DESC, created_at DESC, id DESC").Find(&items).Error
	return items, err
}

// ListPreviewMediaByIDs hydrates selected representatives and link targets in
// one query, reapplying visibility and library scope if rows changed meanwhile.
func (r *MediaRepository) ListPreviewMediaByIDs(ctx context.Context, libraryIDs, ids []string, filter MediaQueryFilter) ([]model.Media, error) {
	items := make([]model.Media, 0)
	if len(libraryIDs) == 0 || len(ids) == 0 {
		return items, nil
	}
	q := r.db.WithContext(ctx).Model(&model.Media{}).
		Where("library_id IN ?", libraryIDs).Where("id IN ?", ids)
	err := applyMediaQueryFilter(q, filter).Find(&items).Error
	return items, err
}
