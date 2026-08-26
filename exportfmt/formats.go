package exportfmt

import (
	"path/filepath"
	"strings"
)

const (
	FormatXLSX    = "xlsx"
	FormatCSV     = "csv"
	FormatParquet = "parquet"
	FormatJSON    = "json"
	FormatNDJSON  = "ndjson"
)

var supportedFormats = []string{
	FormatXLSX,
	FormatCSV,
	FormatParquet,
	FormatJSON,
	FormatNDJSON,
}

func SupportedFormats() []string {
	out := make([]string, len(supportedFormats))
	copy(out, supportedFormats)
	return out
}

func IsSupported(format string) bool {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case FormatXLSX, FormatCSV, FormatParquet, FormatJSON, FormatNDJSON:
		return true
	default:
		return false
	}
}

func Normalize(format, fallback string) string {
	normalized := strings.ToLower(strings.TrimSpace(format))
	if IsSupported(normalized) {
		return normalized
	}
	if IsSupported(fallback) {
		return strings.ToLower(strings.TrimSpace(fallback))
	}
	return FormatXLSX
}

func DuckDBCopyOptions(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case FormatCSV:
		return "CSV, HEADER"
	case FormatParquet:
		return "PARQUET, COMPRESSION 'SNAPPY'"
	case FormatJSON:
		return "JSON, ARRAY true"
	case FormatNDJSON:
		return "JSON"
	case FormatXLSX:
		fallthrough
	default:
		return "XLSX, HEADER true"
	}
}

func ContentType(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case FormatXLSX:
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case FormatCSV:
		return "text/csv; charset=utf-8"
	case FormatJSON:
		return "application/json; charset=utf-8"
	case FormatNDJSON:
		return "application/x-ndjson; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

func ContentTypeForFilename(filename string) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), ".")
	if ext == "" {
		return "application/octet-stream"
	}
	return ContentType(ext)
}
