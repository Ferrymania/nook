package service

import (
	"errors"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/Ferrymania/nook/internal/model"
)

func TestManualSegmentQueryRefreshesCachedResultAndPreservesOnFailure(t *testing.T) {
	var status atomic.Int32
	status.Store(http.StatusOK)
	s, repos, calls := newSegmentServiceFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(int(status.Load()))
		_, _ = w.Write([]byte(introDBMoviePayload))
	})
	m := &model.Media{Base: model.Base{ID: "manual"}, Path: "/never-read-video.strm", TMDbID: 27205}
	if err := repos.DB.Create(m).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		result, err := s.QueryForMedia(t.Context(), m)
		if err != nil || result == nil || !result.Found || result.FetchedAt.IsZero() || len(result.Segments) == 0 {
			t.Fatalf("query=%#v err=%v", result, err)
		}
	}
	if got := atomic.LoadInt32(calls); got != 2 {
		t.Fatalf("manual refresh used playback cache: calls=%d", got)
	}
	before, _ := repos.MediaSegment.GetFetch(t.Context(), m.ID, IntroDBSource)
	status.Store(http.StatusInternalServerError)
	if _, err := s.QueryForMedia(t.Context(), m); err == nil {
		t.Fatal("provider failure reported success")
	}
	after, _ := repos.MediaSegment.GetFetch(t.Context(), m.ID, IntroDBSource)
	rows, err := repos.MediaSegment.ListByMediaSource(t.Context(), m.ID, IntroDBSource)
	if err != nil || len(rows) == 0 || !after.Found || !before.FetchedAt.Equal(after.FetchedAt) {
		t.Fatalf("failure changed cached result: %#v %#v err=%v", before, after, err)
	}
	status.Store(http.StatusNotFound)
	result, err := s.QueryForMedia(t.Context(), m)
	if err != nil || result == nil || result.Found || result.Segments == nil || len(result.Segments) != 0 {
		t.Fatalf("missing data=%#v err=%v", result, err)
	}
	ledger, _ := repos.MediaSegment.GetFetch(t.Context(), m.ID, IntroDBSource)
	if ledger == nil || ledger.Found {
		t.Fatalf("missing result not persisted: %#v", ledger)
	}
}

func TestManualSegmentQueryRejectsMissingMetadataWithoutRequest(t *testing.T) {
	s, _, calls := newSegmentServiceFixture(t, writeJSONBody(introDBMoviePayload))
	for _, m := range []*model.Media{nil, {Base: model.Base{ID: "missing"}}, {Base: model.Base{ID: "episode"}, SeasonNum: 1}} {
		if _, err := s.QueryForMedia(t.Context(), m); !errors.Is(err, ErrSegmentMetadataMissing) {
			t.Fatalf("missing metadata: %v", err)
		}
	}
	if atomic.LoadInt32(calls) != 0 {
		t.Fatal("missing metadata contacted provider")
	}
}

func TestManualSegmentQueryRollsBackWhenLedgerWriteFails(t *testing.T) {
	s, repos, _ := newSegmentServiceFixture(t, writeJSONBody(introDBMoviePayload))
	m := &model.Media{Base: model.Base{ID: "rollback"}, Path: "/never-read.strm", TMDbID: 27205}
	if err := repos.DB.Create(m).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.QueryForMedia(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	// Leave a known old timestamp and reject the ledger update after replacement.
	if err := repos.DB.Model(&model.MediaSegment{}).Where("media_id = ?", m.ID).Update("start_ms", 123).Error; err != nil {
		t.Fatal(err)
	}
	if err := repos.DB.Exec("ALTER TABLE media_segment_fetches ADD CONSTRAINT reject_refresh CHECK (NOT found) NOT VALID").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.QueryForMedia(t.Context(), m); err == nil {
		t.Fatal("expected ledger failure")
	}
	rows, err := repos.MediaSegment.ListByMediaSource(t.Context(), m.ID, IntroDBSource)
	if err != nil || len(rows) == 0 {
		t.Fatalf("cached rows lost: %v", err)
	}
	for _, row := range rows {
		if row.StartMs != 123 {
			t.Fatalf("failed query replaced cached timestamp: %#v", row)
		}
	}
}
