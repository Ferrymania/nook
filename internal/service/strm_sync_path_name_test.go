package service

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Ferrymania/nook/internal/model"
)

func TestResolveSyncPathName(t *testing.T) {
	paths := []model.StrmSyncPath{
		{Base: model.Base{ID: "first"}, Name: " 华语电视剧 ", Enabled: false},
		{Base: model.Base{ID: "second"}, Name: "OpenList 电影"},
		{Base: model.Base{ID: "legacy-duplicate"}, Name: " 华语电视剧 "},
	}
	for _, tc := range []struct {
		name, requested, want, errorText string
		existing                         *model.StrmSyncPath
	}{
		{name: "empty", requested: "", errorText: "请填写目录名称"},
		{name: "whitespace", requested: " \t\n　", errorText: "请填写目录名称"},
		{name: "duplicate including disabled", requested: "华语电视剧", errorText: "已存在"},
		{name: "trimmed duplicate", requested: "  华语电视剧 ", errorText: "已存在"},
		{name: "case insensitive duplicate", requested: "openlist 电影", errorText: "已存在"},
		{name: "new name", requested: " 115 电影 ", want: "115 电影"},
		{name: "own name", requested: "OpenList 电影", existing: &paths[1], want: "OpenList 电影"},
		{name: "rename to taken name", requested: "华语电视剧", existing: &paths[1], errorText: "已存在"},
		{name: "rename to unused name", requested: "OpenList 剧集", existing: &paths[1], want: "OpenList 剧集"},
		{name: "preserve legacy duplicate exactly", requested: paths[0].Name, existing: &paths[0], want: paths[0].Name},
		{name: "clear existing name", requested: " ", existing: &paths[0], errorText: "请填写目录名称"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveSyncPathName(tc.requested, tc.existing, paths)
			if tc.errorText != "" {
				if err == nil || !strings.Contains(err.Error(), tc.errorText) {
					t.Fatalf("expected %q, got name=%q err=%v", tc.errorText, got, err)
				}
			} else if err != nil || got != tc.want {
				t.Fatalf("name=%q err=%v, want %q", got, err, tc.want)
			}
		})
	}
}

func TestPrepareSyncPathNamePreservesExistingName(t *testing.T) {
	// Saving other settings of an old duplicate must not query or rewrite names.
	svc := &StrmService{}
	existing := &model.StrmSyncPath{Name: " 华语电视剧 "}
	p := *existing
	if err := svc.prepareSyncPathName(t.Context(), &p, existing); err != nil || p.Name != existing.Name {
		t.Fatalf("existing name changed: %q, %v", p.Name, err)
	}
	for _, name := range []string{"", " \t　"} {
		p.Name = name
		if err := svc.prepareSyncPathName(t.Context(), &p, existing); err == nil {
			t.Fatal("blank name must be rejected before accessing the database")
		}
	}
}

func TestSyncPathNameCreateAndRename(t *testing.T) {
	svc := testStrmService(t)
	ctx := t.Context()
	root := t.TempDir()
	source := t.TempDir()
	create := func(name, output string) (*model.StrmSyncPath, error) {
		return svc.CreateSyncPath(ctx, &model.StrmSyncPath{
			Name: name, Provider: model.StrmProviderLocal, RemotePath: source, LocalPath: output,
		})
	}
	first, err := create("华语电视剧", filepath.Join(root, "first"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", " \t", " 华语电视剧 "} {
		output := filepath.Join(root, "rejected")
		if _, err := create(name, output); err == nil {
			t.Fatalf("name %q should be rejected", name)
		}
		if _, err := os.Stat(output); !os.IsNotExist(err) {
			t.Fatal("rejected name must not create an output directory")
		}
	}
	second, err := create("OpenList 华语电视剧", filepath.Join(root, "second"))
	if err != nil {
		t.Fatal(err)
	}
	renamed := *second
	renamed.Name = first.Name
	if _, err := svc.UpdateSyncPath(ctx, second.ID, &renamed); err == nil || !strings.Contains(err.Error(), "已存在") {
		t.Fatalf("rename should reject duplicate: %v", err)
	}
	persisted, err := svc.repo.StrmSyncPath.FindByID(ctx, second.ID)
	if err != nil || persisted == nil || persisted.Name != second.Name {
		t.Fatalf("rejected rename changed stored name: %v", err)
	}
	// Seed a duplicate from an older version, then save a different setting.
	legacy := *first
	legacy.ID = ""
	legacy.LocalPath = filepath.Join(root, "legacy")
	if err := svc.repo.StrmSyncPath.Create(ctx, &legacy); err != nil {
		t.Fatal(err)
	}
	legacy.DownloadMeta = !legacy.DownloadMeta
	if _, err := svc.UpdateSyncPath(ctx, legacy.ID, &legacy); err != nil {
		t.Fatalf("legacy duplicate must remain editable without renaming: %v", err)
	}
	storedFirst, err := svc.repo.StrmSyncPath.FindByID(ctx, first.ID)
	if err != nil || storedFirst == nil || storedFirst.Name != first.Name {
		t.Fatalf("another legacy record was renamed: %v", err)
	}
}

func TestCreateSyncPathConcurrentDuplicateNames(t *testing.T) {
	svc := testStrmService(t)
	root, source := t.TempDir(), t.TempDir()
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, dir := range []string{"first", "second"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := svc.CreateSyncPath(t.Context(), &model.StrmSyncPath{
				Name: "同名目录", Provider: model.StrmProviderLocal, RemotePath: source, LocalPath: filepath.Join(root, dir),
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
		} else if !strings.Contains(err.Error(), "已存在") {
			t.Fatalf("unexpected save error: %v", err)
		}
	}
	paths, err := svc.repo.StrmSyncPath.List(t.Context())
	if err != nil || successes != 1 || len(paths) != 1 {
		t.Fatalf("concurrent duplicate saves: successes=%d stored=%d err=%v", successes, len(paths), err)
	}
}
