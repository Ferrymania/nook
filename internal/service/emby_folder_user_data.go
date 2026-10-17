package service

import (
	"context"
	"fmt"
	"maps"
	"strings"

	"github.com/Ferrymania/nook/internal/model"
)

func (e *EmbyService) withFolderUserDataEnvelope(ctx context.Context, userID string, envelope map[string]any) (map[string]any, error) {
	items, ok := envelope["Items"].([]map[string]any)
	if !ok {
		return envelope, nil
	}
	items, err := e.withFolderUserData(ctx, userID, items)
	if err != nil {
		return nil, err
	}
	out := maps.Clone(envelope)
	out["Items"] = items
	return out, nil
}

// Overlay local Series/Season watch counts after reading metadata caches. Playback
// changes must be visible even while list-cache invalidation is throttled. Read
// complete series, since Latest's candidate window may contain only some episodes.
func (e *EmbyService) withFolderUserData(ctx context.Context, userID string, items []map[string]any) ([]map[string]any, error) {
	seriesIDs := []string{}
	wanted := map[string]bool{}
	virtualLibraries := map[string]bool{}
	allVirtualLibraries := false
	for _, item := range items {
		id, _ := item["Id"].(string)
		kind, _ := item["Type"].(string)
		if id == "" || IsEmbyRemoteID(id) || (kind != "Series" && kind != "Season") {
			continue
		}
		seriesID := id
		libraryID, _ := item["ParentId"].(string)
		if kind == "Season" {
			seriesID, _ = item["SeriesId"].(string)
			libraryID = ""
			if season, ok := e.cachedSeasonGroup(id); ok {
				libraryID = season.LibraryID
			}
		}
		if seriesID == "" || wanted[seriesID] {
			continue
		}
		wanted[seriesID] = true
		seriesIDs = append(seriesIDs, seriesID)
		if strings.HasPrefix(seriesID, embyVirtualSeriesPrefix) {
			if libraryID == "" {
				allVirtualLibraries = true
			} else {
				virtualLibraries[libraryID] = true
			}
		}
	}
	if len(seriesIDs) == 0 {
		return items, nil
	}

	q := e.repo.DB.WithContext(ctx).Model(&model.Media{})
	scope := e.repo.DB.Where("series_id IN ?", seriesIDs)
	if allVirtualLibraries {
		scope = scope.Or("series_id IS NULL OR series_id = ''")
	} else if len(virtualLibraries) > 0 {
		libraries := make([]string, 0, len(virtualLibraries))
		for id := range virtualLibraries {
			libraries = append(libraries, id)
		}
		scope = scope.Or("(series_id IS NULL OR series_id = '') AND library_id IN ?", libraries)
	}
	q = e.applyUserMediaVisibility(ctx, q.Where(scope).Where("season_num > 0 OR episode_num > 0"), userID)
	columns := "media.id, media.library_id, media.series_id, media.title, media.original_name, media.path, media.scrape_status, media.season_num, media.episode_num, media.episode_fraction"
	if strings.TrimSpace(userID) != "" {
		q = q.Joins("LEFT JOIN playback_histories AS watched ON watched.media_id = media.id AND watched.user_id = ? AND watched.deleted_at IS NULL AND watched.completed = ?", userID, true)
		columns += ", COALESCE(watched.completed, false) AS episode_played"
	} else {
		columns += ", false AS episode_played"
	}
	var rows []struct {
		model.Media
		EpisodePlayed bool
	}
	if err := q.Select(columns).Find(&rows).Error; err != nil {
		return nil, err
	}

	// A completed version marks that logical episode watched. Different seasons,
	// fractional episodes and special categories remain separate; unknown episode
	// numbers stay independent rather than merging unrelated specials.
	episodes := map[string]map[string]bool{}
	seasons := map[string]map[string]bool{}
	for _, row := range rows {
		seriesID := e.seriesIDForMedia(ctx, &row.Media)
		if !wanted[seriesID] {
			continue
		}
		seasonNum := embySeasonNumForMedia(&row.Media)
		seasonID := seasonID(seriesID, seasonNum)
		key := row.ID
		if row.EpisodeNum > 0 {
			key = fmt.Sprintf("s:%d|e:%d%s", seasonNum, row.EpisodeNum, episodeFractionSuffix(row.EpisodeFraction))
		}
		for _, id := range []string{seriesID, seasonID} {
			if episodes[id] == nil {
				episodes[id] = map[string]bool{}
			}
			episodes[id][key] = episodes[id][key] || row.EpisodePlayed
		}
		if seasons[seriesID] == nil {
			seasons[seriesID] = map[string]bool{}
		}
		seasons[seriesID][seasonID] = true
	}

	// Latest singleflight callers can share these maps. Never write user state
	// into the shared metadata payload or another request's UserData map.
	out := append([]map[string]any(nil), items...)
	for i, item := range items {
		id, _ := item["Id"].(string)
		kind, _ := item["Type"].(string)
		seriesID := id
		if kind == "Season" {
			seriesID, _ = item["SeriesId"].(string)
		}
		if !wanted[seriesID] || IsEmbyRemoteID(id) || (kind != "Series" && kind != "Season") {
			continue
		}
		total, played := len(episodes[id]), 0
		for _, completed := range episodes[id] {
			if completed {
				played++
			}
		}
		data := emptyUserData()
		if existing, ok := item["UserData"].(map[string]any); ok {
			maps.Copy(data, existing)
		}
		data["UnplayedItemCount"] = total - played
		data["Played"] = total > 0 && played == total
		percentage := float64(0)
		if total > 0 {
			percentage = float64(played) * 100 / float64(total)
		}
		data["PlayedPercentage"] = percentage
		out[i] = maps.Clone(item)
		out[i]["UserData"] = data
		out[i]["RecursiveItemCount"] = total
		if kind == "Season" {
			out[i]["ChildCount"] = total
		} else {
			out[i]["ChildCount"] = len(seasons[id])
		}
	}
	return out, nil
}
