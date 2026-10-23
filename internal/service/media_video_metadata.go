package service

import (
	"context"
	"math"
	"strconv"

	"github.com/Ferrymania/nook/internal/model"
)

// Detail requests only read saved metadata; they never resolve a URL or start ffprobe.
func (s *MediaService) attachVideoMetadata(ctx context.Context, m *model.Media) {
	strm := isStrmMediaRow(m)
	if strm {
		// A scan may have saved the STRM pointer's size rather than the video size.
		m.SizeBytes = 0
	}
	probe := cachedMediaProbe(ctx, s.repo, m)
	if probe == nil {
		return
	}
	if strm && probe.SizeBytes > 0 {
		m.SizeBytes = probe.SizeBytes
	}
	for _, stream := range probe.Streams {
		if stream.Type != "video" {
			continue
		}
		kind, subtype, _, _ := embyExtendedVideoType(stream)
		m.VideoMetadata = &model.MediaVideoMetadata{
			FrameRate:         positiveFrameRate(stream.FrameRate),
			ExtendedVideoType: kind, ExtendedVideoSubType: subtype,
		}
		return
	}
}

func positiveFrameRate(raw string) float64 {
	rate, err := strconv.ParseFloat(normalizeFrameRate(raw), 64)
	if err != nil || math.IsNaN(rate) || math.IsInf(rate, 0) || rate <= 0 {
		return 0
	}
	return rate
}
