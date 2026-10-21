package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
)

func newRuntimeCacheForTest(t *testing.T, maxMB int) *RuntimeCacheService {
	t.Helper()
	cfg := &config.Config{}
	cfg.Cache.MemoryMaxSizeMB = maxMB
	return NewRuntimeCacheService(cfg)
}

func TestRuntimeCacheEvictsByByteBudget(t *testing.T) {
	cache := newRuntimeCacheForTest(t, 1)
	cache.SetObject("large", strings.Repeat("a", 800<<10), time.Minute)
	cache.SetObject("small", strings.Repeat("b", 300<<10), time.Minute)

	if cache.bytesUsed > cache.maxBytes {
		t.Fatalf("cache bytes=%d exceeds max=%d", cache.bytesUsed, cache.maxBytes)
	}
	if _, ok := cache.GetObject("large"); ok {
		t.Fatal("expected oldest large entry to be evicted")
	}
	if _, ok := cache.GetObject("small"); !ok {
		t.Fatal("expected newest small entry to remain cached")
	}
}

func TestRuntimeCacheSkipsOversizedObject(t *testing.T) {
	cache := newRuntimeCacheForTest(t, 1)
	cache.SetObject("huge", strings.Repeat("x", 2<<20), time.Minute)

	if _, ok := cache.GetObject("huge"); ok {
		t.Fatal("oversized object must not be cached")
	}
	if cache.bytesUsed != 0 {
		t.Fatalf("bytesUsed=%d, want 0", cache.bytesUsed)
	}
}

func TestRuntimeCachePrefixDeleteReleasesByteBudget(t *testing.T) {
	cache := newRuntimeCacheForTest(t, 2)
	for i := 0; i < 4; i++ {
		cache.SetJSON(context.Background(), fmt.Sprintf("media:%d", i), strings.Repeat("x", 64<<10), time.Minute)
	}
	if cache.bytesUsed == 0 {
		t.Fatal("expected cached bytes")
	}
	cache.DeletePrefix(context.Background(), "media:")
	if cache.bytesUsed != 0 {
		t.Fatalf("bytesUsed=%d after prefix delete, want 0", cache.bytesUsed)
	}
	for i := 0; i < 4; i++ {
		var out string
		if cache.GetJSON(context.Background(), fmt.Sprintf("media:%d", i), &out) {
			t.Fatalf("entry media:%d should have been deleted", i)
		}
	}
}

func TestRuntimeCacheStillEnforcesEntryLimit(t *testing.T) {
	cache := newRuntimeCacheForTest(t, 1)
	cache.limit = 3
	for i := 0; i < 5; i++ {
		cache.SetObject(fmt.Sprintf("item-%d", i), i, time.Minute)
	}
	if got := cache.entryCountLocked(); got != 3 {
		t.Fatalf("entry count=%d, want 3", got)
	}
}

func TestRuntimeCacheEstimatesMediaRows(t *testing.T) {
	rows := []model.Media{{
		Title:       strings.Repeat("t", 1024),
		Path:        "/media/movies/example.mkv",
		PosterURL:   "https://image.example/poster.jpg",
		BackdropURL: "https://image.example/backdrop.jpg",
		Overview:    strings.Repeat("o", 2048),
		Genres:      "Action,Adventure",
	}}
	size := estimateRuntimeCacheObjectSize("media:test", rows)
	if size < int64(len(rows[0].Title)+len(rows[0].Overview)) {
		t.Fatalf("estimated size %d is smaller than payload", size)
	}
}

func TestRuntimeCacheSetMaxSizeEvictsImmediately(t *testing.T) {
	cache := newRuntimeCacheForTest(t, 2)
	cache.SetObject("old", strings.Repeat("a", 700<<10), time.Minute)
	cache.SetObject("new", strings.Repeat("b", 700<<10), time.Minute)
	if cache.bytesUsed <= 1<<20 {
		t.Fatalf("test setup bytesUsed=%d, want >1MiB", cache.bytesUsed)
	}

	cache.SetMaxSizeMB(1)
	if cache.maxBytes != 1<<20 {
		t.Fatalf("maxBytes=%d, want 1MiB", cache.maxBytes)
	}
	if cache.bytesUsed > cache.maxBytes {
		t.Fatalf("bytesUsed=%d exceeds maxBytes=%d", cache.bytesUsed, cache.maxBytes)
	}
	if _, ok := cache.GetObject("old"); ok {
		t.Fatal("oldest entry should be evicted after lowering cache limit")
	}
	if _, ok := cache.GetObject("new"); !ok {
		t.Fatal("newest entry should remain after lowering cache limit")
	}
}

func TestRuntimeCacheReturnsStaleValueAfterFreshTTL(t *testing.T) {
	cache := newRuntimeCacheForTest(t, 1)
	key := "latest:stale"
	cache.SetJSONWithStale(context.Background(), key, "cached-value", time.Minute, time.Hour)

	fullKey := cache.key(key)
	cache.mu.Lock()
	item := cache.memory[fullKey]
	item.expiresAt = time.Now().Add(-time.Second)
	cache.memory[fullKey] = item
	cache.mu.Unlock()

	var fresh string
	if cache.GetJSON(context.Background(), key, &fresh) {
		t.Fatal("expired fresh entry must not be returned by GetJSON")
	}
	var stale string
	found, isStale := cache.GetJSONStale(context.Background(), key, &stale)
	if !found || !isStale {
		t.Fatalf("GetJSONStale found=%t stale=%t, want true/true", found, isStale)
	}
	if stale != "cached-value" {
		t.Fatalf("stale value=%q, want cached-value", stale)
	}
}

