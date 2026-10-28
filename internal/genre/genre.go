// Package genre resolves TMDB genre IDs and normalizes known genre aliases.
// Read-time normalization leaves existing metadata and source NFO files intact.
package genre

import (
	"strconv"
	"strings"
)

// The movie and TV genre lists follow TMDB's taxonomy. Compound TV genres stay
// separate from their movie counterparts. Numeric aliases are official TMDB
// IDs, not arbitrary numbers; unknown/custom names are preserved.
// https://developer.themoviedb.org/reference/genre-movie-list
// https://developer.themoviedb.org/reference/genre-tv-list
var groups = [][]string{
	{"动作", "Action", "28"},
	{"冒险", "Adventure", "12"},
	{"动画", "Animation", "16"},
	{"喜剧", "Comedy", "35"},
	{"犯罪", "Crime", "80"},
	{"纪录片", "Documentary", "纪录", "99"},
	{"剧情", "Drama", "18"},
	{"家庭", "Family", "10751"},
	{"奇幻", "Fantasy", "14"},
	{"历史", "History", "36"},
	{"恐怖", "Horror", "27"},
	{"音乐", "Music", "10402"},
	{"悬疑", "Mystery", "9648"},
	{"爱情", "Romance", "10749"},
	{"科幻", "Science Fiction", "878"},
	{"电视电影", "TV Movie", "10770"},
	{"惊悚", "Thriller", "53"},
	{"战争", "War", "10752"},
	{"西部", "Western", "37"},
	{"动作冒险", "Action & Adventure", "10759"},
	{"儿童", "Kids", "10762"},
	{"新闻", "News", "10763"},
	{"真人秀", "Reality", "10764"},
	{"科幻与奇幻", "Sci-Fi & Fantasy", "10765"},
	{"肥皂剧", "Soap", "10766"},
	{"脱口秀", "Talk", "10767"},
	{"战争与政治", "War & Politics", "10768"},
}

var aliases = func() map[string][]string {
	out := make(map[string][]string)
	for _, group := range groups {
		for _, name := range group {
			out[strings.ToLower(name)] = group
		}
	}
	return out
}()

func Canonical(name string) string {
	name = strings.TrimSpace(name)
	if group, ok := aliases[strings.ToLower(name)]; ok {
		return group[0]
	}
	return name
}

func Key(name string) string {
	return strings.ToLower(Canonical(name))
}

// FromTMDBIDs resolves search-result IDs before they become persisted names.
// Unknown IDs have no display name yet; the existing detail lookup can fill
// those names later rather than persisting another numeric genre label.
func FromTMDBIDs(ids []int) []string {
	out := make([]string, 0, len(ids))
	seen := make(map[string]bool)
	for _, id := range ids {
		if group, ok := aliases[strconv.Itoa(id)]; ok && !seen[group[0]] {
			seen[group[0]] = true
			out = append(out, group[0])
		}
	}
	return out
}

// Aliases returns the canonical name and its known equivalent spellings.
func Aliases(name string) []string {
	name = strings.TrimSpace(name)
	if group, ok := aliases[strings.ToLower(name)]; ok {
		return append([]string(nil), group...)
	}
	if name == "" {
		return nil
	}
	return []string{name}
}

func Split(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '，' || r == '、' || r == ';' || r == '；'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if name := strings.TrimSpace(part); name != "" {
			out = append(out, name)
		}
	}
	return out
}

// Names canonicalizes and deduplicates while preserving order and custom names.
func Names(values ...string) []string {
	out := make([]string, 0)
	seen := make(map[string]bool)
	for _, value := range values {
		for _, name := range Split(value) {
			name = Canonical(name)
			key := strings.ToLower(name)
			if !seen[key] {
				seen[key] = true
				out = append(out, name)
			}
		}
	}
	return out
}

// MatchNames expands filters into lowercase whole-token spellings for SQL.
func MatchNames(values []string) []string {
	out := make([]string, 0)
	seen := make(map[string]bool)
	for _, name := range Names(values...) {
		for _, alias := range Aliases(name) {
			key := strings.ToLower(alias)
			if !seen[key] {
				seen[key] = true
				out = append(out, key)
			}
		}
	}
	return out
}
