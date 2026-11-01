package service

import (
	"context"

	"gorm.io/gorm"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/database"
)

// DatabaseAdminService exposes read-only status for the active database.
type DatabaseAdminService struct {
	cfg *config.Config
	db  *gorm.DB
}

func NewDatabaseAdminService(cfg *config.Config, db *gorm.DB) *DatabaseAdminService {
	return &DatabaseAdminService{cfg: cfg, db: db}
}

func (s *DatabaseAdminService) GetStatus(ctx context.Context) *database.DatabaseStatus {
	return database.InspectDatabaseStatus(ctx, s.db, s.cfg)
}
