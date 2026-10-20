package database

import (
	"testing"
	"time"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/testutil"
	"gorm.io/gorm"
)

func TestSegmentQueueBackfillPreservesKnownHistoryWithoutRequests(t *testing.T) {
	db, err := testutil.Open(t, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatal(err)
	}
	lib := model.Library{Name: "历史媒体库", Path: "/media"}
	if err := db.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	when := time.Now().Add(-24 * time.Hour).Truncate(time.Millisecond)
	for _, found := range []bool{true, false} {
		m := model.Media{LibraryID: lib.ID, Title: "原有影片", Path: "/legacy.strm"}
		if err := db.Create(&m).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&model.MediaSegmentFetch{MediaID: m.ID, Source: "theintrodb", FetchedAt: when, Found: found}).Error; err != nil {
			t.Fatal(err)
		}
		if found {
			if err := db.Create(&model.MediaSegment{MediaID: m.ID, Source: "theintrodb", Kind: "intro", StartMs: 123, EndMs: 456}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	for i := 0; i < 2; i++ {
		if err := backfillSegmentTasks(db); err != nil {
			t.Fatal(err)
		}
	}
	var tasks []model.MediaSegmentTask
	if err := db.Find(&tasks).Error; err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Fatalf("history duplicated: %+v", tasks)
	}
	for _, task := range tasks {
		if task.Trigger != "history" || task.StartedAt != nil || task.LibraryName != lib.Name || task.FinishedAt == nil || !task.FinishedAt.Equal(when) {
			t.Fatalf("invented history: %+v", task)
		}
		if task.Status == "found" && (len(task.Segments) != 1 || task.Segments[0].StartMs != 123) {
			t.Fatalf("missing snapshot: %+v", task)
		}
	}
	if err := db.Delete(&tasks).Error; err != nil {
		t.Fatal(err)
	}
	if err := backfillSegmentTasks(db); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&model.MediaSegmentTask{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("cleared history resurrected: %d %v", count, err)
	}
}
