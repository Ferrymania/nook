package service

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/service/cloud"
	"github.com/Ferrymania/nook/internal/service/cloud115"
	"go.uber.org/zap"
)

const testED2K = "ed2k://|file|Movie 01.mkv|123|01234567890123456789012345678901|/"
const testED2KSecond = "ed2k://|file|Movie 02.mkv|456|11234567890123456789012345678901|/"
const testED2KThird = "ed2k://|file|Movie 03.mkv|789|21234567890123456789012345678901|/"
const rejectedMagnet = "magnet:?xt=urn:btih:0123456789012345678901234567890123456789"

func TestTelegramCloudDownloadLinks(t *testing.T) {
	withRootHash := strings.TrimSuffix(testED2K, "/") + "h=SQ7LUTYUSMDBP2TVE2M7T6VUBLU324KF|/"
	withSources := testED2K + "|sources,192.0.2.1:4662|/"
	for _, tc := range []struct {
		input string
		want  []string
	}{
		{testED2K, []string{testED2K}},
		{withRootHash, []string{withRootHash}},
		{withSources, []string{withSources}},
		{"\r\n" + testED2K + "\r\n\r\n" + withSources + "\n " + testED2KSecond, []string{testED2K, testED2KSecond}},
		{"\r\n ", nil},
		{rejectedMagnet, nil},
		{testED2K + " " + testED2K, nil},
		{"https://example.org/file.mkv", nil},
		{"ed2k://|server|192.0.2.1|4661|/", nil},
		{"ed2k://|file|Movie|0|01234567890123456789012345678901|/", nil},
	} {
		got, err := cloudDownloadLinks(tc.input)
		if (err != nil) != (tc.want == nil) || !slices.Equal(got, tc.want) {
			t.Errorf("cloudDownloadLinks(%q) = %q (%v), want %q", tc.input, got, err, tc.want)
		}
	}
	if _, err := cloudDownloadLinks(testED2K + "\n" + rejectedMagnet); err == nil || !strings.Contains(err.Error(), "第 2 行") {
		t.Fatalf("expected a line-specific validation error, got %v", err)
	}
	var batch strings.Builder
	for i := range cloud115.OfflineLinkBatchLimit {
		fmt.Fprintf(&batch, "\ned2k://|file|Movie.mkv|123|%032x|/", i)
	}
	if links, err := cloudDownloadLinks(batch.String()); err != nil || len(links) != cloud115.OfflineLinkBatchLimit {
		t.Fatalf("expected maximum batch to be accepted, got %d links (%v)", len(links), err)
	}
	if _, err := cloudDownloadLinks(batch.String() + "\n" + testED2K); err == nil || !strings.Contains(err.Error(), "最多提交 115 条") {
		t.Fatalf("expected oversized batch to be rejected, got %v", err)
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

	message = telegramAdminMessage("/download@NookBot " + testED2K)
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
	if len(calls) != 2 || !strings.Contains(calls[1].Payload["text"].(string), "ED2K 文件链接格式无效") || calls[1].Payload["reply_markup"] != nil {
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
	if len(draft.Items) != 1 || !f.strm.crypto.IsEncrypted(draft.Items[0].EncryptedLink) {
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
	// A single download queued before the batch upgrade still gets submitted.
	if _, err := f.strm.repo.TelegramCloudDownload.Update(ctx, queued, map[string]any{
		"items": nil, "encrypted_link": draft.Items[0].EncryptedLink,
	}); err != nil {
		t.Fatal(err)
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
	if item := tracking.Items[0]; item.RemoteTaskID != "remote-task" || item.Progress != 35 || item.EncryptedLink != "" {
		t.Fatalf("failed to reconcile accepted 115 submission: %+v", tracking)
	}
	checkNextPoll(tracking, at, 5*time.Minute)
	poll(at.Add(time.Minute))
	if queries.Load() != 1 {
		t.Fatal("115 was queried before the next polling deadline")
	}
	at = tracking.CheckAfter
	poll(at)
	if tracking = checkTask(model.CloudDownloadTracking); tracking.Items[0].Progress != 100 {
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

func TestTelegramCloudDownloadBatchReportsMixedResults(t *testing.T) {
	f := newTelegramControlFixture(t)
	ctx := t.Context()
	const token = "test-token"
	if err := f.strm.repo.DB.AutoMigrate(&model.TelegramCloudDownload{}); err != nil {
		t.Fatal(err)
	}
	f.onCall = func(method string, _ map[string]any) any {
		if method == "sendMessage" {
			return map[string]int64{"message_id": 201}
		}
		return true
	}
	account := &model.StrmAccount{
		Name: "115", Provider: model.StrmProvider115,
		Config: `{"access_token":"test-access","cloud_download_staging_id":"456","cloud_download_staging_path":"/暂存"}`,
	}
	if err := f.strm.repo.StrmAccount.Create(ctx, account); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(time.Second).Truncate(time.Microsecond)
	var submitted, queries atomic.Int32
	remote := cloud.NewOpenAPI115("app", "test-access", "test-refresh")
	remote.OpenClient().HTTP.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body string
		switch req.URL.Path {
		case "/open/folder/get_info":
			body = `{"state":true,"data":{"file_id":"456","file_category":"0","paths":[{"file_id":"0"}]}}`
		case "/open/offline/add_task_urls":
			submitted.Add(1)
			if err := req.ParseMultipartForm(32 << 10); err != nil || req.Form.Get("urls") != strings.Join([]string{testED2K, testED2KSecond, testED2KThird}, "\n") || req.Form.Get("wp_path_id") != "456" {
				t.Errorf("unexpected batch submission: %v (%v)", req.Form, err)
			}
			// A reordered response rejects one link and omits another.
			body = fmt.Sprintf(`{"state":true,"data":[{"state":false,"url":%q,"code":123,"message":%q},{"state":true,"url":%q,"info_hash":"first-task"}]}`,
				testED2KSecond, "额度不足 "+testED2K, strings.Replace(testED2K, "Movie 01", "Movie%2001", 1))
		case "/open/offline/get_task_list":
			queries.Add(1)
			body = fmt.Sprintf(`{"state":true,"data":{"page_count":1,"tasks":[{"info_hash":"third-task","wp_path_id":"456","url":%q,"add_time":%d,"status":2},{"info_hash":"first-task","wp_path_id":"456","status":2}]}}`, testED2KThird, now.Unix())
		default:
			return nil, fmt.Errorf("unexpected 115 endpoint %s", req.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})
	f.strm.provider115Cache[account.ID] = remote

	f.bot.handleUpdate(ctx, token, telegramAdminMessage(testED2K))
	command := telegramAdminMessage("/download")
	command.Message.MessageID = 196
	f.bot.handleUpdate(ctx, token, command)
	groupMessage := telegramAdminMessage(testED2K)
	groupMessage.Message.Chat.Type = "group"
	f.bot.handleUpdate(ctx, token, groupMessage)
	invalid := telegramAdminMessage(testED2K + "\n" + rejectedMagnet)
	invalid.Message.MessageID = 197
	f.bot.handleUpdate(ctx, token, invalid)
	statusCommand := telegramAdminMessage("/start")
	statusCommand.Message.MessageID = 198
	f.bot.handleUpdate(ctx, token, statusCommand)
	message := telegramAdminMessage(strings.Join([]string{testED2K, testED2KSecond, testED2KThird, testED2K}, "\n"))
	message.Message.MessageID = 200
	f.bot.handleUpdate(ctx, token, message)
	f.bot.handleUpdate(ctx, token, message)
	calls := f.snapshot()
	if len(calls) != 4 || !strings.Contains(calls[0].Payload["text"].(string), "5 分钟内发送") ||
		!strings.Contains(calls[1].Payload["text"].(string), "第 2 行") ||
		!strings.Contains(calls[2].Payload["text"].(string), "Nook 管理员操作") ||
		!strings.Contains(calls[3].Payload["text"].(string), "3 个文件") {
		t.Fatalf("expected a prompt, validation error, regular command response, and one confirmation: %+v", calls)
	}
	keyboard := calls[3].Payload["reply_markup"].(map[string]any)["inline_keyboard"].([]any)
	callback := keyboard[0].([]any)[0].(map[string]any)["callback_data"].(string)
	click := telegramAdminCallback(callback)
	click.CallbackQuery.Message.MessageID = 201
	f.bot.handleUpdate(ctx, token, click)
	if submitted.Load() != 0 {
		t.Fatal("submission should be handled by the background worker")
	}
	id := strings.Split(callback, ":")[2]
	f.bot.runCloudDownloads(ctx, token, testTelegramAdminID, now)
	task, err := f.strm.repo.TelegramCloudDownload.Find(ctx, id)
	if err != nil || task == nil || len(task.Items) != 3 || !task.CheckAfter.Equal(now.Add(time.Minute)) {
		t.Fatalf("expected three items and the first poll in a minute: %+v (%v)", task, err)
	}
	if task.Items[0].RemoteTaskID != "first-task" || task.Items[1].Status != model.CloudDownloadFailed ||
		task.Items[2].Status != model.CloudDownloadSubmitting || !f.strm.crypto.IsEncrypted(task.Items[2].EncryptedLink) {
		t.Fatalf("unexpected statuses after submission: %+v", task.Items)
	}
	f.bot.runCloudDownloads(ctx, token, testTelegramAdminID, task.CheckAfter)
	task, err = f.strm.repo.TelegramCloudDownload.Find(ctx, id)
	if err != nil || task == nil || task.Status != model.CloudDownloadDone || !task.Reported {
		t.Fatalf("expected a reported terminal batch: %+v (%v)", task, err)
	}
	if submitted.Load() != 1 || queries.Load() != 1 || task.Items[2].RemoteTaskID != "third-task" || task.Items[2].EncryptedLink != "" {
		t.Fatalf("expected one submission and shared status query: %+v, requests=%d/%d", task.Items, submitted.Load(), queries.Load())
	}
	last := f.snapshot()
	report := last[len(last)-1]
	reportText, _ := report.Payload["text"].(string)
	if report.Method != "editMessageText" || report.Payload["message_id"] != float64(201) ||
		!strings.Contains(reportText, "已完成 2 · 处理中 0 · 失败 1") || !strings.Contains(reportText, "额度不足") || strings.Contains(reportText, "ed2k://") {
		t.Fatalf("unexpected result report: %+v", report)
	}
}

func TestTelegramCloudDownloadInputExpires(t *testing.T) {
	f := newTelegramControlFixture(t)
	ctx := t.Context()
	const token = "test-token"
	f.bot.handleUpdate(ctx, token, telegramAdminMessage("/download@NookBot"))
	f.bot.mu.Lock()
	f.bot.cloudDownloadInput.expiresAt = time.Now().Add(-time.Minute)
	f.bot.mu.Unlock()
	f.bot.handleUpdate(ctx, token, telegramAdminMessage(testED2K))
	f.bot.handleUpdate(ctx, token, telegramAdminMessage(testED2K))
	calls := f.snapshot()
	if len(calls) != 2 || !strings.Contains(calls[1].Payload["text"].(string), "等待链接已超时") || calls[1].Payload["reply_markup"] != nil {
		t.Fatalf("expired prompt should request a fresh command once: %+v", calls)
	}
}

func TestTelegramCloudDownloadResultPages(t *testing.T) {
	f := newTelegramControlFixture(t)
	ctx := t.Context()
	const token = "test-token"
	if err := f.strm.repo.DB.AutoMigrate(&model.TelegramCloudDownload{}); err != nil {
		t.Fatal(err)
	}
	task := &model.TelegramCloudDownload{
		Base: model.Base{ID: "test"}, BotKey: cloudDownloadBotKey(token), ChatID: testTelegramAdminID,
		MessageID: 201, Status: model.CloudDownloadDone, Reported: true, CheckAfter: time.Now().Add(time.Hour).Truncate(time.Microsecond),
	}
	for i := range cloudDownloadResultPageSize + 1 {
		task.Items = append(task.Items, model.TelegramCloudDownloadItem{Name: fmt.Sprintf("File <%d>", i+1), Status: model.CloudDownloadDone})
	}
	first, keyboard := cloudDownloadMessage(task)
	if !strings.Contains(first, "第 1/2 页") || strings.Contains(first, "File &lt;9&gt;") || len(keyboard) != 1 || len(keyboard[0]) != 1 || keyboard[0][0].Data != "nook:dlresult:test:1" {
		t.Fatalf("unexpected first page: %s, keyboard=%+v", first, keyboard)
	}
	if inserted, err := f.strm.repo.TelegramCloudDownload.Create(ctx, task); err != nil || !inserted {
		t.Fatalf("create batch: %v (%v)", inserted, err)
	}
	click := telegramAdminCallback(keyboard[0][0].Data)
	click.CallbackQuery.Message.MessageID = task.MessageID
	f.bot.handleUpdate(ctx, token, click)
	latest, err := f.strm.repo.TelegramCloudDownload.Find(ctx, task.ID)
	if err != nil || latest == nil || latest.DetailPage != 1 || !latest.Reported || !latest.CheckAfter.Equal(task.CheckAfter) {
		t.Fatalf("navigation should remember the page without changing the schedule: %+v (%v)", latest, err)
	}
	calls := f.snapshot()
	last := calls[len(calls)-1]
	second, _ := last.Payload["text"].(string)
	if last.Method != "editMessageText" || !strings.Contains(second, "第 2/2 页") || !strings.Contains(second, "File &lt;9&gt;") {
		t.Fatalf("unexpected second page: %+v", last)
	}
}
