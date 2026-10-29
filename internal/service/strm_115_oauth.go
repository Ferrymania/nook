// 115 官方 PKCE 设备码扫码授权会话与 Token 维护。
// 授权成功后将 access_token / refresh_token 写入网盘账号配置（加密存储）。
package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/service/cloud115"
)

const strm115AuthSessionTTL = 6 * time.Minute

// strm115AuthSession 是一次扫码授权会话（内存态，重启后需重新授权）。
type strm115AuthSession struct {
	AccountID string
	Source    cloud115.Source
	QrCode    *cloud115.QrCodeDataReturn
	CreatedAt time.Time
}

// Strm115AuthStartResult 是授权发起结果。
type Strm115AuthStartResult struct {
	SessionID string `json:"session_id"`
	Mode      string `json:"mode"` // qrcode
	ExpiresIn int64  `json:"expires_in"`
	QRCode    *struct {
		UID    string `json:"uid"`
		Time   int64  `json:"time"`
		Sign   string `json:"sign"`
		Qrcode string `json:"qrcode"`
	} `json:"qrcode"`
}

// Strm115AuthStatus 是授权轮询结果。
type Strm115AuthStatus struct {
	Status string `json:"status"` // waiting / scanned / confirmed / expired
	Tip    string `json:"tip"`
}

// List115Sources 返回官方应用目录。
func (s *StrmService) List115Sources(ctx context.Context) (map[string][]cloud115.Source, error) {
	return map[string][]cloud115.Source{"built_in": cloud115.BuiltInAppIDSources()}, nil
}

// Start115OAuth 使用官方应用目录或自定义 APP ID 发起设备码扫码。
func (s *StrmService) Start115OAuth(ctx context.Context, accountID, authSource, appID string) (*Strm115AuthStartResult, error) {
	acct, err := s.repo.StrmAccount.FindByID(ctx, accountID)
	if err != nil || acct == nil {
		return nil, errNotFoundOr(err, "网盘账号不存在")
	}
	if acct.Provider != model.StrmProvider115 {
		return nil, errors.New("该账号不是 115 网盘账号")
	}
	source, err := resolve115Source(authSource, appID)
	if err != nil {
		return nil, err
	}
	client := cloud115.NewOpenClient(source.AppID, "", "")
	qr, err := client.GetQrCode()
	if err != nil {
		return nil, fmt.Errorf("获取 115 授权二维码失败：%w", err)
	}
	sessionID := newStrmID()
	session := &strm115AuthSession{
		AccountID: accountID, Source: source, QrCode: qr, CreatedAt: time.Now(),
	}
	result := &Strm115AuthStartResult{SessionID: sessionID, Mode: "qrcode", ExpiresIn: 300}
	result.QRCode = &struct {
		UID    string `json:"uid"`
		Time   int64  `json:"time"`
		Sign   string `json:"sign"`
		Qrcode string `json:"qrcode"`
	}{UID: qr.Uid, Time: qr.Time, Sign: qr.Sign, Qrcode: qr.Qrcode}

	s.mu.Lock()
	if s.oauthSessions == nil {
		s.oauthSessions = map[string]*strm115AuthSession{}
	}
	s.oauthSessions[sessionID] = session
	s.mu.Unlock()
	s.sweep115AuthSessions()
	return result, nil
}

