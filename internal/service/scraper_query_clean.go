package service

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// yearPattern extracts a 4-digit year (1900-2099).
var yearPattern = regexp.MustCompile(`(?:^|[^\d])(19\d{2}|20\d{2})(?:[^\d]|$)`)

var queryYearDigitsRE = regexp.MustCompile(`19\d{2}|20\d{2}`)
var splitCodecTokenRE = regexp.MustCompile(`(?i)\b([hx])[\s._-]+(264|265|266)\b`)

// A release year must be separate from title letters. Keep the 1994 in
// 寒战1994.2026, while accepting release names such as Rush.Hour.2001BluRay.
func queryReleaseYear(name string) (start, end, year int) {
	for _, loc := range queryYearDigitsRE.FindAllStringIndex(name, -1) {
		start, end = loc[0], loc[1]
		if start > 0 {
			before, _ := utf8.DecodeLastRuneInString(name[:start])
			if unicode.IsLetter(before) || unicode.IsDigit(before) {
				continue
			}
		}
		if end < len(name) {
			after, _ := utf8.DecodeRuneInString(name[end:])
			if unicode.IsDigit(after) {
				continue
			}
			if unicode.IsLetter(after) {
				tail := strings.ToLower(name[end:])
				if !strings.HasPrefix(tail, "bluray") && !strings.HasPrefix(tail, "web-dl") {
					continue
				}
			}
		}
		year, _ = strconv.Atoi(name[start:end])
		return start, end, year
	}
	return 0, 0, 0
}

// noiseTokens are stripped before search.
var noiseTokens = []string{
	// 视频规格
	"1080p", "2160p", "4k", "720p", "480p", "uhd", "ds4k", "fhd",
	"bd", "bdrip", "brrip", "dvd", "dvdrip", "hdtv", "pdtv", "webdl",
	"hdrip", "bluray", "blu-ray", "webrip", "web-dl", "web",
	"x264", "x265", "h264", "h265", "h266", "hevc", "avc", "av1", "vvc", "10bit", "8bit", "hi10p", "hi10",
	"hdr", "hdr10", "sdr", "dts", "ddp", "ddp5", "dd5", "dd2", "eac3", "truehd",
	"dovi", "atmos", "aac", "ac3", "flac", "fps", "hlg", "dv",
	"remux", "extended", "uncut", "remastered", "repack", "proper", "internal",
	"limited", "imax", "directors-cut", "directors_cut",
	"hkfree", "yify", "rarbg", "ettv", "fgt", "tgx", "ctrlhd", "ntb", "flux", "qhstudio",

	// 流媒体平台 / 字幕组 / 国家版本（动漫常见）
	// 注意：不要把同时是常见英文单词的标记放进来（如 max / web / judas），
	// 否则 "Mad Max"、"Web Therapy" 这类正常标题会被误删。裸 "WEB" 发布标记
	// 通常位于分辨率之后，由 releaseBoundary 截断规则处理；"WEB-DL" 则由
	// multiWordNoise 单独匹配。
	"netflix", "nf", "amzn", "hulu", "disney", "hbo",
	"linetv", "ourtv", "iqiyi", "youku", "bilibili", "qiyi", "krj",
	"atvp", "appletv", "apple-tv", "tx", "txweb",
	"crunchyroll", "funimation", "anidb", "horriblesubs", "subsplease",
	"erai-raws", "asw", "smcat", "leopard-raws", "ohys-raws", "colortv",
	"mweb", "ubweb", "hhweb", "adweb", "chdweb", "kurosawa", "qhstudio",

	// 中文字幕标记
	"zm", "zw", "ch", "chs", "cht", "cn", "tc", "sc",
	"中字", "繁字", "简中", "繁中", "国语", "粤语", "日语", "国语对白", "粤语对白",

	// 季数前缀残留 — ParseEpisode 已抽取过
	"season", "264", "265", "aac2", "aac5",
}

var noiseTokenSet = func() map[string]struct{} {
	set := make(map[string]struct{}, len(noiseTokens)+1)
	for _, token := range noiseTokens {
		set[token] = struct{}{}
	}
	set["dl"] = struct{}{}
	return set
}()

