package service

import (
	"fmt"
	"slices"
	"testing"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
	"go.uber.org/zap"
)

func TestMediaVisibilityUsesLibraryPermissionsRegardlessOfAdultMetadata(t *testing.T) {
	t.Parallel()
	db := newServiceTestDB(t, &model.User{}, &model.Library{}, &model.Media{}, &model.Setting{})
	repos := repository.New(db)
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	for _, lib := range []model.Library{
		{Base: model.Base{ID: "allowed"}, Name: "成人 9KG", Path: "/media/adult", Type: "movie", Enabled: true},
		{Base: model.Base{ID: "denied"}, Name: "电影", Path: "/media/movies", Type: "movie", Enabled: true},
	} {
		if err := repos.Library.Create(t.Context(), &lib); err != nil {
			t.Fatal(err)
		}
	}
	for key, value := range map[string]string{"adult.enabled": "false", "adult.library_ids": `["allowed"]`, "adult.require_pin": "true", "adult.pin": "1234"} {
		if err := repos.Setting.Set(t.Context(), key, value); err != nil {
			t.Fatal(err)
		}
	}
	viewer := model.User{Username: "viewer", PasswordHash: "x", Role: "user", AllowedLibraryIDs: `["allowed"]`}
	if err := repos.User.Create(t.Context(), &viewer); err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{LibraryID: "allowed", Title: "普通电影", Path: "/media/adult/a.mkv"},
		{LibraryID: "allowed", Title: "成人电影", Path: "/media/adult/b.mkv", NSFW: true},
		{LibraryID: "denied", Title: "未授权电影", Path: "/media/movies/c.mkv"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	visibility := UserDefaultMediaVisibility(t.Context(), repos, viewer.ID)
	for i := range rows {
		if got, want := visibility.Allows(&rows[i]), rows[i].LibraryID == "allowed"; got != want {
			t.Fatalf("media %q visibility=%t, want %t", rows[i].Title, got, want)
		}
	}
	libs, err := repos.Library.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, lib := range libs {
		if got, want := LibraryVisibleForUser(t.Context(), repos, lib, visibility), lib.ID == "allowed"; got != want {
			t.Fatalf("library %q visibility=%t, want %t", lib.Name, got, want)
		}
	}
	items, err := svc.SearchMediaVisible(t.Context(), "电影", 20, visibility)
	if err != nil {
		t.Fatal(err)
	}
	if got := sortedMediaTitles(items); !slices.Equal(got, []string{"成人电影", "普通电影"}) {
		t.Fatalf("search=%v", got)
	}
	listed, total, err := svc.ListMediaVisible(t.Context(), "allowed", 1, 20, visibility)
	if err != nil || total != 2 || len(listed) != 2 {
		t.Fatalf("allowed list total=%d len=%d err=%v", total, len(listed), err)
	}
	listed, total, err = svc.ListMediaVisible(t.Context(), "denied", 1, 20, visibility)
	if err != nil || total != 0 || len(listed) != 0 {
		t.Fatalf("denied list total=%d len=%d err=%v", total, len(listed), err)
	}
}

func TestSearchMediaVisibleHonorsLargePosterWallLimit(t *testing.T) {
	t.Parallel()

	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	lib := model.Library{Name: "海报墙", Path: "/media/all", Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	rows := make([]model.Media, 260)
	for i := range rows {
		rows[i] = model.Media{
			LibraryID: lib.ID,
			Title:     fmt.Sprintf("节目 %03d", i),
			Path:      fmt.Sprintf("/media/all/show-%03d.mkv", i),
		}
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	items, err := NewMediaService(&config.Config{}, zap.NewNop(), repos).
		SearchMediaVisible(t.Context(), "", 240, MediaVisibility{})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 240 {
		t.Fatalf("large poster wall search returned %d rows, want 240", len(items))
	}
}

func TestSearchMediaVisibleCanReturnHugeLibraryResultsWhenRequested(t *testing.T) {
	t.Parallel()

	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	lib := model.Library{Name: "海量剧集", Path: "/media/huge", Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	const total = 2505
	rows := make([]model.Media, total)
	for i := range rows {
		rows[i] = model.Media{
			LibraryID:  lib.ID,
			Title:      fmt.Sprintf("海量剧集 %04d", i),
			Path:       fmt.Sprintf("/media/huge/show-%04d.mkv", i),
			SeasonNum:  1,
			EpisodeNum: i + 1,
		}
	}
	if err := db.CreateInBatches(&rows, 500).Error; err != nil {
		t.Fatal(err)
	}

	items, err := NewMediaService(&config.Config{}, zap.NewNop(), repos).
		SearchMediaVisible(t.Context(), "海量剧集", total, MediaVisibility{})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != total {
		t.Fatalf("huge search returned %d rows, want %d", len(items), total)
	}

	firstPage, totalRows, err := NewMediaService(&config.Config{}, zap.NewNop(), repos).
		SearchMediaVisiblePage(t.Context(), "海量剧集", 1, 2000, MediaVisibility{})
	if err != nil {
		t.Fatal(err)
	}
	if totalRows != total || len(firstPage) != 2000 {
		t.Fatalf("huge search page 1 len=%d total=%d, want len=2000 total=%d", len(firstPage), totalRows, total)
	}
	secondPage, totalRows, err := NewMediaService(&config.Config{}, zap.NewNop(), repos).
		SearchMediaVisiblePage(t.Context(), "海量剧集", 2, 2000, MediaVisibility{})
	if err != nil {
		t.Fatal(err)
	}
	if totalRows != total || len(secondPage) != total-2000 {
		t.Fatalf("huge search page 2 len=%d total=%d, want len=%d total=%d", len(secondPage), totalRows, total-2000, total)
	}
}

func sortedMediaTitles(rows []model.Media) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Title)
	}
	slices.Sort(out)
	return out
}
