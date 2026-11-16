package service

import (
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/model"
)

func newTelegramStatusFixture(t *testing.T) *telegramControlFixture {
	t.Helper()
	f := newTelegramControlFixture(t)
	if err := f.bot.repo.DB.AutoMigrate(&model.Library{}, &model.LibraryRoot{}, &model.User{}, &model.Media{}); err != nil {
		t.Fatal(err)
	}
	f.bot.SetSessionTracker(NewSessionTrackerService(zap.NewNop()))
	return f
}

func TestTelegramStatusCountsLibrariesUsersAndUniquePlayingUsers(t *testing.T) {
	t.Parallel()
	f := newTelegramStatusFixture(t)
	ctx := t.Context()
	db := f.bot.repo.DB

	libraries := []model.Library{
		{Name: "电影", Path: "/movies", Roots: []model.LibraryRoot{{Path: "/movies"}, {Path: "/movies2"}}},
		{Name: "已禁用", Path: "/disabled"},
		{Name: "已删除", Path: "/deleted"},
	}
	for i := range libraries {
		if err := f.bot.repo.Library.Create(ctx, &libraries[i]); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Model(&libraries[1]).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&libraries[2]).Error; err != nil {
		t.Fatal(err)
	}

	users := []model.User{
		{Username: "admin", Role: "admin"}, {Username: "viewer"},
		{Username: "paused"}, {Username: "stopped"}, {Username: "expired-session"},
		{Username: "browsing"}, {Username: "deleted"},
	}
	for i := range users {
		users[i].PasswordHash = "test-hash"
		if err := f.bot.repo.User.Create(ctx, &users[i]); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Delete(&users[6]).Error; err != nil {
		t.Fatal(err)
	}

	tracker := f.bot.sessions
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	tracker.now = func() time.Time { return now }
	record := func(userIndex int, device string, state PlaybackState) {
		t.Helper()
		user := users[userIndex]
		tracker.RecordPlaybackState(ctx, user.ID, user.Username, device, "", "Nook Web", "", state)
	}
	record(4, "expired", PlaybackState{ItemID: "m1"})
	now = now.Add(realtimePlaybackTTL)
	tracker.RecordActivity(ctx, users[4].ID, users[4].Username, "expired", "", "Nook Web", "")
	for _, device := range []string{"web", "emby"} {
		record(1, device, PlaybackState{ItemID: "m1"})
	}
	record(0, "admin", PlaybackState{ItemID: "m1"})
	record(1, "paused-device", PlaybackState{ItemID: "m1", IsPaused: true})
	record(2, "paused", PlaybackState{ItemID: "m1", IsPaused: true})
	record(3, "stopped", PlaybackState{ItemID: "m1"})
	record(3, "stopped", PlaybackState{ItemID: "m1", Stopped: true})
	record(6, "deleted", PlaybackState{ItemID: "m1"})
	tracker.RecordActivity(ctx, users[5].ID, users[5].Username, "browsing", "", "Nook Web", "")
	tracker.RecordPlaybackState(ctx, "missing-user", "", "orphan", "", "", "", PlaybackState{ItemID: "m1"})

	f.bot.handleUpdate(ctx, "test-token", telegramAdminMessage("/status@NookBot"))
	want := "📊 Nook 站点状态\n\n媒体库数量：2 个\n用户数：6 人\n当前正在播放用户数：2 人\n\n电影：0 部\n剧集：0 部\n总集数：0 集"
	calls := f.snapshot()
	if len(calls) != 1 || calls[0].Method != "sendMessage" || calls[0].Payload["text"] != want {
		t.Fatalf("status calls = %+v, want %q", calls, want)
	}

	// Read live data again on every request; old playback expires even when
	// ordinary browsing activity continues.
	now = now.Add(realtimePlaybackTTL)
	f.bot.handleUpdate(ctx, "test-token", telegramAdminMessage("/status"))
	if text := f.snapshot()[1].Payload["text"].(string); !strings.Contains(text, "当前正在播放用户数：0 人") {
		t.Fatalf("status retained stale playback: %s", text)
	}
}

func TestTelegramStatusWorksWithoutStrmAndRegistersNewDescription(t *testing.T) {
	t.Parallel()
	f := newTelegramStatusFixture(t)
	f.bot.SetStrmService(nil)
	f.bot.handleUpdate(t.Context(), "test-token", telegramAdminMessage("/status"))
	f.waitMessage(t, "媒体库数量：0 个\n用户数：0 人\n当前正在播放用户数：0 人\n\n电影：0 部\n剧集：0 部\n总集数：0 集")
	if err := f.bot.registerAdminCommands(t.Context(), f.bot.config(t.Context()), "", ""); err != nil {
		t.Fatal(err)
	}
	calls := f.snapshot()
	if len(calls) != 2 || calls[1].Method != "setMyCommands" {
		t.Fatalf("status menu depends on STRM: %+v", calls)
	}
}

func TestTelegramStatusCountsMoviesShowsAndEpisodes(t *testing.T) {
	t.Parallel()
	f := newTelegramStatusFixture(t)
	db := f.bot.repo.DB
	libraries := []model.Library{
		{Base: model.Base{ID: "movies"}, Name: "电影", Path: "/films", Type: "movie"},
		{Base: model.Base{ID: "tv"}, Name: "电视剧", Path: "/shows", Type: "tv"},
		{Base: model.Base{ID: "anime"}, Name: "番剧", Path: "/anime", Type: "anime"},
		{Base: model.Base{ID: "variety"}, Name: "综艺", Path: "/variety", Type: "variety"},
		{Base: model.Base{ID: "music"}, Name: "音乐", Path: "/music", Type: "music"},
		{Base: model.Base{ID: "deleted"}, Name: "已删除", Path: "/deleted", Type: "movie"},
	}
	for i := range libraries {
		if err := f.bot.repo.Library.Create(t.Context(), &libraries[i]); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Model(&libraries[1]).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&libraries[5]).Error; err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{LibraryID: "movies", Title: "电影 A", Path: "/films/A/movie-1080p.mkv", TMDbID: 100},
		{LibraryID: "movies", Title: "电影 A", Path: "/films/A/movie-4k.mkv", TMDbID: 100},
		{LibraryID: "movies", Title: "电影 B", Path: "/films/B/movie.mkv", TMDbID: 101},
		{LibraryID: "tv", Title: "剧 A", Path: "/shows/A/Season 01/A.S01E01.1080p.mkv", SeriesID: "show-a", TMDbID: 100, SeasonNum: 1, EpisodeNum: 1},
		{LibraryID: "tv", Title: "剧 A", Path: "/shows/A/Season 01/A.S01E01.4k.mkv", SeriesID: "show-a", TMDbID: 100, SeasonNum: 1, EpisodeNum: 1},
		{LibraryID: "tv", Title: "剧 A", Path: "/shows/A/Season 01/A.S01E02.mkv", SeriesID: "show-a", TMDbID: 100, SeasonNum: 1, EpisodeNum: 2},
		{LibraryID: "tv", Title: "剧 A", Path: "/shows/A/Season 02/A.S02E01.mkv", SeriesID: "show-a", TMDbID: 100, SeasonNum: 2, EpisodeNum: 1},
		{LibraryID: "tv", Title: "剧 A", Path: "/shows/A/Specials/A.S00E01.mkv", SeriesID: "show-a", TMDbID: 100, EpisodeNum: 1},
		{LibraryID: "tv", Title: "剧 A", Path: "/shows/A/Season 01/A.S01E01.5.mkv", SeriesID: "show-a", TMDbID: 100, SeasonNum: 1, EpisodeNum: 1, EpisodeFraction: .5},
		{LibraryID: "tv", Title: "未刮削第1集", Path: "/shows/B/Season 01/B.S01E01.mkv"},
		{LibraryID: "tv", Title: "未刮削第2集", Path: "/shows/B/Season 01/B.S01E02.mkv"},
		{LibraryID: "anime", Title: "未识别番剧", Path: "/anime/C/Season 01/raw-alpha.mkv"},
		{LibraryID: "anime", Title: "未识别番剧", Path: "/anime/C/Season 01/raw-beta.mkv"},
		{LibraryID: "variety", Title: "综艺 D", Path: "/variety/D/Season 01/D.S01E01.mkv", SeasonNum: 1, EpisodeNum: 1},
		{LibraryID: "music", Title: "歌曲", Path: "/music/song.mp3"},
		{LibraryID: "deleted", Title: "已删除库中的电影", Path: "/deleted/movie.mkv"},
		{LibraryID: "missing-library", Title: "孤立条目", Path: "/missing/movie.mkv"},
		{LibraryID: "movies", Title: "已删除电影", Path: "/films/deleted.mkv"},
		{LibraryID: "tv", Title: "剧 A", Path: "/shows/A/Season 01/A.S01E99.mkv", SeriesID: "show-a", SeasonNum: 1, EpisodeNum: 99},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	for i := len(rows) - 2; i < len(rows); i++ {
		if err := db.Delete(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}

	f.bot.handleUpdate(t.Context(), "test-token", telegramAdminMessage("/status"))
	f.waitMessage(t, "电影：2 部\n剧集：4 部\n总集数：10 集")
	// Counts derive from inventory, not stored totals or the scraper's Series
	// table. A subsequent request reflects deletions immediately.
	if err := db.Delete(&rows[13]).Error; err != nil {
		t.Fatal(err)
	}
	f.bot.handleUpdate(t.Context(), "test-token", telegramAdminMessage("/status"))
	f.waitMessage(t, "电影：2 部\n剧集：3 部\n总集数：9 集")
}

func TestTelegramStatusReportsQueryFailureWithoutFabricatedCounts(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		model any
	}{{"libraries", &model.Library{}}, {"users", &model.User{}}, {"media", &model.Media{}}} {
		t.Run(test.name, func(t *testing.T) {
			f := newTelegramStatusFixture(t)
			if err := f.bot.repo.Library.Create(t.Context(), &model.Library{Name: "电影", Path: "/films", Type: "movie"}); err != nil {
				t.Fatal(err)
			}
			if err := f.bot.repo.DB.Migrator().DropTable(test.model); err != nil {
				t.Fatal(err)
			}
			f.bot.handleUpdate(t.Context(), "test-token", telegramAdminMessage("/status"))
			calls := f.snapshot()
			if len(calls) != 1 || calls[0].Payload["text"] != "读取站点状态失败，请稍后重试" {
				t.Fatalf("query failure response: %+v", calls)
			}
		})
	}
}
