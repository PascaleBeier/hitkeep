package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"hitkeep/exportfmt"
)

func (s *Store) exportQueryToTempFile(
	ctx context.Context,
	filenamePattern string,
	cleanupPrefix string,
	selectQuery string,
	args []any,
	format string,
) (string, error) {
	normalizedFormat := exportfmt.Normalize(format, exportfmt.FormatCSV)
	tmpFile, err := os.CreateTemp("", fmt.Sprintf("%s*.%s", filenamePattern, normalizedFormat))
	if err != nil {
		return "", fmt.Errorf("failed to create export file: %w", err)
	}

	filename := tmpFile.Name()
	if err := tmpFile.Close(); err != nil {
		cleanupTempExportFile(filename, cleanupPrefix)
		return "", fmt.Errorf("failed to close export file: %w", err)
	}

	duckFormat := exportfmt.DuckDBCopyOptions(normalizedFormat)
	//nolint:gosec // filename is generated locally; selectQuery uses parameter placeholders.
	copyQuery := fmt.Sprintf("COPY (%s) TO '%s' (FORMAT %s);", selectQuery, filename, duckFormat)
	err = s.WithDuckDBSession(ctx, DuckDBSessionOptions{
		Excel: normalizedFormat == exportfmt.FormatXLSX,
	}, func(conn *sql.Conn) error {
		if _, err := conn.ExecContext(ctx, copyQuery, args...); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		cleanupTempExportFile(filename, cleanupPrefix)
		return "", err
	}

	return filename, nil
}

func cleanupTempExportFile(filename, prefix string) {
	if filename == "" || prefix == "" {
		return
	}

	cleaned := filepath.Clean(filename)
	base := filepath.Base(cleaned)
	if !strings.HasPrefix(base, prefix) {
		return
	}

	tempDir := filepath.Clean(os.TempDir())
	rel, err := filepath.Rel(tempDir, cleaned)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return
	}

	//nolint:gosec // cleaned path is constrained to an app-owned temp file under os.TempDir.
	_ = os.Remove(cleaned)
}
