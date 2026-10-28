// Package genre normalizes known genre aliases for display and matching only.
// Stored metadata and source NFO files keep their original values.
package genre

import "strings"

// The movie and TV genre lists follow TMDB's taxonomy. Compound TV genres stay
// separate from their movie counterparts; unknown/custom names are preserved.
// https://developer.themoviedb.org/reference/genre-movie-list
// https://developer.themoviedb.org/reference/genre-tv-list
var groups = [][]string{
	{"动作", "Action"},
	{"冒险", "Adventure"},
	{"动画", "Animation"},
	{"喜剧", "Comedy"},
	{"犯罪", "Crime"},
	{"纪录片", "Documentary", "纪录"},
	{"剧情", "Drama"},
	{"家庭", "Family"},
	{"奇幻", "Fantasy"},
	{"历史", "History"},
	{"恐怖", "Horror"},
	{"音乐", "Music"},
	{"悬疑", "Mystery"},
	{"爱情", "Romance"},
	{"科幻", "Science Fiction"},
	{"电视电影", "TV Movie"},
	{"惊悚", "Thriller"},
	{"战争", "War"},
	{"西部", "Western"},
	{"动作冒险", "Action & Adventure"},
	{"儿童", "Kids"},
	{"新闻", "News"},
	{"真人秀", "Reality"},
	{"科幻与奇幻", "Sci-Fi & Fantasy"},
	{"肥皂剧", "Soap"},
	{"脱口秀", "Talk"},
	{"战争与政治", "War & Politics"},
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
