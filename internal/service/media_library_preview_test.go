package service

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
	"github.com/Ferrymania/nook/internal/testutil"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestSeriesPreviewMatchesCompleteLibraryCards(t *testing.T) {
	db := newServiceTestDB(t)
	repos := repository.New(db)
	lib := model.Library{Name: "番剧", Path: "/media/anime", Type: "anime", Enabled: true}
	other := model.Library{Name: "不可访问", Path: "/media/private", Type: "tv", Enabled: true}
	for _, item := range []*model.Library{&lib, &other} {
		if err := repos.Library.Create(t.Context(), item); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	rows := []model.Media{}
	for show := 0; show < 5; show++ {
		for episode := 1; episode <= 30; episode++ {
			rows = append(rows, model.Media{
				Base: model.Base{ID: fmt.Sprintf("show-%d-ep-%d", show, episode),
					CreatedAt: now.Add(time.Duration(show*100+episode) * time.Minute)},
				LibraryID: lib.ID, LibraryRootID: "root", SeriesID: fmt.Sprintf("stored-show-%d", show),
				Title: fmt.Sprintf("节目%d", show), OriginalName: "original title", EpisodeTitle: "分集名",
				Path:      fmt.Sprintf("/media/anime/节目%d/Season 01/S01E%02d.mkv", show, episode),
				SeasonNum: 1, EpisodeNum: episode, EpisodeFraction: 0.5, ScrapeStatus: "matched",
				TMDbID: show + 100, BangumiID: show + 200, DoubanID: "douban", TheTVDBID: "tvdb",
				PosterURL: "https://images.example/still.jpg", BackdropURL: "backdrop",
				Overview: strings.Repeat("large overview ", 500), Genres: "动画,冒险", Rating: 8.5,
				Year: 2020 + show, ReleaseDate: fmt.Sprintf("%d-01-01", 2020+show),
				STRMURL: "https://stream.example/video", Width: 3840, Height: 2160,
				DurationSec: 1450, SizeBytes: 1000000000, VideoCodec: "hevc", AudioCodec: "aac",
			})
		}
		// The representative is neither the first nor latest episode in SQL order.
		rows[len(rows)-10].PosterURL = "https://images.example/poster.jpg"
	}
	rows = append(rows,
		model.Media{Base: model.Base{ID: "special"}, LibraryID: lib.ID, Title: "节目4 SP",
			Path: "/media/anime/节目4/Extras/Season 01/SP01.mkv", PosterURL: "special.jpg"},
		model.Media{Base: model.Base{ID: "feature"}, LibraryID: lib.ID, Title: "节目4 剧场版",
			Path: "/media/anime/节目4/Movie/feature.mkv", PosterURL: "poster.jpg"},
		// Matched titles in distinct release directories must still merge.
		model.Media{Base: model.Base{ID: "alias-1", CreatedAt: now.Add(24 * time.Hour)}, LibraryID: lib.ID,
			Title: "同一节目", Path: "/media/anime/Release.A/S01E01.mkv", SeasonNum: 1, EpisodeNum: 1,
			ScrapeStatus: "matched", TMDbID: 999, Year: 2026},
		model.Media{Base: model.Base{ID: "alias-2", CreatedAt: now.Add(25 * time.Hour)}, LibraryID: lib.ID,
			Title: "同一节目", Path: "/media/anime/Release.B/S01E02.mkv", SeasonNum: 1, EpisodeNum: 2,
			ScrapeStatus: "matched", TMDbID: 999, Year: 2026},
		model.Media{Base: model.Base{ID: "adult", CreatedAt: now.Add(48 * time.Hour)}, LibraryID: lib.ID,
			Title: "成人节目", Path: "/media/anime/成人节目/S01E01.mkv", SeasonNum: 1, EpisodeNum: 1, NSFW: true},
		model.Media{Base: model.Base{ID: "private"}, LibraryID: other.ID,
			Title: "不可访问", Path: "/media/private/节目/S01E01.mkv", SeasonNum: 1, EpisodeNum: 1},
	)
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	series := model.Series{
		Base:      model.Base{ID: seriesMetadataStorageID(scrapedSeriesPublicID(&rows[0], rows[0].Title))},
		LibraryID: lib.ID, Title: rows[0].Title,
		PosterURL: "series-poster", Overview: "整剧简介", Rating: 9.2, Year: 2019,
	}
	if err := db.Create(&series).Error; err != nil {
		t.Fatal(err)
	}
	for _, includeNSFW := range []bool{false, true} {
		for _, limit := range []int{1, 4, 10} {
			t.Run(fmt.Sprintf("adult=%t/limit=%d", includeNSFW, limit), func(t *testing.T) {
				visibility := MediaVisibility{IncludeNSFW: includeNSFW, AllowedLibraryIDs: []string{lib.ID}}
				baseline := NewMediaService(&config.Config{}, zap.NewNop(), repos)
				all, _, err := baseline.ListLibrarySeriesCards(t.Context(), lib.ID, visibility)
				if err != nil {
					t.Fatal(err)
				}
				want := latestLibraryCards(all, limit)
				svc := NewMediaService(&config.Config{}, zap.NewNop(), repos).
					SetRuntimeCache(NewRuntimeCacheService(&config.Config{}))
				var hydrated int64
				callback := "test:preview-full-rows"
				if err := db.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
					if _, ok := tx.Statement.Dest.(*[]model.Media); ok {
						hydrated += tx.RowsAffected
					}
				}); err != nil {
					t.Fatal(err)
				}
				defer db.Callback().Query().Remove(callback)
				previews, err := svc.ListLibraryPreviews(t.Context(), []model.Library{lib}, visibility, limit)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(previews[0].Cards, want) {
					t.Fatalf("preview differs from complete grouping\ngot: %#v\nwant: %#v", previews[0].Cards, want)
				}
				if hydrated > int64(2*len(want)) || hydrated == 0 {
					t.Fatalf("loaded %d complete rows for %d cards", hydrated, len(want))
				}
				if _, ok := svc.cache.GetObject(svc.libraryRowsCacheKey(lib.ID, visibility)); ok {
					t.Fatal("preview retained the full library episode cache")
				}
				before := hydrated
				if _, err := svc.ListLibraryPreviews(t.Context(), []model.Library{lib}, visibility, limit); err != nil {
					t.Fatal(err)
				}
				if hydrated != before {
					t.Fatal("cached preview queried complete media rows again")
				}
			})
		}
	}
}

