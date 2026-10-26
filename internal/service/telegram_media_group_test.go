package service

import (
	"encoding/json"
	"html"
	"strings"
	"testing"
	"time"

	"github.com/Ferrymania/nook/internal/model"
)

func telegramEpisodeFixture(id string, season, episode int, when time.Time) model.Media {
	return model.Media{Base: model.Base{ID: id, CreatedAt: when}, LibraryID: "library", Title: "同一部剧", Year: 2026, TMDbID: 123, SeasonNum: season, EpisodeNum: episode, ScrapeStatus: "matched"}
}

func TestTelegramEpisodeSummary(t *testing.T) {
	cases := []struct {
		name  string
		media []model.Media
		label string
		count int
	}{
		{"consecutive", []model.Media{{SeasonNum: 1, EpisodeNum: 12}, {SeasonNum: 1, EpisodeNum: 11}}, "S01E11–E12", 2},
		{"gaps", []model.Media{{SeasonNum: 1, EpisodeNum: 1}, {SeasonNum: 1, EpisodeNum: 3}, {SeasonNum: 1, EpisodeNum: 4}}, "S01E01、E03–E04", 3},
		{"seasons", []model.Media{{SeasonNum: 2, EpisodeNum: 1}, {SeasonNum: 0, EpisodeNum: 2}, {SeasonNum: 1, EpisodeNum: 12}}, "S00E02；S01E12；S02E01", 3},
		{"versions", []model.Media{{SeasonNum: 1, EpisodeNum: 11}, {SeasonNum: 1, EpisodeNum: 11}, {SeasonNum: 1, EpisodeNum: 12}}, "S01E11–E12", 2},
		{"fraction", []model.Media{{SeasonNum: 1, EpisodeNum: 11}, {SeasonNum: 1, EpisodeNum: 11, EpisodeFraction: 0.5}, {SeasonNum: 1, EpisodeNum: 12}}, "S01E11、E11.5、E12", 3},
		{"unknown", []model.Media{{SeasonNum: 1, EpisodeNum: 0}}, "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			label, count := telegramEpisodeSummary(tc.media)
			if label != tc.label || count != tc.count {
				t.Fatalf("summary=%q/%d, want %q/%d", label, count, tc.label, tc.count)
			}
		})
	}
}

func TestTelegramMediaGroupIdentityBoundaries(t *testing.T) {
	lib := model.Library{Type: "tv"}
	m := telegramEpisodeFixture("a", 1, 11, time.Now())
	key := telegramMediaSeriesKey(m, lib)
	same := m
	same.Title = "别名"
	same.EpisodeNum = 12
	same.SeasonNum = 2
	if telegramMediaSeriesKey(same, lib) != key {
		t.Fatal("provider identity did not group aliases/seasons")
	}
	for _, tc := range []struct {
		name   string
		change func(*model.Media)
	}{
		{"library", func(m *model.Media) { m.LibraryID = "other" }},
		{"provider", func(m *model.Media) { m.TMDbID = 456 }},
		{"series-id", func(m *model.Media) { m.SeriesID = "another-series" }},
		{"unknown-episode", func(m *model.Media) { m.EpisodeNum = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			other := m
			tc.change(&other)
			if telegramMediaSeriesKey(other, lib) == key {
				t.Fatal("different media merged")
			}
		})
	}
	m.TMDbID = 0
	same = m
	same.Year = 2025
	if telegramMediaSeriesKey(m, lib) == telegramMediaSeriesKey(same, lib) {
		t.Fatal("same title from different years merged without provider identity")
	}
	m.EpisodeNum = 0
	m.SeasonNum = 0
	if telegramMediaSeriesKey(m, model.Library{Type: "movie"}) != "" {
		t.Fatal("film classified as series group")
	}
}

