package service

import (
	"regexp"
	"strings"
)

// Release cleanup follows QMediaSync's RemovePreExlucde / PreProcess stages:
// https://github.com/qicfan/qmediasync/blob/main/internal/helpers/extract.go
// Adapted for Nook on 2026-11-03 (GPL-3.0): keep Nook's year, episode and title
// case handling, and require release evidence before removing an unknown group
// so ordinary hyphenated titles such as Spider-Man remain intact.
var (
	queryReleaseGroupRE = regexp.MustCompile(`(?i)[-@]\s*[\p{L}\p{N}]{2,32}$`)
	queryReleaseSpecRE  = regexp.MustCompile(`(?i)(?:\d{3,4}[pi]|[248]k|(?:[hx][._ -]?26[456]|hevc|avc)|blu[._ -]?ray|web[._ -]?dl)(?:$|[\s._-])`)
	queryResolutionRE   = regexp.MustCompile(`(?i)(?:(?:uhd|hd|bd)?\d{3,4}[pi]|[248]k)\+?(?:$|[^\p{L}\p{N}])`)
	querySubtitleTagsRE = regexp.MustCompile(`(?i)(?:HD)?(?:简|繁|日|英|中|粤|国)+(?:多|双)?(?:语|音轨)(?:配音|版)?|(?:HD)?(?:简|繁|日|英|中|粤|国|特效)+(?:文|语)?字幕|(?:HD)?(?:简|繁|日|英|中|粤|国)+配|无水印|杜比视界(?:版本)?|高码版|高码率`)
)

func cleanQueryReleaseName(name string) string {
	name = strings.TrimSpace(name)
	if group := queryReleaseGroupRE.FindStringIndex(name); group != nil {
		prefix := name[:group[0]]
		yearStart, _, year := queryReleaseYear(prefix)
		if queryReleaseSpecRE.MatchString(prefix) || (year > 0 && yearStart > 0) {
			name = strings.TrimSpace(prefix)
		}
	}
	// QMediaSync strips joined resolution and subtitle tags before splitting
	// the name. Keep a separated resolution token here so CleanQuery can also
	// discard the remaining release tail, including unknown Chinese group names.
	name = queryResolutionRE.ReplaceAllString(name, " $0 ")
	name = querySubtitleTagsRE.ReplaceAllString(name, " ")
	return name
}
