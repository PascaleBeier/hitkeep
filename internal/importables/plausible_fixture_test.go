package importables

import (
	"archive/zip"
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"hitkeep/internal/api"
)

const plausibleFixtureZipEnv = "HITKEEP_PLAUSIBLE_FIXTURE_ZIP"
const plausibleFixtureCSVDirEnv = "HITKEEP_PLAUSIBLE_FIXTURE_CSV_DIR"

func TestPlausibleFixtureZipValidatesAndImports(t *testing.T) {
	fixtureZip := plausibleFixtureZip(t)

	source := SourceSet{
		SourceHash: "fixture",
		Files: []SourceFile{{
			Name:      filepath.Base(fixtureZip),
			Path:      fixtureZip,
			SizeBytes: fileSize(t, fixtureZip),
		}},
	}

	provider := NewPlausibleProvider()
	manifest, err := provider.Validate(context.Background(), source)
	if err != nil {
		t.Fatalf("validate plausible fixture zip: %v", err)
	}
	assertPlausibleFixtureManifest(t, manifest)

	sink := &captureSink{}
	importManifest, err := provider.Import(context.Background(), source, sink)
	if err != nil {
		t.Fatalf("import plausible fixture zip: %v", err)
	}
	assertPlausibleFixtureManifest(t, importManifest)
	assertPlausibleFixtureSink(t, sink)
}

func TestPlausibleFixtureLooseCSVValidatesAndImports(t *testing.T) {
	source := plausibleFixtureLooseCSVSourceSet(t)

	provider := NewPlausibleProvider()
	manifest, err := provider.Validate(context.Background(), source)
	if err != nil {
		t.Fatalf("validate plausible fixture csv files: %v", err)
	}
	assertPlausibleFixtureManifest(t, manifest)

	sink := &captureSink{}
	importManifest, err := provider.Import(context.Background(), source, sink)
	if err != nil {
		t.Fatalf("import plausible fixture csv files: %v", err)
	}
	assertPlausibleFixtureManifest(t, importManifest)
	assertPlausibleFixtureSink(t, sink)
}

func plausibleFixtureZip(t *testing.T) string {
	t.Helper()
	if path := os.Getenv(plausibleFixtureZipEnv); path != "" {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s is not readable: %v", plausibleFixtureZipEnv, err)
		}
		return path
	}
	return zipFixtureCSVDir(t, plausibleFixtureDir(t))
}

func plausibleFixtureLooseCSVSourceSet(t *testing.T) SourceSet {
	t.Helper()
	if dir := os.Getenv(plausibleFixtureCSVDirEnv); dir != "" {
		return sourceSetFromCSVDir(t, dir)
	}
	if os.Getenv(plausibleFixtureZipEnv) != "" {
		return extractFixtureCSVSourceSet(t, plausibleFixtureZip(t))
	}
	return sourceSetFromCSVDir(t, plausibleFixtureDir(t))
}

func plausibleFixtureDir(t *testing.T) string {
	t.Helper()
	dir := repoFixturePath(t, "tests", "fixtures", "imports", "plausible")
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		t.Fatalf("versioned Plausible fixture directory is not readable: %v", err)
	}
	return dir
}

func sourceSetFromCSVDir(t *testing.T, dir string) SourceSet {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "*.csv"))
	if err != nil {
		t.Fatalf("glob fixture csv dir: %v", err)
	}
	if len(paths) == 0 {
		t.Fatalf("no CSV fixture files found in %s", dir)
	}
	sort.Strings(paths)
	files := make([]SourceFile, 0, len(paths))
	for _, path := range paths {
		files = append(files, SourceFile{
			Name:      filepath.Base(path),
			Path:      path,
			SizeBytes: fileSize(t, path),
		})
	}
	return SourceSet{SourceHash: "fixture-csv", Files: files}
}

