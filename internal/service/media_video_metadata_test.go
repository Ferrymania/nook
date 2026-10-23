package service

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
)

func TestMediaDetailUsesSavedVideoMetadataWithoutProbing(t *testing.T) {
	_, repos, m := newProbeFixture(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "must not resolve media while reading detail", http.StatusInternalServerError)
	}))
	defer server.Close()
	m.Path, m.STRMURL, m.SizeBytes = "/media/movie.strm", server.URL+"/movie.mkv", 123
	if err := repos.DB.Save(m).Error; err != nil {
		t.Fatal(err)
	}
	profile, rpu := 5, 1
	result := &FullProbeResult{SizeBytes: 10_000_000_000, Streams: []ProbeStream{
		{Type: "audio", Codec: "aac"},
		{Type: "video", FrameRate: "23.976", SideDataList: []ProbeVideoSideData{
			{Type: "DOVI configuration record", DVProfile: &profile, RPUPresentFlag: &rpu},
		}},
	}}
	row, err := mediaProbeRow(m, result)
	if err != nil {
		t.Fatal(err)
	}
	if err := repos.MediaProbe.Upsert(t.Context(), row); err != nil {
		t.Fatal(err)
	}
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	item, err := svc.GetMediaItem(t.Context(), m.ID)
	if err != nil {
		t.Fatal(err)
	}
	video := item.VideoMetadata
	if video == nil || video.FrameRate != 23.976 || video.ExtendedVideoType != "DolbyVision" || video.ExtendedVideoSubType != "DoviProfile50" || item.SizeBytes != result.SizeBytes {
		t.Fatalf("unexpected saved detail summary: %#v, size=%d", video, item.SizeBytes)
	}
	if requests.Load() != 0 {
		t.Fatal("detail request resolved or probed the media source")
	}
	// Changing the saved target invalidates the old HDR, FPS and source size.
	m.STRMURL = server.URL + "/changed.mkv"
	if err := repos.DB.Save(m).Error; err != nil {
		t.Fatal(err)
	}
	item, err = svc.GetMediaItem(t.Context(), m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if item.VideoMetadata != nil || item.SizeBytes != 0 || requests.Load() != 0 {
		t.Fatalf("stale metadata or pointer size exposed: %#v", item)
	}
}

func TestMediaDetailMissingProbeHidesPointerSize(t *testing.T) {
	_, repos, m := newProbeFixture(t)
	m.Path, m.STRMURL, m.SizeBytes = "/media/movie.strm", "https://example.invalid/movie.mkv", 123
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	svc.attachVideoMetadata(t.Context(), m)
	if m.VideoMetadata != nil || m.SizeBytes != 0 {
		t.Fatalf("missing probe became real media information: %#v", m)
	}
}

func TestMediaVideoFrameRatesRejectInvalidValues(t *testing.T) {
	for _, value := range []string{"0/0", "NaN", "+Inf", "-2", ""} {
		if rate := positiveFrameRate(value); rate != 0 {
			t.Fatalf("%q accepted as %f", value, rate)
		}
	}
	if rate := positiveFrameRate("24000/1001"); math.Abs(rate-23.976) > .001 {
		t.Fatalf("fractional frame rate = %f", rate)
	}
}

func TestRemoteMediaDetailPreservesOfficialVideoMetadata(t *testing.T) {
	r := &EmbyRemoteService{}
	for _, sourceOnly := range []bool{false, true} {
		stream := map[string]any{"Type": "Video", "AverageFrameRate": 23.976, "ExtendedVideoType": "DolbyVision", "ExtendedVideoSubType": "DoviProfile81"}
		item := map[string]any{"Id": "movie", "Name": "Movie"}
		if sourceOnly {
			item["MediaSources"] = []any{map[string]any{"MediaStreams": []any{stream}}}
		} else {
			item["MediaStreams"] = []any{stream}
		}
		media := r.MapRemoteItemToMedia(t.Context(), nil, &model.StrmAccount{Base: model.Base{ID: "acct"}}, &EmbyRemoteConfig{}, item)
		video := media.VideoMetadata
		if video == nil || video.FrameRate != 23.976 || video.ExtendedVideoType != "DolbyVision" || video.ExtendedVideoSubType != "DoviProfile81" {
			t.Fatalf("remote video metadata missing (sourceOnly=%v): %#v", sourceOnly, video)
		}
		if _, err := json.Marshal(media); err != nil {
			t.Fatal(err)
		}
	}
}
