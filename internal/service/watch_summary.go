package service

import (
	"context"
	"fmt"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
)

type WatchSummary struct {
	Movies   int64 `json:"movies"`
	Series   int64 `json:"series"`
	Episodes int64 `json:"episodes"`
}

// CompletedWatchSummary counts visible, completed items from local media libraries.
func CompletedWatchSummary(ctx context.Context, repo *repository.Container, userID string, visibility MediaVisibility) (WatchSummary, error) {
	var libraries []model.Library
	if err := repo.DB.WithContext(ctx).Select("id", "path", "type").Where("enabled AND type <> ?", "music").Find(&libraries).Error; err != nil {
		return WatchSummary{}, err
	}
	byID := make(map[string]model.Library, len(libraries))
	ids := make([]string, 0, len(libraries))
	for _, library := range libraries {
		if LibraryIDAllowed(visibility, library.ID) {
			byID[library.ID] = library
			ids = append(ids, library.ID)
		}
	}
	if len(ids) == 0 {
		return WatchSummary{}, nil
	}

	db := repo.DB.WithContext(ctx)
	completed := db.Model(&model.PlaybackHistory{}).Select("media_id").Where("user_id = ? AND completed", userID)
	var items []model.Media
	if err := db.Model(&model.Media{}).
		Select("id", "library_id", "series_id", "title", "original_name", "path", "season_num", "episode_num", "episode_fraction",
			"scrape_status", "tm_db_id", "bangumi_id", "douban_id", "thetvdb_id", "year", "official_rating", "rating_country").
		Where("library_id IN ? AND id IN (?)", ids, completed).Find(&items).Error; err != nil {
		return WatchSummary{}, err
	}
	visible := items[:0]
	for _, item := range items {
		if visibility.Allows(&item) {
			visible = append(visible, item)
		}
	}
	return countMediaTitles(len(visible), func(i int) model.Media { return visible[i] }, byID), nil
}

func countMediaTitles(count int, mediaAt func(int) model.Media, libraries map[string]model.Library) WatchSummary {
	mediaWithLibrary := func(i int) model.Media {
		media := mediaAt(i)
		media.LibraryPath = libraries[media.LibraryID].Path
		if media.SeasonNum == 0 && media.EpisodeNum == 0 {
			media.SeasonNum, media.EpisodeNum = ParseEpisode(media.Path)
		}
		return media
	}
	_, keys := resolveMediaSeriesKeysFrom(count, mediaWithLibrary)
	movies, series, episodes := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for i, identity := range keys {
		media := mediaWithLibrary(i)
		library := libraries[media.LibraryID]
		key := media.LibraryID + "\x00" + identity
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
	return WatchSummary{Movies: int64(len(movies)), Series: int64(len(series)), Episodes: int64(len(episodes))}
}
