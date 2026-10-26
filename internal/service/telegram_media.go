package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Ferrymania/nook/internal/helper"
	"github.com/Ferrymania/nook/internal/model"
)

const (
	telegramMediaCursorKey    = "telegram.media_ingest_cursor"
	telegramMediaSettleTime   = 10 * time.Second
	telegramMediaMetadataWait = 5 * time.Minute
	telegramMediaInitTimeout  = 2 * time.Second
	telegramMediaPassTimeout  = 30 * time.Second
)

// The durable cursor tracks inserts, not metadata updates. Initial startup and
// changes of recipient establish a baseline so an existing library is never
// broadcast as newly imported media. It contains no Bot Token or media URLs.
type telegramMediaCursor struct {
	CreatedAt   time.Time `json:"created_at"`
	MediaID     string    `json:"media_id"`
	Destination string    `json:"destination"`
}

func (s *TelegramService) SetMediaNotificationSources(images *ImageProxy, people func(context.Context, *model.Media) []map[string]any) {
	s.images, s.people = images, people
}

// StartMediaNotifications runs before watchers/sync workers can import files.
// All network work happens in one background worker, never on the ingest path.
func (s *TelegramService) StartMediaNotifications(ctx context.Context) {
	if s == nil || s.repo == nil || s.repo.DB == nil || s.repo.Setting == nil {
		return
	}
	s.mediaOnce.Do(func() {
		// Synchronously establish the initial baseline, without sending messages.
		if err := s.initializeMediaNotifications(ctx); err != nil {
			s.warn("telegram media notification initialization failed", "admin", err)
		}
		helper.Go(s.log, "telegram.mediaNotifications", func() {
			for ctx.Err() == nil {
				advanced, err := s.processMediaNotification(ctx, time.Now())
				delay := 10 * time.Second
				if advanced {
					delay = time.Second // Also bounds Telegram sends during large imports.
				}
				if err != nil {
					s.warn("telegram media notification failed", "admin", err)
					delay = time.Minute
					var apiErr *telegramAPIError
					if errors.As(err, &apiErr) && time.Duration(apiErr.RetryAfter)*time.Second > delay {
						delay = time.Duration(apiErr.RetryAfter) * time.Second
					}
				}
				if !sleepCtx(ctx, delay) {
					return
				}
			}
		})
	})
}

func (s *TelegramService) initializeMediaNotifications(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, telegramMediaInitTimeout)
	defer cancel()
	cfg, err := s.mediaNotificationConfig(ctx)
	if err != nil {
		return err
	}
	_, err = s.mediaNotificationCursor(ctx, cfg, time.Now())
	return err
}

func telegramMediaDestination(cfg telegramConfig) string {
	if !cfg.Enabled || cfg.BotToken == "" || cfg.AdminID == "" {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(cfg.BotToken+"\x00"+cfg.AdminID)))
}

func (s *TelegramService) mediaNotificationConfig(ctx context.Context) (telegramConfig, error) {
	values := make([]string, 3)
	for i, key := range []string{SettingTelegramEnabled, SettingTelegramBotToken, SettingTelegramAdminChatID} {
		value, err := s.repo.Setting.Get(ctx, key)
		if err != nil {
			return telegramConfig{}, err
		}
		values[i] = strings.TrimSpace(value)
	}
	return telegramConfig{Enabled: parseBoolSetting(values[0], false), BotToken: values[1], AdminID: values[2]}, nil
}

func (s *TelegramService) checkMediaNotificationDestination(ctx context.Context, cfg telegramConfig) error {
	current, err := s.mediaNotificationConfig(ctx)
	if err != nil {
		return err
	}
	if telegramMediaDestination(current) != telegramMediaDestination(cfg) {
		return errors.New("telegram media notification configuration changed")
	}
	return nil
}

func (s *TelegramService) saveMediaNotificationCursor(ctx context.Context, cursor telegramMediaCursor) error {
	raw, err := json.Marshal(cursor)
	if err != nil {
		return err
	}
	return s.repo.Setting.Set(ctx, telegramMediaCursorKey, string(raw))
}

func (s *TelegramService) mediaNotificationCursor(ctx context.Context, cfg telegramConfig, now time.Time) (telegramMediaCursor, error) {
	raw, err := s.repo.Setting.Get(ctx, telegramMediaCursorKey)
	if err != nil {
		return telegramMediaCursor{}, err
	}
	var cursor telegramMediaCursor
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &cursor); err != nil {
			return cursor, err
		}
	}
	destination := telegramMediaDestination(cfg)
	if raw != "" && destination != "" && cursor.Destination == destination {
		return cursor, nil
	}
	var latest model.Media
	if err := s.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Select("id", "created_at").Order("created_at DESC, id DESC").Limit(1).Find(&latest).Error; err != nil {
		return cursor, err
	}
	next := telegramMediaCursor{CreatedAt: latest.CreatedAt, MediaID: latest.ID, Destination: destination}
	if latest.ID == "" {
		next.CreatedAt = now.UTC()
		if raw != "" && cursor.MediaID == "" {
			next.CreatedAt = cursor.CreatedAt
		}
	}
	if next != cursor {
		if err := s.saveMediaNotificationCursor(ctx, next); err != nil {
			return cursor, err
		}
	}
	return next, nil
}

func (s *TelegramService) processMediaNotification(ctx context.Context, now time.Time) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, telegramMediaPassTimeout)
	defer cancel()
	cfg, err := s.mediaNotificationConfig(ctx)
	if err != nil {
		return false, err
	}
	cursor, err := s.mediaNotificationCursor(ctx, cfg, now)
	if err != nil || cursor.Destination == "" {
		return false, err
	}
	var rows []model.Media
	if err := s.repo.DB.WithContext(ctx).
		Where("(created_at, id) > (?, ?)", cursor.CreatedAt, cursor.MediaID).
		Order("created_at ASC, id ASC").Limit(1).Find(&rows).Error; err != nil {
		return false, err
	}
	if len(rows) == 0 {
		return false, nil
	}
	m := rows[0]
	// Let the short insert transaction and its metadata work settle. Pending
	// scrapes get up to five minutes; failed/disabled scraping still notifies
	// using the available metadata rather than blocking the queue forever.
	if now.Before(m.CreatedAt.Add(telegramMediaSettleTime)) ||
		((m.ScrapeStatus == "pending" || m.ScrapeStatus == "") && now.Before(m.CreatedAt.Add(telegramMediaMetadataWait))) {
		return false, nil
	}
	lib, err := s.repo.Library.FindByID(ctx, m.LibraryID)
	if err != nil {
		return false, err
	}
	if lib != nil {
		var people []map[string]any
		if s.people != nil {
			peopleCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
			people = s.people(peopleCtx, &m)
			cancel()
		}
		caption := formatTelegramMediaNotification(m, *lib, people)
		if err := s.checkMediaNotificationDestination(ctx, cfg); err != nil {
			return false, err
		}
		if err := s.sendMediaNotification(ctx, cfg, m.PosterURL, caption); err != nil {
			return false, err
		}
	}
	cursor.CreatedAt, cursor.MediaID = m.CreatedAt, m.ID
	return true, s.saveMediaNotificationCursor(ctx, cursor)
}
