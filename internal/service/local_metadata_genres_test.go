package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNFOGenresStaySeparateFromTagsAndStudios(t *testing.T) {
	for _, kind := range []string{"movie", "tvshow", "episodedetails"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "title.mkv")
			nfo := nfoPath(path)
			if kind == "tvshow" {
				nfo = filepath.Join(root, "tvshow.nfo")
			}
			body := "<" + kind + "><title>标题</title><genre>剧情</genre><genre>Drama</genre><genre>我的自定义类型</genre><genre>drama</genre>" +
				"<tag>based on true story</tag><studio>Comet Film</studio><maker>片商</maker><publisher>发行商</publisher><label>厂牌</label></" + kind + ">"
			if err := os.WriteFile(nfo, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := ReadLocalMetadata(path, root, kind != "movie")
			if err != nil || got == nil || got.Genres != "剧情,Drama,我的自定义类型" {
				t.Fatalf("metadata = %+v, err = %v", got, err)
			}
			unchanged, err := os.ReadFile(nfo)
			if err != nil || string(unchanged) != body {
				t.Fatal("import modified source NFO")
			}
		})
	}
}

func TestSeriesEpisodeGenreFallbackIgnoresShowTags(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "show-S01E01.mkv")
	if err := os.WriteFile(filepath.Join(root, "tvshow.nfo"), []byte(`<tvshow><title>整剧</title><tag>整剧标签</tag><studio>片商</studio></tvshow>`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nfoPath(path), []byte(`<episodedetails><title>单集</title><genre>科幻</genre><tag>单集标签</tag></episodedetails>`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadLocalMetadata(path, root, true)
	if err != nil || got == nil || got.Genres != "科幻" {
		t.Fatalf("metadata = %+v, err = %v", got, err)
	}
}

func TestCountGenresCountsEachMediaOnce(t *testing.T) {
	got := countGenres([]string{"Action，action;剧情、剧情,我的类型", "action；我的类型", "", "Drama"})
	if len(got) != 4 || findGenre(got, "Action") != 2 || findGenre(got, "剧情") != 1 || findGenre(got, "我的类型") != 2 || findGenre(got, "Drama") != 1 {
		t.Fatalf("genres = %+v", got)
	}
}
