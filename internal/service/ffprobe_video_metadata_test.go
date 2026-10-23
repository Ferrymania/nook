package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Ferrymania/nook/internal/config"
	"go.uber.org/zap"
)

func TestFullProbeRetainsHDRMetadataWithoutRawSideData(t *testing.T) {
	fixture := `{"streams":[{"index":0,"codec_type":"video","codec_name":"hevc","codec_tag_string":"hvc1","bits_per_raw_sample":"10","pix_fmt":"yuv420p10le","color_range":"tv","color_space":"bt2020nc","color_transfer":"smpte2084","color_primaries":"bt2020","side_data_list":[
		{"side_data_type":"DOVI configuration record","dv_version_major":1,"dv_version_minor":0,"dv_profile":8,"dv_level":6,"rpu_present_flag":1,"el_present_flag":0,"bl_present_flag":1,"dv_bl_signal_compatibility_id":1,"dv_md_compression":0,"filename":"https://secret.example/token"},
		{"side_data_type":"Mastering display metadata","red_x":"34000/50000","red_y":"16000/50000","green_x":"13250/50000","green_y":"34500/50000","blue_x":"7500/50000","blue_y":"3000/50000","white_point_x":"15635/50000","white_point_y":"16450/50000","min_luminance":"1/10000","max_luminance":"10000000/10000"},
		{"side_data_type":"Content light level metadata","max_content":1000,"max_average":400},
		{"side_data_type":"HDR Dynamic Metadata SMPTE2094-40 (HDR10+)","raw_payload":"secret"},
		{"side_data_type":"unknown metadata https://secret.example/token"}
	]}]}`
	got, err := parseFullProbeJSON([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	s := got.Streams[0]
	if s.BitDepth != 10 || s.ColorTransfer != "smpte2084" || s.ColorSpace != "bt2020nc" || s.ColorPrimaries != "bt2020" || s.ColorRange != "tv" || s.CodecTag != "hvc1" {
		t.Fatalf("video metadata lost: %#v", s)
	}
	if len(s.SideDataList) != 4 || s.SideDataList[1].MaxLuminance != "10000000/10000" || *s.SideDataList[2].MaxContent != 1000 {
		t.Fatalf("HDR side data lost: %#v", s.SideDataList)
	}
	payload, err := got.PayloadJSON()
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"secret", "filename", "raw_payload", "unknown metadata"} {
		if strings.Contains(payload, secret) {
			t.Fatalf("raw probe data persisted: %s", payload)
		}
	}
	var saved mediaProbePayload
	if err := json.Unmarshal([]byte(payload), &saved); err != nil {
		t.Fatal(err)
	}
	dv := saved.Streams[0].SideDataList[0]
	if saved.Version != 2 || *dv.DVProfile != 8 || *dv.DVBLSignalCompatibilityID != 1 || dv.ELPresentFlag == nil || *dv.ELPresentFlag != 0 {
		t.Fatalf("round trip lost DV/zero values: %s", payload)
	}
}

func TestProbeBitDepth(t *testing.T) {
	for _, c := range []struct {
		raw, sample  int
		kind, format string
		want         int
	}{
		{10, 0, "video", "", 10}, {0, 24, "audio", "", 24},
		{0, 0, "video", "yuv420p10le", 10}, {0, 0, "video", "gbrp12be", 12},
		{0, 0, "video", "gray16le", 16}, {0, 0, "video", "p010le", 10},
		{0, 0, "video", "yuv420p", 8}, {0, 0, "video", "", 0},
		{0, 0, "audio", "yuv420p10le", 0}, {999, 0, "video", "", 0},
	} {
		if got := probeBitDepth(c.raw, c.sample, c.kind, c.format); got != c.want {
			t.Errorf("%+v: got %d", c, got)
		}
	}
}

func TestProbeAcceptsDVCompressionAcrossFFprobeVersions(t *testing.T) {
	for _, value := range []string{`0`, `"none"`, `"limited"`, `"extended"`, `"future-value"`} {
		input := `{"streams":[{"codec_type":"video","codec_name":"hevc","side_data_list":[{"side_data_type":"DOVI configuration record","dv_profile":5,"rpu_present_flag":1,"dv_md_compression":` + value + `}]}]}`
		result, err := parseFullProbeJSON([]byte(input))
		if err != nil {
			t.Fatalf("compression %s: %v", value, err)
		}
		kind, subtype, _, _ := embyExtendedVideoType(result.Streams[0])
		if kind != "DolbyVision" || subtype != "DoviProfile50" {
			t.Fatalf("DV identification lost for %s", value)
		}
	}
}

func TestProbeFullDoesNotAddFrameProbe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX test fixture")
	}
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	bin := filepath.Join(dir, "ffprobe")
	t.Setenv("NOOK_HDR_PROBE_ARGS", argsFile)
	script := "#!/bin/sh\nprintf '%s\\n' call \"$@\" >> \"$NOOK_HDR_PROBE_ARGS\"\ncat <<'JSON'\n" + fullProbeFixture + "\nJSON\n"
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.App.FFprobePath = bin
	if _, err := NewFFprobeService(cfg, zap.NewNop()).ProbeFull(t.Context(), ProbeInput{Source: "test.mkv"}); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(args), "call\n") != 1 {
		t.Fatalf("extra ffprobe invocation: %s", args)
	}
	for _, option := range []string{"-show_frames", "-show_packets", "-analyze_frames", "-read_intervals", "-count_frames"} {
		if strings.Contains(string(args), option) {
			t.Fatalf("extra probing option %s: %s", option, args)
		}
	}
	for _, option := range []string{"-show_streams", "-show_format", "-show_chapters"} {
		if !strings.Contains(string(args), option) {
			t.Fatalf("missing existing option %s", option)
		}
	}
}
