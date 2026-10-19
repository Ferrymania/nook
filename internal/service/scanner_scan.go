package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/model"

	"github.com/Ferrymania/nook/internal/config"
)

// ScanLibrary walks the library root and persists discovered media files.
func (s *ScannerService) ScanLibrary(ctx context.Context, libraryID string) (*ScanResult, error) {
	return s.scanLibrary(ctx, libraryID, true)
}

func (s *ScannerService) ScanLibraryRoot(ctx context.Context, libraryID, rootID string) (*ScanResult, error) {
	s.repo.LibraryPaths.RLock()
	defer s.repo.LibraryPaths.RUnlock()
	lib, err := s.repo.Library.FindByID(ctx, libraryID)
	if err != nil {
		return nil, err
	}
	if lib == nil {
		return nil, errors.New("library not found")
	}
	root, err := s.repo.Library.FindRootByID(ctx, libraryID, rootID)
	if err != nil {
		return nil, err
	}
	if root == nil {
		return nil, errors.New("library root not found")
	}
	return s.scanLocalLibraryRoot(ctx, lib, root, true)
}

// ScanLibraryWithoutAutoScrape walks a library without kicking off online
// metadata enrichment. Cloud mounts can contain very large trees; keeping mount
// scans import-only prevents scraper bursts from overwhelming small NAS boxes.
func (s *ScannerService) ScanLibraryWithoutAutoScrape(ctx context.Context, libraryID string) (*ScanResult, error) {
	return s.scanLibrary(ctx, libraryID, false)
}

func (s *ScannerService) TryBeginLocalScan(libraryID string) (func(), bool) {
	if s == nil || strings.TrimSpace(libraryID) == "" {
		return func() {}, true
	}
	s.localScanMu.Lock()
	if s.localScans == nil {
		s.localScans = make(map[string]struct{})
	}
	if _, ok := s.localScans[libraryID]; ok {
		s.localScanMu.Unlock()
		return nil, false
	}
	s.localScans[libraryID] = struct{}{}
	s.localScanMu.Unlock()
	return func() {
		s.localScanMu.Lock()
		delete(s.localScans, libraryID)
		s.localScanMu.Unlock()
	}, true
}

func (s *ScannerService) scanLibrary(ctx context.Context, libraryID string, autoScrape bool) (*ScanResult, error) {
	s.repo.LibraryPaths.RLock()
	defer s.repo.LibraryPaths.RUnlock()
	lib, err := s.repo.Library.FindByID(ctx, libraryID)
	if err != nil {
		return nil, err
	}
	if lib == nil {
		return nil, errors.New("library not found")
	}
	res := newScanResult(ctx, lib.ID)
	writeBatch := newLocalMediaWriteBatch(s, ctx, res, 100)

	roots, err := s.localLibraryScanRoots(ctx, lib)
	if err != nil {
		return res, err
	}
	if len(roots) == 0 {
		return res, errors.New("library has no enabled paths")
	}
	var scanErr error
	scannedRoots := 0
	for i := range roots {
		root := roots[i]
		if err := s.resolveLocalLibraryRootPath(ctx, lib, &root); err != nil {
			// 优雅降级：路径不可达时只跳过该 root，并保留该库已入库的旧媒体（下方 prune 不会执行），
			// 同时输出可操作诊断（候选路径 + 宿主机/容器映射状态），方便定位旧库在容器内扫不出媒体的原因。
			diag := describeUnresolvedLibraryPath(root.Path)
			addScanError(res, "", errors.New(diag))
			s.log.Warn("library root scan skipped; existing media preserved",
				zap.String("library_id", lib.ID),
				zap.String("root_id", root.ID),
				zap.String("path", root.Path),
				zap.Strings("candidates", mappedPathCandidates(root.Path)),
				zap.String("media_dir_env", config.Env("NOOK_MEDIA_DIR")),
				zap.String("media_container_env", envOrDefault("NOOK_MEDIA_CONTAINER_DIR", "/media")),
				zap.String("diagnostic", diag),
				zap.Error(err))
			if scanErr == nil {
				scanErr = err
			}
			continue
		}
		existingMedia, snapshotErr := s.existingLocalMediaSnapshotForRoot(ctx, lib.ID, root.ID, root.Path)
		if snapshotErr != nil {
			s.log.Warn("load existing local media snapshot failed",
				zap.String("library_id", lib.ID),
				zap.String("root_id", root.ID),
				zap.String("path", root.Path),
				zap.Error(snapshotErr))
			existingMedia = nil
		}
		seen, walkErr := s.scanLocalLibraryFiles(ctx, lib, &root, existingMedia, writeBatch, res)
		if walkErr != nil {
			addScanError(res, root.Path, walkErr)
			if scanErr == nil {
				scanErr = walkErr
			}
			continue
		}
		// Flush alias-aware upserts before pruning missing paths. This lets a
		// foo.strm <-> foo.mkv.strm rename migrate the existing row (including
		// scraped metadata) instead of deleting it as "old path missing".
		writeBatch.Flush()
		scannedRoots++
		removed, err := s.pruneMissingMediaForRoot(ctx, lib.ID, root.ID, root.Path, seen)
		if err != nil {
			s.log.Warn("prune missing media failed", zap.String("library_id", lib.ID), zap.String("root_id", root.ID), zap.Error(err))
		} else {
			res.Removed += removed
		}
	}
	writeBatch.Flush()
	if scanErr != nil && scannedRoots == 0 {
		return res, scanErr
	}

	s.finishLocalLibraryScan(ctx, lib, res, autoScrape)
	return res, nil
}

