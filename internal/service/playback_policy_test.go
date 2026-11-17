package service

import (
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/Ferrymania/nook/internal/model"
	"go.uber.org/zap"
)

func TestResumeEligibilityBoundaries(t *testing.T) {
	defaults := defaultLibraryPlaybackSettings()
	for _, tt := range []struct {
		name               string
		settings           LibraryPlaybackSettings
		position, duration int64
		completed, want    bool
	}{
		{"below five percent", defaults, 49_999, 1_000_000, false, false},
		{"at five percent", defaults, 50_000, 1_000_000, false, true},
		{"shorter than three minutes", defaults, 90_000, 179_999, false, false},
		{"exactly three minutes", defaults, 9_000, 180_000, false, true},
		{"completed", defaults, 900_000, 1_000_000, true, false},
		{"unknown runtime", defaults, 10_000, 0, false, false},
		{"disabled minimums", LibraryPlaybackSettings{90, 0, 0}, 1, 10_000, false, true},
		{"disabled minimums unknown runtime", LibraryPlaybackSettings{90, 0, 0}, 1, 0, false, true},
		{"fraction below", LibraryPlaybackSettings{90, 5, 0}, 1, 21, false, false},
		{"fraction reached", LibraryPlaybackSettings{90, 5, 0}, 2, 21, false, true},
		{"large runtime without overflow", defaults, math.MaxInt64 / 2, math.MaxInt64, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := &model.PlaybackHistory{PositionMs: tt.position, DurationMs: tt.duration, Completed: tt.completed}
			original := *h
			if got := tt.settings.resumable(h); got != tt.want {
				t.Fatalf("resumable=%t, want %t", got, tt.want)
			}
			_ = tt.settings.resumeHistory(h)
			if !reflect.DeepEqual(original, *h) {
				t.Fatal("resume view mutated raw history")
			}
		})
	}
}

func TestLibraryResumeSettingsAcrossWebAndEmby(t *testing.T) {
	e := newTestEmbyService(t)
	ctx := t.Context()
	web := NewPlaybackService(zap.NewNop(), e.repo)
	lib := &model.Library{Name: "Resume", Path: "/resume", Type: "movie", Enabled: true, MinResumePct: 10, MinResumeDurationSeconds: 300}
	if err := e.repo.Library.Create(ctx, lib); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name                 string
		position, duration   int64
		resumable, completed bool
	}{
		{"below minimum", 99_999, 1_000_000, false, false},
		{"at minimum", 100_000, 1_000_000, true, false},
		{"short video", 100_000, 299_999, false, false},
		{"duration boundary", 30_000, 300_000, true, false},
		{"short video completed", 108_000, 120_000, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := &model.Media{LibraryID: lib.ID, Title: tt.name, Path: "/resume/" + tt.name, DurationSec: int(tt.duration / 1000)}
			if err := e.repo.DB.Create(m).Error; err != nil {
				t.Fatal(err)
			}
			for _, client := range []string{"web", "emby"} {
				uid := client + "-" + tt.name
				var err error
				if client == "web" {
					err = web.RecordProgress(ctx, uid, m.ID, tt.position, tt.duration)
				} else {
					err = e.RecordProgress(ctx, uid, m.ID, tt.position*10_000, tt.duration*10_000)
				}
				if err != nil {
					t.Fatal(err)
				}
				raw, err := web.GetProgress(ctx, uid, m.ID)
				if err != nil || raw == nil || raw.PositionMs != tt.position || raw.Completed != tt.completed {
					t.Fatalf("raw=%+v err=%v", raw, err)
				}
				resume, err := web.GetResumeProgress(ctx, uid, m.ID)
				wantPosition := tt.position
				if !tt.resumable && !tt.completed {
					wantPosition = 0
				}
				if err != nil || resume == nil || resume.PositionMs != wantPosition {
					t.Fatalf("resume=%+v err=%v want position=%d", resume, err, wantPosition)
				}
				item, err := e.Item(ctx, m.ID, uid)
				if err != nil {
					t.Fatal(err)
				}
				ud := item["UserData"].(map[string]any)
				if ud["Played"] != tt.completed || ud["PlaybackPositionTicks"] != wantPosition*10_000 {
					t.Fatalf("user data=%#v", ud)
				}
				for _, feed := range []string{"resume", "raw"} {
					var out map[string]any
					if feed == "resume" {
						out, err = e.ResumeItems(ctx, uid, "", 10, 0)
					} else {
						out, err = e.Items(ctx, ItemsParams{UserID: uid, Filters: []string{"IsResumable"}, Limit: 10})
					}
					ids := continueWatchingIDs(t, out, err)
					if (len(ids) == 1) != tt.resumable {
						t.Fatalf("%s %s ids=%v, want resumable=%t", client, feed, ids, tt.resumable)
					}
				}
			}
		})
	}
	// Changes affect resume responses immediately without editing saved history.
	zero := 0
	mediaService := NewMediaService(e.cfg, zap.NewNop(), e.repo)
	if err := mediaService.UpdateLibraryFields(ctx, lib.ID, nil, nil, nil, nil, &zero, &zero); err != nil {
		t.Fatal(err)
	}
	m := &model.Media{LibraryID: lib.ID, Title: "Reevaluate", Path: "/resume/reevaluate"}
	if err := e.repo.DB.Create(m).Error; err != nil {
		t.Fatal(err)
	}
	if err := web.RecordProgress(ctx, "viewer", m.ID, 1000, 120_000); err != nil {
		t.Fatal(err)
	}
	before, _ := web.GetProgress(ctx, "viewer", m.ID)
	minimum, seconds := 5, 180
	if err := mediaService.UpdateLibraryFields(ctx, lib.ID, nil, nil, nil, nil, &minimum, &seconds); err != nil {
		t.Fatal(err)
	}
	resume, err := web.GetResumeProgress(ctx, "viewer", m.ID)
	if err != nil || resume.PositionMs != 0 {
		t.Fatalf("changed policy resume=%+v err=%v", resume, err)
	}
	after, _ := web.GetProgress(ctx, "viewer", m.ID)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("settings read rewrote history")
	}
}

