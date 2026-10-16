package service

import (
	"context"
	"strings"

	"github.com/Ferrymania/nook/internal/model"
)

// similarSeriesItems preserves recommendation order while returning the same
// complete Series DTOs and artwork used by library browsing.
func (e *EmbyService) similarSeriesItems(ctx context.Context, candidates []model.Media, userID string) ([]map[string]any, map[string]embyArtworkRef, error) {
	ctx = e.withPayloadCache(ctx)
	e.prefetchPayloadCache(ctx, candidates)
	seriesIDs, virtualLibraries := make([]string, 0, len(candidates)), make([]string, 0)
	for _, m := range candidates {
		if id := strings.TrimSpace(m.SeriesID); id != "" {
			seriesIDs = append(seriesIDs, id)
		} else {
			virtualLibraries = append(virtualLibraries, m.LibraryID)
		}
	}
	// Load complete shows in one query. Grouping only representative episodes
	// would cache incomplete virtual series and hide their other seasons.
	var episodes []model.Media
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Where("(season_num > 0 OR episode_num > 0)").
		Where("series_id IN ? OR ((series_id IS NULL OR series_id = '') AND library_id IN ?)", seriesIDs, virtualLibraries)
	q = e.applyUserMediaVisibility(ctx, q, userID)
	if err := q.Order("media.season_num asc, media.episode_num asc, media.episode_fraction asc, media.created_at asc").
		Limit(embySeriesGroupingLimit).Find(&episodes).Error; err != nil {
		return nil, nil, err
	}
	e.prefetchPayloadCache(ctx, episodes)
	byID := make(map[string]embySeriesGroup)
	for _, group := range e.seriesGroupsFromMedia(ctx, episodes) {
		byID[group.ID] = group
	}
	items := make([]map[string]any, 0, len(candidates))
	groups := make([]embySeriesGroup, 0, len(candidates))
	seen := make(map[string]bool)
	for i := range candidates {
		id := e.seriesIDForMedia(ctx, &candidates[i])
		if group, ok := byID[id]; ok && !seen[id] {
			seen[id] = true
			items = append(items, e.seriesPayload(group))
			groups = append(groups, group)
		}
	}
	return items, e.artworkRefsForSeriesGroups(groups), nil
}
