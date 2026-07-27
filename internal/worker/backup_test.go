package worker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/controlstore"
	"hitkeep/internal/database"
)

func TestBackupExportsSQLiteControlSnapshot(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	backupDir := filepath.Join(t.TempDir(), "backups")

	// Seed some data.
	seedSite(t, ctx, store, 365)

	mgr := newTestTenantMgr(t, store)
	w := NewBackupWorker(mgr, t.TempDir(), backupDir, 60, 24, nil, nil)
	if err := w.Run(ctx); err != nil {
		t.Fatalf("backup run: %v", err)
	}

	// Check that shared backup was created.
	sharedDir := filepath.Join(backupDir, "shared")
	entries, err := os.ReadDir(sharedDir)
	if err != nil {
		t.Fatalf("read shared backup dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one snapshot directory under shared/")
	}

	// The compatibility shared/ location now contains a compressed SQLite
	// control snapshot, manifest, and completion marker.
	snapshotDir := filepath.Join(sharedDir, entries[0].Name())
	for _, name := range []string{"control.db.zst", "manifest.json", "_COMPLETE"} {
		if info, err := os.Stat(filepath.Join(snapshotDir, name)); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("expected control snapshot file %s: info=%v err=%v", name, info, err)
		}
	}
}

func TestBackupUpdatesStatusTracker(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	backupDir := filepath.Join(t.TempDir(), "backups")
	status := &database.BackupStatusTracker{}
	status.SetConfig(true, backupDir, 15, 24)

	seedSite(t, ctx, store, 365)

	mgr := newTestTenantMgr(t, store)
	w := NewBackupWorker(mgr, t.TempDir(), backupDir, 15, 24, nil, status)
	if err := w.Run(ctx); err != nil {
		t.Fatalf("backup run: %v", err)
	}

	got := status.Status()
	if got.LastBackup == nil {
		t.Fatal("expected last backup to be recorded")
	}
	if got.NextBackup == nil {
		t.Fatal("expected next backup to be recorded")
	}
	if got.LastError != "" {
		t.Fatalf("expected last error to be cleared on success, got %q", got.LastError)
	}
	if got.RecentFailures != 0 {
		t.Fatalf("expected no recent failures, got %d", got.RecentFailures)
	}
}

func TestBackupRecordsFailureInStatusTracker(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	blockedBackupPath := filepath.Join(t.TempDir(), "backups")
	if err := os.WriteFile(blockedBackupPath, []byte("not a directory"), 0600); err != nil {
		t.Fatalf("create blocking backup path: %v", err)
	}
	status := &database.BackupStatusTracker{}
	status.SetConfig(true, blockedBackupPath, 15, 24)

	mgr := newTestTenantMgr(t, store)
	w := NewBackupWorker(mgr, t.TempDir(), blockedBackupPath, 15, 24, nil, status)
	if err := w.Run(ctx); err == nil {
		t.Fatal("expected backup run to fail")
	}

	got := status.Status()
	if got.LastBackup != nil {
		t.Fatalf("expected no last backup after failure, got %v", got.LastBackup)
	}
	if got.LastFailedAt == nil {
		t.Fatal("expected last failure time to be recorded")
	}
	if !strings.Contains(got.LastError, "backup shared database") {
		t.Fatalf("expected backup error to be recorded, got %q", got.LastError)
	}
	if got.RecentFailures != 1 {
		t.Fatalf("expected one recent failure, got %d", got.RecentFailures)
	}
	if got.NextBackup == nil {
		t.Fatal("expected next backup to be recorded after failure")
	}
}

