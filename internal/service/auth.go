// Package service — authentication / user management.
package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/helper"
	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
)

// AuthService handles registration, login, and JWT issuance.
type AuthService struct {
	cfg           *config.Config
	log           *zap.Logger
	repo          *repository.Container
	tokenSvc      *TokenService
	permissionSvc *PermissionService
}

// NewAuthService is the constructor.
func NewAuthService(cfg *config.Config, log *zap.Logger, repo *repository.Container, tokenSvc *TokenService, permissionSvc *PermissionService) *AuthService {
	return &AuthService{
		cfg:           cfg,
		log:           log,
		repo:          repo,
		tokenSvc:      tokenSvc,
		permissionSvc: permissionSvc,
	}
}

// Common service-level errors.
var (
	ErrInvalidCredentials    = errors.New("invalid username or password")
	ErrWebAccessDisabled     = errors.New("此账号仅允许通过 Emby 客户端访问")
	ErrUsernameTaken         = errors.New("username already taken")
	ErrUserInactive          = errors.New("user account is inactive")
	ErrUserLimitReached      = errors.New("user limit reached")
	ErrUserExpired           = errors.New("user account has expired")
	ErrPasswordResetRequired = errors.New("请先在 Nook 网页修改初始密码")
)

const DefaultAdminPassword = "admin123"

// MaxUsers is kept for tests that seed up to the default cap.
const MaxUsers = DefaultMaxUsers

// SeedAdmin makes sure at least one admin user exists. It mirrors the
// legacy default behaviour: if no admin row is found we create
// `admin / admin123` (overridable through ADMIN_INITIAL_PASSWORD) and warn.
func (s *AuthService) SeedAdmin(ctx context.Context) error {
	n, err := s.repo.User.CountAdmins(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		// The protected administrator may have been renamed. Check its hash,
		// rather than its username, without changing its password or preferences.
		u, err := s.repo.User.FirstAdmin(ctx)
		if err != nil || u == nil {
			return err
		}
		if !u.ForcePasswordReset && bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(DefaultAdminPassword)) == nil {
			return s.requirePasswordReset(ctx, u)
		}
		return nil
	}
	pwd := os.Getenv("ADMIN_INITIAL_PASSWORD")
	if pwd == "" {
		pwd = DefaultAdminPassword
	}
	hash, err := hashPassword(pwd)
	if err != nil {
		return err
	}
	user := &model.User{
		Username:           "admin",
		PasswordHash:       hash,
		Role:               "admin",
		Tier:               "plus",
		ForcePasswordReset: pwd == DefaultAdminPassword,
	}
	if err := s.repo.User.Create(ctx, user); err != nil {
		return err
	}
	// 确保管理员有权限记录
	_, _ = s.permissionSvc.EnsureForUser(ctx, user.ID)
	s.log.Warn("default admin created — change the password after first login",
		zap.String("username", "admin"),
		zap.String("password_source", "ADMIN_INITIAL_PASSWORD or admin123"),
	)
	return nil
}

// Register creates a new user. The first registered user is auto-promoted to
// admin to support fresh installs that did not run SeedAdmin.
func (s *AuthService) Register(ctx context.Context, username, password string) (*model.User, *TokenPair, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return nil, nil, fmt.Errorf("username and password required")
	}
	if existing, err := s.repo.User.FindByUsername(ctx, username); err != nil {
		return nil, nil, err
	} else if existing != nil {
		return nil, nil, ErrUsernameTaken
	}
	if err := s.repo.User.ReleaseDeletedUsername(ctx, username); err != nil {
		return nil, nil, err
	}
	limit, err := LoadMaxUsers(ctx, s.repo)
	if err != nil {
		return nil, nil, err
	}
	if n, err := s.repo.User.Count(ctx); err != nil {
		return nil, nil, err
	} else if n >= int64(limit) {
		return nil, nil, ErrUserLimitReached
	}
	hash, err := hashPassword(password)
	if err != nil {
		return nil, nil, err
	}
	role := "user"
	if n, err := s.repo.User.CountAdmins(ctx); err == nil && n == 0 {
		role = "admin"
	}
	u := &model.User{
		Username:           username,
		PasswordHash:       hash,
		Role:               role,
		Tier:               "free",
		ForcePasswordReset: role == "admin" && password == DefaultAdminPassword,
	}
	if err := s.repo.User.Create(ctx, u); err != nil {
		return nil, nil, err
	}
	// 自动为新用户创建默认权限
	_, _ = s.permissionSvc.EnsureForUser(ctx, u.ID)
	// 签发令牌对
	tokens, err := s.loginTokens(ctx, u, false)
	if err != nil {
		return u, nil, nil // 用户已创建，令牌签发失败不影响注册成功
	}
	return u, tokens, nil
}

// LoginResponse 登录响应结构。
type LoginResponse struct {
	User   *model.User `json:"user"`
	Tokens *TokenPair  `json:"tokens"`
}

