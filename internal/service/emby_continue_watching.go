package service

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Ferrymania/nook/internal/model"
)

// Keep the existing bounded history window. A slow mounted server must not
// require hydrating every historical remote item just to render the first page.
const continueWatchingHistoryLimit = 200

type continueWatchingSlot struct {
	watchedAt time.Time
	group     string
	media     *model.Media
	remote    *resumeRemoteFetch
	item      map[string]any
	resolved  bool
}

// continueWatchingItems is the dedicated home feed. IsResumable remains the
// raw in-progress query; only Resume also includes an unstarted next episode.
func (e *EmbyService) continueWatchingItems(ctx context.Context, p ItemsParams) (map[string]any, error) {
	if p.Limit <= 0 || p.Limit > 100 {
		p.Limit = 50
	}
	if p.StartIndex < 0 {
		p.StartIndex = 0
	}
	if strings.TrimSpace(p.UserID) == "" {
		return emptyItemsEnvelope(p.StartIndex), nil
	}
	var histories []model.PlaybackHistory
	if err := e.repo.DB.WithContext(ctx).
		Where("user_id = ? AND (position_ms > 0 OR completed = ?)", p.UserID, true).
		Order("watched_at desc, id asc").Limit(continueWatchingHistoryLimit).
		Find(&histories).Error; err != nil {
		return nil, err
	}
	localIDs := make([]string, 0, len(histories))
	for _, h := range histories {
		if !IsEmbyRemoteID(h.MediaID) {
			localIDs = append(localIDs, h.MediaID)
		}
	}
	var rows []model.Media
	if len(localIDs) > 0 {
		q := e.repo.DB.WithContext(ctx).Where("id IN ?", localIDs)
		if err := e.applyUserMediaVisibility(ctx, q, p.UserID).Find(&rows).Error; err != nil {
			return nil, err
		}
	}
	ctx = e.withPayloadCache(ctx)
	e.prefetchPayloadCache(ctx, rows)
	local := make(map[string]*model.Media, len(rows))
	for i := range rows {
		m := &rows[i]
		if resumeMatchesParent(ctx, e, p.ParentID, m.LibraryID, m.SeriesID, m) {
			local[m.ID] = m
		}
	}
	scopedHistories := make([]model.PlaybackHistory, 0, len(histories))
	for _, h := range histories {
		if local[h.MediaID] != nil {
			scopedHistories = append(scopedHistories, h)
		}
	}
	nextRows, err := e.discoveryService().nextUpFromHistories(ctx, p.UserID, continueWatchingHistoryLimit, e.mediaVisibility(ctx, p.UserID), scopedHistories)
	if err != nil {
		return nil, err
	}
	next := make(map[string]*model.Media, len(nextRows))
	for i := range nextRows {
		m := &nextRows[i]
		if resumeMatchesParent(ctx, e, p.ParentID, m.LibraryID, m.SeriesID, m) {
			next[seriesGroupKey(m)] = m
		}
	}

	slots := make([]*continueWatchingSlot, 0, len(histories))
	seen := map[string]bool{}
	visibility := e.mediaVisibility(ctx, p.UserID)
	for _, h := range histories {
		if m := local[h.MediaID]; m != nil {
			group := "local-item:" + m.ID
			if mediaIsEpisode(m) {
				group = "local-series:" + seriesGroupKey(m)
			}
			if seen[group] {
				continue
			}
			// Claim a series even when it is finished, so old partial histories
			// cannot resurrect an episode after the most recent one was completed.
			seen[group] = true
			if mediaIsEpisode(m) {
				m = next[seriesGroupKey(m)]
			} else if h.Completed {
				continue
			}
			if m != nil {
				slots = append(slots, &continueWatchingSlot{watchedAt: h.WatchedAt, group: group, media: m, resolved: true})
			}
			continue
		}
		if e.remote == nil || !IsEmbyRemoteID(h.MediaID) {
			continue
		}
		mountID, remoteID, _ := DecodeEmbyRemoteID(h.MediaID)
		mount, acct, err := e.remote.ResolveMount(ctx, mountID)
		if err != nil || mount == nil || acct == nil || !EmbyMountLibraryAllowed(visibility, mount) {
			continue
		}
		if p.ParentID != "" && p.ParentID != mount.ID {
			wantMount, _, remoteParent := DecodeEmbyRemoteID(p.ParentID)
			if !remoteParent || wantMount != mount.ID {
				continue
			}
		}
		slots = append(slots, &continueWatchingSlot{watchedAt: h.WatchedAt, remote: &resumeRemoteFetch{hist: h, mount: mount, acct: acct, remoteID: remoteID}})
	}
	sort.SliceStable(slots, func(i, j int) bool { return slots[i].watchedAt.After(slots[j].watchedAt) })
	if err := e.fillContinueWatchingLocal(ctx, p.UserID, slots); err != nil {
		return nil, err
	}
	if p.StartIndex >= len(slots) {
		return map[string]any{"Items": []map[string]any{}, "TotalRecordCount": int64(len(slots)), "StartIndex": p.StartIndex}, nil
	}

	// Remote metadata is fetched only far enough to fill this page after
	// de-duplication. Four at a time, with a shared request budget.
	remoteCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	needed := p.StartIndex + p.Limit
	items := make([]map[string]any, 0, minInt(needed, len(slots)))
	seen = map[string]bool{}
	cursor := 0
	for cursor < len(slots) && len(items) < needed {
		slot := slots[cursor]
		if !slot.resolved {
			pending := make([]*continueWatchingSlot, 0, 4)
			for i := cursor; i < len(slots) && len(pending) < minInt(4, needed-len(items)); i++ {
				if !slots[i].resolved {
					pending = append(pending, slots[i])
				}
			}
			e.fillContinueWatchingRemote(remoteCtx, p.UserID, p.ParentID, pending)
		}
		cursor++
		if slot.group == "" || seen[slot.group] {
			continue
		}
		seen[slot.group] = true
		if slot.item != nil {
			items = append(items, slot.item)
		}
	}
	// Local totals are exact after grouping. As in the raw remote Resume feed,
	// unhydrated remote candidates form an upper bound rather than forcing a
	// full history fetch merely to calculate a count.
	total := len(items)
	for _, slot := range slots[cursor:] {
		if !slot.resolved {
			total++
		} else if slot.group != "" && !seen[slot.group] {
			seen[slot.group] = true
			if slot.item != nil {
				total++
			}
		}
	}
	page := pageSlice(items, p.StartIndex, p.Limit)
	if err := e.mergeRemoteUserData(ctx, p.UserID, page); err != nil {
		return nil, err
	}
	return map[string]any{"Items": page, "TotalRecordCount": int64(total), "StartIndex": p.StartIndex}, nil
}

