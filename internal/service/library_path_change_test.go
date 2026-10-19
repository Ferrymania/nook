package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
)

type pathChangeFixture struct {
	svc    *MediaService
	repos  *repository.Container
	lib    *model.Library
	root   model.LibraryRoot
	other  model.LibraryRoot
	oldDir string
	newDir string
}

func newPathChangeFixture(t *testing.T) *pathChangeFixture {
	t.Helper()
	repos := repository.New(newServiceTestDB(t))
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	oldDir, newDir, otherDir := t.TempDir(), t.TempDir(), t.TempDir()
	lib, err := svc.CreateLibraryWithRoots(t.Context(), "来源迁移", "movie", []LibraryRootInput{{Path: oldDir}, {Path: otherDir}})
	if err != nil {
		t.Fatal(err)
	}
	return &pathChangeFixture{svc: svc, repos: repos, lib: lib, root: lib.Roots[0], other: lib.Roots[1], oldDir: oldDir, newDir: newDir}
}

func writePathChangeFile(t *testing.T, dir, relative, content string) string {
	t.Helper()
	full := filepath.Join(dir, relative)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return full
}

func (f *pathChangeFixture) media(t *testing.T, root model.LibraryRoot, rel, content string) model.Media {
	t.Helper()
	full := writePathChangeFile(t, root.Path, rel, content)
	m := model.Media{LibraryID: f.lib.ID, LibraryRootID: root.ID, Path: full, RelativePath: filepath.ToSlash(rel), Title: "已手动匹配", SizeBytes: int64(len(content)), TMDbID: 123, ScrapeStatus: "matched", DurationSec: 42, Overview: "保留的资料"}
	if err := f.repos.DB.Create(&m).Error; err != nil {
		t.Fatal(err)
	}
	return m
}

