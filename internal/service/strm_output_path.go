package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// canonicalSyncOutputPath 只生成比较用路径，不改写配置或创建目录。
// 新输出目录尚不存在时，解析最近的已有父目录，避免经符号链接绕过检查。
func canonicalSyncOutputPath(dir string) (string, error) {
	path, err := filepath.Abs(strings.TrimSpace(dir))
	if err != nil {
		return "", err
	}
	var missing []string
	for {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			if runtime.GOOS == "windows" {
				resolved = strings.ToLower(resolved)
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		if info, statErr := os.Lstat(path); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", err // 悬空符号链接不能当作普通待创建目录。
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", err
		}
		missing = append(missing, filepath.Base(path))
		path = parent
	}
}

func syncOutputPathsOverlap(a, b string) bool {
	contains := func(parent, child string) bool {
		rel, err := filepath.Rel(parent, child)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	}
	return contains(a, b) || contains(b, a)
}

// validateSyncOutputPath 防止不同任务覆盖或清理彼此的 STRM。停用任务也保留目录占用。
func (s *StrmService) validateSyncOutputPath(ctx context.Context, localPath, excludeID string) error {
	target, err := canonicalSyncOutputPath(localPath)
	if err != nil {
		return fmt.Errorf("本地输出目录不可用：%w", err)
	}
	paths, err := s.repo.StrmSyncPath.List(ctx)
	if err != nil {
		return fmt.Errorf("检查同步输出目录失败：%w", err)
	}
	for _, existing := range paths {
		if existing.ID == excludeID || strings.TrimSpace(existing.LocalPath) == "" {
			continue
		}
		other, err := canonicalSyncOutputPath(existing.LocalPath)
		if err != nil {
			return fmt.Errorf("无法检查同步任务「%s」的输出目录「%s」：%w", existing.Name, existing.LocalPath, err)
		}
		if syncOutputPathsOverlap(target, other) {
			return fmt.Errorf("本地输出目录与同步任务「%s」的目录「%s」相同或互相包含，请选择独立目录", existing.Name, existing.LocalPath)
		}
	}
	return nil
}
