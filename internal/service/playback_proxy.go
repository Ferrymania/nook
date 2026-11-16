package service

import (
	"context"

	"github.com/Ferrymania/nook/internal/repository"
)

const PlaybackProxyRemoteSettingKey = "playback.proxy_remote"

// RemotePlaybackProxyEnabled is read for each new playback request so saving
// the setting does not interrupt streams that are already running.
func RemotePlaybackProxyEnabled(ctx context.Context, repo *repository.Container) bool {
	if repo == nil || repo.Setting == nil {
		return false
	}
	value, err := repo.Setting.Get(ctx, PlaybackProxyRemoteSettingKey)
	return err == nil && parseBoolSetting(value, false)
}
