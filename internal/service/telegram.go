package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/helper"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
)

// Telegram 通知相关的设置键。全部存在 settings 表，由管理员在「设备与通知」
// 设置分组里维护；带安全默认值：未启用时所有发送静默跳过。
const (
	SettingTelegramEnabled     = "telegram.enabled"       // 总开关（默认关）
	SettingTelegramBotToken    = "telegram.bot_token"     // Bot API Token
	SettingTelegramAdminChatID = "telegram.admin_chat_id" // 管理员接收运维通知的会话 ID
)

const (
	// telegramAPITimeout 覆盖单次 sendMessage 请求。
	telegramAPITimeout = 10 * time.Second
	// telegramPollClientTimeout 是长轮询专用 HTTP 客户端的超时，必须 > telegramPollTimeout。
	telegramPollClientTimeout = 35 * time.Second
	// telegramPollTimeout 是 getUpdates 的长轮询等待秒数。
	telegramPollTimeout = 30
	// telegramPollBackoff 是长轮询失败后的重试间隔，避免打爆 API。
	telegramPollBackoff = 5 * time.Second
	// telegramDisabledCheckInterval 是禁用状态下检查配置变化的间隔。
	telegramDisabledCheckInterval = 10 * time.Second
)

// TelegramService 提供管理员运维通知与同步控制。
// 管理命令仅接受系统设置里的管理员私聊 ID。
type TelegramService struct {
	log  *zap.Logger
	repo *repository.Container

	mu sync.Mutex

	// apiBase 允许测试指向本地假服务；生产固定为 Telegram 官方地址。
	apiBase string
	// lastUpdateID 是 getUpdates 的增量水位，避免重复处理同一条命令。
	lastUpdateID int64
	// client 可注入，便于测试替换传输层（sendMessage 等短请求）。
	client *http.Client
	// pollClient 专用于 getUpdates 长轮询，超时须 > telegramPollTimeout。
	pollClient *http.Client
	strm       *StrmService
	syncRun    *telegramSyncRun // guarded by mu; at most one Telegram batch
	images     *ImageProxy
	people     func(context.Context, *model.Media) []map[string]any
	mediaOnce  sync.Once
}

// NewTelegramService 构建 TelegramService。
func NewTelegramService(log *zap.Logger, repo *repository.Container) *TelegramService {
	return &TelegramService{
		log:        log,
		repo:       repo,
		apiBase:    "https://api.telegram.org",
		client:     &http.Client{Timeout: telegramAPITimeout},
		pollClient: &http.Client{Timeout: telegramPollClientTimeout},
	}
}

// Start 在后台运行 Bot 命令长轮询。goroutine 始终启动；
// 未启用或未配置 Token 时 pollLoop 内部休眠等待配置就绪，ctx 结束时停止。
func (s *TelegramService) Start(ctx context.Context) {
	if s == nil || s.repo == nil {
		return
	}
	helper.Go(s.log, "service.telegramBotPoll", func() { s.pollLoop(ctx) })
	if s.log != nil {
		s.log.Info("telegram poller started")
	}
}

type telegramConfig struct {
	Enabled  bool
	BotToken string
	AdminID  string
}

func (s *TelegramService) config(ctx context.Context) telegramConfig {
	return telegramConfig{
		Enabled:  s.enabled(ctx),
		BotToken: s.setting(ctx, SettingTelegramBotToken),
		AdminID:  s.setting(ctx, SettingTelegramAdminChatID),
	}
}

