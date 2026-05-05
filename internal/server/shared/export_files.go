package shared

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ServeTempExportFile serves an app-created export under os.TempDir.
func ServeTempExportFile(w http.ResponseWriter, r *http.Request, filename, downloadName, contentType, cleanupPrefix string) bool {
	cleaned, ok := cleanTempExportPath(filename, cleanupPrefix)
	if !ok {
		http.Error(w, "Failed to export file", http.StatusInternalServerError)
		return false
	}

	file, err := os.Open(cleaned) //nolint:gosec // cleaned path is constrained to an app-owned temp export with the expected prefix.
	if err != nil {
		slog.Error("Failed to open export file", "error", err, "filename", filepath.Base(cleaned))
		http.Error(w, "Failed to export file", http.StatusInternalServerError)
		return false
	}
	defer func() {
		if err := file.Close(); err != nil {
			slog.Warn("Failed to close export file", "error", err, "filename", filepath.Base(cleaned))
		}
	}()

	info, err := file.Stat()
	if err != nil {
		slog.Error("Failed to stat export file", "error", err, "filename", filepath.Base(cleaned))
		http.Error(w, "Failed to export file", http.StatusInternalServerError)
		return false
	}

	w.Header().Set("Content-Disposition", "attachment; filename="+downloadName)
	w.Header().Set("Content-Type", contentType)
	http.ServeContent(w, r, downloadName, info.ModTime(), file)
	return true
}

func cleanTempExportPath(filename, cleanupPrefix string) (string, bool) {
	if filename == "" || cleanupPrefix == "" {
		return "", false
	}

	cleaned := filepath.Clean(filename)
	base := filepath.Base(cleaned)
	if !strings.HasPrefix(base, cleanupPrefix) {
		return "", false
	}

	tempDir := filepath.Clean(os.TempDir())
	rel, err := filepath.Rel(tempDir, cleaned)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return "", false
	}

	return cleaned, true
}
