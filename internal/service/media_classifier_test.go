package service

import "testing"

func TestNormalizeMediaTypeAcceptsChineseLibraryTypes(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"华语电影": "movie",
		"欧美剧":  "tv",
		"国产剧":  "tv",
		"日漫":   "anime",
		"国漫":   "anime",
		"综艺":   "variety",
		"成人":   "adult",
	}
	for input, want := range tests {
		if got := normalizeMediaType(input, "测试标题", ""); got != want {
			t.Fatalf("normalizeMediaType(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeMediaTypeDoesNotTreatReleaseTokensAsTV(t *testing.T) {
	t.Parallel()

	tests := []string{
		"They Will Kill You 2026 1080p HDTV x264",
		"Some Movie 2026 2160p AppleTV WEB-DL",
		"Some Movie 2026 2160p ATVP WEB-DL",
	}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			if got := normalizeMediaType("", input, ""); got != "movie" {
				t.Fatalf("normalizeMediaType(%q) = %q, want movie", input, got)
			}
		})
	}

	if got := normalizeMediaType("", "The Last of Us", `F:\media\tv\The Last of Us`); got != "tv" {
		t.Fatalf("standalone tv path token = %q, want tv", got)
	}
}
