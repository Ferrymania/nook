package service

import (
	"context"
	"strings"

	"github.com/Ferrymania/nook/internal/model"
)

type tmdbCertificates struct {
	ReleaseDates struct {
		Success *bool `json:"success"`
		Results []struct {
			Country  string `json:"iso_3166_1"`
			Releases []struct {
				Certification string `json:"certification"`
			} `json:"release_dates"`
		} `json:"results"`
	} `json:"release_dates"`
	ContentRatings struct {
		Success *bool `json:"success"`
		Results []struct {
			Country string `json:"iso_3166_1"`
			Rating  string `json:"rating"`
		} `json:"results"`
	} `json:"content_ratings"`
}

func (c tmdbCertificates) failed() bool {
	return (c.ReleaseDates.Success != nil && !*c.ReleaseDates.Success) || (c.ContentRatings.Success != nil && !*c.ContentRatings.Success)
}

func (c tmdbCertificates) rating() model.ContentRating {
	if c.failed() {
		return model.ContentRating{}
	}
	selected := model.NewContentRating("", "US", "tmdb")
	consider := func(raw, country string) {
		if strings.TrimSpace(raw) == "" {
			return
		}
		candidate := model.NewContentRating(raw, country, "tmdb")
		// Prefer US, then the most restrictive certificate of that country. Preserve
		// a foreign certificate as unknown when no supported US certificate exists.
		if selected.OfficialRating == "" || (country == "US" && selected.RatingCountry != "US") || (country == selected.RatingCountry && candidate.ContentRatingLevel > selected.ContentRatingLevel) {
			selected = candidate
		}
	}
	for _, row := range c.ReleaseDates.Results {
		for _, release := range row.Releases {
			consider(release.Certification, row.Country)
		}
	}
	for _, row := range c.ContentRatings.Results {
		consider(row.Rating, row.Country)
	}
	return selected
}

func contentRatingUpdates(r model.ContentRating) map[string]any {
	return map[string]any{"official_rating": r.OfficialRating, "rating_country": r.RatingCountry, "content_rating_level": r.ContentRatingLevel, "content_rating_source": r.ContentRatingSource, "content_rating_checked_at": r.ContentRatingCheckedAt}
}

func addContentRatingUpdates(updates map[string]any, r model.ContentRating) {
	for key, value := range contentRatingUpdates(r) {
		updates[key] = value
	}
}

// Persist the certificate collected by normal scraping. Every episode receives
// the show certificate; explicit NFO/manual ratings take precedence.
func (s *ScraperService) saveContentRating(ctx context.Context, m *model.Media, lib *model.Library, r model.ContentRating) error {
	if m == nil || r.ContentRatingCheckedAt == nil {
		return nil
	}
	q := s.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("id = ?", m.ID)
	if r.ContentRatingSource != "manual" {
		q = q.Where("content_rating_source <> ?", "manual")
	}
	if r.ContentRatingSource == "tmdb" {
		q = q.Where("content_rating_source <> ?", "nfo")
	}
	if err := q.Updates(contentRatingUpdates(r)).Error; err != nil {
		return err
	}
	if mediaIsEpisodic(m, lib) && r.ContentRatingSource == "tmdb" {
		if err := s.repo.DB.WithContext(ctx).Model(&model.Series{}).Where("library_id = ? AND tm_db_id = ?", m.LibraryID, m.TMDbID).Where("content_rating_source NOT IN ?", []string{"manual", "nfo"}).Updates(contentRatingUpdates(r)).Error; err != nil {
			return err
		}
	}
	s.invalidateMediaCache(ctx)
	if s.onSeriesMetadataChanged != nil {
		s.onSeriesMetadataChanged()
	}
	return nil
}

// Clear user-scoped visibility and payloads immediately after preferences save.
func (e *EmbyService) InvalidateUserVisibility(ctx context.Context, userID string) {
	if e == nil {
		return
	}
	e.visibilityMu.Lock()
	delete(e.visibilityCache, strings.TrimSpace(userID))
	e.visibilityMu.Unlock()
	e.invalidateSeriesMetadata()
	if e.cache != nil {
		e.cache.DeletePrefix(ctx, "media:emby:")
	}
}

type contentRatingContextKey struct{}

func WithContentRatingPolicy(ctx context.Context, p model.ContentRatingPolicy) context.Context {
	return context.WithValue(ctx, contentRatingContextKey{}, p)
}
func contentRatingPolicyFromContext(ctx context.Context) model.ContentRatingPolicy {
	p, _ := ctx.Value(contentRatingContextKey{}).(model.ContentRatingPolicy)
	return p
}
func remoteRatingAllowed(ctx context.Context, item map[string]any) bool {
	p := contentRatingPolicyFromContext(ctx)
	if !p.Restricted() || item == nil {
		return true
	}
	switch remoteItemString(item, "Type") {
	case "CollectionFolder", "Folder", "UserView", "Person", "Genre":
		return true
	}
	return p.AllowsContentRating(model.ContentRatingLevel(firstNonEmptyStr(remoteItemString(item, "OfficialRating"), remoteItemString(item, "SeriesOfficialRating")), ""))
}