func (f *pathChangeFixture) input(t *testing.T, mode string) LibraryRootInput {
	t.Helper()
	in := LibraryRootInput{Path: f.newDir, ChangeMode: mode}
	p, err := f.svc.PreviewLibraryRootPath(t.Context(), f.lib.ID, f.root.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if !p.CanApply {
		t.Fatalf("unexpected conflicts: %+v", p)
	}
	in.PreviewToken = p.PreviewToken
	return in
}

func assertPathChangeFile(t *testing.T, name, want string) {
	t.Helper()
	content, err := os.ReadFile(name)
	if err != nil || string(content) != want {
		t.Fatalf("file %s changed: %q, %v", name, content, err)
	}
}

func TestLibraryPathMigrationPreservesIdentityAndRelatedRecords(t *testing.T) {
	f := newPathChangeFixture(t)
	m := f.media(t, f.root, "影片/电影.mkv", "media")
	legacy := f.media(t, f.root, "旧记录.mkv", "legacy")
	if err := f.repos.DB.Model(&model.Media{}).Where("id = ?", legacy.ID).Updates(map[string]any{"library_root_id": "", "relative_path": ""}).Error; err != nil {
		t.Fatal(err)
	}
	other := f.media(t, f.other, "另一来源.mkv", "other")
	newPath := writePathChangeFile(t, f.newDir, "影片/电影.mkv", "media")
	writePathChangeFile(t, f.newDir, "旧记录.mkv", "legacy")
	// An extra file is deliberately absent from the preview; the normal scan
	// imports it after the index transition, without replacing existing IDs.
	writePathChangeFile(t, f.newDir, "新增.mkv", "new")
	for _, row := range []any{
		&model.PlaybackHistory{MediaID: m.ID, UserID: "user", PositionMs: 456},
		&model.Favorite{MediaID: m.ID, UserID: "user"},
		&model.MediaProbe{MediaID: m.ID, Signature: "old", Payload: "cached"},
		&model.ScrapeTask{MediaID: m.ID, LibraryID: f.lib.ID, MediaPath: m.Path, MediaTitle: m.Title, Status: model.ScrapeTaskDone},
	} {
		if err := f.repos.DB.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	in := f.input(t, LibraryPathMigrate)
	before, _ := f.repos.Library.FindRootByID(t.Context(), f.lib.ID, f.root.ID)
	if before.Path != f.oldDir {
		t.Fatal("preview changed root")
	}
	if _, err := f.svc.UpdateLibraryRoot(t.Context(), f.lib.ID, f.root.ID, in); err != nil {
		t.Fatal(err)
	}
	var got model.Media
	if err := f.repos.DB.First(&got, "id = ?", m.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Path != newPath || got.Overview != m.Overview || got.TMDbID != m.TMDbID || got.Title != m.Title || got.ScrapeStatus != m.ScrapeStatus {
		t.Fatalf("media was not preserved: %+v", got)
	}
	stream := &StreamService{repo: f.repos}
	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	req.Header.Set("Range", "bytes=1-3")
	recorder := httptest.NewRecorder()
	if err := stream.ServeFile(recorder, req, m.ID); err != nil || recorder.Code != http.StatusPartialContent || recorder.Body.String() != "edi" {
		t.Fatalf("playback through original ID failed: code=%d body=%q err=%v", recorder.Code, recorder.Body.String(), err)
	}
	for _, entity := range []any{&model.PlaybackHistory{}, &model.Favorite{}, &model.MediaProbe{}, &model.ScrapeTask{}} {
		var count int64
		if err := f.repos.DB.Model(entity).Where("media_id = ?", m.ID).Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("related %T count=%d err=%v", entity, count, err)
		}
	}
	var task model.ScrapeTask
	f.repos.DB.Where("media_id = ?", m.ID).First(&task)
	if task.MediaPath != newPath {
		t.Fatalf("task path=%s", task.MediaPath)
	}
	scanner := NewScannerService(&config.Config{}, zap.NewNop(), f.repos, NewHub(zap.NewNop()), nil, nil)
	if _, err := scanner.ScanLibraryRoot(t.Context(), f.lib.ID, f.root.ID); err != nil {
		t.Fatal(err)
	}
	if added, err := scanner.IngestPath(t.Context(), f.lib.ID, m.Path); err != nil || added {
		t.Fatalf("stale watcher event imported old directory: %v %v", added, err)
	}
	var count int64
	f.repos.DB.Model(&model.Media{}).Count(&count)
	if count != 4 {
		t.Fatalf("scan count=%d, want 4", count)
	}
	var otherGot model.Media
	if err := f.repos.DB.First(&otherGot, "id = ?", other.ID).Error; err != nil {
		t.Fatal(err)
	}
	if otherGot.Path != other.Path {
		t.Fatal("changed other source")
	}
	assertPathChangeFile(t, m.Path, "media")
	assertPathChangeFile(t, newPath, "media")
}

func TestLibraryPathReplacementClearsOnlySelectedIndexAndLeavesFiles(t *testing.T) {
	f := newPathChangeFixture(t)
	old := f.media(t, f.root, "电影.mkv", "old")
	legacy := f.media(t, f.root, "旧无来源.mkv", "legacy")
	f.repos.DB.Model(&model.Media{}).Where("id = ?", legacy.ID).Update("library_root_id", "")
	other := f.media(t, f.other, "电影.mkv", "other")
	newPath := writePathChangeFile(t, f.newDir, "电影.mkv", "new contents")
	for _, m := range []model.Media{old, other} {
		f.repos.DB.Create(&model.PlaybackHistory{MediaID: m.ID, UserID: "user", PositionMs: 77})
		f.repos.DB.Create(&model.Favorite{MediaID: m.ID, UserID: "user"})
	}
	in := f.input(t, LibraryPathReplace)
	if _, err := f.svc.UpdateLibraryRoot(t.Context(), f.lib.ID, f.root.ID, in); err != nil {
		t.Fatal(err)
	}
	for _, entity := range []any{&model.Media{}, &model.PlaybackHistory{}, &model.Favorite{}} {
		query := "media_id IN ?"
		if _, ok := entity.(*model.Media); ok {
			query = "id IN ?"
		}
		var count int64
		f.repos.DB.Unscoped().Model(entity).Where(query, []string{old.ID, legacy.ID}).Count(&count)
		if count != 0 {
			t.Fatalf("old %T records remain", entity)
		}
		f.repos.DB.Model(entity).Where(query, []string{other.ID}).Count(&count)
		if count != 1 {
			t.Fatalf("other source %T was changed", entity)
		}
	}
	scanner := NewScannerService(&config.Config{}, zap.NewNop(), f.repos, NewHub(zap.NewNop()), nil, nil)
	if _, err := scanner.ScanLibraryRoot(t.Context(), f.lib.ID, f.root.ID); err != nil {
		t.Fatal(err)
	}
	var fresh model.Media
	if err := f.repos.DB.Where("path = ?", newPath).First(&fresh).Error; err != nil || fresh.ID == old.ID || fresh.TMDbID != 0 {
		t.Fatalf("new content adopted old metadata: %+v %v", fresh, err)
	}
	assertPathChangeFile(t, old.Path, "old")
	assertPathChangeFile(t, legacy.Path, "legacy")
	assertPathChangeFile(t, other.Path, "other")
	assertPathChangeFile(t, newPath, "new contents")
}

func TestLibraryPathMigrationBlocksMissingConflictingAndChangedFiles(t *testing.T) {
	for _, scenario := range []string{"missing", "size", "occupied", "relative_escape", "strm", "overlapping_root"} {
		t.Run(scenario, func(t *testing.T) {
			f := newPathChangeFixture(t)
			rel := "电影.mkv"
			if scenario == "strm" {
				rel = "电影.strm"
			}
			m := f.media(t, f.root, rel, "old")
			if scenario != "missing" {
				writePathChangeFile(t, f.newDir, rel, "old")
			}
			switch scenario {
			case "size":
				writePathChangeFile(t, f.newDir, rel, "larger")
			case "occupied":
				if err := f.repos.DB.Create(&model.Media{LibraryID: "another-library", Path: filepath.Join(f.newDir, rel), Title: "occupied"}).Error; err != nil {
					t.Fatal(err)
				}
			case "relative_escape":
				f.repos.DB.Model(&model.Media{}).Where("id = ?", m.ID).Update("relative_path", "../escape.mkv")
			case "strm":
				writePathChangeFile(t, f.oldDir, rel, "https://example.com/old")
				writePathChangeFile(t, f.newDir, rel, "https://example.com/new")
			case "overlapping_root":
				f.newDir = f.other.Path
			}
			p, err := f.svc.PreviewLibraryRootPath(t.Context(), f.lib.ID, f.root.ID, LibraryRootInput{Path: f.newDir, ChangeMode: LibraryPathMigrate})
			if err != nil || p.CanApply || len(p.Issues) == 0 {
				t.Fatalf("preview=%+v err=%v", p, err)
			}
			if _, err := f.svc.UpdateLibraryRoot(t.Context(), f.lib.ID, f.root.ID, LibraryRootInput{Path: f.newDir, ChangeMode: LibraryPathMigrate, PreviewToken: p.PreviewToken}); err == nil {
				t.Fatal("applied invalid migration")
			}
			root, _ := f.repos.Library.FindRootByID(t.Context(), f.lib.ID, f.root.ID)
			if root.Path != f.oldDir {
				t.Fatal("failed migration changed root")
			}
		})
	}
}

func TestLibraryPathConfirmationRejectsStalePreviewAndScanRace(t *testing.T) {
	f := newPathChangeFixture(t)
	m := f.media(t, f.root, "电影.mkv", "old")
	writePathChangeFile(t, f.newDir, "电影.mkv", "old")
	in := f.input(t, LibraryPathMigrate)
	f.repos.LibraryPaths.RLock()
	_, err := f.svc.UpdateLibraryRoot(t.Context(), f.lib.ID, f.root.ID, in)
	f.repos.LibraryPaths.RUnlock()
	if err == nil || !strings.Contains(err.Error(), "扫描") {
		t.Fatalf("scan race accepted: %v", err)
	}
	f.media(t, f.root, "newly-indexed.mkv", "more")
	if _, err := f.svc.UpdateLibraryRoot(t.Context(), f.lib.ID, f.root.ID, in); err == nil || !strings.Contains(err.Error(), "重新检查") {
		t.Fatalf("stale preview accepted: %v", err)
	}
	if _, err := f.svc.UpdateLibraryRoot(t.Context(), f.lib.ID, f.root.ID, LibraryRootInput{Path: f.newDir}); err == nil {
		t.Fatal("unconfirmed path update accepted")
	}
	var got model.Media
	f.repos.DB.First(&got, "id = ?", m.ID)
	if got.Path != m.Path {
		t.Fatal("failed confirmation mutated media")
	}
}

func TestLibraryPathMigrationWithOfflineOldRootAndSTRMRemoteSize(t *testing.T) {
	f := newPathChangeFixture(t)
	// The old filesystem no longer exists, as happens after moving a native
	// installation into Docker. Saved relative paths and metadata remain usable.
	if err := f.repos.DB.Model(&model.LibraryRoot{}).Where("id = ?", f.root.ID).Update("path", `C:\OldMedia`).Error; err != nil {
		t.Fatal(err)
	}
	url := "https://example.com/movie.mkv"
	m := model.Media{LibraryID: f.lib.ID, LibraryRootID: f.root.ID, Path: `C:\OldMedia\电影.strm`, Title: "电影", STRMURL: url, SizeBytes: 1 << 30}
	if err := f.repos.DB.Create(&m).Error; err != nil {
		t.Fatal(err)
	}
	writePathChangeFile(t, f.newDir, "电影.strm", url)
	in := f.input(t, LibraryPathMigrate)
	if _, err := f.svc.UpdateLibraryRoot(t.Context(), f.lib.ID, f.root.ID, in); err != nil {
		t.Fatal(err)
	}
	var got model.Media
	f.repos.DB.First(&got, "id = ?", m.ID)
	if got.Path != filepath.Join(f.newDir, "电影.strm") || got.STRMURL != url || got.SizeBytes != m.SizeBytes {
		t.Fatalf("incorrect STRM migration: %+v", got)
	}
}

func TestLibraryPathConfirmationRechecksFilesAndRunningScrapes(t *testing.T) {
	f := newPathChangeFixture(t)
	m := f.media(t, f.root, "电影.mkv", "old")
	writePathChangeFile(t, f.newDir, "电影.mkv", "old")
	in := f.input(t, LibraryPathMigrate)
	writePathChangeFile(t, f.newDir, "电影.mkv", "changed after preview")
	if _, err := f.svc.UpdateLibraryRoot(t.Context(), f.lib.ID, f.root.ID, in); err == nil {
		t.Fatal("accepted modified target after preview")
	}
	writePathChangeFile(t, f.newDir, "电影.mkv", "old")
	in = f.input(t, LibraryPathMigrate)
	if err := f.repos.DB.Create(&model.ScrapeTask{MediaID: m.ID, MediaTitle: m.Title, MediaPath: m.Path, Status: model.ScrapeTaskRunning}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.UpdateLibraryRoot(t.Context(), f.lib.ID, f.root.ID, in); err == nil || !strings.Contains(err.Error(), "刮削") {
		t.Fatalf("running scrape accepted: %v", err)
	}
}

func TestLibraryPathReplacementKeepsLegacyRecordsFromNestedSource(t *testing.T) {
	f := newPathChangeFixture(t)
	nested := filepath.Join(f.oldDir, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	root, err := f.svc.AddLibraryRoot(t.Context(), f.lib.ID, LibraryRootInput{Path: nested})
	if err != nil {
		t.Fatal(err)
	}
	m := f.media(t, *root, "child.mkv", "child")
	if err := f.repos.DB.Model(&model.Media{}).Where("id = ?", m.ID).Update("library_root_id", "").Error; err != nil {
		t.Fatal(err)
	}
	in := f.input(t, LibraryPathReplace)
	if _, err := f.svc.UpdateLibraryRoot(t.Context(), f.lib.ID, f.root.ID, in); err != nil {
		t.Fatal(err)
	}
	var got model.Media
	if err := f.repos.DB.First(&got, "id = ?", m.ID).Error; err != nil || got.Path != m.Path {
		t.Fatalf("nested source index was removed: %+v %v", got, err)
	}
}

func TestLibraryPathChangeRollsBackMediaAndRootTogether(t *testing.T) {
	for _, mode := range []string{LibraryPathMigrate, LibraryPathReplace} {
		t.Run(mode, func(t *testing.T) {
			f := newPathChangeFixture(t)
			m := f.media(t, f.root, "电影.mkv", "same")
			if err := f.repos.DB.Create(&model.PlaybackHistory{MediaID: m.ID, UserID: "user", PositionMs: 321}).Error; err != nil {
				t.Fatal(err)
			}
			writePathChangeFile(t, f.newDir, "电影.mkv", "same")
			in := f.input(t, mode)
			callback := "test:fail_root_commit"
			if err := f.repos.DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table == "library_roots" {
					tx.AddError(errors.New("injected root update failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { f.repos.DB.Callback().Update().Remove(callback) })
			if _, err := f.svc.UpdateLibraryRoot(t.Context(), f.lib.ID, f.root.ID, in); err == nil {
				t.Fatal("expected transactional failure")
			}
			var got model.Media
			if err := f.repos.DB.First(&got, "id = ?", m.ID).Error; err != nil {
				t.Fatal(err)
			}
			root, _ := f.repos.Library.FindRootByID(t.Context(), f.lib.ID, f.root.ID)
			lib, _ := f.repos.Library.FindByID(t.Context(), f.lib.ID)
			if got.Path != m.Path || root.Path != f.oldDir || lib.Path != f.oldDir {
				t.Fatalf("partial commit: media=%s root=%s library=%s", got.Path, root.Path, lib.Path)
			}
			var history model.PlaybackHistory
			if err := f.repos.DB.Where("media_id = ?", m.ID).First(&history).Error; err != nil || history.PositionMs != 321 {
				t.Fatalf("history was not rolled back: %+v %v", history, err)
			}
		})
	}
}

func TestLibraryPathReplacementRejectsForeignIndexInNewDirectory(t *testing.T) {
	f := newPathChangeFixture(t)
	if err := f.repos.DB.Create(&model.Media{LibraryID: "another-library", Path: filepath.Join(f.newDir, "foreign.mkv"), Title: "foreign"}).Error; err != nil {
		t.Fatal(err)
	}
	preview, err := f.svc.PreviewLibraryRootPath(t.Context(), f.lib.ID, f.root.ID, LibraryRootInput{Path: f.newDir, ChangeMode: LibraryPathReplace})
	if err != nil || preview.CanApply || preview.Conflicts != 1 {
		t.Fatalf("foreign index not protected: %+v %v", preview, err)
	}
}

func TestLibraryPathRelativePathsSupportBinaryMigration(t *testing.T) {
	for _, tc := range []struct {
		root, media, relative string
		valid                 bool
	}{
		{`C:\Media`, `C:\Media\电影\A.mkv`, "电影/A.mkv", true},
		{"/old/media", "/old/media/电影/A.mkv", "电影/A.mkv", true},
		{"/old/media", "/old/media2/A.mkv", "", false},
	} {
		got, ok := relativeMediaPath(model.Media{Path: tc.media}, tc.root)
		if got != tc.relative || ok != tc.valid {
			t.Fatalf("relative(%q,%q)=%q,%v", tc.root, tc.media, got, ok)
		}
	}
}
