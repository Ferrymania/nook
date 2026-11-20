package cloud115

import (
	"sync"
	"time"
)

const (
	requestStatsStep  = 100 * time.Millisecond
	requestStatsSlots = int64(time.Hour / requestStatsStep)
	requestStatsHours = 24
)

type RequestStats struct {
	RequestsLastSecond int64         `json:"requests_last_second"`
	RequestsLastMinute int64         `json:"requests_last_minute"`
	RequestsLastHour   int64         `json:"requests_last_hour"`
	AverageResponseMs  *float64      `json:"average_response_ms"`
	Hourly             []RequestHour `json:"hourly"`
}

type RequestHour struct {
	StartAt   int64 `json:"start_at"`
	Requests  int64 `json:"requests"`
	Throttled int64 `json:"throttled"`
}

type requestSlot struct {
	index     int64
	requests  int64
	responses int64
	duration  time.Duration
}

// RequestMonitor keeps rolling request counts and response times for one account.
type RequestMonitor struct {
	mu    sync.Mutex
	rates []requestSlot
	hours [requestStatsHours]RequestHour
}

// Snapshot reports recent activity. A nil monitor represents an account with no requests yet.
func (m *RequestMonitor) Snapshot() RequestStats {
	if m == nil {
		m = &RequestMonitor{}
	}
	return m.snapshot(time.Now())
}

func (m *RequestMonitor) recordRequest(at time.Time) {
	if m == nil {
		return
	}
	slot := at.UnixNano() / int64(requestStatsStep)
	hour := at.Unix() / 3600
	hourStart := hour * 3600
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rates == nil {
		m.rates = make([]requestSlot, requestStatsSlots)
	}
	rate := &m.rates[slot%requestStatsSlots]
	if rate.index <= slot {
		if rate.index != slot {
			*rate = requestSlot{index: slot}
		}
		rate.requests++
	}
	bucket := &m.hours[hour%requestStatsHours]
	if bucket.StartAt <= hourStart {
		if bucket.StartAt != hourStart {
			*bucket = RequestHour{StartAt: hourStart}
		}
		bucket.Requests++
	}
}

func (m *RequestMonitor) recordResponse(started time.Time, elapsed time.Duration, throttled bool) {
	if m == nil {
		return
	}
	slot := started.UnixNano() / int64(requestStatsStep)
	hour := started.Unix() / 3600
	m.mu.Lock()
	defer m.mu.Unlock()
	if rate := &m.rates[slot%requestStatsSlots]; rate.index == slot {
		rate.responses++
		rate.duration += elapsed
	}
	if bucket := &m.hours[hour%requestStatsHours]; throttled && bucket.StartAt == hour*3600 {
		bucket.Throttled++
	}
}

func (m *RequestMonitor) snapshot(now time.Time) RequestStats {
	stats := RequestStats{Hourly: make([]RequestHour, requestStatsHours)}
	slot := now.UnixNano() / int64(requestStatsStep)
	hour := now.Unix() / 3600
	var responses int64
	var duration time.Duration
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, bucket := range m.rates {
		age := slot - bucket.index
		if age < 0 || age >= requestStatsSlots {
			continue
		}
		stats.RequestsLastHour += bucket.requests
		responses += bucket.responses
		duration += bucket.duration
		if age < int64(time.Minute/requestStatsStep) {
			stats.RequestsLastMinute += bucket.requests
		}
		if age < int64(time.Second/requestStatsStep) {
			stats.RequestsLastSecond += bucket.requests
		}
	}
	if responses > 0 {
		ms := float64(duration) / float64(responses) / float64(time.Millisecond)
		stats.AverageResponseMs = &ms
	}
	for i := range stats.Hourly {
		index := hour - int64(requestStatsHours-1-i)
		start := index * 3600
		stats.Hourly[i].StartAt = start
		if bucket := m.hours[index%requestStatsHours]; bucket.StartAt == start {
			stats.Hourly[i] = bucket
		}
	}
	return stats
}
