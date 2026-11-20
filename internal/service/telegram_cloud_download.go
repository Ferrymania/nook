package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Ferrymania/nook/internal/model"
)

const (
	cloudDownloadDraftLifetime = 15 * time.Minute
	cloudDownloadWorkerTick    = 30 * time.Second
)

func cloudDownloadBotKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func cloudDownloadLink(text string) string {
	text = strings.TrimSpace(text)
	pos := strings.IndexFunc(text, unicode.IsSpace)
	if pos < 0 {
		return ""
	}
	link := strings.TrimSpace(text[pos:])
	if !validCloudDownloadED2K(link) {
		return ""
	}
	return link
}

func validCloudDownloadED2K(link string) bool {
	if len(link) > 4096 || strings.IndexFunc(link, unicode.IsControl) >= 0 || !strings.HasSuffix(link, "|/") {
		return false
	}
	parts := strings.Split(link, "|")
	if len(parts) < 6 || !strings.EqualFold(parts[0], "ed2k://") || parts[1] != "file" || strings.TrimSpace(parts[2]) == "" {
		return false
	}
	size, sizeErr := strconv.ParseUint(parts[3], 10, 64)
	hash, hashErr := hex.DecodeString(parts[4])
	if sizeErr != nil || size == 0 || hashErr != nil || len(hash) != 16 {
		return false
	}
	options := parts[5 : len(parts)-1]
	// Peer sources use a separate section after the file link terminator.
	if n := len(options); n >= 2 && options[n-2] == "/" && strings.HasPrefix(options[n-1], "sources,") {
		if options[n-1] == "sources," || strings.IndexFunc(options[n-1], unicode.IsSpace) >= 0 {
			return false
		}
		options = options[:n-2]
	}
	for _, option := range options {
		name, value, ok := strings.Cut(option, "=")
		if !ok || value == "" || strings.IndexFunc(value, unicode.IsSpace) >= 0 {
			return false
		}
		switch name {
		case "h", "p", "s":
		default:
			return false
		}
	}
	return true
}

type cloudDownloadAccount struct {
	id        string
	label     string
	stagingID string
}

func (s *TelegramService) cloudDownloadAccounts(ctx context.Context) ([]cloudDownloadAccount, error) {
	accounts, err := s.strm.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	var available []cloudDownloadAccount
	for _, account := range accounts {
		if !account.Enabled || account.Provider != model.StrmProvider115 || !HasStrmAccountCredential(&account) {
			continue
		}
		preview := s.strm.StrmAccountConfigPreviewOf(&account)
		if preview.CloudDownloadStagingID == "0" || !valid115ID(preview.CloudDownloadStagingID) {
			continue
		}
		directory := firstNonEmpty(preview.CloudDownloadStagingPath, "目录 ID "+preview.CloudDownloadStagingID)
		label := firstNonEmpty(account.Name, "115 网盘") + " · " + directory
		available = append(available, cloudDownloadAccount{id: account.ID, label: label, stagingID: preview.CloudDownloadStagingID})
	}
	return available, nil
}

func cloudDownloadKeyboard(id string, accounts []cloudDownloadAccount, page int) [][]telegramButton {
	start := min(page*telegramPathPageSize, len(accounts))
	end := min(start+telegramPathPageSize, len(accounts))
	var keyboard [][]telegramButton
	for _, account := range accounts[start:end] {
		keyboard = append(keyboard, []telegramButton{{
			Text: clampTelegramRunes(account.label, 80),
			Data: "nook:dl:" + id + ":" + account.id,
		}})
	}
	var navigation []telegramButton
	if page > 0 {
		navigation = append(navigation, telegramButton{Text: "上一页", Data: fmt.Sprintf("nook:dlpage:%s:%d", id, page-1)})
	}
	if end < len(accounts) {
		navigation = append(navigation, telegramButton{Text: "下一页", Data: fmt.Sprintf("nook:dlpage:%s:%d", id, page+1)})
	}
	if len(navigation) > 0 {
		keyboard = append(keyboard, navigation)
	}
	return keyboard
}

const cloudDownloadDraftText = "☁️ 115 云下载\n\n请选择 115 账号。文件会保存到所选账号的暂存目录。"

