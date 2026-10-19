package service

import (
	"context"
	"time"

	"go.uber.org/zap"
)

const segmentPrewarmJobInterval = 6 * time.Hour

// SetSegments wires the IntroDB prewarm job. Without it the job is not registered.
func (s *SchedulerService) SetSegments(segments *MediaSegmentService) {
	if s != nil {
		s.segments = segments
	}
}

func (s *SchedulerService) jobSegmentPrewarm(ctx context.Context) error {
	if s == nil || s.segments == nil || s.repo == nil {
		return nil
	}
	libraries, err := s.repo.Library.List(ctx)
	if err != nil {
		return err
	}
	var libraryIDs []string
	for i := range libraries {
		lib := &libraries[i]
		if !lib.Enabled {
			continue
		}
		enabled, err := librarySegmentPrewarmEnabled(ctx, s.repo, lib)
		if err != nil {
			return err
		}
		if enabled {
			libraryIDs = append(libraryIDs, lib.ID)
		}
	}
	if len(libraryIDs) == 0 {
		return nil
	}
	// Manual scheduler runs respect library opt-outs too.
	n, err := s.segments.Prewarm(ctx, libraryIDs, segmentPrewarmDefaultLimit)
	if s.log != nil {
		s.log.Info("segment prewarm finished",
			zap.Int("libraries", len(libraryIDs)),
			zap.Int("attempted", n),
			zap.Error(err),
		)
	}
	return err
}
