package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/Ferrymania/nook/internal/model"
	"github.com/Ferrymania/nook/internal/repository"
)

var scrapeProviderNames = map[string]string{"tmdb": "TMDb", "douban": "豆瓣", "bangumi": "Bangumi", "thetvdb": "TheTVDB", "fanart": "Fanart"}
var defaultScrapeProviders = []string{"tmdb", "douban", "bangumi", "thetvdb", "fanart"}

// The former global value is only a compatibility default for libraries that
// have not saved the new preference. Explicit library values always win.
const legacySegmentPrewarmEnabledKey = "segment.prewarm_enabled"

func librarySegmentPrewarmEnabled(ctx context.Context, repo *repository.Container, lib *model.Library) (bool, error) {
	if lib != nil && lib.ScrapeSettings != nil && lib.ScrapeSettings.SegmentPrewarmEnabled != nil {
		return *lib.ScrapeSettings.SegmentPrewarmEnabled, nil
	}
	value, err := repo.Setting.Get(ctx, legacySegmentPrewarmEnabledKey)
	return parseBoolSetting(value, false), err
}

// Legacy settings are read only for libraries that have never saved their own
// preferences. Library-specific preferences never rewrite legacy defaults.
func libraryScrapeSettings(ctx context.Context, repo *repository.Container, lib *model.Library) model.LibraryScrapeSettings {
	if lib != nil && lib.ScrapeSettings != nil {
		return *lib.ScrapeSettings
	}
	out := model.LibraryScrapeSettings{Providers: append([]string(nil), defaultScrapeProviders...), Language: "zh-CN", DelayMinMS: 250, DelayMaxMS: 500}
	if repo == nil || repo.Setting == nil {
		return out
	}
	rows, err := repo.Setting.All(ctx)
	if err != nil {
		return out
	}
	values := map[string]string{}
	for _, row := range rows {
		values[row.Key] = strings.TrimSpace(row.Value)
	}
	out.AutoOnScan = parseBoolSetting(values["scrape.auto_on_scan"], false)
	if v := firstText(values["scrape.language"], values["tmdb.language"]); validScrapeLanguage(v) {
		out.Language = v
	}
	if v := values["scrape.providers"]; v != "" {
		var providers []string
		seen := map[string]bool{}
		for _, p := range strings.Split(v, ",") {
			p = strings.ToLower(strings.TrimSpace(p))
			if scrapeProviderNames[p] != "" && !seen[p] {
				providers = append(providers, p)
				seen[p] = true
			}
		}
		if len(providers) > 0 {
			out.Providers = providers
		}
	}
	out.DelayMinMS = max(0, min(maxScrapeDelayMS, parseIntSettingDefault(values["scrape.delay_min_ms"], 250)))
	out.DelayMaxMS = max(out.DelayMinMS, min(maxScrapeDelayMS, parseIntSettingDefault(values["scrape.delay_max_ms"], 500)))
	return out
}

type libraryScrapeContextKey struct{}

func withLibraryScrapeSettings(ctx context.Context, settings model.LibraryScrapeSettings) context.Context {
	return context.WithValue(ctx, libraryScrapeContextKey{}, settings)
}
func tmdbMetadataLanguage(ctx context.Context) string {
	if settings, ok := ctx.Value(libraryScrapeContextKey{}).(model.LibraryScrapeSettings); ok && validScrapeLanguage(settings.Language) {
		return settings.Language
	}
	return "zh-CN"
}
func chineseMetadataLanguage(ctx context.Context) bool {
	return strings.HasPrefix(tmdbMetadataLanguage(ctx), "zh")
}
func scrapeProviders(ctx context.Context) []string {
	if settings, ok := ctx.Value(libraryScrapeContextKey{}).(model.LibraryScrapeSettings); ok {
		return settings.Providers
	}
	return defaultScrapeProviders
}
func scrapeProviderSelected(ctx context.Context, provider string) bool {
	for _, p := range scrapeProviders(ctx) {
		if p == provider {
			return true
		}
	}
	return false
}
func validScrapeLanguage(v string) bool {
	return v == "zh-CN" || v == "zh-TW" || v == "en-US" || v == "ja-JP"
}
func validateLibraryScrapeSettings(settings model.LibraryScrapeSettings) error {
	if !validScrapeLanguage(settings.Language) {
		return errors.New("请选择支持的 TMDb 元数据语言")
	}
	if settings.DelayMinMS < 0 || settings.DelayMaxMS < settings.DelayMinMS || settings.DelayMaxMS > maxScrapeDelayMS {
		return errors.New("刮削间隔须为 0–300000 毫秒，最大间隔不能小于最小间隔")
	}
	seen := map[string]bool{}
	metadata := false
	for _, p := range settings.Providers {
		if scrapeProviderNames[p] == "" || seen[p] {
			return errors.New("刮削源无效或重复")
		}
		seen[p] = true
		metadata = metadata || p != "fanart"
	}
	if !metadata {
		return errors.New("请至少选择一个元数据来源；Fanart 仅补充图片")
	}
	return nil
}

