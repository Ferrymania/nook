package service

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
	"go.uber.org/zap"
)

func TestLibraryCreationSettingsAndExplicitCreate(t *testing.T) {
	db := newServiceTestDB(t)
	repos := repository.New(db)
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	prewarm := false
	settings := model.LibraryScrapeSettings{AutoOnScan: true, Providers: []string{"bangumi", "tmdb"}, Language: "en-US", DelayMinMS: 100, DelayMaxMS: 200, SegmentPrewarmEnabled: &prewarm}
	options := LibraryCreateOptions{CreateOnly: true, ScrapeSettings: &settings, AutoProbe: true}
	firstPath, secondPath := t.TempDir(), t.TempDir()
	lib, err := svc.CreateLibraryWithOptions(t.Context(), "收藏", "movie", "original-cover", []LibraryRootInput{{Path: firstPath}}, options)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := repos.Library.FindByID(t.Context(), lib.ID)
	if err != nil || stored == nil || !stored.AutoProbe || !reflect.DeepEqual(stored.ScrapeSettings, &settings) || len(stored.Roots) != 1 {
		t.Fatalf("creation did not persist settings and roots together: %#v %v", stored, err)
	}
	changed := settings
	changed.Language = "ja-JP"
	options.ScrapeSettings = &changed
	if _, err := svc.CreateLibraryWithOptions(t.Context(), " 收藏 ", "movie", "replaced-cover", []LibraryRootInput{{Path: secondPath}}, options); !errors.Is(err, ErrLibraryAlreadyExists) {
		t.Fatalf("explicit creation silently appended: %v", err)
	}
	stored, err = repos.Library.FindByID(t.Context(), lib.ID)
	if err != nil || stored.CoverURL != "original-cover" || len(stored.Roots) != 1 || !reflect.DeepEqual(stored.ScrapeSettings, &settings) {
		t.Fatalf("conflict modified the existing library: %#v %v", stored, err)
	}
	invalid := settings
	invalid.Providers = []string{"fanart"}
	if _, err := svc.CreateLibraryWithOptions(t.Context(), "无效", "movie", "", []LibraryRootInput{{Path: secondPath}}, LibraryCreateOptions{CreateOnly: true, ScrapeSettings: &invalid}); err == nil {
		t.Fatal("invalid creation settings accepted")
	}
	var count int64
	if err := db.Model(&model.Library{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("invalid settings left a library behind: %d %v", count, err)
	}
	if err := db.Model(&model.LibraryRoot{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("invalid settings left roots behind: %d %v", count, err)
	}
}

func TestLibraryCreationBatchRollsBackConflictAndSavesSettings(t *testing.T) {
	db := newServiceTestDB(t)
	repos := repository.New(db)
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	parent := t.TempDir()
	for _, name := range []string{"a-new", "z-existing"} {
		if err := os.Mkdir(filepath.Join(parent, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	original, err := svc.CreateLibrary(t.Context(), "z-existing", t.TempDir(), "movie")
	if err != nil {
		t.Fatal(err)
	}
	settings := model.LibraryScrapeSettings{Providers: []string{"tmdb"}, Language: "zh-TW", DelayMinMS: 0, DelayMaxMS: 0}
	options := LibraryCreateOptions{CreateOnly: true, ScrapeSettings: &settings, AutoProbe: true}
	if _, err := svc.CreateLibrariesPerSubfolderWithOptions(t.Context(), parent, "movie", "", options); !errors.Is(err, ErrLibraryAlreadyExists) {
		t.Fatalf("batch conflict = %v", err)
	}
	libraries, err := repos.Library.List(t.Context())
	if err != nil || len(libraries) != 1 || libraries[0].ID != original.ID || len(libraries[0].Roots) != 1 {
		t.Fatalf("partial batch escaped rollback: %#v %v", libraries, err)
	}
	if err := os.Rename(filepath.Join(parent, "z-existing"), filepath.Join(parent, "b-new")); err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateLibrariesPerSubfolderWithOptions(t.Context(), parent, "movie", "", options)
	if err != nil || len(created) != 2 {
		t.Fatalf("batch create = %#v %v", created, err)
	}
	for _, lib := range created {
		stored, err := repos.Library.FindByID(t.Context(), lib.ID)
		if err != nil || stored == nil || !stored.AutoProbe || !reflect.DeepEqual(stored.ScrapeSettings, &settings) || len(stored.Roots) != 1 {
			t.Fatalf("batch settings = %#v %v", stored, err)
		}
	}
}
