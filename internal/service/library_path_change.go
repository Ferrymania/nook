package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gorm.io/gorm"

	"github.com/Ferrymania/nook/internal/model"
)

const (
	LibraryPathMigrate = "migrate"
	LibraryPathReplace = "replace"
)

// PathChangeError is a reviewable validation failure, rather than a server error.
type PathChangeError struct{ Message string }

func (e *PathChangeError) Error() string { return e.Message }

func pathChangeError(message string) error { return &PathChangeError{Message: message} }

type LibraryPathIssue struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type LibraryPathPreview struct {
	OldPath      string             `json:"old_path"`
	NewPath      string             `json:"new_path"`
	ChangeMode   string             `json:"change_mode"`
	Total        int                `json:"total"`
	Matched      int                `json:"matched"`
	Missing      int                `json:"missing"`
	Conflicts    int                `json:"conflicts"`
	Issues       []LibraryPathIssue `json:"issues"`
	CanApply     bool               `json:"can_apply"`
	PreviewToken string             `json:"preview_token"`
}

type libraryPathMove struct {
	Media model.Media
	Path  string
	Rel   string
}

type libraryPathPlan struct {
	LibraryPathPreview
	root  model.LibraryRoot
	rows  []model.Media // Includes STRM rename tombstones, so replacement cannot adopt them.
	moves []libraryPathMove
}

// PreviewLibraryRootPath only reads the filesystem and database. No directories,
// files, or database records are created or modified by this endpoint.
func (s *MediaService) PreviewLibraryRootPath(ctx context.Context, libraryID, rootID string, input LibraryRootInput) (*LibraryPathPreview, error) {
	s.repo.LibraryPaths.RLock()
	defer s.repo.LibraryPaths.RUnlock()
	plan, err := planLibraryPathChange(s.repo.DB.WithContext(ctx), libraryID, rootID, input)
	if err != nil {
		return nil, err
	}
	return &plan.LibraryPathPreview, nil
}

