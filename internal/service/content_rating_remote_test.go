package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/Ferrymania/nook/internal/model"
)

func TestContentRatingRemoteFiltersBeforePaginationAndPreservesUnknownPolicy(t *testing.T) {
	requests := 0
	remote := &EmbyRemoteService{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		all := make([]map[string]any, 205)
		for i := range all {
			rating := "R"
			if i%2 == 0 {
				rating = "G"
			}
			if i == 204 {
				rating = ""
			}
			all[i] = map[string]any{"Id": fmt.Sprint(i), "Name": fmt.Sprint(i), "Type": "Movie", "OfficialRating": rating}
		}
		start, _ := strconv.Atoi(r.URL.Query().Get("StartIndex"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("Limit"))
		if start > len(all) {
			start = len(all)
		}
		end := len(all)
		if limit > 0 && start+limit < end {
			end = start + limit
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Items": all[start:end], "TotalRecordCount": len(all)})
	}))
	defer srv.Close()
	remote.http = srv.Client()
	cfg := &EmbyRemoteConfig{BaseURL: srv.URL, Token: "test", RemoteUserID: "remote-user"}
	for _, hide := range []bool{false, true} {
		ctx := WithContentRatingPolicy(t.Context(), model.ContentRatingPolicy{MaxContentRatingLevel: 30, HideUnrated: hide})
		var body struct {
			Items            []map[string]any
			TotalRecordCount int
			StartIndex       int
		}
		err := remote.doGet(ctx, nil, cfg, "/Users/remote-user/Items", url.Values{"StartIndex": {"2"}, "Limit": {"2"}}, &body)
		want := 103
		if hide {
			want = 102
		}
		if err != nil || body.TotalRecordCount != want || len(body.Items) != 2 || remoteItemString(body.Items[0], "Id") != "4" || body.StartIndex != 2 {
			t.Fatalf("hide=%t result=%+v err=%v", hide, body, err)
		}
	}
	if requests != 4 {
		t.Fatalf("requested %d upstream pages, want 4", requests)
	}
}

func TestContentRatingRemoteDetailCannotBypassWithWarmCache(t *testing.T) {
	svc, mount, acct := newImageTagTestService(t)
	ctx := WithContentRatingPolicy(t.Context(), model.ContentRatingPolicy{MaxContentRatingLevel: 30})
	key := svc.remoteCacheKey(ctx, "item", mount.ID, "blocked")
	svc.cache = NewRuntimeCacheService(nil)
	svc.cache.SetJSON(ctx, key, map[string]any{"Id": "blocked", "Type": "Movie", "OfficialRating": "R"}, svc.remoteMediaCacheTTL())
	item, err := svc.RemoteItem(ctx, mount, acct, "blocked")
	if item != nil || err == nil {
		t.Fatalf("cached blocked detail=%v err=%v", item, err)
	}
}