func (s *ScannerService) scanLocalLibraryRoot(ctx context.Context, lib *model.Library, root *model.LibraryRoot, autoScrape bool) (*ScanResult, error) {
	res := newScanResult(ctx, lib.ID)
	if root == nil || !root.Enabled {
		return res, errors.New("library root disabled or not found")
	}
	if err := s.resolveLocalLibraryRootPath(ctx, lib, root); err != nil {
		return res, err
	}
	writeBatch := newLocalMediaWriteBatch(s, ctx, res, 100)
	existingMedia, err := s.existingLocalMediaSnapshotForRoot(ctx, lib.ID, root.ID, root.Path)
	if err != nil {
		s.log.Warn("load existing local media snapshot failed",
			zap.String("library_id", lib.ID),
			zap.String("root_id", root.ID),
			zap.String("path", root.Path),
			zap.Error(err))
		existingMedia = nil
	}
	seen, walkErr := s.scanLocalLibraryFiles(ctx, lib, root, existingMedia, writeBatch, res)
	writeBatch.Flush()
	if walkErr != nil {
		addScanError(res, root.Path, walkErr)
		return res, walkErr
	}
	removed, err := s.pruneMissingMediaForRoot(ctx, lib.ID, root.ID, root.Path, seen)
	if err != nil {
		s.log.Warn("prune missing media failed", zap.String("library_id", lib.ID), zap.String("root_id", root.ID), zap.Error(err))
	} else {
		res.Removed = removed
	}
	s.finishLocalLibraryScan(ctx, lib, res, autoScrape)
	return res, nil
}

func (s *ScannerService) scanLocalLibraryFiles(ctx context.Context, lib *model.Library, root *model.LibraryRoot, existingMedia map[string]existingLocalMedia, writeBatch *localMediaWriteBatch, res *ScanResult) (map[string]struct{}, error) {
	seen := make(map[string]struct{})
	seenInodes := existingLocalMediaFileIDs(existingMedia)
	walkFn := func(path string, info walkInfo) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if info.isDir {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if _, ok := videoExtensions[ext]; !ok {
			return nil
		}
		seen[filepath.Clean(path)] = struct{}{}
		s.ingestFile(ctx, lib, root, path, info.size, seenInodes, existingMedia, writeBatch, res)
		return nil
	}
	return seen, walk(root.Path, walkFn)
}

func existingLocalMediaFileIDs(existingMedia map[string]existingLocalMedia) map[string]string {
	seenInodes := make(map[string]string)
	for path, existing := range existingMedia {
		if existing.FileID != "" {
			seenInodes[existing.FileID] = path
		}
	}
	return seenInodes
}