// Poll115OAuth 轮询官方扫码状态，确认后将 token 写入账号。
func (s *StrmService) Poll115OAuth(ctx context.Context, sessionID string) (*Strm115AuthStatus, error) {
	s.mu.Lock()
	session := s.oauthSessions[sessionID]
	s.mu.Unlock()
	if session == nil {
		return nil, errors.New("授权会话不存在或已过期，请重新发起")
	}
	if time.Since(session.CreatedAt) > strm115AuthSessionTTL {
		s.drop115AuthSession(sessionID)
		return &Strm115AuthStatus{Status: "expired", Tip: "授权会话已过期"}, nil
	}
	client := cloud115.NewOpenClient(session.Source.AppID, "", "")
	status, err := client.QrCodeScanStatus(&session.QrCode.QrCodeData)
	if err != nil {
		return nil, fmt.Errorf("查询扫码状态失败：%w", err)
	}
	switch status {
	case cloud115.QrCodeScanStatusNotScanned:
		return &Strm115AuthStatus{Status: "waiting", Tip: "等待扫码"}, nil
	case cloud115.QrCodeScanStatusScanned:
		return &Strm115AuthStatus{Status: "scanned", Tip: "已扫码，请在 115 客户端确认"}, nil
	case cloud115.QrCodeScanStatusExpired:
		s.drop115AuthSession(sessionID)
		return &Strm115AuthStatus{Status: "expired", Tip: "二维码已过期"}, nil
	case cloud115.QrCodeScanStatusConfirmed:
		token, err := client.GetToken(session.QrCode)
		if err != nil {
			return nil, fmt.Errorf("获取访问令牌失败：%w", err)
		}
		if err := s.save115OAuthToken(ctx, session, token); err != nil {
			return nil, err
		}
		s.drop115AuthSession(sessionID)
		return &Strm115AuthStatus{Status: "confirmed", Tip: "授权成功"}, nil
	default:
		return &Strm115AuthStatus{Status: "waiting", Tip: "授权处理中"}, nil
	}
}

// save115OAuthToken 把授权 token 写入账号配置并附带用户信息。
func (s *StrmService) save115OAuthToken(ctx context.Context, session *strm115AuthSession, token *cloud115.TokenData) error {
	acct, err := s.repo.StrmAccount.FindByID(ctx, session.AccountID)
	if err != nil || acct == nil {
		return errNotFoundOr(err, "网盘账号不存在")
	}
	cfg, err := s.strmAccountConfig(acct)
	if err != nil {
		return err
	}
	cfg["app_id"] = session.Source.AppID
	cfg["access_token"] = s.crypto.Encrypt(token.AccessToken)
	cfg["refresh_token"] = s.crypto.Encrypt(token.RefreshToken)
	// 尝试补充用户信息（失败不阻塞授权）
	if client := cloud115.NewOpenClient(session.Source.AppID, token.AccessToken, token.RefreshToken); client != nil {
		if info, err := client.FetchUserInfo(ctx); err == nil && info != nil {
			cfg["user_id"] = info.UserId.String()
			cfg["user_name"] = info.UserName
		}
	}
	enc, err := s.strmAccountConfigJSON(cfg, false)
	if err != nil {
		return err
	}
	acct.Config = enc
	now := time.Now()
	acct.LastTestAt = &now
	acct.LastTestResult = "授权成功"
	acct.LastTestOK = true
	if err := s.repo.StrmAccount.Update(ctx, acct); err != nil {
		return err
	}
	s.invalidate115Provider(acct.ID)
	return nil
}

func (s *StrmService) drop115AuthSession(sessionID string) {
	s.mu.Lock()
	delete(s.oauthSessions, sessionID)
	s.mu.Unlock()
}

func (s *StrmService) sweep115AuthSessions() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, session := range s.oauthSessions {
		if time.Since(session.CreatedAt) > strm115AuthSessionTTL {
			delete(s.oauthSessions, id)
		}
	}
}

// resolve115Source 解析授权来源。
func resolve115Source(authSource, appID string) (cloud115.Source, error) {
	authSource = strings.TrimSpace(authSource)
	appID = strings.TrimSpace(appID)
	switch authSource {
	case string(cloud115.SourceTypeBuiltInAppID):
		if source, ok := cloud115.FindSource(cloud115.SourceTypeBuiltInAppID, cloud115.ProviderOfficialPKCE, appID); ok {
			return source, nil
		}
		return cloud115.Source{}, fmt.Errorf("未知的内置应用 ID：%s", appID)
	case string(cloud115.SourceTypeCustomAppID):
		if appID == "" {
			return cloud115.Source{}, errors.New("自定义 APP ID 不能为空")
		}
		return cloud115.Source{SourceType: cloud115.SourceTypeCustomAppID, Provider: cloud115.ProviderOfficialPKCE, AppID: appID, AppName: cloud115.CustomAppName, DisplayName: cloud115.CustomAppName}, nil
	default:
		return cloud115.Source{}, errors.New("不支持的授权来源")
	}
}

// refresh115TokensLoop 定期刷新 115 开放平台访问令牌。
func (s *StrmService) refresh115TokensLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.refresh115TokensOnce(ctx)
		}
	}
}

