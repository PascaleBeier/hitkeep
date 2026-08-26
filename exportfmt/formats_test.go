package exportfmt

import "testing"

func TestNormalize(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		fallback string
		want     string
	}{
		{name: "supported lowercase", format: "json", fallback: FormatXLSX, want: FormatJSON},
		{name: "supported uppercase", format: "NDJSON", fallback: FormatXLSX, want: FormatNDJSON},
		{name: "unknown uses fallback", format: "xml", fallback: FormatCSV, want: FormatCSV},
		{name: "bad fallback uses xlsx", format: "xml", fallback: "bad", want: FormatXLSX},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Normalize(tc.format, tc.fallback); got != tc.want {
				t.Fatalf("expected normalized format %q, got %q", tc.want, got)
			}
		})
	}
}

func TestDuckDBCopyOptions(t *testing.T) {
	tests := []struct {
		format string
		want   string
	}{
		{format: FormatCSV, want: "CSV, HEADER"},
		{format: FormatXLSX, want: "XLSX, HEADER true"},
		{format: FormatParquet, want: "PARQUET, COMPRESSION 'SNAPPY'"},
		{format: FormatJSON, want: "JSON, ARRAY true"},
		{format: FormatNDJSON, want: "JSON"},
	}

	for _, tc := range tests {
		if got := DuckDBCopyOptions(tc.format); got != tc.want {
			t.Fatalf("expected copy option %q for format %q, got %q", tc.want, tc.format, got)
		}
	}
}

func TestContentTypeForFilename(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{filename: "report.xlsx", want: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		{filename: "report.csv", want: "text/csv; charset=utf-8"},
		{filename: "report.parquet", want: "application/octet-stream"},
		{filename: "report.json", want: "application/json; charset=utf-8"},
		{filename: "report.ndjson", want: "application/x-ndjson; charset=utf-8"},
		{filename: "report.bin", want: "application/octet-stream"},
	}

	for _, tc := range tests {
		if got := ContentTypeForFilename(tc.filename); got != tc.want {
			t.Fatalf("expected content type %q for file %q, got %q", tc.want, tc.filename, got)
		}
	}
}
