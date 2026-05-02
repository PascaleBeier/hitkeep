package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

const (
	ImportStatusUploading        = "uploading"
	ImportStatusValidating       = "validating"
	ImportStatusValidated        = "validated"
	ImportStatusValidationFailed = "validation_failed"
	ImportStatusQueued           = "queued"
	ImportStatusRunning          = "running"
	ImportStatusCompleted        = "completed"
	ImportStatusFailed           = "failed"
	ImportStatusDeleted          = "deleted"

	ImportFileStatusPending  = "pending"
	ImportFileStatusUploaded = "uploaded"

	ImportStageExpiredMessage = "staged import files expired before the import was started"
)

type ImportFileCreate struct {
	ID           uuid.UUID
	Filename     string
	RelativePath string
	SizeBytes    int64
	SHA256       string
}

type StagedImportFile struct {
	api.ImportUploadFile
	RelativePath string
}

type StaleImportStageFile struct {
	ImportID     uuid.UUID
	SiteID       uuid.UUID
	Provider     string
	ImportStatus string
	FileID       uuid.UUID
	Filename     string
	RelativePath string
	SizeBytes    int64
	StaleAt      time.Time
}

func (s *Store) CreateSiteImportUpload(ctx context.Context, siteID, actorID uuid.UUID, provider string, files []ImportFileCreate) (*api.ImportJob, error) {
	now := time.Now().UTC()
	importID := uuid.New()
	var total int64
	for _, file := range files {
		total += file.SizeBytes
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin import upload: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO site_imports (
			id, site_id, provider, status, bytes_total, bytes_received,
			rows_scanned, rows_imported, created_by, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, 0, 0, 0, ?, ?, ?)
	`, importID, siteID, provider, ImportStatusUploading, total, nullableUUID(actorID), now, now); err != nil {
		return nil, fmt.Errorf("create import upload: %w", err)
	}

	apiFiles := make([]api.ImportUploadFile, 0, len(files))
	for _, file := range files {
		if file.ID == uuid.Nil {
			file.ID = uuid.New()
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO site_import_files (
				import_id, file_id, filename, relative_path, size_bytes,
				bytes_received, sha256, status, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, 0, NULLIF(?, ''), ?, ?, ?)
		`, importID, file.ID, file.Filename, file.RelativePath, file.SizeBytes, file.SHA256, ImportFileStatusPending, now, now); err != nil {
			return nil, fmt.Errorf("create import file: %w", err)
		}
		apiFiles = append(apiFiles, api.ImportUploadFile{
			ID:        file.ID,
			Filename:  file.Filename,
			SizeBytes: file.SizeBytes,
			SHA256:    file.SHA256,
			Status:    ImportFileStatusPending,
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit import upload: %w", err)
	}

	var createdBy *uuid.UUID
	if actorID != uuid.Nil {
		createdBy = &actorID
	}

	return &api.ImportJob{
		ID:            importID,
		SiteID:        siteID,
		Provider:      provider,
		Status:        ImportStatusUploading,
		BytesTotal:    total,
		BytesReceived: 0,
		Files:         apiFiles,
		CreatedBy:     createdBy,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

func (s *Store) GetSiteImport(ctx context.Context, siteID, importID uuid.UUID) (*api.ImportJob, error) {
	job, err := s.scanImport(ctx, `
		SELECT id, site_id, provider, status, COALESCE(source_hash, ''), COALESCE(CAST(manifest AS VARCHAR), ''),
			COALESCE(error, ''), bytes_total, bytes_received, rows_scanned, rows_imported,
			CAST(created_by AS VARCHAR), created_at, updated_at, validated_at, started_at, finished_at
		FROM site_imports
		WHERE site_id = ? AND id = ?
	`, siteID, importID)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, fmt.Errorf("import not found")
	}
	files, err := s.ListImportFiles(ctx, importID)
	if err != nil {
		return nil, err
	}
	job.Files = make([]api.ImportUploadFile, 0, len(files))
	for _, file := range files {
		job.Files = append(job.Files, file.ImportUploadFile)
	}
	return job, nil
}

func (s *Store) ListSiteImports(ctx context.Context, siteID uuid.UUID) ([]api.ImportJob, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, site_id, provider, status, COALESCE(source_hash, ''), COALESCE(CAST(manifest AS VARCHAR), ''),
			COALESCE(error, ''), bytes_total, bytes_received, rows_scanned, rows_imported,
			CAST(created_by AS VARCHAR), created_at, updated_at, validated_at, started_at, finished_at
		FROM site_imports
		WHERE site_id = ? AND status <> ?
		ORDER BY created_at DESC
	`, siteID, ImportStatusDeleted)
	if err != nil {
		return nil, fmt.Errorf("list imports: %w", err)
	}
	defer rows.Close()

	imports := []api.ImportJob{}
	for rows.Next() {
		job, err := scanImportRow(rows)
		if err != nil {
			return nil, err
		}
		imports = append(imports, *job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read imports: %w", err)
	}
	return imports, nil
}

func (s *Store) ListRunnableImports(ctx context.Context) ([]api.ImportJob, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, site_id, provider, status, COALESCE(source_hash, ''), COALESCE(CAST(manifest AS VARCHAR), ''),
			COALESCE(error, ''), bytes_total, bytes_received, rows_scanned, rows_imported,
			CAST(created_by AS VARCHAR), created_at, updated_at, validated_at, started_at, finished_at
		FROM site_imports
		WHERE status IN (?, ?)
		ORDER BY updated_at ASC, created_at ASC
	`, ImportStatusQueued, ImportStatusRunning)
	if err != nil {
		return nil, fmt.Errorf("list runnable imports: %w", err)
	}
	defer rows.Close()

	imports := []api.ImportJob{}
	for rows.Next() {
		job, err := scanImportRow(rows)
		if err != nil {
			return nil, err
		}
		imports = append(imports, *job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read runnable imports: %w", err)
	}
	return imports, nil
}

func (s *Store) CompletedImportExistsForSourceHash(ctx context.Context, siteID uuid.UUID, provider string, sourceHash string, excludeImportID uuid.UUID) (bool, error) {
	if sourceHash == "" {
		return false, nil
	}
	var exists bool
	if err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM site_imports
			WHERE site_id = ?
				AND provider = ?
				AND source_hash = ?
				AND status = ?
				AND id <> ?
			LIMIT 1
		)
	`, siteID, provider, sourceHash, ImportStatusCompleted, excludeImportID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check duplicate completed import: %w", err)
	}
	return exists, nil
}

func (s *Store) EstimateStaleImportStageFiles(ctx context.Context, cutoff time.Time) (api.ImportStageCleanupEstimate, error) {
	var (
		imports int64
		files   int64
		bytes   int64
	)
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT i.id), COUNT(f.file_id), COALESCE(SUM(f.size_bytes), 0)
		FROM site_imports i
		JOIN site_import_files f ON f.import_id = i.id
		WHERE f.cleaned_at IS NULL
			AND i.status IN (?, ?, ?, ?, ?, ?)
			AND COALESCE(i.finished_at, i.validated_at, i.updated_at, i.created_at) < ?
	`,
		ImportStatusUploading,
		ImportStatusValidated,
		ImportStatusValidationFailed,
		ImportStatusFailed,
		ImportStatusCompleted,
		ImportStatusDeleted,
		cutoff,
	).Scan(&imports, &files, &bytes); err != nil {
		return api.ImportStageCleanupEstimate{}, fmt.Errorf("estimate stale import stage files: %w", err)
	}
	return api.ImportStageCleanupEstimate{
		Imports: int(imports),
		Files:   int(files),
		Bytes:   bytes,
	}, nil
}

