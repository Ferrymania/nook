package service

import (
	"context"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/Ferrymania/nook/internal/helper"
	"github.com/Ferrymania/nook/internal/model"
)

type telegramSyncRun struct {
	cancel        context.CancelFunc
	currentPathID string
	remaining     int
}

func (s *TelegramService) startTelegramSync(ctx context.Context, token, chatID, pathID string) {
	paths, err := s.strm.ListSyncPaths(ctx)
	if err != nil {
		s.controlReply(ctx, token, chatID, "读取同步目录失败，请稍后重试", nil)
		return
	}
	var selected []model.StrmSyncPath
	for _, p := range paths {
		if p.Enabled && (pathID == "all" || pathID == p.ID) {
			selected = append(selected, p)
		}
	}
	if len(selected) == 0 {
		s.controlReply(ctx, token, chatID, "目录不存在或已禁用，请重新发送 /sync", nil)
		return
	}
	s.mu.Lock()
	if s.syncRun != nil {
		s.mu.Unlock()
		s.controlReply(ctx, token, chatID, "已有 Telegram 同步任务，请用 /status 查看或 /cancel 取消。", nil)
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	run := &telegramSyncRun{cancel: cancel, remaining: len(selected)}
	s.syncRun = run
	s.mu.Unlock()
	helper.Go(s.log, "telegram.strmSync", func() { s.runTelegramSync(runCtx, token, chatID, run, selected) })
}

func (s *TelegramService) runTelegramSync(ctx context.Context, token, chatID string, run *telegramSyncRun, paths []model.StrmSyncPath) {
	defer func() {
		run.cancel()
		s.mu.Lock()
		if s.syncRun == run {
			s.syncRun = nil
		}
		s.mu.Unlock()
	}()
	started := time.Now()
	s.controlReply(ctx, token, chatID, fmt.Sprintf("🔄 开始执行增量 STRM 同步，共 %d 个目录，依次执行。", len(paths)), nil)
	var done, failed, canceled int
	for _, p := range paths {
		// Use the same lock as cancellation: /cancel cannot miss a just-started
		// run or accidentally cancel a later run of this directory.
		s.mu.Lock()
		if ctx.Err() != nil {
			s.mu.Unlock()
			break
		}
		adminID := telegramPrivateAdminID(chatID)
		if !s.adminControlAllowed(ctx, token, telegramChat{ID: adminID, Type: "private"}, adminID) {
			s.mu.Unlock()
			break
		}
		result, err := s.strm.StartSyncWithResult(ctx, p.ID, model.StrmSyncTypeIncremental)
		if err == nil {
			run.currentPathID = p.ID
		}
		run.remaining--
		s.mu.Unlock()
		if err != nil {
			failed++
			s.controlReply(ctx, token, chatID, "⚠️ "+html.EscapeString(telegramPathName(p))+" 未启动："+html.EscapeString(redactSensitiveText(err.Error())), nil)
			continue
		}
		// The engine ties this run's cancellation to ctx. Wait for its own
		// final record before starting another directory or releasing the batch.
		rec := <-result
		s.mu.Lock()
		run.currentPathID = ""
		s.mu.Unlock()
		switch rec.Status {
		case model.StrmSyncRecordDone:
			done++
		case model.StrmSyncRecordCanceled:
			canceled++
		default:
			failed++
		}
		// A canceled batch still reports its final outcome.
		replyCtx := context.WithoutCancel(ctx)
		s.controlReply(replyCtx, token, chatID, telegramSyncResult(p, rec), nil)
	}
	s.mu.Lock()
	pending := run.remaining
	s.mu.Unlock()
	label := "✅ 增量 STRM 同步完成"
	if ctx.Err() != nil || pending > 0 {
		label = "⏹ 增量 STRM 同步已取消"
	} else if failed > 0 || canceled > 0 {
		label = "⚠️ 增量 STRM 同步结束"
	}
	text := fmt.Sprintf("%s\n成功 %d，失败/未启动 %d，取消 %d，未执行 %d\n耗时：%s", label, done, failed, canceled, pending, time.Since(started).Round(time.Second))
	s.controlReply(context.WithoutCancel(ctx), token, chatID, text, nil)
}

func telegramSyncResult(p model.StrmSyncPath, rec model.StrmSyncRecord) string {
	label := "✅ 同步完成"
	if rec.Status == model.StrmSyncRecordCanceled {
		label = "⏹ 同步已取消"
	} else if rec.Status != model.StrmSyncRecordDone {
		label = "❌ 同步失败"
	}
	duration := time.Duration(0)
	if rec.StartedAt != nil && rec.FinishedAt != nil {
		duration = rec.FinishedAt.Sub(*rec.StartedAt).Round(time.Second)
	}
	text := fmt.Sprintf("%s：%s\n耗时：%s\n新增/更新 STRM：%d，跳过：%d，清理：%d\n元数据下载入队：%d，上传入队：%d", label, html.EscapeString(telegramPathName(p)), duration, rec.NewStrm, rec.Skipped, rec.Pruned, rec.NewMeta, rec.Uploaded)
	if rec.Message != "" {
		text += "\n原因：" + html.EscapeString(redactSensitiveText(rec.Message))
	}
	return text
}

func (s *TelegramService) cancelTelegramSync(ctx context.Context, token, chatID, pathID string) {
	s.mu.Lock()
	if pathID == "batch" {
		if s.syncRun == nil {
			s.mu.Unlock()
			s.controlReply(ctx, token, chatID, "本次同步已结束", nil)
			return
		}
		s.syncRun.cancel()
		s.mu.Unlock()
		s.controlReply(ctx, token, chatID, "已请求取消本次同步，正在等待当前目录停止；未开始的目录不会继续执行。", nil)
		return
	}
	var err error
	running := pathID != "" && s.strm.IsSyncRunning(pathID)
	if running {
		err = s.strm.CancelSync(ctx, pathID)
	}
	s.mu.Unlock()
	text := "已请求取消该目录的同步，正在等待任务停止。"
	if err != nil {
		text = "取消失败，请稍后重试"
	} else if !running && pathID != "" {
		text = "该目录的同步已结束"
	}
	s.controlReply(ctx, token, chatID, text, nil)
}

func (s *TelegramService) sendSyncStatus(ctx context.Context, token, chatID string) {
	paths, err := s.strm.ListSyncPaths(ctx)
	if err != nil {
		s.controlReply(ctx, token, chatID, "读取同步状态失败，请稍后重试", nil)
		return
	}
	lines := []string{"STRM 同步状态"}
	s.mu.Lock()
	if s.syncRun != nil {
		lines = append(lines, fmt.Sprintf("Telegram 同步进行中，待执行 %d 个目录", s.syncRun.remaining))
	}
	s.mu.Unlock()
	if len(paths) == 0 {
		lines = append(lines, "暂无同步目录")
	}
	for _, p := range paths {
		state := "未同步"
		switch p.LastSyncStatus {
		case model.StrmSyncRecordDone, "ok":
			state = "已完成"
		case model.StrmSyncRecordFailed, "error":
			state = "失败"
		case model.StrmSyncRecordCanceled:
			state = "已取消"
		}
		if s.strm.IsSyncRunning(p.ID) {
			state = "同步中"
		} else if !p.Enabled {
			state = "已禁用"
		}
		line := html.EscapeString(telegramPathName(p)) + "：" + state
		if p.LastSyncAt != nil {
			line += "（" + p.LastSyncAt.Format("01-02 15:04") + "）"
		}
		if p.LastSyncMessage != "" {
			line += "\n" + html.EscapeString(redactSensitiveText(p.LastSyncMessage))
		}
		lines = append(lines, line)
	}
	// Keep each Telegram message within its text limit without splitting HTML.
	text := ""
	for _, line := range lines {
		if len(text)+len(line) > 3500 && text != "" {
			s.controlReply(ctx, token, chatID, text, nil)
			text = ""
		}
		text += line + "\n"
	}
	if strings.TrimSpace(text) != "" {
		s.controlReply(ctx, token, chatID, text, nil)
	}
}
