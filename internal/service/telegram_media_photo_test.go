package service

import (
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ferrymania/nook/internal/config"
	"go.uber.org/zap"
)

func TestTelegramSendPhotoMultipartAndRateLimit(t *testing.T) {
	for _, status := range []int{200, 429} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/botfake-token/sendPhoto" || r.Method != http.MethodPost {
					t.Errorf("request=%s %s", r.Method, r.URL.Path)
				}
				if err := r.ParseMultipartForm(1 << 20); err != nil {
					t.Error(err)
					return
				}
				defer r.MultipartForm.RemoveAll()
				if r.FormValue("caption") != "<b>新影片</b>" || r.FormValue("chat_id") != "123" || r.FormValue("parse_mode") != "HTML" {
					t.Errorf("fields=%v", r.Form)
				}
				file, _, err := r.FormFile("photo")
				if err != nil {
					t.Error(err)
					return
				}
				defer file.Close()
				data, _ := io.ReadAll(file)
				if string(data) != "photo-fixture" {
					t.Errorf("photo=%q", data)
				}
				w.WriteHeader(status)
				if status == 429 {
					_, _ = io.WriteString(w, `{"ok":false,"description":"Too Many Requests","parameters":{"retry_after":90}}`)
					return
				}
				_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":1}}`)
			}))
			defer server.Close()
			s := NewTelegramService(zap.NewNop(), nil)
			s.apiBase = server.URL
			s.client = server.Client()
			err := s.sendPhoto(context.Background(), telegramConfig{BotToken: "fake-token", AdminID: "123"}, []byte("photo-fixture"), "<b>新影片</b>")
			if status == 200 && err != nil {
				t.Fatal(err)
			}
			if status == 429 {
				var apiErr *telegramAPIError
				if !errors.As(err, &apiErr) || apiErr.RetryAfter != 90 || apiErr.Status != 429 {
					t.Fatalf("rate limit=%v", err)
				}
			}
		})
	}
}

func TestTelegramMediaPosterLocalRootAndPlaceholder(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{}
	cfg.App.DataDir = root
	cfg.Cache.CacheDir = filepath.Join(root, "cache")
	s := NewTelegramService(zap.NewNop(), nil)
	s.images = NewImageProxy(cfg, zap.NewNop())
	poster := filepath.Join(root, "poster.png")
	file, err := os.Create(poster)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, image.NewRGBA(image.Rect(0, 0, 20, 30))); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	data, err := s.mediaNotificationPoster(t.Context(), poster)
	if err != nil || len(data) == 0 {
		t.Fatalf("local poster=%d %v", len(data), err)
	}
	outside := filepath.Join(t.TempDir(), "outside.png")
	if err := os.WriteFile(outside, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.mediaNotificationPoster(t.Context(), outside); err == nil {
		t.Fatal("outside image was accepted")
	}
	link := filepath.Join(root, "linked.png")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if _, err := s.mediaNotificationPoster(t.Context(), link); err == nil {
		t.Fatal("symlink escaped image roots")
	}
	if err := os.WriteFile(poster, transparent1x1PNG, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.mediaNotificationPoster(t.Context(), poster); err == nil {
		t.Fatal("placeholder was accepted")
	}
}

func TestTelegramResponseFailureDoesNotExposeToken(t *testing.T) {
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = io.WriteString(w, `{"ok":false,"description":"Bad Request: invalid caption"}`)
	}))
	defer server.Close()
	s := NewTelegramService(zap.NewNop(), nil)
	s.apiBase = server.URL
	s.client = server.Client()
	err := s.call(t.Context(), "secret-token", "sendMessage", map[string]any{"text": "x"}, nil)
	if err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("error=%v", err)
	}
}
