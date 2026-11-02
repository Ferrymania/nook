package service

import (
	"regexp"
	"strings"
)

var (
	classifierEpisodeRE = regexp.MustCompile(`(?i)\bS\d{1,2}E\d{1,3}\b|第\s*\d+\s*[集期]|(?:^|[\s._-])E\d{1,3}(?:[\s._-]|$)`)
	classifierSeasonRE  = regexp.MustCompile(`(?i)\bS\d{1,2}\b|第\s*\d+\s*季`)
	classifierJAVCodeRE = regexp.MustCompile(`(?:^|[\s._\-/\[\]()])[A-Z]{2,6}[-_]?\d{3,5}(?:[\s._\-/\[\]()]|$)`)
	classifierMovieRE   = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(?:movies?|films?)(?:[^a-z0-9]|$)`)
	classifierTVRE      = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(?:tv|series|shows?|dramas?)(?:[^a-z0-9]|$)`)
	classifierAnimeRE   = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(?:anime|bangumi)(?:[^a-z0-9]|$)`)
)

func normalizeMediaType(mediaType, title, category string) string {
	raw := strings.ToLower(strings.TrimSpace(mediaType))
	switch raw {
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
	}
	switch {
	case containsAnyText(raw, "成人", "番号", "jav", "nsfw"):
		return "adult"
	case containsAnyText(raw, "综艺", "真人秀"):
		return "variety"
	case (containsAnyText(raw, "国漫", "日漫", "日番", "韩漫", "美漫", "欧美动漫", "其他动漫", "动漫", "动画") || classifierAnimeRE.MatchString(raw)) && !containsAnyText(raw, "动画电影", "剧场版", "劇場版"):
		return "anime"
	case containsAnyText(raw, "电视剧", "剧集", "连续剧", "短剧", "国产剧", "国剧", "大陆剧", "华语剧", "国产电视剧", "大陆电视剧", "华语电视剧", "欧美剧", "欧美电视剧", "美剧", "英剧", "日韩剧", "日韩电视剧", "日剧", "韩剧", "港剧", "台剧", "港台剧", "泰剧") || classifierTVRE.MatchString(raw):
		return "tv"
	case containsAnyText(raw, "电影", "演唱会", "剧场版", "劇場版") || classifierMovieRE.MatchString(raw):
		return "movie"
	}
	text := strings.ToLower(title + " " + category)
	switch {
	case strings.Contains(text, "adult") || strings.Contains(text, "nsfw") || strings.Contains(text, "成人") || strings.Contains(text, "番号") || strings.Contains(text, "jav") || strings.Contains(text, "9kg") || classifierJAVCodeRE.MatchString(strings.ToUpper(title+" "+category)):
		return "adult"
	case containsAnyText(text, "综艺", "真人秀", "脱口秀", "晚会", "春晚", "gala", "festival gala", "reality", "talk show"):
		return "variety"
	case containsAnyText(text, "电影", "剧场版", "劇場版") || classifierMovieRE.MatchString(text):
		return "movie"
	case classifierAnimeRE.MatchString(text) || strings.Contains(text, "动漫") || strings.Contains(text, "动画"):
		return "anime"
	case strings.Contains(text, "variety") || strings.Contains(text, "综艺") || strings.Contains(text, "真人秀"):
		return "variety"
	case classifierEpisodeRE.MatchString(text) || classifierSeasonRE.MatchString(text) || classifierTVRE.MatchString(text) || containsAnyText(text, "剧集", "电视剧", "连续剧", "短剧", "国产电视剧", "大陆电视剧", "华语电视剧", "欧美电视剧", "日韩电视剧", "美剧", "英剧", "港剧", "台剧", "港台剧", "泰剧"):
		return "tv"
	default:
		return "movie"
	}
}
