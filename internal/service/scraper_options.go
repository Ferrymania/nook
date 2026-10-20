package service

import "context"

type ScrapeOptions struct {
	RetryNoMatch        bool
	IncludeMatched      bool
	RefreshWeakMatched  bool
	EpisodeArtwork      *bool
	DeferEpisodeDetails bool
	ForceRematch        bool
	RebuildIdentity     bool
	resultProvider      *string
	resultFailure       *string
}

func (o ScrapeOptions) episodeArtworkEnabled() bool {
	return o.EpisodeArtwork == nil || *o.EpisodeArtwork
}

// Resolve omitted options once when a task is queued. An explicit false must
// survive persistence, and later settings changes must not alter queued work.
func (s *ScraperService) queuedScrapeOptions(ctx context.Context, options ScrapeOptions) (ScrapeOptions, error) {
	if options.EpisodeArtwork != nil {
		return options, nil
	}
	value, err := s.repo.Setting.Get(ctx, "scrape.episode_images")
	if err != nil {
		return options, err
	}
	enabled := parseBoolSetting(value, false)
	options.EpisodeArtwork = &enabled
	return options, nil
}

func skipEpisodeArtworkOptions(retryNoMatch bool) ScrapeOptions {
	episodeArtwork := false
	return ScrapeOptions{RetryNoMatch: retryNoMatch, EpisodeArtwork: &episodeArtwork}
}

func (o ScrapeOptions) recordProvider(provider string) {
	if o.resultProvider != nil {
		*o.resultProvider = provider
	}
}
