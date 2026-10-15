package service

import (
	"context"
	"strings"

	"go.uber.org/zap"
)

func (s *ScraperService) lookupAutomaticTMDb(ctx context.Context, kind, query string, year, season int, failure *scrapeLookupFailure) *Match {
	if failure == nil {
		failure = &scrapeLookupFailure{}
	}
	if match := s.lookupAutomaticTMDbPrimary(ctx, kind, query, year, season, failure); match != nil {
		return match
	}
	if failure.ambiguous {
		return nil
	}
	fallbackKind := ""
	normalized := normalizeOrganizeMediaType(kind)
	if normalized == "anime" {
		if isTVMetadataKind(kind) {
			fallbackKind = "movie"
		} else {
			fallbackKind = "tv"
		}
	}
	if fallbackKind != "" {
		if fbMatch := s.lookupAutomaticTMDbPrimary(ctx, fallbackKind, query, year, season, failure); fbMatch != nil {
			return fbMatch
		}
	}
	return nil
}

func (s *ScraperService) lookupAutomaticTMDbPrimary(ctx context.Context, kind, query string, year, season int, failure *scrapeLookupFailure) *Match {
	// One unfiltered retry is enough to find adjacent-year releases. Always
	// validate against the original year, including after alias hydration.
	searchYears := []int{year}
	if isTVMetadataKind(kind) && season > 1 {
		searchYears = []int{0}
	} else if year > 0 {
		searchYears = append(searchYears, 0)
	}
	detailsChecked := map[int]bool{}
	for _, searchYear := range searchYears {
		match, hadCandidates := s.lookupAutomaticTMDbCandidates(ctx, kind, query, searchYear, year, season, failure, detailsChecked)
		if match != nil {
			if isTVMetadataKind(kind) && season > 1 && year > 0 {
				seasonYear, err := s.tmdb.GetTVSeasonYear(ctx, match.TMDbID, season)
				if err != nil {
					failure.requestFailed = true
					return nil
				}
				if seasonYear == 0 || seasonYear != year {
					failure.yearMismatch = true
					return nil
				}
				match.seasonYear = seasonYear
			}
			return s.localizeAutomaticTMDbMatch(ctx, kind, query, match)
		}
		if failure.ambiguous || !hadCandidates || ctx.Err() != nil {
			break
		}
	}
	return nil
}

func (s *ScraperService) lookupAutomaticTMDbCandidates(ctx context.Context, kind, query string, searchYear, sourceYear, season int, failure *scrapeLookupFailure, detailsChecked map[int]bool) (*Match, bool) {
	validationYear := sourceYear
	if isTVMetadataKind(kind) && season > 1 {
		validationYear = 0
	}
	var (
		candidates []*Match
		err        error
		mediaLabel string
	)
	if isTVMetadataKind(kind) {
		mediaLabel = "tv"
		candidates, err = s.tmdb.SearchTVCandidates(ctx, query, searchYear)
	} else {
		mediaLabel = "movie"
		candidates, err = s.tmdb.SearchMovieCandidates(ctx, query, searchYear)
	}
	if err != nil {
		failure.requestFailed = true
		s.log.Debug("tmdb "+mediaLabel+" search failed", zap.String("query", query), zap.Error(err))
		return nil, false
	}
	if match := uniqueAutomaticMetadataMatch(query, validationYear, kind, candidates, failure); match != nil {
		return match, true
	}
	if failure.ambiguous {
		return nil, true
	}
	if queryNeedsEnglishTMDbFallback(query) {
		var alternate []*Match
		if isTVMetadataKind(kind) {
			alternate, err = s.tmdb.searchTVCandidates(ctx, query, searchYear, "en-US")
		} else {
			alternate, err = s.tmdb.searchMovieCandidates(ctx, query, searchYear, "en-US")
		}
		if err != nil {
			failure.requestFailed = true
			s.log.Debug("tmdb "+mediaLabel+" alternate-language search failed", zap.String("query", query), zap.Error(err))
		} else {
			candidates = mergeTMDbLanguageCandidates(candidates, alternate)
			if match := uniqueAutomaticMetadataMatch(query, validationYear, kind, candidates, failure); match != nil {
				return match, true
			}
			if failure.ambiguous {
				return nil, true
			}
		}
	}
	// A failed movie search may need a provider-returned translation/alias.
	// Bound extra detail requests to three unique IDs across both year passes.
	if !isTVMetadataKind(kind) {
		for _, candidate := range candidates {
			if candidate == nil || candidate.TMDbID <= 0 || detailsChecked[candidate.TMDbID] ||
				!metadataMatchCompatibleWithType(kind, candidate) ||
				(sourceYear > 0 && (candidate.Year <= 0 || candidate.Year < sourceYear-1 || candidate.Year > sourceYear+1)) {
				continue
			}
			if len(detailsChecked) >= 3 || ctx.Err() != nil {
				break
			}
			detailsChecked[candidate.TMDbID] = true
			details, detailErr := s.tmdb.GetMovieMatch(ctx, candidate.TMDbID)
			if detailErr != nil || details == nil || details.TMDbID != candidate.TMDbID {
				continue
			}
			candidate.Aliases = appendMetadataAliases(candidate.Aliases, details.Title, details.OriginalName)
			candidate.Aliases = appendMetadataAliases(candidate.Aliases, details.Aliases...)
		}
	}
	return uniqueAutomaticMetadataMatch(query, validationYear, kind, candidates, failure), len(candidates) > 0
}