func planLibraryPathChange(db *gorm.DB, libraryID, rootID string, input LibraryRootInput) (*libraryPathPlan, error) {
	if input.ChangeMode != LibraryPathMigrate && input.ChangeMode != LibraryPathReplace {
		return nil, pathChangeError("请选择「迁移原有内容」或「改用新目录」，检查后再保存")
	}
	var root model.LibraryRoot
	if err := db.Where("id = ? AND library_id = ?", rootID, libraryID).First(&root).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, pathChangeError("媒体库路径已不存在，请刷新页面")
		}
		return nil, err
	}
	// Deliberately validate exactly the selected path. A migration must not
	// silently choose a different directory using legacy path-mapping heuristics.
	newPath, err := filepath.Abs(strings.TrimSpace(input.Path))
	if strings.TrimSpace(input.Path) == "" || err != nil {
		return nil, pathChangeError("请填写有效的新目录")
	}
	info, err := os.Stat(newPath)
	if err != nil || !info.IsDir() {
		return nil, pathChangeError("新目录不存在或无法访问，请先配置挂载并选择 Nook 可访问的目录")
	}
	// Opening and reading one entry also detects unreadable mounts; an empty
	// directory is valid for replacement, but missing files block migration.
	dir, err := os.Open(newPath)
	if err != nil {
		return nil, pathChangeError("无法读取新目录，请检查权限")
	}
	_, readErr := dir.ReadDir(1)
	_ = dir.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return nil, pathChangeError("无法读取新目录，请检查权限")
	}
	p := &libraryPathPlan{root: root, LibraryPathPreview: LibraryPathPreview{
		OldPath: root.Path, NewPath: newPath, ChangeMode: input.ChangeMode, Issues: []LibraryPathIssue{},
	}}
	var roots []model.LibraryRoot
	if err := db.Order("id").Find(&roots).Error; err != nil {
		return nil, err
	}
	for _, other := range roots {
		if other.ID != root.ID && pathsOverlap(newPath, other.Path) {
			p.addIssue(newPath, "与已有媒体库来源目录重叠，请选择独立目录", false)
		}
	}
	var rows []model.Media
	if err := db.Unscoped().Where("library_id = ?", libraryID).
		Select("id", "library_id", "library_root_id", "path", "relative_path", "size_bytes", "strm_url", "series_id", "deleted_at").
		Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, media := range rows {
		if media.LibraryRootID == rootID || (media.LibraryRootID == "" && legacyMediaRoot(media.Path, roots, libraryID) == rootID) {
			p.rows = append(p.rows, media)
			if !media.DeletedAt.Valid {
				p.Total++
			}
		}
	}
	// The token binds confirmation to the root, exact affected records, mode,
	// and checked target file sizes/mtimes. Saving rebuilds this plan in a transaction.
	h := sha256.New()
	_ = json.NewEncoder(h).Encode([]any{root, newPath, input.ChangeMode, p.rows, roots})
	for _, row := range p.rows {
		_ = json.NewEncoder(h).Encode(row.DeletedAt)
	}
	if input.ChangeMode == LibraryPathReplace {
		// Scanning upserts by globally unique absolute path. Prevent an
		// inconsistent legacy row outside the selected source from being adopted
		// by the replacement scan, even when no root overlaps were found above.
		var indexed []model.Media
		if err := db.Unscoped().Select("id", "path").Where("path LIKE ?", newPath+string(filepath.Separator)+"%").Find(&indexed).Error; err != nil {
			return nil, err
		}
		owned := make(map[string]bool, len(p.rows))
		for _, row := range p.rows {
			owned[row.ID] = true
		}
		for _, row := range indexed {
			if !owned[row.ID] && pathBelongsToRoot(row.Path, newPath) {
				p.addIssue(row.Path, "新目录中已有属于其他来源的媒体记录", false)
			}
		}
	}
	if input.ChangeMode == LibraryPathMigrate {
		targets := make(map[string]bool)
		for _, media := range p.rows {
			if err := db.Statement.Context.Err(); err != nil {
				return nil, err
			}
			if media.DeletedAt.Valid {
				continue
			}
			rel, ok := relativeMediaPath(media, root.Path)
			if !ok {
				p.addIssue(media.Path, "无法确定文件在原目录中的相对路径", false)
				continue
			}
			target := filepath.Join(newPath, filepath.FromSlash(rel))
			if targets[target] {
				p.addIssue(rel, "多条旧记录指向同一个新路径", false)
				continue
			}
			targets[target] = true
			fi, statErr := os.Stat(target)
			if statErr != nil {
				p.addIssue(rel, "新目录中找不到文件或文件不可读", true)
				continue
			}
			_ = json.NewEncoder(h).Encode([]any{target, fi.Size(), fi.ModTime().UnixNano()})
			if !fi.Mode().IsRegular() {
				p.addIssue(rel, "目标不是普通文件", false)
				continue
			}
			if reason := validateMigratedFile(media, target, fi); reason != "" {
				p.addIssue(rel, reason, false)
				continue
			}
			p.moves = append(p.moves, libraryPathMove{Media: media, Path: target, Rel: rel})
		}
		// Path uniqueness is global (including soft-deleted rows). Check in
		// batches rather than issuing a query for every file on a network mount.
		for start := 0; start < len(p.moves); start += 500 {
			end := min(start+500, len(p.moves))
			paths := make([]string, 0, end-start)
			for _, move := range p.moves[start:end] {
				paths = append(paths, move.Path)
			}
			var occupied []model.Media
			if err := db.Unscoped().Select("id", "path").Where("path IN ?", paths).Find(&occupied).Error; err != nil {
				return nil, err
			}
			owners := make(map[string]string)
			for _, row := range occupied {
				owners[row.Path] = row.ID
			}
			for _, move := range p.moves[start:end] {
				if owner := owners[move.Path]; owner != "" && owner != move.Media.ID {
					p.addIssue(move.Rel, "新路径已经被另一条媒体记录使用", false)
				} else {
					p.Matched++
				}
			}
		}
	}
	p.CanApply = p.Missing == 0 && p.Conflicts == 0
	p.PreviewToken = hex.EncodeToString(h.Sum(nil))
	return p, nil
}

