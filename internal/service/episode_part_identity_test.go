package service

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
)

func TestParseNumberedPartEpisodesRequiresSeriesLibrary(t *testing.T) {
	cases := []struct {
		kind, path      string
		season, episode int
	}{
		{"tv", "/docs/Show/Show 2016 - Part 1 (1080p x265).strm", 1, 1},
		{"tv", "/docs/Show/Season 02/Show.Part.05.mkv.strm", 2, 5},
		{"anime", `C:\Anime\Show\Season 3\Show.part02.mkv`, 3, 2},
		{"variety", "/shows/Show/Show - PART-3.mkv", 1, 3},
		{"movie", "/movies/Film/Film - Part 2.mkv", 0, 0},
		{"music", "/music/Concert.Part1.mp4", 0, 0},
		{"tv", "/shows/Show/Show.电影版.Part2.mkv", 0, 0},
		{"tv", "/shows/Show/Show.Part.2016.mkv", 0, 0},
		{"tv", "/shows/Show/Show.Part.0.mkv", 0, 0},
		{"tv", "/shows/Show/Show.Part.Two.mkv", 0, 0},
		{"tv", "/shows/Part 5/Show.mkv", 0, 0},
		{"tv", "/shows/Show/Show.Part2A.mkv", 0, 0},
		{"tv", "/shows/Show/Show.S02E03.Part1.mkv", 2, 3},
		{"tv", "/shows/Show/Show.S01E11.5.Part1.mkv", 1, 11},
	}
	for _, tc := range cases {
		t.Run(tc.kind+":"+tc.path, func(t *testing.T) {
			s, e, _ := parseEpisodePartsForLibrary(tc.path, &model.Library{Type: tc.kind})
			if s != tc.season || e != tc.episode {
				t.Fatalf("got S%02dE%02d, want S%02dE%02d", s, e, tc.season, tc.episode)
			}
		})
	}
}

func TestNumberedPartIdentityPreservesConfirmedNFOAndManualMovieMatch(t *testing.T) {
	lib := &model.Library{Type: "tv"}
	media := model.Media{Path: "/docs/Show/Show.Part2.strm", SeasonNum: 0, EpisodeNum: 7}
	if fillNumberedPartEpisodeIdentity(&media, lib) || media.SeasonNum != 0 || media.EpisodeNum != 7 {
		t.Fatalf("confirmed NFO episode changed: %#v", media)
	}
	media = model.Media{Path: "/docs/Show/Show.Part2.strm"}
	rebuildManualScrapeIdentity(&media, lib, &Match{MediaType: "tv"})
	if media.SeasonNum != 1 || media.EpisodeNum != 2 {
		t.Fatalf("manual TV rematch lost Part identity: %#v", media)
	}
	rebuildManualScrapeIdentity(&media, lib, &Match{MediaType: "movie"})
	if media.SeasonNum != 0 || media.EpisodeNum != 0 {
		t.Fatalf("manual movie rematch retained episode identity: %#v", media)
	}
}

func TestRescanBackfillsNumberedPartEpisodesOnlyInSeriesLibrary(t *testing.T) {
	for _, kind := range []string{"tv", "movie"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			showDir := filepath.Join(root, "Documentary (2016)")
			if err := os.MkdirAll(showDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(showDir, "tvshow.nfo"), []byte(`<tvshow><title>Documentary</title><year>2016</year><uniqueid type="tmdb">66738</uniqueid></tvshow>`), 0o644); err != nil {
				t.Fatal(err)
			}
			db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.Setting{})
			repos := repository.New(db)
			lib := model.Library{Name: "Documentary", Type: kind, Path: root, Enabled: true}
			if err := repos.Library.Create(t.Context(), &lib); err != nil {
				t.Fatal(err)
			}
			for part := 1; part <= 5; part++ {
				path := filepath.Join(showDir, fmt.Sprintf("Documentary 2016 - Part %d (1080p x265).mkv", part))
				if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
					t.Fatal(err)
				}
				row := model.Media{LibraryID: lib.ID, Title: "Documentary", Path: path, SizeBytes: 1, Year: 2016, TMDbID: 66738, ScrapeStatus: "matched"}
				if err := db.Create(&row).Error; err != nil {
					t.Fatal(err)
				}
			}
			scanner := NewScannerService(&config.Config{}, zap.NewNop(), repos, NewHub(zap.NewNop()), nil, nil)
			if _, err := scanner.ScanLibrary(t.Context(), lib.ID); err != nil {
				t.Fatal(err)
			}
			var rows []model.Media
			if err := db.Where("library_id = ?", lib.ID).Order("path").Find(&rows).Error; err != nil {
				t.Fatal(err)
			}
			if len(rows) != 5 {
				t.Fatalf("got %d rows, want 5", len(rows))
			}
			for i, row := range rows {
				wantSeason, wantEpisode := 0, 0
				if kind == "tv" {
					wantSeason, wantEpisode = 1, i+1
				}
				if row.SeasonNum != wantSeason || row.EpisodeNum != wantEpisode || row.TMDbID != 66738 || row.ScrapeStatus != "matched" {
					t.Fatalf("unexpected rescan identity: %#v", row)
				}
			}
			if kind == "tv" {
				if got := GroupEpisodeVersionsForDisplay(rows); len(got) != 5 {
					t.Fatalf("got %d display episodes, want 5", len(got))
				}
			} else if got := groupMediaVersions(rows); len(got) != 1 || len(got[0].Versions) != 5 {
				t.Fatalf("movie parts no longer grouped: %#v", got)
			}
		})
	}
}

