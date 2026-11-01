package database

import (
	"context"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
)

// DatabaseStatus describes the currently active database engine and runtime metrics.
type DatabaseStatus struct {
	Type         string           `json:"type"`
	DSN          string           `json:"dsn,omitempty"`
	Connected    bool             `json:"connected"`
	Error        string           `json:"error,omitempty"`
	CountsError  string           `json:"counts_error,omitempty"`
	OpenConns    int              `json:"open_conns"`
	InUse        int              `json:"in_use"`
	Idle         int              `json:"idle"`
	MaxOpenConns int              `json:"max_open_conns"`
	TableCounts  map[string]int64 `json:"table_counts"`
}

// InspectDatabaseStatus queries the currently active database for metrics and table rows.
func InspectDatabaseStatus(ctx context.Context, db *gorm.DB, cfg *config.Config) *DatabaseStatus {
	st := &DatabaseStatus{
		Type:        "postgres",
		Error:       "无法连接 PostgreSQL，请检查数据库服务及连接配置。",
		TableCounts: make(map[string]int64),
	}
	if cfg != nil {
		st.DSN = MaskDSN(cfg.Database.DSN)
	}

	if db == nil {
		return st
	}
	sqlDB, err := db.DB()
	if err != nil {
		return st
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return st
	}

	st.Connected = true
	st.Error = ""
	stats := sqlDB.Stats()
	st.OpenConns = stats.OpenConnections
	st.InUse = stats.InUse
	st.Idle = stats.Idle
	st.MaxOpenConns = stats.MaxOpenConnections

	// Failed counts stay absent, so callers can distinguish them from real zeroes.
	for _, table := range []struct {
		name  string
		model any
	}{
		{"media", &model.Media{}},
		{"users", &model.User{}},
		{"playback_histories", &model.PlaybackHistory{}},
	} {
		var count int64
		if err := db.WithContext(ctx).Unscoped().Model(table.model).Count(&count).Error; err != nil {
			st.CountsError = "部分记录统计获取失败。"
			continue
		}
		st.TableCounts[table.name] = count
	}
	return st
}

// MaskDSN masks the password in a connection string for safe API responses.
func MaskDSN(rawDSN string) string {
	rawDSN = strings.TrimSpace(rawDSN)
	if rawDSN == "" {
		return ""
	}
	if u, err := url.Parse(rawDSN); err == nil && u.User != nil {
		if pass, hasPassword := u.User.Password(); hasPassword && pass != "" {
			rawUserPass := u.User.String()
			user := u.User.Username()
			maskedUserPass := user + ":******"
			return strings.Replace(rawDSN, rawUserPass+"@", maskedUserPass+"@", 1)
		}
	}
	// Fallback for keyword-style DSN (e.g. host=... password=...)
	if strings.Contains(rawDSN, "password=") {
		parts := strings.Fields(rawDSN)
		for i, p := range parts {
			if strings.HasPrefix(p, "password=") {
				parts[i] = "password=******"
			}
		}
		return strings.Join(parts, " ")
	}
	return rawDSN
}
