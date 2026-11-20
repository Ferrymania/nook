package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/service/cloud115"
)

const (
	cloudDownloadInputLifetime  = 5 * time.Minute
	cloudDownloadDraftLifetime  = 15 * time.Minute
	cloudDownloadWorkerTick     = 30 * time.Second
	cloudDownloadResultPageSize = 8
)

type cloudDownloadInput struct {
	botKey    string
	chatID    int64
	expiresAt time.Time
}

func cloudDownloadBotKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func cloudDownloadLinks(text string) ([]string, error) {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	var links []string
	seen := make(map[string]bool)
	for i, line := range lines {
		link := strings.TrimSpace(line)
		if link == "" {
			continue
		}
		if !validCloudDownloadED2K(link) {
			return nil, fmt.Errorf("第 %d 行的 ED2K 文件链接格式无效。请每行填写一条链接。", i+1)
		}
		key := cloudDownloadLinkKey(link)
		if !seen[key] {
			seen[key] = true
			links = append(links, link)
		}
	}
	if len(links) == 0 {
		return nil, errors.New("请发送 ED2K 文件链接，每行一条。")
	}
	if len(links) > cloud115.OfflineLinkBatchLimit {
		return nil, fmt.Errorf("每批最多提交 %d 条 ED2K 链接，请分批发送。", cloud115.OfflineLinkBatchLimit)
	}
	return links, nil
}

