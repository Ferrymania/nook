package service

import "testing"

func TestLibraryScanPrefersActiveRootsAndReturnsIndependentSnapshot(t *testing.T) {
	tracker := NewTaskTrackerService(nil, nil)
	first := tracker.Start(TaskKindScan, "first root", TaskUpdate{LibraryID: "A", Metrics: map[string]int64{"visited": 1}})
	second := tracker.Start(TaskKindScan, "second root", TaskUpdate{LibraryID: "A"})
	tracker.Start(TaskKindScan, "other library", TaskUpdate{LibraryID: "B"})
	tracker.Start(TaskKindScrape, "scrape", TaskUpdate{LibraryID: "A"})
	second.Finish(nil, TaskUpdate{})
	snapshot := tracker.LibraryScan("A")
	if snapshot == nil || snapshot.ID != first.ID() || snapshot.Status != TaskStatusRunning {
		t.Fatalf("running root lost: %#v", snapshot)
	}
	snapshot.Metrics["visited"] = 999
	if tracker.LibraryScan("A").Metrics["visited"] != 1 {
		t.Fatal("snapshot mutated live task")
	}
	first.Finish(nil, TaskUpdate{})
	if got := tracker.LibraryScan("A"); got == nil || got.ID != first.ID() || got.Status != TaskStatusCompleted {
		t.Fatalf("completion lost: %#v", got)
	}
	if tracker.LibraryScan("missing") != nil {
		t.Fatal("cross-library task leak")
	}
}
