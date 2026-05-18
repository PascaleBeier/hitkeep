package hitkeepcmd

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
	"hitkeep/internal/worker"
)

func TestMoveExistingDatabaseAsideRenamesDatabaseAndWal(t *testing.T) {
	t.Parallel()

	targetPath := filepath.Join(t.TempDir(), "hitkeep.db")
	if err := os.WriteFile(targetPath, []byte("db"), 0644); err != nil {
		t.Fatalf("write db: %v", err)
	}
	if err := os.WriteFile(targetPath+".wal", []byte("wal"), 0644); err != nil {
		t.Fatalf("write wal: %v", err)
	}

	backupPath, err := moveExistingDatabaseAside(targetPath)
	if err != nil {
		t.Fatalf("moveExistingDatabaseAside: %v", err)
	}
	if backupPath == "" {
		t.Fatal("expected backup path")
	}

	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("expected target db to be moved aside, stat err=%v", err)
	}
	if _, err := os.Stat(targetPath + ".wal"); !os.IsNotExist(err) {
		t.Fatalf("expected target wal to be moved aside, stat err=%v", err)
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("expected backup db to exist: %v", err)
	}
	if _, err := os.Stat(backupPath + ".wal"); err != nil {
		t.Fatalf("expected backup wal to exist: %v", err)
	}
}

