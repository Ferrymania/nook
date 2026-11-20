package service

import (
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
)

func newTokenTestRepo(t *testing.T) *repository.Container {
	t.Helper()
	db := newServiceTestDB(t, &model.User{}, &model.RefreshToken{}, &model.Setting{})
	return repository.New(db)
}

// 落库的 refresh token（仍在后台补写队列中）在刷新时被接受，而不是把用户
// 踢回登录页（历史上「经常登录报错」的来源之一）。
func TestRefreshAcceptsPendingDelayedToken(t *testing.T) {
	t.Parallel()

	repos := newTokenTestRepo(t)
	cfg := &config.Config{}
	cfg.Secrets.JWTSecret = "test-secret"
	svc := NewTokenService(cfg, zap.NewNop(), repos)

	u := &model.User{Username: "u1", PasswordHash: "x", Role: "user", Tier: "free", IsActive: true}
	if err := repos.User.Create(t.Context(), u); err != nil {
		t.Fatal(err)
	}

	refreshToken := "pending-token-value"
	hash := repository.HashToken(refreshToken)
	if !svc.trackDelayedStore(model.RefreshToken{UserID: u.ID, TokenHash: hash, ExpiresAt: time.Now().Add(time.Hour)}) {
		t.Fatal("trackDelayedStore returned false")
	}

	pair, err := svc.Refresh(t.Context(), refreshToken)
	if err != nil {
		t.Fatalf("Refresh rejected pending delayed token: %v", err)
	}
	if pair == nil || pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatalf("Refresh returned incomplete pair: %+v", pair)
	}
	// 轮换后旧令牌应从 pending 表移除。
	if _, still := svc.pendingDelayedStore(hash); still {
		t.Fatal("rotated pending token still tracked")
	}
	// 宽限期内重复提交同一个旧令牌返回同一次轮换的结果（幂等），
	// 而不是把并发/重试的客户端打成 401 并清空会话。
	reused, err := svc.Refresh(t.Context(), refreshToken)
	if err != nil {
		t.Fatalf("reuse within grace window should be idempotent: %v", err)
	}
	if reused.AccessToken != pair.AccessToken || reused.RefreshToken != pair.RefreshToken {
		t.Fatal("reuse should return the same rotated pair")
	}
}

// TestRefreshRejectsRotatedTokenAfterGraceWindow 验证宽限期结束后，
// 旧令牌仍按已撤销处理（token 复用检测语义不变）。
func TestRefreshRejectsRotatedTokenAfterGraceWindow(t *testing.T) {
	t.Parallel()

	repos := newTokenTestRepo(t)
	cfg := &config.Config{}
	cfg.Secrets.JWTSecret = "test-secret"
	svc := NewTokenService(cfg, zap.NewNop(), repos)

	u := &model.User{Username: "u1", PasswordHash: "x", Role: "user", Tier: "free", IsActive: true}
	if err := repos.User.Create(t.Context(), u); err != nil {
		t.Fatal(err)
	}

	pair, err := svc.IssuePair(t.Context(), u)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Refresh(t.Context(), pair.RefreshToken); err != nil {
		t.Fatalf("first refresh: %v", err)
	}

	base := time.Now()
	svc.now = func() time.Time { return base.Add(2 * refreshTokenReuseGrace) }
	if _, err := svc.Refresh(t.Context(), pair.RefreshToken); !errors.Is(err, ErrTokenRevoked) {
		t.Fatalf("expired reuse error = %v, want ErrTokenRevoked", err)
	}
}

// TestRefreshRejectsExpiredPendingToken 验证过期的待落库令牌不会被接受。
func TestRefreshRejectsExpiredPendingToken(t *testing.T) {
	t.Parallel()

	repos := newTokenTestRepo(t)
	cfg := &config.Config{}
	cfg.Secrets.JWTSecret = "test-secret"
	svc := NewTokenService(cfg, zap.NewNop(), repos)

	refreshToken := "expired-pending"
	hash := repository.HashToken(refreshToken)
	svc.trackDelayedStore(model.RefreshToken{UserID: "user-x", TokenHash: hash, ExpiresAt: time.Now().Add(-time.Minute)})

	if _, err := svc.Refresh(t.Context(), refreshToken); err == nil {
		t.Fatal("expired pending token should be rejected")
	}
}

func TestRefreshRejectsOldSessionVersionBeforeAndAfterDelayedStore(t *testing.T) {
	repos, auth, _, _ := newAuthTestServices(t)
	ctx := t.Context()
	user, _, err := auth.Register(ctx, "owner", "original-password")
	if err != nil {
		t.Fatal(err)
	}
	const refreshToken = "pending-session-token"
	row := model.RefreshToken{
		UserID: user.ID, TokenHash: repository.HashToken(refreshToken),
		ExpiresAt: time.Now().Add(time.Hour), SessionVersion: user.SessionVersion,
	}
	auth.tokenSvc.trackDelayedStore(row)
	if err := auth.ChangePassword(ctx, user.ID, "original-password", "updated-password", true); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.RefreshTokens(ctx, refreshToken); !errors.Is(err, ErrTokenRevoked) {
		t.Fatalf("old pending token refresh: got %v, want ErrTokenRevoked", err)
	}
	if err := repos.RefreshToken.Create(ctx, &row); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.RefreshTokens(ctx, refreshToken); !errors.Is(err, ErrTokenRevoked) {
		t.Fatalf("old stored token refresh: got %v, want ErrTokenRevoked", err)
	}
}
