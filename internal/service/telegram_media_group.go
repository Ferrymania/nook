package service

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Ferrymania/nook/internal/model"
)

const (
	telegramMediaBatchLimit   = 200
	telegramSeriesQuietTime   = 30 * time.Second
	telegramSeriesBatchWindow = 2 * time.Minute
)

type telegramMediaPosition struct {
	CreatedAt time.Time `json:"created_at"`
	MediaID   string    `json:"media_id"`
}

func telegramMediaSeriesKey(m model.Media, lib model.Library) string {
	if m.EpisodeNum <= 0 || !mediaIsEpisodic(&m, &lib) {
		return ""
	}
	prefix := m.LibraryID + "|"
	if id := strings.TrimSpace(m.SeriesID); id != "" {
		return prefix + "series:" + id
	}
	if m.TMDbID > 0 {
		return fmt.Sprintf("%stmdb:%d", prefix, m.TMDbID)
	}
	if m.BangumiID > 0 {
		return fmt.Sprintf("%sbangumi:%d", prefix, m.BangumiID)
	}
	if id := strings.TrimSpace(m.TheTVDBID); id != "" {
		return prefix + "tvdb:" + id
	}
	if id := strings.TrimSpace(m.DoubanID); id != "" {
		return prefix + "douban:" + id
	}
	title := strings.TrimSpace(embyEpisodeTitleRE.ReplaceAllString(m.Title, ""))
	title = strings.TrimSpace(embyYearSuffixRE.ReplaceAllString(title, ""))
	if title == "" {
		return prefix + "media:" + m.ID
	}
	return fmt.Sprintf("%stitle:%s|year:%d", prefix, strings.ToLower(strings.Join(strings.Fields(title), " ")), m.Year)
}

func telegramEpisodeSummary(media []model.Media) (string, int) {
	type episode struct {
		season, number int
		fraction       float64
	}
	episodes := make([]episode, 0, len(media))
	seen := map[string]bool{}
	for _, m := range media {
		if m.EpisodeNum <= 0 {
			continue
		}
		key := fmt.Sprintf("%d:%s", m.SeasonNum, FormatEpisodeNumber(m.EpisodeNum, m.EpisodeFraction))
		if seen[key] {
			continue
		}
		seen[key] = true
		episodes = append(episodes, episode{m.SeasonNum, m.EpisodeNum, m.EpisodeFraction})
	}
	sort.Slice(episodes, func(i, j int) bool {
		a, b := episodes[i], episodes[j]
		if a.season != b.season {
			return a.season < b.season
		}
		if a.number != b.number {
			return a.number < b.number
		}
		return a.fraction < b.fraction
	})
	label := func(e episode) string { return fmt.Sprintf("E%02d", e.number) + episodeFractionSuffix(e.fraction) }
	var seasons []string
	for i := 0; i < len(episodes); {
		season := episodes[i].season
		var parts []string
		for i < len(episodes) && episodes[i].season == season {
			start, end := episodes[i], episodes[i]
			i++
			for i < len(episodes) && end.fraction == 0 && episodes[i].fraction == 0 && episodes[i].season == season && episodes[i].number == end.number+1 {
				end = episodes[i]
				i++
			}
			part := label(start)
			if end.number != start.number {
				part += "–" + label(end)
			}
			parts = append(parts, part)
		}
		seasons = append(seasons, fmt.Sprintf("S%02d", season)+strings.Join(parts, "、"))
	}
	return strings.Join(seasons, "；"), len(episodes)
}

func telegramMediaReady(m model.Media, now time.Time) bool {
	return !now.Before(m.CreatedAt.Add(telegramMediaSettleTime)) &&
		((m.ScrapeStatus != "pending" && m.ScrapeStatus != "") || !now.Before(m.CreatedAt.Add(telegramMediaMetadataWait)))
}

// Bounded lookahead can combine episodes even when other films or series are
// interleaved. SentAhead protects those other rows from being skipped by the
// main cursor when a later episode is included in an earlier notification.
func telegramMediaGroup(rows []model.Media, cursor telegramMediaCursor, lib model.Library, now time.Time) ([]model.Media, bool) {
	if len(rows) == 0 {
		return nil, false
	}
	first := rows[0]
	if !telegramMediaReady(first, now) {
		return nil, false
	}
	key := telegramMediaSeriesKey(first, lib)
	if key == "" {
		return []model.Media{first}, true
	}
	sent := map[string]bool{}
	for _, p := range cursor.SentAhead {
		sent[p.MediaID] = true
	}
	end := first.CreatedAt.Add(telegramSeriesBatchWindow)
	latest := first.CreatedAt
	group := make([]model.Media, 0)
	for _, m := range rows {
		if m.CreatedAt.After(end) {
			break
		}
		if sent[m.ID] || m.LibraryID != first.LibraryID || telegramMediaSeriesKey(m, lib) != key {
			continue
		}
		if !telegramMediaReady(m, now) {
			return nil, false
		}
		group = append(group, m)
		if m.CreatedAt.After(latest) {
			latest = m.CreatedAt
		}
	}
	// Let consecutive watcher/batch inserts settle, but do not keep extending
	// a notification forever if episodes continue arriving.
	if now.Before(latest.Add(telegramSeriesQuietTime)) && now.Before(end) {
		return nil, false
	}
	return group, len(group) > 0
}

func advanceTelegramMediaCursor(cursor telegramMediaCursor, rows, delivered []model.Media) telegramMediaCursor {
	acked := make(map[string]telegramMediaPosition, len(cursor.SentAhead)+len(delivered))
	for _, p := range cursor.SentAhead {
		acked[p.MediaID] = p
	}
	for _, m := range delivered {
		acked[m.ID] = telegramMediaPosition{CreatedAt: m.CreatedAt, MediaID: m.ID}
	}
	for _, m := range rows {
		if _, ok := acked[m.ID]; !ok {
			break
		}
		cursor.CreatedAt, cursor.MediaID = m.CreatedAt, m.ID
	}
	cursor.SentAhead = nil
	for _, p := range acked {
		if p.CreatedAt.After(cursor.CreatedAt) || (p.CreatedAt.Equal(cursor.CreatedAt) && p.MediaID > cursor.MediaID) {
			cursor.SentAhead = append(cursor.SentAhead, p)
		}
	}
	sort.Slice(cursor.SentAhead, func(i, j int) bool {
		a, b := cursor.SentAhead[i], cursor.SentAhead[j]
		if a.CreatedAt.Equal(b.CreatedAt) {
			return a.MediaID < b.MediaID
		}
		return a.CreatedAt.Before(b.CreatedAt)
	})
	return cursor
}
