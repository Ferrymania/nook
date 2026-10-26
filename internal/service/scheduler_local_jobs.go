package service

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
)

// jobScanLibraries re-walks every enabled library.
//
// 默认关闭：文件变更由 WatcherService 增量入库，无需周期性全量重扫。
// 仅当用户在设置中显式开启 scan.periodic_enabled 时才执行整库重扫，
// 避免对硬盘的高频反复读取造成损伤（用户明确要求）。
func (s *SchedulerService) jobScanLibraries(ctx context.Context) error {
	manual, _ := ctx.Value(schedulerManualRunKey{}).(bool)
	now := s.currentTime()
	if !manual && !s.periodicScanDue(ctx, now) {
		return nil
	}
	libs, err := s.repo.Library.List(ctx)
	if err != nil {
		return err
	}
	for _, l := range libs {
		if !l.Enabled {
			continue
		}
		if _, err := s.scanner.ScanLibrary(ctx, l.ID); err != nil {
			s.log.Warn("scheduled scan failed",
				zap.String("library", l.ID), zap.Error(err))
		}
	}
	if !manual {
		_ = s.markPeriodicScanCompleted(ctx, now)
	}
	return nil
}

// periodicScanEnabled reports whether the operator opted into periodic full
// library re-scans. Defaults to false so the incremental watcher is the only
// thing touching the disk under normal operation.
func (s *SchedulerService) periodicScanEnabled(ctx context.Context) bool {
	if s.repo == nil || s.repo.Setting == nil {
		return false
	}
	v, err := s.repo.Setting.Get(ctx, "scan.periodic_enabled")
	if err != nil {
		return false
	}
	return parseBoolSetting(v, false)
}

func (s *SchedulerService) periodicScanDue(ctx context.Context, now time.Time) bool {
	if !s.periodicScanEnabled(ctx) {
		return false
	}
	if s.repo == nil || s.repo.Setting == nil {
		return true
	}
	last, err := s.repo.Setting.Get(ctx, localLastPeriodicScanDateKey)
	if err != nil {
		return true
	}
	return strings.TrimSpace(last) != now.In(time.Local).Format("2006-01-02")
}

func (s *SchedulerService) markPeriodicScanCompleted(ctx context.Context, now time.Time) error {
	if s.repo == nil || s.repo.Setting == nil {
		return nil
	}
	return s.repo.Setting.Set(ctx, localLastPeriodicScanDateKey, now.In(time.Local).Format("2006-01-02"))
}

// jobCleanTranscodeCache deletes HLS artefacts older than 24h.
func (s *SchedulerService) jobCleanTranscodeCache(ctx context.Context) error {
	if s.cacheDir == "" {
		return nil
	}
	cutoff := time.Now().Add(-24 * time.Hour)
	return walkAndPrune(s.cacheDir+"/hls", cutoff)
}

// jobCleanImageCache prunes the image proxy cache: 原图按保留时长与独立配额
// 优先淘汰，总量超限时再淘汰派生成品。
func (s *SchedulerService) jobCleanImageCache(ctx context.Context) error {
	if s.cacheDir == "" {
		return nil
	}
	policy := s.imageCachePolicy()
	if policy.TotalBytes <= 0 && policy.OriginalsBytes <= 0 && policy.OriginalsAge <= 0 {
		return nil
	}
	imagesDir := filepath.Join(s.cacheDir, "images")
	pools := ImageCachePools(imagesDir, policy.OriginalsBytes, policy.OriginalsAge)
	res, err := PruneImageCachePools(pools, policy.TotalBytes)
	if err != nil {
		if s.log != nil {
			s.log.Warn("scheduled image cache cleanup failed", zap.Error(err))
		}
		return err
	}
	if res.DeletedFiles > 0 && s.log != nil {
		s.log.Info("scheduled image cache cleanup completed",
			zap.Int("deleted_files", res.DeletedFiles),
			zap.Int64("freed_bytes", res.FreedBytes),
			zap.Int64("remaining_bytes", res.RemainingBytes),
		)
	}
	return nil
}
