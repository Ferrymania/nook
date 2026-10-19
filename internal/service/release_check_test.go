package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type releaseCheckTransport func(*http.Request) (*http.Response, error)

func (f releaseCheckTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func releaseCheckResponse(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

func TestReleaseCheckVersionComparison(t *testing.T) {
	for _, tc := range []struct{ current, latest, want string }{
		{"nook-v0.0.9", "nook-v0.0.10", "update_available"},
		{"nook-v0.0.26", "v0.0.26", "up_to_date"},
		{"0.0.26", "nook-v0.0.26", "up_to_date"},
		{"nook-v0.0.26+build.7", "nook-v0.0.26", "up_to_date"},
		{"v0.0.26-rc.1", "nook-v0.0.26", "update_available"},
		{"0.0.27-beta.1", "0.0.26", "ahead"},
		{"1.0.0", "0.99.99", "ahead"},
		{"0.9.99", "0.10.0", "update_available"},
		{"dev", "nook-v0.0.26", "unknown"},
		{"git-abc123", "0.0.26", "unknown"},
		{"0.01.0", "0.1.0", "unknown"},
		{"0.1.0-01", "0.1.0", "unknown"},
		{"0.0.1", "not-a-version", "unavailable"},
		{"0.0.1", "0.0.2-beta.1", "unavailable"},
		{"0.0.1", "999999999999999999999.0.0", "unavailable"},
	} {
		t.Run(tc.current+"_"+tc.latest, func(t *testing.T) {
			s := NewReleaseCheckService(tc.current)
			s.client.Transport = releaseCheckTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != githubLatestReleaseURL || r.Method != http.MethodGet || r.Header.Get("User-Agent") == "" || r.Header.Get("Authorization") != "" {
					t.Fatalf("unexpected release request: %s %s", r.Method, r.URL)
				}
				payload, _ := json.Marshal(map[string]any{"tag_name": tc.latest, "body": "修复播放问题", "published_at": "2026-10-19T06:59:43Z", "html_url": "javascript:alert(1)"})
				return releaseCheckResponse(http.StatusOK, string(payload)), nil
			})
			got := s.Check(t.Context(), false)
			if got.Status != tc.want || got.CurrentVersion != tc.current || got.CheckedAt == "" {
				t.Fatalf("got %+v, want status %s", got, tc.want)
			}
			if !strings.HasPrefix(got.ReleaseURL, nookReleasesURL) {
				t.Fatalf("untrusted release URL: %s", got.ReleaseURL)
			}
			if tc.want != "unavailable" && (got.ReleaseNotes != "修复播放问题" || got.LatestVersion != tc.latest) {
				t.Fatalf("lost release metadata: %+v", got)
			}
		})
	}
}

func TestReleaseCheckFailuresNeverReportLatest(t *testing.T) {
	for _, tc := range []struct {
		name string
		code int
		body string
		err  error
	}{
		{"rate limit", 403, `{}`, nil},
		{"too many requests", 429, `{}`, nil},
		{"no release", 404, `{}`, nil},
		{"upstream error", 502, `<html>error</html>`, nil},
		{"invalid JSON", 200, `not-json`, nil},
		{"draft", 200, `{"tag_name":"v0.0.27","draft":true}`, nil},
		{"prerelease", 200, `{"tag_name":"v0.0.27","prerelease":true}`, nil},
		{"network", 0, "", errors.New("connection failed")},
		{"oversized", 200, `{"body":"` + strings.Repeat("x", 1<<20) + `","tag_name":"v0.0.27"}`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewReleaseCheckService("v0.0.26")
			s.client.Transport = releaseCheckTransport(func(*http.Request) (*http.Response, error) {
				if tc.err != nil {
					return nil, tc.err
				}
				return releaseCheckResponse(tc.code, tc.body), nil
			})
			got := s.Check(t.Context(), false)
			if got.Status != "unavailable" || got.LatestVersion != "" || got.Message == "" {
				t.Fatalf("failure must be explicit, got %+v", got)
			}
		})
	}
}

func TestReleaseCheckCacheRefreshAndConcurrentRequests(t *testing.T) {
	s := NewReleaseCheckService("v0.0.26")
	now := time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	var calls atomic.Int32
	s.client.Transport = releaseCheckTransport(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return releaseCheckResponse(200, `{"tag_name":"nook-v0.0.27"}`), nil
	})
	var group sync.WaitGroup
	for range 12 {
		group.Go(func() { s.Check(t.Context(), false) })
	}
	group.Wait()
	s.Check(t.Context(), true)
	if calls.Load() != 1 {
		t.Fatalf("concurrent/rapid checks made %d requests", calls.Load())
	}
	now = now.Add(time.Minute)
	s.Check(t.Context(), false)
	if calls.Load() != 1 {
		t.Fatal("automatic check bypassed cache")
	}
	s.Check(t.Context(), true)
	if calls.Load() != 2 {
		t.Fatal("manual refresh did not fetch after cooldown")
	}
	now = now.Add(releaseCheckCacheTTL)
	s.Check(t.Context(), false)
	if calls.Load() != 3 {
		t.Fatal("expired cache was not refreshed")
	}
}

func TestReleaseCheckFailureRetryAndCancellation(t *testing.T) {
	s := NewReleaseCheckService("v0.0.26")
	now := time.Now()
	s.now = func() time.Time { return now }
	calls := 0
	s.client.Transport = releaseCheckTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if err := r.Context().Err(); err != nil {
			return nil, err
		}
		if calls == 2 {
			return releaseCheckResponse(503, ""), nil
		}
		return releaseCheckResponse(200, `{"tag_name":"v0.0.27"}`), nil
	})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	s.Check(ctx, false)
	if !s.checked.IsZero() {
		t.Fatal("cancelled request populated cache")
	}
	if got := s.Check(t.Context(), false); got.Status != "unavailable" {
		t.Fatalf("expected upstream error, got %+v", got)
	}
	s.Check(t.Context(), false)
	if calls != 2 {
		t.Fatal("failed check was not cached briefly")
	}
	now = now.Add(releaseCheckRetryTTL)
	if got := s.Check(t.Context(), false); got.Status != "update_available" || calls != 3 {
		t.Fatalf("retry did not recover: %+v calls=%d", got, calls)
	}
}
