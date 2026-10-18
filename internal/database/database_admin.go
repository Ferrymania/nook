package database

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
)

// DatabaseStatus describes the currently active database engine and runtime metrics.
type DatabaseStatus struct {
	Type         string           `json:"type"`
	DSN          string           `json:"dsn,omitempty"`
	OpenConns    int              `json:"open_conns"`
	InUse        int              `json:"in_use"`
	Idle         int              `json:"idle"`
	MaxOpenConns int              `json:"max_open_conns"`
	TableCounts  map[string]int64 `json:"table_counts"`
}

// PostgresTestResult returns latency and version info after testing connection.
type PostgresTestResult struct {
	Success   bool   `json:"success"`
	LatencyMS int64  `json:"latency_ms"`
	Version   string `json:"version,omitempty"`
	Message   string `json:"message,omitempty"`
	Error     string `json:"error,omitempty"`
}

// InspectDatabaseStatus queries the currently active database for metrics and table rows.
func InspectDatabaseStatus(db *gorm.DB, cfg *config.Config) *DatabaseStatus {
	st := &DatabaseStatus{
		Type:        "postgres",
		TableCounts: make(map[string]int64),
	}
	if cfg != nil {
		st.DSN = MaskDSN(cfg.Database.DSN)
	}

	if db != nil {
		if sqlDB, err := db.DB(); err == nil {
			stats := sqlDB.Stats()
			st.OpenConns = stats.OpenConnections
			st.InUse = stats.InUse
			st.Idle = stats.Idle
			st.MaxOpenConns = stats.MaxOpenConnections
		}

		// Count rows for major model tables
		for _, m := range model.AllModels() {
			if tbl, err := modelTableName(db, m); err == nil {
				if db.Migrator().HasTable(tbl) {
					var count int64
					if err := db.Raw("SELECT COUNT(1) FROM " + quoteIdent(tbl)).Scan(&count).Error; err == nil {
						st.TableCounts[tbl] = count
					}
				}
			}
		}
	}
	return st
}

// TestPostgres establishes a temporary connection to verify reachability and permissions.
func TestPostgres(dsn string) (*PostgresTestResult, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return &PostgresTestResult{
			Success: false,
			Error:   "PostgreSQL DSN 不能为空",
		}, nil
	}

	start := time.Now()
	testDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return &PostgresTestResult{
			Success: false,
			Error:   fmt.Sprintf("连接失败: %v", err),
		}, nil
	}

	sqlDB, err := testDB.DB()
	if err != nil {
		return &PostgresTestResult{
			Success: false,
			Error:   fmt.Sprintf("获取底层连接失败: %v", err),
		}, nil
	}
	defer sqlDB.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		return &PostgresTestResult{
			Success: false,
			Error:   fmt.Sprintf("Ping 超时或失败: %v", err),
		}, nil
	}

	var version string
	if err := testDB.WithContext(ctx).Raw("SELECT version()").Scan(&version).Error; err != nil {
		version = "PostgreSQL (unknown version)"
	}

	latency := time.Since(start).Milliseconds()
	return &PostgresTestResult{
		Success:   true,
		LatencyMS: latency,
		Version:   version,
		Message:   "连接成功",
	}, nil
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
