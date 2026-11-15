package model

import "testing"

func TestContentRatingNormalizationAndPolicy(t *testing.T) {
	for _, tc := range []struct {
		raw, country string
		level        int
	}{{"PG-13", "US", 30}, {"us:pg-13", "", 30}, {"TV-Y7-FV", "", 15}, {"TV-14", "", 35}, {"TV-MA", "US", 50}, {"PG", "GB", 0}, {"NR", "US", 0}, {"18", "DE", 0}, {"", "", 0}} {
		if got := ContentRatingLevel(tc.raw, tc.country); got != tc.level {
			t.Errorf("%q %q level=%d want %d", tc.raw, tc.country, got, tc.level)
		}
	}
	p := ContentRatingPolicy{MaxContentRatingLevel: 30}
	if !p.AllowsContentRating(10) || !p.AllowsContentRating(30) || !p.AllowsContentRating(0) || p.AllowsContentRating(35) || p.AllowsContentRating(50) {
		t.Fatal("PG-13 policy incorrect")
	}
	p.HideUnrated = true
	if p.AllowsContentRating(0) {
		t.Fatal("unknown content allowed")
	}
}
