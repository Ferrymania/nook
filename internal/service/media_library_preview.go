package service

import (
	"context"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
)

func (s *MediaService) listLibrarySeriesPreviewCards(ctx context.Context, libraryID string, visibility MediaVisibility, limit int) ([]SeriesCard, error) {
	// A library detail page may already have built the canonical cards. Reuse
	// those without loading or repopulating its full episode index for previews.
	if s.cache != nil {
		if obj, ok := s.cache.GetObject(s.libraryCardsObjectKey(libraryID, visibility)); ok {
			if cached, ok := obj.(*seriesCardsCacheValue); ok {
				return latestLibraryCards(cached.Cards, limit), nil
			}
		}
	}
	libraryIDs, err := MergedLibraryIDsForLibrary(ctx, s.repo, libraryID)
	if err != nil {
		return nil, err
	}
	filter := repository.MediaQueryFilter{
		IncludeNSFW: visibility.IncludeNSFW, AllowedLibraryIDs: visibility.AllowedLibraryIDs,
		HiddenLibraryIDs: visibility.HiddenLibraryIDs,
	}
	rows, err := s.repo.Media.ListSeriesPreviewRows(ctx, libraryIDs, filter)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []SeriesCard{}, nil
	}
	// Resolve display-library metadata without retaining another Media slice.
	// Like attachLibraryMetadata, failure to load display libraries leaves the
	// original library identity available for grouping and links.
	libs, _ := s.displayLibraries(ctx)
	resolver := newMediaDisplayLibraryResolver(ctx, s.repo, libs)
	mediaAt := func(i int) model.Media {
		m := seriesPreviewMedia(rows[i])
		resolver.attachMetadata(&m)
		return m
	}
	_, keys := resolveMediaSeriesKeysFrom(len(rows), mediaAt)
	cards := latestLibraryCards(groupMediaSeriesCardsFromKeys(len(rows), mediaAt, keys), limit)
	ids := make([]string, 0, len(cards)*2)
	seen := make(map[string]bool, len(cards)*2)
	for _, card := range cards {
		for _, id := range []string{card.Rep.ID, card.LinkMedia.ID} {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	full, err := s.repo.Media.ListPreviewMediaByIDs(ctx, libraryIDs, ids, filter)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]model.Media, len(full))
	for i := range full {
		resolver.attachMetadata(&full[i])
		byID[full[i].ID] = full[i]
	}
	out := cards[:0]
	for _, card := range cards {
		rep, ok := byID[card.Rep.ID]
		if !ok {
			continue // Removed or no longer visible since the grouping query.
		}
		link, ok := byID[card.LinkMedia.ID]
		if !ok {
			link = rep
		}
		card.Rep, card.LinkMedia = rep, link
		out = append(out, card)
	}
	s.attachSeriesMetadataToCards(ctx, out)
	return out, nil
}

func seriesPreviewMedia(row repository.MediaSeriesPreviewRow) model.Media {
	return model.Media{
		Base:      model.Base{ID: row.ID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt},
		LibraryID: row.LibraryID, SeriesID: row.SeriesID, Title: row.Title,
		OriginalName: row.OriginalName, Path: row.Path,
		PosterURL: row.PosterURL, BackdropURL: row.BackdropURL,
		SeasonNum: row.SeasonNum, EpisodeNum: row.EpisodeNum, ScrapeStatus: row.ScrapeStatus,
		TMDbID: row.TMDbID, BangumiID: row.BangumiID, DoubanID: row.DoubanID, TheTVDBID: row.TheTVDBID,
		Year: row.Year, ReleaseDate: row.ReleaseDate,
	}
}