func TestBackupFailsWhenActiveTenantEnumerationFails(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	backupDir := filepath.Join(t.TempDir(), "backups")
	oldSnapshot := filepath.Join(backupDir, "shared", "2026-01-01T000000Z")
	if err := os.MkdirAll(oldSnapshot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldSnapshot, "_COMPLETE"), []byte("ok\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	status := &database.BackupStatusTracker{}
	status.SetConfig(true, backupDir, 15, 1)

	mgr := newTestTenantMgr(t, store)
	w := NewBackupWorker(mgr, t.TempDir(), backupDir, 15, 1, nil, status)
	w.listTenantIDs = func(context.Context) ([]uuid.UUID, error) {
		return nil, errors.New("catalog unavailable")
	}

	err := w.Run(ctx)
	if err == nil || !strings.Contains(err.Error(), "list active tenants") {
		t.Fatalf("expected tenant enumeration failure, got %v", err)
	}
	got := status.Status()
	if got.LastBackup != nil {
		t.Fatalf("incomplete cycle must not be marked successful: %v", got.LastBackup)
	}
	if got.LastFailedAt == nil || got.RecentFailures != 1 {
		t.Fatalf("expected failed backup status, got %+v", got)
	}
	sharedEntries, readErr := os.ReadDir(filepath.Join(backupDir, "shared"))
	if readErr != nil {
		t.Fatalf("read shared snapshots after failure: %v", readErr)
	}
	if len(sharedEntries) != 1 || sharedEntries[0].Name() != filepath.Base(oldSnapshot) {
		t.Fatalf("failed cycle pruned or published snapshots: %v", sharedEntries)
	}
	if _, markerErr := os.Stat(filepath.Join(oldSnapshot, "_COMPLETE")); markerErr != nil {
		t.Fatalf("older completed backup was changed: %v", markerErr)
	}
}

func TestBackupDisabledWhenPathEmpty(t *testing.T) {
	store := newTestStore(t)
	mgr := newTestTenantMgr(t, store)

	w := NewBackupWorker(mgr, t.TempDir(), "", 60, 24, nil, nil)

	// Start should return immediately (no-op).
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		w.Start(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Good — Start returned immediately.
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return immediately when backupPath is empty")
	}
}

func TestBackupPrunesOldLocalSnapshots(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	backupDir := filepath.Join(t.TempDir(), "backups")

	mgr := newTestTenantMgr(t, store)

	// Seed a site so the DB has tables to export.
	seedSite(t, ctx, store, 365)

	retentionCount := 2
	w := NewBackupWorker(mgr, t.TempDir(), backupDir, 60, retentionCount, nil, nil)

	// Run 4 backups.
	for i := range 4 {
		if err := w.Run(ctx); err != nil {
			t.Fatalf("backup run %d: %v", i, err)
		}
		// Small delay so timestamps differ.
		time.Sleep(1100 * time.Millisecond)
	}

	// Check that only retentionCount snapshots remain under shared/.
	sharedDir := filepath.Join(backupDir, "shared")
	entries, err := os.ReadDir(sharedDir)
	if err != nil {
		t.Fatalf("read shared dir: %v", err)
	}

	dirCount := 0
	for _, e := range entries {
		if e.IsDir() {
			dirCount++
		}
	}
	if dirCount != retentionCount {
		t.Fatalf("expected %d snapshots after pruning, got %d", retentionCount, dirCount)
	}
}

func TestPruneLocalSnapshotsDiscardsIncompleteBeforeRetentionCounting(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"2026-07-25T000000Z", "2026-07-26T000000Z"} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "_COMPLETE"), []byte("ok\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	incomplete := filepath.Join(root, "2026-07-27T000000Z")
	if err := os.MkdirAll(incomplete, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(incomplete, "control.db.zst"), []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}

	(&BackupWorker{retentionCount: 2}).pruneLocalSnapshots(root)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Name() != "2026-07-25T000000Z" || entries[1].Name() != "2026-07-26T000000Z" {
		t.Fatalf("completed snapshots after prune=%v", entries)
	}
}

func TestBackupAndRestoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	backupDir := filepath.Join(t.TempDir(), "backups")
	siteID := seedSite(t, ctx, store, 365)

	// Insert a hit.
	isUnique := true
	if err := store.CreateHit(ctx, &api.Hit{
		SiteID:    siteID,
		SessionID: uuid.New(),
		PageID:    uuid.New(),
		Timestamp: time.Now().UTC(),
		Path:      "/test-roundtrip",
		IsUnique:  &isUnique,
	}); err != nil {
		t.Fatalf("create hit: %v", err)
	}

	// Backup.
	mgr := newTestTenantMgr(t, store)
	w := NewBackupWorker(mgr, t.TempDir(), backupDir, 60, 24, nil, nil)
	if err := w.Run(ctx); err != nil {
		t.Fatalf("backup run: %v", err)
	}

	defaultTenantID, err := mgr.DefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("resolve default tenant: %v", err)
	}
	snapshotPath := latestTenantSnapshot(t, backupDir, defaultTenantID)

	// Restore into a fresh DB.
	restoredDBPath := filepath.Join(t.TempDir(), "restored.db")
	restoredStore := database.NewStore(restoredDBPath)
	if err := restoredStore.Connect(); err != nil {
		t.Fatalf("connect restored db: %v", err)
	}
	defer restoredStore.Close()

	safePath := filepath.ToSlash(snapshotPath)
	importQuery := "IMPORT DATABASE '" + safePath + "';"
	if _, err := restoredStore.DB().ExecContext(ctx, importQuery); err != nil {
		t.Fatalf("import database: %v", err)
	}

	// Verify data survived the round-trip.
	var count int
	if err := restoredStore.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM hits WHERE site_id = ?", siteID,
	).Scan(&count); err != nil {
		t.Fatalf("count restored hits: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 hit in restored DB, got %d", count)
	}

	var path string
	if err := restoredStore.DB().QueryRowContext(ctx,
		"SELECT path FROM hits WHERE site_id = ? LIMIT 1", siteID,
	).Scan(&path); err != nil {
		t.Fatalf("query restored hit path: %v", err)
	}
	if path != "/test-roundtrip" {
		t.Fatalf("expected path=/test-roundtrip, got %q", path)
	}
}

