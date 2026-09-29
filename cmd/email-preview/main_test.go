package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBaselineCheckRejectsNewStaleAndUnjustifiedWarnings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	seen := map[string][]string{"css-border": {"password_reset"}, "css-width": {"site_report"}}

	if err := updateBaseline(path, seen); err != nil {
		t.Fatal(err)
	}
	if err := checkBaseline(path, seen); err == nil || !strings.Contains(err.Error(), "no justification") {
		t.Fatalf("fresh baseline must require justifications, got %v", err)
	}

	if err := os.WriteFile(path, []byte(`{"css-border":"ok","css-width":"ok","css-stale":"ok"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	seen["html-new"] = []string{"site_report"}
	err := checkBaseline(path, seen)
	if err == nil || !strings.Contains(err.Error(), `new client compatibility warning "html-new" in site_report`) || !strings.Contains(err.Error(), `"css-stale" no longer occurs`) {
		t.Fatalf("expected new and stale warnings, got %v", err)
	}

	if err := updateBaseline(path, seen); err != nil {
		t.Fatal(err)
	}
	baseline, err := readBaseline(path)
	if err != nil {
		t.Fatal(err)
	}
	if baseline["css-border"] != "ok" || baseline["html-new"] != todoJustification || len(baseline) != 3 {
		t.Fatalf("update must keep justifications, add TODOs, and drop stale slugs: %v", baseline)
	}
}
