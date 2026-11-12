package service

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/Ferrymania/nook/internal/model"
)

func (e *EmbyService) applyUserMediaVisibility(ctx context.Context, q *gorm.DB, userID string) *gorm.DB {
	visibility := e.mediaVisibility(ctx, userID)
	if len(visibility.AllowedLibraryIDs) > 0 {
		q = q.Where("library_id IN ?", visibility.AllowedLibraryIDs)
	}
	return q
}

func (e *EmbyService) filterMediaRowsForUser(ctx context.Context, rows []model.Media, userID string) []model.Media {
	visibility := e.mediaVisibility(ctx, userID)
	if len(visibility.AllowedLibraryIDs) == 0 {
		return rows
	}
	out := rows[:0]
	for i := range rows {
		if visibility.Allows(&rows[i]) {
			out = append(out, rows[i])
		}
	}
	return out
}

func (e *EmbyService) mediaVisibility(ctx context.Context, userID string) MediaVisibility {
	if e == nil {
		return MediaVisibility{}
	}
	key := strings.TrimSpace(userID)
	now := time.Now()
	e.visibilityMu.RLock()
	entry, ok := e.visibilityCache[key]
	e.visibilityMu.RUnlock()
	if ok && now.Before(entry.expiresAt) {
		return cloneMediaVisibility(entry.visibility)
	}

	visibility := UserDefaultMediaVisibility(ctx, e.repo, userID)
	visibility = ExpandMediaVisibilityForMergedCloudLibraries(ctx, e.repo, visibility)
	visibility = cloneMediaVisibility(visibility)

	e.visibilityMu.Lock()
	if e.visibilityCache == nil {
		e.visibilityCache = make(map[string]embyVisibilityCacheEntry)
	}
	if len(e.visibilityCache) > 1000 {
		e.visibilityCache = make(map[string]embyVisibilityCacheEntry)
	}
	e.visibilityCache[key] = embyVisibilityCacheEntry{
		visibility: cloneMediaVisibility(visibility),
		expiresAt:  now.Add(embyVisibilityCacheTTL),
	}
	e.visibilityMu.Unlock()

	return visibility
}

func (e *EmbyService) mergedLibraryIDs(ctx context.Context, libraryID string) []string {
	ids, err := MergedLibraryIDsForLibrary(ctx, e.repo, libraryID)
	if err != nil || len(ids) == 0 {
		return []string{libraryID}
	}
	return ids
}

func cloneMediaVisibility(visibility MediaVisibility) MediaVisibility {
	if visibility.AllowedLibraryIDs != nil {
		visibility.AllowedLibraryIDs = append([]string(nil), visibility.AllowedLibraryIDs...)
	}
	return visibility
}

func (e *EmbyService) libraryVisibleFromCachedVisibility(lib model.Library, visibility MediaVisibility) bool {
	return LibraryIDAllowed(visibility, lib.ID)
}