var releaseBoundaryTokenSet = map[string]struct{}{
	"1080p": {}, "2160p": {}, "4k": {}, "720p": {}, "480p": {}, "uhd": {}, "fhd": {},
	"bd": {}, "bdrip": {}, "brrip": {}, "dvd": {}, "dvdrip": {}, "hdtv": {}, "pdtv": {},
	"webdl": {}, "hdrip": {}, "bluray": {}, "webrip": {}, "web": {}, "remux": {},
	"x264": {}, "x265": {}, "h264": {}, "h265": {}, "h266": {}, "hevc": {}, "avc": {}, "av1": {}, "vvc": {},
}

// weakReleaseBoundaryTokenSet are release tags that double as plausible title
// words. Before any release signal has been seen they are kept as part of the
// title ("Mad Max", "Web Therapy"), so a title is never truncated to nothing;
// after a real signal they behave like any other tag.
var weakReleaseBoundaryTokenSet = map[string]struct{}{
	"bd": {}, "dvd": {}, "web": {},
}

// releaseSignalToken marks the position of an extracted year. The year itself
// is not a title token, but its presence still proves the following tokens are
// release tags ("复仇者联盟4.2019.BD.1080p" must drop "BD"). A control rune is
// used so it can never collide with a real filename token.
const releaseSignalToken = "\u0001"
const episodeSignalToken = "\u0002"

var dynamicReleaseBoundaryTokenRE = regexp.MustCompile(`(?i)^(?:(?:hd|bd|uhd)?\d{3,4}[pi]|\d{2,3}fps)$`)
var releaseVersionTokenRE = regexp.MustCompile(`(?i)^v\d+$`)

var releaseLanguageTokenSet = map[string]struct{}{
	"ita": {}, "eng": {}, "jpn": {}, "kor": {}, "fra": {}, "ger": {}, "rus": {}, "spa": {},
	"usa": {}, "cc": {},
}

