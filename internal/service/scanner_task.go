package service

import (
	"context"
	"time"
)

type scanTaskContextKey struct{}

// WithScanTask associates progress with the existing background task. It does
// not tie the scan's lifetime to the HTTP request that started it.
func WithScanTask(ctx context.Context, task *TaskHandle) context.Context {
	return context.WithValue(ctx, scanTaskContextKey{}, task)
}

func newScanResult(ctx context.Context, libraryID string) *ScanResult {
	task, _ := ctx.Value(scanTaskContextKey{}).(*TaskHandle)
	return &ScanResult{LibraryID: libraryID, task: task}
}

func (r *ScanResult) TaskMetrics() map[string]int64 {
	if r == nil {
		return nil
	}
	return map[string]int64{
		"visited": int64(r.Visited), "added": int64(r.Added), "updated": int64(r.Updated),
		"skipped": int64(r.Skipped), "probed": int64(r.Probed), "local_metadata": int64(r.LocalMetadata),
		"removed": r.Removed, "errors": int64(r.ErrorCount),
	}
}

func (r *ScanResult) updateTaskProgress() {
	if r == nil || r.task == nil || time.Since(r.taskProgressAt) < 500*time.Millisecond {
		return
	}
	r.taskProgressAt = time.Now()
	r.task.Update(TaskUpdate{Metrics: r.TaskMetrics()})
}
