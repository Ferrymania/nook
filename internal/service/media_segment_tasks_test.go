package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ferrymania/nook/internal/model"
	"gorm.io/gorm"
)

// Observe when the follower is waiting for the shared lookup, rather than
// depending on sleeps or on how quickly PostgreSQL finishes a failed task.
type segmentWaitContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *segmentWaitContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

func TestSegmentQueueRetriesAfterSharedCallerDeadline(t *testing.T) {
	s, repos, calls := newSegmentServiceFixture(t, writeJSONBody(introDBMoviePayload))
	var requests atomic.Int32
	s.introdb.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if requests.Add(1) == 1 {
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		return http.DefaultTransport.RoundTrip(r)
	})
	m := &model.Media{Title: "超时后重试", Path: "/never-read.strm", TMDbID: 27205}
	if err := repos.DB.Create(m).Error; err != nil {
		t.Fatal(err)
	}
	failing, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	if err := repos.DB.Callback().Update().Before("gorm:update").Register("hold_failed_segment_task", func(tx *gorm.DB) {
		if tx.Statement.Table == "media_segment_tasks" {
			if task, ok := tx.Statement.Dest.(*model.MediaSegmentTask); ok && task.Status == model.SegmentTaskFailed {
				close(failing)
				<-release
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	budgeted, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	if rows, err := s.ListForPlayback(budgeted, m); err != nil || len(rows) != 0 {
		t.Fatalf("expired playback rows=%+v err=%v", rows, err)
	}
	select {
	case <-failing:
	case <-time.After(3 * time.Second):
		t.Fatal("timed-out lookup did not reach task completion")
	}
	follower := &segmentWaitContext{Context: t.Context(), waiting: make(chan struct{})}
	finished := make(chan error, 1)
	go func() {
		rows, attempted, err := s.refreshWithTrigger(follower, m, "manual")
		if err == nil && (!attempted || len(rows) != 1) {
			err = errors.New("valid follower did not fetch timestamps")
		}
		finished <- err
	}()
	select {
	case <-follower.waiting:
	case <-time.After(3 * time.Second):
		t.Fatal("follower did not join the lookup")
	}
	unblock()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("retry inherited the previous caller's deadline: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("retry did not finish")
	}
	queue, err := repos.SegmentTask.List(t.Context(), "all", "", 1, 50)
	if err != nil || requests.Load() != 2 || atomic.LoadInt32(calls) != 1 || queue.Total != 2 || queue.Counts[model.SegmentTaskFailed] != 1 || queue.Counts[model.SegmentTaskFound] != 1 {
		t.Fatalf("retry history=%+v err=%v requests=%d", queue, err, requests.Load())
	}
}

func TestSegmentQueueDoesNotRetryProviderTimeout(t *testing.T) {
	s, repos, _ := newSegmentServiceFixture(t, writeJSONBody(introDBMoviePayload))
	var requests atomic.Int32
	s.introdb.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, context.DeadlineExceeded
	})
	m := &model.Media{Title: "上游超时", Path: "/never-read.strm", TMDbID: 27205}
	if err := repos.DB.Create(m).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if _, _, err := s.refreshWithTrigger(ctx, m, "manual"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout=%v", err)
	}
	if requests.Load() != 1 {
		t.Fatalf("provider timeout retried %d times", requests.Load())
	}
}

func TestSegmentQueueRecordsLookupsButNotCacheHits(t *testing.T) {
	var status atomic.Int32
	status.Store(200)
	s, repos, calls := newSegmentServiceFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(int(status.Load()))
		_, _ = w.Write([]byte(introDBMoviePayload))
	})
	s.introdb.SetRetryDelay(0)
	lib := model.Library{Name: "电影库", Path: "/media"}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	m := &model.Media{LibraryID: lib.ID, Title: "测试影片", Path: "/never-open-video.strm", TMDbID: 27205}
	if err := repos.DB.Create(m).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListForPlayback(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListForPlayback(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	queue, err := repos.SegmentTask.List(t.Context(), "all", "电影库", 1, 50)
	if err != nil || queue.Total != 1 || queue.Counts[model.SegmentTaskFound] != 1 || atomic.LoadInt32(calls) != 1 {
		t.Fatalf("queue=%+v err=%v calls=%d", queue, err, atomic.LoadInt32(calls))
	}
	first := queue.Tasks[0]
	if first.Trigger != "playback" || first.LibraryName != lib.Name || len(first.Segments) != 1 || first.FinishedAt == nil {
		t.Fatalf("wrong snapshot: %+v", first)
	}
	status.Store(404)
	if _, err := s.QueryForMedia(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	status.Store(500)
	if _, err := s.RetryForMedia(t.Context(), m); err == nil {
		t.Fatal("upstream failure hidden")
	}
	queue, err = repos.SegmentTask.List(t.Context(), "all", "", 1, 50)
	if err != nil || queue.Total != 3 || queue.Counts[model.SegmentTaskMissing] != 1 || queue.Counts[model.SegmentTaskFailed] != 1 {
		t.Fatalf("queue=%+v err=%v", queue, err)
	}
	for _, task := range queue.Tasks {
		if task.Status == "failed" && (task.Trigger != "retry" || task.Error == "" || len(task.Segments) != 0) {
			t.Fatalf("bad failed query: %+v", task)
		}
		if task.Status == "missing" && task.Trigger != "manual" {
			t.Fatalf("bad trigger: %+v", task)
		}
	}
	status.Store(200)
	other := &model.Media{LibraryID: lib.ID, Title: "后台预取影片", Path: "/another-never-read.strm", TMDbID: 10}
	if err := repos.DB.Create(other).Error; err != nil {
		t.Fatal(err)
	}
	if attempted, err := s.SetPrewarmGap(0).Prewarm(t.Context(), []string{lib.ID}, 1); err != nil || attempted != 1 {
		t.Fatalf("prewarm=%d %v", attempted, err)
	}
	prewarmed, err := repos.SegmentTask.List(t.Context(), "found", "后台预取影片", 1, 50)
	if err != nil || prewarmed.Total != 1 || prewarmed.Tasks[0].Trigger != "prewarm" {
		t.Fatalf("prewarm not recorded: %+v %v", prewarmed, err)
	}
	old, err := repos.SegmentTask.FindByID(t.Context(), first.ID)
	if err != nil || len(old.Segments) != 1 {
		t.Fatalf("history rewritten: %+v %v", old, err)
	}
	if n, err := repos.SegmentTask.DeleteFinished(t.Context(), ""); err != nil || n != 4 {
		t.Fatalf("clear=%d %v", n, err)
	}
	ledger, err := repos.MediaSegment.GetFetch(t.Context(), m.ID, IntroDBSource)
	if err != nil || ledger == nil || ledger.Found {
		t.Fatalf("clearing records removed/changed negative cache: %+v %v", ledger, err)
	}
}

func TestSegmentQueueWriteFailureRollsBackTimestampAndLedger(t *testing.T) {
	s, repos, _ := newSegmentServiceFixture(t, writeJSONBody(introDBMoviePayload))
	m := &model.Media{Title: "保留旧时间点", Path: "/never-read.strm", TMDbID: 27205}
	if err := repos.DB.Create(m).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.QueryForMedia(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	before, _ := repos.MediaSegment.GetFetch(t.Context(), m.ID, IntroDBSource)
	if err := repos.DB.Model(&model.MediaSegment{}).Where("media_id = ?", m.ID).Update("start_ms", 123).Error; err != nil {
		t.Fatal(err)
	}
	if err := repos.DB.Exec("ALTER TABLE media_segment_tasks ADD CONSTRAINT reject_success CHECK (status <> 'found') NOT VALID").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.QueryForMedia(t.Context(), m); err == nil {
		t.Fatal("expected task save failure")
	}
	after, _ := repos.MediaSegment.GetFetch(t.Context(), m.ID, IntroDBSource)
	rows, err := repos.MediaSegment.ListByMediaSource(t.Context(), m.ID, IntroDBSource)
	if err != nil || !before.FetchedAt.Equal(after.FetchedAt) || len(rows) == 0 || rows[0].StartMs != 123 {
		t.Fatalf("cache overwritten on transaction failure: %+v %v", rows, err)
	}
	queue, err := repos.SegmentTask.List(t.Context(), "failed", "", 1, 50)
	if err != nil || queue.Total != 1 || len(queue.Tasks[0].Segments) != 0 || strings.Contains(queue.Tasks[0].Error, "SQL") {
		t.Fatalf("failure snapshot=%+v %v", queue, err)
	}
}

func TestSegmentQueueSharesInFlightLookupAndSurvivesDialogClose(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	s, repos, calls := newSegmentServiceFixture(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-entered:
		default:
			close(entered)
		}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		writeJSONBody(introDBMoviePayload)(w, r)
	})
	defer close(release)
	m := &model.Media{Title: "正在查询", Path: "/never-read.strm", TMDbID: 27205}
	if err := repos.DB.Create(m).Error; err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { _, err := s.ListForPlayback(t.Context(), m); finished <- err }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("query did not start")
	}
	queue, err := repos.SegmentTask.List(t.Context(), "running", "", 1, 50)
	if err != nil || queue.Total != 1 {
		t.Fatalf("running task invisible: %+v %v", queue, err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if _, err := s.QueryForMedia(ctx, m); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("dialog cancellation=%v", err)
	}
	// Release via a send so the deferred close also releases any test failures.
	release <- struct{}{}
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("query did not finish")
	}
	queue, err = repos.SegmentTask.List(t.Context(), "all", "", 1, 50)
	if err != nil || queue.Total != 1 || queue.Tasks[0].Status != "found" || atomic.LoadInt32(calls) != 1 {
		t.Fatalf("duplicate or discarded query: %+v %v calls=%d", queue, err, atomic.LoadInt32(calls))
	}
}
