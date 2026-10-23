package service

import (
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
	"go.uber.org/zap"
)

func playlistTestService(t *testing.T) (*PlaybackService, *EmbyService) {
	t.Helper()
	db := newServiceTestDB(t)
	repo := repository.New(db)
	for _, id := range []string{"viewer", "other"} {
		if err := repo.User.Create(t.Context(), &model.User{Base: model.Base{ID: id}, Username: id, PasswordHash: "unused", Role: "user", IsActive: true}); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"allowed", "denied"} {
		if err := repo.Library.Create(t.Context(), &model.Library{Base: model.Base{ID: id}, Name: id, Path: "/" + id, Type: "movie", Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"a", "b", "c", "secret"} {
		lib := "allowed"
		if id == "secret" {
			lib = "denied"
		}
		if err := db.Create(&model.Media{Base: model.Base{ID: id}, LibraryID: lib, Title: id, Path: "/" + id + ".mp4"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.User.UpdateFields(t.Context(), "viewer", map[string]any{"allowed_library_ids": `["allowed"]`}); err != nil {
		t.Fatal(err)
	}
	return NewPlaybackService(zap.NewNop(), repo), NewEmbyService(&config.Config{}, zap.NewNop(), repo)
}
func TestPlaylistAtomicCreationVisibilityAndOrder(t *testing.T) {
	p, e := playlistTestService(t)
	ctx := t.Context()
	if _, err := p.CreatePlaylistWithItems(ctx, "viewer", "bad", false, []string{"a", "secret"}); !errors.Is(err, ErrPlaylistMediaUnavailable) {
		t.Fatalf("forbidden media: %v", err)
	}
	lists, _ := p.ListPlaylists(ctx, "viewer")
	if len(lists) != 0 {
		t.Fatal("failed creation left an empty playlist")
	}
	for _, name := range []string{"   ", string(make([]rune, 129))} {
		if _, err := p.CreatePlaylist(ctx, "viewer", name, false); err == nil {
			t.Fatal("invalid name accepted")
		}
	}
	pl, err := p.CreatePlaylistWithItems(ctx, "viewer", " 周末 ", false, []string{"a", "b", "a", "c"})
	if err != nil {
		t.Fatal(err)
	}
	if pl.Name != "周末" {
		t.Fatal(pl.Name)
	}
	added, err := p.AddPlaylistItems(ctx, pl.ID, "viewer", []string{"a", "b"}, false)
	if err != nil || added != 0 {
		t.Fatalf("duplicate: %d %v", added, err)
	}
	if err = p.ReorderPlaylist(ctx, pl.ID, "viewer", []string{"c", "a"}, false); err != nil {
		t.Fatal(err)
	}
	detail, _ := p.GetPlaylistForUser(ctx, pl.ID, "viewer", false)
	ids := []string{}
	for _, m := range detail.Items {
		ids = append(ids, m.ID)
	}
	if !reflect.DeepEqual(ids, []string{"c", "a", "b"}) {
		t.Fatal(ids)
	}
	if err = p.ReorderPlaylist(ctx, pl.ID, "viewer", []string{"b", "missing"}, false); !errors.Is(err, ErrPlaylistInvalid) {
		t.Fatal(err)
	}
	if _, err = p.GetPlaylistForUser(ctx, pl.ID, "other", false); !errors.Is(err, ErrPlaylistForbidden) {
		t.Fatal(err)
	}
	if err = p.RemoveFromPlaylist(ctx, pl.ID, "other", "a", false); !errors.Is(err, ErrPlaylistForbidden) {
		t.Fatal(err)
	}
	if err = p.RemoveFromPlaylist(ctx, pl.ID, "viewer", "a", false); err != nil {
		t.Fatal(err)
	}
	if _, err = p.AddPlaylistItems(ctx, pl.ID, "viewer", []string{"a"}, false); err != nil {
		t.Fatal(err)
	}
	detail, _ = p.GetPlaylistForUser(ctx, pl.ID, "viewer", false)
	if detail.Items[len(detail.Items)-1].ID != "a" {
		t.Fatal("append reused a deleted position")
	}
	if err = p.MovePlaylistEntry(ctx, pl.ID, "viewer", detail.Entries[2].ID, 0, false); err != nil {
		t.Fatal(err)
	}
	page, err := e.PlaylistItems(ctx, EmbyPlaylistID(pl.ID), ItemsParams{UserID: "viewer", StartIndex: 1, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	entries := page["Items"].([]map[string]any)
	if len(entries) != 1 || page["TotalRecordCount"] != int64(3) || entries[0]["PlaylistItemId"] == nil {
		t.Fatalf("bad page: %#v", page)
	}
	// Permissions are re-evaluated when opening an existing list.
	if err = p.repo.User.UpdateFields(ctx, "viewer", map[string]any{"allowed_library_ids": `["denied"]`}); err != nil {
		t.Fatal(err)
	}
	detail, err = p.GetPlaylistForUser(ctx, pl.ID, "viewer", false)
	if err != nil || len(detail.Items) != 0 {
		t.Fatalf("ACL changed but items leaked: %+v %v", detail, err)
	}
	if err = p.DeletePlaylist(ctx, pl.ID, "viewer", false); err != nil {
		t.Fatal(err)
	}
	var count int64
	p.repo.DB.Unscoped().Model(&model.PlaylistItem{}).Where("playlist_id = ?", pl.ID).Count(&count)
	if count != 0 {
		t.Fatal("orphaned entries")
	}
}
func TestPlaylistConcurrentAddsAreIdempotent(t *testing.T) {
	p, _ := playlistTestService(t)
	pl, err := p.CreatePlaylist(t.Context(), "viewer", "queue", false)
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := p.AddPlaylistItems(t.Context(), pl.ID, "viewer", []string{"a", "b", "c"}, false)
			errs <- err
		}()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	detail, err := p.GetPlaylistForUser(t.Context(), pl.ID, "viewer", false)
	if err != nil || len(detail.Entries) != 3 {
		t.Fatalf("duplicates under concurrency: %+v %v", detail, err)
	}
	for i, entry := range detail.Entries {
		if entry.Position != i {
			t.Fatalf("position: %+v", detail.Entries)
		}
	}
}
func TestEmbyPlaylistDiscoveryIsUserScoped(t *testing.T) {
	p, e := playlistTestService(t)
	ctx := t.Context()
	var alphaID string
	for _, name := range []string{"Beta", "Alpha"} {
		pl, err := p.CreatePlaylistWithItems(ctx, "viewer", name, false, []string{"a"})
		if err != nil {
			t.Fatal(err)
		}
		if name == "Alpha" {
			alphaID = pl.ID
		}
	}
	other, err := p.CreatePlaylistWithItems(ctx, "other", "Private", false, []string{"b"})
	if err != nil {
		t.Fatal(err)
	}
	root, err := e.Views(ctx, "viewer")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range root["Items"].([]map[string]any) {
		if item["CollectionType"] == "playlists" {
			found = true
			if item["ChildCount"] != 2 {
				t.Fatal(item)
			}
		}
	}
	if !found {
		t.Fatal("playlist entry missing")
	}
	page, err := e.Items(ctx, ItemsParams{UserID: "viewer", IncludeItemTypes: []string{"Playlist"}, Recursive: true, SortBy: "SortName", Limit: 1, StartIndex: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page["TotalRecordCount"] != int64(2) || page["Items"].([]map[string]any)[0]["Name"] != "Beta" {
		t.Fatal(page)
	}
	selected, err := e.Items(ctx, ItemsParams{UserID: "viewer", IncludeItemTypes: []string{"Playlist"}, IDs: []string{EmbyPlaylistID(alphaID)}, Limit: 50})
	if err != nil || selected["TotalRecordCount"] != int64(1) || selected["Items"].([]map[string]any)[0]["Name"] != "Alpha" {
		t.Fatal(selected, err)
	}
	hidden, err := e.Item(ctx, EmbyPlaylistID(other.ID), "viewer")
	if err != nil || hidden != nil {
		t.Fatalf("private playlist leaked: %v %v", hidden, err)
	}
	rootItem, err := e.Item(ctx, EmbyPlaylistsFolderID, "viewer")
	if err != nil || rootItem["Type"] != "PlaylistsFolder" {
		t.Fatal(rootItem, err)
	}
}

func TestPublicPlaylistIsReadOnlyAndStillAppliesReaderACL(t *testing.T) {
	p, _ := playlistTestService(t)
	ctx := t.Context()
	pl, err := p.CreatePlaylistWithItems(ctx, "other", "Shared", true, []string{"a", "secret"})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := p.GetPlaylistForUser(ctx, pl.ID, "viewer", false)
	if err != nil || detail.CanEdit || len(detail.Items) != 1 || detail.Items[0].ID != "a" {
		t.Fatal(detail, err)
	}
	if err := p.RenamePlaylist(ctx, pl.ID, "viewer", "Changed", false); !errors.Is(err, ErrPlaylistForbidden) {
		t.Fatal(err)
	}
	if _, err := p.AddPlaylistItems(ctx, pl.ID, "viewer", []string{"b"}, false); !errors.Is(err, ErrPlaylistForbidden) {
		t.Fatal(err)
	}
	if err := p.repo.DB.Delete(&model.Media{}, "id = ?", "a").Error; err != nil {
		t.Fatal(err)
	}
	detail, err = p.GetPlaylistForUser(ctx, pl.ID, "viewer", false)
	if err != nil || len(detail.Items) != 0 {
		t.Fatal(detail, err)
	}
}

func TestEmbyPlaylistSeriesAndSeasonExpandIntoOrderedEpisodes(t *testing.T) {
	p, e := playlistTestService(t)
	ctx := t.Context()
	for _, m := range []model.Media{
		{Base: model.Base{ID: "s2e1"}, LibraryID: "allowed", SeriesID: "playlist-show", Title: "Queue show", SeasonNum: 2, EpisodeNum: 1, Path: "/allowed/Queue show/Season 02/S02E01.mp4"},
		{Base: model.Base{ID: "s1e2"}, LibraryID: "allowed", SeriesID: "playlist-show", Title: "Queue show", SeasonNum: 1, EpisodeNum: 2, Path: "/allowed/Queue show/Season 01/S01E02.mp4"},
		{Base: model.Base{ID: "s1e1"}, LibraryID: "allowed", SeriesID: "playlist-show", Title: "Queue show", SeasonNum: 1, EpisodeNum: 1, Path: "/allowed/Queue show/Season 01/S01E01.mp4"},
	} {
		if err := p.repo.DB.Create(&m).Error; err != nil {
			t.Fatal(err)
		}
	}
	item, err := e.Item(ctx, "s1e1", "viewer")
	if err != nil {
		t.Fatal(err)
	}
	for _, selection := range []struct {
		id   string
		want []string
	}{
		{item["SeriesId"].(string), []string{"s1e1", "s1e2", "s2e1"}},
		{item["SeasonId"].(string), []string{"s1e1", "s1e2"}},
	} {
		ids, err := e.PlaylistMediaIDs(ctx, "viewer", []string{selection.id})
		if err != nil || !reflect.DeepEqual(ids, selection.want) {
			t.Fatalf("%s: %v %v", selection.id, ids, err)
		}
	}
}
