package database

import (
	"testing"
	"time"
)

func TestBuildRollupWindowSplitsPartialEdges(t *testing.T) {
	start := time.Date(2026, 4, 20, 10, 15, 0, 0, time.UTC)
	end := time.Date(2026, 4, 20, 13, 45, 0, 0, time.UTC)

	window := buildRollupWindow(start, end, "hour")

	if !window.UseRollup {
		t.Fatalf("expected aligned middle buckets to use rollups")
	}
	if !window.FullStart.Equal(time.Date(2026, 4, 20, 11, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected full start: %s", window.FullStart)
	}
	if !window.FullEnd.Equal(time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected full end: %s", window.FullEnd)
	}
	if window.Leading == nil || !window.Leading.Equal(window.FullStart) {
		t.Fatalf("expected leading edge to end at %s, got %v", window.FullStart, window.Leading)
	}
	trailing := time.Date(2026, 4, 20, 13, 0, 0, 0, time.UTC)
	if window.Trailing == nil || !window.Trailing.Equal(trailing) {
		t.Fatalf("expected trailing edge to start at %s, got %v", trailing, window.Trailing)
	}
}

func TestBuildSeriesBucketsIncludesBoundaryBuckets(t *testing.T) {
	start := time.Date(2026, 4, 20, 10, 15, 0, 0, time.UTC)
	end := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)

	got := buildSeriesBuckets(start, end, "hour")
	want := []time.Time{
		time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 20, 11, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC),
	}

	if len(got) != len(want) {
		t.Fatalf("got %d buckets, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			t.Fatalf("bucket %d = %s, want %s", i, got[i], want[i])
		}
	}
}
