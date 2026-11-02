package service

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
)

func newFileManagerTestServiceWithRepo(t *testing.T, root string) (*FileManagerService, *repository.Container) {
	t.Helper()
	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.Setting{})
	repos := repository.New(db)
	lib := model.Library{Name: "downloads", Path: root, Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.App.DataDir = root
	cfg.Cache.CacheDir = root
	return NewFileManagerService(cfg, zap.NewNop(), repos), repos
}

func newFileManagerTestService(t *testing.T, root string) *FileManagerService {
	t.Helper()
	svc, _ := newFileManagerTestServiceWithRepo(t, root)
	return svc
}

func TestFileManagerRecursiveListAndMutations(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "downloads", "国产剧")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(nested, "狂飙.S01E01.mkv")
	if err := os.WriteFile(mediaPath, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := newFileManagerTestService(t, root)

	listing, err := svc.List(root, 100, true)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range listing.Entries {
		if entry.Path == mediaPath {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("recursive listing did not include %s", mediaPath)
	}

	created, err := svc.CreateFolder(root, "整理目标")
	if err != nil {
		t.Fatal(err)
	}
	emptyListing, err := svc.List(created.Path, 100, false)
	if err != nil {
		t.Fatal(err)
	}
	if emptyListing.Entries == nil || len(emptyListing.Entries) != 0 {
		t.Fatalf("empty directory entries = %#v, want empty slice", emptyListing.Entries)
	}
	copied, err := svc.Transfer(mediaPath, created.Path, TransferCopy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(copied.Path); err != nil {
		t.Fatalf("copied file missing: %v", err)
	}
	renamed, err := svc.Rename(copied.Path, "狂飙 - S01E01.mkv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(renamed.Path); err != nil {
		t.Fatalf("renamed file missing: %v", err)
	}
	if err := svc.Delete(renamed.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(renamed.Path); !os.IsNotExist(err) {
		t.Fatalf("expected file deleted, stat err=%v", err)
	}
}

func TestFileManagerTransferDirectoryHardlinksFiles(t *testing.T) {
	root := t.TempDir()
	if hardlinksUnsupported(t, root) {
		t.Skip("hardlinks unsupported on this filesystem")
	}
	sourceDir := filepath.Join(root, "downloads", "Show")
	seasonDir := filepath.Join(sourceDir, "Season 01")
	targetRoot := filepath.Join(root, "media")
	if err := os.MkdirAll(seasonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	sourceFile := filepath.Join(seasonDir, "Show.S01E01.mkv")
	if err := os.WriteFile(sourceFile, []byte("episode"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := newFileManagerTestService(t, root)

	res, err := svc.Transfer(sourceDir, targetRoot, TransferHardlink)
	if err != nil {
		t.Fatal(err)
	}
	targetFile := filepath.Join(res.Path, "Season 01", "Show.S01E01.mkv")
	sourceInfo, err := os.Stat(sourceFile)
	if err != nil {
		t.Fatal(err)
	}
	targetInfo, err := os.Stat(targetFile)
	if err != nil {
		t.Fatalf("hardlinked directory file missing: %v", err)
	}
	if !os.SameFile(sourceInfo, targetInfo) {
		t.Fatal("directory hardlink should hardlink contained files")
	}
	listing, err := svc.List(res.Path, 100, true)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range listing.Entries {
		if entry.Path == targetFile {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("target directory listing did not include %s", targetFile)
	}
}

func TestFileManagerRefusesRootMutation(t *testing.T) {
	root := t.TempDir()
	svc := newFileManagerTestService(t, root)
	if err := svc.Delete(root); !errors.Is(err, ErrRootMutation) {
		t.Fatalf("Delete(root) err = %v, want ErrRootMutation", err)
	}
}

// TestFileManagerResolvePath 阅读模块的服务器选书/选目录入口靠它做边界校验：
// 允许根目录内放行，越界一律拒绝。
func TestFileManagerResolvePath(t *testing.T) {
	root := t.TempDir()
	svc := newFileManagerTestService(t, root)

	inside := filepath.Join(root, "books", "小说.txt")
	if err := os.MkdirAll(filepath.Dir(inside), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inside, []byte("正文"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := svc.ResolvePath(inside)
	if err != nil {
		t.Fatalf("允许根目录内的路径应放行: %v", err)
	}
	if got != inside {
		t.Fatalf("ResolvePath = %q，期望 %q", got, inside)
	}

	outside := filepath.Join(t.TempDir(), "越界.txt")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ResolvePath(outside); !errors.Is(err, ErrPathOutOfBounds) {
		t.Fatalf("越界路径 err = %v, want ErrPathOutOfBounds", err)
	}
}

func hardlinksUnsupported(t *testing.T, root string) bool {
	t.Helper()
	src := filepath.Join(root, "hardlink-probe-src")
	dst := filepath.Join(root, "hardlink-probe-dst")
	if err := os.WriteFile(src, []byte("probe"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := os.Link(src, dst)
	_ = os.Remove(src)
	_ = os.Remove(dst)
	return err != nil
}

func TestFileManagerIgnoresRetiredDownloadAndOrganizeRoots(t *testing.T) {
	svc, repos := newFileManagerTestServiceWithRepo(t, t.TempDir())
	legacyPaths := []string{t.TempDir()}
	t.Setenv("NOOK_DOWNLOAD_CONTAINER_DIR", legacyPaths[0])
	for _, key := range []string{"organize.source_dir", "organize.target_dir", "downloader.savepath", "qbittorrent.savepath"} {
		directory := t.TempDir()
		legacyPaths = append(legacyPaths, directory)
		if err := repos.Setting.Set(t.Context(), key, directory); err != nil {
			t.Fatal(err)
		}
	}
	listing, err := svc.List("", 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, root := range listing.Roots {
		for _, legacy := range legacyPaths {
			if root.Path == legacy {
				t.Fatalf("retired configuration still exposes root %#v", root)
			}
		}
	}
	for _, directory := range legacyPaths {
		if _, err := svc.List(directory, 100); !errors.Is(err, ErrPathOutOfBounds) {
			t.Fatalf("List(%q) error = %v, want out-of-bounds", directory, err)
		}
		if _, err := svc.ResolvePath(directory); !errors.Is(err, ErrPathOutOfBounds) {
			t.Fatalf("ResolvePath(%q) error = %v, want out-of-bounds", directory, err)
		}
		if _, err := svc.CreateFolder(directory, "child"); !errors.Is(err, ErrPathOutOfBounds) {
			t.Fatalf("CreateFolder(%q) error = %v, want out-of-bounds", directory, err)
		}
	}
	// An explicit library root remains usable, even when its directory is named downloads.
	directory := filepath.Join(t.TempDir(), "downloads")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	library := &model.Library{Name: "显式下载目录", Path: directory, Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), library); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateFolder(directory, "影片"); err != nil {
		t.Fatalf("explicit library root must remain writable: %v", err)
	}
}

func TestFileManagerIncludesAllLibraryRoots(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	nestedB := filepath.Join(rootB, "second-root")
	if err := os.MkdirAll(nestedB, 0o755); err != nil {
		t.Fatal(err)
	}
	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{}, &model.Setting{})
	repos := repository.New(db)
	lib := &model.Library{Name: "电影", Path: rootA, Type: "movie", Enabled: true}
	if err := repos.Library.CreateWithRoots(t.Context(), lib, []model.LibraryRoot{
		{Name: "硬盘1", Path: rootA, Enabled: true},
		{Name: "硬盘2", Path: rootB, Enabled: true, SortOrder: 1},
	}); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.App.DataDir = t.TempDir()
	cfg.Cache.CacheDir = t.TempDir()
	svc := NewFileManagerService(cfg, zap.NewNop(), repos)

	listing, err := svc.List("", 100)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, root := range listing.Roots {
		got[root.Label] = root.Path
	}
	if got["library:电影:硬盘1"] != filepath.Clean(rootA) {
		t.Fatalf("root A missing from listing: %#v", listing.Roots)
	}
	if got["library:电影:硬盘2"] != filepath.Clean(rootB) {
		t.Fatalf("root B missing from listing: %#v", listing.Roots)
	}
	if _, err := svc.List(nestedB, 100); err != nil {
		t.Fatalf("list second library root: %v", err)
	}
}
