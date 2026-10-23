package service

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/Ferrymania/nook/internal/model"
	"gorm.io/gorm"
)

const EmbyPlaylistsFolderID = "nook-playlists"
const embyPlaylistPrefix = "playlist~"

func EmbyPlaylistID(id string) string { return embyPlaylistPrefix + id }
func DecodeEmbyPlaylistID(id string) (string, bool) {
	id, ok := strings.CutPrefix(id, embyPlaylistPrefix)
	return id, ok && id != ""
}
func (e *EmbyService) playlistService() *PlaybackService {
	return NewPlaybackService(e.log, e.repo).SetEmbyRemote(e.remote)
}
func (e *EmbyService) playlistAdmin(ctx context.Context, uid string) bool {
	u, err := e.repo.User.FindByID(ctx, uid)
	return err == nil && u != nil && u.Role == "admin"
}
func (e *EmbyService) playlistsFolder(ctx context.Context, uid string) (map[string]any, error) {
	lists, err := e.playlistService().ListPlaylists(ctx, uid)
	if err != nil {
		return nil, err
	}
	return map[string]any{"Id": EmbyPlaylistsFolderID, "Name": "播放列表", "Type": "PlaylistsFolder", "CollectionType": "playlists", "IsFolder": true, "ChildCount": len(lists), "ServerId": embyServerID, "UserData": emptyUserData()}, nil
}
func embyPlaylistPayload(pl model.Playlist, count int, edit bool) map[string]any {
	return map[string]any{"Id": EmbyPlaylistID(pl.ID), "Name": pl.Name, "SortName": pl.Name, "Type": "Playlist", "MediaType": "Video", "IsFolder": true, "ServerId": embyServerID, "ParentId": EmbyPlaylistsFolderID, "ChildCount": count, "RecursiveItemCount": count, "DateCreated": pl.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"), "CanDelete": edit, "CanEditItems": edit, "UserData": emptyUserData()}
}
func (e *EmbyService) playlistList(ctx context.Context, p ItemsParams) (map[string]any, error) {
	summaries, err := e.playlistService().PlaylistSummaries(ctx, p.UserID, "")
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(summaries))
	for _, pl := range summaries {
		if len(p.IDs) > 0 && !containsExactString(p.IDs, EmbyPlaylistID(pl.ID)) {
			continue
		}
		if p.SearchTerm != "" && !strings.Contains(strings.ToLower(pl.Name), strings.ToLower(p.SearchTerm)) {
			continue
		}
		items = append(items, embyPlaylistPayload(pl.Playlist, pl.ItemCount, true))
	}
	sort.SliceStable(items, func(i, j int) bool {
		key := "Name"
		if strings.EqualFold(firstCSVValue(p.SortBy), "DateCreated") {
			key = "DateCreated"
		}
		left, right := strings.ToLower(items[i][key].(string)), strings.ToLower(items[j][key].(string))
		if left == right {
			return items[i]["Id"].(string) < items[j]["Id"].(string)
		}
		if strings.EqualFold(p.SortOrder, "Descending") {
			return left > right
		}
		return left < right
	})
	return map[string]any{"Items": pageSlice(items, p.StartIndex, p.Limit), "TotalRecordCount": int64(len(items)), "StartIndex": p.StartIndex}, nil
}
func (e *EmbyService) playlistItem(ctx context.Context, id, uid string) (map[string]any, error) {
	detail, err := e.playlistService().GetPlaylistForUser(ctx, id, uid, e.playlistAdmin(ctx, uid))
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, ErrPlaylistForbidden) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return embyPlaylistPayload(detail.Playlist, len(detail.Items), detail.CanEdit), nil
}
func (e *EmbyService) PlaylistItems(ctx context.Context, id string, p ItemsParams) (map[string]any, error) {
	if decoded, ok := DecodeEmbyPlaylistID(id); ok {
		id = decoded
	}
	detail, err := e.playlistService().GetPlaylistForUser(ctx, id, p.UserID, e.playlistAdmin(ctx, p.UserID))
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(detail.Entries))
	for _, entry := range detail.Entries {
		if p.SearchTerm != "" && !strings.Contains(strings.ToLower(entry.Media.Title), strings.ToLower(p.SearchTerm)) {
			continue
		}
		item, err := e.Item(ctx, entry.MediaID, p.UserID)
		if err != nil {
			return nil, err
		}
		if item == nil {
			continue
		}
		if len(p.IncludeItemTypes) > 0 && !containsItemType(p.IncludeItemTypes, item["Type"].(string)) {
			continue
		}
		item["PlaylistItemId"] = entry.ID
		items = append(items, item)
	}
	return map[string]any{"Items": pageSlice(items, p.StartIndex, p.Limit), "TotalRecordCount": int64(len(items)), "StartIndex": maxInt(0, p.StartIndex)}, nil
}

// Expand series and season selections into concrete episodes in display order.
func (e *EmbyService) PlaylistMediaIDs(ctx context.Context, uid string, ids []string) ([]string, error) {
	ids, err := cleanPlaylistIDs(ids)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, id := range ids {
		item, err := e.Item(ctx, id, uid)
		if err != nil {
			return nil, err
		}
		if item == nil {
			return nil, ErrPlaylistMediaUnavailable
		}
		kind, _ := item["Type"].(string)
		if kind == "Series" || kind == "Season" {
			for offset := 0; ; offset += 500 {
				page, err := e.Items(ctx, ItemsParams{UserID: uid, ParentID: id, IncludeItemTypes: []string{"Episode"}, Recursive: true, Limit: 500, StartIndex: offset})
				if err != nil {
					return nil, err
				}
				rows, _ := page["Items"].([]map[string]any)
				for _, row := range rows {
					mid, _ := row["Id"].(string)
					out = append(out, mid)
				}
				if len(out) > 1000 {
					return nil, ErrPlaylistInvalid
				}
				if len(rows) < 500 {
					break
				}
			}
		} else if kind == "Movie" || kind == "Episode" || kind == "Video" {
			out = append(out, id)
		} else {
			return nil, ErrPlaylistInvalid
		}
	}
	return cleanPlaylistIDs(out)
}

func containsExactString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
