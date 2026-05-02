package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/database"
)

func createImportStageCleanupTestUpload(t *testing.T, store *database.Store, dataPath string, siteID, userID uuid.UUID, filename string, content string) (uuid.UUID, uuid.UUID, string) {
	t.Helper()
	fileID := uuid.New()
	relativePath := filepath.Join("imports", siteID.String(), fileID.String()+"-"+filename)
	job, err := store.CreateSiteImportUpload(context.Background(), siteID, userID, "plausible", []database.ImportFileCreate{
		{
			ID:           fileID,
			Filename:     filename,
			RelativePath: relativePath,
			SizeBytes:    int64(len(content)),
		},
	})
	if err != nil {
		t.Fatalf("create import upload: %v", err)
	}
	path := filepath.Join(dataPath, relativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("create stage dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write staged file: %v", err)
	}
	if err := store.UpdateImportFileProgress(context.Background(), job.ID, fileID, int64(len(content)), ""); err != nil {
		t.Fatalf("mark staged file uploaded: %v", err)
	}
	return job.ID, fileID, path
}

func forceImportStageCleanupStatus(t *testing.T, store *database.Store, importID uuid.UUID, status string, at time.Time) {
	t.Helper()
	if _, err := store.DB().ExecContext(context.Background(), `
		UPDATE site_imports
		SET status = ?, created_at = ?, updated_at = ?, validated_at = ?, finished_at = ?
		WHERE id = ?
	`, status, at, at, at, at, importID); err != nil {
		t.Fatalf("force import status: %v", err)
	}
}

func TestImportStageCleanerRemovesStaleFilesAndPrunesEmptySiteDir(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	dataPath := t.TempDir()
	userID, err := store.CreateUser(ctx, "cleanup-worker@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "cleanup-worker.example")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	importID, fileID, path := createImportStageCleanupTestUpload(t, store, dataPath, site.ID, userID, "stale.csv", "12345")
	forceImportStageCleanupStatus(t, store, importID, database.ImportStatusCompleted, time.Now().UTC().AddDate(0, 0, -8))

	result, err := NewImportStageCleaner(store, dataPath, 7).Run(ctx)
	if err != nil {
		t.Fatalf("cleanup run: %v", err)
	}
	if result.ImportsCleaned != 1 || result.FilesCleaned != 1 || result.BytesCleaned != 5 || result.ImportsMarkedFailed != 0 {
		t.Fatalf("unexpected cleanup result: %+v", result)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected staged file to be removed, stat err: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataPath, "imports", site.ID.String())); !os.IsNotExist(err) {
		t.Fatalf("expected empty site staging dir to be pruned, stat err: %v", err)
	}

	files, err := store.ListStaleImportStageFiles(ctx, time.Now().UTC().AddDate(0, 0, -7))
	if err != nil {
		t.Fatalf("list stale files: %v", err)
	}
	for _, file := range files {
		if file.ImportID == importID && file.FileID == fileID {
			t.Fatalf("cleaned file should not remain stale: %+v", file)
		}
	}
}

func TestImportStageCleanerMarksResumableImportsFailed(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	dataPath := t.TempDir()
	userID, err := store.CreateUser(ctx, "cleanup-resumable@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "cleanup-resumable.example")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	importID, _, _ := createImportStageCleanupTestUpload(t, store, dataPath, site.ID, userID, "validated.csv", "abcdef")
	forceImportStageCleanupStatus(t, store, importID, database.ImportStatusValidated, time.Now().UTC().AddDate(0, 0, -8))

	result, err := NewImportStageCleaner(store, dataPath, 7).Run(ctx)
	if err != nil {
		t.Fatalf("cleanup run: %v", err)
	}
	if result.ImportsMarkedFailed != 1 {
		t.Fatalf("expected one import marked failed, got %+v", result)
	}
	job, err := store.GetSiteImport(ctx, site.ID, importID)
	if err != nil {
		t.Fatalf("get import: %v", err)
	}
	if job.Status != database.ImportStatusFailed || job.Error != database.ImportStageExpiredMessage {
		t.Fatalf("unexpected import state: %+v", job)
	}
}

func TestImportStageCleanerHandlesMissingFilesAndKeepsRunIdempotent(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	dataPath := t.TempDir()
	userID, err := store.CreateUser(ctx, "cleanup-missing@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "cleanup-missing.example")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	importID, _, path := createImportStageCleanupTestUpload(t, store, dataPath, site.ID, userID, "missing.csv", "abc")
	forceImportStageCleanupStatus(t, store, importID, database.ImportStatusCompleted, time.Now().UTC().AddDate(0, 0, -8))
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove staged file before cleanup: %v", err)
	}

	result, err := NewImportStageCleaner(store, dataPath, 7).Run(ctx)
	if err != nil {
		t.Fatalf("cleanup run: %v", err)
	}
	if result.FilesCleaned != 1 || result.BytesCleaned != 3 {
		t.Fatalf("missing file should still be marked cleaned, got %+v", result)
	}

	result, err = NewImportStageCleaner(store, dataPath, 7).Run(ctx)
	if err != nil {
		t.Fatalf("second cleanup run: %v", err)
	}
	if result.FilesCleaned != 0 || result.ImportsCleaned != 0 {
		t.Fatalf("expected second run to be idempotent, got %+v", result)
	}
}

func TestImportStageCleanerRejectsUnsafeRelativePaths(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	dataPath := t.TempDir()
	userID, err := store.CreateUser(ctx, "cleanup-unsafe@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "cleanup-unsafe.example")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	importID, _, _ := createImportStageCleanupTestUpload(t, store, dataPath, site.ID, userID, "unsafe.csv", "abc")
	forceImportStageCleanupStatus(t, store, importID, database.ImportStatusCompleted, time.Now().UTC().AddDate(0, 0, -8))
	if _, err := store.DB().ExecContext(ctx, "UPDATE site_import_files SET relative_path = ? WHERE import_id = ?", filepath.Join("imports", site.ID.String(), "..", "..", "escape.csv"), importID); err != nil {
		t.Fatalf("set unsafe relative path: %v", err)
	}

	result, err := NewImportStageCleaner(store, dataPath, 7).Run(ctx)
	if err == nil {
		t.Fatal("expected unsafe path cleanup to fail")
	}
	if result.FilesCleaned != 0 || len(result.Errors) == 0 {
		t.Fatalf("expected no cleaned files and recorded error, got %+v", result)
	}
	if !strings.Contains(result.Errors[0], "staged import path") {
		t.Fatalf("expected unsafe path error, got %+v", result.Errors)
	}
}
