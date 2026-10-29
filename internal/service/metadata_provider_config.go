package service

import (
	"context"
	"strings"
)

// The persisted global switch wins even when a key also exists in config.yaml.
// Empty DB credentials retain config-file compatibility; anonymous sources
// remain usable without credentials, but still obey the global switch.
func resolveMetadataProvider(ctx context.Context, store *APIConfigService, provider, key, base string) Resolved {
	out := Resolved{APIKey: strings.TrimSpace(key), BaseURL: strings.TrimRight(base, "/"), Enabled: true}
	if store == nil {
		return out
	}
	saved, err := store.Resolve(ctx, provider)
	if err != nil {
		return Resolved{}
	}
	if !saved.Found {
		return out
	}
	out.Enabled = saved.Enabled
	if saved.APIKey != "" {
		out.APIKey = saved.APIKey
	}
	if saved.BaseURL != "" {
		out.BaseURL = strings.TrimRight(saved.BaseURL, "/")
	}
	return out
}
func (t *TMDbProvider) runtimeConfig(ctx context.Context) Resolved {
	return resolveMetadataProvider(ctx, t.apiConfig, "tmdb", t.cfg.Secrets.TMDbAPIKey, t.base)
}
func (b *BangumiProvider) runtimeConfig(ctx context.Context) Resolved {
	return resolveMetadataProvider(ctx, b.apiConfig, "bangumi", b.cfg.Secrets.BangumiToken, b.base)
}
func (t *TheTVDBProvider) runtimeConfig(ctx context.Context) Resolved {
	return resolveMetadataProvider(ctx, t.apiConfig, "thetvdb", t.cfg.Secrets.TheTVDBAPIKey, "https://api4.thetvdb.com/v4")
}
func (f *FanartProvider) runtimeConfig(ctx context.Context) Resolved {
	return resolveMetadataProvider(ctx, f.apiConfig, "fanart", f.cfg.Secrets.FanartAPIKey, "https://webservice.fanart.tv/v3")
}
