package service

import (
	"testing"

	"github.com/Ferrymania/nook/internal/model"
)

// 系列路径的筛选在内存里复现 SQL 语义，必须与仓储侧一致，否则会出现
// 「电影库能筛、剧集库筛不动」的行为差异。
func TestMediaRowMatchesFilters(t *testing.T) {
	row := &model.Media{
		Base:   model.Base{ID: "m1"},
		Title:  "片",
		Genres: "Action,Drama",
		Year:   2015,
		Rating: 7.5,
	}

	cases := []struct {
		name      string
		filters   MediaListFilters
		completed map[string]bool
		want      bool
	}{
		{name: "no filters", filters: MediaListFilters{}, want: true},
		{name: "genre hit", filters: MediaListFilters{Genres: []string{"Action"}}, want: true},
		{name: "Chinese alias hit", filters: MediaListFilters{Genres: []string{"动作"}}, want: true},
		{name: "genre miss", filters: MediaListFilters{Genres: []string{"Comedy"}}, want: false},
		{name: "year in range", filters: MediaListFilters{YearMin: 2010, YearMax: 2020}, want: true},
		{name: "year below min", filters: MediaListFilters{YearMin: 2016}, want: false},
		{name: "year above max", filters: MediaListFilters{YearMax: 2014}, want: false},
		{name: "rating ok", filters: MediaListFilters{RatingMin: 7}, want: true},
		{name: "rating too high", filters: MediaListFilters{RatingMin: 8}, want: false},
		{name: "unwatched passes", filters: MediaListFilters{Unwatched: true}, want: true},
		{
			name:      "unwatched excludes completed",
			filters:   MediaListFilters{Unwatched: true},
			completed: map[string]bool{"m1": true},
			want:      false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mediaRowMatchesFilters(row, tc.filters, tc.completed); got != tc.want {
				t.Fatalf("matched = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestSeriesGenreAliasMatching(t *testing.T) {
	for _, tc := range []struct {
		raw, query string
		want       bool
	}{
		{"恐怖", "Horror", true},
		{"Horror,恐怖", "恐怖", true},
		{"Drama；\tHORROR\n、Crime", "恐怖", true},
		{"Sci-Fi & Fantasy", "科幻与奇幻", true},
		{"Sci-Fi & Fantasy", "科幻", false},
		{"Action & Adventure", "动作冒险", true},
		{"Action & Adventure", "动作", false},
		{"我的类型", "我的类型", true},
		{"100ABCustom", "100%_Custom", false},
		{"100%_Custom", "100%_custom", true},
		{"HorrorComedy", "Horror", false},
	} {
		row := &model.Media{Genres: tc.raw}
		if got := mediaRowMatchesFilters(row, MediaListFilters{Genres: []string{tc.query}}, nil); got != tc.want {
			t.Fatalf("%q matches %q = %v, want %v", tc.raw, tc.query, got, tc.want)
		}
		if row.Genres != tc.raw {
			t.Fatalf("filter modified shared library row: %+v", row)
		}
	}
}

// 多词类型（如 "Science Fiction"）在内存筛选中必须整词命中。
func TestMediaRowMatchesFiltersMultiWordGenre(t *testing.T) {
	row := &model.Media{
		Base:   model.Base{ID: "m2"},
		Title:  "科幻片",
		Genres: "Science Fiction,Drama",
	}
	cases := []struct {
		name    string
		filters MediaListFilters
		want    bool
	}{
		{name: "multi-word hit", filters: MediaListFilters{Genres: []string{"Science Fiction"}}, want: true},
		{name: "partial word miss", filters: MediaListFilters{Genres: []string{"Science"}}, want: false},
		{name: "case insensitive hit", filters: MediaListFilters{Genres: []string{"science fiction"}}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mediaRowMatchesFilters(row, tc.filters, nil); got != tc.want {
				t.Fatalf("matched = %t, want %t", got, tc.want)
			}
		})
	}
}

// 无筛选时 empty() 为真，调用方据此走带缓存的原路径。
func TestMediaListFiltersEmpty(t *testing.T) {
	if !(MediaListFilters{}).empty() {
		t.Fatal("zero filters must be reported as empty")
	}
	if (MediaListFilters{Genres: []string{"Action"}}).empty() {
		t.Fatal("genre filter must not be reported as empty")
	}
	if (MediaListFilters{Unwatched: true}).empty() {
		t.Fatal("unwatched filter must not be reported as empty")
	}
}
