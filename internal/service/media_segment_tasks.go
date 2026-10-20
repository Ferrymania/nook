package service

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/Ferrymania/nook/internal/model"
)

func (s *MediaSegmentService) startSegmentTask(ctx context.Context, m *model.Media, trigger string) (*model.MediaSegmentTask, error) {
	started := time.Now()
	task := &model.MediaSegmentTask{
		MediaID: m.ID, LibraryID: m.LibraryID, LibraryName: m.LibraryName,
		MediaTitle: m.Title, MediaPath: m.Path, SeasonNum: m.SeasonNum, EpisodeNum: m.EpisodeNum,
		Provider: IntroDBSource, Trigger: trigger, Status: model.SegmentTaskRunning,
		StartedAt: &started, Segments: []model.SegmentTaskSpan{},
	}
	if m.LibraryID != "" {
		lib, err := s.repo.Library.FindByID(ctx, m.LibraryID)
		if err != nil {
			return nil, err
		}
		if lib != nil {
			task.LibraryName = lib.Name
		}
	}
	if err := s.repo.SegmentTask.Create(ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

func segmentTaskSpans(rows []model.MediaSegment) []model.SegmentTaskSpan {
	spans := make([]model.SegmentTaskSpan, 0, len(rows))
	for _, row := range rows {
		spans = append(spans, model.SegmentTaskSpan{Kind: row.Kind, StartMs: row.StartMs, EndMs: row.EndMs})
	}
	return spans
}

// Persist only an actionable message, never upstream URLs, SQL, or credentials.
func SegmentQueryErrorMessage(err error) string {
	var upstream *IntroDBHTTPError
	var timeout net.Error
	switch {
	case errors.Is(err, ErrSegmentMetadataMissing), errors.Is(err, ErrSegmentQueryUnavailable):
		return err.Error()
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeout) && timeout.Timeout()):
		return "TheIntroDB 查询超时，请稍后重试；已有时间点保留"
	case errors.As(err, &upstream) && upstream.StatusCode == http.StatusTooManyRequests:
		return "TheIntroDB 暂时限制了查询请求，请稍后重试；已有时间点保留"
	default:
		return "片头片尾查询或保存失败，请稍后重试；已有时间点保留"
	}
}