func (p *libraryPathPlan) addIssue(pathValue, reason string, missing bool) {
	if missing {
		p.Missing++
	} else {
		p.Conflicts++
	}
	if len(p.Issues) < 30 {
		p.Issues = append(p.Issues, LibraryPathIssue{Path: pathValue, Reason: reason})
	}
}

func pathsOverlap(a, b string) bool {
	// Resolve accessible symlinks too, so a second name for an existing root
	// cannot silently adopt its indexed media on the subsequent scan.
	if resolved, err := filepath.EvalSymlinks(a); err == nil {
		a = resolved
	}
	if resolved, err := filepath.EvalSymlinks(b); err == nil {
		b = resolved
	}
	return pathBelongsToRoot(a, b) || pathBelongsToRoot(b, a)
}

func legacyMediaRoot(mediaPath string, roots []model.LibraryRoot, libraryID string) string {
	best, length := "", 0
	for _, root := range roots {
		if root.LibraryID != libraryID {
			continue
		}
		if _, ok := relativeMediaPath(model.Media{Path: mediaPath}, root.Path); ok && len(root.Path) > length {
			best, length = root.ID, len(root.Path)
		}
	}
	return best
}

func relativeMediaPath(media model.Media, oldRoot string) (string, bool) {
	// Slash-based parsing also handles a Windows binary's saved paths when
	// the database is subsequently opened by a Linux Docker deployment.
	clean := func(value string) string { return path.Clean(strings.ReplaceAll(value, "\\", "/")) }
	root := strings.TrimSuffix(clean(oldRoot), "/") + "/"
	oldPath := clean(media.Path)
	rel := clean(media.RelativePath)
	if media.RelativePath == "" {
		if !strings.HasPrefix(oldPath, root) {
			return "", false
		}
		rel = strings.TrimPrefix(oldPath, root)
	} else if strings.HasPrefix(oldPath, root) && strings.TrimPrefix(oldPath, root) != rel {
		return "", false
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") || path.IsAbs(rel) || strings.Contains(rel, ":") {
		return "", false
	}
	return rel, true
}

func validateMigratedFile(media model.Media, target string, info os.FileInfo) string {
	if strings.EqualFold(filepath.Ext(target), ".strm") {
		newURL, err := readLocalSTRMTarget(target)
		oldURL := strings.TrimSpace(media.STRMURL)
		if oldURL == "" {
			oldURL, _ = readLocalSTRMTarget(media.Path)
		}
		if err != nil || newURL == "" || oldURL == "" || newURL != oldURL {
			return "STRM 播放地址不一致或缺少原地址，无法保留为同一个媒体"
		}
		return ""
	}
	expected := media.SizeBytes
	if old, err := os.Stat(media.Path); err == nil && old.Mode().IsRegular() {
		expected = old.Size()
	}
	if expected <= 0 || info.Size() != expected {
		return "文件大小不一致或缺少原大小，无法确认是原有内容"
	}
	// Check read access without reading a large media file or writing anything.
	f, err := os.Open(target)
	if err != nil {
		return "文件不可读，请检查权限"
	}
	_ = f.Close()
	return ""
}

