package importables

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSimpleAnalyticsValidateDatapointsCSV(t *testing.T) {
	path := writeSimpleAnalyticsCSV(t, "traffic.csv", simpleAnalyticsCSVHeader+
		"2026-04-01T10:00:00.000Z,DE,pageview,desktop,https://google.com/,12,true,/pricing,s1,chatgpt.com\n"+
		"2026-04-01T10:02:00.000Z,DE,pageview,mobile,,4,false,/docs,s2,\n"+
		"2026-04-01T10:03:00.000Z,DE,event,desktop,,1,true,/docs,s3,\n")

	provider := NewSimpleAnalyticsProvider()
	manifest, err := provider.Validate(context.Background(), SourceSet{
		SourceHash: "test",
		Files: []SourceFile{{
			Name:      filepath.Base(path),
			Path:      path,
			SizeBytes: fileSize(t, path),
		}},
	})
	if err != nil {
		t.Fatalf("validate simple analytics csv: %v", err)
	}

	if manifest.Provider != ProviderSimpleAnalytics {
		t.Fatalf("unexpected provider %q", manifest.Provider)
	}
	if manifest.RowsScanned != 3 || manifest.RowsAccepted != 2 || manifest.RowsSkipped != 1 {
		t.Fatalf("unexpected row counts: %+v", manifest)
	}
	if len(manifest.Datasets) != 1 || manifest.Datasets[0].Pageviews != 2 || manifest.Datasets[0].Visitors != 1 {
		t.Fatalf("unexpected dataset summary: %+v", manifest.Datasets)
	}
	if manifest.DateStart == nil || manifest.DateStart.Format(time.DateOnly) != "2026-04-01" {
		t.Fatalf("unexpected date start: %v", manifest.DateStart)
	}
	if len(manifest.Warnings) == 0 {
		t.Fatalf("expected validation warnings for skipped/limited data")
	}
}

func TestSimpleAnalyticsImportDatapointsCSV(t *testing.T) {
	path := writeSimpleAnalyticsCSV(t, "traffic.csv", simpleAnalyticsCSVHeaderWithBrowserLanguage+
		"2026-04-01T10:00:00.000Z,DE,pageview,desktop,https://google.com/,12,true,/pricing,s1,chatgpt.com,Firefox,125.0,de,DE\n"+
		"2026-04-01T10:02:00.000Z,US,pageview,mobile,,4,false,/docs,s2,,Safari,17.4,en,US\n"+
		"2026-04-01T10:03:00.000Z,DE,pageview,desktop,https://example.com/internal,2,true,/pricing,s3,,Firefox,125.0,de,DE\n")

	sink := &simpleAnalyticsCaptureSink{}
	provider := NewSimpleAnalyticsProvider()
	manifest, err := provider.Import(context.Background(), SourceSet{
		SourceHash: "test",
		SiteDomain: "example.com",
		Files: []SourceFile{{
			Name:      filepath.Base(path),
			Path:      path,
			SizeBytes: fileSize(t, path),
		}},
	}, sink)
	if err != nil {
		t.Fatalf("import simple analytics csv: %v", err)
	}
	if manifest.RowsAccepted != 3 {
		t.Fatalf("expected three accepted rows, got %d", manifest.RowsAccepted)
	}
	if len(sink.trafficRows) != 1 {
		t.Fatalf("expected one daily traffic row, got %d", len(sink.trafficRows))
	}
	traffic := sink.trafficRows[0]
	if traffic.Pageviews != 3 || traffic.Visitors != 2 || traffic.Visits != 2 || traffic.VisitDuration != 18 {
		t.Fatalf("unexpected traffic row: %+v", traffic)
	}
	requireSimpleAnalyticsDimension(t, sink, "page", "/pricing", 2)
	requireSimpleAnalyticsDimension(t, sink, "source", "google.com", 1)
	requireNoSimpleAnalyticsDimension(t, sink, "source", "example.com", 1)
	requireSimpleAnalyticsDimension(t, sink, "utm_source", "chatgpt.com", 1)
	requireSimpleAnalyticsDimension(t, sink, "browser", "Firefox", 2)
	requireSimpleAnalyticsDimension(t, sink, "browser", "Safari", 1)
	requireSimpleAnalyticsDimension(t, sink, "language", "de", 2)
	requireSimpleAnalyticsDimension(t, sink, "language", "en", 1)
}

const simpleAnalyticsCSVHeader = "added_iso,country_code,datapoint,device_type,document_referrer,duration_seconds,is_unique,path,session_id,utm_source\n"
const simpleAnalyticsCSVHeaderWithBrowserLanguage = "added_iso,country_code,datapoint,device_type,document_referrer,duration_seconds,is_unique,path,session_id,utm_source,browser_name,browser_version,lang_language,lang_region\n"

func writeSimpleAnalyticsCSV(t *testing.T, name string, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatalf("write simple analytics csv: %v", err)
	}
	return path
}

type simpleAnalyticsCaptureSink struct {
	trafficRows   []TrafficRow
	dimensionRows []DimensionRow
}

func (s *simpleAnalyticsCaptureSink) PutTraffic(_ context.Context, row TrafficRow) error {
	s.trafficRows = append(s.trafficRows, row)
	return nil
}

func (s *simpleAnalyticsCaptureSink) PutDimension(_ context.Context, row DimensionRow) error {
	s.dimensionRows = append(s.dimensionRows, row)
	return nil
}

func (s *simpleAnalyticsCaptureSink) PutEvent(context.Context, EventRow) error {
	return nil
}

func (s *simpleAnalyticsCaptureSink) PutEventProperty(context.Context, EventPropertyRow) error {
	return nil
}

func (s *simpleAnalyticsCaptureSink) Flush(context.Context) error {
	return nil
}

func (s *simpleAnalyticsCaptureSink) hasDimension(dimension string, name string, pageviews int64) bool {
	for _, row := range s.dimensionRows {
		if row.Dimension == dimension && row.Name == name && row.Pageviews == pageviews {
			return true
		}
	}
	return false
}

func requireSimpleAnalyticsDimension(t *testing.T, sink *simpleAnalyticsCaptureSink, dimension string, name string, pageviews int64) {
	t.Helper()
	if !sink.hasDimension(dimension, name, pageviews) {
		t.Fatalf("expected %s dimension %q=%d, got %+v", dimension, name, pageviews, sink.dimensionRows)
	}
}

func requireNoSimpleAnalyticsDimension(t *testing.T, sink *simpleAnalyticsCaptureSink, dimension string, name string, pageviews int64) {
	t.Helper()
	if sink.hasDimension(dimension, name, pageviews) {
		t.Fatalf("did not expect %s dimension %q=%d, got %+v", dimension, name, pageviews, sink.dimensionRows)
	}
}
