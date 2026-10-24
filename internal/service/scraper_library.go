package service

import (
	"context"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/model"
)

// lookup runs the provider chain after local NFO has been considered:
// TMDb -> Douban -> Bangumi -> TheTVDB. Douban and Bangumi do not require API
// keys; providers that are unavailable or return an error are skipped.
//
// Results are cached per (kind, theatrical, query, year, season) because every
// episode of a show produces the same candidate; bypassCache forces a fresh
// provider round-trip for user-triggered rematches but still refreshes the
// cache so later episodes reuse the corrected result.
func (s *ScraperService) lookup(ctx context.Context, lib *model.Library, media *model.Media, query string, year int, bypassCache bool, failure *scrapeLookupFailure) *Match {
	if _, bound := ctx.Value(libraryScrapeContextKey{}).(model.LibraryScrapeSettings); !bound || (lib != nil && lib.ID != "") {
		ctx = withLibraryScrapeSettings(ctx, libraryScrapeSettings(ctx, s.repo, lib))
	}
	kind := ""
	if lib != nil {
		kind = lib.Type
	}
	// An explicit movie lookup is used to repair filenames whose release year
	// was previously polluted into S01E20x. Do not let the dirty path override
	// that caller-supplied media type; TV/anime libraries still force TV below.
	explicitEpisode := media != nil && (media.SeasonNum > 0 || media.EpisodeNum > 0)
	isTheatrical := mediaLooksLikeTheatricalFeature(media)
	if isTheatrical {
		kind = "movie"
	} else if (normalizeOrganizeMediaType(kind) != "movie" || explicitEpisode) && mediaIsEpisodic(media, lib) {
		kind = "tv"
	}

	season := 0
	if media != nil {
		season = media.SeasonNum
	}
	cacheKey := s.lookupSettingsCacheKey(ctx, kind, query, year, isTheatrical, season)
	if !bypassCache {
		if cached, ok := s.lookupCache.get(cacheKey, failure); ok {
			return cached
		}
	}
	currentFailure := scrapeLookupFailure{}
	match := s.lookupUncached(ctx, kind, isTheatrical, query, year, season, &currentFailure)
	failure.merge(currentFailure)
	// Always refresh the cache, even on bypassed (forced) lookups, so a
	// corrected rematch replaces the stale entry for later episodes.
	s.lookupCache.set(cacheKey, match, &currentFailure)
	return match
}

func (s *ScraperService) lookupSettingsCacheKey(ctx context.Context, kind, query string, year int, theatrical bool, season int) string {
	key := scrapeLookupCacheKey(kind, query, year, theatrical) + "|language:" + tmdbMetadataLanguage(ctx) + "|providers:" + strings.Join(scrapeProviders(ctx), ",")
	for _, p := range scrapeProviders(ctx) {
		cfg := s.scrapeProviderConfig(ctx, p)
		key += "|" + p + ":" + strconv.FormatBool(cfg.Enabled) + ":" + strconv.FormatBool(cfg.APIKey != "") + ":" + cfg.BaseURL
	}
	if season > 0 {
		key += "|season:" + strconv.Itoa(season)
	}
	return key
}

func (s *ScraperService) lookupUncached(ctx context.Context, kind string, isTheatrical bool, query string, year, season int, failure *scrapeLookupFailure) *Match {
	if failure == nil {
		failure = &scrapeLookupFailure{}
	}
	for _, provider := range scrapeProviders(ctx) {
		var match *Match
		var err error
		switch provider {
		case "tmdb":
			if s.tmdb == nil || !s.tmdb.Enabled() {
				continue
			}
			match = s.lookupAutomaticTMDb(ctx, kind, query, year, season, failure)
			if match == nil && !failure.ambiguous && isTheatrical {
				match = s.lookupAutomaticTMDb(ctx, "tv", query, year, 0, failure)
			}
		case "douban":
			if s.douban == nil || !s.douban.Enabled() {
				continue
			}
			match, err = s.douban.SearchMatch(ctx, query)
			if !metadataMatchCompatibleWithTheatrical(kind, isTheatrical, match) {
				match = nil
			}
		case "bangumi":
			if s.bangumi == nil || !s.bangumi.Enabled() {
				continue
			}
			match, err = s.bangumi.Search(ctx, query)
			if !metadataMatchCompatibleWithTheatrical(kind, isTheatrical, match) {
				match = nil
			}
		case "thetvdb":
			if !isTVMetadataKind(kind) || s.thetvdb == nil || !s.thetvdb.Enabled() {
				continue
			}
			match, err = s.thetvdb.SearchSeries(ctx, query)
			if !metadataMatchCompatibleWithType(kind, match) {
				match = nil
			}
		default:
			continue // Fanart augments artwork only.
		}
		if failure.ambiguous {
			return nil
		}
		if err != nil {
			failure.requestFailed = true
			s.log.Debug("metadata provider search failed", zap.String("provider", provider), zap.Error(redactSensitiveError(err)))
			continue
		}
		if match != nil {
			match.Provider = provider
			return match
		}
	}
	return nil
}

func isTVMetadataKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "anime", "tv", "variety", "show", "shows":
		return true
	default:
		return false
	}
}

// EnrichLibrary runs the provider chain for every pending media in a library.
// When retryNoMatch is true it also retries rows previously marked no_match,
// which is the expected behaviour for a manual "重新刮削" action. Scanner-driven
// automatic enrichment keeps the default false path to avoid repeated scraping.
//
// Pending status includes both the canonical "pending" string and the
// empty / NULL values, because MediaRepository.Upsert can wipe the GORM
// default when re-running a scan over an already-existing row.
func (s *ScraperService) EnrichLibrary(ctx context.Context, libraryID string, retryNoMatch ...bool) (int, error) {
	result, err := s.EnrichLibraryDetailed(ctx, libraryID, retryNoMatch...)
	return result.Matched, err
}

