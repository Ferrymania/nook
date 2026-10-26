package service

import (
	"fmt"
	"html"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/Ferrymania/nook/internal/model"
)

const telegramMediaCaptionLimit = 1024

func formatTelegramMediaNotification(m model.Media, lib model.Library, people []map[string]any, episodes ...model.Media) string {
	kind := "电影"
	if mediaIsEpisodic(&m, &lib) {
		kind = "剧集"
	}
	heading := "📚 Nook " + kind + "入库通知"
	title := truncateTelegramText(strings.TrimSpace(m.Title), 140)
	if m.Year > 0 {
		title += fmt.Sprintf(" (%d)", m.Year)
	}
	lines := []string{heading, "", title}
	if len(episodes) == 0 {
		episodes = []model.Media{m}
	}
	if label, count := telegramEpisodeSummary(episodes); count > 0 {
		line := "集数：" + truncateTelegramText(label, 160)
		if len(episodes) > 1 {
			line += fmt.Sprintf(" · 新增 %d 集", count)
		}
		lines = append(lines, line)
	}
	if lib.Name != "" {
		lines = append(lines, "媒体库："+truncateTelegramText(lib.Name, 80))
	}
	if m.Rating > 0 {
		lines = append(lines, fmt.Sprintf("评分：%.1f", m.Rating))
	}
	if strings.TrimSpace(m.Genres) != "" {
		lines = append(lines, "类型："+truncateTelegramText(strings.Join(splitCSV(m.Genres), "、"), 120))
	}
	var actors []string
	seen := map[string]bool{}
	for _, person := range people {
		name, _ := person["Name"].(string)
		typ, _ := person["Type"].(string)
		name = strings.TrimSpace(name)
		if name != "" && strings.EqualFold(typ, "Actor") && !seen[name] {
			actors = append(actors, name)
			seen[name] = true
			if len(actors) == 5 {
				break
			}
		}
	}
	if len(actors) > 0 {
		lines = append(lines, "主演："+truncateTelegramText(strings.Join(actors, "、"), 160))
	}
	if !m.CreatedAt.IsZero() {
		lines = append(lines, "入库时间："+m.CreatedAt.In(time.Local).Format("2006-01-02 15:04:05"))
	}
	plain := strings.Join(lines, "\n")
	if overview := strings.TrimSpace(m.Overview); overview != "" {
		prefix := "\n\n简介\n"
		remaining := telegramMediaCaptionLimit - telegramTextLength(plain+prefix)
		if remaining > 0 {
			plain += prefix + truncateTelegramText(overview, remaining)
		}
	}
	// Escape only after budgeting the visible text: entities count as one
	// character, and truncating encoded HTML can split an entity or tag.
	return "<b>" + html.EscapeString(heading) + "</b>" + html.EscapeString(strings.TrimPrefix(plain, heading))
}

func telegramTextLength(text string) int { return len(utf16.Encode([]rune(text))) }

func truncateTelegramText(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if telegramTextLength(text) <= limit {
		return text
	}
	var out strings.Builder
	used := 0
	for _, r := range text {
		n := 1
		if r > 0xffff {
			n = 2
		}
		if used+n > limit-1 {
			break
		}
		out.WriteRune(r)
		used += n
	}
	return out.String() + "…"
}
