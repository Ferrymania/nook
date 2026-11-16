package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/service/cloud"
)

func TestProxyDirectFollowsRedirectAndCancelsUpstream(t *testing.T) {
	t.Parallel()
	started, stopped := make(chan struct{}), make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/video", http.StatusFound)
			return
		}
		close(started)
		<-r.Context().Done()
		close(stopped)
	}))
	defer upstream.Close()
	svc := &StrmService{streamHTTP: upstream.Client()}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- svc.ProxyDirect(ctx, httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/video", nil), &cloud.DirectLink{URL: upstream.URL + "/redirect"})
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel error = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("proxy ignored cancellation")
	}
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream request was not canceled")
	}
}

func TestGlobalProxyRewritesRemoteMountPlaybackURLs(t *testing.T) {
	t.Parallel()
	repos := directRedirectTestRepo(t)
	svc := &EmbyRemoteService{repo: repos}
	mount := &model.EmbyMount{Base: model.Base{ID: "mount"}}
	cfg := &EmbyRemoteConfig{BaseURL: "https://remote.example", Token: "remote-secret"}
	for _, value := range []string{"false", "true", "false"} {
		if err := repos.Setting.Set(t.Context(), PlaybackProxyRemoteSettingKey, value); err != nil {
			t.Fatal(err)
		}
		source := map[string]any{"Id": "source", "MediaStreams": []any{map[string]any{"Type": "Subtitle", "DeliveryUrl": "/Videos/id/Subtitles/1/Stream.srt"}}}
		svc.rewritePlayURLs(t.Context(), map[string]any{"MediaSources": []any{source}}, mount, cfg, "movie")
		path := source["DirectStreamUrl"].(string)
		subtitle := source["MediaStreams"].([]any)[0].(map[string]any)["DeliveryUrl"].(string)
		if value == "true" {
			if !strings.HasPrefix(path, "/Videos/") || strings.Contains(subtitle, "remote-secret") || !strings.HasPrefix(subtitle, "/Videos/") {
				t.Fatalf("proxy leaked remote playback addresses: %v", source)
			}
		} else if !strings.HasPrefix(path, "https://remote.example") {
			t.Fatalf("direct playback = %s", path)
		}
	}
	if mount.ProxyPlay || cfg.ProxyPlay {
		t.Fatal("global proxy mutated the saved mount settings")
	}
}

type uaBoundProxyProvider struct {
	cloud.Provider
	url string
}

func (p uaBoundProxyProvider) ResolveWithUA(_ context.Context, ref, ua string) (*cloud.DirectLink, error) {
	if ref != "pickcode" || ua != "BoundPlayer/1" {
		return nil, errors.New("wrong player UA used to resolve the video")
	}
	return &cloud.DirectLink{URL: p.url, Headers: map[string]string{"User-Agent": ua}}, nil
}

func TestProxyMediaDirectResolvesWithPlayerUA(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.UserAgent() != "BoundPlayer/1" {
			t.Errorf("CDN UA=%q", r.UserAgent())
		}
		_, _ = w.Write([]byte("original"))
	}))
	defer upstream.Close()
	svc := testStrmService(t)
	account := &model.StrmAccount{Base: model.Base{ID: "ua-account"}, Name: "115", Provider: model.StrmProvider115, Enabled: true}
	if err := svc.repo.StrmAccount.Create(t.Context(), account); err != nil {
		t.Fatal(err)
	}
	svc.provider115Cache[account.ID] = uaBoundProxyProvider{url: upstream.URL}
	media := &model.Media{STRMURL: "/api/strm/play/cloud115/video.mkv?acct=ua-account&pickcode=pickcode"}
	req := httptest.NewRequest(http.MethodGet, "/api/stream/id", nil)
	req.Header.Set("User-Agent", "BoundPlayer/1")
	rec := httptest.NewRecorder()
	if err := svc.ProxyMediaDirect(t.Context(), rec, req, media); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || rec.Body.String() != "original" || rec.Header().Get("Location") != "" {
		t.Fatalf("proxy response=%d %v %s", rec.Code, rec.Header(), rec.Body.String())
	}
}

func TestRemoteVideoProxyPreservesHeadAndRange(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "bytes=2-4" || r.Header.Get("If-Range") != `"etag"` || r.UserAgent() != "RemotePlayer/1" {
			t.Errorf("headers lost: %v", r.Header)
		}
		w.Header().Set("Content-Length", "3")
		w.Header().Set("Content-Range", "bytes 2-4/10")
		w.WriteHeader(http.StatusPartialContent)
		if r.Method != http.MethodHead {
			_, _ = w.Write([]byte("234"))
		}
	}))
	defer upstream.Close()
	svc := &EmbyRemoteService{stream: upstream.Client()}
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		req := httptest.NewRequest(method, "/Videos/remote/stream", nil)
		req.Header.Set("User-Agent", "RemotePlayer/1")
		req.Header.Set("Range", "bytes=2-4")
		req.Header.Set("If-Range", `"etag"`)
		rec := httptest.NewRecorder()
		if err := svc.proxyVideoStreamOnLine(t.Context(), rec, req, &EmbyRemoteConfig{BaseURL: upstream.URL, Token: "remote-token"}, "movie"); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusPartialContent || rec.Header().Get("Content-Range") != "bytes 2-4/10" {
			t.Fatalf("%s response=%d %v", method, rec.Code, rec.Header())
		}
		if method == http.MethodHead && rec.Body.Len() != 0 {
			t.Fatal("HEAD transferred a video body")
		}
	}
}
