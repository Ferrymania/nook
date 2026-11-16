package service

import (
	"context"
	"testing"

	"github.com/Ferrymania/nook/internal/model"
)

func TestScraperWithoutProvidersIsDisabled(t *testing.T) {
	t.Parallel()
	scraper := NewScraperService(nil, nil, nil, nil, nil, nil, nil, nil)
	if scraper.AnyEnabled() {
		t.Fatal("scraper without configured providers must be disabled")
	}
	if _, err := scraper.manualRequestMatch(context.Background(), ManualScrapeRequest{Source: "adult", Title: "Example"}); err == nil {
		t.Fatal("removed provider must not accept a manual match")
	}
}

func TestCodeNamedMetadataUsesOrdinaryMovieRules(t *testing.T) {
	t.Parallel()
	doc := &nfoDocument{Title: "Example", OriginalTitle: "SSIS-001"}
	meta := metadataFromDoc(doc, t.TempDir(), false)
	if meta.Title != "Example" || meta.OriginalName != "SSIS-001" {
		t.Fatalf("NFO titles changed: %+v", meta)
	}
	for _, path := range []string{"/media/SSIS-001.mkv", "/media/成人/Example.mkv"} {
		if got := inferLibraryKind("Movies", path, "movie"); got != "movie" {
			t.Fatalf("library type for %q = %q", path, got)
		}
	}
	// A shared code in a path cannot override distinct titles or provider IDs.
	items := []model.Media{
		{LibraryID: "movies", Title: "First", Path: "/media/SSIS-001/part1.mkv", TMDbID: 1},
		{LibraryID: "movies", Title: "Second", Path: "/media/SSIS-001/part2.mkv", TMDbID: 2},
	}
	if got := groupMediaVersions(items); len(got) != 2 {
		t.Fatalf("unrelated movies merged: %+v", got)
	}
}
