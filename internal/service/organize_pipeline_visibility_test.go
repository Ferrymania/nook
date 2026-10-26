package service

import (
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/config"
	"github.com/Ferrymania/nook/internal/model"
)

func TestOrganizePipelineSyncsVisibilityWhenTargetAlreadyExists(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	writeOrgFile(t, filepath.Join(src, "国产剧", "狂飙.S01E01.2023.1080p.mkv"), "episode")

	repos := newOrganizerTestRepo(t)
	for key, value := range map[string]string{
		"organize.source_dir":    src,
		"organize.target_dir":    dest,
		"organize.transfer_mode": "copy",
	} {
		if err := repos.Setting.Set(t.Context(), key, value); err != nil {
			t.Fatal(err)
		}
	}
	lib := model.Library{Name: "国产剧", Path: filepath.Join(dest, "电视剧", "国产剧"), Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	organizer := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	if _, err := organizer.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   src,
		DestPath:     dest,
		TransferMode: TransferCopy,
	}); err != nil {
		t.Fatalf("seed organize destination: %v", err)
	}
	scanner := NewScannerService(&config.Config{}, zap.NewNop(), repos, NewHub(zap.NewNop()), nil, nil)
	pipeline := NewOrganizePipelineService(zap.NewNop(), repos, organizer, scanner, nil)
	if _, err := pipeline.Run(t.Context(), OrganizePipelineRequest{
		Scope:   OrganizeScopeDirectory,
		Trigger: OrganizeTriggerManual,
	}); err != nil {
		t.Fatalf("organize existing destination: %v", err)
	}
	var count int64
	if err := repos.DB.Model(&model.Media{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("target already exists should still be scanned into DB, count=%d want 1", count)
	}
}
