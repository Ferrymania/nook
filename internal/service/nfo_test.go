package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ferrymania/nook/internal/model"
)

func TestWriteMediaNFOUsesMappedDestinationPath(t *testing.T) {
	root := t.TempDir()
	t.Setenv("NOOK_MEDIA_DIR", root)
	t.Setenv("NOOK_MEDIA_CONTAINER_DIR", "/media")
	mediaPath := filepath.Join(root, "电影", "测试电影.mkv")
	if err := os.MkdirAll(filepath.Dir(mediaPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mediaPath, []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := WriteMediaNFO(&model.Media{
		Title: "测试电影",
		Path:  "/media/电影/测试电影.mkv",
		Year:  2026,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "电影", "测试电影.nfo")
	if got != want {
		t.Fatalf("nfo path = %q, want %q", got, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatal(err)
	}
}

func TestWriteMediaNFOUsesEpisodeTitleForEpisodeDetails(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "剧集", "间谍过家家", "Season 02", "间谍过家家 - S02E01.mkv")
	if err := os.MkdirAll(filepath.Dir(mediaPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mediaPath, []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := WriteMediaNFO(&model.Media{
		Title:        "间谍过家家",
		OriginalName: "SPY×FAMILY",
		EpisodeTitle: "任务代号: 猫",
		Path:         mediaPath,
		SeasonNum:    2,
		EpisodeNum:   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "<title>任务代号: 猫</title>") || !strings.Contains(text, "<showtitle>间谍过家家</showtitle>") {
		t.Fatalf("episode nfo did not keep episode/show titles separate:\n%s", text)
	}
}

func TestWriteMediaNFORoundTripsExternalUniqueIDs(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "Movies", "Example", "Example.strm")
	if err := os.MkdirAll(filepath.Dir(mediaPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mediaPath, []byte("strm"), 0o644); err != nil {
		t.Fatal(err)
	}

	dst, err := WriteMediaNFO(&model.Media{
		Title:        "Example",
		OriginalName: "1234567",
		Path:         mediaPath,
		Year:         2023,
		DoubanID:     "1234567",
		TheTVDBID:    "7654321",
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{
		`<uniqueid type="douban">1234567</uniqueid>`,
		`<uniqueid type="thetvdb">7654321</uniqueid>`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("sidecar missing %s, got:\n%s", want, text)
		}
	}

	meta, err := ReadLocalMetadata(mediaPath, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if meta == nil || meta.DoubanID != "1234567" || meta.TheTVDBID != "7654321" {
		t.Fatalf("external ids did not round trip: %#v", meta)
	}
}