func (s *StrmService) refresh115TokensOnce(ctx context.Context) {
	accounts, err := s.repo.StrmAccount.List(ctx)
	if err != nil {
		return
	}
	for i := range accounts {
		acct := &accounts[i]
		if acct.Provider != model.StrmProvider115 || !acct.Enabled {
			continue
		}
		cfg, err := s.strmAccountConfig(acct)
		if err != nil || cfg["access_token"] == "" || cfg["refresh_token"] == "" {
			continue
		}
		// 临期才刷：115 refresh_token 是一次性轮转，无条件周期刷新会与
		// 运行中任务的自动刷新互相作废对方的凭据。24h 内刷新过（含运行
		// 中回调落库）就跳过；access_token 有效期远长于 24h。
		if last := strings.TrimSpace(cfg["token_refreshed_at"]); last != "" {
			if ts, perr := strconv.ParseInt(last, 10, 64); perr == nil {
				if time.Since(time.Unix(ts, 0)) < 24*time.Hour {
					continue
				}
			}
		}
		provider, err := s.providerFor(ctx, acct)
		if err != nil {
			continue
		}
		openProvider, ok := provider.(interface{ OpenClient() *cloud115.OpenClient })
		if !ok || openProvider.OpenClient() == nil {
			continue
		}
		token, err := openProvider.OpenClient().RefreshToken("")
		if err != nil {
			msg := "令牌刷新失败：" + err.Error()
			if cloud115.IsRefreshTokenDead(err) {
				msg = "授权已失效，请重新授权：" + err.Error()
				cfg["access_token"] = ""
			}
			now := time.Now()
			acct.LastTestAt = &now
			acct.LastTestResult = msg
			acct.LastTestOK = false
			if cfg["access_token"] == "" {
				enc, encErr := s.strmAccountConfigJSON(cfg, false)
				if encErr == nil {
					acct.Config = enc
				}
			}
			_ = s.repo.StrmAccount.Update(ctx, acct)
			s.log.Warn("115 token refresh failed", zap.String("account", acct.Name), zap.String("error", msg))
			continue
		}
		cfg["access_token"] = s.crypto.Encrypt(token.AccessToken)
		cfg["refresh_token"] = s.crypto.Encrypt(token.RefreshToken)
		cfg["token_refreshed_at"] = strconv.FormatInt(time.Now().Unix(), 10)
		enc, err := s.strmAccountConfigJSON(cfg, false)
		if err != nil {
			continue
		}
		acct.Config = enc
		now := time.Now()
		acct.LastTestAt = &now
		acct.LastTestResult = "ok"
		acct.LastTestOK = true
		if err := s.repo.StrmAccount.Update(ctx, acct); err != nil {
			s.log.Warn("update 115 token failed", zap.Error(err))
		}
	}
}

// persist115Tokens 把运行中任务自动刷新得到的新令牌加密写回账号配置，
// 并记录刷新时间供定时刷新线程做临期判断。
func (s *StrmService) persist115Tokens(accountID, accessToken, refreshToken string) {
	ctx := context.Background()
	acct, err := s.repo.StrmAccount.FindByID(ctx, accountID)
	if err != nil || acct == nil {
		return
	}
	cfg, err := s.strmAccountConfig(acct)
	if err != nil {
		return
	}
	cfg["access_token"] = s.crypto.Encrypt(accessToken)
	cfg["refresh_token"] = s.crypto.Encrypt(refreshToken)
	cfg["token_refreshed_at"] = strconv.FormatInt(time.Now().Unix(), 10)
	enc, err := s.strmAccountConfigJSON(cfg, false)
	if err != nil {
		return
	}
	acct.Config = enc
	now := time.Now()
	acct.LastTestAt = &now
	acct.LastTestResult = "ok"
	acct.LastTestOK = true
	if err := s.repo.StrmAccount.Update(ctx, acct); err != nil {
		s.log.Warn("persist refreshed 115 token failed", zap.Error(err), zap.String("account", acct.Name))
		return
	}
	s.log.Info("115 token refreshed and persisted", zap.String("account", acct.Name))
}

// newStrmID 生成授权会话 ID。
func newStrmID() string {
	return "auth-" + strings.ReplaceAll(time.Now().Format("150405.000000000"), ".", "") + "-" + cloud115.RandomString(8)
}
