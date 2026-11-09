package model

import "time"

// ExternalRating is shared by every local version or episode of a TMDb work.
type ExternalRating struct {
	Kind           string     `gorm:"primaryKey;size:5" json:"-"`
	TMDbID         int        `gorm:"primaryKey;autoIncrement:false" json:"-"`
	IMDbID         string     `gorm:"column:imdb_id;size:24" json:"-"`
	IMDb           *float64   `gorm:"column:imdb" json:"imdb,omitempty"`
	RottenTomatoes *int       `json:"rotten_tomatoes,omitempty"`
	Metacritic     *int       `json:"metacritic,omitempty"`
	Status         string     `gorm:"size:16;index:idx_external_ratings_due,priority:1" json:"-"`
	Reason         string     `gorm:"size:32" json:"-"`
	CheckedAt      *time.Time `json:"-"`
	NextAttemptAt  time.Time  `gorm:"index:idx_external_ratings_due,priority:2" json:"-"`
}

// ExternalRatingBudget persists reservations across server restarts.
type ExternalRatingBudget struct {
	KeyDigest string `gorm:"primaryKey;size:64"`
	Day       string `gorm:"size:10"`
	Used      int
	Paused    string `gorm:"size:16"`
}