type ScrapeProviderAvailability struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Status    string `json:"status"`
}
type LibraryScrapeSettingsView struct {
	EpisodeImages bool                         `json:"episode_images"`
	Settings      model.LibraryScrapeSettings  `json:"settings"`
	AutoProbe     bool                         `json:"auto_probe"`
	Providers     []ScrapeProviderAvailability `json:"providers"`
}

func (s *ScraperService) GetLibraryScrapeSettings(ctx context.Context, id string) (*LibraryScrapeSettingsView, error) {
	lib, err := s.repo.Library.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if lib == nil {
		return nil, errors.New("媒体库不存在")
	}
	return s.libraryScrapeSettingsView(ctx, lib)
}

func (s *ScraperService) DefaultLibraryScrapeSettings(ctx context.Context) (*LibraryScrapeSettingsView, error) {
	return s.libraryScrapeSettingsView(ctx, &model.Library{})
}

func (s *ScraperService) libraryScrapeSettingsView(ctx context.Context, lib *model.Library) (*LibraryScrapeSettingsView, error) {
	episodeImages, err := s.repo.Setting.Get(ctx, "scrape.episode_images")
	if err != nil {
		return nil, err
	}
	view := &LibraryScrapeSettingsView{EpisodeImages: parseBoolSetting(episodeImages, false), Settings: libraryScrapeSettings(ctx, s.repo, lib), AutoProbe: lib.AutoProbe}
	prewarmEnabled, err := librarySegmentPrewarmEnabled(ctx, s.repo, lib)
	if err != nil {
		return nil, err
	}
	view.Settings.SegmentPrewarmEnabled = &prewarmEnabled
	for _, p := range defaultScrapeProviders {
		cfg := s.scrapeProviderConfig(ctx, p)
		optional := p == "bangumi" || p == "douban"
		state := ScrapeProviderAvailability{ID: p, Name: scrapeProviderNames[p], Available: cfg.Enabled && (optional || cfg.APIKey != ""), Status: "已配置"}
		switch {
		case !cfg.Enabled:
			state.Status = "全局已关闭"
		case cfg.APIKey == "" && optional:
			state.Status = "可匿名使用"
		case cfg.APIKey == "":
			state.Status = "未配置 API"
		}
		view.Providers = append(view.Providers, state)
	}
	return view, nil
}
func (s *ScraperService) SaveLibraryScrapeSettings(ctx context.Context, id string, settings model.LibraryScrapeSettings, autoProbe bool, episodeImages *bool) (*LibraryScrapeSettingsView, error) {
	if err := validateLibraryScrapeSettings(settings); err != nil {
		return nil, err
	}
	current, err := s.GetLibraryScrapeSettings(ctx, id)
	if err != nil {
		return nil, err
	}
	// Older clients omit this new field; retain the effective preference instead
	// of resetting it when they save unrelated scrape settings.
	if settings.SegmentPrewarmEnabled == nil {
		settings.SegmentPrewarmEnabled = current.Settings.SegmentPrewarmEnabled
	}
	data, err := json.Marshal(settings)
	if err != nil {
		return nil, err
	}
	if err = s.repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Library{}).Where("id = ?", id).Updates(map[string]any{"scrape_settings": string(data), "auto_probe": autoProbe}).Error; err != nil {
			return err
		}
		if episodeImages != nil {
			return repository.New(tx).Setting.Set(ctx, "scrape.episode_images", strconv.FormatBool(*episodeImages))
		}
		return nil
	}); err != nil {
		return nil, err
	}
	s.invalidateMediaCache(ctx)
	return s.GetLibraryScrapeSettings(ctx, id)
}
func (s *ScraperService) scrapeProviderConfig(ctx context.Context, p string) Resolved {
	switch p {
	case "tmdb":
		if s.tmdb != nil {
			return s.tmdb.runtimeConfig(ctx)
		}
	case "bangumi":
		if s.bangumi != nil {
			return s.bangumi.runtimeConfig(ctx)
		}
	case "douban":
		if s.douban != nil {
			return s.douban.runtimeConfig(ctx)
		}
	case "thetvdb":
		if s.thetvdb != nil {
			return s.thetvdb.runtimeConfig(ctx)
		}
	case "fanart":
		if s.fanart != nil {
			return s.fanart.runtimeConfig(ctx)
		}
	}
	return Resolved{}
}
func (s *ScraperService) onlineScrapeAvailable(ctx context.Context) bool {
	for _, p := range scrapeProviders(ctx) {
		if p == "fanart" {
			continue
		}
		cfg := s.scrapeProviderConfig(ctx, p)
		if cfg.Enabled && (cfg.APIKey != "" || p == "bangumi" || p == "douban") {
			return true
		}
	}
	return false
}

// Share pacing across queue workers and synchronous scan/organize scrapes.
// This spaces task starts without introducing a second task queue.
func (s *ScraperService) waitScrapeTurn(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		s.paceMu.Lock()
		wait := time.Until(s.nextScrapeAt)
		if wait <= 0 {
			s.nextScrapeAt = time.Now().Add(s.scrapeDelay(ctx))
			s.paceMu.Unlock()
			return nil
		}
		s.paceMu.Unlock()
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

var errNoScrapeProvider = fmt.Errorf("本库选中的刮削源均不可用，请在系统设置中启用服务或配置 API")
