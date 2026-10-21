package repository

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// sqlCaptureLogger 只记录 GORM 生成的 SQL，不打印任何日志。
type sqlCaptureLogger struct {
	mu    sync.Mutex
	lines []string
}

func (l *sqlCaptureLogger) LogMode(gormlogger.LogLevel) gormlogger.Interface { return l }
func (l *sqlCaptureLogger) Info(context.Context, string, ...interface{})     {}
func (l *sqlCaptureLogger) Warn(context.Context, string, ...interface{})     {}
func (l *sqlCaptureLogger) Error(context.Context, string, ...interface{})    {}

func (l *sqlCaptureLogger) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	l.mu.Lock()
	l.lines = append(l.lines, sql)
	l.mu.Unlock()
}

func (l *sqlCaptureLogger) all() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.lines, "\n")
}

// newPostgresDryRunRepos 返回使用 PostgreSQL 方言的 repository 容器：
// DryRun 只构建 SQL 不执行，DisableAutomaticPing 让它无需真实数据库也能离线运行，
// SkipDefaultTransaction 则挡掉 GORM 默认事务包装——那一步会真的去开事务连接，
// 在 DryRun 下同样会拨号（ToSQL 内部也是这么绕开的）。于是可以直接断言生成语句
// 在 PostgreSQL 下是否合法。
func newPostgresDryRunRepos(t *testing.T) (*Container, *sqlCaptureLogger) {
	t.Helper()
	rec := &sqlCaptureLogger{}
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "postgres://nook:nook@127.0.0.1:5432/nook?sslmode=disable",
	}), &gorm.Config{
		DryRun:                 true,
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
		Logger:                 rec,
	})
	if err != nil {
		t.Fatalf("open dry-run postgres: %v", err)
	}
	return New(db), rec
}
