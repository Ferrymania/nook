package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type telegramMediaFixture struct {
	mu                      sync.Mutex
	bot                     *TelegramService
	lib                     model.Library
	now                     time.Time
	calls                   []telegramAPICall
	photoStatus, textStatus int
}

func newTelegramMediaFixture(t *testing.T) *telegramMediaFixture {
	t.Helper()
	repo := repository.New(newServiceTestDB(t))
	f := &telegramMediaFixture{bot: NewTelegramService(zap.NewNop(), repo), now: time.Now().UTC().Truncate(time.Second)}
	for key, value := range map[string]string{SettingTelegramEnabled: "true", SettingTelegramBotToken: "fake-token", SettingTelegramAdminChatID: "123"} {
		if err := repo.Setting.Set(t.Context(), key, value); err != nil {
			t.Fatal(err)
		}
	}
	f.lib = model.Library{Name: "外语电影", Type: "movie", Path: t.TempDir()}
	if err := repo.Library.Create(t.Context(), &f.lib); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := filepath.Base(r.URL.Path)
		var payload map[string]any
		if method == "sendPhoto" {
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
				return
			}
			defer r.MultipartForm.RemoveAll()
			payload = map[string]any{"chat_id": r.FormValue("chat_id"), "caption": r.FormValue("caption")}
		} else if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
			return
		}
		f.mu.Lock()
		f.calls = append(f.calls, telegramAPICall{Method: method, Payload: payload})
		status := f.textStatus
		if method == "sendPhoto" {
			status = f.photoStatus
		}
		f.mu.Unlock()
		if status >= 400 {
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "description": "test failure", "parameters": map[string]int{"retry_after": 120}})
			return
		}
		_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":1}}`)
	}))
	t.Cleanup(server.Close)
	f.bot.apiBase, f.bot.client = server.URL, server.Client()
	return f
}

func (f *telegramMediaFixture) add(t *testing.T, id, status string, created time.Time) model.Media {
	t.Helper()
	m := model.Media{Base: model.Base{ID: id, CreatedAt: created}, LibraryID: f.lib.ID, Title: id, Path: filepath.Join(f.lib.Path, id+".strm"), ScrapeStatus: status, Year: 2026}
	if err := f.bot.repo.DB.Create(&m).Error; err != nil {
		t.Fatal(err)
	}
	return m
}

func (f *telegramMediaFixture) baseline(t *testing.T) {
	t.Helper()
	f.add(t, "history", "matched", f.now.Add(-time.Hour))
	if _, err := f.bot.mediaNotificationCursor(t.Context(), f.bot.config(t.Context()), f.now); err != nil {
		t.Fatal(err)
	}
}

func (f *telegramMediaFixture) step(t *testing.T, now time.Time) bool {
	t.Helper()
	advanced, err := f.bot.processMediaNotification(t.Context(), now)
	if err != nil {
		t.Fatal(err)
	}
	return advanced
}

func TestTelegramMediaOnlyNewRowsAndDurableCursor(t *testing.T) {
	f := newTelegramMediaFixture(t)
	f.baseline(t)
	if f.step(t, f.now) || len(f.snapshot()) != 0 {
		t.Fatal("history was notified")
	}
	// Equal timestamps are normal for batch imports and need an ID tie-breaker.
	first := f.add(t, "a-new", "matched", f.now.Add(-time.Minute))
	f.add(t, "b-new", "matched", first.CreatedAt)
	if !f.step(t, f.now) || !f.step(t, f.now) || f.step(t, f.now) || len(f.snapshot()) != 2 {
		t.Fatalf("new-row calls=%+v", f.snapshot())
	}
	first.Title = "更新已有元数据"
	if err := f.bot.repo.Media.Upsert(t.Context(), &first); err != nil {
		t.Fatal(err)
	}
	if f.step(t, f.now) || len(f.snapshot()) != 2 {
		t.Fatal("rescan/update was notified twice")
	}
	// A fresh TelegramService reads the checkpoint rather than replaying imports.
	restarted := NewTelegramService(zap.NewNop(), f.bot.repo)
	restarted.apiBase, restarted.client = f.bot.apiBase, f.bot.client
	f.bot = restarted
	if f.step(t, f.now) || len(f.snapshot()) != 2 {
		t.Fatal("restart replayed a notification")
	}
	raw, err := f.bot.repo.Setting.Get(t.Context(), telegramMediaCursorKey)
	if err != nil || strings.Contains(raw, "fake-token") {
		t.Fatalf("unsafe cursor=%q %v", raw, err)
	}
}

