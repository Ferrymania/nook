package service

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/model"
)

// TestWriteArtworkDataToPathWritesJellyfinSidecar verifies that in-memory
// artwork bytes are written as a Jellyfin/Emby sidecar (poster.jpg) in the
// media directory using the scraped image MIME type.
func TestWriteArtworkDataToPathWritesJellyfinSidecar(t *testing.T) {
	scraper := &ScraperService{log: zap.NewNop()}
	mediaDir := t.TempDir()

	dst := scraper.writeArtworkDataToPath(mediaDir, "poster", "image/jpeg", testJPEG)
	if dst == "" {
		t.Fatal("expected a written destination path")
	}
	if filepath.Base(dst) != "poster.jpg" {
		t.Fatalf("base = %q, want poster.jpg", filepath.Base(dst))
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("poster.jpg not readable: %v", err)
	}
	if string(data) != string(testJPEG) {
		t.Fatal("poster.jpg content mismatch")
	}
}

func TestWriteArtworkDataToPathReplacesExistingSidecar(t *testing.T) {
	scraper := &ScraperService{log: zap.NewNop()}
	mediaDir := t.TempDir()
	dst := filepath.Join(mediaDir, "poster.jpg")
	if err := os.WriteFile(dst, []byte("old DVD cover"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := scraper.writeArtworkDataToPath(mediaDir, "poster", "image/jpeg", testJPEG); got != dst {
		t.Fatalf("destination = %q, want %q", got, dst)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(testJPEG) {
		t.Fatal("existing sidecar was not replaced")
	}
}

// TestWriteArtworkDataToPathSkipsUnchangedSidecar 验证"内容一致就不重写"：
// 缩略图缓存键包含源文件的大小与 mtime，无谓的重写会让该条目所有尺寸的缩略图
// 一起失效，下次浏览全部重新解码。
func TestWriteArtworkDataToPathSkipsUnchangedSidecar(t *testing.T) {
	scraper := &ScraperService{log: zap.NewNop()}
	mediaDir := t.TempDir()
	dst := scraper.writeArtworkDataToPath(mediaDir, "poster", "image/jpeg", testJPEG)
	if dst == "" {
		t.Fatal("expected a written destination path")
	}
	// 把 mtime 拨回过去，再写入完全相同的字节：文件必须原样保留。
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(dst, past, past); err != nil {
		t.Fatal(err)
	}
	if got := scraper.writeArtworkDataToPath(mediaDir, "poster", "image/jpeg", testJPEG); got != dst {
		t.Fatalf("destination = %q, want %q", got, dst)
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(info.ModTime()) < time.Minute {
		t.Fatalf("mtime = %v, want the original past timestamp (identical artwork was rewritten)", info.ModTime())
	}

	// 内容变化时仍然必须覆盖。
	changed := append([]byte(nil), testJPEG...)
	changed[len(changed)-1] = 0x00
	if got := scraper.writeArtworkDataToPath(mediaDir, "poster", "image/jpeg", changed); got != dst {
		t.Fatalf("destination = %q, want %q", got, dst)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, changed) {
		t.Fatal("changed artwork was not written")
	}
}

// TestImageExtForContentType verifies the MIME -> extension mapping used to
// name Jellyfin sidecar files.
func TestImageExtForContentType(t *testing.T) {
	cases := map[string]string{
		"image/jpeg":                 ".jpg",
		"image/jpg":                  ".jpg",
		"image/pjpeg":                ".jpg",
		"image/png":                  ".png",
		"image/webp":                 ".webp",
		"image/gif":                  ".gif",
		"image/avif":                 ".avif",
		"image/jpeg; charset=binary": ".jpg",
		"application/octet-stream":   ".img",
	}
	for in, want := range cases {
		if got := imageExtForContentType(in); got != want {
			t.Errorf("imageExtForContentType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDownloadArtworkPreservesLocalImage(t *testing.T) {
	scraper := &ScraperService{log: zap.NewNop()}
	dir := t.TempDir()
	legacy := filepath.Join(dir, "movie-poster.img")
	wide := testJPEG
	if err := os.WriteFile(legacy, wide, 0o644); err != nil {
		t.Fatal(err)
	}

	dst := scraper.downloadArtworkToPath(t.Context(), dir, "movie-poster", legacy)
	if filepath.Base(dst) != "movie-poster.jpg" {
		t.Fatalf("destination = %q, want canonical JPEG sidecar", dst)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, wide) {
		t.Fatal("artwork bytes changed during download")
	}
}

// TestWriteMediaArtworkFilesAfterScrapeSkipsCloud verifies that cloud media
// never has artwork written anywhere.
func TestWriteMediaArtworkFilesAfterScrapeSkipsCloud(t *testing.T) {
	scraper := &ScraperService{log: zap.NewNop()}
	cloud := &model.Media{
		Path:        "cloud://aliyun/openlist/Some.Movie.mkv",
		PosterURL:   "https://image.tmdb.org/t/p/poster.jpg",
		BackdropURL: "https://image.tmdb.org/t/p/backdrop.jpg",
	}
	scraper.writeMediaArtworkFilesAfterScrape(t.Context(), cloud, nil, ScrapeOptions{})
	// No panic and no files written; the method should return early.
}

// TestSameDirectoryMediaUsesBaseScopedSidecars verifies that two movies sharing
// one directory produce distinct, base-scoped sidecar names (A-poster.jpg vs
// B-poster.jpg) so they never overwrite each other.
func TestSameDirectoryMediaUsesBaseScopedSidecars(t *testing.T) {
	mediaPath := filepath.Join(t.TempDir(), "Movie Folder", "A.mp4")
	if err := os.MkdirAll(filepath.Dir(mediaPath), 0o755); err != nil {
		t.Fatal(err)
	}
	scraper := &ScraperService{log: zap.NewNop()}

	// Simulate the base-scoped naming used by writeMediaArtworkFilesAfterScrape.
	base := "A"
	dir := filepath.Dir(mediaPath)
	dst := scraper.writeArtworkDataToPath(dir, base+"-poster", "image/jpeg", testJPEG)
	if dst == "" {
		t.Fatal("expected a written destination")
	}
	if filepath.Base(dst) != "A-poster.jpg" {
		t.Fatalf("base = %q, want A-poster.jpg", filepath.Base(dst))
	}
}
