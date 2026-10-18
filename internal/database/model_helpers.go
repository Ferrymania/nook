package database

import (
	"gorm.io/gorm"
	"strings"
)

func modelTableName(db *gorm.DB, m any) (string, error) {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(m); err != nil {
		return "", err
	}
	return stmt.Schema.Table, nil
}

func quoteIdent(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func isPostgres(db *gorm.DB) bool {
	return db != nil && db.Dialector != nil && db.Dialector.Name() == "postgres"
}
