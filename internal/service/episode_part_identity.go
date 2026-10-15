package service

import (
	"regexp"
	"strings"

	"github.com/Ferrymania/nook/internal/model"
)

// Part N is ambiguous for movies, so only a series library supplies this fallback.
var patNumberedEpisodePart = regexp.MustCompile(`(?i)(?:^|[\s._-])part[\s._-]*0*([1-9]\d{0,2})(?:$|[\s._(\-])`)

func numberedPartEpisodeIdentity(path string, lib *model.Library) (season, episode int) {
	if !librarySupportsSeasons(lib) || mediaLooksLikeTheatricalFeature(&model.Media{Path: path}) {
		return 0, 0
	}
	if season, episode := ParseEpisode(path); season > 0 || episode > 0 {
		return 0, 0
	}
	match := patNumberedEpisodePart.FindStringSubmatch(mediaSidecarBase(path))
	if len(match) != 2 {
		return 0, 0
	}
	season, found := seasonFromParents(strings.ReplaceAll(path, "\\", "/"))
	if !found {
		season = 1
	}
	return season, mustAtoi(match[1])
}

func parseEpisodePartsForLibrary(path string, lib *model.Library) (season, episode int, fraction float64) {
	season, episode, fraction = ParseEpisodeParts(path)
	if season == 0 && episode == 0 {
		season, episode = numberedPartEpisodeIdentity(path, lib)
	}
	return
}

// Preserve confirmed episode identities, including NFO and manual corrections.
func fillNumberedPartEpisodeIdentity(media *model.Media, lib *model.Library) bool {
	if media == nil || media.EpisodeNum > 0 {
		return false
	}
	season, episode := numberedPartEpisodeIdentity(media.Path, lib)
	if episode <= 0 {
		return false
	}
	if media.SeasonNum <= 0 {
		media.SeasonNum = season
	}
	media.EpisodeNum = episode
	media.EpisodeFraction = 0
	return true
}