func TestBackupHandlesMultipleTenants(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	backupDir := filepath.Join(t.TempDir(), "backups")
	dataPath := testDataPath(t, store)

	// Seed shared data.
	seedSite(t, ctx, store, 365)

	// Create a non-default tenant.
	control := testControlStore(t, store)
	creatorID, err := control.CreateUser(ctx, "multi-tenant-backup@example.test", "hash")
	if err != nil {
		t.Fatalf("create tenant owner: %v", err)
	}
	tenant, err := control.CreateTenant(ctx, creatorID, "Test Tenant", "")
	if err != nil {
		t.Fatalf("create custom tenant: %v", err)
	}
	customTenantID := tenant.ID
	mgr := newTestTenantMgr(t, store)

	// Open tenant store to create the DB file.
	tenantStore, err := mgr.ForTenant(ctx, customTenantID)
	if err != nil {
		t.Fatalf("open tenant store: %v", err)
	}

	// Seed tenant data.
	tenantSite, err := control.CreateSite(ctx, creatorID, "tenant-backup.example.test")
	if err != nil {
		t.Fatalf("create tenant site: %v", err)
	}
	if err := control.UpdateSiteTenant(ctx, tenantSite.ID, customTenantID); err != nil {
		t.Fatalf("assign tenant site: %v", err)
	}
	if err := mgr.SyncSite(ctx, tenantSite.ID); err != nil {
		t.Fatalf("sync tenant site: %v", err)
	}
	if _, err := tenantStore.DB().ExecContext(ctx,
		"INSERT INTO hits (id, site_id, session_id, page_id, timestamp, path, is_unique) VALUES (?, ?, ?, ?, ?, ?, ?)",
		uuid.New(), tenantSite.ID, uuid.New(), uuid.New(), time.Now().UTC(), "/tenant-page", true,
	); err != nil {
		t.Fatalf("seed tenant hit: %v", err)
	}

	w := NewBackupWorker(mgr, dataPath, backupDir, 60, 24, nil, nil)
	if err := w.Run(ctx); err != nil {
		t.Fatalf("backup run: %v", err)
	}

	// Verify shared backup exists.
	sharedEntries, err := os.ReadDir(filepath.Join(backupDir, "shared"))
	if err != nil {
		t.Fatalf("read shared dir: %v", err)
	}
	if len(sharedEntries) == 0 {
		t.Fatal("expected shared backup snapshot")
	}

	// Verify tenant backup exists.
	tenantDir := filepath.Join(backupDir, "tenants", customTenantID.String())
	tenantEntries, err := os.ReadDir(tenantDir)
	if err != nil {
		t.Fatalf("read tenant backup dir: %v", err)
	}
	if len(tenantEntries) == 0 {
		t.Fatal("expected tenant backup snapshot")
	}

	// Check parquet files in tenant snapshot.
	files, err := findParquetFiles(filepath.Join(tenantDir, tenantEntries[0].Name()))
	if err != nil {
		t.Fatalf("find tenant parquet files: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("expected parquet files in tenant backup snapshot")
	}
}

func TestBackupExportsSplitDefaultAndAttachedTenantCatalogs(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sharedPath := filepath.Join(root, "hitkeep.db")
	dataPath := filepath.Join(root, "data")
	backupDir := filepath.Join(root, "backups")

	shared := database.NewStore(sharedPath)
	if err := shared.Connect(); err != nil {
		t.Fatalf("connect shared database: %v", err)
	}
	if err := shared.Migrate(ctx); err != nil {
		t.Fatalf("migrate shared database: %v", err)
	}
	defaultSiteID := seedLegacySite(t, ctx, shared, 365)
	isUnique := true
	if err := shared.CreateHit(ctx, &api.Hit{
		SiteID: defaultSiteID, SessionID: uuid.New(), PageID: uuid.New(),
		Timestamp: time.Now().UTC(), Path: "/default-backup", IsUnique: &isUnique,
	}); err != nil {
		t.Fatalf("seed default hit: %v", err)
	}
	otherTenantID := uuid.New()
	if _, err := shared.DB().ExecContext(ctx,
		"INSERT INTO tenants (id, name, is_default, created_at) VALUES (?, ?, FALSE, ?)",
		otherTenantID, "Attached Backup Tenant", time.Now().UTC(),
	); err != nil {
		t.Fatalf("insert attached tenant: %v", err)
	}
	if err := shared.Close(); err != nil {
		t.Fatalf("close shared database before split: %v", err)
	}
	if err := database.RunDefaultTenantSplit(ctx, sharedPath, dataPath); err != nil {
		t.Fatalf("split default tenant: %v", err)
	}

	if err := convertLegacyControlForWorkerTest(ctx, sharedPath); err != nil {
		t.Fatalf("convert legacy control database: %v", err)
	}
	control, err := controlstore.Open(ctx, sharedPath)
	if err != nil {
		t.Fatalf("open SQLite control database: %v", err)
	}
	t.Cleanup(func() { _ = control.Close() })
	mgr := database.NewTenantStoreManager(control, dataPath, nil)
	t.Cleanup(func() { _ = mgr.Close() })
	defaultID, err := mgr.DefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("resolve default tenant: %v", err)
	}
	otherStore, err := mgr.ForTenant(ctx, otherTenantID)
	if err != nil {
		t.Fatalf("open attached tenant store: %v", err)
	}
	otherSiteID := uuid.New()
	if _, err := otherStore.DB().ExecContext(ctx, `
		INSERT INTO sites (id, domain, data_retention_days) VALUES (?, ?, ?)
	`, otherSiteID, "attached-backup.test", 365); err != nil {
		t.Fatalf("seed attached tenant site: %v", err)
	}
	if err := otherStore.CreateHit(ctx, &api.Hit{
		SiteID: otherSiteID, SessionID: uuid.New(), PageID: uuid.New(),
		Timestamp: time.Now().UTC(), Path: "/attached-backup", IsUnique: &isUnique,
	}); err != nil {
		t.Fatalf("seed attached tenant hit: %v", err)
	}

	w := NewBackupWorker(mgr, dataPath, backupDir, 60, 24, nil, nil)
	if err := w.Run(ctx); err != nil {
		t.Fatalf("backup split data plane: %v", err)
	}

	assertBackupContainsOnlyHitPath(t, ctx, latestTenantSnapshot(t, backupDir, defaultID), "/default-backup")
	assertBackupContainsOnlyHitPath(t, ctx, latestTenantSnapshot(t, backupDir, otherTenantID), "/attached-backup")
}

func latestTenantSnapshot(t *testing.T, backupDir string, tenantID uuid.UUID) string {
	t.Helper()
	dir := filepath.Join(backupDir, "tenants", tenantID.String())
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read tenant backup directory %s: %v", tenantID, err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one backup for tenant %s, got %d", tenantID, len(entries))
	}
	return filepath.Join(dir, entries[0].Name())
}

