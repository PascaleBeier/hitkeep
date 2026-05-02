package importables

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"

	"hitkeep/internal/api"
)

func TestPlausibleValidateZipIncludesEvents(t *testing.T) {
	path := writePlausibleZip(t, map[string]string{
		"traffic.csv":    `"date","visitors","pageviews","bounces","visits","visit_duration"` + "\n" + `"2026-04-01",2,5,1,3,42` + "\n",
		"events.csv":     `"date","name","link_url","path","visitors","events"` + "\n" + `"2026-04-01","Outbound Link: Click","https://example.com","",1,1` + "\n" + `"2026-04-02","signup","","/pricing",2,3` + "\n",
		"properties.csv": `"date","property","value","visitors","events"` + "\n" + `"2026-04-01","url","https://example.com",1,1` + "\n",
	})

	provider := NewPlausibleProvider()
	manifest, err := provider.Validate(context.Background(), SourceSet{
		SourceHash: "test",
		Files: []SourceFile{{
			Name:      filepath.Base(path),
			Path:      path,
			SizeBytes: fileSize(t, path),
		}},
	})
	if err != nil {
		t.Fatalf("validate plausible zip: %v", err)
	}

	if manifest.RowsAccepted != 4 {
		t.Fatalf("expected 4 accepted rows, got %d", manifest.RowsAccepted)
	}
	if manifest.EventCoverage.RowsScanned != 2 || manifest.EventCoverage.RowsAccepted != 2 {
		t.Fatalf("expected only custom_events rows in event coverage, got %+v", manifest.EventCoverage)
	}
	if manifest.EventCoverage.Events != 4 {
		t.Fatalf("expected 4 queryable events, got %d", manifest.EventCoverage.Events)
	}
	if got := manifest.EventCoverage.EventNames; len(got) != 2 || got[0] != "outbound_click" || got[1] != "signup" {
		t.Fatalf("unexpected event names: %#v", got)
	}
	if manifest.EventPropertyCoverage.UnattributedRows != 1 || manifest.EventPropertyCoverage.UnattributedEvents != 1 {
		t.Fatalf("expected unattributed custom prop row")
	}
}

func TestPlausibleImportLooseCSV(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "traffic.csv")
	if err := os.WriteFile(path, []byte(`"date","hostname","page","visits","visitors","pageviews","total_scroll_depth","total_scroll_depth_visits","total_time_on_page","total_time_on_page_visits"`+"\n"+`"2026-04-01","example.com","/pricing",3,2,8,0,0,0,0`+"\n"), 0600); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	sink := &captureSink{}
	provider := NewPlausibleProvider()
	manifest, err := provider.Import(context.Background(), SourceSet{
		SourceHash: "test",
		Files: []SourceFile{{
			Name:      filepath.Base(path),
			Path:      path,
			SizeBytes: fileSize(t, path),
		}},
	}, sink)
	if err != nil {
		t.Fatalf("import plausible csv: %v", err)
	}
	if manifest.RowsAccepted != 1 || sink.dimensions != 1 {
		t.Fatalf("expected one imported dimension row, manifest=%d sink=%d", manifest.RowsAccepted, sink.dimensions)
	}
}

func TestPlausibleImportPagesWithoutTimeOnPageColumns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pages-export.csv")
	if err := os.WriteFile(path, []byte(`"date","hostname","page","visits","visitors","pageviews","total_scroll_depth","total_scroll_depth_visits"`+"\n"+`"2026-04-01","example.com","/pricing",3,2,8,0,0`+"\n"), 0600); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	sink := &captureSink{}
	provider := NewPlausibleProvider()
	manifest, err := provider.Import(context.Background(), SourceSet{
		SourceHash: "test",
		Files: []SourceFile{{
			Name:      filepath.Base(path),
			Path:      path,
			SizeBytes: fileSize(t, path),
		}},
	}, sink)
	if err != nil {
		t.Fatalf("import plausible csv without time columns: %v", err)
	}
	if manifest.RowsAccepted != 1 || sink.dimensions != 1 {
		t.Fatalf("expected one imported dimension row, manifest=%d sink=%d", manifest.RowsAccepted, sink.dimensions)
	}
}

