package service

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/service/cloud"
	"go.uber.org/zap"
)

const testED2K = "ed2k://|file|Movie 01.mkv|123|01234567890123456789012345678901|/"
const rejectedMagnet = "magnet:?xt=urn:btih:0123456789012345678901234567890123456789"

func TestTelegramCloudDownloadLink(t *testing.T) {
	withRootHash := strings.TrimSuffix(testED2K, "/") + "h=SQ7LUTYUSMDBP2TVE2M7T6VUBLU324KF|/"
	withSources := testED2K + "|sources,192.0.2.1:4662|/"
	for _, tc := range []struct{ input, want string }{
		{"/download " + testED2K, testED2K},
		{"/download@NookBot " + testED2K, testED2K},
		{"/download " + withRootHash, withRootHash},
		{"/download " + withSources, withSources},
		{"/download " + rejectedMagnet, ""},
		{"/download " + testED2K + "\n" + testED2K, ""},
		{"/download " + testED2K + " " + testED2K, ""},
		{"/download https://example.org/file.mkv", ""},
		{"/download ed2k://|server|192.0.2.1|4661|/", ""},
		{"/download ed2k://|file|Movie|0|01234567890123456789012345678901|/", ""},
	} {
		if got := cloudDownloadLink(tc.input); got != tc.want {
			t.Errorf("cloudDownloadLink(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestTelegramCloudDownloadStopsAfterStaging(t *testing.T) {
	f := newTelegramControlFixture(t)
	ctx := t.Context()
	const token = "test-token"
	if err := f.strm.repo.DB.AutoMigrate(&model.TelegramCloudDownload{}); err != nil {
		t.Fatal(err)
	}
	f.onCall = func(method string, _ map[string]any) any {
		if method == "sendMessage" {
			return map[string]int64{"message_id": 151}
		}
		return true
	}
	account := &model.StrmAccount{
		Name: "我的 115", Provider: model.StrmProvider115,
		Config: `{"access_token":"test-access","cloud_download_staging_id":"456","cloud_download_staging_path":"/暂存"}`,
	}
	backup := &model.StrmAccount{
		Name: "备用 115", Provider: model.StrmProvider115,
		Config: `{"access_token":"backup-access","cloud_download_staging_id":"789","cloud_download_staging_path":"/备用暂存"}`,
	}
	unconfigured := &model.StrmAccount{
		Name: "未配置暂存", Provider: model.StrmProvider115, Config: `{"access_token":"test-access"}`,
	}
	if err := f.strm.repo.StrmAccount.Create(ctx, unconfigured); err != nil {
		t.Fatal(err)
	}
	message := telegramAdminMessage("/download " + testED2K)
	message.Message.MessageID = 148
	f.bot.handleUpdate(ctx, token, message)
	calls := f.snapshot()
	if len(calls) != 1 || !strings.Contains(calls[0].Payload["text"].(string), "没有配置暂存目录") || calls[0].Payload["reply_markup"] != nil {
		t.Fatalf("an account without staging should get configuration guidance, calls = %+v", calls)
	}
	for _, entry := range []*model.StrmAccount{account, backup} {
		if err := f.strm.repo.StrmAccount.Create(ctx, entry); err != nil {
			t.Fatal(err)
		}
	}

	var submitted, queries atomic.Int32
	now := time.Now().UTC().Truncate(time.Microsecond)
	remote := cloud.NewOpenAPI115("app", "test-access", "test-refresh")
	remote.OpenClient().HTTP.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") != "Bearer test-access" {
			t.Errorf("wrong 115 authorization: %q", req.Header.Get("Authorization"))
		}
		var body string
		switch req.URL.Path {
		case "/open/folder/get_info":
			if req.URL.Query().Get("file_id") != "456" {
				t.Errorf("unexpected staging folder lookup: %s", req.URL)
			}
			body = `{"state":true,"data":{"file_id":"456","file_category":"0","paths":[{"file_id":"0"}]}}`
		case "/open/offline/add_task_urls":
			submitted.Add(1)
			if err := req.ParseMultipartForm(32 << 10); err != nil || req.Form.Get("urls") != testED2K || req.Form.Get("wp_path_id") != "456" || req.Method != http.MethodPost {
				t.Errorf("submit payload: %v (%v)", req.Form, err)
			}
			// The upstream accepts the task before the response is lost.
			return nil, errors.New("connection closed before response")
		case "/open/offline/get_task_list":
			if req.Method != http.MethodGet || req.URL.Query().Get("page") != "1" {
				t.Errorf("unexpected status request: %s %s", req.Method, req.URL)
			}
			status, progress := 1, 100
			switch queries.Add(1) {
			case 1:
				progress = 35
			case 8:
				status = 2
			}
			body = fmt.Sprintf(`{"state":true,"data":{"page_count":1,"tasks":[{"info_hash":"remote-task","wp_path_id":"456","url":%q,"add_time":%d,"status":%d,"percentDone":%d}]}}`, testED2K, now.Unix(), status, progress)
		default:
			t.Errorf("unexpected 115 API: %s", req.URL)
			return nil, errors.New("unexpected API")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})
	f.strm.provider115Cache[account.ID] = remote

	message = telegramAdminMessage("/download " + testED2K)
	message.Message.MessageID = 150
	untrusted := telegramAdminMessage(message.Message.Text)
	untrusted.Message.MessageID = 150
	untrusted.Message.Chat.Type = "group"
	f.bot.handleUpdate(ctx, token, untrusted)
	if len(f.snapshot()) != 1 {
		t.Fatal("a group message was allowed to create a task")
	}
	magnet := telegramAdminMessage("/download " + rejectedMagnet)
	magnet.Message.MessageID = 149
	f.bot.handleUpdate(ctx, token, magnet)
	calls = f.snapshot()
	if len(calls) != 2 || !strings.Contains(calls[1].Payload["text"].(string), "只接受 ED2K") || calls[1].Payload["reply_markup"] != nil {
		t.Fatalf("magnet should get only the ED2K usage reply, calls = %+v", calls)
	}
	f.bot.handleUpdate(ctx, token, message)
	f.bot.handleUpdate(ctx, token, message)
	calls = f.snapshot()
	if len(calls) != 3 || calls[2].Method != "sendMessage" {
		t.Fatalf("replayed message should have one confirmation, calls = %+v", calls)
	}
	keyboard := calls[2].Payload["reply_markup"].(map[string]any)["inline_keyboard"].([]any)
	if len(keyboard) != 2 {
		t.Fatalf("only the two configured accounts should be listed: %+v", keyboard)
	}
	var callback string
	for _, row := range keyboard {
		button := row.([]any)[0].(map[string]any)
		data := button["callback_data"].(string)
		if len(data) > 64 {
			t.Fatalf("invalid callback length: %s", data)
		}
		if strings.HasSuffix(data, ":"+account.ID) && strings.Contains(button["text"].(string), account.Name+" · /暂存") {
			callback = data
		}
	}
	if callback == "" {
		t.Fatalf("confirmation should include the account and staging folder: %+v", keyboard)
	}
	taskID := strings.Split(callback, ":")[2]
	checkTask := func(status string) *model.TelegramCloudDownload {
		t.Helper()
		task, err := f.strm.repo.TelegramCloudDownload.Find(ctx, taskID)
		if err != nil || task == nil || task.Status != status {
			t.Fatalf("expected status %s, found %+v (%v)", status, task, err)
		}
		return task
	}
	draft := checkTask(model.CloudDownloadDraft)
	if !f.strm.crypto.IsEncrypted(draft.EncryptedLink) {
		t.Fatal("source link is not encrypted")
	}
	click := telegramAdminCallback(callback)
	click.CallbackQuery.Message.MessageID = 152
	f.bot.handleUpdate(ctx, token, click)
	checkTask(model.CloudDownloadDraft)
	click.CallbackQuery.Message.MessageID = 151
	f.bot.handleUpdate(ctx, token, click)
	f.bot.handleUpdate(ctx, token, click)
	queued := checkTask(model.CloudDownloadQueued)
	if queued.AccountID != account.ID || queued.StagingFolderID != "456" {
		t.Fatalf("unexpected staging selection: %+v", queued)
	}

	poll := func(at time.Time) {
		f.bot.runCloudDownloads(ctx, token, testTelegramAdminID, at)
	}
	checkNextPoll := func(task *model.TelegramCloudDownload, at time.Time, delay time.Duration) {
		t.Helper()
		if !task.CheckAfter.Equal(at.Add(delay)) || task.PollDelaySeconds != int(delay/time.Second) {
			t.Fatalf("next poll after %s with %ds delay, want %s and %s", task.CheckAfter, task.PollDelaySeconds, at.Add(delay), delay)
		}
	}
	at := now.Add(time.Second)
	poll(at)
	submitting := checkTask(model.CloudDownloadSubmitting)
	checkNextPoll(submitting, at, time.Minute)
	poll(at.Add(30 * time.Second))
	if queries.Load() != 0 {
		t.Fatal("115 was queried less than a minute after submission")
	}

	// Reconcile the accepted submission after restarting the Bot.
	restarted := NewTelegramService(zap.NewNop(), f.strm.repo)
	restarted.SetStrmService(f.strm)
	restarted.apiBase, restarted.client = f.bot.apiBase, f.bot.client
	f.bot = restarted
	at = submitting.CheckAfter
	poll(at)
	tracking := checkTask(model.CloudDownloadTracking)
	if tracking.RemoteTaskID != "remote-task" || tracking.Progress != 35 || tracking.EncryptedLink != "" {
		t.Fatalf("failed to reconcile accepted 115 submission: %+v", tracking)
	}
	checkNextPoll(tracking, at, 5*time.Minute)
	poll(at.Add(time.Minute))
	if queries.Load() != 1 {
		t.Fatal("115 was queried before the next polling deadline")
	}
	at = tracking.CheckAfter
	poll(at)
	if tracking = checkTask(model.CloudDownloadTracking); tracking.Progress != 100 {
		t.Fatalf("100%% can still be an in-progress task: %+v", tracking)
	}
	checkNextPoll(tracking, at, 30*time.Minute)
	for _, delay := range []time.Duration{time.Hour, 6 * time.Hour, 12 * time.Hour, 24 * time.Hour, 24 * time.Hour} {
		at = tracking.CheckAfter
		poll(at)
		tracking = checkTask(model.CloudDownloadTracking)
		checkNextPoll(tracking, at, delay)
	}
	at = tracking.CheckAfter
	poll(at)
	done := checkTask(model.CloudDownloadDone)
	if !done.Reported {
		t.Fatal("download completion was not reported")
	}
	poll(at.Add(24 * time.Hour))
	if submitted.Load() != 1 || queries.Load() != 8 {
		t.Fatalf("expected one submission and no polling after completion, got %d submissions and %d status checks", submitted.Load(), queries.Load())
	}
	var count int64
	if err := f.strm.repo.DB.Model(&model.StrmSyncRecord{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("cloud download should not start a STRM sync: records=%d err=%v", count, err)
	}
	for _, text := range []string{"35%", "下载完成，文件已存入暂存目录"} {
		var found bool
		for _, call := range f.snapshot() {
			message, _ := call.Payload["text"].(string)
			if call.Method == "editMessageText" && call.Payload["message_id"] == float64(151) && strings.Contains(message, text) {
				found = true
			}
		}
		if !found {
			t.Errorf("no edit with %q", text)
		}
	}
}
