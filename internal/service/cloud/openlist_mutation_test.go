package cloud

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenListRequiresTokenBeforeMakingRequests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("missing Token must not make a request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	for _, token := range []string{"", "  "} {
		cfg := map[string]any{"server": srv.URL, "username": "legacy", "password": "secret", "token": token}
		if _, err := New(TypeOpenList, cfg, srv.Client()); !errors.Is(err, ErrOpenListTokenRequired) {
			t.Fatalf("construction error = %v, want Token required", err)
		}
		p := newOpenList(cfg, srv.Client())
		ctx := context.Background()
		operations := map[string]func() error{
			"test":    func() error { return p.Ping(ctx) },
			"list":    func() error { _, err := p.List(ctx, "/"); return err },
			"resolve": func() error { _, err := p.Resolve(ctx, "/Movie.mkv"); return err },
			"mkdir":   func() error { _, err := p.Mkdir(ctx, "/", "TV"); return err },
			"rename":  func() error { _, err := p.Rename(ctx, "/TV", "Movies"); return err },
			"move":    func() error { _, err := p.Move(ctx, "/TV", "/Movies", "TV"); return err },
			"upload":  func() error { return p.PutFile(ctx, "/Movie.nfo", strings.NewReader("nfo")) },
		}
		for name, operation := range operations {
			if err := operation(); !errors.Is(err, ErrOpenListTokenRequired) {
				t.Errorf("%s error = %v, want Token required", name, err)
			}
		}
	}
}

func TestOpenListUploadUsesOnlyConfiguredToken(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodPut || r.URL.Path != "/api/fs/form" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "configured-token" {
			t.Error("upload did not use configured Token")
		}
		if r.Header.Get("File-Path") != "Movie.nfo" {
			t.Error("missing upload path")
		}
		if err := r.ParseMultipartForm(1024); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200}`))
	}))
	defer srv.Close()
	p := newOpenList(map[string]any{"server": srv.URL, "token": "configured-token", "username": "legacy", "password": "ignored"}, srv.Client())
	if err := p.PutFile(context.Background(), "/Movie.nfo", strings.NewReader("nfo")); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want only upload without login", requests)
	}
}
