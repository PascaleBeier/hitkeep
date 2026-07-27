package controlstore

import (
	"strings"
	"testing"
)

func TestLegacyRegistryIsClosedAndUnique(t *testing.T) {
	t.Parallel()
	live := append(LegacyControlTables(), legacyEmptyAnalyticsTables...)
	live = append(live, legacyReplacedMetadataTables...)
	classified, err := ClassifyLegacyTables(live)
	if err != nil {
		t.Fatal(err)
	}
	if len(classified) != len(live) {
		t.Fatalf("classified=%d live=%d", len(classified), len(live))
	}

	if _, err := ClassifyLegacyTables(append(live, "future_control_table")); err == nil || !strings.Contains(err.Error(), "unclassified") {
		t.Fatalf("unknown table error=%v", err)
	}
	if _, err := ClassifyLegacyTables(live[:len(live)-1]); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("missing table error=%v", err)
	}
}