func TestPlausibleValidateIgnoresUnsupportedSchemas(t *testing.T) {
	dir := t.TempDir()
	validPath := filepath.Join(dir, "traffic.csv")
	writeFile(t, validPath, `"date","visitors","pageviews","bounces","visits","visit_duration"`+"\n"+`"2026-04-01",2,5,1,3,42`+"\n")
	unsupportedPath := filepath.Join(dir, "notes.csv")
	writeFile(t, unsupportedPath, `"date","notes"`+"\n"+`"2026-04-01","not an export"`+"\n")

	provider := NewPlausibleProvider()
	manifest, err := provider.Validate(context.Background(), SourceSet{
		SourceHash: "test",
		Files: []SourceFile{
			{Name: filepath.Base(validPath), Path: validPath, SizeBytes: fileSize(t, validPath)},
			{Name: filepath.Base(unsupportedPath), Path: unsupportedPath, SizeBytes: fileSize(t, unsupportedPath)},
		},
	})
	if err != nil {
		t.Fatalf("validate plausible mixed csvs: %v", err)
	}
	requirePlausibleRowsAccepted(t, manifest, 1)
	requireIgnoredFile(t, manifest, "notes.csv")
	requireWarningCode(t, manifest, "unsupported_schema")
}

func TestPlausibleValidateFailsWithoutRecognizedSchemas(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.csv")
	writeFile(t, path, `"date","notes"`+"\n"+`"2026-04-01","not an export"`+"\n")

	provider := NewPlausibleProvider()
	_, err := provider.Validate(context.Background(), SourceSet{
		SourceHash: "test",
		Files: []SourceFile{{
			Name:      filepath.Base(path),
			Path:      path,
			SizeBytes: fileSize(t, path),
		}},
	})
	if err == nil || err.Error() != "no recognized Plausible CSV schemas found" {
		t.Fatalf("expected no recognized schemas error, got %v", err)
	}
}

func TestNormalizePlausibleEventName(t *testing.T) {
	tests := map[string]string{
		"Outbound Link: Click": "outbound_click",
		"File Download":        "file_download",
		"Form: Submission":     "form_submit",
		"engagement":           "engagement",
		"Signup":               "Signup",
	}

	for input, want := range tests {
		if got := normalizePlausibleEventName(input); got != want {
			t.Fatalf("normalizePlausibleEventName(%q) = %q, want %q", input, got, want)
		}
	}
}

type captureSink struct {
	traffic    int
	dimensions int
	events     int
	properties int
}

func (s *captureSink) PutTraffic(context.Context, TrafficRow) error {
	s.traffic++
	return nil
}
func (s *captureSink) PutDimension(context.Context, DimensionRow) error {
	s.dimensions++
	return nil
}
func (s *captureSink) PutEvent(context.Context, EventRow) error {
	s.events++
	return nil
}
func (s *captureSink) PutEventProperty(context.Context, EventPropertyRow) error {
	s.properties++
	return nil
}
func (s *captureSink) Flush(context.Context) error {
	return nil
}

func writePlausibleZip(t *testing.T, files map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plausible.zip")
	out, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	zw := zip.NewWriter(out)
	for name, content := range files {
		entry, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}
	return path
}

func writeFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.Size()
}

func requirePlausibleRowsAccepted(t *testing.T, manifest *api.ImportManifest, want int64) {
	t.Helper()
	if manifest.RowsAccepted != want {
		t.Fatalf("expected %d accepted row(s), got %d", want, manifest.RowsAccepted)
	}
}

func requireIgnoredFile(t *testing.T, manifest *api.ImportManifest, filename string) {
	t.Helper()
	if len(manifest.IgnoredFiles) != 1 || manifest.IgnoredFiles[0] != filename {
		t.Fatalf("expected ignored file %q, got %#v", filename, manifest.IgnoredFiles)
	}
}

func requireWarningCode(t *testing.T, manifest *api.ImportManifest, code string) {
	t.Helper()
	if len(manifest.Warnings) == 0 || manifest.Warnings[0].Code != code {
		t.Fatalf("expected %s warning, got %#v", code, manifest.Warnings)
	}
}