func TestRuntimeCacheDoesNotReturnOrdinaryEntryAsStale(t *testing.T) {
	cache := newRuntimeCacheForTest(t, 1)
	key := "latest:ordinary"
	cache.SetJSON(context.Background(), key, "cached-value", time.Minute)

	fullKey := cache.key(key)
	cache.mu.Lock()
	item := cache.memory[fullKey]
	expiredAt := time.Now().Add(-time.Second)
	item.expiresAt = expiredAt
	item.staleUntil = expiredAt
	cache.memory[fullKey] = item
	cache.mu.Unlock()

	var out string
	if found, isStale := cache.GetJSONStale(context.Background(), key, &out); found || isStale {
		t.Fatalf("ordinary expired entry found=%t stale=%t, want false/false", found, isStale)
	}
}

func TestRuntimeCacheRefreshReplacesStaleValue(t *testing.T) {
	cache := newRuntimeCacheForTest(t, 1)
	ctx := context.Background()
	key := "latest:refresh"
	cache.SetJSONWithStale(ctx, key, "old", time.Minute, time.Hour)
	cache.mu.Lock()
	item := cache.memory[cache.key(key)]
	item.expiresAt = time.Now().Add(-time.Second)
	cache.memory[cache.key(key)] = item
	cache.mu.Unlock()

	var out string
	if found, stale := cache.GetJSONStale(ctx, key, &out); !found || !stale || out != "old" {
		t.Fatalf("retained value: found=%v stale=%v value=%q", found, stale, out)
	}
	cache.SetJSONWithStale(ctx, key, "new", time.Minute, time.Hour)
	if found, stale := cache.GetJSONStale(ctx, key, &out); !found || stale || out != "new" {
		t.Fatalf("refreshed value: found=%v stale=%v value=%q", found, stale, out)
	}
	if !cache.GetJSON(ctx, key, &out) || out != "new" {
		t.Fatalf("fresh read after refresh = %q", out)
	}
}

func TestRuntimeCachePrefixInvalidationCoversJSONAndObjects(t *testing.T) {
	cache := newRuntimeCacheForTest(t, 1)
	ctx := context.Background()
	cache.SetJSON(ctx, "media:json", "remove", time.Minute)
	cache.SetObject("media:object", "remove", time.Minute)
	cache.SetJSONWithStale(ctx, "media:stale", "remove", time.Minute, time.Hour)
	cache.SetJSON(ctx, "history:json", "keep", time.Minute)
	cache.SetObject("history:object", "keep", time.Minute)

	cache.DeletePrefix(ctx, "media:")
	var out string
	if cache.GetJSON(ctx, "media:json", &out) {
		t.Fatal("JSON entry survived invalidation")
	}
	if found, _ := cache.GetJSONStale(ctx, "media:stale", &out); found {
		t.Fatal("retained entry survived invalidation")
	}
	if _, found := cache.GetObject("media:object"); found {
		t.Fatal("object entry survived invalidation")
	}
	if !cache.GetJSON(ctx, "history:json", &out) || out != "keep" {
		t.Fatal("unrelated JSON entry was invalidated")
	}
	if value, found := cache.GetObject("history:object"); !found || value != "keep" {
		t.Fatal("unrelated object entry was invalidated")
	}
}

func TestRuntimeCacheConcurrentMemoryOperations(t *testing.T) {
	cache := newRuntimeCacheForTest(t, 1)
	ctx := context.Background()
	var workers sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			jsonKey := fmt.Sprintf("concurrent:json:%d", worker)
			objectKey := fmt.Sprintf("concurrent:object:%d", worker)
			for iteration := 0; iteration < 100; iteration++ {
				want := fmt.Sprintf("%d:%d", worker, iteration)
				cache.SetJSONWithStale(ctx, jsonKey, want, time.Minute, time.Hour)
				var got string
				if found, _ := cache.GetJSONStale(ctx, jsonKey, &got); found && got != want {
					t.Errorf("JSON value=%q, want %q", got, want)
				}
				cache.SetObject(objectKey, want, time.Minute)
				if value, found := cache.GetObject(objectKey); found && value != want {
					t.Errorf("object value=%v, want %q", value, want)
				}
				if iteration%20 == 0 {
					cache.DeletePrefix(ctx, "concurrent:")
				}
				if iteration%25 == 0 {
					cache.SetMaxSizeMB(1)
				}
			}
		}()
	}
	workers.Wait()
	cache.DeletePrefix(ctx, "concurrent:")
	if cache.bytesUsed != 0 || cache.entryCountLocked() != 0 {
		t.Fatal("invalidation did not release all memory cache entries")
	}
}
