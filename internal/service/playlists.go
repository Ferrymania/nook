package service

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Ferrymania/nook/internal/model"
)

var (
	ErrPlaylistForbidden        = errors.New("无权操作此播放列表")
	ErrPlaylistInvalid          = errors.New("播放列表名称或条目无效")
	ErrPlaylistMediaUnavailable = errors.New("影片不存在或无权访问")
)

type PlaylistEntry struct {
	model.PlaylistItem
	Media model.Media `json:"media"`
}
type PlaylistDetail struct {
	Playlist model.Playlist  `json:"playlist"`
	Items    []model.Media   `json:"items"`
	Entries  []PlaylistEntry `json:"entries"`
	CanEdit  bool            `json:"can_edit"`
}
type PlaylistSummary struct {
	model.Playlist
	ItemCount     int  `json:"item_count"`
	ContainsMedia bool `json:"contains_media"`
}

func playlistName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 128 {
		return "", ErrPlaylistInvalid
	}
	return name, nil
}
func cleanPlaylistIDs(ids []string) ([]string, error) {
	if len(ids) > 1000 {
		return nil, ErrPlaylistInvalid
	}
	result := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || len(id) > 128 {
			return nil, ErrPlaylistInvalid
		}
		if !seen[id] {
			result = append(result, id)
			seen[id] = true
		}
	}
	return result, nil
}
func (p *PlaybackService) CreatePlaylist(ctx context.Context, uid, name string, public bool) (*model.Playlist, error) {
	return p.CreatePlaylistWithItems(ctx, uid, name, public, nil)
}
func (p *PlaybackService) CreatePlaylistWithItems(ctx context.Context, uid, name string, public bool, ids []string) (*model.Playlist, error) {
	name, err := playlistName(name)
	if err != nil {
		return nil, err
	}
	ids, err = cleanPlaylistIDs(ids)
	if err != nil {
		return nil, err
	}
	if err = p.validatePlaylistMediaIDs(ctx, uid, ids); err != nil {
		return nil, err
	}
	pl := &model.Playlist{UserID: uid, Name: name, IsPublic: public}
	err = p.repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(pl).Error; err != nil {
			return err
		}
		for i, id := range ids {
			if err := tx.Create(&model.PlaylistItem{PlaylistID: pl.ID, MediaID: id, Position: i}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return pl, err
}
func (p *PlaybackService) ListPlaylists(ctx context.Context, uid string) ([]model.Playlist, error) {
	return p.repo.Playlist.ListByUser(ctx, uid)
}

// Summaries do not hydrate remote videos: membership and counts need IDs only.
func (p *PlaybackService) PlaylistSummaries(ctx context.Context, uid, mediaID string) ([]PlaylistSummary, error) {
	lists, err := p.ListPlaylists(ctx, uid)
	if err != nil {
		return nil, err
	}
	out := make([]PlaylistSummary, len(lists))
	ids := make([]string, 0, len(lists))
	positions := map[string]int{}
	for i, pl := range lists {
		out[i].Playlist = pl
		ids = append(ids, pl.ID)
		positions[pl.ID] = i
	}
	if len(ids) == 0 {
		return out, nil
	}
	var entries []model.PlaylistItem
	if err = p.repo.DB.WithContext(ctx).Where("playlist_id IN ?", ids).Find(&entries).Error; err != nil {
		return nil, err
	}
	mediaIDs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !IsEmbyRemoteID(entry.MediaID) {
			mediaIDs = append(mediaIDs, entry.MediaID)
		}
	}
	var rows []model.Media
	if len(mediaIDs) > 0 {
		if err = p.repo.DB.WithContext(ctx).Where("id IN ?", mediaIDs).Find(&rows).Error; err != nil {
			return nil, err
		}
	}
	visible := map[string]bool{}
	v := UserDefaultMediaVisibility(ctx, p.repo, uid)
	for i := range rows {
		visible[rows[i].ID] = v.Allows(&rows[i])
	}
	for _, entry := range entries {
		allowed := visible[entry.MediaID]
		if p.remote != nil && IsEmbyRemoteID(entry.MediaID) {
			mountID, _, _ := DecodeEmbyRemoteID(entry.MediaID)
			mount, _, _ := p.remote.ResolveMount(ctx, mountID)
			allowed = mount != nil && EmbyMountLibraryAllowed(v, mount)
		}
		if allowed {
			i := positions[entry.PlaylistID]
			out[i].ItemCount++
			if entry.MediaID == mediaID {
				out[i].ContainsMedia = true
			}
		}
	}
	return out, nil
}

// Validate local batches with one media query and one visibility snapshot.
func (p *PlaybackService) validatePlaylistMediaIDs(ctx context.Context, uid string, ids []string) error {
	local := []string{}
	for _, id := range ids {
		if IsEmbyRemoteID(id) {
			if _, err := p.playlistMedia(ctx, uid, id); err != nil {
				return err
			}
		} else {
			local = append(local, id)
		}
	}
	if len(local) == 0 {
		return nil
	}
	var rows []model.Media
	if err := p.repo.DB.WithContext(ctx).Where("id IN ?", local).Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) != len(local) {
		return ErrPlaylistMediaUnavailable
	}
	visibility := UserDefaultMediaVisibility(ctx, p.repo, uid)
	for i := range rows {
		if !visibility.Allows(&rows[i]) {
			return ErrPlaylistMediaUnavailable
		}
	}
	return nil
}

