package model

import (
	"strings"
	"time"
)

// ContentRating preserves the certificate independently of the audience score.
// Level is an application ordering, not an age recommendation. Unknown is 0.
type ContentRating struct {
	OfficialRating         string     `gorm:"size:64;not null;default:''" json:"official_rating,omitempty"`
	RatingCountry          string     `gorm:"size:8;not null;default:''" json:"rating_country,omitempty"`
	ContentRatingLevel     int        `gorm:"not null;default:0" json:"content_rating_level,omitempty"`
	ContentRatingSource    string     `gorm:"size:16;not null;default:''" json:"content_rating_source,omitempty"`
	ContentRatingCheckedAt *time.Time `json:"content_rating_checked_at,omitempty"`
}

// Only US certificates have an explicit comparison policy in this version.
// TV-14 falls between PG-13 and R; TV-MA shares the highest adult-only tier.
func ContentRatingLevel(raw, country string) int {
	raw, country = NormalizeContentRating(raw, country)
	if country != "US" {
		return 0
	}
	switch raw {
	case "G", "TV-Y", "TV-G":
		return 10
	case "TV-Y7", "TV-Y7-FV":
		return 15
	case "PG", "TV-PG":
		return 20
	case "PG-13":
		return 30
	case "TV-14":
		return 35
	case "R":
		return 40
	case "NC-17", "TV-MA":
		return 50
	default:
		return 0
	}
}

func NormalizeContentRating(raw, country string) (string, string) {
	raw, country = strings.ToUpper(strings.TrimSpace(raw)), strings.ToUpper(strings.TrimSpace(country))
	if prefix, value, ok := strings.Cut(raw, ":"); ok {
		if country == "" {
			country = strings.TrimSpace(prefix)
		}
		raw = strings.TrimSpace(value)
	}
	if country == "USA" {
		country = "US"
	}
	if country == "" {
		switch raw {
		case "G", "PG", "PG-13", "R", "NC-17", "TV-Y", "TV-Y7", "TV-Y7-FV", "TV-G", "TV-PG", "TV-14", "TV-MA":
			country = "US"
		}
	}
	return raw, country
}

func NewContentRating(raw, country, source string) ContentRating {
	raw, country = NormalizeContentRating(raw, country)
	now := time.Now().UTC()
	return ContentRating{OfficialRating: raw, RatingCountry: country, ContentRatingLevel: ContentRatingLevel(raw, country), ContentRatingSource: source, ContentRatingCheckedAt: &now}
}

type ContentRatingPolicy struct {
	MaxContentRatingLevel int
	HideUnrated           bool
}

func (p ContentRatingPolicy) AllowsContentRating(level int) bool {
	if level <= 0 {
		return !p.HideUnrated
	}
	return p.MaxContentRatingLevel <= 0 || level <= p.MaxContentRatingLevel
}

func (p ContentRatingPolicy) Restricted() bool { return p.MaxContentRatingLevel > 0 || p.HideUnrated }