func TestTelegramMediaWaitsForMetadataAndFallsBackAfterDeadline(t *testing.T) {
	f := newTelegramMediaFixture(t)
	f.baseline(t)
	m := f.add(t, "pending", "pending", f.now.Add(-time.Minute))
	if f.step(t, f.now) || len(f.snapshot()) != 0 {
		t.Fatal("pending metadata was sent too soon")
	}
	if err := f.bot.repo.DB.Model(&m).Updates(map[string]any{"scrape_status": "matched", "title": "刮削后片名", "genres": "动作"}).Error; err != nil {
		t.Fatal(err)
	}
	if !f.step(t, f.now) || len(f.snapshot()) != 1 || !strings.Contains(f.snapshot()[0].Payload["text"].(string), "刮削后片名") {
		t.Fatalf("not using final metadata: %+v", f.snapshot())
	}
	f.add(t, "no-provider", "pending", f.now.Add(-30*time.Second))
	if f.step(t, f.now) {
		t.Fatal("metadata deadline ignored")
	}
	if !f.step(t, f.now.Add(telegramMediaMetadataWait)) || len(f.snapshot()) != 2 {
		t.Fatal("missing metadata blocked notifications forever")
	}
}

func TestTelegramMediaDisabledAndRecipientChangesSkipBacklog(t *testing.T) {
	f := newTelegramMediaFixture(t)
	f.baseline(t)
	f.add(t, "disabled", "matched", f.now.Add(-time.Minute))
	if err := f.bot.repo.Setting.Set(t.Context(), SettingTelegramEnabled, "false"); err != nil {
		t.Fatal(err)
	}
	f.step(t, f.now)
	if err := f.bot.repo.Setting.Set(t.Context(), SettingTelegramEnabled, "true"); err != nil {
		t.Fatal(err)
	}
	f.step(t, f.now)
	if len(f.snapshot()) != 0 {
		t.Fatal("disabled-period media was replayed")
	}
	f.add(t, "old-recipient", "matched", f.now.Add(-50*time.Second))
	if err := f.bot.repo.Setting.Set(t.Context(), SettingTelegramAdminChatID, "456"); err != nil {
		t.Fatal(err)
	}
	f.step(t, f.now)
	if len(f.snapshot()) != 0 {
		t.Fatal("backlog leaked to a new recipient")
	}
	f.add(t, "new-recipient", "matched", f.now.Add(-40*time.Second))
	if !f.step(t, f.now) || len(f.snapshot()) != 1 || f.snapshot()[0].Payload["chat_id"] != "456" {
		t.Fatalf("recipient=%+v", f.snapshot())
	}
}

func TestTelegramMediaSendFailureKeepsCheckpoint(t *testing.T) {
	f := newTelegramMediaFixture(t)
	f.baseline(t)
	f.add(t, "retry", "matched", f.now.Add(-time.Minute))
	f.setTextStatus(http.StatusTooManyRequests)
	advanced, err := f.bot.processMediaNotification(t.Context(), f.now)
	var apiErr *telegramAPIError
	if advanced || !errors.As(err, &apiErr) || apiErr.RetryAfter != 120 {
		t.Fatalf("send failure=%v %v", advanced, err)
	}
	f.setTextStatus(0)
	if !f.step(t, f.now) || len(f.snapshot()) != 2 {
		t.Fatal("failed notification could not retry")
	}
	if f.step(t, f.now) || len(f.snapshot()) != 2 {
		t.Fatal("successful notification was replayed")
	}
}

func TestTelegramMediaPhotoAndTextFallback(t *testing.T) {
	for _, photoStatus := range []int{200, 400, 429, 503} {
		t.Run(http.StatusText(photoStatus), func(t *testing.T) {
			f := newTelegramMediaFixture(t)
			f.baseline(t)
			cfg := &config.Config{}
			cfg.App.DataDir = f.lib.Path
			cfg.Cache.CacheDir = t.TempDir()
			poster := filepath.Join(f.lib.Path, "poster.png")
			if err := os.WriteFile(poster, encodeTestPNG(t, 30, 45, 255), 0600); err != nil {
				t.Fatal(err)
			}
			f.bot.SetMediaNotificationSources(NewImageProxy(cfg, zap.NewNop()), func(_ context.Context, _ *model.Media) []map[string]any {
				return []map[string]any{{"Name": "演员甲", "Type": "Actor"}}
			})
			m := f.add(t, "photo", "matched", f.now.Add(-time.Minute))
			if err := f.bot.repo.DB.Model(&m).Update("poster_url", poster).Error; err != nil {
				t.Fatal(err)
			}
			f.setPhotoStatus(photoStatus)
			advanced, err := f.bot.processMediaNotification(t.Context(), f.now)
			if photoStatus == 429 || photoStatus == 503 {
				if advanced || err == nil || len(f.snapshot()) != 1 {
					t.Fatalf("throttling bypassed: %+v %v", f.snapshot(), err)
				}
				return
			}
			if !advanced || err != nil || len(f.snapshot()) == 0 || f.snapshot()[0].Method != "sendPhoto" {
				t.Fatalf("photo=%+v %v", f.snapshot(), err)
			}
			if !strings.Contains(f.snapshot()[0].Payload["caption"].(string), "主演：演员甲") {
				t.Fatal("cast missing from caption")
			}
			if photoStatus == 400 && (len(f.snapshot()) != 2 || f.snapshot()[1].Method != "sendMessage") {
				t.Fatal("rejected photo did not fall back to text")
			}
			if photoStatus == 200 && len(f.snapshot()) != 1 {
				t.Fatal("photo sent redundant text")
			}
		})
	}
}