func TestTelegramMediaGroupWaitsForQuietWindowAndMetadata(t *testing.T) {
	start := time.Now().UTC().Truncate(time.Second)
	first := telegramEpisodeFixture("a", 1, 11, start)
	second := telegramEpisodeFixture("c", 1, 12, start.Add(20*time.Second))
	movie := model.Media{Base: model.Base{ID: "b", CreatedAt: start.Add(10 * time.Second)}, LibraryID: "library", Title: "电影", ScrapeStatus: "matched"}
	rows := []model.Media{first, movie, second}
	lib := model.Library{Type: "movie"}
	if _, ready := telegramMediaGroup(rows, telegramMediaCursor{}, lib, start.Add(40*time.Second)); ready {
		t.Fatal("did not wait for burst to finish")
	}
	group, ready := telegramMediaGroup(rows, telegramMediaCursor{}, lib, start.Add(51*time.Second))
	if !ready || len(group) != 2 || group[0].ID != "a" || group[1].ID != "c" {
		t.Fatalf("interleaved group=%+v ready=%v", group, ready)
	}
	rows[2].ScrapeStatus = "pending"
	if _, ready := telegramMediaGroup(rows, telegramMediaCursor{}, lib, start.Add(time.Minute)); ready {
		t.Fatal("included an episode still waiting for metadata")
	}
	if group, ready := telegramMediaGroup(rows, telegramMediaCursor{}, lib, start.Add(6*time.Minute)); !ready || len(group) != 2 {
		t.Fatal("pending metadata blocked group forever")
	}
	rows[2] = telegramEpisodeFixture("c", 1, 12, start.Add(110*time.Second))
	if group, ready := telegramMediaGroup(rows, telegramMediaCursor{}, lib, start.Add(121*time.Second)); !ready || len(group) != 2 {
		t.Fatal("continuous arrivals extended collection beyond bound")
	}
}

func TestTelegramMediaCursorDoesNotSkipInterleavedRows(t *testing.T) {
	when := time.Now().UTC().Truncate(time.Second)
	rows := []model.Media{
		telegramEpisodeFixture("a", 1, 11, when),
		{Base: model.Base{ID: "b", CreatedAt: when}, Title: "电影"},
		telegramEpisodeFixture("c", 1, 12, when),
		telegramEpisodeFixture("d", 1, 1, when),
	}
	cursor := telegramMediaCursor{CreatedAt: when.Add(-time.Minute), MediaID: "old", Destination: "configured"}
	cursor = advanceTelegramMediaCursor(cursor, rows, []model.Media{rows[0], rows[2]})
	if cursor.MediaID != "a" || len(cursor.SentAhead) != 1 || cursor.SentAhead[0].MediaID != "c" {
		t.Fatalf("skipped intervening film: %+v", cursor)
	}
	// The sparse acknowledgements survive restart and are folded into the main
	// cursor only after the intervening film has been sent.
	raw, err := json.Marshal(cursor)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &cursor); err != nil {
		t.Fatal(err)
	}
	cursor = advanceTelegramMediaCursor(cursor, rows[1:], []model.Media{rows[1]})
	if cursor.MediaID != "c" || len(cursor.SentAhead) != 0 {
		t.Fatalf("replayed episode or skipped other series: %+v", cursor)
	}
	// Removing the unsent film must also let the cursor pass the already-sent
	// later episode without producing a duplicate message.
	cursor = telegramMediaCursor{CreatedAt: when, MediaID: "a", SentAhead: []telegramMediaPosition{{CreatedAt: when, MediaID: "c"}}}
	cursor = advanceTelegramMediaCursor(cursor, rows[2:], nil)
	if cursor.MediaID != "c" || len(cursor.SentAhead) != 0 {
		t.Fatalf("deleted intervening row blocked cursor: %+v", cursor)
	}
}

func TestTelegramGroupedCaptionShowsOneRange(t *testing.T) {
	when := time.Now()
	first := telegramEpisodeFixture("a", 1, 11, when)
	second := telegramEpisodeFixture("b", 1, 12, when)
	first.Overview = "一部剧的简介"
	got := formatTelegramMediaNotification(first, model.Library{Name: "华语电视剧", Type: "tv"}, nil, first, second)
	if !strings.Contains(got, "集数：S01E11–E12 · 新增 2 集") || strings.Count(got, "简介") != 2 || strings.Count(got, "Nook 剧集入库通知") != 1 {
		t.Fatalf("grouped caption=%s", got)
	}
}

func TestTelegramGroupedCaptionHonorsLimit(t *testing.T) {
	first := telegramEpisodeFixture("first", 1, 1, time.Now())
	first.Title, first.Overview, first.Genres = strings.Repeat("长剧名", 80), strings.Repeat("剧情😀<&>", 500), strings.Repeat("类型", 100)
	var episodes []model.Media
	for i := 1; i <= telegramMediaBatchLimit; i++ {
		episodes = append(episodes, telegramEpisodeFixture("episode", i, i, first.CreatedAt))
	}
	caption := formatTelegramMediaNotification(first, model.Library{Name: strings.Repeat("媒体库", 100), Type: "tv"}, []map[string]any{{"Type": "Actor", "Name": strings.Repeat("演员", 100)}}, episodes...)
	plain := html.UnescapeString(strings.ReplaceAll(strings.ReplaceAll(caption, "<b>", ""), "</b>", ""))
	if telegramTextLength(plain) > telegramMediaCaptionLimit || !strings.Contains(plain, "新增 200 集") {
		t.Fatalf("caption length=%d: %s", telegramTextLength(plain), caption)
	}
}
