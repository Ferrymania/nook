package service

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/database"
	"github.com/Ferrymania/nook/internal/repository"
)

// DatabaseAdminService manages database configuration, and connectivity testing.
type DatabaseAdminService struct {
	cfg   *config.Config
	log   *zap.Logger
	repos *repository.Container
	db    *gorm.DB
}

// NewDatabaseAdminService creates a new DatabaseAdminService.
func NewDatabaseAdminService(cfg *config.Config, log *zap.Logger, repos *repository.Container, db *gorm.DB) *DatabaseAdminService {
	if log == nil {
		log = zap.NewNop()
	}
	return &DatabaseAdminService{
		cfg:   cfg,
		log:   log,
		repos: repos,
		db:    db,
	}
}

// GetStatus returns the status of the currently active database.
func (s *DatabaseAdminService) GetStatus(ctx context.Context) *database.DatabaseStatus {
	return database.InspectDatabaseStatus(s.db, s.cfg)
}

// TestPostgres verifies connectivity and permissions to the specified PostgreSQL DSN.
func (s *DatabaseAdminService) TestPostgres(ctx context.Context, dsn string) (*database.PostgresTestResult, error) {
	return database.TestPostgres(dsn)
}

// SaveConfig persists the database configuration to config.yaml and the database settings table.
func (s *DatabaseAdminService) SaveConfig(ctx context.Context, dbType, dsn string) error {
	dbType = strings.TrimSpace(dbType)
	dsn = strings.TrimSpace(dsn)
	if dbType == "" {
		dbType = "postgres"
	}
	if dbType != "postgres" {
		return fmt.Errorf("仅支持 PostgreSQL")
	}
	if dsn == "" {
		return fmt.Errorf("PostgreSQL DSN 不能为空")
	}

	// 1. 保存到本地 config.yaml
	if err := config.SaveDatabaseConfig(dbType, dsn); err != nil {
		return fmt.Errorf("保存配置文件失败: %w", err)
	}

	// 2. 更新内存配置
	s.cfg.Database.Type = dbType
	s.cfg.Database.DSN = dsn

	// 3. 同时更新 settings 存储库作为副本
	if s.repos != nil && s.repos.Setting != nil {
		_ = s.repos.Setting.Set(ctx, "database.type", dbType)
		_ = s.repos.Setting.Set(ctx, "database.dsn", dsn)
	}

	s.log.Info("database configuration saved", zap.String("type", dbType), zap.String("dsn", database.MaskDSN(dsn)))
	return nil
}
