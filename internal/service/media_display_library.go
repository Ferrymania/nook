package service

import (
	"context"
	"sort"
	"strings"

	"github.com/Ferrymania/nook/internal/model"
)

func (s *MediaService) attachLibraryMetadata(ctx context.Context, items []model.Media) {
	if s == nil || s.repo == nil || s.repo.Library == nil || len(items) == 0 {
		return
	}
	libs, err := s.displayLibraries(ctx)
	if err != nil {
		return
	}
	byID := make(map[string]model.Library, len(libs))
	for _, lib := range libs {
		byID[lib.ID] = lib
	}
	resolver := newMediaDisplayLibraryResolver(libs)
	for i := range items {
		if lib, ok := byID[items[i].LibraryID]; ok {
			items[i].LibraryName = lib.Name
			items[i].LibraryPath = lib.Path
		}
		if lib, ok := resolver.DisplayLibraryForMedia(items[i]); ok {
			items[i].DisplayLibraryID = lib.ID
			items[i].DisplayLibraryName = lib.Name
			items[i].DisplayLibraryPath = lib.Path
		}
	}
}

func (s *MediaService) displayLibraries(ctx context.Context) ([]model.Library, error) {
	const cacheKey = "media:obj:library-metadata"
	if s.cache != nil {
		if cachedObj, ok := s.cache.GetObject(cacheKey); ok {
			if cached, ok := cachedObj.([]model.Library); ok {
				return cached, nil
			}
		}
	}
	libs, err := s.repo.Library.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range libs {
		libs[i] = normalizeLocalLibraryPathForDisplay(libs[i])
	}
	if s.cache != nil {
		s.cache.SetObject(cacheKey, libs, s.mediaObjectTTL())
	}
	return libs, nil
}

type mediaDisplayLibraryResolver struct {
	displayByID   map[string]model.Library
	localDisplays []mediaLocalDisplayLibrary
}

type mediaLocalDisplayLibrary struct {
	library model.Library
	path    string
}

func newMediaDisplayLibraryResolver(libs []model.Library) mediaDisplayLibraryResolver {
	resolver := mediaDisplayLibraryResolver{
		displayByID:   make(map[string]model.Library, len(libs)),
		localDisplays: make([]mediaLocalDisplayLibrary, 0, len(libs)),
	}
	for _, lib := range libs {
		resolver.displayByID[lib.ID] = lib
		if !lib.Enabled {
			continue
		}
		displayPath := cleanPathForVolumeMapping(resolveMappedDestinationPath(lib.Path))
		if displayPath == "" || displayPath == "." {
			continue
		}
		resolver.localDisplays = append(resolver.localDisplays, mediaLocalDisplayLibrary{
			library: lib,
			path:    displayPath,
		})
	}
	// 最长路径优先：一次命中就是原逻辑中最具体的媒体库，避免每条媒体都
	// 重新规范化全部库路径并扫描整个库列表。
	sort.SliceStable(resolver.localDisplays, func(i, j int) bool {
		return len(resolver.localDisplays[i].path) > len(resolver.localDisplays[j].path)
	})
	return resolver
}

func (r mediaDisplayLibraryResolver) DisplayLibraryForMedia(media model.Media) (model.Library, bool) {
	if lib, ok := r.bestPathDisplayLibrary(media); ok {
		return lib, true
	}
	lib, ok := r.displayByID[media.LibraryID]
	return lib, ok
}

func (r mediaDisplayLibraryResolver) bestPathDisplayLibrary(media model.Media) (model.Library, bool) {
	// Retired cloud paths have no local directory match; fall back to ownership.
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(media.Path)), "cloud://") {
		return model.Library{}, false
	}

	mediaPath := cleanPathForVolumeMapping(media.Path)
	if isRelativeVolumeMarkerPath(media.Path) {
		mediaPath = cleanPathForVolumeMapping(resolveMappedDestinationPath(media.Path))
	}
	for _, display := range r.localDisplays {
		if mediaPath == display.path || strings.HasPrefix(mediaPath, strings.TrimRight(display.path, "/")+"/") {
			return display.library, true
		}
	}
	return model.Library{}, false
}

func normalizeLocalLibraryPathForDisplay(lib model.Library) model.Library {
	lib.Path = resolveMappedDestinationPath(lib.Path)
	for i := range lib.Roots {
		lib.Roots[i].Path = resolveMappedDestinationPath(lib.Roots[i].Path)
	}
	return lib
}
