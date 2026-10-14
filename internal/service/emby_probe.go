package service

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/Ferrymania/nook/internal/model"
)

// Read saved probe data only; serving an Emby response must not start a probe.
func (e *EmbyService) cachedMediaProbe(ctx context.Context, m *model.Media) *mediaProbePayload {
	if e == nil || e.repo == nil || e.repo.MediaProbe == nil {
		return nil
	}
	row, err := e.repo.MediaProbe.Get(ctx, m.ID)
	if err != nil || row == nil || row.Payload == "" {
		return nil
	}
	if row.Signature != "" && row.Signature != mediaProbeSignature(m) {
		return nil
	}
	var payload mediaProbePayload
	if json.Unmarshal([]byte(row.Payload), &payload) != nil {
		return nil
	}
	return &payload
}

func probedMediaStreams(probe *mediaProbePayload) []map[string]any {
	streams := make([]map[string]any, 0, len(probe.Streams))
	for _, s := range probe.Streams {
		kind := ""
		switch s.Type {
		case "video":
			kind = "Video"
		case "audio":
			kind = "Audio"
		case "subtitle":
			kind = "Subtitle"
		default:
			continue
		}
		stream := map[string]any{
			"Index": s.Index, "Type": kind, "Codec": s.Codec,
			"Profile": s.Profile, "Language": s.Language, "Title": s.Title,
			"IsDefault": s.Default, "IsForced": s.Forced, "IsExternal": false,
			"DisplayTitle": strings.TrimSpace(strings.Join([]string{s.Language, s.Title, s.Codec}, " ")),
		}
		if s.BitRate > 0 {
			stream["BitRate"] = s.BitRate
		}
		switch s.Type {
		case "video":
			stream["Width"], stream["Height"] = s.Width, s.Height
			stream["PixelFormat"] = s.PixFmt
			if rate, err := strconv.ParseFloat(s.FrameRate, 64); err == nil && rate > 0 {
				stream["AverageFrameRate"] = rate
			}
		case "audio":
			stream["Channels"], stream["ChannelLayout"] = s.Channels, s.ChannelLayout
			stream["SampleRate"] = s.SampleRate
		case "subtitle":
			stream["DeliveryMethod"] = "Embed"
			stream["SupportsExternalStream"] = false
		}
		streams = append(streams, stream)
	}
	return streams
}

func (e *EmbyService) externalSubtitleStart(ctx context.Context, m *model.Media) int {
	if probe := e.cachedMediaProbe(ctx, m); probe != nil {
		if streams := probedMediaStreams(probe); len(streams) > 0 {
			return nextMediaStreamIndex(streams)
		}
	}
	if m.AudioCodec != "" {
		return 2
	}
	return 1
}

func nextMediaStreamIndex(streams []map[string]any) int {
	next := 1
	for _, stream := range streams {
		if index, ok := stream["Index"].(int); ok && index >= next {
			next = index + 1
		}
	}
	return next
}

func mediaWithHistoryDuration(m *model.Media, durationMs int64) *model.Media {
	if m.DurationSec > 0 || durationMs <= 0 {
		return m
	}
	copy := *m
	copy.DurationSec = int(durationMs / 1000)
	return &copy
}