func TestRecentHistoryHidesMissingMediaAndPreservesRecords(t *testing.T) {
	e := newTestEmbyService(t)
	ctx := t.Context()
	web := NewPlaybackService(zap.NewNop(), e.repo)
	now := time.Now()
	valid := &model.Media{Title: "Visible", Path: "/visible"}
	deleted := &model.Media{Title: "Deleted", Path: "/deleted"}
	for _, m := range []*model.Media{valid, deleted} {
		if err := e.repo.DB.Create(m).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := e.repo.DB.Delete(deleted).Error; err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{valid.ID, deleted.ID, "physically-removed"} {
		if err := e.repo.History.Upsert(ctx, &model.PlaybackHistory{UserID: "viewer", MediaID: id, PositionMs: 90_000, DurationMs: 600_000, WatchedAt: now.Add(time.Duration(i) * time.Minute)}); err != nil {
			t.Fatal(err)
		}
	}
	items, err := web.RecentHistory(ctx, "viewer", 1)
	if err != nil || len(items) != 1 || items[0].Media.ID != valid.ID {
		t.Fatalf("recent=%+v err=%v", items, err)
	}
	remoteID := EncodeEmbyRemoteID("unavailable", "movie")
	if err := e.repo.History.Upsert(ctx, &model.PlaybackHistory{UserID: "viewer", MediaID: remoteID, PositionMs: 90_000, DurationMs: 600_000, WatchedAt: now}); err != nil {
		t.Fatal(err)
	}
	items, err = web.RecentHistory(ctx, "viewer", 50)
	if err != nil || len(items) != 1 {
		t.Fatalf("unavailable remote leaked placeholder: %+v %v", items, err)
	}
	var count int64
	if err := e.repo.DB.Model(&model.PlaybackHistory{}).Where("user_id = ?", "viewer").Count(&count).Error; err != nil || count != 4 {
		t.Fatalf("history was removed: %d %v", count, err)
	}
	if err := e.repo.DB.Unscoped().Model(deleted).Update("deleted_at", nil).Error; err != nil {
		t.Fatal(err)
	}
	items, err = web.RecentHistory(ctx, "viewer", 50)
	if err != nil || len(items) != 2 {
		t.Fatalf("restored media history unavailable: %+v %v", items, err)
	}
}
