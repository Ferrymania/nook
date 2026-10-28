package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/config"
)

func TestTMDBSearchResolvesGenreIDsWithoutExtraRequests(t *testing.T) {
	var requests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		var result map[string]any
		switch r.URL.Path {
		case "/search/movie":
			result = map[string]any{"id": 101, "title": "Movie", "original_language": "en", "genre_ids": []int{18, 28, 10749, 999999}}
		case "/search/tv":
			result = map[string]any{"id": 202, "name": "TV", "original_language": "ja", "origin_country": []string{"JP"}, "genre_ids": []int{16, 10759, 10765}}
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{result}})
	}))
	defer upstream.Close()
	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey, cfg.Secrets.TMDbAPIProxy = "test-key", upstream.URL
	provider := NewTMDbProvider(cfg, zap.NewNop(), nil)
	movie, err := provider.SearchMovie(t.Context(), "Movie", 0)
	if err != nil || movie == nil || !slices.Equal(movie.Genres, []string{"剧情", "动作", "爱情"}) {
		t.Fatalf("movie match = %+v, err=%v", movie, err)
	}
	tv, err := provider.SearchTV(t.Context(), "TV", 0)
	if err != nil || tv == nil || !slices.Equal(tv.Genres, []string{"动画", "动作冒险", "科幻与奇幻"}) {
		t.Fatalf("TV match = %+v, err=%v", tv, err)
	}
	category := classifyMediaCategory(mediaClassifyInput{MediaType: tv.MediaType, Languages: tv.Languages, Countries: tv.Countries, Genres: tv.Genres}, nil)
	if category != "日番" {
		t.Fatalf("resolved genres changed animation classification: %q", category)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("genre resolution made extra requests: %d, want 2 searches", got)
	}
}
