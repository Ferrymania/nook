package service

import (
	"path/filepath"
	"testing"

	"github.com/Ferrymania/nook/internal/config"
)

func TestBackupFilePathRejectsTraversal(t *testing.T) {
	svc := &BackupService{cfg: &config.Config{}}
	svc.cfg.App.DataDir = t.TempDir()
	for _, name := range []string{"../evil.json.gz", `..\evil.json.gz`, "nested/evil.json.gz", "evil.txt", "old-format.db"} {
		if _, err := svc.backupFilePath(name); err == nil {
			t.Fatalf("backupFilePath(%q) allowed traversal or non-backup file", name)
		}
	}
	path, err := svc.backupFilePath("nook_20260611_010203.json.gz")
	if err != nil {
		t.Fatalf("backupFilePath(valid) = %v", err)
	}
	if filepath.Dir(path) != filepath.Join(svc.cfg.App.DataDir, "backups") {
		t.Fatalf("backupFilePath(valid) dir = %q", filepath.Dir(path))
	}
}