func assertBackupContainsOnlyHitPath(t *testing.T, ctx context.Context, snapshotPath, wantPath string) {
	t.Helper()
	restored := database.NewStore(filepath.Join(t.TempDir(), "restored.db"))
	if err := restored.Connect(); err != nil {
		t.Fatalf("connect restored tenant database: %v", err)
	}
	defer restored.Close()
	query := "IMPORT DATABASE '" + strings.ReplaceAll(filepath.ToSlash(snapshotPath), "'", "''") + "';"
	if _, err := restored.DB().ExecContext(ctx, query); err != nil {
		t.Fatalf("import tenant backup: %v", err)
	}
	var paths []string
	rows, err := restored.DB().QueryContext(ctx, "SELECT path FROM hits ORDER BY path")
	if err != nil {
		t.Fatalf("query restored tenant hits: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			t.Fatalf("scan restored tenant hit: %v", err)
		}
		paths = append(paths, path)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read restored tenant hits: %v", err)
	}
	if len(paths) != 1 || paths[0] != wantPath {
		t.Fatalf("expected isolated backup path %q, got %v", wantPath, paths)
	}
}

func TestBackupFailsWhenTenantSnapshotFails(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	backupDir := filepath.Join(t.TempDir(), "backups")
	dataPath := testDataPath(t, store)

	control := testControlStore(t, store)
	creatorID, err := control.CreateUser(ctx, "broken-tenant-backup@example.test", "hash")
	if err != nil {
		t.Fatalf("create tenant owner: %v", err)
	}
	tenant, err := control.CreateTenant(ctx, creatorID, "Broken Backup Tenant", "")
	if err != nil {
		t.Fatalf("create custom tenant: %v", err)
	}
	customTenantID := tenant.ID
	mgr := newTestTenantMgr(t, store)
	tenantStore, err := mgr.ForTenant(ctx, customTenantID)
	if err != nil {
		t.Fatalf("open tenant store: %v", err)
	}
	if err := tenantStore.Close(); err != nil {
		t.Fatalf("close tenant store: %v", err)
	}

	w := NewBackupWorker(mgr, dataPath, backupDir, 60, 24, nil, nil)
	err = w.Run(ctx)
	if err == nil || !strings.Contains(err.Error(), customTenantID.String()) {
		t.Fatalf("expected tenant-specific backup failure, got %v", err)
	}

	tenantBackupDir := filepath.Join(backupDir, "tenants", customTenantID.String())
	if entries, readErr := os.ReadDir(tenantBackupDir); readErr == nil && len(entries) != 0 {
		t.Fatalf("expected incomplete tenant snapshot cleanup, found %d entries", len(entries))
	} else if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatalf("inspect tenant backup directory: %v", readErr)
	}
}

func TestRemoveIncompleteLocalSnapshot(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "partial")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create partial snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "partial.parquet"), []byte("partial"), 0o600); err != nil {
		t.Fatalf("write partial snapshot: %v", err)
	}

	worker := &BackupWorker{}
	worker.removeIncompleteLocalSnapshot(dir, false)
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("expected incomplete local snapshot to be removed, got %v", err)
	}
}

func seedLegacySite(t *testing.T, ctx context.Context, store *database.Store, retentionDays int) uuid.UUID {
	t.Helper()
	userID, err := store.CreateUser(ctx, "legacy-backup-"+uuid.NewString()+"@example.test", "hash")
	if err != nil {
		t.Fatalf("create legacy user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "legacy-backup-"+uuid.NewString()+".example.test")
	if err != nil {
		t.Fatalf("create legacy site: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx, "UPDATE sites SET data_retention_days = ? WHERE id = ?", retentionDays, site.ID); err != nil {
		t.Fatalf("set legacy retention: %v", err)
	}
	return site.ID
}

func convertLegacyControlForWorkerTest(ctx context.Context, path string) error {
	source, sourceSHA, schemaSHA, err := database.OpenLegacyControlSource(ctx, path)
	if err != nil {
		return err
	}
	_, importErr := controlstore.ImportLegacy(ctx, controlstore.SQLiteWorkPath(path), source, sourceSHA, schemaSHA)
	closeErr := source.Close()
	if importErr != nil {
		return importErr
	}
	if closeErr != nil {
		return closeErr
	}
	return controlstore.PublishLegacyConversion(ctx, path)
}
