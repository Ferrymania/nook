package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/Ferrymania/nook/internal/config"
)

// BackupService manages database backups.
type BackupService struct {
	cfg *config.Config
	log *zap.Logger
	db  *gorm.DB
}

// NewBackupService is the constructor.
func NewBackupService(cfg *config.Config, log *zap.Logger, db *gorm.DB) *BackupService {
	return &BackupService{cfg: cfg, log: log, db: db}
}

// BackupInfo describes one stored backup file.
type BackupInfo struct {
	Filename  string    `json:"filename"`
	Path      string    `json:"path"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

func (b *BackupService) backupDir() string {
	return filepath.Join(b.cfg.App.DataDir, "backups")
}

func (b *BackupService) backupFilePath(filename string) (string, error) {
	if !isValidBackupFilename(filename) {
		return "", errors.New("invalid filename")
	}
	dir, err := filepath.Abs(b.backupDir())
	if err != nil {
		return "", err
	}
	path, err := filepath.Abs(filepath.Join(dir, filename))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(dir, path)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", errors.New("invalid filename")
	}
	return path, nil
}

// Create produces a consistent PostgreSQL logical backup.
func (b *BackupService) Create(ctx context.Context) (*BackupInfo, error) {
	dir := b.backupDir()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	ts := time.Now().Format("20060102_150405")

	dst, err := b.createPostgresBackup(ctx, ts)
	if err != nil {
		return nil, err
	}
	stat, err := os.Stat(dst)
	if err != nil {
		return nil, err
	}
	b.log.Info("backup created", zap.String("path", dst), zap.Int64("size", stat.Size()))
	return &BackupInfo{
		Filename:  filepath.Base(dst),
		Path:      dst,
		Size:      stat.Size(),
		CreatedAt: stat.ModTime(),
	}, nil
}

// List returns every backup file in the backup directory, newest first.
func (b *BackupService) List() ([]BackupInfo, error) {
	dir := b.backupDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]BackupInfo, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !isBackupFilename(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, BackupInfo{
			Filename:  e.Name(),
			Path:      filepath.Join(dir, e.Name()),
			Size:      info.Size(),
			CreatedAt: info.ModTime(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

// Delete removes a single backup file.
func (b *BackupService) Delete(filename string) error {
	path, err := b.backupFilePath(filename)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

// Restore replaces database rows in a single transaction.
func (b *BackupService) Restore(ctx context.Context, filename string) error {
	src, err := b.backupFilePath(filename)
	if err != nil {
		return err
	}
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("backup not found: %s", filename)
	}
	return b.restorePostgresBackup(ctx, src)
}

// isBackupFilename accepts PostgreSQL logical backup files.
func isBackupFilename(name string) bool {
	return strings.HasSuffix(name, postgresBackupSuffix)
}

// isValidBackupFilename rejects path traversal attempts and non-backup files.
func isValidBackupFilename(name string) bool {
	if name == "" {
		return false
	}
	if strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		return false
	}
	return isBackupFilename(name)
}