func TestEpisodeDisplayDoesNotMergeUnknownEpisodeNumbers(t *testing.T) {
	for _, season := range []int{0, 1} {
		rows := []model.Media{
			{Base: model.Base{ID: "a"}, LibraryID: "tv", TMDbID: 99, SeasonNum: season, Path: "/tv/Show/unidentified-a.strm"},
			{Base: model.Base{ID: "b"}, LibraryID: "tv", TMDbID: 99, SeasonNum: season, Path: "/tv/Show/unidentified-b.strm"},
		}
		if got := GroupEpisodeVersionsForDisplay(rows); len(got) != 2 {
			t.Fatalf("unknown episode identities collapsed: %#v", got)
		}
	}
}

func TestPartEpisodesKeepSameEpisodeEncodesTogether(t *testing.T) {
	lib := &model.Library{Type: "tv"}
	rows := []model.Media{
		{Base: model.Base{ID: "p1-hd"}, LibraryID: "tv", TMDbID: 99, Path: "/docs/Show/Show.Part1.1080p.strm", SizeBytes: 100},
		{Base: model.Base{ID: "p2"}, LibraryID: "tv", TMDbID: 99, Path: "/docs/Show/Show.Part2.1080p.strm", SizeBytes: 100},
		{Base: model.Base{ID: "p1-uhd"}, LibraryID: "tv", TMDbID: 99, Path: "/docs/Show/Show.Part1.2160p.strm", SizeBytes: 200},
	}
	for i := range rows {
		fillNumberedPartEpisodeIdentity(&rows[i], lib)
	}
	got := GroupEpisodeVersionsForDisplay(rows)
	if len(got) != 2 || got[0].EpisodeNum != 1 || len(got[0].Versions) != 2 || got[1].EpisodeNum != 2 {
		t.Fatalf("unexpected episode/version grouping: %#v", got)
	}
}

func TestProviderMatchPersistsNumberedPartIdentityOnlyForTV(t *testing.T) {
	for _, mediaType := range []string{"tv", "movie"} {
		t.Run(mediaType, func(t *testing.T) {
			db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.Series{}, &model.Setting{})
			repos := repository.New(db)
			lib := model.Library{Name: "Show", Type: "tv", Path: t.TempDir(), Enabled: true}
			if err := repos.Library.Create(t.Context(), &lib); err != nil {
				t.Fatal(err)
			}
			media := model.Media{LibraryID: lib.ID, Title: "Show", Path: filepath.Join(lib.Path, "Show.Part3.strm")}
			if err := db.Create(&media).Error; err != nil {
				t.Fatal(err)
			}
			log := zap.NewNop()
			scraper := NewScraperService(&config.Config{}, log, repos, nil, nil, nil, nil, NewHub(log))
			if err := scraper.applyProviderMatch(t.Context(), &media, &lib, &Match{Title: "Show", TMDbID: 99, MediaType: mediaType}); err != nil {
				t.Fatal(err)
			}
			var got model.Media
			if err := db.First(&got, "id = ?", media.ID).Error; err != nil {
				t.Fatal(err)
			}
			wantSeason, wantEpisode := 0, 0
			if mediaType == "tv" {
				wantSeason, wantEpisode = 1, 3
			}
			if got.SeasonNum != wantSeason || got.EpisodeNum != wantEpisode {
				t.Fatalf("provider match stored S%02dE%02d, want S%02dE%02d", got.SeasonNum, got.EpisodeNum, wantSeason, wantEpisode)
			}
		})
	}
}
