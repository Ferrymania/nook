package repository

import "gorm.io/gorm"

// LikeOperator returns PostgreSQL's case-insensitive pattern operator.
func LikeOperator(_ *gorm.DB) string { return "ILIKE" }
func likeOperator(_ *gorm.DB) string { return "ILIKE" }