func zipFixtureCSVDir(t *testing.T, dir string) string {
	t.Helper()
	source := sourceSetFromCSVDir(t, dir)
	path := filepath.Join(t.TempDir(), "plausible-export.zip")
	out, err := os.Create(path)
	if err != nil {
		t.Fatalf("create Plausible fixture zip: %v", err)
	}
	zw := zip.NewWriter(out)
	for _, file := range source.Files {
		entry, err := zw.Create(file.Name)
		if err != nil {
			_ = out.Close()
			t.Fatalf("create fixture zip entry: %v", err)
		}
		input, err := os.Open(file.Path)
		if err != nil {
			_ = out.Close()
			t.Fatalf("open fixture csv %s: %v", file.Path, err)
		}
		if _, err := io.Copy(entry, input); err != nil {
			_ = input.Close()
			_ = out.Close()
			t.Fatalf("write fixture zip entry %s: %v", file.Name, err)
		}
		if err := input.Close(); err != nil {
			_ = out.Close()
			t.Fatalf("close fixture csv %s: %v", file.Path, err)
		}
	}
	if err := zw.Close(); err != nil {
		_ = out.Close()
		t.Fatalf("close fixture zip writer: %v", err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("close fixture zip file: %v", err)
	}
	return path
}

func extractFixtureCSVSourceSet(t *testing.T, fixtureZip string) SourceSet {
	t.Helper()
	reader, err := zip.OpenReader(fixtureZip)
	if err != nil {
		t.Fatalf("open fixture zip: %v", err)
	}
	defer reader.Close()

	dir := t.TempDir()
	files := make([]SourceFile, 0, len(reader.File))
	for _, entry := range reader.File {
		name := filepath.Base(entry.Name)
		if strings.ToLower(filepath.Ext(name)) != ".csv" {
			continue
		}
		src, err := entry.Open()
		if err != nil {
			t.Fatalf("open zip entry %s: %v", entry.Name, err)
		}
		path := filepath.Join(dir, name)
		dst, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
		if err != nil {
			_ = src.Close()
			t.Fatalf("create extracted csv %s: %v", name, err)
		}
		if _, err := io.Copy(dst, src); err != nil {
			_ = dst.Close()
			_ = src.Close()
			t.Fatalf("copy zip entry %s: %v", entry.Name, err)
		}
		if err := dst.Close(); err != nil {
			_ = src.Close()
			t.Fatalf("close extracted csv %s: %v", name, err)
		}
		if err := src.Close(); err != nil {
			t.Fatalf("close zip entry %s: %v", entry.Name, err)
		}
		files = append(files, SourceFile{Name: name, Path: path, SizeBytes: fileSize(t, path)})
	}
	if len(files) == 0 {
		t.Fatalf("fixture zip contains no CSV files")
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return SourceSet{SourceHash: "fixture-csv", Files: files}
}

func assertPlausibleFixtureManifest(t *testing.T, manifest *api.ImportManifest) {
	t.Helper()
	if manifest.RowsScanned != 16 || manifest.RowsAccepted != 16 || manifest.RowsSkipped != 0 {
		t.Fatalf("unexpected rows: scanned=%d accepted=%d skipped=%d", manifest.RowsScanned, manifest.RowsAccepted, manifest.RowsSkipped)
	}
	if len(manifest.Files) != 11 || len(manifest.IgnoredFiles) != 0 || len(manifest.MissingFiles) != 0 {
		t.Fatalf("unexpected file coverage: files=%d ignored=%v missing=%v", len(manifest.Files), manifest.IgnoredFiles, manifest.MissingFiles)
	}
	if manifest.DateStart == nil || manifest.DateEnd == nil {
		t.Fatalf("expected date range")
	}
	expectedStart := time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)
	expectedEnd := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
	if !manifest.DateStart.Equal(expectedStart) || !manifest.DateEnd.Equal(expectedEnd) {
		t.Fatalf("unexpected date range: %v - %v", manifest.DateStart, manifest.DateEnd)
	}
	coverage := manifest.EventCoverage
	if coverage.RowsScanned != 2 || coverage.RowsAccepted != 2 || coverage.Events != 8 || coverage.Visitors != 6 {
		t.Fatalf("unexpected event coverage: %+v", coverage)
	}
	if len(coverage.EventNames) != 2 || coverage.EventNames[0] != "engagement" || coverage.EventNames[1] != "outbound_click" {
		t.Fatalf("unexpected event names: %v", coverage.EventNames)
	}
	if len(coverage.PropertyKeys) != 2 || coverage.PropertyKeys[0] != "path" || coverage.PropertyKeys[1] != "url" {
		t.Fatalf("unexpected property keys: %v", coverage.PropertyKeys)
	}
	propertyCoverage := manifest.EventPropertyCoverage
	if propertyCoverage.UnattributedRows != 2 || propertyCoverage.UnattributedEvents != 5 {
		t.Fatalf("unexpected unattributed property coverage: %+v", propertyCoverage)
	}
}

func assertPlausibleFixtureSink(t *testing.T, sink *captureSink) {
	t.Helper()
	if sink.traffic != 2 || sink.dimensions != 10 || sink.events != 2 || sink.properties != 5 {
		t.Fatalf("unexpected imported sink rows: traffic=%d dimensions=%d events=%d properties=%d", sink.traffic, sink.dimensions, sink.events, sink.properties)
	}
}
