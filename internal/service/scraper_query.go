package service

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
)

var (
	episodeOnlyQueryRE       = regexp.MustCompile(`(?i)^\s*(?:e(?:p(?:isode)?)?\s*\d{1,3}|episode\s*\d{1,3}|第\s*[0-9一二三四五六七八九十百零两]+\s*[集期话話](?:\s*[上下])?)\s*$`)
	episodeTitleQueryRE      = regexp.MustCompile(`^\s*第\s*[0-9一二三四五六七八九十百零两]+\s*[集期话話](?:\s*[上下])?\s*[:：].+`)
	genericEpisodeWordsRE    = regexp.MustCompile(`^\s*第\s*[集期话話]\s*$`)
	episodeReleaseTitleTagRE = regexp.MustCompile(`(?i)(?:^|[\s._-])s\d{1,2}e\d{1,3}(?:[\s._-]|$)`)
	patTheatricalTitle       = regexp.MustCompile(`(?i)(?:剧场版|劇場版|动画电影|動畫電影|电影版|電影版|\bthe\s+movie\b|\bmovie\s*\d{1,2}\b)`)
	patTheatricalFolder      = regexp.MustCompile(`(?i)[\\/][^\\/]*(?:剧场版|劇場版|動畫電影|动画电影|电影版|電影版)[^\\/]*[\\/]`)
	theatricalNoiseRE        = regexp.MustCompile(`(?i)(?:剧场版|劇場版)\s*(?:第?\s*\d{1,3}\s*[部篇]?)?|电影版|電影版|动画电影|動畫電影`)
)

func theatricalTitleVariants(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if !theatricalNoiseRE.MatchString(raw) {
		return []string{raw}
	}
	stripped := theatricalNoiseRE.ReplaceAllString(raw, " ")
	stripped = strings.Join(strings.Fields(stripped), " ")
	if stripped != "" && !strings.EqualFold(stripped, raw) {
		return []string{stripped, raw}
	}
	return []string{raw}
}

func mediaLooksLikeTheatricalFeature(m *model.Media) bool {
	if m == nil {
		return false
	}
	// Trust an episode marker that is actually present in the path, but do not
	// trust persisted season/episode fields here. Older scans could incorrectly
	// assign those fields to a theatrical file, which would permanently prevent
	// both manual re-scraping and separation from the TV series.
	if season, ep := ParseEpisode(m.Path); season > 0 || ep > 0 {
		return false
	}
	text := m.Title + " " + pathBaseSlash(m.Path)
	return patTheatricalTitle.MatchString(text) || pathHasTheatricalFolder(m.Path)
}

func scrapeQueryCandidates(m *model.Media, lib *model.Library) []string {
	return scrapeQueryCandidatesWithNormalizer(m, lib, func(raw string) (string, int) {
		return CleanQuery(raw)
	})
}

func scrapeQueryCandidatesWithRecognition(ctx context.Context, repo *repository.Container, m *model.Media, lib *model.Library) []string {
	return scrapeQueryCandidatesWithNormalizer(m, lib, func(raw string) (string, int) {
		return CleanQueryWithRecognition(ctx, repo, raw)
	})
}

