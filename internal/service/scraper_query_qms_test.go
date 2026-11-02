package service

import (
	"testing"

	"github.com/Ferrymania/nook/internal/model"
)

func TestCleanQueryQMSReleaseCleanup(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		input, title string
		year         int
	}{
		{"Resident.Evil.2026.2160p.iTunes.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-年糕.strm", "Resident Evil", 2026},
		{"Resident.Evil.2026.2160p.iTunes.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-年糕.mkv.strm", "Resident Evil", 2026},
		{"生化危机爆发夜4K.杜比视界 内封精修简英双语＆简中特效sup字幕.mkv", "生化危机爆发夜", 0},
		{"Movie.2026.1080p.未知发布组.mkv", "Movie", 2026},
		{"Movie.2026-未知发布组.mkv", "Movie", 2026},
		{"Movie.2026.简英双语.特效字幕.mkv", "Movie", 2026},
		{"Spider-Man.mkv", "Spider Man", 0},
		{"4Kids.2005.mkv", "4Kids", 2005},
		{"2026.Spider-Man.mkv", "Spider Man", 2026},
		{"Spider-Man.2002.1080p-GROUP.mkv", "Spider Man", 2002},
		{"寒战1994.2026.2160p-年糕.strm", "寒战1994", 2026},
		{"Dune.Part.2.2024.2160p-年糕.mkv", "Dune Part 2", 2024},
		{"剧名.S02E11.5.2160p-年糕.strm", "剧名", 0},
	} {
		t.Run(tc.input, func(t *testing.T) {
			got, year := CleanQuery(tc.input)
			if got != tc.title || year != tc.year {
				t.Fatalf("CleanQuery(%q) = %q/%d, want %q/%d", tc.input, got, year, tc.title, tc.year)
			}
		})
	}
}

func TestScrapeQueryQMSCleanupKeepsMovieIdentityChecks(t *testing.T) {
	t.Parallel()
	m := &model.Media{
		Title: "Resident Evil 年糕",
		Year:  2026,
		Path:  "/media/外语电影/生化危机爆发夜4K.杜比视界 内封精修简英双语＆简中特效sup字幕/Resident.Evil.2026.2160p.iTunes.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-年糕.strm",
	}
	queries := scrapeQueryCandidates(m, &model.Library{Path: "/media/外语电影", Type: "movie"})
	want := []string{"生化危机爆发夜", "Resident Evil"}
	if len(queries) != len(want) {
		t.Fatalf("queries = %q, want %q", queries, want)
	}
	for i := range want {
		if queries[i] != want[i] {
			t.Fatalf("queries = %q, want %q", queries, want)
		}
	}
	match := &Match{Title: "生化危机：爆发夜", OriginalName: "Resident Evil", Year: 2026, TMDbID: 1423191}
	if !metadataMatchTrusted(queries[0], 2026, match) || !automaticSourceNamesTrusted(queries, match) {
		t.Fatal("release noise still blocks the matching movie")
	}
	if metadataMatchTrusted(queries[0], 2026, &Match{Title: "生化危机：爆发夜2", Year: 2026}) {
		t.Fatal("cleanup weakened sequel identity checks")
	}
	if automaticSourceNamesTrusted([]string{"另一部电影", "Resident Evil"}, match) {
		t.Fatal("cleanup weakened bilingual identity checks")
	}
}
