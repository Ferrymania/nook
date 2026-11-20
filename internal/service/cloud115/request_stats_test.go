package cloud115

import (
	"testing"
	"time"
)

func TestRequestMonitorRollingWindowsAndHourlyHistory(t *testing.T) {
	t.Parallel()

	var metrics RequestMonitor
	now := time.Date(2026, time.November, 20, 13, 30, 0, 200000000, time.UTC)
	if stats := metrics.snapshot(now); stats.AverageResponseMs != nil || len(stats.Hourly) != 24 {
		t.Fatalf("unexpected empty snapshot: %+v", stats)
	}
	for _, request := range []struct {
		age       time.Duration
		elapsed   time.Duration
		throttled bool
	}{
		{24 * time.Hour, 80 * time.Millisecond, false},
		{65 * time.Minute, 50 * time.Millisecond, false},
		{45 * time.Minute, 300 * time.Millisecond, false},
		{2 * time.Second, 200 * time.Millisecond, false},
		{700 * time.Millisecond, 100 * time.Millisecond, true},
	} {
		start := now.Add(-request.age)
		metrics.recordRequest(start)
		metrics.recordResponse(start, request.elapsed, request.throttled)
	}
	metrics.recordRequest(now) // An in-flight request contributes to rate but not to latency.

	stats := metrics.snapshot(now)
	if stats.RequestsLastSecond != 2 || stats.RequestsLastMinute != 3 || stats.RequestsLastHour != 4 {
		t.Fatalf("wrong rolling counts: %+v", stats)
	}
	if stats.AverageResponseMs == nil || *stats.AverageResponseMs != 200 {
		t.Fatalf("average = %v, want 200 ms", stats.AverageResponseMs)
	}
	previous, current := stats.Hourly[22], stats.Hourly[23]
	if previous.Requests != 2 || current.Requests != 3 || current.Throttled != 1 || current.StartAt != now.Truncate(time.Hour).Unix() {
		t.Fatalf("wrong hourly counts: previous=%+v current=%+v", previous, current)
	}
	if stats.Hourly[0].Requests != 0 || stats.Hourly[0].StartAt != now.Truncate(time.Hour).Add(-23*time.Hour).Unix() {
		t.Fatalf("oldest hour should exclude expired samples: %+v", stats.Hourly[0])
	}
	later := metrics.snapshot(now.Add(time.Hour))
	if later.RequestsLastHour != 0 || later.AverageResponseMs != nil || later.Hourly[22].Requests != 3 {
		t.Fatalf("rolling rates should expire before the hourly history: %+v", later)
	}
}