func (s *Store) ListStaleImportStageFiles(ctx context.Context, cutoff time.Time) ([]StaleImportStageFile, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT i.id, i.site_id, i.provider, i.status,
			f.file_id, f.filename, f.relative_path, f.size_bytes,
			COALESCE(i.finished_at, i.validated_at, i.updated_at, i.created_at) AS stale_at
		FROM site_imports i
		JOIN site_import_files f ON f.import_id = i.id
		WHERE f.cleaned_at IS NULL
			AND i.status IN (?, ?, ?, ?, ?, ?)
			AND COALESCE(i.finished_at, i.validated_at, i.updated_at, i.created_at) < ?
		ORDER BY stale_at ASC, i.id, f.filename, f.file_id
	`,
		ImportStatusUploading,
		ImportStatusValidated,
		ImportStatusValidationFailed,
		ImportStatusFailed,
		ImportStatusCompleted,
		ImportStatusDeleted,
		cutoff,
	)
	if err != nil {
		return nil, fmt.Errorf("list stale import stage files: %w", err)
	}
	defer rows.Close()

	files := []StaleImportStageFile{}
	for rows.Next() {
		var file StaleImportStageFile
		if err := rows.Scan(
			&file.ImportID,
			&file.SiteID,
			&file.Provider,
			&file.ImportStatus,
			&file.FileID,
			&file.Filename,
			&file.RelativePath,
			&file.SizeBytes,
			&file.StaleAt,
		); err != nil {
			return nil, fmt.Errorf("scan stale import stage file: %w", err)
		}
		files = append(files, file)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read stale import stage files: %w", err)
	}
	return files, nil
}

func (s *Store) MarkStaleImportExpired(ctx context.Context, importID uuid.UUID, message string, now time.Time) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE site_imports
		SET status = ?, error = ?, updated_at = ?, finished_at = COALESCE(finished_at, ?)
		WHERE id = ?
			AND status IN (?, ?, ?)
	`,
		ImportStatusFailed,
		message,
		now,
		now,
		importID,
		ImportStatusUploading,
		ImportStatusValidated,
		ImportStatusValidationFailed,
	)
	if err != nil {
		return false, fmt.Errorf("mark stale import expired: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read stale import expiration count: %w", err)
	}
	return affected > 0, nil
}

