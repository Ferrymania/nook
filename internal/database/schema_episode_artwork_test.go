package database

import (
	"testing"

	"github.com/Ferrymania/nook/internal/testutil"
	"gorm.io/gorm"

	"github.com/Ferrymania/nook/internal/model"
)

func TestLegacyEpisodeArtworkBackfillPreservesExistingImages(t *testing.T) {
	db, err := testutil.Open(t, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Media{}); err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{Path: "old-episode.strm", SeasonNum: 1, EpisodeNum: 1, BackdropURL: "/media/old-thumb.jpg"},
		{Path: "existing-episode.strm", SeasonNum: 1, EpisodeNum: 2, BackdropURL: "old.jpg", EpisodeStillURL: "keep.jpg"},
		{Path: "movie.strm", BackdropURL: "movie.jpg"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if err := backfillLegacyEpisodeStills(db, true); err != nil {
		t.Fatal(err)
	}
	fresh := model.Media{Path: "new-episode.strm", SeasonNum: 1, EpisodeNum: 3, TMDbID: 123, BackdropURL: "https://image.tmdb.org/t/p/w1280/series.jpg"}
	if err := db.Create(&fresh).Error; err != nil {
		t.Fatal(err)
	}
	if err := backfillLegacyEpisodeStills(db, false); err != nil {
		t.Fatal(err)
	}
	var got []model.Media
	if err := db.Order("path").Find(&got).Error; err != nil {
		t.Fatal(err)
	}
	byPath := map[string]string{}
	for _, m := range got {
		byPath[m.Path] = m.EpisodeStillURL
	}
	if byPath["old-episode.strm"] != "/media/old-thumb.jpg" || byPath["existing-episode.strm"] != "keep.jpg" || byPath["movie.strm"] != "" || byPath["new-episode.strm"] != "" {
		t.Fatalf("bad migration: %+v", byPath)
	}
}
