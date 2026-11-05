package service

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/service/cloud"
)

// A config/link change must update existing STRM files even when the remote
// video and its mtime did not change. No provider or database is needed here:
// link reconciliation must not resolve/download the remote video.
func TestIncrementalStrmSyncUpdatesLinksWithUnchangedMtime(t *testing.T) {
	for _, provider := range []string{model.StrmProvider115, model.StrmProviderOpenList, model.StrmProviderLocal} {
		t.Run(provider, func(t *testing.T) {
			t.Parallel()
			out := t.TempDir()
			p := &model.StrmSyncPath{Provider: provider, AccountID: "account", LocalPath: out, RemotePath: t.TempDir()}
			cfg := &strmPathConfig{BaseURL: "http://old.example:18080", AddPath: 1}
			entry := cloud.FileEntry{ID: "ref-1", PickCode: "pick-1", MTime: 1700000000}
			st := &strmSyncState{p: p, cfg: cfg, rec: &model.StrmSyncRecord{}, syncType: model.StrmSyncTypeIncremental, seenVideo: map[string]bool{}}
			const rel = "Series/episode.mkv"
			target := filepath.Join(out, "Series", "episode.strm")
			previous, err := st.strmContent(entry, rel, ".mkv")
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, target, previous)
			mtime := time.Unix(entry.MTime, 0)
			if err := os.Chtimes(target, mtime, mtime); err != nil {
				t.Fatal(err)
			}

			cfg.BaseURL = "http://127.0.0.1:18080"
			st.writeVideoStrm(entry, rel, ".mkv", "Series/episode.strm", false)
			data, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			want := strings.Replace(previous, "http://old.example:18080", cfg.BaseURL, 1)
			if string(data) != want || st.rec.NewStrm != 1 || st.rec.Skipped != 0 {
				t.Fatalf("unchanged remote mtime prevented link update: content=%q, updated=%d skipped=%d", data, st.rec.NewStrm, st.rec.Skipped)
			}
		})
	}
}

func TestIncrementalStrmSyncUpdatesLinkParameters(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		change   func(*strmSyncState, *cloud.FileEntry)
		key      string
		want     string
	}{
		{"pickcode", model.StrmProvider115, func(_ *strmSyncState, e *cloud.FileEntry) { e.PickCode = "pick-2" }, "pickcode", "pick-2"},
		{"openlist reference", model.StrmProviderOpenList, func(_ *strmSyncState, e *cloud.FileEntry) { e.ID = "ref-2" }, "ref", "ref-2"},
		{"account", model.StrmProvider115, func(s *strmSyncState, _ *cloud.FileEntry) { s.p.AccountID = "account-2" }, "acct", "account-2"},
		{"path mode", model.StrmProvider115, func(s *strmSyncState, _ *cloud.FileEntry) { s.cfg.AddPath = 2 }, "path", "episode.mkv"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := t.TempDir()
			st := &strmSyncState{
				p:   &model.StrmSyncPath{Provider: tc.provider, AccountID: "account-1", LocalPath: out},
				cfg: &strmPathConfig{BaseURL: "http://127.0.0.1:18080", AddPath: 1},
				rec: &model.StrmSyncRecord{}, syncType: model.StrmSyncTypeIncremental, seenVideo: map[string]bool{},
			}
			entry := cloud.FileEntry{ID: "ref-1", PickCode: "pick-1", MTime: 1700000000}
			const rel = "Series/episode.mkv"
			target := filepath.Join(out, "episode.strm")
			previous, err := st.strmContent(entry, rel, ".mkv")
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, target, previous)
			mtime := time.Unix(entry.MTime, 0)
			if err := os.Chtimes(target, mtime, mtime); err != nil {
				t.Fatal(err)
			}
			tc.change(st, &entry)
			st.writeVideoStrm(entry, rel, ".mkv", "episode.strm", false)
			data, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			u, err := url.Parse(string(data))
			if err != nil {
				t.Fatal(err)
			}
			if u.Query().Get(tc.key) != tc.want || st.rec.NewStrm != 1 {
				t.Fatalf("link parameter not updated: content=%q updated=%d", data, st.rec.NewStrm)
			}
		})
	}
}

func TestIncrementalStrmSyncDoesNotRewriteUnchangedLink(t *testing.T) {
	t.Parallel()
	out := t.TempDir()
	st := &strmSyncState{
		p:   &model.StrmSyncPath{Provider: model.StrmProvider115, AccountID: "account", LocalPath: out},
		cfg: &strmPathConfig{BaseURL: "http://127.0.0.1:18080", AddPath: 1},
		rec: &model.StrmSyncRecord{}, syncType: model.StrmSyncTypeIncremental, seenVideo: map[string]bool{},
	}
	entry := cloud.FileEntry{PickCode: "pick-1", MTime: 1700000000}
	previous, err := st.strmContent(entry, "episode.mkv", ".mkv")
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(out, "episode.strm")
	writeFile(t, target, previous)
	before, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	st.writeVideoStrm(entry, "episode.mkv", ".mkv", "episode.strm", false)
	after, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) || st.rec.NewStrm != 0 || st.rec.Skipped != 1 {
		t.Fatalf("unchanged link was rewritten: same_file=%v updated=%d skipped=%d", os.SameFile(before, after), st.rec.NewStrm, st.rec.Skipped)
	}
}
