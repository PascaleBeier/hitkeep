package api

import (
	"encoding/json"
	"testing"
	"time"
)

func TestGoogleSearchConsoleSyncStatusImportedDatesEncodeAsDateOnly(t *testing.T) {
	importedStart := DateOnly(time.Date(2026, 5, 1, 15, 30, 0, 0, time.UTC))
	importedEnd := DateOnly(time.Date(2026, 5, 3, 23, 59, 0, 0, time.UTC))
	payload := GoogleSearchConsoleSyncStatus{
		State:             "succeeded",
		ImportedStartDate: &importedStart,
		ImportedEndDate:   &importedEnd,
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal sync status: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal sync status: %v", err)
	}
	if decoded["imported_start_date"] != "2026-05-01" {
		t.Fatalf("expected date-only imported_start_date, got %v in %s", decoded["imported_start_date"], encoded)
	}
	if decoded["imported_end_date"] != "2026-05-03" {
		t.Fatalf("expected date-only imported_end_date, got %v in %s", decoded["imported_end_date"], encoded)
	}
}
