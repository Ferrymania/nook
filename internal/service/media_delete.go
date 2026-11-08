package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/Ferrymania/nook/internal/model"
)

// DeleteMedia removes a media row from the library index.
// When deleteFiles is true and the path is a local filesystem file, the media
// file and its sidecar NFO are also removed. cloud:// paths are never touched on disk.
func (s *MediaService) DeleteMedia(ctx context.Context, id string, deleteFiles bool) error {
	return s.deleteMedia(ctx, id, deleteFiles, "")
}

func (s *MediaService) deleteMedia(ctx context.Context, id string, deleteFiles bool, expectedPath string) error {
	var media model.Media
	err := s.repo.DB.WithContext(ctx).Where("id = ?", id).First(&media).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}

	if expectedPath != "" && media.Path != expectedPath {
		return errors.New("媒体路径已变更")
	}

	if deleteFiles {
		if expectedPath != "" {
			err = deleteRemoteMediaLocalFiles(media.Path)
		} else {
			err = deleteLocalMediaFiles(s.log, media.Path)
		}
		if err != nil {
			return err
		}
	}

	err = s.repo.DB.WithContext(ctx).Unscoped().Where("id = ?", id).Delete(&model.Media{}).Error
	if err == nil {
		s.invalidateMediaCache(ctx)
	}
	return err
}

func deleteRemoteMediaLocalFiles(path string) error {
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		return err
	}
	stem := mediaSidecarBase(path)
	for _, entry := range entries {
		_, video := videoExtensions[strings.ToLower(filepath.Ext(entry.Name()))]
		if video && !entry.IsDir() && entry.Name() != filepath.Base(path) &&
			strings.EqualFold(mediaSidecarBase(entry.Name()), stem) {
			return os.Remove(path)
		}
	}
	return removeMediaAndNFO(path)
}

func deleteLocalMediaFiles(log *zap.Logger, path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	if strings.HasPrefix(strings.ToLower(path), "cloud://") {
		if log != nil {
			log.Info("skip local file delete for cloud media path", zap.String("path", path))
		}
		return nil
	}
	if err := removeMediaAndNFO(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return nil
}
