package service

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Ferrymania/nook/internal/model"
)

func TestSyncOutputPathsOverlap(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "shows")
	for _, tc := range []struct {
		name, other string
		want        bool
	}{
		{"same", base, true},
		{"child", filepath.Join(base, "season"), true},
		{"parent", root, true},
		{"sibling with same prefix", filepath.Join(root, "shows-other"), false},
		{"different sibling", filepath.Join(root, "movies"), false},
		{"filesystem root", filepath.VolumeName(root) + string(filepath.Separator), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := syncOutputPathsOverlap(base, tc.other); got != tc.want {
				t.Fatalf("overlap = %v, want %v", got, tc.want)
			}
		})
	}
	canonical, err := canonicalSyncOutputPath(base + string(filepath.Separator))
	if err != nil {
		t.Fatal(err)
	}
	alias, err := canonicalSyncOutputPath(filepath.Join(root, "other", "..", "shows"))
	if err != nil || canonical != alias {
		t.Fatalf("equivalent paths differ: %q / %q, %v", canonical, alias, err)
	}
}

func TestCanonicalSyncOutputPathSymlinks(t *testing.T) {
	root := t.TempDir()
	realPath := filepath.Join(root, "real")
	if err := os.Mkdir(realPath, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(realPath, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	a, err := canonicalSyncOutputPath(filepath.Join(realPath, "new", "nested"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := canonicalSyncOutputPath(filepath.Join(alias, "new", "nested"))
	if err != nil || a != b {
		t.Fatalf("symlink alias differs: %q / %q, %v", a, b, err)
	}
	if _, err := os.Stat(filepath.Join(realPath, "new")); !os.IsNotExist(err) {
		t.Fatal("path comparison must not create output directories")
	}
	dangling := filepath.Join(root, "dangling")
	if err := os.Symlink(filepath.Join(root, "missing"), dangling); err != nil {
		t.Fatal(err)
	}
	if _, err := canonicalSyncOutputPath(filepath.Join(dangling, "child")); err == nil {
		t.Fatal("dangling symlink should fail validation")
	}
}

func TestSyncOutputPathConflictsOnCreateAndUpdate(t *testing.T) {
	svc := testStrmService(t)
	ctx := t.Context()
	root := t.TempDir()
	output := filepath.Join(root, "shows")
	source := t.TempDir()
	first := syncPathRecord(t, svc, model.StrmProviderLocal, source, output, false)
	if err := os.WriteFile(filepath.Join(output, "existing.strm"), []byte("unchanged"), 0o644); err != nil {
		t.Fatal(err)
	}
	account, err := svc.CreateStrmAccount(ctx, "OpenList", model.StrmProviderOpenList, map[string]string{"server": "https://list.example.com", "token": "test-token"})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{output, output + string(filepath.Separator), filepath.Join(output, "child"), root} {
		_, err := svc.CreateSyncPath(ctx, &model.StrmSyncPath{
			Name: "new", Provider: model.StrmProviderOpenList, AccountID: account.ID,
			RemotePath: "/shows", LocalPath: path,
		})
		if err == nil || !strings.Contains(err.Error(), first.Name) || !strings.Contains(err.Error(), output) {
			t.Fatalf("path %q: expected error naming conflicting task and output, got %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(output, "child")); !os.IsNotExist(err) {
		t.Fatal("rejected output directory must not be created")
	}
	second := syncPathRecord(t, svc, model.StrmProviderLocal, source, filepath.Join(root, "shows-other"), true)
	unchanged := *first
	unchanged.Name = "renamed"
	if _, err := svc.UpdateSyncPath(ctx, first.ID, &unchanged); err != nil {
		t.Fatalf("editing own path must succeed: %v", err)
	}
	conflicting := *second
	conflicting.LocalPath = filepath.Join(output, "child")
	if _, err := svc.UpdateSyncPath(ctx, second.ID, &conflicting); err == nil {
		t.Fatal("update must reject overlap with another task")
	}
	persisted, err := svc.repo.StrmSyncPath.FindByID(ctx, second.ID)
	if err != nil || persisted == nil || persisted.LocalPath != second.LocalPath {
		t.Fatalf("rejected update changed stored output path: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(output, "existing.strm"))
	if err != nil || string(data) != "unchanged" {
		t.Fatalf("validation changed existing STRM: %v", err)
	}
}

func TestCreateSyncPathConcurrentOutputConflict(t *testing.T) {
	svc := testStrmService(t)
	root := t.TempDir()
	source := t.TempDir()
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, output := range []string{filepath.Join(root, "shows"), filepath.Join(root, "shows", "child")} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := svc.CreateSyncPath(t.Context(), &model.StrmSyncPath{
				Name: "concurrent " + filepath.Base(output), Provider: model.StrmProviderLocal, RemotePath: source, LocalPath: output,
			})
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	paths, err := svc.repo.StrmSyncPath.List(t.Context())
	if err != nil || successes != 1 || len(paths) != 1 {
		t.Fatalf("overlapping concurrent saves: successful=%d stored=%d err=%v", successes, len(paths), err)
	}
}