func TestTelegramMediaRecipientChangeDuringPreparationDoesNotSend(t *testing.T) {
	f := newTelegramMediaFixture(t)
	f.baseline(t)
	f.add(t, "changing-recipient", "matched", f.now.Add(-time.Minute))
	f.bot.SetMediaNotificationSources(nil, func(ctx context.Context, _ *model.Media) []map[string]any {
		if err := f.bot.repo.Setting.Set(ctx, SettingTelegramAdminChatID, "456"); err != nil {
			t.Error(err)
		}
		return nil
	})
	advanced, err := f.bot.processMediaNotification(t.Context(), f.now)
	if advanced || err == nil || len(f.snapshot()) != 0 {
		t.Fatalf("sent to revoked recipient: %+v %v", f.snapshot(), err)
	}
}

func (f *telegramMediaFixture) snapshot() []telegramAPICall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]telegramAPICall(nil), f.calls...)
}
func (f *telegramMediaFixture) setTextStatus(status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.textStatus = status
}
func (f *telegramMediaFixture) setPhotoStatus(status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.photoStatus = status
}

func TestTelegramMediaDatabaseOperationsHaveBoundedContexts(t *testing.T) {
	f := newTelegramMediaFixture(t)
	interrupted := errors.New("stop before database query")
	var remaining time.Duration
	probe := func(tx *gorm.DB) {
		deadline, ok := tx.Statement.Context.Deadline()
		if ok {
			remaining = time.Until(deadline)
		} else {
			remaining = 0
		}
		tx.AddError(interrupted)
	}
	db := f.bot.repo.DB
	if err := db.Callback().Row().Before("gorm:row").Register("test:telegram_deadline", probe); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Query().Before("gorm:query").Register("test:telegram_deadline", probe); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Callback().Row().Remove("test:telegram_deadline")
		_ = db.Callback().Query().Remove("test:telegram_deadline")
	})
	if err := f.bot.initializeMediaNotifications(t.Context()); !errors.Is(err, interrupted) {
		t.Fatalf("initialization error=%v", err)
	}
	if remaining <= 0 || remaining > telegramMediaInitTimeout {
		t.Fatalf("startup query timeout=%s", remaining)
	}
	if _, err := f.bot.processMediaNotification(t.Context(), f.now); !errors.Is(err, interrupted) {
		t.Fatalf("background pass error=%v", err)
	}
	if remaining <= telegramMediaInitTimeout || remaining > telegramMediaPassTimeout {
		t.Fatalf("background query timeout=%s", remaining)
	}
}

func (f *telegramMediaFixture) addEpisode(t *testing.T, id, series string, episode int, created time.Time) model.Media {
	t.Helper()
	m := f.add(t, id, "matched", created)
	if err := f.bot.repo.DB.Model(&m).Updates(map[string]any{"title": "同一部剧", "series_id": series, "season_num": 1, "episode_num": episode, "tm_db_id": 123}).Error; err != nil {
		t.Fatal(err)
	}
	m.Title, m.SeriesID, m.SeasonNum, m.EpisodeNum, m.TMDbID = "同一部剧", series, 1, episode, 123
	return m
}