func (p *PlaybackService) playlistMedia(ctx context.Context, uid, id string) (*model.Media, error) {
	v := UserDefaultMediaVisibility(ctx, p.repo, uid)
	if p.remote != nil && IsEmbyRemoteID(id) {
		mountID, remoteID, _ := DecodeEmbyRemoteID(id)
		mount, acct, err := p.remote.ResolveMount(ctx, mountID)
		if err != nil || mount == nil || acct == nil || !EmbyMountLibraryAllowed(v, mount) {
			return nil, ErrPlaylistMediaUnavailable
		}
		m, err := p.remote.RemoteMediaDetail(ctx, mount, acct, remoteID)
		if err != nil {
			return nil, err
		}
		if m == nil || !v.Allows(m) {
			return nil, ErrPlaylistMediaUnavailable
		}
		return m, nil
	}
	m, err := p.repo.Media.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if m == nil || !v.Allows(m) {
		return nil, ErrPlaylistMediaUnavailable
	}
	return m, nil
}
func (p *PlaybackService) playlistRecord(ctx context.Context, id, uid string, admin, write bool) (*model.Playlist, error) {
	var pl model.Playlist
	if err := p.repo.DB.WithContext(ctx).First(&pl, "id = ?", id).Error; err != nil {
		return nil, err
	}
	if pl.UserID != uid && !admin && (write || !pl.IsPublic) {
		return nil, ErrPlaylistForbidden
	}
	return &pl, nil
}
func (p *PlaybackService) EnsurePlaylistOwner(ctx context.Context, id, uid string, admin bool) error {
	_, err := p.playlistRecord(ctx, id, uid, admin, true)
	return err
}
func (p *PlaybackService) GetPlaylistForUser(ctx context.Context, id, uid string, admin bool) (*PlaylistDetail, error) {
	pl, err := p.playlistRecord(ctx, id, uid, admin, false)
	if err != nil {
		return nil, err
	}
	var rows []model.PlaylistItem
	if err = p.repo.DB.WithContext(ctx).Where("playlist_id = ?", id).Order("position asc, created_at asc, id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := &PlaylistDetail{Playlist: *pl, Items: []model.Media{}, Entries: []PlaylistEntry{}, CanEdit: pl.UserID == uid || admin}
	// Local media are loaded in one query; remote items retain their mounted IDs.
	ids := []string{}
	for _, row := range rows {
		if !IsEmbyRemoteID(row.MediaID) {
			ids = append(ids, row.MediaID)
		}
	}
	var media []model.Media
	if len(ids) > 0 {
		if err = p.repo.DB.WithContext(ctx).Where("id IN ?", ids).Find(&media).Error; err != nil {
			return nil, err
		}
	}
	byID := map[string]model.Media{}
	v := UserDefaultMediaVisibility(ctx, p.repo, uid)
	for _, m := range media {
		if v.Allows(&m) {
			byID[m.ID] = m
		}
	}
	for _, row := range rows {
		m, ok := byID[row.MediaID]
		if !ok && IsEmbyRemoteID(row.MediaID) {
			remote, err := p.playlistMedia(ctx, uid, row.MediaID)
			if err == nil && remote != nil {
				m = *remote
				ok = true
			}
		}
		if ok {
			out.Items = append(out.Items, m)
			out.Entries = append(out.Entries, PlaylistEntry{PlaylistItem: row, Media: m})
		}
	}
	return out, nil
}

// All mutations serialize on the parent row so appends, reorders and removals
// cannot assign conflicting positions or resurrect items during deletion.
func (p *PlaybackService) mutatePlaylist(ctx context.Context, id, uid string, admin bool, fn func(*gorm.DB, *model.Playlist) error) error {
	return p.repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var pl model.Playlist
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&pl, "id = ?", id).Error; err != nil {
			return err
		}
		if pl.UserID != uid && !admin {
			return ErrPlaylistForbidden
		}
		if err := fn(tx, &pl); err != nil {
			return err
		}
		return tx.Model(&pl).Update("updated_at", gorm.Expr("CURRENT_TIMESTAMP")).Error
	})
}
func (p *PlaybackService) AddToPlaylist(ctx context.Context, id, uid, mediaID string, admin bool) error {
	_, err := p.AddPlaylistItems(ctx, id, uid, []string{mediaID}, admin)
	return err
}
func (p *PlaybackService) AddPlaylistItems(ctx context.Context, id, uid string, ids []string, admin bool) (int, error) {
	if err := p.EnsurePlaylistOwner(ctx, id, uid, admin); err != nil {
		return 0, err
	}
	ids, err := cleanPlaylistIDs(ids)
	if err != nil || len(ids) == 0 {
		return 0, ErrPlaylistInvalid
	}
	if err = p.validatePlaylistMediaIDs(ctx, uid, ids); err != nil {
		return 0, err
	}
	added := 0
	err = p.mutatePlaylist(ctx, id, uid, admin, func(tx *gorm.DB, _ *model.Playlist) error {
		var rows []model.PlaylistItem
		if err := tx.Where("playlist_id = ?", id).Find(&rows).Error; err != nil {
			return err
		}
		known := map[string]bool{}
		position := -1
		for _, row := range rows {
			known[row.MediaID] = true
			if row.Position > position {
				position = row.Position
			}
		}
		for _, mid := range ids {
			if known[mid] {
				continue
			}
			position++
			if err := tx.Create(&model.PlaylistItem{PlaylistID: id, MediaID: mid, Position: position}).Error; err != nil {
				return err
			}
			known[mid] = true
			added++
		}
		return nil
	})
	return added, err
}
func (p *PlaybackService) RenamePlaylist(ctx context.Context, id, uid, name string, admin bool) error {
	name, err := playlistName(name)
	if err != nil {
		return err
	}
	return p.mutatePlaylist(ctx, id, uid, admin, func(tx *gorm.DB, pl *model.Playlist) error { return tx.Model(pl).Update("name", name).Error })
}
func (p *PlaybackService) RemoveFromPlaylist(ctx context.Context, id, uid, mediaID string, admin bool) error {
	return p.mutatePlaylist(ctx, id, uid, admin, func(tx *gorm.DB, _ *model.Playlist) error {
		return tx.Unscoped().Where("playlist_id = ? AND media_id = ?", id, mediaID).Delete(&model.PlaylistItem{}).Error
	})
}
func (p *PlaybackService) RemovePlaylistEntries(ctx context.Context, id, uid string, entryIDs []string, admin bool) error {
	if len(entryIDs) == 0 || len(entryIDs) > 1000 {
		return ErrPlaylistInvalid
	}
	return p.mutatePlaylist(ctx, id, uid, admin, func(tx *gorm.DB, _ *model.Playlist) error {
		return tx.Unscoped().Where("playlist_id = ? AND id IN ?", id, entryIDs).Delete(&model.PlaylistItem{}).Error
	})
}
func (p *PlaybackService) ReorderPlaylist(ctx context.Context, id, uid string, order []string, admin bool) error {
	order, err := cleanPlaylistIDs(order)
	if err != nil || len(order) == 0 {
		return ErrPlaylistInvalid
	}
	return p.mutatePlaylist(ctx, id, uid, admin, func(tx *gorm.DB, _ *model.Playlist) error {
		var rows []model.PlaylistItem
		if err := tx.Where("playlist_id = ?", id).Order("position asc, created_at asc, id asc").Find(&rows).Error; err != nil {
			return err
		}
		known := map[string]bool{}
		for _, row := range rows {
			known[row.MediaID] = true
		}
		rank := map[string]int{}
		for _, mid := range order {
			if !known[mid] {
				return ErrPlaylistInvalid
			}
			rank[mid] = len(rank)
		}
		for _, row := range rows {
			if _, ok := rank[row.MediaID]; !ok {
				rank[row.MediaID] = len(rank)
			}
		}
		for _, row := range rows {
			if err := tx.Model(&model.PlaylistItem{}).Where("id = ?", row.ID).Update("position", rank[row.MediaID]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
func (p *PlaybackService) MovePlaylistEntry(ctx context.Context, id, uid, entryID string, index int, admin bool) error {
	if index < 0 {
		return ErrPlaylistInvalid
	}
	return p.mutatePlaylist(ctx, id, uid, admin, func(tx *gorm.DB, _ *model.Playlist) error {
		var rows []model.PlaylistItem
		if err := tx.Where("playlist_id = ?", id).Order("position asc, created_at asc, id asc").Find(&rows).Error; err != nil {
			return err
		}
		from := -1
		for i, row := range rows {
			if row.ID == entryID {
				from = i
				break
			}
		}
		if from < 0 {
			return gorm.ErrRecordNotFound
		}
		if index >= len(rows) {
			return ErrPlaylistInvalid
		}
		entry := rows[from]
		rows = append(rows[:from], rows[from+1:]...)
		rows = append(rows, entry)
		copy(rows[index+1:], rows[index:len(rows)-1])
		rows[index] = entry
		for i, row := range rows {
			if err := tx.Model(&model.PlaylistItem{}).Where("id = ?", row.ID).Update("position", i).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
func (p *PlaybackService) DeletePlaylist(ctx context.Context, id, uid string, admin bool) error {
	return p.mutatePlaylist(ctx, id, uid, admin, func(tx *gorm.DB, pl *model.Playlist) error {
		if err := tx.Unscoped().Where("playlist_id = ?", id).Delete(&model.PlaylistItem{}).Error; err != nil {
			return err
		}
		return tx.Unscoped().Delete(pl).Error
	})
}