func (s *ScraperService) localizeAutomaticTMDbMatch(ctx context.Context, kind, query string, match *Match) *Match {
	if s == nil || s.tmdb == nil || match == nil || match.TMDbID <= 0 || !metadataTitleNeedsChineseLocalization(match) {
		return match
	}
	var (
		localized *Match
		err       error
	)
	if isTVMetadataKind(kind) {
		localized, err = s.tmdb.GetTVMatch(ctx, match.TMDbID)
	} else {
		localized, err = s.tmdb.GetMovieMatch(ctx, match.TMDbID)
	}
	if err != nil || localized == nil {
		if err != nil && s.log != nil {
			s.log.Debug("tmdb localized title lookup failed",
				zap.String("query", query),
				zap.Int("tmdb_id", match.TMDbID),
				zap.Error(err))
		}
		return match
	}
	mergeAutomaticTMDbLocalizedMatch(localized, match)
	localized.SearchKeyword = query
	localized.Aliases = appendMetadataAliases(localized.Aliases,
		match.Title, match.OriginalName)
	localized.Aliases = appendMetadataAliases(localized.Aliases, match.Aliases...)
	return localized
}

func mergeAutomaticTMDbLocalizedMatch(localized, search *Match) {
	if localized == nil || search == nil {
		return
	}
	localized.seasonYear = search.seasonYear
	if strings.TrimSpace(localized.OriginalName) == "" {
		localized.OriginalName = strings.TrimSpace(firstNonEmpty(search.OriginalName, search.Title))
	}
	localized.Overview = firstNonEmpty(localized.Overview, search.Overview)
	localized.PosterURL = firstNonEmpty(localized.PosterURL, search.PosterURL)
	localized.BackdropURL = firstNonEmpty(localized.BackdropURL, search.BackdropURL)
	localized.MediaType = firstNonEmpty(localized.MediaType, search.MediaType)
	if localized.Year <= 0 {
		localized.Year = search.Year
	}
	if localized.ReleaseDate == "" {
		localized.ReleaseDate = search.ReleaseDate
	}
	if localized.Rating <= 0 {
		localized.Rating = search.Rating
	}
	if len(localized.Languages) == 0 {
		localized.Languages = append([]string(nil), search.Languages...)
	}
	if len(localized.Countries) == 0 {
		localized.Countries = append([]string(nil), search.Countries...)
	}
	if len(localized.Genres) == 0 {
		localized.Genres = append([]string(nil), search.Genres...)
	}
}

func uniqueAutomaticMetadataMatch(query string, year int, expectedType string, candidates []*Match, failure *scrapeLookupFailure) *Match {
	if failure == nil {
		failure = &scrapeLookupFailure{}
	}
	var unique *Match
	for _, candidate := range candidates {
		if candidate == nil || !metadataMatchCompatibleWithType(expectedType, candidate) {
			if candidate != nil {
				failure.typeMismatch = true
			}
			continue
		}
		if !organizeMetadataMatchTrusted(query, year, candidate) {
			failure.reject(query, year, candidate)
			continue
		}
		if unique != nil && (unique.TMDbID <= 0 || unique.TMDbID != candidate.TMDbID) {
			failure.ambiguous = true
			return nil
		}
		unique = candidate
	}
	return unique
}

func metadataMatchCompatibleWithType(expectedType string, match *Match) bool {
	if match == nil {
		return false
	}
	expectedType = normalizeOrganizeMediaType(expectedType)
	matchType := normalizeOrganizeMediaType(match.MediaType)
	if expectedType == "" || matchType == "" {
		return true
	}
	switch expectedType {
	case "tv", "variety":
		return matchType == "tv" || matchType == "anime" || matchType == "variety"
	case "anime":
		return matchType == "anime" || matchType == "tv" || matchType == "movie" || matchType == "variety"
	case "movie", "adult":
		return matchType == "movie" || matchType == "adult"
	default:
		return expectedType == matchType
	}
}

func metadataMatchCompatibleWithTheatrical(expectedType string, isTheatrical bool, match *Match) bool {
	if isTheatrical &&
		normalizeOrganizeMediaType(expectedType) == "movie" &&
		match != nil &&
		normalizeOrganizeMediaType(match.MediaType) == "anime" {
		return true
	}
	return metadataMatchCompatibleWithType(expectedType, match)
}

func queryNeedsEnglishTMDbFallback(query string) bool {
	for _, r := range query {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return true
		}
	}
	return false
}

func mergeTMDbLanguageCandidates(primary, alternate []*Match) []*Match {
	if len(primary) == 0 {
		return alternate
	}
	byID := make(map[int]*Match, len(primary))
	out := append([]*Match(nil), primary...)
	for _, candidate := range primary {
		if candidate != nil && candidate.TMDbID > 0 {
			byID[candidate.TMDbID] = candidate
		}
	}
	for _, candidate := range alternate {
		if candidate == nil {
			continue
		}
		if localized := byID[candidate.TMDbID]; localized != nil {
			localized.Aliases = appendMetadataAliases(localized.Aliases,
				candidate.Title, candidate.OriginalName)
			localized.Aliases = appendMetadataAliases(localized.Aliases, candidate.Aliases...)
			continue
		}
		out = append(out, candidate)
	}
	return out
}

func appendMetadataAliases(existing []string, values ...string) []string {
	seen := make(map[string]struct{}, len(existing)+len(values))
	out := make([]string, 0, len(existing)+len(values))
	add := func(value string) {
		value = strings.TrimSpace(value)
		key := metadataTrustKey(value)
		if value == "" || key == "" {
			return
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	for _, value := range existing {
		add(value)
	}
	for _, value := range values {
		add(value)
	}
	return out
}
