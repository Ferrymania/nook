package service

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
)

func TestSeriesPreviewMatchesCompleteLibraryGroup(t *testing.T) {
	db := newServiceTestDB(t)
	repos := repository.New(db)
	lib := model.Library{Name: "番剧", Path: "/media/番剧", Type: "anime", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	rows := make([]model.Media, 0, 144)
	for i := 1; i <= 24; i++ {
		added := now.Add(-24 * time.Hour)
		if i == 24 {
			added = now.Add(time.Hour)
		}
		rows = append(rows, model.Media{
			Base:      model.Base{ID: fmt.Sprintf("target-%d", i), CreatedAt: added},
			LibraryID: lib.ID, Title: "甲贺忍法帖", Year: 2005, ScrapeStatus: "matched",
			Rating:    1,
			Path:      fmt.Sprintf("/media/番剧/Basilisk/Season 01/S01E%02d.strm", i),
			SeasonNum: 1, EpisodeNum: i,
		})
		if i == 24 {
			rows[len(rows)-1].Rating = 9
		}
	}
	for i := 1; i <= 120; i++ {
		rows = append(rows, model.Media{
			Base:      model.Base{ID: fmt.Sprintf("other-%d", i), CreatedAt: now},
			LibraryID: lib.ID, Title: "另一部剧", Year: 2026, ScrapeStatus: "matched",
			Path:      fmt.Sprintf("/media/番剧/Another/Season 01/S01E%03d.strm", i),
			SeasonNum: 1, EpisodeNum: i,
		})
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos).
		SetRuntimeCache(NewRuntimeCacheService(&config.Config{}))
	visibility := MediaVisibility{IncludeNSFW: true}
	cards, _, err := svc.ListLibrarySeriesCards(t.Context(), lib.ID, visibility)
	if err != nil {
		t.Fatal(err)
	}
	var target SeriesCard
	for _, card := range cards {
		if card.Rep.Title == "甲贺忍法帖" {
			target = card
		}
	}
	if target.Key == "" || target.Count != 24 {
		t.Fatalf("complete group = %+v, want 24 episodes", target)
	}
	partialKey := groupMediaSeriesCards([]model.Media{rows[23]})[0].Key
	if partialKey == target.Key {
		t.Fatal("fixture must reproduce a different key when only one recent episode is grouped")
	}
	filtered, _, err := svc.ListLibrarySeriesCardsFiltered(t.Context(), lib.ID, visibility, MediaListFilters{RatingMin: 8})
	if err != nil || len(filtered) != 1 || filtered[0].Key != target.Key {
		t.Fatalf("filtered cards must retain their complete group key: %+v err=%v", filtered, err)
	}
	originalOrder := make([]string, len(cards))
	for i, card := range cards {
		originalOrder[i] = card.Key
	}
	// A warm preview and a stale deep link must reuse the existing index. Any
	// additional database access here would panic because the repository is nil.
	svc.repo = nil
	for _, limit := range []int{1, 2, 10} {
		previews, err := svc.ListLibraryPreviews(t.Context(), []model.Library{lib}, visibility, limit)
		if err != nil {
			t.Fatal(err)
		}
		if len(previews) != 1 || len(previews[0].Cards) == 0 {
			t.Fatalf("missing preview: %+v", previews)
		}
		first := previews[0].Cards[0]
		if first.Key != target.Key || first.Count != 24 || first.Rep.ID != target.Rep.ID {
			t.Fatalf("preview must use the complete latest-added group: %+v", first)
		}
	}
	resolved, err := svc.ResolveLibrarySeriesCard(t.Context(), lib.ID, partialKey, "target-24", visibility)
	if err != nil || resolved == nil || resolved.Key != target.Key {
		t.Fatalf("stale link resolved to %+v, err=%v", resolved, err)
	}
	episodes, err := svc.ListLibrarySeriesEpisodes(t.Context(), lib.ID, resolved.Key, visibility)
	if err != nil || len(episodes) != 24 {
		t.Fatalf("resolved episodes=%d err=%v", len(episodes), err)
	}
	unchanged, _, err := svc.ListLibrarySeriesCards(t.Context(), lib.ID, visibility)
	if err != nil {
		t.Fatal(err)
	}
	for i, card := range unchanged {
		if card.Key != originalOrder[i] {
			t.Fatal("preview sorting mutated the shared library cards")
		}
	}
}

func TestResolveLibrarySeriesCardUsesScopedCachedIndex(t *testing.T) {
	svc := NewMediaService(&config.Config{}, zap.NewNop(), nil).
		SetRuntimeCache(NewRuntimeCacheService(&config.Config{}))
	visibility := MediaVisibility{AllowedLibraryIDs: []string{"allowed"}}
	card := SeriesCard{Key: "current", Rep: model.Media{Base: model.Base{ID: "episode"}}, Count: 24}
	svc.cache.SetObject(svc.libraryRowsCacheKey("allowed", visibility), &libraryRowsCacheValue{
		Cards: []SeriesCard{card}, CardIndexes: map[string]int{"current": 0},
		MediaKeys: map[string]string{"episode": "current"},
	}, time.Minute)
	for _, tc := range []struct {
		key, mediaID string
		want         bool
	}{
		{"current", "", true}, {"stale", "episode", true}, {"", "episode", true},
		{"stale", "foreign-episode", false}, {"stale", "deleted-episode", false},
	} {
		got, err := svc.ResolveLibrarySeriesCard(t.Context(), "allowed", tc.key, tc.mediaID, visibility)
		if err != nil || (got != nil) != tc.want || (got != nil && !reflect.DeepEqual(*got, card)) {
			t.Fatalf("key=%q media=%q card=%+v err=%v", tc.key, tc.mediaID, got, err)
		}
	}
}

func TestLatestLibraryCardsDoesNotMutateSharedOrder(t *testing.T) {
	older, newer := time.Now().Add(-time.Hour), time.Now()
	cards := []SeriesCard{{Key: "older", LastAddedAt: &older}, {Key: "newer", LastAddedAt: &newer}}
	latest := latestLibraryCards(cards, 1)
	if len(latest) != 1 || latest[0].Key != "newer" || cards[0].Key != "older" {
		t.Fatalf("latest=%+v shared=%+v", latest, cards)
	}
}

func TestSeriesMediaAnchorHonorsVisibility(t *testing.T) {
	db := newServiceTestDB(t)
	repos := repository.New(db)
	lib := model.Library{Base: model.Base{ID: "allowed"}, Name: "番剧", Path: "/media/番剧", Type: "anime", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]model.Media{
		{Base: model.Base{ID: "safe"}, LibraryID: lib.ID, Title: "安全剧集", Path: "/media/番剧/Safe/S01E01.strm", SeasonNum: 1, EpisodeNum: 1},
		{Base: model.Base{ID: "adult"}, LibraryID: lib.ID, Title: "成人剧集", Path: "/media/番剧/Adult/S01E01.strm", SeasonNum: 1, EpisodeNum: 1, NSFW: true},
	}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos).
		SetRuntimeCache(NewRuntimeCacheService(&config.Config{}))
	for _, tc := range []struct {
		mediaID    string
		visibility MediaVisibility
		want       bool
	}{
		{"safe", MediaVisibility{AllowedLibraryIDs: []string{lib.ID}}, true},
		{"adult", MediaVisibility{IncludeNSFW: true, AllowedLibraryIDs: []string{lib.ID}}, true},
		{"adult", MediaVisibility{AllowedLibraryIDs: []string{lib.ID}}, false},
		{"safe", MediaVisibility{HiddenLibraryIDs: []string{lib.ID}}, false},
		{"safe", MediaVisibility{AllowedLibraryIDs: []string{"another"}}, false},
	} {
		card, err := svc.ResolveLibrarySeriesCard(t.Context(), lib.ID, "obsolete", tc.mediaID, tc.visibility)
		if err != nil || (card != nil) != tc.want {
			t.Fatalf("media=%q visibility=%+v card=%+v err=%v", tc.mediaID, tc.visibility, card, err)
		}
	}
}
