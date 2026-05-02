package importables

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hitkeep/internal/api"
)

func TestSimpleAnalyticsFixtureCSVValidatesAndImports(t *testing.T) {
	fixtureCSV := simpleAnalyticsFixtureCSV(t)
	source := SourceSet{
		SourceHash: "simpleanalytics-fixture",
		SiteDomain: "example.com",
		Files: []SourceFile{{
			Name:      filepath.Base(fixtureCSV),
			Path:      fixtureCSV,
			SizeBytes: fileSize(t, fixtureCSV),
		}},
	}

	provider := NewSimpleAnalyticsProvider()
	manifest, err := provider.Validate(context.Background(), source)
	if err != nil {
		t.Fatalf("validate simple analytics fixture csv: %v", err)
	}
	assertSimpleAnalyticsFixtureManifest(t, manifest)

	sink := &simpleAnalyticsCaptureSink{}
	importManifest, err := provider.Import(context.Background(), source, sink)
	if err != nil {
		t.Fatalf("import simple analytics fixture csv: %v", err)
	}
	assertSimpleAnalyticsFixtureManifest(t, importManifest)
	if len(sink.trafficRows) != 2 {
		t.Fatalf("expected 2 daily traffic rows, got %d", len(sink.trafficRows))
	}
	if len(sink.dimensionRows) == 0 {
		t.Fatalf("expected dimension rows from fixture import")
	}
	requireNoSimpleAnalyticsDimension(t, sink, "source", "example.com", 1)
}

func simpleAnalyticsFixtureCSV(t *testing.T) string {
	t.Helper()
	path := os.Getenv("HITKEEP_SIMPLEANALYTICS_FIXTURE_CSV")
	if path == "" {
		path = repoFixturePath(t, "tests", "fixtures", "imports", "simple-analytics", "datapoints.csv")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Simple Analytics fixture CSV is not readable: %v", err)
	}
	return path
}

func assertSimpleAnalyticsFixtureManifest(t *testing.T, manifest *api.ImportManifest) {
	t.Helper()
	if manifest.RowsScanned != 4 || manifest.RowsAccepted != 3 || manifest.RowsSkipped != 1 {
		t.Fatalf("unexpected fixture row counts: accepted=%d skipped=%d", manifest.RowsAccepted, manifest.RowsSkipped)
	}
	if len(manifest.Datasets) != 1 {
		t.Fatalf("expected one fixture dataset, got %+v", manifest.Datasets)
	}
	dataset := manifest.Datasets[0]
	if dataset.Pageviews != 3 || dataset.Visitors != 2 {
		t.Fatalf("unexpected fixture dataset metrics: %+v", dataset)
	}
	if manifest.DateStart == nil || manifest.DateStart.Format(time.DateOnly) != "2026-04-01" {
		t.Fatalf("unexpected fixture date start: %v", manifest.DateStart)
	}
	if manifest.DateEnd == nil || manifest.DateEnd.Format(time.DateOnly) != "2026-04-02" {
		t.Fatalf("unexpected fixture date end: %v", manifest.DateEnd)
	}
}