func (s *TelegramService) createCloudDownloadDraft(ctx context.Context, token string, message *telegramMessage) {
	chatID := strconv.FormatInt(message.Chat.ID, 10)
	link := cloudDownloadLink(message.Text)
	if link == "" {
		s.controlReply(ctx, token, chatID, "云下载只接受 ED2K 单文件链接。请发送 <code>/download &lt;ED2K 链接&gt;</code>，每次一条。", nil)
		return
	}
	accounts, err := s.cloudDownloadAccounts(ctx)
	if err != nil {
		s.controlReply(ctx, token, chatID, "读取 115 账号失败，请稍后重试。", nil)
		return
	}
	if len(accounts) == 0 {
		s.controlReply(ctx, token, chatID, "没有配置暂存目录的可用 115 账号。请先授权账号并设置云下载暂存目录。", nil)
		return
	}
	encrypted := ""
	if s.strm.crypto != nil {
		encrypted = s.strm.crypto.Encrypt(link)
	}
	if encrypted == "" || !s.strm.crypto.IsEncrypted(encrypted) {
		s.controlReply(ctx, token, chatID, "加密服务暂不可用，无法保存下载任务。", nil)
		return
	}
	key := cloudDownloadBotKey(token)
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d", key, message.Chat.ID, message.MessageID)))
	task := &model.TelegramCloudDownload{
		Base:   model.Base{ID: hex.EncodeToString(sum[:8])},
		BotKey: key, ChatID: message.Chat.ID, Status: model.CloudDownloadDraft, EncryptedLink: encrypted,
	}
	inserted, err := s.repo.TelegramCloudDownload.Create(ctx, task)
	if err != nil {
		s.warn("save cloud download draft failed", "admin", err)
		s.controlReply(ctx, token, chatID, "保存下载任务失败，请稍后重试。", nil)
		return
	}
	if !inserted || !s.adminControlAllowed(ctx, token, message.Chat, message.From.ID) {
		return
	}
	var result telegramMessage
	err = s.call(ctx, token, "sendMessage", map[string]any{
		"chat_id": message.Chat.ID, "text": cloudDownloadDraftText,
		"reply_markup": map[string]any{"inline_keyboard": cloudDownloadKeyboard(task.ID, accounts, 0)},
	}, &result)
	if err != nil || result.MessageID == 0 {
		if err != nil {
			s.warn("send cloud download draft failed", "admin", err)
		}
		return
	}
	if _, err := s.repo.TelegramCloudDownload.Update(ctx, task, map[string]any{"message_id": result.MessageID}); err != nil {
		s.warn("save cloud download message ID failed", "admin", err)
	}
}

func (s *TelegramService) handleCloudDownloadCallback(ctx context.Context, token string, message *telegramMessage, parts []string) {
	if len(parts) != 4 {
		return
	}
	task, err := s.repo.TelegramCloudDownload.Find(ctx, parts[2])
	if err != nil || task == nil || message.MessageID == 0 || task.MessageID != message.MessageID ||
		task.ChatID != message.Chat.ID || task.BotKey != cloudDownloadBotKey(token) {
		return
	}
	if task.Status != model.CloudDownloadDraft {
		return
	}
	if time.Since(task.CreatedAt) > cloudDownloadDraftLifetime {
		_ = s.editCloudDownloadMessage(ctx, token, task, "此下载确认已过期，请重新发送 /download。", nil)
		return
	}
	accounts, err := s.cloudDownloadAccounts(ctx)
	if err != nil {
		s.controlReply(ctx, token, strconv.FormatInt(task.ChatID, 10), "读取 115 账号失败，请稍后重试。", nil)
		return
	}
	if parts[1] == "dlpage" {
		page, err := strconv.Atoi(parts[3])
		pageCount := (len(accounts) + telegramPathPageSize - 1) / telegramPathPageSize
		if err == nil && page >= 0 && page < pageCount {
			_ = s.editCloudDownloadMessage(ctx, token, task, cloudDownloadDraftText, cloudDownloadKeyboard(task.ID, accounts, page))
		}
		return
	}
	for _, account := range accounts {
		if account.id == parts[3] {
			s.queueCloudDownload(ctx, token, task, account)
			return
		}
	}
	s.controlReply(ctx, token, strconv.FormatInt(task.ChatID, 10), "此账号或暂存目录已不可用，请重新发送 /download。", nil)
}

func (s *TelegramService) queueCloudDownload(ctx context.Context, token string, task *model.TelegramCloudDownload, account cloudDownloadAccount) {
	changed, err := s.repo.TelegramCloudDownload.Update(ctx, task, map[string]any{
		"status": model.CloudDownloadQueued, "account_id": account.id, "staging_folder_id": account.stagingID,
		"directory": account.label, "check_after": time.Now(), "reported": false,
	})
	if err != nil {
		s.warn("queue cloud download failed", "admin", err)
		s.controlReply(ctx, token, strconv.FormatInt(task.ChatID, 10), "保存操作失败，请稍后重试。", nil)
		return
	}
	if changed {
		select {
		case s.cloudDownloadWake <- struct{}{}:
		default:
		}
	}
}

func (s *TelegramService) editCloudDownloadMessage(ctx context.Context, token string, task *model.TelegramCloudDownload, text string, keyboard [][]telegramButton) error {
	if keyboard == nil {
		keyboard = [][]telegramButton{}
	}
	return s.call(ctx, token, "editMessageText", map[string]any{
		"chat_id": task.ChatID, "message_id": task.MessageID, "parse_mode": "HTML", "text": text,
		"reply_markup": map[string]any{"inline_keyboard": keyboard},
	}, nil)
}

func clampTelegramRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit-1]) + "…"
}

func cloudDownloadMessage(task *model.TelegramCloudDownload) string {
	var status string
	switch task.Status {
	case model.CloudDownloadQueued:
		status = "准备提交"
	case model.CloudDownloadSubmitting:
		status = "正在确认 115 是否已接收任务"
	case model.CloudDownloadTracking:
		status = "已提交，等待 115 下载"
		if task.Progress >= 0 {
			status = fmt.Sprintf("115 下载中 · %d%%", task.Progress)
		}
	case model.CloudDownloadDone:
		status = "下载完成，文件已存入暂存目录"
	case model.CloudDownloadFailed:
		status = "115 云下载失败"
	}
	text := "☁️ <b>115 云下载</b>\n\n暂存目录：" + html.EscapeString(clampTelegramRunes(task.Directory, 200)) + "\n状态：" + status
	if task.Detail != "" {
		text += "\n" + html.EscapeString(clampTelegramRunes(task.Detail, 320))
	}
	return text
}
