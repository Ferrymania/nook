package service

import (
	"context"
	"encoding/json"
	"testing"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
)

func TestStorageUsesKnownSourceSizes(t *testing.T) {
	db := newServiceTestDB(t)
	libs := []model.Library{
		{Base: model.Base{ID: "movies"}, Name: "电影", Path: "/movies", Type: "movie"},
		{Base: model.Base{ID: "tv"}, Name: "剧集", Path: "/tv", Type: "tv"},
		{Base: model.Base{ID: "empty"}, Name: "空库", Path: "/empty", Type: "movie"},
		{Base: model.Base{ID: "removed"}, Name: "已删除", Path: "/removed", Type: "movie"},
	}
	if err := db.Create(&libs).Error; err != nil {
		t.Fatal(err)
	}
	media := []model.Media{
		{Base: model.Base{ID: "local"}, LibraryID: "movies", Path: "/unmounted/movie.mkv", Title: "本地", Container: "mkv", SizeBytes: 1024, DurationSec: 120},
		{Base: model.Base{ID: "tiny-local"}, LibraryID: "movies", Path: "/unmounted/clip.mp4", Title: "小视频", Container: "mp4", SizeBytes: 12, DurationSec: 2},
		{Base: model.Base{ID: "pointer"}, LibraryID: "movies", Path: "/unmounted/movie.strm", Title: "未探测", Container: "strm", SizeBytes: 248},
		{Base: model.Base{ID: "huge-pointer"}, LibraryID: "movies", Path: "/unmounted/large.STRM", Title: "不可猜测来源", Container: "mkv", SizeBytes: 9_000_000_000},
		{Base: model.Base{ID: "probed"}, LibraryID: "tv", Path: "/unmounted/episode.STRM", STRMURL: "https://example.test/episode", Title: "已探测", Container: "matroska,webm", SizeBytes: 250, DurationSec: 3600},
		{Base: model.Base{ID: "deleted"}, LibraryID: "tv", Path: "/removed.mkv", Title: "已删除", SizeBytes: 10000},
		{Base: model.Base{ID: "removed-library"}, LibraryID: "removed", Path: "/removed/movie.mkv", Title: "已删除库", SizeBytes: 10000},
		{Base: model.Base{ID: "orphan"}, LibraryID: "missing", Path: "/orphan.mkv", Title: "孤立条目", SizeBytes: 10000},
	}
	if err := db.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	probe := model.MediaProbe{
		MediaID: "probed", Signature: mediaProbeSignature(&media[4]), Source: "strm",
		// Source size remains available even after a scan restores the pointer's size_bytes.
		Payload: `{"size_bytes":4096}`, LastError: "a later retry failed",
	}
	if err := db.Create(&probe).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&media[5]).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&libs[3]).Error; err != nil {
		t.Fatal(err)
	}

	got, err := NewStorageService(zap.NewNop(), repository.New(db)).Compute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.TotalBytes != 5132 || got.TotalSeconds != 3722 {
		t.Fatalf("totals include STRM text/unverified size: bytes=%d seconds=%d", got.TotalBytes, got.TotalSeconds)
	}
	if len(got.ByLibrary) != 3 {
		t.Fatalf("libraries = %d, want 3 (including empty)", len(got.ByLibrary))
	}
	var count, size int64
	for _, c := range got.ByContainer {
		count += c.Count
		size += c.Bytes
	}
	if count != 5 || size != got.TotalBytes {
		t.Fatalf("container scope differs from library scope: count=%d bytes=%d", count, size)
	}
	// Assert the wire contract too: clients must distinguish unknown size from zero.
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		MediaCount       *int64 `json:"media_count"`
		KnownSizeCount   *int64 `json:"known_size_count"`
		UnknownSizeCount *int64 `json:"unknown_size_count"`
		Libraries        []struct {
			MediaCount       int64 `json:"media_count"`
			KnownSizeCount   int64 `json:"known_size_count"`
			UnknownSizeCount int64 `json:"unknown_size_count"`
		} `json:"by_library"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.MediaCount == nil || *wire.MediaCount != 5 || wire.KnownSizeCount == nil || *wire.KnownSizeCount != 3 || wire.UnknownSizeCount == nil || *wire.UnknownSizeCount != 2 {
		t.Fatalf("missing or incorrect coverage: %s", raw)
	}
	for _, l := range wire.Libraries {
		if l.KnownSizeCount+l.UnknownSizeCount != l.MediaCount {
			t.Fatalf("coverage does not add up: %+v", l)
		}
	}
}

func TestStorageRejectsUnavailableOrStaleSTRMSizes(t *testing.T) {
	db := newServiceTestDB(t)
	lib := model.Library{Base: model.Base{ID: "strm"}, Name: "STRM", Path: "/unmounted", Type: "tv"}
	if err := db.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, path, target, container, payload, signature, source string
		deletedProbe                                              bool
	}{
		{name: "malformed", path: "/malformed.strm", payload: "not json"},
		{name: "absent-size", path: "/absent.strm", payload: `{"duration_sec":100}`},
		{name: "zero", path: "/zero.strm", payload: `{"size_bytes":0}`},
		{name: "negative", path: "/negative.strm", payload: `{"size_bytes":-5}`},
		{name: "stale-target", path: "/stale.strm", target: "https://example.test/new", payload: `{"size_bytes":4096}`, signature: "old-target"},
		{name: "missing-target", path: "/unmounted/missing.strm", payload: `{"size_bytes":4096}`, signature: "old-target"},
		{name: "deleted-probe", path: "/deleted.strm", payload: `{"size_bytes":4096}`, deletedProbe: true},
		{name: "local-probe", path: "/changed.strm", payload: `{"size_bytes":4096}`, source: "local"},
		{name: "container-hint", path: "/no-extension", container: " STRM "},
		{name: "target-hint", path: "/no-strm-extension.mkv", target: "https://example.test/video"},
	}
	for _, tc := range cases {
		m := model.Media{Base: model.Base{ID: tc.name}, LibraryID: lib.ID, Path: tc.path, STRMURL: tc.target, Container: tc.container, Title: tc.name, SizeBytes: 10000, DurationSec: -1}
		if err := db.Create(&m).Error; err != nil {
			t.Fatal(err)
		}
		if tc.payload != "" {
			p := model.MediaProbe{MediaID: m.ID, Payload: tc.payload, Signature: tc.signature, Source: tc.source}
			if err := db.Create(&p).Error; err != nil {
				t.Fatal(err)
			}
			if tc.deletedProbe {
				if err := db.Delete(&p).Error; err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	got, err := NewStorageService(zap.NewNop(), repository.New(db)).Compute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.TotalBytes != 0 || got.TotalSeconds != 0 || got.ByLibrary[0].MediaCount != int64(len(cases)) {
		t.Fatalf("unknown sizes must not be counted as source bytes: %+v", got)
	}
}

func TestStorageEmptyLibraryList(t *testing.T) {
	db := newServiceTestDB(t)
	// Orphaned rows must not leak into container totals when there are no libraries.
	if err := db.Create(&model.Media{LibraryID: "missing", Title: "orphan", Path: "/orphan.mkv", SizeBytes: 10}).Error; err != nil {
		t.Fatal(err)
	}
	got, err := NewStorageService(zap.NewNop(), repository.New(db)).Compute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ByLibrary) != 0 || len(got.ByContainer) != 0 || got.TotalBytes != 0 {
		t.Fatalf("empty scope = %+v", got)
	}
}
