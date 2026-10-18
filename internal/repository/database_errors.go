package repository

import (
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
)

// IsTransientDatabaseError identifies PostgreSQL transaction/lock errors that may be retried.
func IsTransientDatabaseError(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	switch pgErr.Code {
	case "40001", "40P01", "55P03":
		return true
	}
	return false
}
