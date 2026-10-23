package service

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Ferrymania/nook/internal/model"
)

func hdrTestDV(profile, compatibility, rpu int) ProbeVideoSideData {
	level := 6
	return ProbeVideoSideData{Type: "DOVI configuration record", DVProfile: &profile, DVLevel: &level, DVBLSignalCompatibilityID: &compatibility, RPUPresentFlag: &rpu}
}

func TestEmbyExtendedVideoTypesUseOfficialEnums(t *testing.T) {
	for _, c := range []struct {
		name             string
		stream           ProbeStream
		kind, sub, label string
	}{
		{"SDR", ProbeStream{ColorTransfer: "bt709"}, "None", "None", "SDR"},
		{"10 bit is not HDR", ProbeStream{Profile: "Main 10", PixFmt: "yuv420p10le", Title: "HDR10 Dolby Vision"}, "", "", ""},
		{"HDR10", ProbeStream{ColorTransfer: "smpte2084"}, "Hdr10", "Hdr10", "HDR 10"},
		{"HLG", ProbeStream{ColorTransfer: "arib-std-b67"}, "HyperLogGamma", "HyperLogGamma", "HLG"},
		{"HDR10+", ProbeStream{ColorTransfer: "smpte2084", SideDataList: []ProbeVideoSideData{{Type: "HDR Dynamic Metadata SMPTE2094-40 (HDR10+)"}}}, "Hdr10Plus", "Hdr10Plus0", "HDR 10+"},
		{"DV5", ProbeStream{SideDataList: []ProbeVideoSideData{hdrTestDV(5, 0, 1)}}, "DolbyVision", "DoviProfile50", "Dolby Vision"},
		{"DV7", ProbeStream{ColorTransfer: "smpte2084", SideDataList: []ProbeVideoSideData{hdrTestDV(7, 6, 1)}}, "DolbyVision", "DoviProfile76", "Dolby Vision"},
		{"DV8.1 level6", ProbeStream{ColorTransfer: "smpte2084", SideDataList: []ProbeVideoSideData{hdrTestDV(8, 1, 1)}}, "DolbyVision", "DoviProfile81", "Dolby Vision"},
		{"DV8.2", ProbeStream{ColorTransfer: "bt709", SideDataList: []ProbeVideoSideData{hdrTestDV(8, 2, 1)}}, "DolbyVision", "DoviProfile82", "Dolby Vision"},
		{"DV8.4", ProbeStream{ColorTransfer: "arib-std-b67", SideDataList: []ProbeVideoSideData{hdrTestDV(8, 4, 1)}}, "DolbyVision", "DoviProfile84", "Dolby Vision"},
		{"DV plus HDR10+", ProbeStream{SideDataList: []ProbeVideoSideData{{Type: "HDR Dynamic Metadata SMPTE2094-40 (HDR10+)"}, hdrTestDV(8, 1, 1)}}, "DolbyVision", "DoviProfile81", "Dolby Vision"},
		{"unknown DV subtype", ProbeStream{SideDataList: []ProbeVideoSideData{hdrTestDV(8, 15, 1)}}, "DolbyVision", "None", "Dolby Vision"},
		{"future DV profile", ProbeStream{SideDataList: []ProbeVideoSideData{hdrTestDV(10, 1, 1)}}, "DolbyVision", "None", "Dolby Vision"},
		{"DV tag without RPU", ProbeStream{ColorTransfer: "smpte2084", SideDataList: []ProbeVideoSideData{hdrTestDV(8, 1, 0)}}, "Hdr10", "Hdr10", "HDR 10"},
		{"incomplete DV", ProbeStream{SideDataList: []ProbeVideoSideData{{Type: "DOVI configuration record"}}}, "", "", ""},
		{"legacy unknown", ProbeStream{}, "", "", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			stream := map[string]any{}
			appendEmbyVideoMetadata(stream, c.stream)
			for key, want := range map[string]string{"ExtendedVideoType": c.kind, "ExtendedVideoSubType": c.sub, "VideoRange": c.label} {
				got, exists := stream[key]
				if want == "" {
					if exists {
						t.Errorf("unknown metadata emitted %s=%v", key, got)
					}
					continue
				}
				if got != want {
					t.Errorf("%s=%v, want %s", key, got, want)
				}
			}
			if _, wrong := stream["VideoRangeType"]; wrong {
				t.Fatal("Jellyfin enum leaked into Emby response")
			}
		})
	}
}

