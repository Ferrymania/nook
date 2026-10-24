package service

import (
	"path/filepath"
	"testing"

	"github.com/Ferrymania/nook/internal/model"
)

func TestMediaDisplayLibraryResolverUsesMostSpecificPath(t *testing.T) {
	root := t.TempDir()
	parentPath := filepath.Join(root, "media")
	nestedPath := filepath.Join(parentPath, "anime")
	parent := model.Library{
		Base:    model.Base{ID: "parent"},
		Name:    "媒体",
		Path:    parentPath,
		Enabled: true,
	}
	nested := model.Library{
		Base:    model.Base{ID: "nested"},
		Name:    "动漫",
		Path:    nestedPath,
		Enabled: true,
	}

	resolver := newMediaDisplayLibraryResolver([]model.Library{parent, nested})
	got, ok := resolver.DisplayLibraryForMedia(model.Media{
		LibraryID: parent.ID,
		Path:      filepath.Join(nestedPath, "Show", "S01E01.mkv"),
	})
	if !ok || got.ID != nested.ID {
		t.Fatalf("display library = %q, %t; want nested library", got.ID, ok)
	}

	got, ok = resolver.DisplayLibraryForMedia(model.Media{
		LibraryID: parent.ID,
		Path:      filepath.Join(parentPath, "Movie", "Movie.mkv"),
	})
	if !ok || got.ID != parent.ID {
		t.Fatalf("display library = %q, %t; want parent library", got.ID, ok)
	}
}

func TestMediaDisplayLibraryResolverPreservesOwnershipFallback(t *testing.T) {
	root := filepath.Join(t.TempDir(), "media")
	resolver := newMediaDisplayLibraryResolver([]model.Library{
		{Base: model.Base{ID: "parent"}, Path: root, Enabled: true},
		{Base: model.Base{ID: "disabled"}, Path: filepath.Join(root, "disabled"), Enabled: false},
		{Base: model.Base{ID: "owner"}, Path: filepath.Join(root, "owner"), Enabled: true},
	})
	for _, tc := range []struct {
		name, owner, path, want string
	}{
		{"disabled nested path", "owner", filepath.Join(root, "disabled", "movie.mkv"), "parent"},
		{"directory boundary", "owner", root + "-other/movie.mkv", "owner"},
		{"disabled owner fallback", "disabled", "/unmatched/movie.mkv", "disabled"},
		{"missing owner with matching path", "missing", filepath.Join(root, "movie.mkv"), "parent"},
		{"missing owner and path", "missing", "/unmatched/movie.mkv", ""},
		{"legacy cloud path", "owner", "cloud://cloud115/Movies/movie.mkv", "owner"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := resolver.DisplayLibraryForMedia(model.Media{LibraryID: tc.owner, Path: tc.path})
			if got.ID != tc.want || ok != (tc.want != "") {
				t.Fatalf("display library = %q, %t; want %q", got.ID, ok, tc.want)
			}
		})
	}
}
