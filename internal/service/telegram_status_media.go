package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
)

type telegramMediaStatus struct {
	Movies   int64
	Series   int64
	Episodes int64
}

type telegramStatusMediaRow struct {
	repository.MediaSeriesPreviewRow `gorm:"embedded"`
	EpisodeFraction                  float64
}

func (s *TelegramService) mediaStatus(ctx context.Context, libraries []model.Library) (telegramMediaStatus, error) {
	var status telegramMediaStatus
	byID := make(map[string]model.Library, len(libraries))
	ids := make([]string, 0, len(libraries))
	for _, library := range libraries {
		if strings.EqualFold(strings.TrimSpace(library.Type), "music") {
			continue
		}
		byID[library.ID] = library
		ids = append(ids, library.ID)
	}
	if len(ids) == 0 {
		return status, nil
	}

	// Read only identity fields. Artwork, descriptions, probe data and remote
	// Emby mounts are not needed for local inventory counts.
	var rows []telegramStatusMediaRow
	if err := s.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Select("id", "library_id", "series_id", "title", "original_name", "path",
			"season_num", "episode_num", "episode_fraction", "scrape_status",
			"tm_db_id", "bangumi_id", "douban_id", "thetvdb_id", "year").
		Where("library_id IN ?", ids).Find(&rows).Error; err != nil {
		return status, err
	}
	mediaAt := func(i int) model.Media {
		media := seriesPreviewMedia(rows[i].MediaSeriesPreviewRow)
		media.EpisodeFraction = rows[i].EpisodeFraction
		media.LibraryPath = byID[media.LibraryID].Path
		if media.SeasonNum == 0 && media.EpisodeNum == 0 {
			media.SeasonNum, media.EpisodeNum = ParseEpisode(media.Path)
		}
		return media
	}
	// Use the library card identity resolver so unmatched episodes
	// can still share a show via their directory or title, without a Series row.
	_, keys := resolveMediaSeriesKeysFrom(len(rows), mediaAt)
	movies, series, episodes := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for i := range rows {
		media := mediaAt(i)
		library := byID[media.LibraryID]
		key := media.LibraryID + "\x00" + keys[i]
		if !mediaIsEpisodic(&media, &library) {
			movies[key] = true
			continue
		}
		series[key] = true
		episodeKey := key + "\x00media:" + media.ID
		if media.EpisodeNum > 0 {
			episodeKey = fmt.Sprintf("%s\x00S%dE%s", key, media.SeasonNum, FormatEpisodeNumber(media.EpisodeNum, media.EpisodeFraction))
		}
		episodes[episodeKey] = true
	}
	status.Movies, status.Series, status.Episodes = int64(len(movies)), int64(len(series)), int64(len(episodes))
	return status, nil
}
