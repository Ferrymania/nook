package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/Ferrymania/nook/internal/repository"
)

// 任务失败必须通知管理员：否则只会停留在任务队列里等人自己发现。
func TestTaskFailureNotifiesAdmin(t *testing.T) {
	repos := repository.New(newServiceTestDB(t))
	tracker := NewTaskTrackerService(zap.NewNop(), nil)

	type call struct{ text string }
	var adminCalls []call
	tracker.SetFailureNotifier(func(_ context.Context, text string) {
		adminCalls = append(adminCalls, call{text})
	})

	h := tracker.Start(TaskKindOrganize, "自动整理", TaskUpdate{})
	h.Finish(errors.New("disk full"), TaskUpdate{})

	if len(adminCalls) != 1 {
		t.Fatalf("admin notifications = %d, want 1", len(adminCalls))
	}
	if !strings.Contains(adminCalls[0].text, "自动整理") {
		t.Fatalf("notification = %q, want it to name the task", adminCalls[0].text)
	}
	if !strings.Contains(adminCalls[0].text, "disk full") {
		t.Fatalf("notification = %q, want it to carry the error", adminCalls[0].text)
	}
	_ = repos
}

// 动态字段须 HTML 转义，避免路径/错误里的 <>& 破坏 parse_mode。
func TestTaskFailureAlertEscapesHTML(t *testing.T) {
	got := formatTaskFailureAlert(BackgroundTask{
		Name:       "整理 <script>",
		SourcePath: "C:\\a&b>c",
		Error:      "fail <b>now</b>",
	})
	for _, bad := range []string{"<script>", "a&b>c", "<b>now</b>"} {
		if strings.Contains(got, bad) {
			t.Fatalf("alert still contains raw %q: %s", bad, got)
		}
	}
	for _, want := range []string{"整理 &lt;script&gt;", "a&amp;b&gt;c", "fail &lt;b&gt;now&lt;/b&gt;"} {
		if !strings.Contains(got, want) {
			t.Fatalf("alert missing escaped %q: %s", want, got)
		}
	}
}

// 成功结束的任务不应触发失败通知。
func TestTaskSuccessDoesNotNotifyAdmin(t *testing.T) {
	tracker := NewTaskTrackerService(zap.NewNop(), nil)
	var count int
	tracker.SetFailureNotifier(func(context.Context, string) { count++ })

	h := tracker.Start(TaskKindOrganize, "自动整理", TaskUpdate{})
	h.Finish(nil, TaskUpdate{})

	if count != 0 {
		t.Fatalf("admin notifications = %d, want 0", count)
	}
}

// 未接线通知时，任务路径必须照常完成。
func TestTaskTrackerWorksWithoutNotifier(t *testing.T) {
	tracker := NewTaskTrackerService(zap.NewNop(), nil)
	h := tracker.Start(TaskKindScan, "扫描", TaskUpdate{})
	h.Finish(errors.New("boom"), TaskUpdate{})
}
