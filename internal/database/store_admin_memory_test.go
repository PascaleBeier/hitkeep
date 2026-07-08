package database

import (
	"context"
	"testing"
)

func TestGetDuckDBMemoryStats(t *testing.T) {
	ctx := context.Background()
	store := newSharedTestStore(t)

	stats, err := store.GetDuckDBMemoryStats(ctx)
	if err != nil {
		t.Fatalf("GetDuckDBMemoryStats: %v", err)
	}
	if len(stats) == 0 {
		t.Fatal("expected at least one memory stat row")
	}
	for _, stat := range stats {
		if stat.Tag == "" {
			t.Fatal("expected every memory stat to carry a tag")
		}
		if stat.MemoryBytes < 0 {
			t.Fatalf("expected non-negative memory bytes for %s, got %d", stat.Tag, stat.MemoryBytes)
		}
	}
}
