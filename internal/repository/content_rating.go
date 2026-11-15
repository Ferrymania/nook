package repository

import (
	"github.com/Ferrymania/nook/internal/model"
	"gorm.io/gorm"
)

// Apply before counting, grouping or pagination. Unknown is independent of limit.
func ApplyContentRatingPolicy(q *gorm.DB, policy model.ContentRatingPolicy) *gorm.DB {
	if policy.HideUnrated {
		q = q.Where("media.content_rating_level > 0")
	}
	if policy.MaxContentRatingLevel > 0 {
		q = q.Where("media.content_rating_level <= ?", policy.MaxContentRatingLevel)
	}
	return q
}
