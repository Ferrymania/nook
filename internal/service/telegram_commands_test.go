package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/model"
)

const testTelegramAdminID = int64(123456)

type telegramAPICall struct {
	Method  string
	Payload map[string]any
}
type telegramControlFixture struct {
	bot    *TelegramService
	strm   *StrmService
	mu     sync.Mutex
	calls  []telegramAPICall
	onCall func(string, map[string]any) any
}

func newTelegramControlFixture(t *testing.T) *telegramControlFixture {
	t.Helper()
	f := &telegramControlFixture{strm: testStrmService(t)}
	f.bot = NewTelegramService(zap.NewNop(), f.strm.repo)
	f.bot.SetStrmService(f.strm)
	for key, value := range map[string]string{SettingTelegramEnabled: "true", SettingTelegramBotToken: "test-token", SettingTelegramAdminChatID: "123456"} {
		if err := f.strm.repo.Setting.Set(t.Context(), key, value); err != nil {
			t.Fatal(err)
		}
	}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
			return
		}
		method := filepath.Base(r.URL.Path)
		f.mu.Lock()
		f.calls = append(f.calls, telegramAPICall{method, payload})
		f.mu.Unlock()
		var result any = true
		if method == "getUpdates" {
			result = []telegramUpdate{}
		}
		if f.onCall != nil {
			result = f.onCall(method, payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": result})
	}))
	t.Cleanup(api.Close)
	f.bot.apiBase, f.bot.client, f.bot.pollClient = api.URL, api.Client(), api.Client()
	return f
}

func (f *telegramControlFixture) snapshot() []telegramAPICall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]telegramAPICall(nil), f.calls...)
}