func (e *EmbyService) fillContinueWatchingLocal(ctx context.Context, userID string, slots []*continueWatchingSlot) error {
	ids := []string{}
	rows := []model.Media{}
	for _, slot := range slots {
		if slot.media != nil {
			ids = append(ids, slot.media.ID)
			rows = append(rows, *slot.media)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	e.prefetchPayloadCache(ctx, rows)
	var histories []model.PlaybackHistory
	if err := e.repo.DB.WithContext(ctx).Where("user_id = ? AND media_id IN ?", userID, ids).Find(&histories).Error; err != nil {
		return err
	}
	byID := map[string]*model.PlaybackHistory{}
	for i := range histories {
		byID[histories[i].MediaID] = &histories[i]
	}
	var favorites []model.Favorite
	if err := e.repo.DB.WithContext(ctx).Where("user_id = ? AND media_id IN ?", userID, ids).Find(&favorites).Error; err != nil {
		return err
	}
	favs := map[string]bool{}
	for _, f := range favorites {
		favs[f.MediaID] = true
	}
	for _, slot := range slots {
		if slot.media == nil {
			continue
		}
		m := slot.media
		h := byID[m.ID]
		position := int64(0)
		if h != nil {
			position = h.PositionMs
			m = mediaWithHistoryDuration(m, h.DurationMs)
		}
		slot.item = e.itemPayload(ctx, m, favs[m.ID], position, false)
		slot.item["UserData"] = applyNookUserData(slot.item["UserData"], h, favs[m.ID])
	}
	return nil
}

func (e *EmbyService) fillContinueWatchingRemote(ctx context.Context, userID, parentID string, slots []*continueWatchingSlot) {
	var group sync.WaitGroup
	for _, slot := range slots {
		group.Add(1)
		go func(slot *continueWatchingSlot) {
			defer group.Done()
			slot.resolved = true
			f := slot.remote
			item, err := e.remote.RemoteItem(ctx, f.mount, f.acct, f.remoteID)
			if err != nil || item == nil || !continueRemoteMatchesParent(parentID, item, f.mount) {
				return
			}
			slot.group = "remote-item:" + remoteItemString(item, "Id")
			seriesID := remoteItemString(item, "SeriesId")
			if remoteItemString(item, "Type") == "Episode" && seriesID != "" {
				slot.group = "remote-series:" + seriesID
				if f.hist.Completed {
					_, remoteSeriesID, ok := DecodeEmbyRemoteID(seriesID)
					if !ok {
						return
					}
					next, err := e.remoteNextUpFromHistories(ctx, userID, f.mount, f.acct, remoteSeriesID, []model.PlaybackHistory{f.hist}, 1)
					if err != nil {
						return
					}
					rows, _ := next["Items"].([]map[string]any)
					if len(rows) == 0 || !continueRemoteMatchesParent(parentID, rows[0], f.mount) {
						return
					}
					item = rows[0]
				}
			} else if f.hist.Completed {
				return
			}
			slot.item = item
		}(slot)
	}
	group.Wait()
}

func continueRemoteMatchesParent(parentID string, item map[string]any, mount *model.EmbyMount) bool {
	if parentID == "" || parentID == mount.ID || parentID == EmbyMountLibraryID(mount) {
		return true
	}
	for _, key := range []string{"Id", "SeriesId", "SeasonId", "ParentId"} {
		if remoteItemString(item, key) == parentID {
			return true
		}
	}
	return false
}