// Compare cold previews on the same database: 100 shows, 60 episodes each,
// with per-episode descriptions that the old full-library scan also loads.
func BenchmarkLibrarySeriesPreview(b *testing.B) {
	db, err := testutil.Open(b, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		b.Fatal(err)
	}
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		b.Fatal(err)
	}
	repos := repository.New(db)
	lib := model.Library{Name: "电视剧", Path: "/media/tv", Type: "tv", Enabled: true}
	if err := repos.Library.Create(b.Context(), &lib); err != nil {
		b.Fatal(err)
	}
	rows := make([]model.Media, 0, 6000)
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for show := 0; show < 100; show++ {
		for episode := 1; episode <= 60; episode++ {
			rows = append(rows, model.Media{
				Base: model.Base{ID: fmt.Sprintf("show-%d-episode-%d", show, episode),
					CreatedAt: now.Add(time.Duration(show*100+episode) * time.Minute)},
				LibraryID: lib.ID, Title: fmt.Sprintf("节目%d", show),
				Path:      fmt.Sprintf("/media/tv/节目%d/S01E%02d.mkv", show, episode),
				SeasonNum: 1, EpisodeNum: episode, Year: 2026, TMDbID: show + 1,
				PosterURL: "https://images.example/poster.jpg", Overview: strings.Repeat("节目简介", 256),
				STRMURL: "https://stream.example/video.mkv", VideoCodec: "hevc", AudioCodec: "aac",
			})
		}
	}
	if err := db.CreateInBatches(rows, 100).Error; err != nil {
		b.Fatal(err)
	}
	visibility := MediaVisibility{IncludeNSFW: true}
	b.Run("complete-library", func(b *testing.B) {
		svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			cards, _, err := svc.ListLibrarySeriesCards(b.Context(), lib.ID, visibility)
			if err != nil {
				b.Fatal(err)
			}
			if len(latestLibraryCards(cards, 10)) != 10 {
				b.Fatal("missing preview cards")
			}
		}
	})
	b.Run("lightweight-preview", func(b *testing.B) {
		svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			cards, err := svc.listLibrarySeriesPreviewCards(b.Context(), lib.ID, visibility, 10)
			if err != nil {
				b.Fatal(err)
			}
			if len(cards) != 10 {
				b.Fatal("missing preview cards")
			}
		}
	})
}