func TestRestoreDatabaseDoesNotLeaveWal(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tmpDir := t.TempDir()
	sourceDBPath := filepath.Join(tmpDir, "source.db")
	sourceSnapshotPath := filepath.Join(tmpDir, "snapshot")

	sourceStore := database.NewStore(sourceDBPath)
	if err := sourceStore.Connect(); err != nil {
		t.Fatalf("connect source: %v", err)
	}

	sourceDB := sourceStore.DB()
	if _, err := sourceDB.ExecContext(ctx, "CREATE TABLE recover_test(id INTEGER, name VARCHAR);"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := sourceDB.ExecContext(ctx, "INSERT INTO recover_test VALUES (1, 'acme');"); err != nil {
		t.Fatalf("insert row: %v", err)
	}
	if _, err := sourceDB.ExecContext(ctx, "CHECKPOINT;"); err != nil {
		t.Fatalf("checkpoint source: %v", err)
	}
	if err := os.MkdirAll(sourceSnapshotPath, 0755); err != nil {
		t.Fatalf("mkdir snapshot: %v", err)
	}
	if _, err := sourceDB.ExecContext(ctx, fmt.Sprintf("EXPORT DATABASE '%s' (FORMAT PARQUET);", sourceSnapshotPath)); err != nil {
		t.Fatalf("export database: %v", err)
	}
	if err := sourceStore.Close(); err != nil {
		t.Fatalf("close source store: %v", err)
	}

	targetPath := filepath.Join(tmpDir, "restored.db")
	if err := restoreDatabase(ctx, targetPath, sourceSnapshotPath, false, nil); err != nil {
		t.Fatalf("restoreDatabase: %v", err)
	}

	if _, err := os.Stat(targetPath + ".wal"); !os.IsNotExist(err) {
		t.Fatalf("expected no wal file after restore, stat err=%v", err)
	}

	targetStore := database.NewStore(targetPath)
	if err := targetStore.Connect(); err != nil {
		t.Fatalf("connect restored: %v", err)
	}
	defer targetStore.Close()

	var (
		id   int
		name string
	)
	if err := targetStore.DB().QueryRowContext(ctx, "SELECT id, name FROM recover_test;").Scan(&id, &name); err != nil {
		t.Fatalf("query restored row: %v", err)
	}
	if id != 1 || name != "acme" {
		t.Fatalf("unexpected restored row: id=%d name=%q", id, name)
	}
}

func TestBackupRestorePreservesHitGeoNetworkMetadata(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	sourceDBPath := filepath.Join(tmpDir, "hitkeep.db")
	backupDir := filepath.Join(tmpDir, "backups")
	dataPath := filepath.Join(tmpDir, "data")
	expected := geoNetworkBackupFixture{
		region:   "California",
		city:     "Mountain View",
		provider: "Google LLC",
		asn:      15169,
		asnOrg:   "Google LLC",
	}

	sourceStore := newMigratedRecoverTestStore(t, ctx, sourceDBPath)
	siteID := seedGeoNetworkHitForBackup(t, ctx, sourceStore, expected)
	runRecoverTestBackup(t, ctx, sourceStore, dataPath, backupDir)
	targetPath := filepath.Join(tmpDir, "restored.db")
	restoreLatestSharedSnapshot(t, ctx, backupDir, targetPath)
	assertRestoredGeoNetworkHit(t, ctx, targetPath, siteID, expected)
}

type geoNetworkBackupFixture struct {
	region   string
	city     string
	provider string
	asn      int
	asnOrg   string
}

func newMigratedRecoverTestStore(t *testing.T, ctx context.Context, dbPath string) *database.Store {
	t.Helper()
	store := database.NewStore(dbPath)
	if err := store.Connect(); err != nil {
		t.Fatalf("connect source: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate source: %v", err)
	}
	return store
}

func seedGeoNetworkHitForBackup(t *testing.T, ctx context.Context, store *database.Store, fixture geoNetworkBackupFixture) uuid.UUID {
	t.Helper()
	userID, err := store.CreateUser(ctx, "backup-geo@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "backup-geo.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	if err := store.CreateHit(ctx, &api.Hit{
		SiteID:    site.ID,
		SessionID: uuid.New(),
		PageID:    uuid.New(),
		Timestamp: time.Now().UTC(),
		Path:      "/geo-backup",
		Region:    &fixture.region,
		City:      &fixture.city,
		Provider:  &fixture.provider,
		ASN:       &fixture.asn,
		ASNOrg:    &fixture.asnOrg,
	}); err != nil {
		t.Fatalf("create geo hit: %v", err)
	}
	return site.ID
}

func runRecoverTestBackup(t *testing.T, ctx context.Context, sourceStore *database.Store, dataPath string, backupDir string) {
	t.Helper()
	tenantMgr := database.NewTenantStoreManager(sourceStore, dataPath)
	t.Cleanup(func() { _ = tenantMgr.Close() })
	backupWorker := worker.NewBackupWorker(tenantMgr, dataPath, backupDir, 60, 24, nil, nil)
	if err := backupWorker.Run(ctx); err != nil {
		t.Fatalf("backup run: %v", err)
	}
}

func restoreLatestSharedSnapshot(t *testing.T, ctx context.Context, backupDir string, targetPath string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(backupDir, "shared"))
	if err != nil {
		t.Fatalf("read shared backup dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected shared backup snapshot")
	}
	snapshotPath := filepath.Join(backupDir, "shared", entries[0].Name())
	if err := restoreDatabase(ctx, targetPath, snapshotPath, false, nil); err != nil {
		t.Fatalf("restoreDatabase: %v", err)
	}
}

func assertRestoredGeoNetworkHit(t *testing.T, ctx context.Context, targetPath string, siteID uuid.UUID, expected geoNetworkBackupFixture) {
	t.Helper()
	restoredStore := database.NewStore(targetPath)
	if err := restoredStore.Connect(); err != nil {
		t.Fatalf("connect restored: %v", err)
	}
	defer restoredStore.Close()

	var (
		gotRegion   sql.NullString
		gotCity     sql.NullString
		gotProvider sql.NullString
		gotASN      sql.NullInt64
		gotASNOrg   sql.NullString
	)
	if err := restoredStore.DB().QueryRowContext(ctx, `
		SELECT region, city, provider, asn, asn_org
		FROM hits
		WHERE site_id = ? AND path = ?
	`, siteID, "/geo-backup").Scan(&gotRegion, &gotCity, &gotProvider, &gotASN, &gotASNOrg); err != nil {
		t.Fatalf("query restored geo hit: %v", err)
	}
	assertRestoredString(t, "region", expected.region, gotRegion)
	assertRestoredString(t, "city", expected.city, gotCity)
	assertRestoredString(t, "provider", expected.provider, gotProvider)
	assertRestoredInt64(t, "ASN", int64(expected.asn), gotASN)
	assertRestoredString(t, "ASN org", expected.asnOrg, gotASNOrg)
}

func assertRestoredString(t *testing.T, label string, want string, got sql.NullString) {
	t.Helper()
	if !got.Valid || got.String != want {
		t.Fatalf("expected restored %s %q, got %q valid=%v", label, want, got.String, got.Valid)
	}
}

func assertRestoredInt64(t *testing.T, label string, want int64, got sql.NullInt64) {
	t.Helper()
	if !got.Valid || got.Int64 != want {
		t.Fatalf("expected restored %s %d, got %d valid=%v", label, want, got.Int64, got.Valid)
	}
}

func TestRestoreDatabasePreservesExistingBrokenWalWithoutOpeningTarget(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tmpDir := t.TempDir()
	sourceDBPath := filepath.Join(tmpDir, "source.db")
	sourceSnapshotPath := filepath.Join(tmpDir, "snapshot")

	db, err := sql.Open("duckdb", sourceDBPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx, "CREATE TABLE recover_test(id INTEGER);"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO recover_test VALUES (7);"); err != nil {
		t.Fatalf("insert row: %v", err)
	}
	if err := os.MkdirAll(sourceSnapshotPath, 0755); err != nil {
		t.Fatalf("mkdir snapshot: %v", err)
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf("EXPORT DATABASE '%s' (FORMAT PARQUET);", sourceSnapshotPath)); err != nil {
		t.Fatalf("export database: %v", err)
	}

	targetPath := filepath.Join(tmpDir, "hitkeep.db")
	if err := os.WriteFile(targetPath, []byte("not-a-db"), 0644); err != nil {
		t.Fatalf("write target db: %v", err)
	}
	if err := os.WriteFile(targetPath+".wal", []byte("broken-wal"), 0644); err != nil {
		t.Fatalf("write target wal: %v", err)
	}

	if err := restoreDatabase(ctx, targetPath, sourceSnapshotPath, false, nil); err != nil {
		t.Fatalf("restoreDatabase with existing wal: %v", err)
	}

	if _, err := os.Stat(targetPath + ".wal"); !os.IsNotExist(err) {
		t.Fatalf("expected no wal file after restore, stat err=%v", err)
	}
	if matches, err := filepath.Glob(targetPath + ".pre-restore.*.wal"); err != nil || len(matches) != 1 {
		t.Fatalf("expected exactly one preserved wal backup, matches=%v err=%v", matches, err)
	}
}
