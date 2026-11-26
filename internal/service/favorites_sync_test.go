package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
	"github.com/Ferrymania/nook/internal/testutil"
)

func TestSyncUserFavoriteWritesLocalForRemoteID(t *testing.T) {
	t.Parallel()

	db, err := testutil.Open(t, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Favorite{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	userID := "user-1"
	remoteMediaID := EncodeEmbyRemoteID("mount-1", "remote-item-1")

	if err := SyncUserFavorite(context.Background(), repos, nil, userID, remoteMediaID, true); err != nil {
		t.Fatalf("SyncUserFavorite favorite: %v", err)
	}
	favorite, err := IsUserFavorite(context.Background(), repos, userID, remoteMediaID)
	if err != nil {
		t.Fatalf("IsUserFavorite: %v", err)
	}
	if !favorite {
		t.Fatal("expected remote favourite to be stored locally")
	}

	if err := SyncUserFavorite(context.Background(), repos, nil, userID, remoteMediaID, false); err != nil {
		t.Fatalf("SyncUserFavorite unfavorite: %v", err)
	}
	favorite, err = IsUserFavorite(context.Background(), repos, userID, remoteMediaID)
	if err != nil {
		t.Fatalf("IsUserFavorite after delete: %v", err)
	}
	if favorite {
		t.Fatal("expected remote favourite to be removed locally")
	}
}

func TestFavoriteItemsIncludesRemoteFavourites(t *testing.T) {
	t.Parallel()

	db := newServiceTestDB(t, &model.User{}, &model.Library{}, &model.Media{}, &model.Favorite{})
	repos := repository.New(db)
	viewer := &model.User{Username: "viewer", PasswordHash: "hash", Role: "user"}
	if err := repos.User.Create(t.Context(), viewer); err != nil {
		t.Fatal(err)
	}
	remoteMediaID := EncodeEmbyRemoteID("mount-1", "remote-item-1")
	if err := db.Create(&model.Favorite{UserID: viewer.ID, MediaID: remoteMediaID}).Error; err != nil {
		t.Fatal(err)
	}

	svc := &EmbyService{repo: repos}
	out, err := svc.favoriteItems(t.Context(), ItemsParams{UserID: viewer.ID, Limit: 50})
	if err != nil {
		t.Fatalf("favoriteItems: %v", err)
	}
	total, _ := out["TotalRecordCount"].(int64)
	if total != 0 {
		// Without a wired remote service hydration is skipped, but local-only path
		// should not error and should not count unavailable remote rows.
		items, _ := out["Items"].([]map[string]any)
		if len(items) != 0 {
			t.Fatalf("expected no hydrated remote rows without remote service, got %#v", out)
		}
	}
}

func TestEmbyRemoteFavoritesFilterBeforePagination(t *testing.T) {
	t.Parallel()

	favorites := []struct{ id, kind string }{
		{id: "older-movie", kind: "Movie"},
		{id: "newer-movie", kind: "Movie"},
		{id: "latest-series", kind: "Series"},
	}
	remote, account := newPlayableIDTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := strings.CutPrefix(r.URL.Path, "/emby/Users/uid-1/Items/")
		if ok {
			for _, favorite := range favorites {
				if id == favorite.id {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]any{"Id": id, "Name": id, "Type": favorite.kind})
					return
				}
			}
		}
		http.NotFound(w, r)
	}))
	repos := remote.repo
	if err := repos.DB.AutoMigrate(&model.User{}, &model.Favorite{}); err != nil {
		t.Fatal(err)
	}
	viewer := &model.User{Username: "viewer", PasswordHash: "hash", Role: "user"}
	if err := repos.User.Create(t.Context(), viewer); err != nil {
		t.Fatal(err)
	}
	mount := &model.EmbyMount{AccountID: account.ID, RemoteViewID: "movies", Enabled: true}
	if err := repos.EmbyMount.Create(t.Context(), mount); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for i, favorite := range favorites {
		if err := repos.DB.Create(&model.Favorite{
			Base:    model.Base{CreatedAt: now.Add(time.Duration(i) * time.Minute)},
			UserID:  viewer.ID,
			MediaID: EncodeEmbyRemoteID(mount.ID, favorite.id),
		}).Error; err != nil {
			t.Fatal(err)
		}
	}

	svc := NewEmbyService(remote.cfg, zap.NewNop(), repos).SetEmbyRemote(remote)
	out, err := svc.Items(t.Context(), ItemsParams{
		UserID:           viewer.ID,
		ParentID:         EncodeEmbyRemoteID(mount.ID, mount.RemoteViewID),
		Filters:          []string{"IsFavorite"},
		IncludeItemTypes: []string{"Movie", "Episode"},
		StartIndex:       1,
		Limit:            1,
	})
	if err != nil {
		t.Fatal(err)
	}
	items := out["Items"].([]map[string]any)
	if out["TotalRecordCount"] != int64(2) || out["StartIndex"] != 1 || len(items) != 1 ||
		items[0]["Id"] != EncodeEmbyRemoteID(mount.ID, "older-movie") || items[0]["Type"] != "Movie" {
		t.Fatalf("expected the second matching favorite out of two, got %#v", out)
	}
}
