package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const telegramPhotoMaxBytes = 10 << 20

type telegramAPIError struct {
	Method, Description string
	Status, RetryAfter  int
}

func (e *telegramAPIError) Error() string {
	return fmt.Sprintf("telegram %s: %s", e.Method, e.Description)
}

func (s *TelegramService) sendMediaNotification(ctx context.Context, cfg telegramConfig, poster, caption string) error {
	if poster != "" && s.images != nil {
		photoCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		photo, err := s.mediaNotificationPoster(photoCtx, poster)
		cancel()
		if err == nil {
			if err := s.checkMediaNotificationDestination(ctx, cfg); err != nil {
				return err
			}
			err = s.sendPhoto(ctx, cfg, photo, caption)
			if err == nil {
				return nil
			}
			var apiErr *telegramAPIError
			// Only an explicit photo rejection falls back. Authentication,
			// throttling and ambiguous transport errors must keep the cursor pending.
			if !errors.As(err, &apiErr) || apiErr.Status != http.StatusBadRequest {
				return err
			}
		}
		s.warn("telegram media poster unavailable; sending text", "admin", redactSensitiveError(err))
	}
	if err := s.checkMediaNotificationDestination(ctx, cfg); err != nil {
		return err
	}
	return s.call(ctx, cfg.BotToken, "sendMessage", map[string]any{
		"chat_id": cfg.AdminID, "text": caption, "parse_mode": "HTML",
		"link_preview_options": map[string]any{"is_disabled": true},
	}, nil)
}

// Fetch/upload the image itself: local artwork and authenticated image URLs
// remain private, and Telegram never needs credentials to reach Nook.
func (s *TelegramService) mediaNotificationPoster(ctx context.Context, raw string) ([]byte, error) {
	if strings.HasPrefix(raw, "/api/img?") {
		u, err := url.Parse(raw)
		if err != nil {
			return nil, err
		}
		raw = u.Query().Get("url")
	}
	var data []byte
	var err error
	if isLocalImagePath(raw) {
		abs, err := filepath.Abs(raw)
		if err != nil {
			return nil, err
		}
		abs, err = filepath.EvalSymlinks(abs)
		if err != nil || !telegramPosterPathAllowed(s.images, abs) {
			return nil, errors.New("poster is outside allowed image roots")
		}
		file, err := os.Open(abs) // #nosec G304 -- resolved path is restricted to configured image roots.
		if err != nil {
			return nil, err
		}
		defer func() { _ = file.Close() }()
		data, err = io.ReadAll(io.LimitReader(file, telegramPhotoMaxBytes+1))
	} else {
		data, _, err = s.images.Fetch(ctx, raw)
	}
	if err != nil {
		return nil, err
	}
	if len(data) > telegramPhotoMaxBytes {
		return nil, errors.New("poster exceeds Telegram photo size limit")
	}
	if _, ok := validImageContentType(data); !ok {
		return nil, errors.New("poster is not a usable image")
	}
	data, contentType, _, err := resizeImageData(data, imageResizeOptions{MaxWidth: 1000, MaxHeight: 1500, Quality: 85})
	if err != nil {
		return nil, err
	}
	if contentType == "image/jpeg" || contentType == "image/png" {
		return data, nil
	}
	// Small WebP/BMP posters can bypass resizing; convert them to a photo
	// format supported by Telegram instead of sending a sticker/document.
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if int64(cfg.Width)*int64(cfg.Height) > imageResizeMaxSourcePixels {
		return nil, errImageResizeTooLarge
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, img, &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	return encoded.Bytes(), nil
}

func telegramPosterPathAllowed(images *ImageProxy, resolved string) bool {
	if images == nil || images.cfg == nil {
		return false
	}
	roots := []string{images.cfg.App.DataDir, images.cfg.Cache.CacheDir, images.cfg.Media.MoviesDir, images.cfg.Media.TVDir, images.cfg.Media.AnimeDir}
	roots = append(roots, images.libraryRoots()...)
	for _, root := range roots {
		if strings.TrimSpace(root) == "" {
			continue
		}
		abs, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		canonical, err := filepath.EvalSymlinks(abs)
		if err == nil && (resolved == canonical || strings.HasPrefix(resolved, canonical+string(filepath.Separator))) {
			return true
		}
	}
	return false
}

func (s *TelegramService) sendPhoto(ctx context.Context, cfg telegramConfig, photo []byte, caption string) error {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range map[string]string{"chat_id": cfg.AdminID, "caption": caption, "parse_mode": "HTML"} {
		if err := writer.WriteField(key, value); err != nil {
			return err
		}
	}
	filename := "poster.jpg"
	if detectContentType(photo) == "image/png" {
		filename = "poster.png"
	}
	part, err := writer.CreateFormFile("photo", filename)
	if err != nil {
		return err
	}
	if _, err := part.Write(photo); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/bot%s/sendPhoto", s.apiBase, cfg.BotToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	client := s.client
	if client == nil {
		client = &http.Client{Timeout: telegramAPITimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		var requestError *url.Error
		if errors.As(err, &requestError) {
			err = requestError.Err
		}
		return fmt.Errorf("telegram sendPhoto: %w", redactSensitiveError(err))
	}
	defer func() { _ = resp.Body.Close() }()
	return decodeTelegramResponse(resp, "sendPhoto", nil)
}
