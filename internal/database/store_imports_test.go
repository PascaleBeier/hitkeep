package database

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

func setupImportStoreTest(t *testing.T) (*Store, uuid.UUID, uuid.UUID) {
	t.Helper()
	store := NewStore(filepath.Join(t.TempDir(), "hitkeep.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate store: %v", err)
	}

	userID, err := store.CreateUser(context.Background(), "import-store@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(context.Background(), userID, "imports-store.example")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	return store, site.ID, userID
}

func createImportStageTestUpload(t *testing.T, store *Store, siteID, userID uuid.UUID, filename string, size int64) (uuid.UUID, uuid.UUID) {
	t.Helper()
	fileID := uuid.New()
	job, err := store.CreateSiteImportUpload(context.Background(), siteID, userID, "plausible", []ImportFileCreate{
		{
			ID:           fileID,
			Filename:     filename,
			RelativePath: filepath.Join("imports", siteID.String(), fileID.String()+"-"+filename),
			SizeBytes:    size,
		},
	})
	if err != nil {
		t.Fatalf("create import upload: %v", err)
	}
	return job.ID, fileID
}

func forceImportStageStatus(t *testing.T, store *Store, importID uuid.UUID, status string, at time.Time) {
	t.Helper()
	if _, err := store.DB().ExecContext(context.Background(), `
		UPDATE site_imports
		SET status = ?, created_at = ?, updated_at = ?, validated_at = ?, finished_at = ?
		WHERE id = ?
	`, status, at, at, at, at, importID); err != nil {
		t.Fatalf("force import status: %v", err)
	}
}

func TestListStaleImportStageFilesFiltersStatusesAndCleanedFiles(t *testing.T) {
	store, siteID, userID := setupImportStoreTest(t)
	ctx := context.Background()
	old := time.Now().UTC().AddDate(0, 0, -8)
	cutoff := time.Now().UTC().AddDate(0, 0, -7)

	staleUploadID, staleFileID := createImportStageTestUpload(t, store, siteID, userID, "stale.csv", 100)
	forceImportStageStatus(t, store, staleUploadID, ImportStatusUploading, old)

	activeQueuedID, _ := createImportStageTestUpload(t, store, siteID, userID, "queued.csv", 200)
	forceImportStageStatus(t, store, activeQueuedID, ImportStatusQueued, old)

	cleanedCompletedID, cleanedFileID := createImportStageTestUpload(t, store, siteID, userID, "cleaned.csv", 300)
	forceImportStageStatus(t, store, cleanedCompletedID, ImportStatusCompleted, old)
	if err := store.MarkImportFileCleaned(ctx, cleanedCompletedID, cleanedFileID, time.Now().UTC()); err != nil {
		t.Fatalf("mark file cleaned: %v", err)
	}

	files, err := store.ListStaleImportStageFiles(ctx, cutoff)
	if err != nil {
		t.Fatalf("list stale files: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one stale file, got %+v", files)
	}
	if files[0].ImportID != staleUploadID || files[0].FileID != staleFileID {
		t.Fatalf("unexpected stale file: %+v", files[0])
	}

	estimate, err := store.EstimateStaleImportStageFiles(ctx, cutoff)
	if err != nil {
		t.Fatalf("estimate stale files: %v", err)
	}
	if estimate.Imports != 1 || estimate.Files != 1 || estimate.Bytes != 100 {
		t.Fatalf("unexpected estimate: %+v", estimate)
	}
}

func TestMarkStaleImportExpiredOnlyTouchesResumableStatuses(t *testing.T) {
	store, siteID, userID := setupImportStoreTest(t)
	ctx := context.Background()
	now := time.Now().UTC()

	validatedID, _ := createImportStageTestUpload(t, store, siteID, userID, "validated.csv", 100)
	forceImportStageStatus(t, store, validatedID, ImportStatusValidated, now.AddDate(0, 0, -8))

	changed, err := store.MarkStaleImportExpired(ctx, validatedID, ImportStageExpiredMessage, now)
	if err != nil {
		t.Fatalf("mark stale import expired: %v", err)
	}
	if !changed {
		t.Fatal("expected validated import to be marked expired")
	}
	job, err := store.GetSiteImport(ctx, siteID, validatedID)
	if err != nil {
		t.Fatalf("get expired import: %v", err)
	}
	if job.Status != ImportStatusFailed || job.Error != ImportStageExpiredMessage || job.FinishedAt == nil {
		t.Fatalf("unexpected expired import state: %+v", job)
	}

	queuedID, _ := createImportStageTestUpload(t, store, siteID, userID, "queued.csv", 100)
	forceImportStageStatus(t, store, queuedID, ImportStatusQueued, now.AddDate(0, 0, -8))
	changed, err = store.MarkStaleImportExpired(ctx, queuedID, ImportStageExpiredMessage, now)
	if err != nil {
		t.Fatalf("mark queued import expired: %v", err)
	}
	if changed {
		t.Fatal("did not expect queued import to be marked expired")
	}
}
