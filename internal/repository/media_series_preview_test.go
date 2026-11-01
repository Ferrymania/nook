package repository

import (
	"strings"
	"testing"
)

func TestSeriesPreviewQueriesKeepScopeAndTrimColumns(t *testing.T) {
	repos, log := newPostgresDryRunRepos(t)
	filter := MediaQueryFilter{AllowedLibraryIDs: []string{"allowed"}, HiddenLibraryIDs: []string{"hidden"}}
	if _, err := repos.Media.ListSeriesPreviewRows(t.Context(), []string{"allowed", "hidden"}, filter); err != nil {
		t.Fatal(err)
	}
	sql := log.all()
	for _, want := range []string{"\"title\"", "\"path\"", "\"poster_url\"", "\"tm_db_id\"", "\"thetvdb_id\"",
		"library_id IN", "library_id NOT IN", "nsfw = false", "deleted_at", "release_date DESC"} {
		if !strings.Contains(sql, want) {
			t.Fatalf("preview SQL missing %q: %s", want, sql)
		}
	}
	selectClause := strings.SplitN(sql, " FROM ", 2)[0]
	for _, omitted := range []string{"*", "overview", "strm_url", "video_codec", "audio_codec", "genres", "size_bytes"} {
		if strings.Contains(selectClause, omitted) {
			t.Fatalf("preview scans unnecessary field %q: %s", omitted, sql)
		}
	}

	repos, log = newPostgresDryRunRepos(t)
	if _, err := repos.Media.ListPreviewMediaByIDs(t.Context(), []string{"allowed"}, []string{"selected"}, filter); err != nil {
		t.Fatal(err)
	}
	sql = log.all()
	for _, want := range []string{"id IN ('selected')", "library_id IN ('allowed')", "library_id NOT IN ('hidden')", "nsfw = false", "deleted_at"} {
		if !strings.Contains(sql, want) {
			t.Fatalf("selected-card SQL missing %q: %s", want, sql)
		}
	}
}

func TestSeriesPreviewQueriesSkipEmptyScope(t *testing.T) {
	repos, log := newPostgresDryRunRepos(t)
	if _, err := repos.Media.ListSeriesPreviewRows(t.Context(), nil, MediaQueryFilter{}); err != nil {
		t.Fatal(err)
	}
	if _, err := repos.Media.ListPreviewMediaByIDs(t.Context(), []string{"library"}, nil, MediaQueryFilter{}); err != nil {
		t.Fatal(err)
	}
	if _, err := repos.Media.ListPreviewMediaByIDs(t.Context(), nil, []string{"id"}, MediaQueryFilter{}); err != nil {
		t.Fatal(err)
	}
	if sql := log.all(); sql != "" {
		t.Fatalf("empty preview scope queried all media: %s", sql)
	}
}
