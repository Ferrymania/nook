package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Ferrymania/nook/internal/model"
)

func TestGenreRepairRequiresReliableUnmodifiedNFO(t *testing.T) {
	for _, tc := range []struct {
		name, body, current, want string
		wantErr                   bool
	}{
		{"mixed", `<movie><genre>Drama</genre><genre>自定义</genre><tag>关键词</tag><studio>公司</studio></movie>`, "Drama,自定义,关键词,公司", "Drama,自定义", false},
		{"already clean", `<movie><genre>Drama</genre><tag>关键词</tag></movie>`, "Drama", "", false},
		{"custom database edit", `<movie><genre>Drama</genre><tag>关键词</tag></movie>`, "Drama,关键词,手动分类", "", true},
		{"no genre", `<movie><tag>关键词</tag></movie>`, "关键词", "", true},
		{"truncated", `<movie><title>标题</title><genre>Drama</genre><tag>关键词</tag>`, "Drama,关键词", "", true},
		{"missing", "", "Drama,关键词", "", true},
		{"already exported as genres", `<movie><genre>Drama</genre><genre>关键词</genre></movie>`, "Drama,关键词", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "movie.mkv")
			if tc.body != "" {
				if err := os.WriteFile(nfoPath(path), []byte(tc.body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			row := genreRepairMedia{ID: "id", Path: path, Genres: tc.current, UpdatedAt: time.Now()}
			change, err := planNFOSourcedGenres(row, root, false)
			if (err != nil) != tc.wantErr {
				t.Fatalf("change=%+v, err=%v", change, err)
			}
			if tc.want != "" && (change == nil || change.After != tc.want || change.Before != tc.current) {
				t.Fatalf("change=%+v, want=%q", change, tc.want)
			}
			if tc.want == "" && change != nil {
				t.Fatalf("unexpected change: %+v", change)
			}
		})
	}
}

func TestGenreRepairSeriesUsesShowGenres(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "S01E01.mkv")
	show := `<tvshow><genre>剧情</genre><tag>整剧标签</tag><studio>公司</studio></tvshow>`
	if err := os.WriteFile(filepath.Join(root, "tvshow.nfo"), []byte(show), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nfoPath(path), []byte(`<episodedetails><genre>单集类型</genre><tag>单集标签</tag></episodedetails>`), 0o600); err != nil {
		t.Fatal(err)
	}
	change, err := planNFOSourcedGenres(genreRepairMedia{Path: path, Genres: "剧情,整剧标签,公司"}, root, true)
	if err != nil || change == nil || change.After != "剧情" {
		t.Fatalf("change=%+v, err=%v", change, err)
	}
}

func TestGenreRepairRootRejectsRemoteAndSiblingPaths(t *testing.T) {
	root := t.TempDir()
	library := model.Library{Path: root}
	for _, path := range []string{"cloud://account/a.mkv", root + "-other/a.mkv", filepath.Dir(root)} {
		if got := genreRepairRoot(library, genreRepairMedia{Path: path}); got != "" {
			t.Fatalf("accepted outside path %q under %q", path, got)
		}
	}
	path := filepath.Join(root, "a.mkv")
	if got := genreRepairRoot(library, genreRepairMedia{Path: path}); got != root {
		t.Fatalf("root=%q", got)
	}
}

