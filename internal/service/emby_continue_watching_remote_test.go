package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
	"go.uber.org/zap"
)

func TestContinueWatchingMountedEpisodesUseNookProgressAndPermissions(t *testing.T) {
	var hits atomic.Int32
	episode := func(id string, index int) map[string]any {
		return map[string]any{
			"Id": id, "Name": id, "Type": "Episode", "SeriesId": "show", "SeasonId": "season", "ParentId": "season",
			"ParentIndexNumber": 1, "IndexNumber": index, "RunTimeTicks": 6000000000,
			// A shared upstream account must never supply this user's progress.
			"UserData": map[string]any{"Played": true, "PlaybackPositionTicks": 5990000000, "IsFavorite": true},
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/emby/Users/upstream/Items/ep1":
			json.NewEncoder(w).Encode(episode("ep1", 1))
		case "/emby/Users/upstream/Items/ep2":
			json.NewEncoder(w).Encode(episode("ep2", 2))
		case "/emby/Users/upstream/Items/ep3":
			json.NewEncoder(w).Encode(episode("ep3", 3))
		case "/emby/Users/upstream/Items/show":
			json.NewEncoder(w).Encode(map[string]any{"Id": "show", "Name": "Remote show", "Type": "Series"})
		case "/emby/Users/upstream/Items":
			if r.URL.Query().Get("ParentId") != "show" || r.URL.Query().Get("IncludeItemTypes") != "Episode" {
				t.Errorf("unexpected remote query: %s", r.URL.RawQuery)
			}
			json.NewEncoder(w).Encode(map[string]any{"Items": []map[string]any{episode("ep1", 1), episode("ep2", 2), episode("ep3", 3)}, "TotalRecordCount": 3})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	e := newTestEmbyService(t)
	cfg := &config.Config{}
	remote := NewEmbyRemoteService(cfg, zap.NewNop(), e.repo, NewCryptoService("", zap.NewNop()))
	e.SetEmbyRemote(remote)
	ctx := t.Context()
	raw, _ := json.Marshal(map[string]string{"url": server.URL, "api_key": "fixture-token", "remote_user_id": "upstream"})
	account := &model.StrmAccount{Base: model.Base{ID: "account"}, Name: "Remote", Provider: model.StrmProviderEmbyRemote, Config: string(raw), Enabled: true}
	if err := e.repo.StrmAccount.Create(ctx, account); err != nil {
		t.Fatal(err)
	}
	mount := &model.EmbyMount{Base: model.Base{ID: "mount"}, AccountID: account.ID, RemoteViewID: "view", CollectionType: "tvshows", Enabled: true}
	if err := e.repo.EmbyMount.Create(ctx, mount); err != nil {
		t.Fatal(err)
	}
	for _, uid := range []string{"viewer", "denied"} {
		allowed := `["` + EmbyMountLibraryID(mount) + `","local"]`
		if uid == "denied" {
			allowed = `["no-access"]`
		}
		if err := e.repo.User.Create(ctx, &model.User{Base: model.Base{ID: uid}, Username: uid, PasswordHash: "x", Role: "user", IsActive: true, AllowedLibraryIDs: allowed}); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.repo.Library.Create(ctx, &model.Library{Base: model.Base{ID: "local"}, Name: "Movies", Path: "/media", Type: "movie", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := e.repo.DB.Create(&model.Media{Base: model.Base{ID: "movie"}, LibraryID: "local", Title: "Movie", Path: "/media/movie.mp4"}).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	history := func(uid, id string, completed bool, minute int) {
		t.Helper()
		if err := e.repo.History.Upsert(ctx, &model.PlaybackHistory{UserID: uid, MediaID: id, PositionMs: 14000, DurationMs: 600000, Completed: completed, WatchedAt: now.Add(-time.Duration(minute) * time.Minute)}); err != nil {
			t.Fatal(err)
		}
	}
	ep1, ep2, ep3 := EncodeEmbyRemoteID(mount.ID, "ep1"), EncodeEmbyRemoteID(mount.ID, "ep2"), EncodeEmbyRemoteID(mount.ID, "ep3")
	history("viewer", ep1, false, 3)
	history("viewer", ep2, false, 2)
	history("viewer", "movie", false, 1)
	history("denied", ep2, false, 0)
	// The local first page should not fetch later remote history merely to count it.
	page, err := e.ResumeItems(ctx, "viewer", "", 1, 0)
	if got := continueWatchingIDs(t, page, err); !reflect.DeepEqual(got, []string{"movie"}) || hits.Load() != 0 {
		t.Fatalf("unnecessary remote hydration: %v, hits %d", got, hits.Load())
	}
	out, err := e.ResumeItems(ctx, "viewer", EmbyMountLibraryID(mount), 20, 0)
	if got := continueWatchingIDs(t, out, err); !reflect.DeepEqual(got, []string{ep2}) {
		t.Fatalf("mounted series not grouped: %v", got)
	}
	data := out["Items"].([]map[string]any)[0]["UserData"].(map[string]any)
	if data["Played"] != false || data["PlaybackPositionTicks"] != int64(140000000) || data["IsFavorite"] != false {
		t.Fatalf("upstream account data leaked: %#v", data)
	}
	history("viewer", ep2, true, 0)
	// Completion is authoritative even if an imported history has no position.
	if err := e.repo.DB.Model(&model.PlaybackHistory{}).Where("user_id = ? AND media_id = ?", "viewer", ep2).Update("position_ms", 0).Error; err != nil {
		t.Fatal(err)
	}
	out, err = e.ResumeItems(ctx, "viewer", EncodeEmbyRemoteID(mount.ID, "show"), 20, 0)
	if got := continueWatchingIDs(t, out, err); !reflect.DeepEqual(got, []string{ep3}) {
		t.Fatalf("completed episode did not advance: %v", got)
	}
	data = out["Items"].([]map[string]any)[0]["UserData"].(map[string]any)
	if data["Played"] != false || data["PlaybackPositionTicks"] != int64(0) {
		t.Fatalf("unstarted episode has upstream progress: %#v", data)
	}
	history("viewer", ep3, true, -1)
	out, err = e.ResumeItems(ctx, "viewer", EmbyMountLibraryID(mount), 20, 0)
	if got := continueWatchingIDs(t, out, err); len(got) != 0 {
		t.Fatalf("completed remote show resurrected old partial history: %v", got)
	}
	before := hits.Load()
	out, err = e.ResumeItems(ctx, "denied", "", 20, 0)
	if got := continueWatchingIDs(t, out, err); len(got) != 0 || hits.Load() != before {
		t.Fatalf("denied mount fetched or leaked: %v", got)
	}
}