func (s *MediaService) changeLibraryRootPath(ctx context.Context, libraryID, rootID string, input LibraryRootInput) (*model.LibraryRoot, error) {
	if !s.repo.LibraryPaths.TryLock() {
		return nil, pathChangeError("媒体库正在扫描或处理文件，请稍后重新检查并保存")
	}
	defer s.repo.LibraryPaths.Unlock()
	if input.PreviewToken == "" {
		return nil, pathChangeError("请先检查新目录，再确认保存")
	}
	err := s.repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Also protect against other processes writing the index between the
		// final plan and commit. Lock only during confirmation, never preview.
		if err := tx.Exec("LOCK TABLE libraries, library_roots, media, scrape_tasks IN SHARE ROW EXCLUSIVE MODE").Error; err != nil {
			return err
		}
		plan, err := planLibraryPathChange(tx, libraryID, rootID, input)
		if err != nil {
			return err
		}
		if plan.PreviewToken != input.PreviewToken {
			return pathChangeError("目录或媒体记录已变化，请重新检查后确认")
		}
		if !plan.CanApply {
			return pathChangeError("仍有缺失文件或冲突，请根据检查结果处理后再保存")
		}
		ids := make([]string, 0, len(plan.rows))
		for _, row := range plan.rows {
			ids = append(ids, row.ID)
		}
		for start := 0; start < len(ids); start += 500 {
			batch := ids[start:min(start+500, len(ids))]
			var running int64
			if err := tx.Model(&model.ScrapeTask{}).Where("media_id IN ? AND status = ?", batch, model.ScrapeTaskRunning).Count(&running).Error; err != nil {
				return err
			}
			if running > 0 {
				return pathChangeError("原目录中有正在刮削的媒体，请等待完成后重新检查")
			}
			if input.ChangeMode == LibraryPathReplace {
				if err := deleteReplacedLibraryMedia(tx, batch); err != nil {
					return err
				}
			}
		}
		if input.ChangeMode == LibraryPathMigrate {
			for _, move := range plan.moves {
				fileID, _ := fileIdentity(move.Path)
				if err := tx.Model(&model.Media{}).Where("id = ?", move.Media.ID).Updates(map[string]any{
					"path": move.Path, "relative_path": move.Rel, "library_root_id": rootID, "file_id": fileID,
				}).Error; err != nil {
					return err
				}
				if err := tx.Model(&model.ScrapeTask{}).Where("media_id = ?", move.Media.ID).Update("media_path", move.Path).Error; err != nil {
					return err
				}
				// Rebind the existing technical cache to the validated file.
				m := move.Media
				m.Path = move.Path
				if err := tx.Model(&model.MediaProbe{}).Where("media_id = ?", m.ID).Update("signature", mediaProbeSignature(&m)).Error; err != nil {
					return err
				}
			}
		}
		updates := map[string]any{"path": plan.NewPath}
		if input.Name != "" {
			updates["name"] = strings.TrimSpace(input.Name)
		}
		if input.Enabled != nil {
			updates["enabled"] = *input.Enabled
		}
		if input.SortOrder != nil {
			updates["sort_order"] = *input.SortOrder
		}
		if err := tx.Model(&model.LibraryRoot{}).Where("id = ?", rootID).Updates(updates).Error; err != nil {
			return err
		}
		var primary model.LibraryRoot
		if err := tx.Where("library_id = ?", libraryID).Order("sort_order asc, created_at asc").First(&primary).Error; err != nil {
			return err
		}
		return tx.Model(&model.Library{}).Where("id = ?", libraryID).Update("path", primary.Path).Error
	})
	if err != nil {
		return nil, err
	}
	s.invalidateMediaCache(ctx)
	return s.repo.Library.FindRootByID(ctx, libraryID, rootID)
}

// Database-only purge: never call DeleteMedia(deleteFiles=true), the organizer,
// or any filesystem mutation as part of changing a source directory.
func deleteReplacedLibraryMedia(tx *gorm.DB, ids []string) error {
	for _, entity := range []any{&model.PlaybackHistory{}, &model.Favorite{}, &model.PlaylistItem{},
		&model.MediaSegment{}, &model.MediaSegmentFetch{}, &model.MediaProbe{}, &model.ScrapeTask{}} {
		if err := tx.Unscoped().Where("media_id IN ?", ids).Delete(entity).Error; err != nil {
			return fmt.Errorf("清理旧媒体索引失败: %w", err)
		}
	}
	return tx.Unscoped().Where("id IN ?", ids).Delete(&model.Media{}).Error
}
