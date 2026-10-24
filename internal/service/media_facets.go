package service

import (
	"context"
	"math/rand"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
)

// LibraryFacets 是媒体库筛选面板需要的元数据：可选类型与年份区间。
type LibraryFacets struct {
	Genres  []GenreCount `json:"genres"`
	YearMin int          `json:"year_min"`
	YearMax int          `json:"year_max"`
}

// libraryFilterFrom 组装「可见性 + 用户筛选」的最终仓储条件，
// 与列表查询保持完全一致的语义。
func libraryFilterFrom(visibility MediaVisibility, filters MediaListFilters) repository.MediaQueryFilter {
	return filters.apply(repository.MediaQueryFilter{
		IncludeNSFW:       visibility.IncludeNSFW,
		AllowedLibraryIDs: visibility.AllowedLibraryIDs,
		HiddenLibraryIDs:  visibility.HiddenLibraryIDs,
	})
}

// LibraryFacets 返回某个库的筛选项。类型统计复用 MediaDiscoveryService，
// 避免在筛选面板与 Emby /Genres 之间出现两套口径。
//
// 作用范围必须与列表查询一致：仅统计目标媒体库内用户可见的条目。
func (s *MediaService) LibraryFacets(
	ctx context.Context,
	libraryID string,
	visibility MediaVisibility,
	discovery *MediaDiscoveryService,
) (LibraryFacets, error) {
	var facets LibraryFacets
	if s == nil || s.repo == nil || s.repo.Media == nil {
		return facets, nil
	}
	scoped, ok := scopeVisibilityToLibraries(visibility, []string{libraryID})
	if !ok {
		facets.Genres = []GenreCount{}
		return facets, nil
	}

	yearMin, yearMax, err := s.repo.Media.YearRange(ctx, libraryFilterFrom(scoped, MediaListFilters{}))
	if err != nil {
		return facets, err
	}
	facets.YearMin = yearMin
	facets.YearMax = yearMax

	if discovery != nil {
		genres, err := discovery.AggregateGenres(ctx, scoped, "")
		if err != nil {
			return facets, err
		}
		facets.Genres = genres
	}
	if facets.Genres == nil {
		facets.Genres = []GenreCount{}
	}
	return facets, nil
}

// scopeVisibilityToLibraries 把可见性收窄到给定库集合。
//
// 返回 ok=false 表示「这些库与用户的可见性没有交集」——此时必须返回空结果，
// 而不是退化成不过滤，否则受限用户会看到别人的库。
func scopeVisibilityToLibraries(visibility MediaVisibility, libraryIDs []string) (MediaVisibility, bool) {
	allowed := make(map[string]struct{}, len(visibility.AllowedLibraryIDs))
	for _, id := range visibility.AllowedLibraryIDs {
		allowed[id] = struct{}{}
	}
	out := make([]string, 0, len(libraryIDs))
	for _, id := range libraryIDs {
		if len(allowed) > 0 {
			if _, ok := allowed[id]; !ok {
				continue
			}
		}
		out = append(out, id)
	}
	if len(out) == 0 {
		return visibility, false
	}
	visibility.AllowedLibraryIDs = out
	return visibility, true
}

// RandomMedia selects one visible item using count and a random offset, avoiding a full random sort.
func (s *MediaService) RandomMedia(
	ctx context.Context,
	libraryID string,
	visibility MediaVisibility,
	filters MediaListFilters,
) (*model.Media, error) {
	if s == nil || s.repo == nil || s.repo.Media == nil {
		return nil, nil
	}
	libraryIDs := []string{libraryID}
	filter := libraryFilterFrom(visibility, filters)

	_, total, err := s.repo.Media.ListByLibrariesFiltered(ctx, libraryIDs, 0, 1, filter)
	if err != nil {
		return nil, err
	}
	if total <= 0 {
		return nil, nil
	}

	// 当总量超过阈值时，收窄到按更新时间最新的前 N 条（列表排序首列是
	// release_date / updated_at，前 randomPoolCap 行即为"最新"子集），
	// 偏移量取其中随机位置，兼顾性能与覆盖面。
	const randomPoolCap = 5000
	effectiveTotal := total
	if effectiveTotal > randomPoolCap {
		effectiveTotal = randomPoolCap
	}
	offset := 0
	if effectiveTotal > 1 {
		offset = rand.Intn(int(effectiveTotal))
	}
	items, err := s.repo.Media.ListByLibrariesFilteredNoCount(ctx, libraryIDs, offset, 1, filter)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	s.attachLibraryMetadata(ctx, items)
	return &items[0], nil
}
