package service

import (
	"testing"

	"go.uber.org/zap"
)

// 播放接口不启动技术探测；缺少缓存时也不打开视频文件。
func TestListForPlaybackDoesNotCreateMediaProbe(t *testing.T) {
	_, repos, m := newProbeFixture(t)
	segments := NewMediaSegmentService(zap.NewNop(), repos)

	rows, err := segments.ListForPlayback(t.Context(), m)
	if err != nil {
		t.Fatalf("ListForPlayback: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows = %#v, want none without a provider", rows)
	}

	probeRow, err := repos.MediaProbe.Get(t.Context(), m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if probeRow != nil {
		t.Fatal("the playback path must leave media probing to the scraper")
	}
}
