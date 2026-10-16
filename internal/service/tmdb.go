// Package service — TMDb metadata provider.
//
// TMDbProvider implements the (minimal) MetadataProvider interface and uses
// the public The Movie Database REST API. The API key is taken from
// secrets.tmdb_api_key; when empty the provider returns nil from every
// method so the scraper can no-op gracefully.
//
// We only call the two endpoints the scrape pipeline actually needs:
//
//	GET /search/movie?query=...&year=...
//	GET /movie/{id}?language=zh-CN
//
// TV / anime support follows the same pattern; for the bootstrap we expose
// a single SearchMovie path so that the home page and library gallery can
// show real posters as soon as a TMDb key is configured.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/config"
)

// TMDbProvider talks to https://api.themoviedb.org/3.
type TMDbProvider struct {
	cfg       *config.Config
	log       *zap.Logger
	client    *http.Client
	base      string
	imgCDN    string
	apiConfig *APIConfigService
}

// NewTMDbProvider is the constructor. APIBase / image CDN can be overridden
// via secrets.tmdb_api_proxy + tmdb_image_proxy for users behind GFW.
// apiConfig is optional; when non-nil, the provider will also check the
// api_configs table for TMDB API key.
func NewTMDbProvider(cfg *config.Config, log *zap.Logger, apiConfig *APIConfigService) *TMDbProvider {
	base := cfg.Secrets.TMDbAPIProxy
	if base == "" {
		base = "https://api.themoviedb.org/3"
	}
	img := cfg.Secrets.TMDbImageProxy
	if img == "" {
		img = "https://image.tmdb.org/t/p"
	}
	return &TMDbProvider{
		cfg:       cfg,
		log:       log,
		apiConfig: apiConfig,
		base:      base,
		imgCDN:    img,
		// 默认 8s 超时：首页同时调 trending + popular，15s 太久会让用户感觉
		// 卡死。如果 TMDb 真有问题，handler 层会快速降级返回空列表。
		// 同时让 client 读取 HTTP(S)_PROXY 与 Windows 本机系统代理。
		client: NewExternalHTTPClient(8 * time.Second),
	}
}

// Enabled reports whether the operator has supplied an API key.
// It checks both the config file and the database (via apiConfig).
func (t *TMDbProvider) Enabled() bool {
	cfg := t.runtimeConfig(context.Background())
	return cfg.Enabled && cfg.APIKey != ""
}

func (t *TMDbProvider) resolveAPIKey(ctx context.Context) string {
	cfg := t.runtimeConfig(ctx)
	if !cfg.Enabled {
		return ""
	}
	return cfg.APIKey
}

func (t *TMDbProvider) resolveBaseURL(ctx context.Context) string {
	return t.runtimeConfig(ctx).BaseURL
}

func (t *TMDbProvider) getJSON(ctx context.Context, url string, out any) error {
	if !t.runtimeConfig(ctx).Enabled {
		return fmt.Errorf("TMDb 已在系统设置中关闭")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("tmdb %s: %d", url, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
