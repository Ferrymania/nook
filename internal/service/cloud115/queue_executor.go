package cloud115

import (
	"context"
	"sync"

	"golang.org/x/time/rate"
)

const DefaultQPS = 1

// QueueExecutor 全局 115 请求调度执行器，通过三级令牌桶（QPS / QPM / QPH）平滑所有外发请求。
type QueueExecutor struct {
	configMu        sync.Mutex
	qpsLimiter      *rate.Limiter
	qpmLimiter      *rate.Limiter
	qphLimiter      *rate.Limiter
	throttleManager *ThrottleManager
}

var (
	globalExecutor *QueueExecutor
	executorOnce   sync.Once
)

// GetGlobalExecutor returns the request limiter shared by all 115 clients.
func GetGlobalExecutor() *QueueExecutor {
	executorOnce.Do(func() {
		globalExecutor = NewQueueExecutor(DefaultQPS, DefaultQPS*60, DefaultQPS*3600)
	})
	return globalExecutor
}

// NewQueueExecutor 创建新的队列调度执行器。
func NewQueueExecutor(qps, qpm, qph int) *QueueExecutor {
	if qps <= 0 {
		qps = DefaultQPS
	}
	if qpm <= 0 {
		qpm = qps * 60
	}
	if qph <= 0 {
		qph = qps * 3600
	}

	return &QueueExecutor{
		qpsLimiter:      rate.NewLimiter(rate.Limit(qps), qps),
		qpmLimiter:      rate.NewLimiter(rate.Limit(qpm)/60, qpm),
		qphLimiter:      rate.NewLimiter(rate.Limit(qph)/3600, qph),
		throttleManager: GetGlobalThrottleManager(),
	}
}

// Acquire 统一在发送请求前获取令牌，并等待熔断恢复。
func (qe *QueueExecutor) Acquire(ctx context.Context) error {
	// 1. 如果处于限流状态，先阻塞等待 60s 静默冷却结束
	if err := qe.throttleManager.WaitThrottleRecovery(ctx); err != nil {
		return err
	}

	// 2. 依次获取小时级、分钟级、秒级令牌
	if err := qe.qphLimiter.Wait(ctx); err != nil {
		return err
	}
	if err := qe.qpmLimiter.Wait(ctx); err != nil {
		return err
	}
	if err := qe.qpsLimiter.Wait(ctx); err != nil {
		return err
	}

	return nil
}

// SetRequestsPerSecond updates all three buckets from the validated QPS setting.
func (qe *QueueExecutor) SetRequestsPerSecond(qps int) {
	qe.configMu.Lock()
	defer qe.configMu.Unlock()

	limit := rate.Limit(qps)
	qe.qpsLimiter.SetLimit(limit)
	qe.qpsLimiter.SetBurst(qps)
	qe.qpmLimiter.SetLimit(limit)
	qe.qpmLimiter.SetBurst(qps * 60)
	qe.qphLimiter.SetLimit(limit)
	qe.qphLimiter.SetBurst(qps * 3600)
}

// MarkThrottled 触发限流熔断。
func (qe *QueueExecutor) MarkThrottled() {
	qe.throttleManager.MarkThrottled()
}