func TestGenreRepairPreviewApplyAndEmbyFacets(t *testing.T) {
	svc := newTestEmbyService(t)
	db := svc.repo.DB
	root := t.TempDir()
	library := model.Library{Name: "修复测试", Path: root, Type: "movie"}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "movie.mkv")
	body := `<movie><genre>剧情</genre><genre>自定义类型</genre><tag>关键词</tag><studio>公司</studio></movie>`
	if err := os.WriteFile(nfoPath(path), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	m := model.Media{LibraryID: library.ID, Path: path, Title: "原有标题", Genres: "剧情,自定义类型,关键词,公司", Overview: "原有简介", Rating: 8.8, TMDbID: 123, DurationSec: 7200}
	if err := db.Create(&m).Error; err != nil {
		t.Fatal(err)
	}
	plan, err := BuildGenreRepairPlan(t.Context(), db, library.ID)
	if err != nil || len(plan.Changes) != 1 || len(plan.Skipped) != 0 {
		t.Fatalf("plan=%+v, err=%v", plan, err)
	}
	var before model.Media
	if err := db.First(&before, "id = ?", m.ID).Error; err != nil {
		t.Fatal(err)
	}
	if before.Genres != m.Genres {
		t.Fatal("preview changed database")
	}
	// Exercise the actual report round trip, including timestamp precision.
	encoded, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	var reviewed GenreRepairPlan
	if err := json.Unmarshal(encoded, &reviewed); err != nil {
		t.Fatal(err)
	}
	if err := ApplyGenreRepairPlan(t.Context(), db, &reviewed); err != nil {
		t.Fatal(err)
	}
	var after model.Media
	if err := db.First(&after, "id = ?", m.ID).Error; err != nil {
		t.Fatal(err)
	}
	before.Genres = "剧情,自定义类型"
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("changed unrelated fields: before=%+v after=%+v", before, after)
	}
	discovery := svc.discoveryService()
	media := &MediaService{repo: svc.repo}
	facets, err := media.LibraryFacets(t.Context(), library.ID, MediaVisibility{IncludeNSFW: true}, discovery)
	if err != nil || len(facets.Genres) != 2 || findGenre(facets.Genres, "自定义类型") != 1 {
		t.Fatalf("facets=%+v, err=%v", facets, err)
	}
	catalogue, err := svc.Genres(t.Context(), "", library.ID)
	if err != nil || len(similarResponseItems(t, catalogue)) != 2 {
		t.Fatalf("catalogue=%+v, err=%v", catalogue, err)
	}
	item, err := svc.Item(t.Context(), m.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	assertEmbyGenres(t, item, "剧情", "自定义类型")
	if err := ApplyGenreRepairPlan(t.Context(), db, &reviewed); err == nil {
		t.Fatal("stale report was applied twice")
	}
}

func TestGenreRepairRollsBackOnConcurrentEdit(t *testing.T) {
	db := newServiceTestDB(t)
	library := model.Library{Name: "修复测试", Path: t.TempDir(), Type: "movie"}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	plan := &GenreRepairPlan{Version: 1, LibraryID: library.ID}
	var rows []model.Media
	for _, name := range []string{"first", "second"} {
		path := filepath.Join(library.Path, name+".mkv")
		row := model.Media{LibraryID: library.ID, Title: name, Path: path, Genres: "剧情,关键词"}
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
		rows = append(rows, row)
		plan.Changes = append(plan.Changes, GenreRepairChange{MediaID: row.ID, Path: path, Before: row.Genres, After: "剧情", UpdatedAt: row.UpdatedAt})
	}
	if err := db.Model(&rows[1]).Update("title", "并发修改").Error; err != nil {
		t.Fatal(err)
	}
	if err := ApplyGenreRepairPlan(t.Context(), db, plan); err == nil {
		t.Fatal("concurrent edit was overwritten")
	}
	var first model.Media
	if err := db.First(&first, "id = ?", rows[0].ID).Error; err != nil || first.Genres != "剧情,关键词" {
		t.Fatalf("transaction not rolled back: %+v, %v", first, err)
	}
}

func TestGenreRepairPreviewBatchesAndScopesRows(t *testing.T) {
	db := newServiceTestDB(t)
	library := model.Library{Name: "大媒体库", Path: t.TempDir(), Type: "movie"}
	other := model.Library{Name: "其他媒体库", Path: t.TempDir(), Type: "movie"}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	rows := make([]model.Media, 201)
	for i := range rows {
		rows[i] = model.Media{LibraryID: library.ID, Title: "无 NFO", Path: filepath.Join(library.Path, fmt.Sprintf("movie-%d.mkv", i)), Genres: "剧情,关键词"}
	}
	if err := db.CreateInBatches(&rows, 100).Error; err != nil {
		t.Fatal(err)
	}
	foreign := model.Media{LibraryID: other.ID, Title: "其他库", Path: filepath.Join(other.Path, "other.mkv"), Genres: "剧情"}
	deleted := model.Media{LibraryID: library.ID, Title: "已删除", Path: filepath.Join(library.Path, "deleted.mkv"), Genres: "剧情"}
	if err := db.Create(&foreign).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&deleted).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&deleted).Error; err != nil {
		t.Fatal(err)
	}
	plan, err := BuildGenreRepairPlan(t.Context(), db, library.ID)
	if err != nil || len(plan.Changes) != 0 || len(plan.Skipped) != 201 {
		t.Fatalf("plan=%+v, err=%v", plan, err)
	}
	seen := map[string]bool{}
	for _, skipped := range plan.Skipped {
		if seen[skipped.MediaID] || skipped.MediaID == foreign.ID || skipped.MediaID == deleted.ID {
			t.Fatalf("duplicate or out-of-scope row: %+v", skipped)
		}
		seen[skipped.MediaID] = true
	}
}
