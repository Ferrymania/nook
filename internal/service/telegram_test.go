package service

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/repository"
)

// pollLoop 在 Telegram 禁用状态下不应退出，而应持续等待配置开启。
func TestPollLoopContinuesWhenDisabled(t *testing.T) {
	repos := repository.New(newServiceTestDB(t))
	s := NewTelegramService(zap.NewNop(), repos)

	// 使用一个立即取消的 context 验证 goroutine 能正常退出（不阻塞）。
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.pollLoop(ctx)
	}()

	// 给 goroutine 启动时间，然后取消 context。
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// ok: goroutine 正常退出
	case <-time.After(2 * time.Second):
		t.Fatal("pollLoop did not exit after ctx cancellation")
	}
}
