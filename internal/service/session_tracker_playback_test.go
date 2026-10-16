package service

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestPlaybackSessionExpiresDespiteBrowsingActivity(t *testing.T) {
	tracker := NewSessionTrackerService(zap.NewNop())
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	tracker.now = func() time.Time { return now }
	tracker.RecordPlayback(t.Context(), "u1", "viewer", "d1", "Mac", "SenPlayer", "", "m1", 100, 200, false)
	now = now.Add(2 * time.Minute)
	tracker.RecordActivity(t.Context(), "u1", "viewer", "d1", "Mac", "SenPlayer", "")
	if got := tracker.ListPlaying(t.Context()); len(got) != 0 {
		t.Fatalf("browsing must not keep playback alive: %#v", got)
	}
	if got := tracker.List(t.Context()); len(got) != 1 {
		t.Fatalf("online session should remain after playback expires: %#v", got)
	}
}

func TestPlaybackSessionPauseHeartbeatResumeAndStop(t *testing.T) {
	tracker := NewSessionTrackerService(zap.NewNop())
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	tracker.now = func() time.Time { return now }
	state := PlaybackState{ItemID: "m1", PositionTicks: 100, RuntimeTicks: 200, IsPaused: true}
	tracker.RecordPlaybackState(t.Context(), "u1", "viewer", "d1", "Mac", "SenPlayer", "", state)
	for i := 0; i < 3; i++ {
		now = now.Add(time.Minute)
		tracker.RecordPlaybackState(t.Context(), "u1", "viewer", "d1", "Mac", "SenPlayer", "", state)
	}
	if got := tracker.ListPlaying(t.Context()); len(got) != 1 || !got[0].IsPaused {
		t.Fatalf("pause heartbeats must preserve paused session: %#v", got)
	}
	state.IsPaused = false
	tracker.RecordPlaybackState(t.Context(), "u1", "viewer", "d1", "Mac", "SenPlayer", "", state)
	if got := tracker.ListPlaying(t.Context()); len(got) != 1 || got[0].IsPaused {
		t.Fatalf("resume must clear pause: %#v", got)
	}
	state.Stopped = true
	tracker.RecordPlaybackState(t.Context(), "u1", "viewer", "d1", "Mac", "SenPlayer", "", state)
	if got := tracker.ListPlaying(t.Context()); len(got) != 0 {
		t.Fatalf("stop must remove playback immediately: %#v", got)
	}
}

func TestPlaybackSessionIgnoresDelayedWebReports(t *testing.T) {
	tracker := NewSessionTrackerService(zap.NewNop())
	for _, state := range []PlaybackState{
		{ItemID: "m1", Sequence: 2, IsPaused: true},
		{ItemID: "m1", Sequence: 1},
	} {
		tracker.RecordPlaybackState(t.Context(), "u1", "viewer", "web:s1", "", "Nook Web", "", state)
	}
	if got := tracker.ListPlaying(t.Context()); len(got) != 1 || !got[0].IsPaused {
		t.Fatalf("delayed progress cleared pause: %#v", got)
	}
	for _, state := range []PlaybackState{
		{ItemID: "m1", Sequence: 4, Stopped: true},
		{ItemID: "m1", Sequence: 3},
	} {
		tracker.RecordPlaybackState(t.Context(), "u1", "viewer", "web:s1", "", "Nook Web", "", state)
	}
	if got := tracker.ListPlaying(t.Context()); len(got) != 0 {
		t.Fatalf("delayed progress resurrected stopped playback: %#v", got)
	}
}
