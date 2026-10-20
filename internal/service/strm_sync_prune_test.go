package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Ferrymania/nook/internal/model"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// No database or remote service is needed: reproduce a complete remote snapshot
// with a live episode, but no remote counterpart for Nook's local artwork folder.
func pruneTestState(t *testing.T, deleteEmpty bool) *strmSyncState {
	t.Helper()
	return &strmSyncState{
		s: &StrmService{log: zap.NewNop()}, ctx: t.Context(),
		p:   &model.StrmSyncPath{Base: model.Base{ID: "artwork-prune"}, LocalPath: t.TempDir()},
		cfg: &strmPathConfig{DeleteDir: deleteEmpty}, rec: &model.StrmSyncRecord{},
		syncType:  model.StrmSyncTypeIncremental,
		seenVideo: map[string]bool{"v:show/episode01": true},
	}
}

func TestPruneLocalPreservesSharedArtworkAndMetadata(t *testing.T) {
	for _, deleteEmpty := range []bool{false, true} {
		t.Run(map[bool]string{false: "keep-empty", true: "delete-empty"}[deleteEmpty], func(t *testing.T) {
			st := pruneTestState(t, deleteEmpty)
			root := st.p.LocalPath
			kept := []string{
				"show/episode01.strm", "show/episode01.nfo", "show/episode01-thumb.webp",
				"show/nook-artwork/poster.webp", "show/nook-artwork/backdrop.webp", "show/nook-artwork/cleanup.jsonl",
				"orphan/nook-artwork/poster.webp", "orphan/user-subtitle.srt", "orphan/user-file.mkv",
			}
			for _, name := range kept {
				writeFile(t, filepath.Join(root, name), "preserve:"+name)
			}
			writeFile(t, filepath.Join(root, "orphan/episode02.strm"), "stale")
			writeFile(t, filepath.Join(root, "show/stale.strm"), "stale")
			if err := st.pruneLocal(); err != nil {
				t.Fatal(err)
			}
			for _, name := range kept {
				data, err := os.ReadFile(filepath.Join(root, name))
				if err != nil || string(data) != "preserve:"+name {
					t.Errorf("changed local file %s: %q %v", name, data, err)
				}
			}
			for _, name := range []string{"orphan/episode02.strm", "show/stale.strm"} {
				if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
					t.Errorf("stale STRM remains: %s: %v", name, err)
				}
			}
			if st.rec.Pruned != 2 {
				t.Errorf("pruned=%d, want only two stale STRMs", st.rec.Pruned)
			}
		})
	}
}

func TestPruneLocalOnlyRemovesEmptyDirectoriesWhenEnabled(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
			st := pruneTestState(t, enabled)
			writeFile(t, filepath.Join(st.p.LocalPath, "gone/nested/stale.strm"), "stale")
			protected := filepath.Join(st.p.LocalPath, "show/nook-artwork")
			if err := os.MkdirAll(protected, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := st.pruneLocal(); err != nil {
				t.Fatal(err)
			}
			_, err := os.Stat(filepath.Join(st.p.LocalPath, "gone"))
			if enabled && !os.IsNotExist(err) {
				t.Errorf("empty orphan directory remains: %v", err)
			}
			if !enabled && err != nil {
				t.Errorf("empty directory removed with cleanup disabled: %v", err)
			}
			if _, err := os.Stat(protected); err != nil {
				t.Errorf("reserved artwork directory removed: %v", err)
			}
			if _, err := os.Stat(st.p.LocalPath); err != nil {
				t.Errorf("sync root removed: %v", err)
			}
		})
	}
}

func TestPruneLocalIncompleteOrCanceledScanDoesNotDelete(t *testing.T) {
	for _, kind := range []string{model.StrmSyncTypeFull, model.StrmSyncTypeIncremental} {
		t.Run(kind, func(t *testing.T) {
			st := pruneTestState(t, true)
			st.syncType = kind
			st.scanIncomplete.Store(true)
			path := filepath.Join(st.p.LocalPath, "gone/stale.strm")
			writeFile(t, path, "keep")
			if err := st.pruneLocal(); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("incomplete scan removed STRM: %v", err)
			}
			if st.rec.Pruned != 0 {
				t.Fatalf("incomplete scan pruned=%d", st.rec.Pruned)
			}
		})
	}
	t.Run("canceled", func(t *testing.T) {
		st := pruneTestState(t, true)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		st.ctx = ctx
		path := filepath.Join(st.p.LocalPath, "gone/stale.strm")
		writeFile(t, path, "keep")
		if err := st.pruneLocal(); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled scan err=%v", err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("canceled scan removed STRM: %v", err)
		}
	})
}

func TestPruneLocalSkipsSymlinksAndFailsOnUnreadableRoot(t *testing.T) {
	st := pruneTestState(t, true)
	out := filepath.Join(t.TempDir(), "outside.strm")
	writeFile(t, out, "outside")
	link := filepath.Join(st.p.LocalPath, "linked.strm")
	if err := os.Symlink(out, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := st.pruneLocal(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Errorf("unmanaged symlink removed: %v", err)
	}
	if data, err := os.ReadFile(out); err != nil || string(data) != "outside" {
		t.Errorf("outside file changed: %v", err)
	}
	st.p.LocalPath = filepath.Join(st.p.LocalPath, "missing-root")
	if err := st.pruneLocal(); err == nil {
		t.Fatal("missing root must fail closed")
	}
}

func TestPruneLocalLogsDeletedFileAndReason(t *testing.T) {
	st := pruneTestState(t, false)
	core, logs := observer.New(zap.InfoLevel)
	st.s.log = zap.New(core)
	path := filepath.Join(st.p.LocalPath, "stale.strm")
	writeFile(t, path, "stale")
	if err := st.pruneLocal(); err != nil {
		t.Fatal(err)
	}
	entries := logs.FilterMessage("strm local cleanup removed file").All()
	if len(entries) != 1 {
		t.Fatalf("missing per-file cleanup log: %+v", logs.All())
	}
	fields := entries[0].ContextMap()
	if fields["path"] != path || fields["path_id"] != st.p.ID || fields["reason"] != "remote_video_missing" {
		t.Fatalf("cleanup evidence missing: %+v", fields)
	}
}
