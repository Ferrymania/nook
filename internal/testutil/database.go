// Package testutil provides isolated PostgreSQL schemas for database tests.
package testutil

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Open creates a fresh schema in the explicitly configured test database.
// It never reads the application's production DSN or touches existing schemas.
func Open(t testing.TB, opts ...gorm.Option) (*gorm.DB, error) {
	t.Helper()
	dsn := os.Getenv("NOOK_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("NOOK_TEST_POSTGRES_DSN is required for PostgreSQL integration tests")
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	admin, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	var nonce [12]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		_ = admin.Close(ctx)
		return nil, err
	}
	schema := "nook_test_" + hex.EncodeToString(nonce[:])
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		_ = admin.Close(ctx)
		return nil, err
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		if _, err := admin.Exec(cleanupCtx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); err != nil {
			t.Errorf("clean test schema: %v", err)
		}
		_ = admin.Close(cleanupCtx)
	})
	cfg.RuntimeParams["search_path"] = schema + ",public"
	pool := stdlib.OpenDB(*cfg)
	pool.SetMaxOpenConns(2)
	pool.SetMaxIdleConns(2)
	t.Cleanup(func() { _ = pool.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: pool}), opts...)
	if err != nil {
		return nil, fmt.Errorf("open test schema: %w", err)
	}
	return db, nil
}
