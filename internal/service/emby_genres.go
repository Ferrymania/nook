package service

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"maps"
	"strconv"
	"strings"

	"github.com/Ferrymania/nook/internal/genre"
	"github.com/Ferrymania/nook/internal/model"
)

// Emby exposes GenreItems as NameLongIdPair[], while Genres remains useful to
// clients that read the older string array. Keep IDs numeric and exactly
// representable by JSON consumers, and share them with the /Genres catalogue.
type embyGenreItem struct {
	Name string `json:"Name"`
	ID   int64  `json:"Id"`
}

func embyGenreNumber(name string) int64 {
	sum := sha256.Sum256([]byte("nook:genre:" + genre.Key(name)))
	id := int64(binary.BigEndian.Uint64(sum[:8]) & ((1 << 53) - 1))
	if id == 0 {
		return 1
	}
	return id
}

func embyGenreID(name string) string {
	return strconv.FormatInt(embyGenreNumber(name), 10)
}

func normalizedEmbyGenres(names []string) []string {
	return genre.Names(names...)
}

func setEmbyGenres(item map[string]any, names []string) {
	genres := normalizedEmbyGenres(names)
	entries := make([]embyGenreItem, 0, len(genres))
	for _, name := range genres {
		entries = append(entries, embyGenreItem{Name: name, ID: embyGenreNumber(name)})
	}
	item["Genres"], item["GenreItems"] = genres, entries
}

func embyMediaGenres(rows []model.Media) []string {
	var names []string
	for _, row := range rows {
		names = append(names, row.Genres)
	}
	return normalizedEmbyGenres(names)
}

func embyPayloadGenres(item map[string]any) []string {
	switch genres := item["Genres"].(type) {
	case []string:
		return normalizedEmbyGenres(genres)
	case []any: // JSON metadata-cache hit.
		names := make([]string, 0, len(genres))
		for _, value := range genres {
			if name, ok := value.(string); ok {
				names = append(names, name)
			}
		}
		return normalizedEmbyGenres(names)
	}
	return []string{}
}

// Copy local payloads before enriching them: Latest singleflight/cache callers
// can share maps across requests. Only episodes without their own genres need
// a fallback query, batched across the entire response and scoped to the viewer.
// Series/Season genres are overlaid from complete visible rows alongside their
// watch counts in withFolderUserData, rather than a partial Latest window.
func (e *EmbyService) withMediaGenres(ctx context.Context, userID string, items []map[string]any) ([]map[string]any, error) {
	if len(items) == 0 {
		return items, nil
	}
	out := append([]map[string]any(nil), items...)
	wanted := map[string]bool{}
	missing := map[int]string{}
	virtual := false
	for i, item := range items {
		id, _ := item["Id"].(string)
		kind, _ := item["Type"].(string)
		if IsEmbyRemoteID(id) || (kind != "Movie" && kind != "Episode") {
			continue
		}
		genres := embyPayloadGenres(item)
		out[i] = maps.Clone(item)
		setEmbyGenres(out[i], genres)
		if kind != "Episode" || len(genres) > 0 {
			continue
		}
		seriesID, _ := item["SeriesId"].(string)
		if seriesID == "" {
			continue
		}
		wanted[seriesID], missing[i] = true, seriesID
		virtual = virtual || strings.HasPrefix(seriesID, embyVirtualSeriesPrefix)
	}
	if len(missing) == 0 || e == nil || e.repo == nil {
		return out, nil
	}
	ids := make([]string, 0, len(wanted))
	for id := range wanted {
		if !strings.HasPrefix(id, embyVirtualSeriesPrefix) {
			ids = append(ids, id)
		}
	}
	scope := e.repo.DB.Where("series_id IN ?", ids)
	if virtual {
		scope = scope.Or("(series_id IS NULL OR series_id = '') AND (season_num > 0 OR episode_num > 0)")
	}
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).Where(scope).
		Where("genres IS NOT NULL AND TRIM(genres) <> ''")
	q = e.applyUserMediaVisibility(ctx, q, userID)
	var rows []model.Media
	if err := q.Select("media.id, media.library_id, media.series_id, media.title, media.original_name, media.path, media.scrape_status, media.season_num, media.episode_num, media.genres").
		Order("media.season_num, media.episode_num, media.id").Find(&rows).Error; err != nil {
		return nil, err
	}
	bySeries := map[string][]string{}
	for _, row := range rows {
		seriesID := row.SeriesID
		if seriesID == "" {
			seriesID = e.seriesIDForMedia(ctx, &row)
		}
		if wanted[seriesID] {
			bySeries[seriesID] = append(bySeries[seriesID], row.Genres)
		}
	}
	for i, seriesID := range missing {
		setEmbyGenres(out[i], bySeries[seriesID])
	}
	return out, nil
}