func (f *telegramControlFixture) waitMessage(t *testing.T, contains string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, call := range f.snapshot() {
			if text, _ := call.Payload["text"].(string); call.Method == "sendMessage" && strings.Contains(text, contains) {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("missing message containing %q: %+v", contains, f.snapshot())
}

func (f *telegramControlFixture) waitIdle(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		f.bot.mu.Lock()
		idle := f.bot.syncRun == nil
		f.bot.mu.Unlock()
		if idle {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("Telegram batch did not finish")
}

func telegramAdminMessage(text string) telegramUpdate {
	return telegramUpdate{Message: &telegramMessage{Text: text, Date: time.Now().Unix(), Chat: telegramChat{ID: testTelegramAdminID, Type: "private"}, From: telegramSender{ID: testTelegramAdminID}}}
}

func telegramAdminCallback(data string) telegramUpdate {
	return telegramUpdate{CallbackQuery: &telegramCallbackQuery{ID: "callback-id", Data: data, From: telegramSender{ID: testTelegramAdminID}, Message: &telegramMessage{Chat: telegramChat{ID: testTelegramAdminID, Type: "private"}}}}
}

func telegramLocalSyncPath(t *testing.T, f *telegramControlFixture, name string) *model.StrmSyncPath {
	t.Helper()
	src, out := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(src, "movie.mkv"), "video")
	p := syncPathRecord(t, f.strm, model.StrmProviderLocal, src, out, true)
	p.Name, p.SyncMode = name, model.StrmSyncTypeFull // Telegram must explicitly request incremental.
	if err := f.strm.repo.StrmSyncPath.Update(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestTelegramLegacyBindCommandsIgnored(t *testing.T) {
	f := newTelegramControlFixture(t)
	for _, command := range []string{"/bind ABCDEF", "/bind@NookBot ABCDEF"} {
		for _, chatID := range []int64{9, testTelegramAdminID} {
			update := telegramAdminMessage(command)
			update.Message.Chat.ID, update.Message.From.ID = chatID, chatID
			f.bot.handleUpdate(t.Context(), "test-token", update)
		}
	}
	if calls := f.snapshot(); len(calls) != 0 {
		t.Fatalf("removed bind command made Telegram API calls: %+v", calls)
	}
}

func TestTelegramStartOnlyExplainsAdminSetup(t *testing.T) {
	f := newTelegramControlFixture(t)
	update := telegramAdminMessage("/start")
	update.Message.Chat.ID, update.Message.From.ID = 9, 9
	f.bot.handleUpdate(t.Context(), "test-token", update)
	calls := f.snapshot()
	if len(calls) != 1 || calls[0].Method != "sendMessage" {
		t.Fatalf("start calls = %+v, want one setup reply", calls)
	}
	text, _ := calls[0].Payload["text"].(string)
	if strings.Contains(text, "/bind") || strings.Contains(text, "绑定码") || strings.Contains(text, "个人资料") {
		t.Fatalf("start still advertises personal binding: %s", text)
	}
	if !strings.Contains(text, "管理员") || !strings.Contains(text, "Chat ID：<code>9</code>") {
		t.Fatalf("start lost administrator setup guidance: %s", text)
	}
}

func TestTelegramControlRejectsUntrustedChatsAndRevokedConfig(t *testing.T) {
	for _, test := range []string{"other-user", "forged-sender", "group", "disabled", "new-token", "group-admin-id"} {
		t.Run(test, func(t *testing.T) {
			f := newTelegramControlFixture(t)
			telegramLocalSyncPath(t, f, "电影")
			u := telegramAdminMessage("/strm_inc")
			switch test {
			case "other-user":
				u.Message.Chat.ID, u.Message.From.ID = 9, 9
			case "forged-sender":
				u.Message.From.ID = 9
			case "group":
				u.Message.Chat.Type = "group"
			case "disabled":
				_ = f.strm.repo.Setting.Set(t.Context(), SettingTelegramEnabled, "false")
			case "new-token":
				_ = f.strm.repo.Setting.Set(t.Context(), SettingTelegramBotToken, "replacement")
			case "group-admin-id":
				_ = f.strm.repo.Setting.Set(t.Context(), SettingTelegramAdminChatID, "-123456")
			}
			f.bot.handleUpdate(t.Context(), "test-token", u)
			q := telegramAdminCallback("nook:sync:all")
			q.CallbackQuery.Message.Chat, q.CallbackQuery.From = u.Message.Chat, u.Message.From
			f.bot.handleUpdate(t.Context(), "test-token", q)
			for _, call := range f.snapshot() {
				if call.Method != "answerCallbackQuery" {
					t.Fatalf("unauthorized API call: %+v", call)
				}
			}
			records, err := f.strm.ListSyncRecords(t.Context(), "", 10)
			if err != nil || len(records) != 0 {
				t.Fatalf("unauthorized sync: %v %+v", err, records)
			}
		})
	}
}

func TestTelegramAdminMenuRegistrationIsScopedAndRotates(t *testing.T) {
	f := newTelegramControlFixture(t)
	cfg := f.bot.config(t.Context())
	if err := f.bot.registerAdminCommands(t.Context(), cfg, "old-token", "654321"); err != nil {
		t.Fatal(err)
	}
	calls := f.snapshot()
	if len(calls) != 2 || calls[0].Method != "deleteMyCommands" || calls[1].Method != "setMyCommands" {
		t.Fatalf("calls: %+v", calls)
	}
	scope := calls[1].Payload["scope"].(map[string]any)
	if scope["type"] != "chat" || scope["chat_id"] != float64(testTelegramAdminID) {
		t.Fatalf("scope: %+v", scope)
	}
	commands := calls[1].Payload["commands"].([]any)
	if len(commands) != 5 {
		t.Fatalf("commands: %+v", commands)
	}
	cfg.AdminID = "-100123"
	if err := f.bot.registerAdminCommands(t.Context(), cfg, "", ""); err != nil {
		t.Fatal(err)
	}
	if len(f.snapshot()) != 2 {
		t.Fatal("registered management menu for a group")
	}
}

func TestTelegramDirectoryButtonsPaginateAndRecheckDisabledPaths(t *testing.T) {
	f := newTelegramControlFixture(t)
	var last *model.StrmSyncPath
	for i := 0; i < 10; i++ {
		last = telegramLocalSyncPath(t, f, fmt.Sprintf("目录%d", i))
	}
	f.bot.handleUpdate(t.Context(), "test-token", telegramAdminMessage("/sync"))
	calls := f.snapshot()
	rows := calls[0].Payload["reply_markup"].(map[string]any)["inline_keyboard"].([]any)
	if len(rows) != 10 {
		t.Fatalf("want 8 directories, next page and all button: %+v", rows)
	}
	f.bot.handleUpdate(t.Context(), "test-token", telegramAdminCallback("nook:page:sync:1"))
	if f.snapshot()[1].Method != "answerCallbackQuery" {
		t.Fatal("callback was not acknowledged before responding")
	}
	last.Enabled = false
	if err := f.strm.repo.StrmSyncPath.Update(t.Context(), last); err != nil {
		t.Fatal(err)
	}
	f.bot.handleUpdate(t.Context(), "test-token", telegramAdminCallback("nook:sync:"+last.ID))
	f.waitMessage(t, "目录不存在或已禁用")
	records, _ := f.strm.ListSyncRecords(t.Context(), last.ID, 10)
	if len(records) != 0 {
		t.Fatal("stale button started a disabled path")
	}
}

func TestTelegramIncrementalSyncUsesRealEngineAndReportsResults(t *testing.T) {
	f := newTelegramControlFixture(t)
	a := telegramLocalSyncPath(t, f, "电影&lt;&gt;")
	b := telegramLocalSyncPath(t, f, "电视剧")
	disabled := telegramLocalSyncPath(t, f, "已禁用")
	disabled.Enabled = false
	if err := f.strm.repo.StrmSyncPath.Update(t.Context(), disabled); err != nil {
		t.Fatal(err)
	}
	f.bot.handleUpdate(t.Context(), "test-token", telegramAdminMessage("/strm_inc@NookBot"))
	f.waitMessage(t, "成功 2，失败/未启动 0")
	f.waitIdle(t)
	var records []model.StrmSyncRecord
	for _, p := range []*model.StrmSyncPath{a, b} {
		r, err := f.strm.ListSyncRecords(t.Context(), p.ID, 10)
		if err != nil || len(r) != 1 || r[0].Status != model.StrmSyncRecordDone || r[0].SyncType != model.StrmSyncTypeIncremental || r[0].NewStrm != 1 {
			t.Fatalf("sync result: %v %+v", err, r)
		}
		records = append(records, r[0])
		if _, err := os.Stat(filepath.Join(p.LocalPath, "movie.strm")); err != nil {
			t.Fatal(err)
		}
	}
	if !records[0].FinishedAt.Before(*records[1].StartedAt) && !records[1].FinishedAt.Before(*records[0].StartedAt) {
		t.Fatal("directories overlapped")
	}
	r, _ := f.strm.ListSyncRecords(t.Context(), disabled.ID, 10)
	if len(r) != 0 {
		t.Fatal("disabled path was synchronized")
	}
	f.waitMessage(t, "元数据下载入队：0")
	// A directory button uses the same engine and sees unchanged files as skipped.
	f.bot.handleUpdate(t.Context(), "test-token", telegramAdminCallback("nook:sync:"+a.ID))
	f.waitMessage(t, "新增/更新 STRM：0，跳过：1")
	f.waitIdle(t)
	f.bot.handleUpdate(t.Context(), "test-token", telegramAdminMessage("/status"))
	f.waitMessage(t, "STRM 同步状态")
}

func TestTelegramCancelBatchStopsRunningDirectoryAndPendingOnes(t *testing.T) {
	f := newTelegramControlFixture(t)
	queued := telegramLocalSyncPath(t, f, "待执行")
	entered := make(chan struct{}, 1)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case entered <- struct{}{}:
		default:
		}
		<-r.Context().Done()
	}))
	t.Cleanup(remote.Close)
	acct, err := f.strm.CreateStrmAccount(t.Context(), "slow", model.StrmProviderOpenList, map[string]string{"server": remote.URL, "token": "fake"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := f.strm.CreateSyncPath(t.Context(), &model.StrmSyncPath{Name: "慢目录", Provider: model.StrmProviderOpenList, AccountID: acct.ID, RemotePath: "/", LocalPath: t.TempDir(), Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	// Repository order is newest first, so the remote directory blocks the batch.
	f.bot.handleUpdate(t.Context(), "test-token", telegramAdminMessage("/strm_inc"))
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("sync did not reach remote")
	}
	f.bot.handleUpdate(t.Context(), "test-token", telegramAdminMessage("/sync_all"))
	f.waitMessage(t, "已有 Telegram 同步任务")
	f.bot.handleUpdate(t.Context(), "test-token", telegramAdminMessage("/status"))
	f.waitMessage(t, "待执行 1 个目录")
	f.bot.handleUpdate(t.Context(), "test-token", telegramAdminMessage("/cancel"))
	f.bot.handleUpdate(t.Context(), "test-token", telegramAdminCallback("nook:cancel:batch"))
	f.waitMessage(t, "增量 STRM 同步已取消")
	f.waitIdle(t)
	r, err := f.strm.ListSyncRecords(t.Context(), p.ID, 10)
	if err != nil || len(r) != 1 || r[0].Status != model.StrmSyncRecordCanceled {
		t.Fatalf("canceled record: %v %+v", err, r)
	}
	r, _ = f.strm.ListSyncRecords(t.Context(), queued.ID, 10)
	if len(r) != 0 {
		t.Fatal("batch continued after cancellation")
	}
	f.bot.handleUpdate(t.Context(), "test-token", telegramAdminCallback("nook:sync:"+queued.ID))
	f.waitMessage(t, "成功 1，失败/未启动 0")
	f.waitIdle(t)
}

func TestTelegramPollingDropsBacklogAndRequestsCallbacks(t *testing.T) {
	f := newTelegramControlFixture(t)
	telegramLocalSyncPath(t, f, "电影")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var fetches int
	f.onCall = func(method string, payload map[string]any) any {
		if method != "getUpdates" {
			return true
		}
		fetches++
		updates, _ := payload["allowed_updates"].([]any)
		if len(updates) != 2 || updates[1] != "callback_query" {
			t.Errorf("update types: %+v", payload)
		}
		if fetches == 1 {
			if payload["offset"] != float64(-1) || payload["timeout"] != float64(0) {
				t.Errorf("bootstrap: %+v", payload)
			}
			old := telegramAdminCallback("nook:sync:all")
			old.UpdateID = 40
			return []telegramUpdate{old}
		}
		if fetches == 2 {
			if payload["offset"] != float64(41) {
				t.Errorf("offset: %+v", payload)
			}
			fresh := telegramAdminMessage("/strm_inc")
			fresh.UpdateID = 41
			return []telegramUpdate{fresh, fresh} // Duplicate delivery must not start twice.
		}
		<-ctx.Done()
		return []telegramUpdate{}
	}
	done := make(chan struct{})
	go func() { defer close(done); f.bot.pollLoop(ctx) }()
	f.waitMessage(t, "成功 1，失败/未启动 0")
	f.waitIdle(t)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("poller did not stop")
	}
	var starts int
	for _, c := range f.snapshot() {
		if text, _ := c.Payload["text"].(string); strings.Contains(text, "开始执行") {
			starts++
		}
	}
	if starts != 1 {
		t.Fatalf("backlog or duplicate replayed: %d starts", starts)
	}
}

func TestTelegramReplyRevocationAndFailureEscaping(t *testing.T) {
	f := newTelegramControlFixture(t)
	text := telegramSyncResult(model.StrmSyncPath{Name: "<电影>"}, model.StrmSyncRecord{Status: model.StrmSyncRecordFailed, Message: "api_key=secret <bad>"})
	if strings.Contains(text, "secret") || strings.Contains(text, "<bad>") || !strings.Contains(text, "&lt;电影&gt;") {
		t.Fatalf("unsafe message: %s", text)
	}
	_ = f.strm.repo.Setting.Set(t.Context(), SettingTelegramAdminChatID, "654321")
	f.bot.controlReply(t.Context(), "test-token", "123456", text, nil)
	if len(f.snapshot()) != 0 {
		t.Fatal("old admin received a result after revocation")
	}
	api := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	api.Close()
	f.bot.apiBase = api.URL
	err := f.bot.call(context.Background(), "secret-token", "getMe", nil, nil)
	if err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("token leaked via transport error: %v", err)
	}
}

func TestTelegramBatchContinuesPastFailedAndAlreadyRunningDirectories(t *testing.T) {
	f := newTelegramControlFixture(t)
	good := telegramLocalSyncPath(t, f, "可同步目录")
	bad := telegramLocalSyncPath(t, f, "错误源目录")
	bad.RemotePath = filepath.Join(bad.RemotePath, "movie.mkv") // A file cannot be walked as a directory.
	if err := f.strm.repo.StrmSyncPath.Update(t.Context(), bad); err != nil {
		t.Fatal(err)
	}
	busy := telegramLocalSyncPath(t, f, "网页已在同步")
	var canceled bool
	f.strm.mu.Lock()
	f.strm.running[busy.ID] = func() { canceled = true }
	f.strm.mu.Unlock()
	f.bot.handleUpdate(t.Context(), "test-token", telegramAdminMessage("/strm_inc"))
	f.waitMessage(t, "成功 1，失败/未启动 2")
	f.waitIdle(t)
	if canceled {
		t.Fatal("Telegram canceled a run owned by the web UI")
	}
	r, err := f.strm.ListSyncRecords(t.Context(), good.ID, 10)
	if err != nil || len(r) != 1 || r[0].Status != model.StrmSyncRecordDone {
		t.Fatalf("remaining directory was not synchronized: %v %+v", err, r)
	}
	f.waitMessage(t, "网页已在同步 未启动")
	f.waitMessage(t, "同步失败：错误源目录")
}
