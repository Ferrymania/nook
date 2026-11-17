package service

import (
	"context"
	"testing"
	"time"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
	"go.uber.org/zap"
)

func TestRecordProgressPreservesDurationWhenMissing(t *testing.T) {
	db := newServiceTestDB(t, &model.PlaybackHistory{}, &model.Media{})
	repos := repository.New(db)
	userID := "user-1"
	mediaID := "local-movie-1"
	if err := db.Create(&model.PlaybackHistory{
		Base:       model.Base{ID: "hist-1"},
		UserID:     userID,
		MediaID:    mediaID,
		PositionMs: 30_000,
		DurationMs: 120_000,
		WatchedAt:  time.Now(),
	}).Error; err != nil {
		t.Fatalf("seed history: %v", err)
	}

	svc := NewPlaybackService(zap.NewNop(), repos)
	if err := svc.RecordProgress(context.Background(), userID, mediaID, 60_000, 0); err != nil {
		t.Fatalf("RecordProgress: %v", err)
	}

	var hist model.PlaybackHistory
	if err := db.Where("user_id = ? AND media_id = ?", userID, mediaID).First(&hist).Error; err != nil {
		t.Fatalf("find history: %v", err)
	}
	if hist.DurationMs != 120_000 {
		t.Fatalf("expected duration preserved, got %d", hist.DurationMs)
	}
	if hist.PositionMs != 60_000 {
		t.Fatalf("expected position updated, got %d", hist.PositionMs)
	}
}

func TestGetProgressReturnsNilWhenMissing(t *testing.T) {
	db := newServiceTestDB(t, &model.PlaybackHistory{})
	repos := repository.New(db)
	svc := NewPlaybackService(zap.NewNop(), repos)

	row, err := svc.GetProgress(context.Background(), "user-1", "missing-media")
	if err != nil {
		t.Fatalf("GetProgress: %v", err)
	}
	if row != nil {
		t.Fatalf("expected nil progress, got %#v", row)
	}
}

func TestListFavouritesIncludesLocalAndRemoteIDs(t *testing.T) {
	db := newServiceTestDB(t, &model.Favorite{}, &model.Media{})
	repos := repository.New(db)
	userID := "user-1"
	local := model.Media{
		Base:  model.Base{ID: "local-movie-1"},
		Title: "Local Movie",
	}
	if err := db.Create(&local).Error; err != nil {
		t.Fatalf("create local media: %v", err)
	}
	remoteID := EncodeEmbyRemoteID("mount-1", "remote-series-1")
	favs := []model.Favorite{
		{Base: model.Base{CreatedAt: time.Now().Add(-time.Minute)}, UserID: userID, MediaID: remoteID},
		{Base: model.Base{CreatedAt: time.Now()}, UserID: userID, MediaID: local.ID},
	}
	for i := range favs {
		if err := db.Create(&favs[i]).Error; err != nil {
			t.Fatalf("create favorite %d: %v", i, err)
		}
	}

	svc := NewPlaybackService(zap.NewNop(), repos)
	items, err := svc.ListFavourites(context.Background(), userID)
	if err != nil {
		t.Fatalf("ListFavourites: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one hydrated local favourite without remote service, got %d: %#v", len(items), items)
	}
	if items[0].ID != local.ID {
		t.Fatalf("expected local favourite first by created_at desc, got %#v", items[0])
	}
}

func TestPlaybackProgressCompletedThreshold(t *testing.T) {
	tests := []struct {
		name     string
		position int64
		duration int64
		percent  int
		want     bool
	}{
		{name: "zero duration", position: 10_000, duration: 0, want: false},
		{name: "negative duration", position: 10_000, duration: -1, want: false},
		{name: "zero position", position: 0, duration: 20_000, want: false},
		{name: "negative position", position: -1, duration: 20_000, want: false},
		{name: "short clip early", position: 10_000, duration: 20_000, want: false},
		{name: "short clip near end", position: 18_000, duration: 20_000, want: true},
		{name: "last thirty seconds below ninety percent", position: 95_000, duration: 120_000, want: false},
		{name: "movie just below threshold", position: 107_999, duration: 120_000, want: false},
		{name: "movie at ninety percent", position: 108_000, duration: 120_000, want: true},
		{name: "movie with fifty six seconds left", position: 5_497_000, duration: 5_553_000, want: true},
		{name: "fully played", position: 120_000, duration: 120_000, want: true},
		{name: "position exceeds duration", position: 121_000, duration: 120_000, want: true},
		{name: "fractional threshold rounds up", position: 27, duration: 31, want: false},
		{name: "fractional threshold reached", position: 28, duration: 31, want: true},
		{name: "custom threshold below", position: 940_000, duration: 1_000_000, percent: 95, want: false},
		{name: "custom threshold reached", position: 950_000, duration: 1_000_000, percent: 95, want: true},
		{name: "one hundred percent requires end", position: 999_999, duration: 1_000_000, percent: 100, want: false},
		{name: "one hundred percent reached", position: 1_000_000, duration: 1_000_000, percent: 100, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			percent := tt.percent
			if percent == 0 {
				percent = model.DefaultPlayedThresholdPct
			}
			if got := playbackProgressCompleted(tt.position, tt.duration, percent); got != tt.want {
				t.Fatalf("playbackProgressCompleted(%d, %d) = %t, want %t", tt.position, tt.duration, got, tt.want)
			}
		})
	}
}