func TestEmbyHDRMetadataPersistsAndAppearsInDetailAndPlaybackInfo(t *testing.T) {
	svc := newTestEmbyService(t)
	for i, s := range []ProbeStream{
		{Type: "video", Codec: "hevc", PixFmt: "yuv420p10le", ColorTransfer: "smpte2084", ColorPrimaries: "bt2020", ColorSpace: "bt2020nc", BitDepth: 10},
		{Type: "video", Codec: "hevc", PixFmt: "yuv420p10le", SideDataList: []ProbeVideoSideData{hdrTestDV(8, 1, 1)}, BitDepth: 10},
	} {
		m := model.Media{Base: model.Base{ID: fmt.Sprintf("hdr-%d", i)}, Title: fmt.Sprintf("HDR-%d", i), Path: fmt.Sprintf("/media/hdr-%d.strm", i), Container: "strm"}
		if err := svc.repo.DB.Create(&m).Error; err != nil {
			t.Fatal(err)
		}
		row, err := mediaProbeRow(&m, &FullProbeResult{Container: "matroska,webm", DurationSec: 100, Streams: []ProbeStream{s}})
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.repo.MediaProbe.Upsert(t.Context(), row); err != nil {
			t.Fatal(err)
		}
		item, err := svc.Item(t.Context(), m.ID, "")
		if err != nil {
			t.Fatal(err)
		}
		playback, err := svc.PlaybackInfo(t.Context(), m.ID, "")
		if err != nil {
			t.Fatal(err)
		}
		for _, streams := range [][]map[string]any{item["MediaStreams"].([]map[string]any), item["MediaSources"].([]map[string]any)[0]["MediaStreams"].([]map[string]any), playback["MediaSources"].([]map[string]any)[0]["MediaStreams"].([]map[string]any)} {
			video := streams[0]
			want, _, _, _ := embyExtendedVideoType(s)
			if video["ExtendedVideoType"] != want || video["BitDepth"] != 10 {
				t.Fatalf("HDR missing from response: %#v", video)
			}
			body, _ := json.Marshal(video)
			var typed model.EmbyMediaStream
			if err := json.Unmarshal(body, &typed); err != nil || typed.ExtendedVideoType != want || typed.BitDepth != 10 {
				t.Fatalf("typed DTO dropped metadata: %s / %v", body, err)
			}
		}
	}
}

func TestLegacyProbeRemainsSettledWithoutInventingHDR(t *testing.T) {
	svc := newTestEmbyService(t)
	m := model.Media{Base: model.Base{ID: "legacy-hdr"}, Title: "DV HDR10 Main 10", Path: "/media/legacy.strm", Container: "strm"}
	if err := svc.repo.DB.Create(&m).Error; err != nil {
		t.Fatal(err)
	}
	row := &model.MediaProbe{MediaID: m.ID, Signature: mediaProbeSignature(&m), Payload: `{"duration_sec":100,"streams":[{"index":0,"type":"video","codec":"hevc","profile":"Main 10","pix_fmt":"yuv420p10le"}]}`}
	if !mediaProbeSettled(row, &m) {
		t.Fatal("adding HDR support must not automatically reprobe old caches")
	}
	if err := svc.repo.MediaProbe.Upsert(t.Context(), row); err != nil {
		t.Fatal(err)
	}
	item, err := svc.Item(t.Context(), m.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	video := item["MediaStreams"].([]map[string]any)[0]
	if _, exists := video["ExtendedVideoType"]; exists {
		t.Fatalf("legacy metadata fabricated HDR/SDR: %#v", video)
	}
	if _, exists := video["VideoRange"]; exists {
		t.Fatalf("legacy metadata fabricated range: %#v", video)
	}
	if video["BitDepth"] != 10 {
		t.Fatalf("legacy pixel format should still supply bit depth: %#v", video)
	}
}
