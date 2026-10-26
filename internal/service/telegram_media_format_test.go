package service

import (
	"html"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Ferrymania/nook/internal/model"
)

func TestTelegramMediaCaption(t *testing.T) {
	m := model.Media{
		Base:  model.Base{CreatedAt: time.Date(2026, 10, 26, 12, 34, 56, 0, time.Local)},
		Title: "电影 <&>", Year: 2026, Rating: 7.9, Genres: "科幻, 动作,冒险",
		Overview: "简介中的 <b>文字</b> & 剧情。",
	}
	people := []map[string]any{
		{"Name": "导演甲", "Type": "Director"},
		{"Name": "演员甲", "Type": "Actor"}, {"Name": "演员甲", "Type": "Actor"},
		{"Name": "演员 <乙>", "Type": "Actor"},
	}
	got := formatTelegramMediaNotification(m, model.Library{Name: "外语电影", Type: "movie"}, people)
	for _, want := range []string{"<b>📚 Nook 电影入库通知</b>", "电影 &lt;&amp;&gt; (2026)", "评分：7.9", "类型：科幻、动作、冒险", "主演：演员甲、演员 &lt;乙&gt;", "入库时间：2026-10-26 12:34:56", "&lt;b&gt;文字&lt;/b&gt; &amp;"} {
		if !strings.Contains(got, want) {
			t.Errorf("caption missing %q: %s", want, got)
		}
	}
	if strings.Contains(got, "导演甲") || strings.Count(got, "演员甲") != 1 {
		t.Fatalf("unexpected cast: %s", got)
	}
}

func TestTelegramMediaCaptionHonorsUnicodeLimit(t *testing.T) {
	long := strings.Repeat("中文😀<&>", 600)
	got := formatTelegramMediaNotification(model.Media{Title: long, Overview: long, Genres: long}, model.Library{Name: long}, []map[string]any{{"Name": long, "Type": "Actor"}})
	plain := html.UnescapeString(strings.ReplaceAll(strings.ReplaceAll(got, "<b>", ""), "</b>", ""))
	if telegramTextLength(plain) > telegramMediaCaptionLimit || !utf8.ValidString(got) {
		t.Fatalf("caption length=%d valid=%v", telegramTextLength(plain), utf8.ValidString(got))
	}
	if !strings.HasSuffix(plain, "…") || !strings.Contains(plain, "简介") {
		t.Fatalf("missing bounded overview: %s", plain)
	}
	if strings.Contains(got, "<&>") {
		t.Fatal("unescaped HTML")
	}
}

func TestTelegramMediaCaptionMissingFieldsAndEpisode(t *testing.T) {
	m := model.Media{Title: "新剧", SeasonNum: 0, EpisodeNum: 11, EpisodeFraction: 0.5}
	got := formatTelegramMediaNotification(m, model.Library{Type: "tv"}, nil)
	for _, want := range []string{"Nook 剧集入库通知", "集数：S00E11.5"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q: %s", want, got)
		}
	}
	for _, missing := range []string{"评分：", "类型：", "主演：", "入库时间：", "简介", "(0)"} {
		if strings.Contains(got, missing) {
			t.Errorf("empty field %q displayed", missing)
		}
	}
}

func TestTelegramMediaDestinationRequiresConfiguredBot(t *testing.T) {
	good := telegramConfig{Enabled: true, BotToken: "secret", AdminID: "123"}
	key := telegramMediaDestination(good)
	if key == "" || strings.Contains(key, good.BotToken) {
		t.Fatal("destination must be nonempty and conceal token")
	}
	for _, cfg := range []telegramConfig{{BotToken: "secret", AdminID: "123"}, {Enabled: true, AdminID: "123"}, {Enabled: true, BotToken: "secret"}} {
		if telegramMediaDestination(cfg) != "" {
			t.Fatal("incomplete bot must not have destination")
		}
	}
	good.AdminID = "456"
	if telegramMediaDestination(good) == key {
		t.Fatal("recipient change must change cursor destination")
	}
}
