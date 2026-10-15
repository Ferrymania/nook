package service

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	metadataTrustTokenRE           = regexp.MustCompile(`[\p{L}\p{N}]+`)
	metadataTrustDanglingEpisodeRE = regexp.MustCompile(`(?i)^s\d{1,2}e$`)
)

func automaticMetadataTitleTrusted(query string, match *Match) bool {
	queryKey := metadataTrustKey(query)
	if queryKey == "" || match == nil {
		return false
	}
	titles := make([]string, 0, 2+len(match.Aliases))
	titles = append(titles, match.Title, match.OriginalName)
	titles = append(titles, match.Aliases...)
	for _, title := range titles {
		titleKey := metadataTrustKey(title)
		if titleKey == "" {
			continue
		}
		if metadataTitlesEquivalent(queryKey, titleKey) {
			return true
		}
	}
	return false
}

// A bilingual source must not be confirmed from its English name while its
// explicit Chinese name disagrees. Unknown translations stay manual.
func automaticSourceNamesTrusted(queries []string, match *Match) bool {
	hasChinese := false
	for _, query := range queries {
		if containsCJK(query) {
			hasChinese = true
			if automaticMetadataTitleTrusted(query, match) {
				return true
			}
		}
	}
	return !hasChinese
}

func metadataTitlesEquivalent(queryKey, titleKey string) bool {
	if queryKey == titleKey {
		return true
	}
	// Chinese word boundaries are not consistently spaced by providers. Keep
	// every letter and digit, and require equality rather than a fuzzy prefix.
	return containsCJK(queryKey) && containsCJK(titleKey) &&
		metadataCJKSpacingKey(queryKey) == metadataCJKSpacingKey(titleKey)
}

func metadataCJKSpacingKey(key string) string {
	runes := []rune(key)
	var out strings.Builder
	for i, r := range runes {
		if r == ' ' && ((i > 0 && containsCJK(string(runes[i-1]))) ||
			(i+1 < len(runes) && containsCJK(string(runes[i+1])))) {
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}

func metadataTrustKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	value = strings.Map(func(r rune) rune {
		if r == '\'' || r == '’' || r == '‘' {
			return -1
		}
		return r
	}, value)
	value = bracketedTag.ReplaceAllString(value, " ")
	if start, end, year := queryReleaseYear(value); year > 0 {
		value = value[:start] + " " + value[end:]
	}
	for _, re := range []*regexp.Regexp{patSEnE, patDanglingSE, patNxE, patEP, patCN, patSeasonOnly, patCNSeason} {
		value = re.ReplaceAllString(value, " ")
	}
	tokens := metadataTrustTokenRE.FindAllString(value, -1)
	out := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" || metadataTrustNoiseToken(token) {
			continue
		}
		out = append(out, token)
	}
	return strings.Join(normalizeMetadataSequelTokens(out), " ")
}

// Normalize explicit part numbers and Roman sequel tokens only. Bare title
// words (I, V, One, etc.) retain their meaning, and the number stays in the key.
func normalizeMetadataSequelTokens(tokens []string) []string {
	numbers := map[string]string{"i": "1", "ii": "2", "iii": "3", "iv": "4", "v": "5", "vi": "6", "vii": "7", "viii": "8", "ix": "9", "x": "10",
		"one": "1", "two": "2", "three": "3", "four": "4", "five": "5", "six": "6", "seven": "7", "eight": "8", "nine": "9", "ten": "10"}
	out := make([]string, 0, len(tokens))
	for i, token := range tokens {
		if (token == "part" || token == "chapter") && i+1 < len(tokens) {
			if _, ok := numbers[tokens[i+1]]; ok {
				continue
			}
			if n, err := strconv.Atoi(tokens[i+1]); err == nil && n > 0 {
				continue
			}
		}
		if n, ok := numbers[token]; ok {
			explicit := i > 0 && (tokens[i-1] == "part" || tokens[i-1] == "chapter")
			romanSequel := i > 0 && strings.Contains(" ii iii iv vi vii viii ix ", " "+token+" ") && !containsCJK(strings.Join(tokens, ""))
			if explicit || romanSequel {
				token = n
			}
		}
		out = append(out, token)
	}
	return out
}

func metadataTrustNoiseToken(token string) bool {
	if token == "x" || token == "×" {
		return true
	}
	if _, ok := noiseTokenSet[token]; ok {
		return true
	}
	if metadataTrustDanglingEpisodeRE.MatchString(token) {
		return true
	}
	return false
}

func metadataTrustLocalizedSearchKeyword(queryKey string, match *Match) bool {
	if match == nil || !metadataMatchHasExternalID(match) {
		return false
	}
	if !metadataTrustStrongCJKQuery(queryKey) {
		return false
	}
	for _, alias := range match.Aliases {
		if aliasKey := metadataTrustKey(alias); aliasKey != "" && aliasKey == queryKey {
			return true
		}
	}
	return false
}

func preferLocalizedSearchTitle(query string, match *Match) {
	if match == nil || containsCJK(match.Title) {
		return
	}
	if !metadataTrustLocalizedSearchKeyword(metadataTrustKey(query), match) {
		return
	}
	localized := strings.TrimSpace(match.SearchKeyword)
	if localized == "" {
		localized = strings.TrimSpace(query)
	}
	if localized == "" {
		return
	}
	if strings.TrimSpace(match.OriginalName) == "" {
		match.OriginalName = strings.TrimSpace(match.Title)
	}
	match.Title = localized
}

func metadataTrustStrongCJKQuery(queryKey string) bool {
	cjk := 0
	for _, r := range queryKey {
		switch {
		case r >= '\u3400' && r <= '\u4dbf':
			cjk++
		case r >= '\u4e00' && r <= '\u9fff':
			cjk++
		case r >= '\uf900' && r <= '\ufaff':
			cjk++
		}
	}
	return cjk >= 4
}

func metadataMatchHasExternalID(match *Match) bool {
	return match != nil &&
		(match.TMDbID > 0 ||
			match.BangumiID > 0 ||
			strings.TrimSpace(match.DoubanID) != "" ||
			strings.TrimSpace(match.TheTVDBID) != "")
}
