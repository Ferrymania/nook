// Package service — third-party API key store.
//
// APIConfigService is a small CRUD layer over the api_configs table. It
// transparently encrypts the api_key column on write and decrypts it on
// read so values stored on disk are useless without the JWT secret.
//
// On first read it seeds the table with the providers supported by
// Nook today (TMDb / Bangumi / TheTVDB / Fanart / OpenAI).
package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
)

// APIConfigService coordinates third-party API key storage.
type APIConfigService struct {
	log            *zap.Logger
	repo           *repository.Container
	crypto         *CryptoService
	onChange       func()
	deploymentKeys map[string]bool
}

// NewAPIConfigService is the constructor.
func NewAPIConfigService(log *zap.Logger, repo *repository.Container, crypto *CryptoService) *APIConfigService {
	return &APIConfigService{log: log, repo: repo, crypto: crypto}
}

// SeedDefaults adds disabled providers without changing existing configurations.
func (s *APIConfigService) SeedDefaults(ctx context.Context) error {
	defaults := []model.APIConfig{
		{Provider: "tmdb", BaseURL: "https://api.themoviedb.org/3", Description: "TMDb (movies + tv)"},
		{Provider: "bangumi", BaseURL: "https://api.bgm.tv", Description: "Bangumi (anime)"},
		{Provider: "thetvdb", BaseURL: "https://api4.thetvdb.com/v4", Description: "TheTVDB (tv)"},
		{Provider: "fanart", BaseURL: "https://webservice.fanart.tv/v3", Description: "Fanart.tv (artwork)"},
		{Provider: "openai", BaseURL: "https://api.openai.com/v1", Description: "OpenAI-compatible (smart search)"},
	}
	for i := range defaults {
		var existing model.APIConfig
		err := s.repo.DB.WithContext(ctx).
			Where("provider = ?", defaults[i].Provider).
			First(&existing).Error
		if err == nil {
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := s.repo.DB.WithContext(ctx).Create(&defaults[i]).Error; err != nil {
			return err
		}
	}
	return nil
}

// PublicView is the safe-to-display projection of an API config row.
// The plaintext key is never returned — only a mask.
type PublicView struct {
	ID            string    `json:"id"`
	Provider      string    `json:"provider"`
	BaseURL       string    `json:"base_url,omitempty"`
	Extra         string    `json:"extra,omitempty"`
	Enabled       bool      `json:"enabled"`
	Description   string    `json:"description,omitempty"`
	HasKey        bool      `json:"has_key"`
	KeyFromConfig bool      `json:"key_from_config,omitempty"`
	MaskedKey     string    `json:"masked_key,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// List returns visible API configurations, with configured keys first.
func (s *APIConfigService) List(ctx context.Context) ([]PublicView, error) {
	var rows []model.APIConfig
	if err := s.repo.DB.WithContext(ctx).
		Where("provider IN ?", []string{"tmdb", "bangumi", "thetvdb", "fanart", "openai"}).
		Order("provider asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]PublicView, 0, len(rows))
	for _, r := range rows {
		view, err := s.toPublic(&r)
		if err != nil {
			return nil, err
		}
		out = append(out, *view)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return (out[i].HasKey || out[i].KeyFromConfig) && !(out[j].HasKey || out[j].KeyFromConfig)
	})
	return out, nil
}

// Get returns the public view for a single provider, or nil.
func (s *APIConfigService) Get(ctx context.Context, provider string) (*PublicView, error) {
	row, err := s.findByProvider(ctx, provider)
	if err != nil || row == nil {
		return nil, err
	}
	return s.toPublic(row)
}

// Resolve returns the decrypted key + base url ready for use by an HTTP
// client. Empty struct (with no error) when the provider is unknown or
// the API key is empty.
type Resolved struct {
	Found   bool
	APIKey  string
	BaseURL string
	Extra   string
	Enabled bool
}

// Resolve fetches the live configuration for a provider, decrypting the
// API key. Callers can use Resolved.APIKey != "" as the "configured" check.
func (s *APIConfigService) Resolve(ctx context.Context, provider string) (Resolved, error) {
	row, err := s.findByProvider(ctx, provider)
	if err != nil {
		s.log.Warn("api_config.resolve: query failed", zap.String("provider", provider), zap.Error(err))
		return Resolved{}, err
	}
	if row == nil {
		s.log.Warn("api_config.resolve: provider not found", zap.String("provider", provider))
		return Resolved{}, nil
	}
	key, err := s.crypto.Decrypt(row.APIKey)
	if err != nil {
		return Resolved{}, fmt.Errorf("读取 %s API 密钥失败: %w", provider, err)
	}
	resolved := Resolved{
		Found:   true,
		APIKey:  key,
		BaseURL: row.BaseURL,
		Extra:   row.Extra,
		Enabled: row.Enabled && s.canEnable(provider, key),
	}
	s.log.Debug("api_config.resolve: success",
		zap.String("provider", provider),
		zap.Bool("has_key", resolved.APIKey != ""),
		zap.Bool("enabled", resolved.Enabled))
	return resolved, nil
}

// APIConfigPatch updates only the fields provided. An empty key or "<clear>" wipes it.
type APIConfigPatch struct {
	APIKey      *string `json:"api_key,omitempty"`
	BaseURL     *string `json:"base_url,omitempty"`
	Extra       *string `json:"extra,omitempty"`
	Enabled     *bool   `json:"enabled,omitempty"`
	Description *string `json:"description,omitempty"`
}

// Update applies the patch and returns the new public view.
func (s *APIConfigService) Update(ctx context.Context, provider string, patch APIConfigPatch) (*PublicView, error) {
	provider = strings.TrimSpace(strings.ToLower(provider))
	if provider == "" {
		return nil, errors.New("provider required")
	}

	updates := map[string]any{}
	if patch.APIKey != nil {
		value := strings.TrimSpace(*patch.APIKey)
		if value == "" || value == "<clear>" {
			updates["api_key"] = ""
		} else {
			updates["api_key"] = s.crypto.Encrypt(value)
		}
	}
	if patch.BaseURL != nil {
		updates["base_url"] = *patch.BaseURL
	}
	if patch.Extra != nil {
		updates["extra"] = *patch.Extra
	}
	if patch.Description != nil {
		updates["description"] = *patch.Description
	}
	var view *PublicView
	err := s.repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row := model.APIConfig{Provider: provider}
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("provider = ?", provider).Session(&gorm.Session{})
		if err := query.First(&row).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "provider"}}, DoNothing: true}).Create(&row).Error; err != nil {
				return err
			}
			row = model.APIConfig{}
			if err := query.First(&row).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

		key := row.APIKey
		if replacement, ok := updates["api_key"].(string); ok {
			key = replacement
		}
		// Preserve the previous effective state unless the caller changes it explicitly.
		enabled := row.Enabled && s.canEnable(provider, row.APIKey)
		if patch.Enabled != nil {
			enabled = *patch.Enabled
			if enabled && !s.canEnable(provider, key) {
				return errors.New("请先配置 API 密钥，再启用该服务")
			}
		}
		enabled = enabled && s.canEnable(provider, key)
		if enabled != row.Enabled {
			updates["enabled"] = enabled
		}
		if len(updates) > 0 {
			if err := tx.Model(&row).Updates(updates).Error; err != nil {
				return err
			}
		}
		var err error
		view, err = s.toPublic(&row)
		return err
	})
	if err == nil && s.onChange != nil {
		s.onChange()
	}
	return view, err
}

// Delete clears a provider's API key and keeps the configuration row.
func (s *APIConfigService) Delete(ctx context.Context, provider string) error {
	updates := map[string]any{"api_key": ""}
	if !s.canEnable(provider, "") {
		updates["enabled"] = false
	}
	result := s.repo.DB.WithContext(ctx).Model(&model.APIConfig{}).
		Where("provider = ?", provider).Updates(updates)
	if result.Error == nil && result.RowsAffected > 0 && s.onChange != nil {
		s.onChange()
	}
	return result.Error
}

func (s *APIConfigService) canEnable(provider, key string) bool {
	switch provider {
	case "tmdb", "thetvdb", "fanart", "openai":
		return strings.TrimSpace(key) != "" || s.deploymentKeys[provider]
	default:
		return true
	}
}

func (s *APIConfigService) findByProvider(ctx context.Context, provider string) (*model.APIConfig, error) {
	var row model.APIConfig
	err := s.repo.DB.WithContext(ctx).Where("provider = ?", provider).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *APIConfigService) toPublic(r *model.APIConfig) (*PublicView, error) {
	plain, err := s.crypto.Decrypt(r.APIKey)
	if err != nil {
		return nil, fmt.Errorf("读取 %s API 密钥失败: %w", r.Provider, err)
	}
	pv := PublicView{
		ID:            r.ID,
		Provider:      r.Provider,
		BaseURL:       r.BaseURL,
		Extra:         r.Extra,
		Enabled:       r.Enabled && s.canEnable(r.Provider, plain),
		KeyFromConfig: s.deploymentKeys[r.Provider],
		Description:   r.Description,
		HasKey:        plain != "",
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
	if pv.HasKey {
		pv.MaskedKey = MaskAPIKey(plain)
	}
	return &pv, nil
}
