package service

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
)

// Bot / 设备管控相关的设置键，存储在 settings 表。
// 设备限制默认关闭。
const (
	// 开放注册（开注名额）。
	SettingOpenRegEnabled = "telegram.openreg_enabled" // 是否开放注册
	SettingOpenRegLimit   = "telegram.openreg_limit"   // 本轮开注名额上限（0=不限）
	SettingOpenRegUsed    = "telegram.openreg_used"    // 本轮已用名额

	// 防共享：并发播放 / 登录终端数量限制。
	SettingAntiShareEnabled  = "device.antishare_enabled"   // 总开关（默认关）
	SettingMaxConcurrentPlay = "device.max_concurrent_play" // 最大并发播放设备
	SettingMaxLoggedClients  = "device.max_logged_clients"  // 最大同时登录终端
	SettingPlayWindowSeconds = "device.play_window_seconds" // 并发播放判定窗口（秒）
	SettingClientActiveDays  = "device.client_active_days"  // 登录设备活跃天数窗口

	// 受保护账号同时用于设备策略和用户管理。
	SettingProtectedUserIDs = "device.protected_user_ids" // comma separated user IDs
)

// botConfig 是设备管控的已解析配置（含默认值）。
type botConfig struct {
	AntiShareEnabled  bool
	MaxConcurrentPlay int
	MaxLoggedClients  int
	PlayWindowSeconds int
	ClientActiveDays  int
}

// defaultBotConfig returns the safe defaults requested by the operator.
func defaultBotConfig() botConfig {
	return botConfig{
		AntiShareEnabled:  false, // 防共享默认关闭，需管理员显式开启
		MaxConcurrentPlay: 1,
		MaxLoggedClients:  3,
		PlayWindowSeconds: 90,
		ClientActiveDays:  30,
	}
}

// loadBotConfig reads the device-management configuration from settings,
// falling back to safe defaults for any missing/invalid key.
func loadBotConfig(ctx context.Context, repo *repository.Container) botConfig {
	cfg := defaultBotConfig()
	get := func(key string) string {
		v, err := repo.Setting.Get(ctx, key)
		if err != nil {
			return ""
		}
		return v
	}
	cfg.AntiShareEnabled = parseBoolSetting(get(SettingAntiShareEnabled), cfg.AntiShareEnabled)
	cfg.MaxConcurrentPlay = parseIntSettingDefault(get(SettingMaxConcurrentPlay), cfg.MaxConcurrentPlay)
	cfg.MaxLoggedClients = parseIntSettingDefault(get(SettingMaxLoggedClients), cfg.MaxLoggedClients)
	cfg.PlayWindowSeconds = parseIntSettingDefault(get(SettingPlayWindowSeconds), cfg.PlayWindowSeconds)
	cfg.ClientActiveDays = parseIntSettingDefault(get(SettingClientActiveDays), cfg.ClientActiveDays)
	return cfg
}

// ProtectedUserIDSet returns the explicit protected user list.
// Admins and the default admin are protected separately by UserIsProtectedAccount.
func ProtectedUserIDSet(ctx context.Context, repo *repository.Container) map[string]struct{} {
	out := make(map[string]struct{})
	if repo == nil || repo.Setting == nil {
		return out
	}
	raw, err := repo.Setting.Get(ctx, SettingProtectedUserIDs)
	if err != nil {
		return out
	}
	for _, value := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '，' || r == '；' || r == ' ' || r == '\n' || r == '\t'
	}) {
		value = strings.TrimSpace(value)
		if value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

// SaveProtectedUserIDSet persists the explicit protected user list.
func SaveProtectedUserIDSet(ctx context.Context, repo *repository.Container, ids map[string]struct{}) error {
	if repo == nil || repo.Setting == nil {
		return nil
	}
	values := make([]string, 0, len(ids))
	for id := range ids {
		if strings.TrimSpace(id) != "" {
			values = append(values, id)
		}
	}
	sort.Strings(values)
	return repo.Setting.Set(ctx, SettingProtectedUserIDs, strings.Join(values, ","))
}

// UserIsProtectedAccount reports whether a user is protected from destructive
// Bot/device-policy operations.
func UserIsProtectedAccount(ctx context.Context, repo *repository.Container, user *model.User) bool {
	if repo == nil || user == nil {
		return true
	}
	if user.Role == "admin" {
		return true
	}
	if first, err := repo.User.FirstAdmin(ctx); err == nil && first != nil && first.ID == user.ID {
		return true
	}
	_, ok := ProtectedUserIDSet(ctx, repo)[user.ID]
	return ok
}

// parseIntSettingDefault parses an int setting, returning fallback on error.
func parseIntSettingDefault(value string, fallback int) int {
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}
