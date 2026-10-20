package service

import (
	"context"
	"errors"
	"time"

	"github.com/Ferrymania/nook/internal/model"
)

var (
	ErrSegmentMetadataMissing  = errors.New("缺少可查询的影片编号或季集信息，请先完成刮削")
	ErrSegmentQueryUnavailable = errors.New("片头片尾查询服务暂不可用")
)

type MediaSegmentQueryResult struct {
	Source    string        `json:"source"`
	Found     bool          `json:"found"`
	FetchedAt time.Time     `json:"fetched_at"`
	Segments  []SegmentView `json:"segments"`
}

// QueryForMedia explicitly refreshes this media's provider data, even if its
// playback cache is fresh. It only reads metadata and TheIntroDB's public API.
func (s *MediaSegmentService) QueryForMedia(ctx context.Context, m *model.Media) (*MediaSegmentQueryResult, error) {
	return s.queryForMedia(ctx, m, "manual")
}

func (s *MediaSegmentService) RetryForMedia(ctx context.Context, m *model.Media) (*MediaSegmentQueryResult, error) {
	return s.queryForMedia(ctx, m, "retry")
}

func (s *MediaSegmentService) queryForMedia(ctx context.Context, m *model.Media, trigger string) (*MediaSegmentQueryResult, error) {
	if s == nil || s.repo == nil || s.introdb == nil {
		return nil, ErrSegmentQueryUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m == nil || m.ID == "" {
		return nil, ErrSegmentMetadataMissing
	}
	if tmdbID, _, _ := s.queryIDs(ctx, m); tmdbID <= 0 {
		return nil, ErrSegmentMetadataMissing
	}
	// Concurrent clicks for one media share a bounded request. Closing a dialog
	// need not discard a result that can still be saved for later playback.
	result := s.manualQueryFlight.DoChan(m.ID, func() (any, error) {
		fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), introDBTimeout+2*time.Second)
		defer cancel()
		rows, attempted, err := s.refreshWithTrigger(fetchCtx, m, trigger)
		if err != nil {
			return nil, err
		}
		if !attempted {
			return nil, ErrSegmentMetadataMissing
		}
		ledger, err := s.repo.MediaSegment.GetFetch(fetchCtx, m.ID, IntroDBSource)
		if err != nil {
			return nil, err
		}
		if ledger == nil {
			return nil, ErrSegmentQueryUnavailable
		}
		return &MediaSegmentQueryResult{Source: IntroDBSource, Found: ledger.Found, FetchedAt: ledger.FetchedAt, Segments: ToSegmentViews(rows)}, nil
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case outcome := <-result:
		if outcome.Err != nil {
			return nil, outcome.Err
		}
		return outcome.Val.(*MediaSegmentQueryResult), nil
	}
}