func (s *ScannerService) finishLocalLibraryScan(ctx context.Context, lib *model.Library, res *ScanResult, autoScrape bool) {
	s.hub.Publish("scan", map[string]any{
		"library_id":  lib.ID,
		"finished":    true,
		"visited":     res.Visited,
		"added":       res.Added,
		"updated":     res.Updated,
		"probed":      res.Probed,
		"local_meta":  res.LocalMetadata,
		"removed":     res.Removed,
		"error_count": res.ErrorCount,
		"errors":      res.Errors,
	})
	s.invalidateMediaCache(ctx)

	if scanHasImportChanges(res) && autoScrape && s.scraper != nil && s.scraper.AnyEnabled() && s.autoScrapeEnabled(ctx, lib) {
		s.startAutoScrape(ctx, lib.ID)
	}
}

// IngestPath ingests a single file into the given library without walking the
// whole tree. Used by the watcher for incremental, event-driven additions so
// adding one new file no longer triggers a full library re-scan (减少硬盘损耗).
// Non-video files and directories are ignored. Returns true if a media row was
// added or updated.
func (s *ScannerService) IngestPath(ctx context.Context, libraryID, path string) (bool, error) {
	s.repo.LibraryPaths.RLock()
	defer s.repo.LibraryPaths.RUnlock()
	lib, err := s.repo.Library.FindByID(ctx, libraryID)
	if err != nil {
		return false, err
	}
	if lib == nil {
		return false, errors.New("library not found")
	}
	root, err := s.localLibraryRootForPath(ctx, lib, path)
	if err != nil || root == nil {
		return false, err
	}
	if err := s.resolveLocalLibraryRootPath(ctx, lib, root); err != nil {
		return false, err
	}
	// A debounced watcher event can outlive a root change. Never re-import
	// an old directory through localLibraryRootForPath's compatibility fallback.
	if !pathBelongsToRoot(path, root.Path) {
		return false, nil
	}
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return false, err
	}
	ext := strings.ToLower(filepath.Ext(path))
	if _, ok := videoExtensions[ext]; !ok {
		return false, nil
	}
	res := &ScanResult{LibraryID: lib.ID}
	s.ingestFile(ctx, lib, root, path, fi.Size(), make(map[string]string), nil, nil, res)
	if res.Added+res.Updated > 0 {
		s.invalidateMediaCache(ctx)
		if s.scraper != nil && s.autoScrapeEnabled(ctx, lib) {
			var media model.Media
			if err := s.repo.DB.WithContext(ctx).Select("id", "scrape_status").Where("library_id = ? AND path = ?", lib.ID, path).First(&media).Error; err == nil && (media.ScrapeStatus == "pending" || media.ScrapeStatus == "") {
				if _, err := s.scraper.EnqueueMedia(ctx, media.ID, skipEpisodeArtworkOptions(false)); err != nil {
					s.log.Warn("queue scrape after incremental ingest failed", zap.String("media_id", media.ID), zap.Error(err))
				}
			}
		}
	}
	return res.Added+res.Updated > 0, nil
}

func (s *ScannerService) resolveLocalLibraryPath(ctx context.Context, lib *model.Library) error {
	if lib == nil || strings.TrimSpace(lib.Path) == "" {
		return nil
	}
	resolved, err := resolveAccessibleLibraryPath(lib.Path)
	if err != nil {
		return err
	}
	if sameLibraryPath(resolved, lib.Path) {
		lib.Path = filepath.Clean(lib.Path)
		return nil
	}
	if s.repo != nil && s.repo.DB != nil {
		if updateErr := s.repo.DB.WithContext(ctx).Model(&model.Library{}).Where("id = ?", lib.ID).Update("path", resolved).Error; updateErr != nil && s.log != nil {
			s.log.Warn("update mapped library path failed",
				zap.String("library_id", lib.ID),
				zap.String("from", lib.Path),
				zap.String("to", resolved),
				zap.Error(updateErr))
		}
	}
	if s.log != nil {
		s.log.Info("mapped library path for scan",
			zap.String("library_id", lib.ID),
			zap.String("from", lib.Path),
			zap.String("to", resolved))
	}
	lib.Path = resolved
	return nil
}