// Only call with a validated ED2K file link.
func cloudDownloadLinkKey(link string) string {
	parts := strings.SplitN(link, "|", 6)
	return strings.ToLower(parts[4])
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

func cloudDownloadDraftText(count int) string {
	return fmt.Sprintf("☁️ 115 云下载 · %d 个文件\n\n请选择 115 账号。文件会保存到所选账号的暂存目录，同一文件只提交一次。", count)
}

func (s *TelegramService) handleCloudDownloadCommand(ctx context.Context, token string, message *telegramMessage, text string) {
	if pos := strings.IndexFunc(text, unicode.IsSpace); pos >= 0 {
		s.handleCloudDownloadLinks(ctx, token, message, text[pos:])
		return
	}
	s.mu.Lock()
	s.cloudDownloadInput = cloudDownloadInput{
		botKey: cloudDownloadBotKey(token), chatID: message.Chat.ID, expiresAt: time.Now().Add(cloudDownloadInputLifetime),
	}
	s.mu.Unlock()
	s.controlReply(ctx, token, strconv.FormatInt(message.Chat.ID, 10), "请在 5 分钟内发送 ED2K 文件链接，每行一条。", nil)
}

func (s *TelegramService) handleCloudDownloadFollowUp(ctx context.Context, token string, message *telegramMessage, text string) {
	s.mu.Lock()
	input := s.cloudDownloadInput
	s.mu.Unlock()
	if input.chatID != message.Chat.ID || input.botKey != cloudDownloadBotKey(token) ||
		!s.adminControlAllowed(ctx, token, message.Chat, message.From.ID) {
		return
	}
	if time.Now().Before(input.expiresAt) {
		s.handleCloudDownloadLinks(ctx, token, message, text)
		return
	}
	s.mu.Lock()
	if s.cloudDownloadInput == input {
		s.cloudDownloadInput = cloudDownloadInput{}
	}
	s.mu.Unlock()
	if strings.HasPrefix(strings.ToLower(text), "ed2k://") {
		s.controlReply(ctx, token, strconv.FormatInt(message.Chat.ID, 10), "等待链接已超时，请重新发送 /download。", nil)
	}
}

func (s *TelegramService) handleCloudDownloadLinks(ctx context.Context, token string, message *telegramMessage, text string) {
	links, err := cloudDownloadLinks(text)
	if err != nil {
		s.controlReply(ctx, token, strconv.FormatInt(message.Chat.ID, 10), html.EscapeString(err.Error()), nil)
		return
	}
	s.mu.Lock()
	if s.cloudDownloadInput.chatID == message.Chat.ID && s.cloudDownloadInput.botKey == cloudDownloadBotKey(token) {
		s.cloudDownloadInput = cloudDownloadInput{}
	}
	s.mu.Unlock()
	s.createCloudDownloadDraft(ctx, token, message, links)
}

func (s *TelegramService) createCloudDownloadDraft(ctx context.Context, token string, message *telegramMessage, links []string) {
	chatID := strconv.FormatInt(message.Chat.ID, 10)
	accounts, err := s.cloudDownloadAccounts(ctx)
	if err != nil {
		s.controlReply(ctx, token, chatID, "读取 115 账号失败，请稍后重试。", nil)
		return
	}
	if len(accounts) == 0 {
		s.controlReply(ctx, token, chatID, "没有配置暂存目录的可用 115 账号。请先授权账号并设置云下载暂存目录。", nil)
		return
	}
	if s.strm.crypto == nil {
		s.controlReply(ctx, token, chatID, "加密服务暂不可用，无法保存下载任务。", nil)
		return
	}
	items := make([]model.TelegramCloudDownloadItem, 0, len(links))
	for _, link := range links {
		encrypted := s.strm.crypto.Encrypt(link)
		if !s.strm.crypto.IsEncrypted(encrypted) {
			s.controlReply(ctx, token, chatID, "加密服务暂不可用，无法保存下载任务。", nil)
			return
		}
		items = append(items, model.TelegramCloudDownloadItem{
			Name: strings.SplitN(link, "|", 4)[2], EncryptedLink: encrypted, Status: model.CloudDownloadQueued, Progress: -1,
		})
	}
	key := cloudDownloadBotKey(token)
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d", key, message.Chat.ID, message.MessageID)))
	task := &model.TelegramCloudDownload{
		Base:   model.Base{ID: hex.EncodeToString(sum[:8])},
		BotKey: key, ChatID: message.Chat.ID, Status: model.CloudDownloadDraft, Items: items,
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
		"chat_id": message.Chat.ID, "text": cloudDownloadDraftText(len(items)),
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
	if parts[1] == "dlresult" {
		page, err := strconv.Atoi(parts[3])
		if err != nil || task.Status == model.CloudDownloadDraft || page < 0 || page >= cloudDownloadResultPages(len(task.Items)) {
			return
		}
		if err := s.repo.TelegramCloudDownload.SetResultPage(ctx, task.ID, page); err != nil {
			s.warn("save cloud download result page failed", "admin", err)
			return
		}
		s.reportCloudDownload(ctx, token, task.ID, time.Now())
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
			_ = s.editCloudDownloadMessage(ctx, token, task, cloudDownloadDraftText(len(cloudDownloadItems(task))), cloudDownloadKeyboard(task.ID, accounts, page))
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

// Records created before batch support have a single link stored on the task.
func cloudDownloadItems(task *model.TelegramCloudDownload) []model.TelegramCloudDownloadItem {
	if len(task.Items) > 0 {
		return task.Items
	}
	return []model.TelegramCloudDownloadItem{{
		EncryptedLink: task.EncryptedLink, Status: task.Status, RemoteTaskID: task.RemoteTaskID,
		MissingSince: task.MissingSince, Progress: task.Progress, Detail: task.Detail,
	}}
}

func cloudDownloadItemStatus(item model.TelegramCloudDownloadItem) string {
	switch item.Status {
	case model.CloudDownloadQueued:
		return "准备提交"
	case model.CloudDownloadSubmitting:
		return "正在确认 115 是否已接收任务"
	case model.CloudDownloadTracking:
		if item.Progress >= 0 {
			return fmt.Sprintf("115 下载中 · %d%%", item.Progress)
		}
		return "已提交，等待 115 下载"
	case model.CloudDownloadDone:
		return "下载完成，文件已存入暂存目录"
	case model.CloudDownloadFailed:
		return "115 云下载失败"
	}
	return "等待提交"
}

func cloudDownloadResultPages(count int) int {
	return (count + cloudDownloadResultPageSize - 1) / cloudDownloadResultPageSize
}

func cloudDownloadMessage(task *model.TelegramCloudDownload) (string, [][]telegramButton) {
	text := "☁️ <b>115 云下载</b>\n\n暂存目录：" + html.EscapeString(clampTelegramRunes(task.Directory, 200))
	items := cloudDownloadItems(task)
	if len(items) == 1 {
		text += "\n状态：" + cloudDownloadItemStatus(items[0])
		if detail := firstNonEmpty(items[0].Detail, task.Detail); detail != "" {
			text += "\n" + html.EscapeString(clampTelegramRunes(detail, 320))
		}
		return text, nil
	}
	var done, failed int
	for _, item := range items {
		switch item.Status {
		case model.CloudDownloadDone:
			done++
		case model.CloudDownloadFailed:
			failed++
		}
	}
	text += fmt.Sprintf("\n共 %d 项 · 已完成 %d · 处理中 %d · 失败 %d", len(items), done, len(items)-done-failed, failed)
	if task.Detail != "" {
		text += "\n" + html.EscapeString(clampTelegramRunes(task.Detail, 320))
	}
	pages := cloudDownloadResultPages(len(items))
	page := min(max(task.DetailPage, 0), pages-1)
	start := page * cloudDownloadResultPageSize
	end := min(start+cloudDownloadResultPageSize, len(items))
	for i := start; i < end; i++ {
		item := items[i]
		text += fmt.Sprintf("\n\n%d. %s\n%s", i+1, html.EscapeString(clampTelegramRunes(item.Name, 64)), cloudDownloadItemStatus(item))
		if item.Detail != "" {
			text += " · " + html.EscapeString(clampTelegramRunes(item.Detail, 100))
		}
	}
	if pages == 1 {
		return text, nil
	}
	text += fmt.Sprintf("\n\n第 %d/%d 页", page+1, pages)
	var navigation []telegramButton
	if page > 0 {
		navigation = append(navigation, telegramButton{Text: "上一页", Data: fmt.Sprintf("nook:dlresult:%s:%d", task.ID, page-1)})
	}
	if page+1 < pages {
		navigation = append(navigation, telegramButton{Text: "下一页", Data: fmt.Sprintf("nook:dlresult:%s:%d", task.ID, page+1)})
	}
	return text, [][]telegramButton{navigation}
}
