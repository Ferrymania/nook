package handler

import (
	"fmt"
	"net/url"
	"testing"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
	"github.com/Ferrymania/nook/internal/service"
	"github.com/Ferrymania/nook/internal/testutil"
)

func TestSeriesEpisodesRecoversCanonicalCardFromMediaID(t *testing.T) {
	db, err := testutil.Open(t, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	lib := model.Library{Name: "番剧", Type: "anime", Path: "/media/番剧", Enabled: true}
	other := model.Library{Name: "其他", Type: "anime", Path: "/media/其他", Enabled: true}
	for _, item := range []*model.Library{&lib, &other} {
		if err := repos.Library.Create(t.Context(), item); err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i <= 2; i++ {
		if err := repos.Media.Upsert(t.Context(), &model.Media{
			Base: model.Base{ID: fmt.Sprintf("episode-%d", i)}, LibraryID: lib.ID,
			Title: "示例番剧", Year: 2005, ScrapeStatus: "matched",
			Path: fmt.Sprintf("/media/番剧/Example/S01E%02d.strm", i), SeasonNum: 1, EpisodeNum: i,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := repos.Media.Upsert(t.Context(), &model.Media{
		Base: model.Base{ID: "foreign-episode"}, LibraryID: other.ID,
		Title: "示例番剧", Path: "/media/其他/Example/S01E01.strm", SeasonNum: 1, EpisodeNum: 1,
	}); err != nil {
		t.Fatal(err)
	}
	svc := &service.Container{Repo: repos, Media: service.NewMediaService(&config.Config{}, zap.NewNop(), repos)}
	cards := requestLibrarySeries(t, svc, "/api/libraries/"+lib.ID+"/series", lib.ID)
	if len(cards.Items) != 1 {
		t.Fatalf("series=%+v, want one group", cards)
	}
	for _, tc := range []struct {
		key, mediaID string
		want         int
	}{
		{cards.Items[0].Key, "", 2}, {"obsolete", "episode-2", 2}, {"", "episode-1", 2},
		{"obsolete", "foreign-episode", 0}, {"obsolete", "deleted", 0}, {"obsolete", "", 0},
	} {
		query := url.Values{"key": {tc.key}, "media_id": {tc.mediaID}}
		response := requestLibrarySeriesEpisodes(t, svc, "/api/libraries/"+lib.ID+"/series/episodes?"+query.Encode(), lib.ID)
		if response.Total != int64(tc.want) || len(response.Items) != tc.want {
			t.Fatalf("key=%q media=%q response=%+v", tc.key, tc.mediaID, response)
		}
		if tc.want > 0 && (response.Card == nil || response.Card.Key != cards.Items[0].Key || response.Card.Count != 2) {
			t.Fatalf("canonical card missing from response: %+v", response)
		}
		if tc.want == 0 && response.Card != nil {
			t.Fatalf("unresolved media returned a card: %+v", response.Card)
		}
	}
}