type EnrichLibraryResult struct {
	LibraryID  string
	Matched    int
	Processed  int
	Failed     int
	Candidates int
}

func (s *ScraperService) EnrichLibraryDetailed(ctx context.Context, libraryID string, retryNoMatch ...bool) (EnrichLibraryResult, error) {
	options := ScrapeOptions{}
	if len(retryNoMatch) > 0 {
		options.RetryNoMatch = retryNoMatch[0]
	}
	return s.EnrichLibraryDetailedWithOptions(ctx, libraryID, options)
}

func (s *ScraperService) EnrichLibraryDetailedWithOptions(ctx context.Context, libraryID string, options ScrapeOptions) (EnrichLibraryResult, error) {
	result := EnrichLibraryResult{LibraryID: libraryID}
	// A manual retry must not reuse stale negative cache entries from the
	// previous run, otherwise "重新刮削" looks like it did nothing. Positives
	// are kept so the rest of the season still dedupes; the first re-queried
	// episode refreshes the entry for the rows behind it.
	if options.RetryNoMatch && s != nil {
		s.lookupCache.clearNegatives()
	}
	rows, err := s.scrapeCandidateRows(ctx, libraryID, options)
	if err != nil {
		return result, err
	}
	result.Candidates = len(rows)
	runOptions := options
	runOptions.DeferEpisodeDetails = true
	for i := range rows {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}
		if err := s.EnrichOneWithOptions(ctx, &rows[i], runOptions); err != nil {
			s.log.Warn("enrich failed", zap.String("media", rows[i].ID), zap.Error(err))
			result.Failed++
			continue
		}
		result.Processed++
		if s.mediaIsMatched(ctx, rows[i].ID) {
			result.Matched++
		}
	}
	if err := s.enrichDeferredEpisodeDetails(ctx, rows, options); err != nil {
		return result, err
	}
	s.hub.Publish("scrape", map[string]any{
		"library_id": libraryID,
		"finished":   true,
		"matched":    result.Matched,
		"processed":  result.Processed,
		"failed":     result.Failed,
		"candidates": result.Candidates,
	})
	return result, nil
}

func (s *ScraperService) scrapeCandidateRows(ctx context.Context, libraryID string, options ScrapeOptions) ([]model.Media, error) {
	var rows []model.Media
	statusFilter := "scrape_status IS NULL OR scrape_status = '' OR scrape_status = ?"
	statusArgs := []any{"pending"}
	if options.RetryNoMatch {
		statusFilter += " OR scrape_status = ?"
		statusArgs = append(statusArgs, "no_match")
	}
	if options.IncludeMatched || options.RefreshWeakMatched {
		statusFilter += " OR scrape_status = ?"
		statusArgs = append(statusArgs, "matched")
	}
	q := s.repo.DB.WithContext(ctx).Where(statusFilter, statusArgs...)
	if strings.TrimSpace(libraryID) != "" {
		q = q.Where("library_id IN ?", []string{libraryID})
	}
	if err := q.
		Order("CASE WHEN COALESCE(season_num, 0) > 0 OR COALESCE(episode_num, 0) > 0 THEN 1 ELSE 0 END").
		Order("id ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	if options.RefreshWeakMatched && !options.IncludeMatched {
		rows = filterWeakMatchedScrapeRows(rows)
	}
	return rows, nil
}

func filterWeakMatchedScrapeRows(rows []model.Media) []model.Media {
	out := rows[:0]
	for _, row := range rows {
		if shouldScrapeCandidateRow(row) {
			out = append(out, row)
		}
	}
	return out
}

func shouldScrapeCandidateRow(media model.Media) bool {
	if strings.TrimSpace(media.ScrapeStatus) != "matched" {
		return true
	}
	return organizeMediaTitleLooksLikeRelease(media.Title)
}

func (s *ScraperService) scrapeDelay(ctx context.Context) time.Duration {
	var minMS, maxMS int
	if settings, ok := ctx.Value(libraryScrapeContextKey{}).(model.LibraryScrapeSettings); ok {
		minMS = settings.DelayMinMS
		maxMS = settings.DelayMaxMS
	} else {
		minMS = s.scrapeDelaySetting(ctx, "scrape.delay_min_ms", defaultScrapeDelayMinMS)
		maxMS = s.scrapeDelaySetting(ctx, "scrape.delay_max_ms", defaultScrapeDelayMaxMS)
	}
	if minMS < 0 {
		minMS = 0
	}
	if maxMS < 0 {
		maxMS = 0
	}
	if minMS > maxScrapeDelayMS {
		minMS = maxScrapeDelayMS
	}
	if maxMS > maxScrapeDelayMS {
		maxMS = maxScrapeDelayMS
	}
	if maxMS < minMS {
		maxMS = minMS
	}
	if maxMS == 0 {
		return 0
	}
	if maxMS == minMS {
		return time.Duration(minMS) * time.Millisecond
	}
	return time.Duration(minMS+secureRandomIntn(maxMS-minMS+1)) * time.Millisecond
}

func (s *ScraperService) scrapeDelaySetting(ctx context.Context, key string, fallback int) int {
	if s == nil || s.repo == nil || s.repo.Setting == nil {
		return fallback
	}
	value, err := s.repo.Setting.Get(ctx, key)
	if err != nil || strings.TrimSpace(value) == "" {
		return fallback
	}
	return parseIntSettingDefault(strings.TrimSpace(value), fallback)
}

func (s *ScraperService) mediaIsMatched(ctx context.Context, mediaID string) bool {
	var status string
	err := s.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Select("scrape_status").
		Where("id = ?", mediaID).
		Scan(&status).Error
	return err == nil && status == "matched"
}
