package takeout

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/database"
	"hitkeep/internal/exportfmt"
)

type TakeoutService struct {
	store *database.Store
	path  string
}

type ExportFile struct {
	File *os.File
	Info os.FileInfo
	Name string
}

func NewTakeoutService(store *database.Store, path string) *TakeoutService {
	return &TakeoutService{
		store: store,
		path:  path,
	}
}

func (s *TakeoutService) ExportUserData(ctx context.Context, userID uuid.UUID, format string) (string, error) {
	// Ensure export directory exists
	if err := os.MkdirAll(s.path, 0755); err != nil {
		return "", fmt.Errorf("failed to create export directory: %w", err)
	}

	normalizedFormat := exportfmt.Normalize(format, exportfmt.FormatXLSX)
	filename := filepath.Join(s.path, fmt.Sprintf("user_takeout_%s_%d.%s", userID, time.Now().Unix(), normalizedFormat))
	whereClause := fmt.Sprintf("site_id IN (SELECT site_id FROM site_members WHERE user_id = '%s')", userID)

	return s.exportTakeout(ctx, "user", filename, normalizedFormat, exportfmt.DuckDBCopyOptions(normalizedFormat), whereClause)
}

// ExportSiteData exports active data to the specified format.
// Format is validated by the handler (xlsx, csv, parquet, json, ndjson).
func (s *TakeoutService) ExportSiteData(ctx context.Context, siteID uuid.UUID, format string) (string, error) {
	if err := os.MkdirAll(s.path, 0755); err != nil {
		return "", fmt.Errorf("failed to create export directory: %w", err)
	}

	normalizedFormat := exportfmt.Normalize(format, exportfmt.FormatXLSX)
	filename := filepath.Join(s.path, fmt.Sprintf("site_takeout_%s_%d.%s", siteID, time.Now().Unix(), normalizedFormat))
	whereClause := fmt.Sprintf("site_id = '%s'", siteID)

	return s.exportTakeout(ctx, "site", filename, normalizedFormat, exportfmt.DuckDBCopyOptions(normalizedFormat), whereClause)
}

func (s *TakeoutService) exportTakeout(ctx context.Context, label, filename, normalizedFormat, duckFormat string, whereClause string) (string, error) {
	query := buildTakeoutQuery(whereClause, filename, duckFormat)
	err := s.store.WithDuckDBSession(ctx, database.DuckDBSessionOptions{
		Excel: normalizedFormat == exportfmt.FormatXLSX,
	}, func(conn *sql.Conn) error {
		if _, err := conn.ExecContext(ctx, query); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to export %s data: %w", label, err)
	}

	return filename, nil
}

func (s *TakeoutService) CleanupExportFile(filename string) {
	if filename == "" {
		return
	}

	cleanedFile, ok := s.cleanExportPath(filename)
	if !ok {
		return
	}

	//nolint:gosec // cleaned path is constrained to a takeout export under the configured export directory.
	_ = os.Remove(cleanedFile)
}

func (s *TakeoutService) OpenExportFile(filename string) (*ExportFile, error) {
	cleanedFile, ok := s.cleanExportPath(filename)
	if !ok {
		return nil, fmt.Errorf("invalid takeout export path")
	}

	file, err := os.Open(cleanedFile) //nolint:gosec // cleaned path is constrained to a takeout export under the configured export directory.
	if err != nil {
		return nil, fmt.Errorf("open takeout export: %w", err)
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("stat takeout export: %w", err)
	}

	return &ExportFile{
		File: file,
		Info: info,
		Name: filepath.Base(cleanedFile),
	}, nil
}

func (s *TakeoutService) cleanExportPath(filename string) (string, bool) {
	cleanedFile := filepath.Clean(filename)
	base := filepath.Base(cleanedFile)
	if !strings.HasPrefix(base, "user_takeout_") && !strings.HasPrefix(base, "site_takeout_") {
		return "", false
	}

	exportDir := filepath.Clean(s.path)
	rel, err := filepath.Rel(exportDir, cleanedFile)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return "", false
	}

	return cleanedFile, true
}

func buildTakeoutQuery(whereClause, filename, format string) string {
	return fmt.Sprintf(`
	COPY (
		SELECT 'hit' as record_type, * FROM hits WHERE %s
		UNION BY NAME
		SELECT 'event' as record_type, * FROM events WHERE %s
		UNION BY NAME
		SELECT 'ai_fetch' as record_type, * FROM ai_fetches WHERE %s
		UNION BY NAME
		SELECT 'goal' as record_type, * FROM goals WHERE %s
		UNION BY NAME
		SELECT 'funnel' as record_type, * FROM funnels WHERE %s
	) TO '%s' (FORMAT %s);
`, whereClause, whereClause, whereClause, whereClause, whereClause, filename, format)
}
