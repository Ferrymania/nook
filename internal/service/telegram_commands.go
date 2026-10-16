package service

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/Ferrymania/nook/internal/model"
)

const telegramPathPageSize = 8

type telegramButton struct {
	Text string `json:"text"`
	Data string `json:"callback_data"`
}

func (s *TelegramService) SetStrmService(strm *StrmService) { s.strm = strm }

func telegramCommand(text string) string {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return ""
	}
	command, _, _ := strings.Cut(strings.ToLower(fields[0]), "@")
	return command
}

func isTelegramControlCommand(text string) bool {
	switch telegramCommand(text) {
	case "/strm_inc", "/sync_all", "/sync", "/status", "/cancel":
		return true
	}
	return false
}

func telegramPrivateAdminID(raw string) int64 {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

func (s *TelegramService) adminControlAllowed(ctx context.Context, token string, chat telegramChat, senderID int64) bool {
	cfg := s.config(ctx)
	id := telegramPrivateAdminID(cfg.AdminID)
	return cfg.Enabled && cfg.BotToken != "" && cfg.BotToken == token && id > 0 &&
		chat.Type == "private" && chat.ID == id && senderID == id
}

// Only the configured private admin chat gets this menu. Group notification
// destinations remain supported, but cannot grant sync control to group members.
func (s *TelegramService) registerAdminCommands(ctx context.Context, cfg telegramConfig, oldToken, oldAdmin string) error {
	if oldToken != "" && telegramPrivateAdminID(oldAdmin) > 0 && (oldToken != cfg.BotToken || oldAdmin != cfg.AdminID) {
		if err := s.call(ctx, oldToken, "deleteMyCommands", map[string]any{
			"scope": map[string]any{"type": "chat", "chat_id": telegramPrivateAdminID(oldAdmin)},
		}, nil); err != nil {
			// Revoked tokens must not prevent registration for the new bot.
			s.warn("telegram old command menu cleanup failed", "admin", err)
		}
	}
	id := telegramPrivateAdminID(cfg.AdminID)
	if !cfg.Enabled || cfg.BotToken == "" || id == 0 || s.strm == nil {
		return nil
	}
	return s.call(ctx, cfg.BotToken, "setMyCommands", map[string]any{
		"scope": map[string]any{"type": "chat", "chat_id": id},
		"commands": []map[string]string{
			{"command": "start", "description": "查看 Nook 操作说明"},
			{"command": "strm_inc", "description": "全部启用目录依次增量同步"},
			{"command": "sync", "description": "选择目录增量同步"},
			{"command": "status", "description": "查看同步状态和最近结果"},
			{"command": "cancel", "description": "选择并取消同步任务"},
		},
	}, nil)
}

func (s *TelegramService) handleAdminCommand(ctx context.Context, token, chatID, command string) {
	if s.strm == nil {
		s.controlReply(ctx, token, chatID, "STRM 同步服务暂不可用", nil)
		return
	}
	switch command {
	case "/strm_inc", "/sync_all":
		s.startTelegramSync(ctx, token, chatID, "all")
	case "/sync":
		s.sendPathMenu(ctx, token, chatID, "sync", 0)
	case "/status":
		s.sendSyncStatus(ctx, token, chatID)
	case "/cancel":
		s.sendPathMenu(ctx, token, chatID, "cancel", 0)
	default:
		s.controlReply(ctx, token, chatID, "Nook 管理员操作\n\n/strm_inc — 全部目录依次增量同步\n/sync — 选择目录增量同步\n/status — 查看同步状态\n/cancel — 选择取消任务\n\n使用系统设置中的 Bot 和管理员私聊 ID，无需个人资料绑定。", nil)
	}
}

func (s *TelegramService) handleAdminCallback(ctx context.Context, token string, q *telegramCallbackQuery) {
	allowed := q.Message != nil && s.adminControlAllowed(ctx, token, q.Message.Chat, q.From.ID)
	answer := map[string]any{"callback_query_id": q.ID}
	if !allowed {
		answer["text"] = "仅系统设置中的管理员可在私聊中操作"
	}
	if err := s.call(ctx, token, "answerCallbackQuery", answer, nil); err != nil {
		s.warn("telegram callback acknowledgement failed", "admin", err)
	}
	if !allowed || s.strm == nil {
		return
	}
	chatID := strconv.FormatInt(q.Message.Chat.ID, 10)
	parts := strings.Split(q.Data, ":")
	if len(parts) < 3 || parts[0] != "nook" {
		return
	}
	switch parts[1] {
	case "sync":
		if len(parts) == 3 {
			s.startTelegramSync(ctx, token, chatID, parts[2])
		}
	case "cancel":
		if len(parts) == 3 {
			s.cancelTelegramSync(ctx, token, chatID, parts[2])
		}
	case "page":
		if len(parts) != 4 || (parts[2] != "sync" && parts[2] != "cancel") {
			return
		}
		page, err := strconv.Atoi(parts[3])
		if err == nil && page >= 0 {
			s.sendPathMenu(ctx, token, chatID, parts[2], page)
		}
	}
}

// Replies re-check configuration so a changed admin ID or token immediately
// revokes an old chat's access, including results of a long-running batch.
func (s *TelegramService) controlReply(ctx context.Context, token, chatID, text string, keyboard [][]telegramButton) {
	id := telegramPrivateAdminID(chatID)
	if !s.adminControlAllowed(ctx, token, telegramChat{ID: id, Type: "private"}, id) {
		return
	}
	payload := map[string]any{"chat_id": chatID, "text": text, "parse_mode": "HTML"}
	if len(keyboard) > 0 {
		payload["reply_markup"] = map[string]any{"inline_keyboard": keyboard}
	}
	if err := s.call(ctx, token, "sendMessage", payload, nil); err != nil {
		s.warn("telegram control reply failed", "admin", err)
	}
}

func telegramPathName(p model.StrmSyncPath) string {
	if name := strings.TrimSpace(p.Name); name != "" {
		return name
	}
	return "未命名目录"
}

func (s *TelegramService) sendPathMenu(ctx context.Context, token, chatID, mode string, page int) {
	paths, err := s.strm.ListSyncPaths(ctx)
	if err != nil {
		s.controlReply(ctx, token, chatID, "读取同步目录失败，请稍后重试", nil)
		return
	}
	var selected []model.StrmSyncPath
	for _, p := range paths {
		if (mode == "sync" && p.Enabled) || (mode == "cancel" && s.strm.IsSyncRunning(p.ID)) {
			selected = append(selected, p)
		}
	}
	var rows [][]telegramButton
	title := "选择要增量同步的目录："
	if mode == "cancel" {
		title = "选择要取消的同步任务："
		s.mu.Lock()
		hasBatch := s.syncRun != nil
		s.mu.Unlock()
		if hasBatch {
			rows = append(rows, []telegramButton{{Text: "取消本次同步（含未开始目录）", Data: "nook:cancel:batch"}})
		}
	}
	if len(selected) == 0 && len(rows) == 0 {
		text := "暂无已启用的同步目录"
		if mode == "cancel" {
			text = "暂无运行中的同步任务"
		}
		s.controlReply(ctx, token, chatID, text, nil)
		return
	}
	pages := (len(selected) + telegramPathPageSize - 1) / telegramPathPageSize
	if page >= pages {
		page = 0
	}
	for i := page * telegramPathPageSize; i < min(len(selected), (page+1)*telegramPathPageSize); i++ {
		p := selected[i]
		rows = append(rows, []telegramButton{{Text: telegramPathName(p), Data: "nook:" + mode + ":" + p.ID}})
	}
	var navigation []telegramButton
	if page > 0 {
		navigation = append(navigation, telegramButton{Text: "上一页", Data: fmt.Sprintf("nook:page:%s:%d", mode, page-1)})
	}
	if page+1 < pages {
		navigation = append(navigation, telegramButton{Text: "下一页", Data: fmt.Sprintf("nook:page:%s:%d", mode, page+1)})
	}
	if len(navigation) > 0 {
		rows = append(rows, navigation)
	}
	if mode == "sync" {
		rows = append(rows, []telegramButton{{Text: "全部目录增量同步", Data: "nook:sync:all"}})
	}
	s.controlReply(ctx, token, chatID, html.EscapeString(title), rows)
}