func TestLibraryPlaybackThresholdIsIndependentAndAppliesToNewReports(t *testing.T) {
	e := newTestEmbyService(t)
	ctx := t.Context()
	web := NewPlaybackService(zap.NewNop(), e.repo)
	mediaService := NewMediaService(e.cfg, zap.NewNop(), e.repo)
	media := []model.Media{}
	for _, percent := range []int{85, 95} {
		lib := &model.Library{Name: "Movies", Path: t.TempDir(), Type: "movie", Enabled: true, PlayedThresholdPct: percent}
		if err := e.repo.Library.Create(ctx, lib); err != nil {
			t.Fatal(err)
		}
		item := model.Media{LibraryID: lib.ID, Title: "Movie", Path: lib.Path + "/movie.mkv", DurationSec: 1000}
		if err := e.repo.DB.Create(&item).Error; err != nil {
			t.Fatal(err)
		}
		media = append(media, item)
	}
	report := func() {
		t.Helper()
		for _, item := range media {
			if err := web.RecordProgress(ctx, "web-viewer", item.ID, 900_000, 1_000_000); err != nil {
				t.Fatal(err)
			}
			if err := e.RecordProgress(ctx, "emby-viewer", item.ID, 9_000_000_000, 10_000_000_000); err != nil {
				t.Fatal(err)
			}
		}
	}
	assertCompleted := func(index int, want bool) {
		t.Helper()
		for _, userID := range []string{"web-viewer", "emby-viewer"} {
			history, err := web.GetProgress(ctx, userID, media[index].ID)
			if err != nil || history == nil || history.Completed != want || history.PositionMs != 900_000 {
				t.Fatalf("%s library %d: history=%#v err=%v, want completed=%t", userID, index, history, err, want)
			}
		}
	}
	report()
	assertCompleted(0, true)
	assertCompleted(1, false)
	percent := 80
	if err := mediaService.UpdateLibraryFields(ctx, media[1].LibraryID, nil, nil, nil, &percent, nil, nil); err != nil {
		t.Fatal(err)
	}
	// Saving a threshold does not rewrite an existing user's history.
	assertCompleted(1, false)
	report()
	assertCompleted(0, true)
	assertCompleted(1, true)
}

func TestPlaybackCompletionAcrossWebAndEmby(t *testing.T) {
	e := newTestEmbyService(t)
	ctx := t.Context()
	user := &model.User{Username: "completion-viewer", Role: "user", IsActive: true}
	if err := e.repo.User.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	library := &model.Library{Name: "Movies", Path: "/movies", Type: "movie", Enabled: true}
	if err := e.repo.Library.Create(ctx, library); err != nil {
		t.Fatal(err)
	}
	web := NewPlaybackService(zap.NewNop(), e.repo)
	clients := []struct {
		name   string
		record func(string, int64, int64) error
	}{
		{name: "web", record: func(mediaID string, position, duration int64) error {
			return web.RecordProgress(ctx, user.ID, mediaID, position, duration)
		}},
		{name: "emby", record: func(mediaID string, position, duration int64) error {
			return e.RecordProgress(ctx, user.ID, mediaID, position*10_000, duration*10_000)
		}},
	}
	for _, client := range clients {
		t.Run(client.name, func(t *testing.T) {
			media := &model.Media{LibraryID: library.ID, Title: client.name, Path: "/movies/" + client.name + ".mkv", DurationSec: 5553}
			if err := e.repo.DB.Create(media).Error; err != nil {
				t.Fatal(err)
			}
			for _, step := range []struct {
				position  int64
				completed bool
			}{
				{position: 4_997_699, completed: false},
				{position: 4_997_700, completed: true},
				{position: 5_497_000, completed: true},
			} {
				if err := client.record(media.ID, step.position, 5_553_000); err != nil {
					t.Fatal(err)
				}
				history, err := web.GetProgress(ctx, user.ID, media.ID)
				if err != nil || history == nil {
					t.Fatalf("get progress: history=%#v err=%v", history, err)
				}
				if history.Completed != step.completed || history.PositionMs != step.position {
					t.Fatalf("position %d: history=%#v, want completed=%t", step.position, history, step.completed)
				}
				out, err := e.ResumeItems(ctx, user.ID, "", 20, 0)
				ids := continueWatchingIDs(t, out, err)
				if step.completed && len(ids) != 0 {
					t.Fatalf("completed movie remains in continue watching: %v", ids)
				}
				if !step.completed && (len(ids) != 1 || ids[0] != media.ID) {
					t.Fatalf("unfinished movie missing from continue watching: %v", ids)
				}
			}
		})
	}
}