func (s *Store) MarkImportFileCleaned(ctx context.Context, importID, fileID uuid.UUID, cleanedAt time.Time) error {
	if _, err := s.db.ExecContext(ctx, `
		UPDATE site_import_files
		SET cleaned_at = COALESCE(cleaned_at, ?), updated_at = ?
		WHERE import_id = ? AND file_id = ?
	`, cleanedAt, cleanedAt, importID, fileID); err != nil {
		return fmt.Errorf("mark import file cleaned: %w", err)
	}
	return nil
}

func (s *Store) MarkImportFilesCleaned(ctx context.Context, importID uuid.UUID, cleanedAt time.Time) error {
	if _, err := s.db.ExecContext(ctx, `
		UPDATE site_import_files
		SET cleaned_at = COALESCE(cleaned_at, ?), updated_at = ?
		WHERE import_id = ?
	`, cleanedAt, cleanedAt, importID); err != nil {
		return fmt.Errorf("mark import files cleaned: %w", err)
	}
	return nil
}

func (s *Store) scanImport(ctx context.Context, query string, args ...any) (*api.ImportJob, error) {
	row := s.db.QueryRowContext(ctx, query, args...)
	job, err := scanImportRow(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return job, nil
}

type importScanner interface {
	Scan(dest ...any) error
}

func scanImportRow(row importScanner) (*api.ImportJob, error) {
	var (
		job          api.ImportJob
		manifestRaw  string
		createdByRaw sql.NullString
		validatedAt  sql.NullTime
		startedAt    sql.NullTime
		finishedAt   sql.NullTime
	)
	if err := row.Scan(
		&job.ID,
		&job.SiteID,
		&job.Provider,
		&job.Status,
		&job.SourceHash,
		&manifestRaw,
		&job.Error,
		&job.BytesTotal,
		&job.BytesReceived,
		&job.RowsScanned,
		&job.RowsImported,
		&createdByRaw,
		&job.CreatedAt,
		&job.UpdatedAt,
		&validatedAt,
		&startedAt,
		&finishedAt,
	); err != nil {
		return nil, err
	}
	if manifestRaw != "" {
		var manifest api.ImportManifest
		if err := json.Unmarshal([]byte(manifestRaw), &manifest); err == nil {
			job.Manifest = &manifest
		}
	}
	if createdByRaw.Valid && strings.TrimSpace(createdByRaw.String) != "" {
		if createdBy, err := uuid.Parse(createdByRaw.String); err == nil {
			job.CreatedBy = &createdBy
		}
	}
	if validatedAt.Valid {
		job.ValidatedAt = &validatedAt.Time
	}
	if startedAt.Valid {
		job.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		job.FinishedAt = &finishedAt.Time
	}
	return &job, nil
}

func (s *Store) ListImportFiles(ctx context.Context, importID uuid.UUID) ([]StagedImportFile, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT file_id, filename, relative_path, size_bytes, bytes_received, COALESCE(sha256, ''), status
		FROM site_import_files
		WHERE import_id = ?
		ORDER BY filename, file_id
	`, importID)
	if err != nil {
		return nil, fmt.Errorf("list import files: %w", err)
	}
	defer rows.Close()

	files := []StagedImportFile{}
	for rows.Next() {
		var file StagedImportFile
		if err := rows.Scan(&file.ID, &file.Filename, &file.RelativePath, &file.SizeBytes, &file.BytesReceived, &file.SHA256, &file.Status); err != nil {
			return nil, fmt.Errorf("scan import file: %w", err)
		}
		files = append(files, file)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read import files: %w", err)
	}
	return files, nil
}

func (s *Store) GetImportFile(ctx context.Context, importID, fileID uuid.UUID) (*StagedImportFile, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT file_id, filename, relative_path, size_bytes, bytes_received, COALESCE(sha256, ''), status
		FROM site_import_files
		WHERE import_id = ? AND file_id = ?
	`, importID, fileID)
	var file StagedImportFile
	if err := row.Scan(&file.ID, &file.Filename, &file.RelativePath, &file.SizeBytes, &file.BytesReceived, &file.SHA256, &file.Status); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("import file not found")
		}
		return nil, fmt.Errorf("get import file: %w", err)
	}
	return &file, nil
}

