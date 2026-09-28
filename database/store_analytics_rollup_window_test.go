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

func TestTruncUnitForRangeUsesDenseShortBuckets(t *testing.T) {
	end := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		from time.Time
		want string
	}{
		{name: "last hour", from: end.Add(-time.Hour), want: "5minute"},
		{name: "six hours", from: end.Add(-6 * time.Hour), want: "15minute"},
		{name: "three days", from: end.Add(-72 * time.Hour), want: "day"},
		{name: "thirty days", from: end.Add(-30 * 24 * time.Hour), want: "day"},
		{name: "one year", from: end.Add(-365 * 24 * time.Hour), want: "month"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncUnitForRange(tt.from, end); got != tt.want {
				t.Fatalf("truncUnitForRange() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildSeriesBucketsSupportsFiveMinuteBuckets(t *testing.T) {
	start := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 20, 11, 0, 0, 0, time.UTC)

	got := buildSeriesBuckets(start, end, "5minute")
	if len(got) != 13 {
		t.Fatalf("got %d buckets, want 13", len(got))
	}
	if !got[0].Equal(start) || !got[len(got)-1].Equal(end) {
		t.Fatalf("unexpected bucket boundaries: first=%s last=%s", got[0], got[len(got)-1])
	}
}