func scrapeQueryCandidatesWithNormalizer(m *model.Media, lib *model.Library, clean func(string) (string, int)) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(raw string) {
		for _, variant := range theatricalTitleVariants(raw) {
			cleaned, _ := clean(variant)
			if cleaned == "" {
				cleaned = strings.TrimSpace(variant)
			}
			for _, candidate := range titleCandidates(cleaned) {
				if unsafeAutomaticEpisodeQuery(candidate) {
					continue
				}
				key := strings.ToLower(candidate)
				if _, ok := seen[key]; ok || candidate == "" {
					continue
				}
				seen[key] = struct{}{}
				out = append(out, candidate)
			}
		}
	}

	isTheatrical := mediaLooksLikeTheatricalFeature(m)
	if isTheatrical {
		add(m.Title)
		add(m.Path)
		if lib != nil {
			seriesTitle := seriesFolderTitle(m.Path, lib.Path)
			if seriesTitle != "" {
				baseName := pathBaseSlash(m.Path)
				stem := mediaFileStem(baseName)
				if stem == "" {
					stem = strings.TrimSuffix(baseName, filepath.Ext(baseName))
				}
				cleanStem, _ := clean(stem)
				if cleanStem == "" {
					cleanStem = stem
				}
				if !strings.Contains(strings.ToLower(cleanStem), strings.ToLower(seriesTitle)) {
					add(seriesTitle + " " + cleanStem)
				}
			}
			add(mediaFolderTitle(m.Path, lib.Path))
		}
	} else {
		episodic := mediaIsEpisodic(m, lib)
		if episodic {
			if lib != nil {
				add(seriesFolderTitle(m.Path, lib.Path))
				add(mediaFolderTitle(m.Path, lib.Path))
			}
			add(m.Title)
			add(m.Path)
		} else {
			fileTitle, _ := clean(m.Path)
			if genericMovieFileTitle(fileTitle) {
				if lib != nil {
					add(mediaFolderTitle(m.Path, lib.Path))
				}
			} else {
				add(m.Path)
				// Bilingual movie releases also provide a literal English title.
				// Use it as a fallback when the Chinese distributor prefix differs.
				if containsCJK(fileTitle) {
					var latin []string
					for _, field := range strings.Fields(fileTitle) {
						if isASCIIWord(strings.ToLower(field)) {
							latin = append(latin, field)
						}
					}
					if len(latin) >= 2 {
						add(strings.Join(latin, " "))
					}
				}
			}
			if !genericMovieFileTitle(m.Title) {
				add(m.Title)
			}
			// The Chinese single-movie directory can supply a release alias.
			// Never borrow a collection/category title or a conflicting year.
			if lib != nil {
				dir := parentSlashPath(m.Path)
				base := pathBaseSlash(dir)
				if !isMediaCollectionFolder(base) && !isGenericMediaCategoryFolder(base) &&
					!isTechnicalMediaFolder(base) && !strictSeasonFolderMatched(base) && !isTheatricalFolder(base) {
					folderTitle, folderYear := clean(base + ".mkv")
					fileYear := mediaYearHint(m)
					yearDiff := folderYear - fileYear
					if containsCJK(folderTitle) && (folderYear == 0 || fileYear == 0 || (yearDiff >= -1 && yearDiff <= 1)) {
						add(folderTitle)
					}
				}
			}
		}
	}

	if len(out) == 0 {
		base := pathBaseSlash(m.Path)
		out = append(out, strings.TrimSuffix(base, filepath.Ext(base)))
	}
	return out
}

func titleCandidates(title string) []string {
	title = strings.Join(strings.Fields(strings.TrimSpace(title)), " ")
	if title == "" {
		return nil
	}
	out := make([]string, 0, 2)
	if cjk := cjkTitleOnly(title); cjk != "" {
		out = append(out, cjk)
		if cjk != title {
			return out
		}
	}
	out = append(out, title)
	return out
}

func cjkTitleOnly(title string) string {
	parts := make([]string, 0, 4)
	for _, field := range strings.Fields(title) {
		if containsCJK(field) {
			parts = append(parts, field)
		}
	}
	return strings.Join(parts, " ")
}

func containsCJK(s string) bool {
	for _, r := range s {
		switch {
		case r >= '\u3400' && r <= '\u4dbf':
			return true
		case r >= '\u4e00' && r <= '\u9fff':
			return true
		case r >= '\uf900' && r <= '\ufaff':
			return true
		}
	}
	return false
}

func mediaIsEpisodic(m *model.Media, lib *model.Library) bool {
	if m != nil && mediaLooksLikeTheatricalFeature(m) {
		return false
	}
	if m != nil && (m.SeasonNum > 0 || m.EpisodeNum > 0) {
		return true
	}
	if m != nil {
		season, episode := ParseEpisode(m.Path)
		if season > 0 || episode > 0 {
			return true
		}
	}
	return librarySupportsSeasons(lib)
}

func librarySupportsSeasons(lib *model.Library) bool {
	if lib == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(lib.Type)) {
	case "tv", "anime", "variety", "show", "shows":
		return true
	default:
		return false
	}
}

func unsafeAutomaticEpisodeQuery(query string) bool {
	query = strings.TrimSpace(query)
	if query == "" {
		return true
	}
	if episodeOnlyQueryRE.MatchString(query) || genericEpisodeWordsRE.MatchString(query) {
		return true
	}
	if episodeTitleQueryRE.MatchString(query) {
		return true
	}
	_, episode := ParseEpisode(query)
	if episode > 0 && !looksLikeSeriesReleaseTitle(query) {
		return true
	}
	return false
}

func looksLikeSeriesReleaseTitle(query string) bool {
	cleaned, _ := CleanQuery(query)
	return strings.TrimSpace(cleaned) != "" && episodeReleaseTitleTagRE.MatchString(query)
}
