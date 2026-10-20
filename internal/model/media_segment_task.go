package model

import "time"

const (
	SegmentTaskRunning = "running"
	SegmentTaskFound   = "found"
	SegmentTaskMissing = "missing"
	SegmentTaskFailed  = "failed"
)

// MediaSegmentTask is one actual provider lookup, not a playback cache hit.
// Keep a result snapshot so later refreshes cannot rewrite query history.
type MediaSegmentTask struct {
	Base
	MediaID     string            `gorm:"size:128;index" json:"media_id"`
	LibraryID   string            `gorm:"size:36;index" json:"library_id"`
	LibraryName string            `gorm:"size:128" json:"library_name"`
	MediaTitle  string            `gorm:"size:255" json:"media_title"`
	MediaPath   string            `gorm:"type:text" json:"media_path"`
	SeasonNum   int               `json:"season_num"`
	EpisodeNum  int               `json:"episode_num"`
	Provider    string            `gorm:"size:32" json:"provider"`
	Trigger     string            `gorm:"size:16" json:"trigger"`
	Status      string            `gorm:"size:16;index:idx_segment_task_status_started,priority:1" json:"status"`
	Error       string            `gorm:"type:text" json:"error"`
	Segments    []SegmentTaskSpan `gorm:"serializer:json;type:text" json:"segments"`
	StartedAt   *time.Time        `gorm:"index:idx_segment_task_status_started,priority:2" json:"started_at,omitempty"`
	FinishedAt  *time.Time        `json:"finished_at,omitempty"`
}

type SegmentTaskSpan struct {
	Kind    string `json:"kind"`
	StartMs int64  `json:"start_ms"`
	EndMs   int64  `json:"end_ms"`
}