func (s *Store) UpdateImportFileProgress(ctx context.Context, importID, fileID uuid.UUID, bytesReceived int64, sha256 string) error {
	file, err := s.GetImportFile(ctx, importID, fileID)
	if err != nil {
		return err
	}
	if bytesReceived < 0 {
		bytesReceived = 0
	}
	if bytesReceived >= file.SizeBytes {
		bytesReceived = file.SizeBytes
	}
	now := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx, `
		UPDATE site_import_files
		SET bytes_received = LEAST(size_bytes, GREATEST(bytes_received, ?)),
			sha256 = COALESCE(NULLIF(?, ''), sha256),
			status = CASE
				WHEN LEAST(size_bytes, GREATEST(bytes_received, ?)) >= size_bytes THEN ?
				ELSE status
			END,
			updated_at = ?
		WHERE import_id = ? AND file_id = ?
	`, bytesReceived, sha256, bytesReceived, ImportFileStatusUploaded, now, importID, fileID); err != nil {
		return fmt.Errorf("update import file progress: %w", err)
	}
	return s.recalculateImportUploadProgress(ctx, importID)
}

func (s *Store) recalculateImportUploadProgress(ctx context.Context, importID uuid.UUID) error {
	now := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx, `
		UPDATE site_imports
		SET bytes_received = COALESCE((SELECT SUM(bytes_received) FROM site_import_files WHERE import_id = ?), 0),
			updated_at = ?
		WHERE id = ?
	`, importID, now, importID); err != nil {
		return fmt.Errorf("recalculate import progress: %w", err)
	}
	return nil
}

func (s *Store) MarkImportValidating(ctx context.Context, siteID, importID uuid.UUID) error {
	return s.updateImportStatus(ctx, siteID, importID, ImportStatusValidating, "", nil)
}

