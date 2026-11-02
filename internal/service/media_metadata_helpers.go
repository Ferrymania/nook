package service

import "strings"

func normalizeMetadataMediaType(mediaType string) string {
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "movie", "film":
		return "movie"
	case "tv", "series", "show", "drama":
		return "tv"
	case "anime", "animation":
		return "anime"
	case "variety":
		return "variety"
	case "adult", "nsfw":
		return "adult"
	default:
		return ""
	}
}

func isWeakMediaFileTitle(title string) bool {
	title = strings.TrimSpace(title)
	if title == "" {
		return true
	}
	fields := strings.Fields(strings.ToLower(title))
	if len(fields) == 0 {
		return true
	}
	meaningful := 0
	for _, field := range fields {
		if _, ok := noiseTokenSet[field]; ok {
			continue
		}
		if _, ok := releaseBoundaryTokenSet[field]; ok {
			continue
		}
		if len(field) == 4 && strings.HasPrefix(field, "20") {
			continue
		}
		meaningful++
	}
	return meaningful == 0
}

func mediaTitleLooksLikeRelease(title string) bool {
	title = strings.TrimSpace(title)
	if title == "" || isWeakMediaFileTitle(title) {
		return true
	}
	if season, episode := ParseEpisode(title); season > 0 || episode > 0 {
		return true
	}
	normalized := strings.ToLower(strings.NewReplacer(".", " ", "_", " ", "-", " ").Replace(title))
	for _, field := range strings.Fields(normalized) {
		if _, ok := releaseBoundaryTokenSet[field]; ok {
			return true
		}
	}
	return false
}

func metadataMatchTrusted(query string, sourceYear int, match *Match) bool {
	if match == nil || strings.TrimSpace(match.Title) == "" {
		return false
	}
	if unsafeAutomaticEpisodeQuery(query) {
		return false
	}
	matchYear := match.Year
	if match.seasonYear > 0 {
		matchYear = match.seasonYear
	}
	if sourceYear > 0 && matchYear > 0 {
		diff := sourceYear - matchYear
		if diff < 0 {
			diff = -diff
		}
		if diff > 1 {
			return false
		}
	}
	return automaticMetadataTitleTrusted(query, match)
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