func (s *TelegramService) setting(ctx context.Context, key string) string {
	if s == nil || s.repo == nil || s.repo.Setting == nil {
		return ""
	}
	v, err := s.repo.Setting.Get(ctx, key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(v)
}

func (s *TelegramService) enabled(ctx context.Context) bool {
	return parseBoolSetting(s.setting(ctx, SettingTelegramEnabled), false)
}

// SendToAdmin 把消息发给管理员会话，用于运维类事件（任务失败等）。
func (s *TelegramService) SendToAdmin(ctx context.Context, htmlText string) {
	if s == nil || s.repo == nil || strings.TrimSpace(htmlText) == "" {
		return
	}
	chatID := s.setting(ctx, SettingTelegramAdminChatID)
	if chatID == "" {
		return
	}
	if err := s.sendMessage(ctx, chatID, htmlText); err != nil {
		s.warn("telegram send to admin failed", chatID, err)
	}
}

func (s *TelegramService) warn(msg, target string, err error) {
	if s.log == nil {
		return
	}
	s.log.Warn(msg, zap.String("target", target), zap.Error(err))
}

// Configured 报告通知是否已具备发送条件：启用 + Token + 管理员会话都齐了。
func (s *TelegramService) Configured(ctx context.Context) bool {
	if s == nil {
		return false
	}
	cfg := s.config(ctx)
	return cfg.Enabled && cfg.BotToken != "" && cfg.AdminID != ""
}

// SendToAdminChecked 是需要把失败反馈给管理员的场景（例如设置页的测试按钮）。
// 它配置未就绪时返回错误；普通事件通知仍应使用静默的 SendToAdmin。
func (s *TelegramService) SendToAdminChecked(ctx context.Context, htmlText string) error {
	if s == nil || s.repo == nil {
		return errors.New("telegram service unavailable")
	}
	chatID := s.setting(ctx, SettingTelegramAdminChatID)
	if chatID == "" {
		return errors.New("未配置管理员 Chat ID")
	}
	if !s.enabled(ctx) {
		return errors.New("Telegram 通知未启用")
	}
	if s.setting(ctx, SettingTelegramBotToken) == "" {
		return errors.New("未配置 Bot Token")
	}
	return s.sendMessage(ctx, chatID, htmlText)
}

// sendMessage sends ordinary text notifications after checking configuration.
func (s *TelegramService) sendMessage(ctx context.Context, chatID, htmlText string) error {
	cfg := s.config(ctx)
	if !cfg.Enabled || cfg.BotToken == "" {
		return nil
	}
	payload := map[string]any{
		"chat_id":    chatID,
		"text":       htmlText,
		"parse_mode": "HTML",
	}
	return s.call(ctx, cfg.BotToken, "sendMessage", payload, nil)
}

// call 发起一次 Bot API 调用（使用短请求客户端）。
func (s *TelegramService) call(ctx context.Context, token, method string, payload map[string]any, out any) error {
	return s.callWithClient(ctx, s.client, token, method, payload, out)
}

// callWithClient 发起一次 Bot API 调用，允许注入自定义 HTTP 客户端（例如长轮询专用）。
// 失败时返回 Telegram 的 description，便于排查是 Token 错误、chat 不存在还是被限流。
func (s *TelegramService) callWithClient(ctx context.Context, client *http.Client, token, method string, payload map[string]any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/bot%s/%s", s.apiBase, token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	if client == nil {
		client = &http.Client{Timeout: telegramAPITimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		var requestError *url.Error
		if errors.As(err, &requestError) {
			err = requestError.Err
		}
		return fmt.Errorf("telegram %s: %w", method, redactSensitiveError(err))
	}
	defer func() { _ = resp.Body.Close() }()
	return decodeTelegramResponse(resp, method, out)
}

func decodeTelegramResponse(resp *http.Response, method string, out any) error {
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	var envelope struct {
		OK          bool            `json:"ok"`
		Description string          `json:"description"`
		ErrorCode   int             `json:"error_code"`
		Result      json.RawMessage `json:"result"`
		Parameters  struct {
			RetryAfter int `json:"retry_after"`
		} `json:"parameters"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("telegram %s: decode response: %w", method, err)
	}
	if !envelope.OK {
		if envelope.Description == "" {
			envelope.Description = fmt.Sprintf("http %d", resp.StatusCode)
		}
		status := envelope.ErrorCode
		if status == 0 {
			status = resp.StatusCode
		}
		return &telegramAPIError{Method: method, Description: envelope.Description, Status: status, RetryAfter: envelope.Parameters.RetryAfter}
	}
	if out != nil && len(envelope.Result) > 0 {
		return json.Unmarshal(envelope.Result, out)
	}
	return nil
}

// MaskSecret 保留首尾各两个字符，中间以 *** 取代。用于 Token 与会话 ID
// 这类「界面需要辨识、但不应完整下发」的凭据。
func MaskSecret(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	r := []rune(value)
	if len(r) <= 4 {
		return "***"
	}
	return string(r[:2]) + "***" + string(r[len(r)-2:])
}

// pollLoop 长轮询 Bot 更新，并按配置自动注册管理员命令菜单。
// 当 Telegram 未启用或 Token 未配置时，休眠后继续循环以便配置变更后自动激活。
func (s *TelegramService) pollLoop(ctx context.Context) {
	startedAt := time.Now().Unix()
	var previousToken, menuToken, menuAdmin string
	var nextMenuAttempt time.Time
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		cfg := s.config(ctx)
		if !cfg.Enabled || cfg.BotToken == "" {
			previousToken = ""
			if !sleepCtx(ctx, telegramDisabledCheckInterval) {
				return
			}
			continue
		}
		if cfg.BotToken != previousToken {
			// Establish a fresh watermark before registering menus: do not
			// replay commands or button presses queued before startup/token change.
			if err := s.initializePolling(ctx, cfg.BotToken); err != nil {
				s.warn("telegram poll initialization failed", "bot", err)
				if !sleepCtx(ctx, telegramPollBackoff) {
					return
				}
				continue
			}
			previousToken = cfg.BotToken
			startedAt = time.Now().Unix()
		}
		if (cfg.BotToken != menuToken || cfg.AdminID != menuAdmin) && time.Now().After(nextMenuAttempt) {
			if err := s.registerAdminCommands(ctx, cfg, menuToken, menuAdmin); err != nil {
				s.warn("telegram command menu registration failed", "admin", err)
				nextMenuAttempt = time.Now().Add(time.Minute)
			} else {
				menuToken, menuAdmin = cfg.BotToken, cfg.AdminID
				nextMenuAttempt = time.Time{}
			}
		}
		updates, err := s.fetchUpdates(ctx, cfg.BotToken)
		if err != nil {
			if s.log != nil && ctx.Err() == nil {
				s.log.Warn("telegram getUpdates failed", zap.Error(err))
			}
			if !sleepCtx(ctx, telegramPollBackoff) {
				return
			}
			continue
		}
		for _, u := range updates {
			s.mu.Lock()
			duplicate := u.UpdateID < s.lastUpdateID
			if !duplicate {
				s.lastUpdateID = u.UpdateID + 1
			}
			s.mu.Unlock()
			if duplicate {
				continue
			}
			// Commands sent while Nook was stopped must not start old sync jobs.
			if u.Message != nil && u.Message.Date > 0 && u.Message.Date < startedAt && isTelegramControlCommand(u.Message.Text) {
				continue
			}
			s.handleUpdate(ctx, cfg.BotToken, u)
		}
	}
}

func (s *TelegramService) initializePolling(ctx context.Context, token string) error {
	var updates []telegramUpdate
	if err := s.callWithClient(ctx, s.pollClient, token, "getUpdates", map[string]any{
		"offset": -1, "timeout": 0, "allowed_updates": []string{"message", "callback_query"},
	}, &updates); err != nil {
		return err
	}
	var offset int64
	for _, u := range updates {
		offset = max(offset, u.UpdateID+1)
	}
	s.mu.Lock()
	s.lastUpdateID = offset
	s.mu.Unlock()
	return nil
}

type telegramUpdate struct {
	UpdateID      int64                  `json:"update_id"`
	Message       *telegramMessage       `json:"message"`
	CallbackQuery *telegramCallbackQuery `json:"callback_query"`
}

type telegramChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type telegramSender struct {
	ID int64 `json:"id"`
}

type telegramMessage struct {
	Text string         `json:"text"`
	Date int64          `json:"date"`
	Chat telegramChat   `json:"chat"`
	From telegramSender `json:"from"`
}

type telegramCallbackQuery struct {
	ID      string           `json:"id"`
	Data    string           `json:"data"`
	From    telegramSender   `json:"from"`
	Message *telegramMessage `json:"message"`
}

func (s *TelegramService) fetchUpdates(ctx context.Context, token string) ([]telegramUpdate, error) {
	s.mu.Lock()
	offset := s.lastUpdateID
	s.mu.Unlock()

	payload := map[string]any{
		"timeout":         telegramPollTimeout,
		"allowed_updates": []string{"message", "callback_query"},
	}
	if offset > 0 {
		payload["offset"] = offset
	}
	var updates []telegramUpdate
	if err := s.callWithClient(ctx, s.pollClient, token, "getUpdates", payload, &updates); err != nil {
		return nil, err
	}
	return updates, nil
}

func (s *TelegramService) handleUpdate(ctx context.Context, token string, u telegramUpdate) {
	if u.CallbackQuery != nil {
		s.handleAdminCallback(ctx, token, u.CallbackQuery)
		return
	}
	if u.Message == nil {
		return
	}
	text := strings.TrimSpace(u.Message.Text)
	chatID := fmt.Sprintf("%d", u.Message.Chat.ID)
	if chatID == "0" || text == "" {
		return
	}

	command := telegramCommand(text)
	if isTelegramControlCommand(text) || command == "/start" {
		if s.adminControlAllowed(ctx, token, u.Message.Chat, u.Message.From.ID) {
			s.handleAdminCommand(ctx, token, chatID, command)
			return
		}
	}
	if command == "/start" {
		message := "Nook Bot 提供管理员运维通知与同步控制。"
		if u.Message.Chat.Type == "private" {
			message += "\n\n当前私聊 Chat ID：<code>" + chatID + "</code>。管理员可将此 ID 填入系统设置，使用同步控制命令。"
		}
		if err := s.call(ctx, token, "sendMessage", map[string]any{
			"chat_id":    chatID,
			"text":       message,
			"parse_mode": "HTML",
		}, nil); err != nil {
			s.warn("telegram /start reply failed", chatID, err)
		}
	}
}

// sleepCtx 等待指定时长，ctx 结束则提前返回 false。
func sleepCtx(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