// bracketedTag matches "[anything]", "(anything)" or "{anything}" segments.
var bracketedTag = regexp.MustCompile(`[\[\(\{][^\]\)\}]*[\]\)\}]`)
var multiWordNoise = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bweb[\s._-]*dl\b`),
	regexp.MustCompile(`(?i)\bblu[\s._-]*ray\b`),
	regexp.MustCompile(`(?i)\bdirectors[\s._-]*cut\b`),
	regexp.MustCompile(`(?i)\berai[\s._-]*raws\b`),
	regexp.MustCompile(`(?i)\bohys[\s._-]*raws\b`),
}

// CleanQuery converts a filename like "Inception.2010.1080p.BluRay.x264.mkv"
// into a TMDb-friendly title plus an optional year hint.
func CleanQuery(raw string) (title string, year int) {
	base := pathBaseSlash(raw)
	if base == "" {
		base = strings.TrimSpace(raw)
	}
	name := strings.TrimSuffix(base, filepath.Ext(base))
	// .strm 可能保留视频扩展名（name.mkv.strm），再剥一层真实视频扩展，
	// 避免「竞女01.mkv」与「竞女01.mp4」被当成不同标题。
	name = mediaFileStem(base)
	if name == "" {
		name = strings.TrimSuffix(base, filepath.Ext(base))
	}
	lower := strings.ToLower(name)
	_, episode := ParseEpisode(base)

	if start, end, v := queryReleaseYear(lower); v > 0 {
		year = v
		lower = lower[:start] + " " + releaseSignalToken + " " + lower[end:]
	}

	lower = bracketedTag.ReplaceAllString(lower, " ")

	lower = patSEnE.ReplaceAllString(lower, " "+episodeSignalToken+" ")
	lower = patDanglingSE.ReplaceAllString(lower, " ")
	lower = patNxE.ReplaceAllString(lower, " "+episodeSignalToken+" ")
	lower = patEP.ReplaceAllString(lower, " "+episodeSignalToken+" ")
	lower = patCN.ReplaceAllString(lower, " "+episodeSignalToken+" ")
	// 去掉中文季/部标记（如「第二季」「第2部」），避免残留在标题里既污染
	// 搜索查询又导致整理后的目录名重复季信息。
	lower = patSeasonOnly.ReplaceAllString(lower, " ")
	lower = patCNSeason.ReplaceAllString(lower, " ")

	for _, pat := range multiWordNoise {
		lower = pat.ReplaceAllString(lower, " ")
	}
	lower = splitCodecTokenRE.ReplaceAllString(lower, "${1}${2}")
	for _, sep := range []string{".", "_", "-", "[", "]", "(", ")", "×"} {
		lower = strings.ReplaceAll(lower, sep, " ")
	}
	// 保留标题里的续集数字；集号之后的单个数字仍作为发布碎片丢弃。
	// 保留电影片名里的单个字母（Project A、X）；剧集沿用原有碎片清洗。
	//
	// seenReleaseBoundary 只有在遇到分辨率/编码等强标记后才置位，置位后其余
	// ASCII 词一律视为发布尾巴丢弃；releaseSignalled 则宽松得多，年份标记也会
	// 置位，它只用来武装弱标记（bd/dvd/web）。这样 "Web Therapy" 不会被截空，
	// 而 "2019.Avatar.1080p" 这种年份前置的标题也不会因为年份标记把后面的
	// 真实标题词当成尾巴丢掉。
	out := make([]string, 0, 8)
	seenReleaseBoundary := false
	seenEpisodeBoundary := false
	releaseSignalled := false
	for _, w := range strings.Fields(lower) {
		if w == episodeSignalToken {
			seenEpisodeBoundary = true
			continue
		}
		if w == releaseSignalToken {
			releaseSignalled = true
			continue
		}
		if _, languageTag := releaseLanguageTokenSet[w]; languageTag && releaseSignalled && len(out) > 0 {
			continue
		}
		if releaseSignalled && len(out) > 0 && releaseVersionTokenRE.MatchString(w) {
			continue
		}
		if dynamicReleaseBoundaryTokenRE.MatchString(w) {
			releaseSignalled = true
			seenReleaseBoundary = true
			continue
		}
		if _, boundary := releaseBoundaryTokenSet[w]; boundary {
			if _, weak := weakReleaseBoundaryTokenSet[w]; weak && !releaseSignalled {
				// Ambiguous tag that is also a plausible title word; keep it
				// until a real release signal proves we are past the title.
			} else {
				releaseSignalled = true
				seenReleaseBoundary = true
				continue
			}
		} else if _, ok := noiseTokenSet[w]; ok {
			continue
		}
		if seenReleaseBoundary && isASCIIWord(w) {
			continue
		}
		if len(w) == 1 && w[0] < 128 && (seenEpisodeBoundary || (episode > 0 && (w[0] < '0' || w[0] > '9'))) {
			continue
		}
		out = append(out, w)
	}
	title = strings.TrimSpace(strings.Join(restoreQueryTokenCase(name, out), " "))
	return title, year
}

// queryTokenSeparators mirrors the separator split used while tokenising a
// cleaned filename.
var queryTokenSeparators = []string{".", "_", "-", "[", "]", "(", ")", "×"}

// restoreQueryTokenCase maps the surviving lowercase tokens of a cleaned
// filename back onto the spelling used in the original name.
//
// Tokenisation itself stays case-insensitive: the release-tag rules in this file
// rely on lowercased tokens (and patEP's [^a-z] boundary would misfire on
// "SE7EN"-style names if it ran on mixed case). Rebuilding the display title from
// the original tokens afterwards keeps "The.Matrix.1999.1080p..." as "The Matrix"
// instead of overwriting the user's media with a lowercased title — the lowercased
// form also used to poison metadata lookups. Tokens with no original counterpart
// (fragments produced by a removal) fall back to their lowercase form.
func restoreQueryTokenCase(name string, tokens []string) []string {
	if len(tokens) == 0 {
		return tokens
	}
	originalByLower := make(map[string]string, len(tokens))
	normalized := bracketedTag.ReplaceAllString(name, " ")
	for _, sep := range queryTokenSeparators {
		normalized = strings.ReplaceAll(normalized, sep, " ")
	}
	for _, token := range strings.Fields(normalized) {
		key := strings.ToLower(token)
		if key == "" || key == releaseSignalToken {
			continue
		}
		if _, exists := originalByLower[key]; !exists {
			originalByLower[key] = token
		}
	}
	cased := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if original, ok := originalByLower[token]; ok {
			cased = append(cased, original)
			continue
		}
		cased = append(cased, token)
	}
	return cased
}

func isASCIIWord(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r >= 128 {
			return false
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}