func TestTelegramMediaGroupsInterleavedEpisodesAcrossRestart(t *testing.T) {
	f := newTelegramMediaFixture(t)
	f.baseline(t)
	when := f.now.Add(-time.Minute)
	f.addEpisode(t, "a", "same-series", 11, when)
	f.add(t, "b-movie", "matched", when)
	f.addEpisode(t, "c", "same-series", 12, when)
	f.addEpisode(t, "d", "different-series", 1, when)
	if err := f.bot.repo.DB.Create(&model.Series{Base: model.Base{ID: "same-series"}, LibraryID: f.lib.ID, Title: "整剧标题", Overview: "整剧简介"}).Error; err != nil {
		t.Fatal(err)
	}
	if !f.step(t, f.now) || len(f.snapshot()) != 1 {
		t.Fatalf("group=%+v", f.snapshot())
	}
	caption := f.snapshot()[0].Payload["text"].(string)
	for _, want := range []string{"S01E11–E12 · 新增 2 集", "整剧标题", "整剧简介"} {
		if !strings.Contains(caption, want) {
			t.Fatalf("missing %q: %s", want, caption)
		}
	}
	raw, err := f.bot.repo.Setting.Get(t.Context(), telegramMediaCursorKey)
	if err != nil {
		t.Fatal(err)
	}
	var cursor telegramMediaCursor
	if err := json.Unmarshal([]byte(raw), &cursor); err != nil {
		t.Fatal(err)
	}
	if cursor.MediaID != "a" || len(cursor.SentAhead) != 1 || cursor.SentAhead[0].MediaID != "c" {
		t.Fatalf("progress skipped movie: %+v", cursor)
	}
	restarted := NewTelegramService(zap.NewNop(), f.bot.repo)
	restarted.apiBase, restarted.client = f.bot.apiBase, f.bot.client
	f.bot = restarted
	if !f.step(t, f.now) || !f.step(t, f.now) || f.step(t, f.now) || len(f.snapshot()) != 3 {
		t.Fatalf("lost or duplicated interleaved media: %+v", f.snapshot())
	}
	if !strings.Contains(f.snapshot()[1].Payload["text"].(string), "b-movie") || !strings.Contains(f.snapshot()[2].Payload["text"].(string), "S01E01") {
		t.Fatalf("other media missing: %+v", f.snapshot())
	}
}

func TestTelegramMediaGroupQuietTimeAndLaterBatch(t *testing.T) {
	f := newTelegramMediaFixture(t)
	f.baseline(t)
	f.addEpisode(t, "first", "series", 11, f.now.Add(-20*time.Second))
	second := f.addEpisode(t, "second", "series", 12, f.now.Add(-10*time.Second))
	if f.step(t, f.now) || len(f.snapshot()) != 0 {
		t.Fatal("burst sent individual episode too early")
	}
	if err := f.bot.repo.DB.Model(&second).Update("scrape_status", "pending").Error; err != nil {
		t.Fatal(err)
	}
	if f.step(t, f.now.Add(21*time.Second)) {
		t.Fatal("group did not wait for episode metadata")
	}
	if err := f.bot.repo.DB.Model(&second).Update("scrape_status", "matched").Error; err != nil {
		t.Fatal(err)
	}
	if !f.step(t, f.now.Add(21*time.Second)) || len(f.snapshot()) != 1 {
		t.Fatal("settled episodes did not form one notification")
	}
	f.addEpisode(t, "third", "series", 13, f.now.Add(3*time.Minute))
	if !f.step(t, f.now.Add(4*time.Minute)) || len(f.snapshot()) != 2 {
		t.Fatal("later episode batch was suppressed")
	}
	if strings.Contains(f.snapshot()[1].Payload["text"].(string), "E11") || !strings.Contains(f.snapshot()[1].Payload["text"].(string), "E13") {
		t.Fatal("later batch replayed previously sent episodes")
	}
}

func TestTelegramMediaGroupFailureDoesNotAcknowledgeEpisodes(t *testing.T) {
	f := newTelegramMediaFixture(t)
	f.baseline(t)
	when := f.now.Add(-time.Minute)
	f.addEpisode(t, "a", "series", 11, when)
	f.add(t, "b-movie", "matched", when)
	f.addEpisode(t, "c", "series", 12, when)
	before, err := f.bot.repo.Setting.Get(t.Context(), telegramMediaCursorKey)
	if err != nil {
		t.Fatal(err)
	}
	f.setTextStatus(http.StatusTooManyRequests)
	if advanced, err := f.bot.processMediaNotification(t.Context(), f.now); advanced || err == nil {
		t.Fatal("failed group advanced progress")
	}
	after, err := f.bot.repo.Setting.Get(t.Context(), telegramMediaCursorKey)
	if err != nil || before != after {
		t.Fatal("failed group changed durable acknowledgements")
	}
	f.setTextStatus(0)
	if !f.step(t, f.now) || !f.step(t, f.now) || f.step(t, f.now) || len(f.snapshot()) != 3 {
		t.Fatalf("group retry lost media: %+v", f.snapshot())
	}
}