// Login validates credentials and returns the user + a fresh JWT token pair.
func (s *AuthService) Login(ctx context.Context, username, password string) (*LoginResponse, error) {
	u, err := s.authenticatePassword(ctx, username, password)
	if err != nil {
		return nil, err
	}
	if !u.AllowsWebAccess() {
		return nil, ErrWebAccessDisabled
	}
	tokens, err := s.loginTokens(ctx, u, true)
	if err != nil {
		return nil, err
	}
	s.touchLoginBestEffort(u.ID)
	return &LoginResponse{User: u, Tokens: tokens}, nil
}

// LoginEmby authenticates a compatible client with their account password.
func (s *AuthService) LoginEmby(ctx context.Context, username, password string) (*model.User, string, error) {
	u, err := s.authenticatePassword(ctx, username, password)
	if err != nil {
		return nil, "", err
	}
	token, err := s.IssueEmbyToken(u)
	if err != nil {
		return nil, "", err
	}
	s.touchLoginBestEffort(u.ID)
	return u, token, nil
}

func (s *AuthService) authenticatePassword(ctx context.Context, username, password string) (*model.User, error) {
	u, err := s.repo.User.FindByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, ErrInvalidCredentials
	}
	// 检查用户是否激活
	if !u.IsActive {
		return nil, ErrUserInactive
	}
	// 账号到期则停用登录，直到管理员或兑换码续期。
	if u.ExpiredAt != nil && time.Now().After(*u.ExpiredAt) {
		return nil, ErrUserExpired
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}
	if u.Role == "admin" && password == DefaultAdminPassword && !u.ForcePasswordReset {
		if err := s.requirePasswordReset(ctx, u); err != nil {
			return nil, err
		}
	}
	return u, nil
}

func (s *AuthService) requirePasswordReset(ctx context.Context, u *model.User) error {
	result := s.repo.DB.WithContext(ctx).Model(&model.User{}).
		Where("id = ? AND password_hash = ?", u.ID, u.PasswordHash).
		Update("force_password_reset", true)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrInvalidCredentials
	}
	u.ForcePasswordReset = true
	return nil
}

func (s *AuthService) loginTokens(ctx context.Context, u *model.User, bestEffort bool) (*TokenPair, error) {
	if u.ForcePasswordReset {
		return s.issuePasswordResetToken(u)
	}
	if bestEffort {
		return s.tokenSvc.IssuePairBestEffort(ctx, u)
	}
	return s.tokenSvc.IssuePair(ctx, u)
}

func (s *AuthService) touchLoginBestEffort(userID string) {
	if s == nil || s.repo == nil || s.repo.User == nil || strings.TrimSpace(userID) == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		helper.Run(s.log, "auth.touchLogin", func() {
			if err := s.repo.User.TouchLogin(ctx, userID); err != nil && s.log != nil {
				s.log.Debug("touch login delayed", zap.String("user_id", userID), zap.Error(err))
			}
		})
	}()
}

// ChangePassword updates the user password if the old one matches.
func (s *AuthService) ChangePassword(ctx context.Context, userID, oldPwd, newPwd string, signOutDevices bool) error {
	if strings.TrimSpace(newPwd) == "" || len(newPwd) < 6 {
		return errors.New("new password must be at least 6 characters")
	}
	u, err := s.repo.User.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if u == nil {
		return ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(oldPwd)); err != nil {
		return ErrInvalidCredentials
	}
	if newPwd == DefaultAdminPassword {
		return errors.New("新密码不能使用默认密码 admin123")
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(newPwd)) == nil {
		return errors.New("新密码不能与当前密码相同")
	}
	hash, err := hashPassword(newPwd)
	if err != nil {
		return err
	}
	changed, err := s.repo.User.ReplacePassword(ctx, userID, u.PasswordHash, hash, signOutDevices || u.ForcePasswordReset)
	if err != nil {
		return err
	}
	if !changed {
		return ErrInvalidCredentials
	}
	return nil
}

// ResetPassword lets an administrator set a new password without knowing the
// user's old password.
func (s *AuthService) ResetPassword(ctx context.Context, userID, newPwd string) error {
	if strings.TrimSpace(userID) == "" {
		return errors.New("missing user id")
	}
	if strings.TrimSpace(newPwd) == "" || len(newPwd) < 6 {
		return errors.New("new password must be at least 6 characters")
	}
	if newPwd == DefaultAdminPassword {
		return errors.New("新密码不能使用默认密码 admin123")
	}
	u, err := s.repo.User.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if u == nil {
		return errors.New("user not found")
	}
	hash, err := hashPassword(newPwd)
	if err != nil {
		return err
	}
	return s.repo.User.UpdatePassword(ctx, userID, hash)
}

// VerifyPassword checks a user's current password without mutating account
// state. It is used for sensitive self-service actions such as deleting play profiles.
func (s *AuthService) VerifyPassword(ctx context.Context, userID, password string) error {
	u, err := s.repo.User.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if u == nil || strings.TrimSpace(password) == "" {
		return ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return ErrInvalidCredentials
	}
	return nil
}

func hashPassword(p string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}
