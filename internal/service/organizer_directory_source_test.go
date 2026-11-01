package service

import (
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/config"
)

func TestOrganizeDirectoryUsesConfiguredSourceWhenRequestSourceEmpty(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	writeOrgFile(t, filepath.Join(src, "Dune 2021 2160p WEB-DL.mkv"), "dune-uhd")

	repos := newOrganizerTestRepo(t)
	if err := repos.Setting.Set(t.Context(), "organize.source_dir", src); err != nil {
		t.Fatal(err)
	}
	org := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	res, err := org.OrganizeDirectory(t.Context(), OrganizeOptions{
		DestPath:     dest,
		TransferMode: TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize directory with configured source: %v", err)
	}
	if res.SourcePath != filepath.Clean(src) || res.Organized != 1 {
		t.Fatalf("result = %+v, want source=%q organized=1", res, src)
	}
}

func TestOrganizeDirectoryMapsConfiguredHostPathsToContainerPaths(t *testing.T) {
	root := t.TempDir()
	hostDownloads := filepath.Join(root, "nas-host", "downloads")
	hostMedia := filepath.Join(root, "nas-host", "media")
	containerDownloads := filepath.Join(root, "container", "downloads")
	containerMedia := filepath.Join(root, "container", "media")
	containerSource := filepath.Join(containerDownloads, "国产剧")
	writeOrgFile(t, filepath.Join(containerSource, "Some Show S01E01 2024 1080p.mkv"), "show-e01")

	t.Setenv("NOOK_DOWNLOAD_DIR", hostDownloads)
	t.Setenv("NOOK_DOWNLOAD_CONTAINER_DIR", containerDownloads)
	t.Setenv("NOOK_MEDIA_DIR", hostMedia)
	t.Setenv("NOOK_MEDIA_CONTAINER_DIR", containerMedia)

	repos := newOrganizerTestRepo(t)
	for key, value := range map[string]string{
		"organize.source_dir":    filepath.Join(hostDownloads, "国产剧"),
		"organize.target_dir":    hostMedia,
		"organize.transfer_mode": "copy",
	} {
		if err := repos.Setting.Set(t.Context(), key, value); err != nil {
			t.Fatal(err)
		}
	}
	org := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	res, err := org.OrganizeDirectory(t.Context(), OrganizeOptions{})
	if err != nil {
		t.Fatalf("organize mapped host paths: %v", err)
	}
	if res.SourcePath != filepath.Clean(containerSource) || res.DestPath != filepath.Clean(containerMedia) {
		t.Fatalf("result paths = source %q dest %q, want %q -> %q", res.SourcePath, res.DestPath, containerSource, containerMedia)
	}
	want := filepath.Join(containerMedia, "电视剧", "国产剧", "Some Show", "Season 01", "Some Show - S01E01.mkv")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected organized file at %q: %v", want, err)
	}
}

func TestOrganizeDirectoryAcceptsSingleVideoFileSource(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads", "Dune 2021 2160p WEB-DL.mkv")
	dest := filepath.Join(root, "media")
	writeOrgFile(t, src, "dune-uhd")

	org := NewOrganizerService(&config.Config{}, zap.NewNop(), newOrganizerTestRepo(t))
	res, err := org.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   src,
		DestPath:     dest,
		TransferMode: TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize single file: %v", err)
	}
	if res.Organized != 1 {
		t.Fatalf("result = %+v, want organized=1", res)
	}
	want := filepath.Join(dest, "电影", "Dune (2021)", "Dune (2021).mkv")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected organized file at %q: %v", want, err)
	}
}