func (s *Store) MarkImportValidationFailed(ctx context.Context, siteID, importID uuid.UUID, errMsg string) error {
	return s.updateImportStatus(ctx, siteID, importID, ImportStatusValidationFailed, errMsg, nil)
}

func (s *Store) MarkImportValidated(ctx context.Context, siteID, importID uuid.UUID, sourceHash string, manifest *api.ImportManifest) error {
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshal import manifest: %w", err)
	}
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
		UPDATE site_imports
		SET status = ?, source_hash = ?, manifest = CAST(? AS JSON), error = NULL,
			rows_scanned = ?, updated_at = ?, validated_at = ?
		WHERE site_id = ? AND id = ?
	`, ImportStatusValidated, sourceHash, string(manifestJSON), manifest.RowsScanned, now, now, siteID, importID)
	if err != nil {
		return fmt.Errorf("mark import validated: %w", err)
	}
	return nil
}

func (s *Store) MarkImportQueued(ctx context.Context, siteID, importID uuid.UUID) error {
	return s.updateImportStatus(ctx, siteID, importID, ImportStatusQueued, "", nil)
}

func (s *Store) MarkImportRunning(ctx context.Context, siteID, importID uuid.UUID) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		UPDATE site_imports
		SET status = ?, error = NULL, updated_at = ?, started_at = COALESCE(started_at, ?)
		WHERE site_id = ? AND id = ?
	`, ImportStatusRunning, now, now, siteID, importID)
	if err != nil {
		return fmt.Errorf("mark import running: %w", err)
	}
	return nil
}

func (s *Store) MarkImportCompleted(ctx context.Context, siteID, importID uuid.UUID, rowsImported int64, manifest *api.ImportManifest) error {
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshal completed import manifest: %w", err)
	}
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
		UPDATE site_imports
		SET status = ?, manifest = CAST(? AS JSON), rows_scanned = ?, rows_imported = ?,
			error = NULL, updated_at = ?, finished_at = ?
		WHERE site_id = ? AND id = ?
	`, ImportStatusCompleted, string(manifestJSON), manifest.RowsScanned, rowsImported, now, now, siteID, importID)
	if err != nil {
		return fmt.Errorf("mark import completed: %w", err)
	}
	return nil
}

func (s *Store) MarkImportFailed(ctx context.Context, siteID, importID uuid.UUID, errMsg string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		UPDATE site_imports
		SET status = ?, error = ?, updated_at = ?, finished_at = ?
		WHERE site_id = ? AND id = ?
	`, ImportStatusFailed, errMsg, now, now, siteID, importID)
	if err != nil {
		return fmt.Errorf("mark import failed: %w", err)
	}
	return nil
}

func (s *Store) MarkImportDeleted(ctx context.Context, siteID, importID uuid.UUID) error {
	return s.updateImportStatus(ctx, siteID, importID, ImportStatusDeleted, "", nil)
}

func (s *Store) updateImportStatus(ctx context.Context, siteID, importID uuid.UUID, status string, errMsg string, manifest *api.ImportManifest) error {
	now := time.Now().UTC()
	if manifest != nil {
		manifestJSON, err := json.Marshal(manifest)
		if err != nil {
			return fmt.Errorf("marshal import manifest: %w", err)
		}
		_, err = s.db.ExecContext(ctx, `
			UPDATE site_imports
			SET status = ?, error = NULLIF(?, ''), manifest = CAST(? AS JSON), updated_at = ?
			WHERE site_id = ? AND id = ?
		`, status, errMsg, string(manifestJSON), now, siteID, importID)
		if err != nil {
			return fmt.Errorf("update import status: %w", err)
		}
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE site_imports
		SET status = ?, error = NULLIF(?, ''), updated_at = ?
		WHERE site_id = ? AND id = ?
	`, status, errMsg, now, siteID, importID)
	if err != nil {
		return fmt.Errorf("update import status: %w", err)
	}
	return nil
}
