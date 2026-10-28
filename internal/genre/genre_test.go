package genre

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestNamesMergeKnownAliasesAndPreserveCustomGenres(t *testing.T) {
	got := Names(" Drama，剧情;Horror、恐怖；", " 我的类型,MY GENRE,my genre ", "纪录,Documentary")
	want := []string{"剧情", "恐怖", "我的类型", "MY GENRE", "纪录片"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Names = %v, want %v", got, want)
	}
	if empty := Names(" ,； "); empty == nil || len(empty) != 0 {
		t.Fatalf("empty names = %#v", empty)
	}
	// Raw splitting is still available to importers/exporters without translation.
	if got := Split("Drama,剧情"); !reflect.DeepEqual(got, []string{"Drama", "剧情"}) {
		t.Fatalf("raw genres changed: %v", got)
	}
}

func TestCompoundGenresStayDistinct(t *testing.T) {
	for _, pair := range [][2]string{
		{"Action", "Action & Adventure"},
		{"Science Fiction", "Sci-Fi & Fantasy"},
		{"War", "War & Politics"},
		{"Fantasy", "Sci-Fi & Fantasy"},
	} {
		if len(Names(pair[:]...)) != 2 || Key(pair[0]) == Key(pair[1]) {
			t.Fatalf("distinct genres merged: %v", pair)
		}
	}
	if got := Names("动作冒险,Action & Adventure,Sci-Fi & Fantasy,科幻与奇幻"); !reflect.DeepEqual(got, []string{"动作冒险", "科幻与奇幻"}) {
		t.Fatalf("compound aliases = %v", got)
	}
}

func TestAliasExpansionMatchesCanonicalNames(t *testing.T) {
	for _, name := range []string{"Horror", "恐怖", " horror "} {
		if got := MatchNames([]string{name}); !reflect.DeepEqual(got, []string{"恐怖", "horror"}) {
			t.Fatalf("MatchNames(%q) = %v", name, got)
		}
	}
	for _, group := range groups {
		for _, name := range group {
			if Canonical(name) != group[0] || !slices.Contains(MatchNames([]string{group[0]}), strings.ToLower(name)) {
				t.Fatalf("alias %q does not round trip to %q", name, group[0])
			}
		}
	}
	if got := MatchNames([]string{" ", "100%_Custom"}); !reflect.DeepEqual(got, []string{"100%_custom"}) {
		t.Fatalf("custom query changed: %v", got)
	}
}
