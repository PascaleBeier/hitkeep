package database

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func indexMutationTestError(tables ...string) error {
	table := ""
	if len(tables) > 0 {
		table = tables[0]
	}
	return &databaseOperationError{
		err:   errors.New("FATAL Error: Invalid Input Error: Failed to delete all rows from index. Only deleted 0 out of 1 rows"),
		table: table,
	}
}

func TestStoreRepairsOnlyFailedTableNonUniqueIndexes(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "unsafe.db")

	source := NewStore(dbPath, WithCheckpointInterval(0))
	if err := source.Connect(); err != nil {
		t.Fatalf("connect source: %v", err)
	}
	if _, err := source.DB().ExecContext(ctx, `
			CREATE TABLE site_activity_summary (
				site_id UUID PRIMARY KEY,
				tenant_id UUID NOT NULL,
				last_hit_at TIMESTAMPTZ,
				last_event_at TIMESTAMPTZ
			);
			CREATE INDEX site_activity_summary_tenant_idx
				ON site_activity_summary(tenant_id);
			CREATE INDEX site_activity_summary_last_hit_idx
				ON site_activity_summary(last_hit_at);
			CREATE INDEX site_activity_summary_last_event_idx
				ON site_activity_summary(last_event_at);
			CREATE UNIQUE INDEX site_activity_summary_unique_probe_idx
				ON site_activity_summary(tenant_id, site_id);
			CREATE TABLE recovery_probe (id BIGINT, value VARCHAR);
			CREATE INDEX idx_recovery_probe_value ON recovery_probe(value);
			INSERT INTO site_activity_summary
				VALUES (uuid(), uuid(), now(), NULL);
			INSERT INTO recovery_probe VALUES (1, 'kept');
		`); err != nil {
		t.Fatalf("seed unsafe database: %v", err)
	}
	if err := source.Close(); err != nil {
		t.Fatalf("close source: %v", err)
	}

	recoveryRoot := filepath.Join(dir, "recovery")
	store := NewStore(dbPath,
		WithCheckpointInterval(0),
		WithAutomaticRecovery(true, recoveryRoot),
	)
	if err := store.recovery.recoverInvalidation(ctx, indexMutationTestError("SITE_ACTIVITY_SUMMARY")); err != nil {
		t.Fatalf("repair unsafe indexes: %v", err)
	}
	if err := store.Connect(); err != nil {
		t.Fatalf("connect recovered database: %v", err)
	}
	var unsafeCount, retainedCount, uniqueCount int
	if err := store.DB().QueryRowContext(ctx,
		"SELECT count(*) FROM duckdb_indexes() WHERE table_name = 'site_activity_summary' AND NOT is_unique").Scan(&unsafeCount); err != nil {
		t.Fatalf("count unsafe indexes: %v", err)
	}
	if err := store.DB().QueryRowContext(ctx,
		"SELECT count(*) FROM duckdb_indexes() WHERE index_name = 'idx_recovery_probe_value'").Scan(&retainedCount); err != nil {
		t.Fatalf("count retained indexes: %v", err)
	}
	if err := store.DB().QueryRowContext(ctx,
		"SELECT count(*) FROM duckdb_indexes() WHERE index_name = 'site_activity_summary_unique_probe_idx'").Scan(&uniqueCount); err != nil {
		t.Fatalf("count retained unique indexes: %v", err)
	}
	if unsafeCount != 0 || retainedCount != 1 || uniqueCount != 1 {
		t.Fatalf("unexpected recovered index inventory: unsafe=%d retained=%d unique=%d", unsafeCount, retainedCount, uniqueCount)
	}

	status := store.DatabaseStatus()
	if status.State != DatabaseStateHealthy || !status.RecoveryBundleAvailable || status.RemovedUnsafeIndexes != 3 {
		t.Fatalf("unexpected recovery status: %+v", status)
	}
	if _, err := os.Stat(store.recovery.markerPath()); !os.IsNotExist(err) {
		t.Fatalf("expected recovery marker to be cleared, got %v", err)
	}
	entries, err := os.ReadDir(recoveryRoot)
	if err != nil {
		t.Fatalf("read recovery root: %v", err)
	}
	var bundleDir string
	for _, entry := range entries {
		if entry.IsDir() {
			bundleDir = filepath.Join(recoveryRoot, entry.Name())
			break
		}
	}
	if bundleDir == "" {
		t.Fatal("expected retained recovery bundle")
	}
	manifest, err := os.ReadFile(filepath.Join(bundleDir, "manifest.json"))
	if err != nil {
		t.Fatalf("read recovery manifest: %v", err)
	}
	if strings.Contains(string(manifest), dbPath) {
		t.Fatal("recovery manifest must not expose the database path")
	}

	if err := store.Close(); err != nil {
		t.Fatalf("close recovered database: %v", err)
	}
	reopened := NewStore(dbPath,
		WithCheckpointInterval(0),
		WithAutomaticRecovery(true, recoveryRoot),
	)
	if err := reopened.Connect(); err != nil {
		t.Fatalf("reopen recovered database: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	reopenedStatus := reopened.DatabaseStatus()
	if !reopenedStatus.RecoveryBundleAvailable || reopenedStatus.LastRecoveryAt == nil || reopenedStatus.RemovedUnsafeIndexes != 3 {
		t.Fatalf("expected retained recovery history after restart, got %+v", reopenedStatus)
	}
}

func TestStoreFallsBackToAllNonUniqueIndexesWhenMutationTableIsUnknown(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "fallback.db")

	source := NewStore(dbPath, WithCheckpointInterval(0))
	if err := source.Connect(); err != nil {
		t.Fatalf("connect source: %v", err)
	}
	if _, err := source.DB().ExecContext(ctx, `
		CREATE TABLE first_probe (id BIGINT PRIMARY KEY, value VARCHAR);
		CREATE INDEX first_probe_value_idx ON first_probe(value);
		CREATE TABLE second_probe (id BIGINT PRIMARY KEY, value VARCHAR);
		CREATE INDEX second_probe_value_idx ON second_probe(value);
		CREATE UNIQUE INDEX second_probe_unique_idx ON second_probe(value);
	`); err != nil {
		t.Fatalf("seed fallback database: %v", err)
	}
	if err := source.Close(); err != nil {
		t.Fatalf("close source: %v", err)
	}

	store := NewStore(dbPath,
		WithCheckpointInterval(0),
		WithAutomaticRecovery(true, filepath.Join(dir, "recovery")),
	)
	if err := store.recovery.recoverInvalidation(ctx, indexMutationTestError()); err != nil {
		t.Fatalf("repair indexes without a mutation target: %v", err)
	}
	if err := store.Connect(); err != nil {
		t.Fatalf("connect recovered database: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	var nonUniqueCount, uniqueCount int
	if err := store.DB().QueryRowContext(ctx, "SELECT count(*) FROM duckdb_indexes() WHERE NOT is_unique").Scan(&nonUniqueCount); err != nil {
		t.Fatalf("count non-unique indexes: %v", err)
	}
	if err := store.DB().QueryRowContext(ctx, "SELECT count(*) FROM duckdb_indexes() WHERE index_name = 'second_probe_unique_idx'").Scan(&uniqueCount); err != nil {
		t.Fatalf("count unique indexes: %v", err)
	}
	if nonUniqueCount != 0 || uniqueCount != 1 {
		t.Fatalf("unexpected fallback inventory: non_unique=%d unique=%d", nonUniqueCount, uniqueCount)
	}
	if status := store.DatabaseStatus(); status.RemovedUnsafeIndexes != 2 {
		t.Fatalf("expected two repaired fallback indexes, got %+v", status)
	}
}

func TestStoreDoesNotReportIndexRecoverySuccessWhenNothingWasRepaired(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "no-secondary-index.db")

	source := NewStore(dbPath, WithCheckpointInterval(0))
	if err := source.Connect(); err != nil {
		t.Fatalf("connect source: %v", err)
	}
	if _, err := source.DB().ExecContext(ctx, `
		CREATE TABLE primary_only (id BIGINT PRIMARY KEY, value VARCHAR);
		CREATE TABLE unrelated_probe (id BIGINT PRIMARY KEY, value VARCHAR);
		CREATE INDEX unrelated_probe_value_idx ON unrelated_probe(value);
	`); err != nil {
		t.Fatalf("seed primary-only database: %v", err)
	}
	if err := source.Close(); err != nil {
		t.Fatalf("close source: %v", err)
	}

	store := NewStore(dbPath,
		WithCheckpointInterval(0),
		WithAutomaticRecovery(true, filepath.Join(dir, "recovery")),
	)
	err := store.recovery.recoverInvalidation(ctx, indexMutationTestError("primary_only"))
	if err == nil || !strings.Contains(err.Error(), "no non-unique secondary indexes") {
		t.Fatalf("expected explicit no-repair error, got %v", err)
	}
	status := store.DatabaseStatus()
	if status.State != DatabaseStateNeedsAttention || status.LastRecoveryAt != nil || status.RemovedUnsafeIndexes != 0 {
		t.Fatalf("unexpected zero-repair status: %+v", status)
	}
	if _, err := os.Stat(store.recovery.markerPath()); err != nil {
		t.Fatalf("expected recovery marker to remain for operator diagnosis: %v", err)
	}
	db, err := store.openRawDB(ctx)
	if err != nil {
		t.Fatalf("open database after failed targeted repair: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var unrelatedIndexes int
	if err := db.QueryRowContext(ctx,
		"SELECT count(*) FROM duckdb_indexes() WHERE index_name = 'unrelated_probe_value_idx'").Scan(&unrelatedIndexes); err != nil {
		t.Fatalf("count unrelated indexes: %v", err)
	}
	if unrelatedIndexes != 1 {
		t.Fatalf("expected failed targeted repair to preserve unrelated index, got %d", unrelatedIndexes)
	}
}

func TestIndexRecoveryResumesAfterIndexesWereAlreadyDropped(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "resume-index-repair.db")
	recoveryRoot := filepath.Join(dir, "recovery")
	bundleDir := filepath.Join(recoveryRoot, "bundle")
	if err := os.MkdirAll(bundleDir, 0o700); err != nil {
		t.Fatalf("create bundle directory: %v", err)
	}

	store := NewStore(dbPath,
		WithCheckpointInterval(0),
		WithAutomaticRecovery(true, recoveryRoot),
	)
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx, `
		CREATE TABLE resume_probe (id BIGINT PRIMARY KEY, value VARCHAR);
		CREATE INDEX resume_probe_value_idx ON resume_probe(value);
		DROP INDEX resume_probe_value_idx;
	`); err != nil {
		t.Fatalf("prepare partially completed repair: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close before recovery resume: %v", err)
	}

	marker := &recoveryMarker{
		Version:              recoveryMarkerVersion,
		DatabaseID:           store.recovery.databaseID(),
		Kind:                 "remove_unsafe_indexes",
		Trigger:              "index_mutation_corruption",
		Phase:                "drop_unsafe_indexes",
		BundleDir:            bundleDir,
		RepairTable:          "resume_probe",
		RepairIndexes:        []string{"resume_probe_value_idx"},
		RemovedUnsafeIndexes: 1,
		CreatedAt:            time.Now().UTC(),
	}
	if err := store.recovery.writeMarker(marker); err != nil {
		t.Fatalf("write resumable index marker: %v", err)
	}
	if err := store.recovery.recoverStartup(ctx); err != nil {
		t.Fatalf("resume index recovery: %v", err)
	}
	if _, err := os.Stat(store.recovery.markerPath()); !os.IsNotExist(err) {
		t.Fatalf("expected resumed index marker to be cleared, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(bundleDir, "result.json")); err != nil {
		t.Fatalf("expected resumed index result: %v", err)
	}
	status := store.DatabaseStatus()
	if status.State != DatabaseStateHealthy || status.RemovedUnsafeIndexes != 1 || status.LastRecoveryAt == nil {
		t.Fatalf("unexpected resumed recovery status: %+v", status)
	}
}

func TestIndexRecoveryRediscoverIndexesFromLegacyPreDropMarker(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "legacy-index-repair.db")
	recoveryRoot := filepath.Join(dir, "recovery")
	bundleDir := filepath.Join(recoveryRoot, "bundle")
	if err := os.MkdirAll(bundleDir, 0o700); err != nil {
		t.Fatalf("create bundle directory: %v", err)
	}

	store := NewStore(dbPath,
		WithCheckpointInterval(0),
		WithAutomaticRecovery(true, recoveryRoot),
	)
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx, `
		CREATE TABLE legacy_probe (id BIGINT PRIMARY KEY, value VARCHAR);
		CREATE INDEX legacy_probe_value_idx ON legacy_probe(value);
	`); err != nil {
		t.Fatalf("prepare legacy recovery marker: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close before legacy recovery resume: %v", err)
	}

	marker := &recoveryMarker{
		Version:              recoveryMarkerVersion,
		DatabaseID:           store.recovery.databaseID(),
		Kind:                 "remove_unsafe_indexes",
		Trigger:              "index_mutation_corruption",
		Phase:                "drop_unsafe_indexes",
		BundleDir:            bundleDir,
		RemovedUnsafeIndexes: 1,
		CreatedAt:            time.Now().UTC(),
	}
	if err := store.recovery.writeMarker(marker); err != nil {
		t.Fatalf("write legacy index marker: %v", err)
	}
	if err := store.recovery.recoverStartup(ctx); err != nil {
		t.Fatalf("resume legacy index recovery: %v", err)
	}

	db, err := store.openRawDB(ctx)
	if err != nil {
		t.Fatalf("open recovered database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var indexes int
	if err := db.QueryRowContext(ctx,
		"SELECT count(*) FROM duckdb_indexes() WHERE index_name = 'legacy_probe_value_idx'").Scan(&indexes); err != nil {
		t.Fatalf("count legacy indexes: %v", err)
	}
	if indexes != 0 {
		t.Fatalf("expected legacy marker resume to rediscover and remove the index, got %d", indexes)
	}
}

func TestIndexMutationCorruptionClassifier(t *testing.T) {
	err := errors.New("FATAL Error: Failed to delete all rows from index. Only deleted 0 out of 1 rows. The database has been invalidated")
	if !isIndexMutationCorruption(err) {
		t.Fatal("expected issue #259 index failure to be allowlisted")
	}
	if isIndexMutationCorruption(errors.New("database has been invalidated after an out of memory error")) {
		t.Fatal("expected unrelated invalidation to remain outside automatic index recovery")
	}
	if isIndexMutationCorruption(errors.New("database has been invalidated while reading an index")) {
		t.Fatal("expected generic index-related invalidation to remain outside the exact allowlist")
	}
}

func TestRecoverInvalidationRequiresAutomaticRecoveryForExactIndexFailure(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "disabled.db"), WithAutomaticRecovery(false, ""))
	err := store.recovery.recoverInvalidation(context.Background(), errors.New(
		"FATAL Error: Failed to delete all rows from index. The database has been invalidated",
	))
	if err == nil || !strings.Contains(err.Error(), "automatic database recovery is disabled") {
		t.Fatalf("expected explicit disabled recovery error, got %v", err)
	}
	status := store.DatabaseStatus()
	if status.State != DatabaseStateNeedsAttention || status.Phase != "automatic_recovery_disabled" {
		t.Fatalf("unexpected disabled recovery status: %+v", status)
	}
}

func TestRecoverInvalidationReopensUnrelatedFatalErrorsWithoutRepair(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "generic.db"), WithAutomaticRecovery(true, t.TempDir()))
	store.status.recovering("drain_connections", "fatal_invalidation")

	if err := store.recovery.recoverInvalidation(
		context.Background(),
		errors.New("FATAL Error: database has been invalidated after an out of memory error"),
	); err != nil {
		t.Fatalf("expected generic invalidation to reopen without index repair: %v", err)
	}
	status := store.DatabaseStatus()
	if status.State != DatabaseStateHealthy || status.RecoveryBundleAvailable {
		t.Fatalf("unexpected generic invalidation status: %+v", status)
	}
}

func TestWALRecoveryRequiresExplicitOptInAfterRetainingBundle(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := createRecoveryTestDatabase(t, dir)
	walPath := dbPath + ".wal"
	if err := os.WriteFile(walPath, []byte("unreplayable WAL"), 0o600); err != nil {
		t.Fatalf("write synthetic WAL: %v", err)
	}

	store := NewStore(dbPath,
		WithCheckpointInterval(0),
		WithAutomaticRecovery(false, filepath.Join(dir, "recovery")),
	)
	err := store.recovery.recoverWAL(ctx, knownWALReplayTestError())
	if !errors.Is(err, errAutomaticWALRecoveryDisabled) {
		t.Fatalf("expected WAL opt-in error, got %v", err)
	}
	if _, err := os.Stat(walPath); err != nil {
		t.Fatalf("expected live WAL to remain untouched: %v", err)
	}
	if _, err := os.Stat(store.recovery.markerPath()); err != nil {
		t.Fatalf("expected resumable operator-action marker, got %v", err)
	}
	status := store.DatabaseStatus()
	if status.State != DatabaseStateNeedsAttention || status.Phase != "operator_action_required" || !status.RecoveryBundleAvailable {
		t.Fatalf("unexpected opt-in status: %+v", status)
	}
}

func TestWALRecoveryAwaitingOperatorReusesBundleAndAcceptsManualRemoval(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := createRecoveryTestDatabase(t, dir)
	walPath := dbPath + ".wal"
	if err := os.WriteFile(walPath, []byte("unreplayable WAL"), 0o600); err != nil {
		t.Fatalf("write synthetic WAL: %v", err)
	}

	recoveryRoot := filepath.Join(dir, "recovery")
	store := NewStore(dbPath,
		WithCheckpointInterval(0),
		WithAutomaticRecovery(true, recoveryRoot),
	)
	if err := store.recovery.recoverWAL(ctx, knownWALReplayTestError()); !errors.Is(err, errAutomaticWALRecoveryDisabled) {
		t.Fatalf("expected initial WAL opt-in error, got %v", err)
	}
	entriesBefore, err := os.ReadDir(recoveryRoot)
	if err != nil {
		t.Fatalf("read recovery root: %v", err)
	}
	if err := store.recovery.recoverStartup(ctx); !errors.Is(err, errAutomaticWALRecoveryDisabled) {
		t.Fatalf("expected resumed operator-action error, got %v", err)
	}
	entriesAfter, err := os.ReadDir(recoveryRoot)
	if err != nil {
		t.Fatalf("read recovery root after restart: %v", err)
	}
	if len(entriesAfter) != len(entriesBefore) {
		t.Fatalf("expected restart to reuse retained bundle, entries before=%d after=%d", len(entriesBefore), len(entriesAfter))
	}

	if err := os.Remove(walPath); err != nil {
		t.Fatalf("remove WAL manually: %v", err)
	}
	if err := store.recovery.recoverStartup(ctx); err != nil {
		t.Fatalf("complete manually resolved WAL recovery: %v", err)
	}
	if _, err := os.Stat(store.recovery.markerPath()); !os.IsNotExist(err) {
		t.Fatalf("expected operator-action marker to be cleared, got %v", err)
	}
}

func TestWALRecoveryOptInCompletesAndRemovesSyntheticWAL(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := createRecoveryTestDatabase(t, dir)
	walPath := dbPath + ".wal"
	if err := os.WriteFile(walPath, []byte("unreplayable WAL"), 0o600); err != nil {
		t.Fatalf("write synthetic WAL: %v", err)
	}

	store := NewStore(dbPath,
		WithCheckpointInterval(0),
		WithAutomaticRecovery(false, filepath.Join(dir, "recovery")),
		WithAutomaticWALRecovery(true),
	)
	if err := store.recovery.recoverWAL(ctx, knownWALReplayTestError()); err != nil {
		t.Fatalf("recover WAL: %v", err)
	}
	if _, err := os.Stat(walPath); !os.IsNotExist(err) {
		t.Fatalf("expected replay-failing WAL to be removed, got %v", err)
	}
	if _, err := os.Stat(store.recovery.markerPath()); !os.IsNotExist(err) {
		t.Fatalf("expected recovery marker to be cleared, got %v", err)
	}
	if status := store.DatabaseStatus(); status.State != DatabaseStateHealthy || !status.RecoveryBundleAvailable {
		t.Fatalf("unexpected completed WAL recovery status: %+v", status)
	}
}

func TestWALRecoveryResumesCleanupAfterAsideWasAlreadyRemoved(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := createRecoveryTestDatabase(t, dir)
	recoveryRoot := filepath.Join(dir, "recovery")
	bundleDir := filepath.Join(recoveryRoot, "bundle")
	if err := os.MkdirAll(bundleDir, 0o700); err != nil {
		t.Fatalf("create bundle directory: %v", err)
	}

	store := NewStore(dbPath,
		WithCheckpointInterval(0),
		WithAutomaticRecovery(true, recoveryRoot),
	)
	marker := &recoveryMarker{
		Version:      recoveryMarkerVersion,
		DatabaseID:   store.recovery.databaseID(),
		Kind:         "wal_bypass",
		Trigger:      "wal_replay_default_binding",
		Phase:        "cleanup_wal",
		BundleDir:    bundleDir,
		WALAsidePath: dbPath + ".wal.recovery-test",
		CreatedAt:    time.Now().UTC(),
	}
	if err := store.recovery.writeMarker(marker); err != nil {
		t.Fatalf("write recovery marker: %v", err)
	}
	if err := store.recovery.recoverStartup(ctx); err != nil {
		t.Fatalf("resume WAL cleanup: %v", err)
	}
	if _, err := os.Stat(store.recovery.markerPath()); !os.IsNotExist(err) {
		t.Fatalf("expected resumed marker to be cleared, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(bundleDir, "result.json")); err != nil {
		t.Fatalf("expected completed recovery result: %v", err)
	}
}

func TestWALRecoveryResumesCheckpointWithAsidePresent(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := createRecoveryTestDatabase(t, dir)
	recoveryRoot := filepath.Join(dir, "recovery")
	bundleDir := filepath.Join(recoveryRoot, "bundle")
	if err := os.MkdirAll(bundleDir, 0o700); err != nil {
		t.Fatalf("create bundle directory: %v", err)
	}
	asidePath := dbPath + ".wal.recovery-test"
	if err := os.WriteFile(asidePath, []byte("retained replay-failing WAL"), 0o600); err != nil {
		t.Fatalf("write aside WAL: %v", err)
	}

	store := NewStore(dbPath,
		WithCheckpointInterval(0),
		WithAutomaticRecovery(true, recoveryRoot),
	)
	marker := &recoveryMarker{
		Version:      recoveryMarkerVersion,
		DatabaseID:   store.recovery.databaseID(),
		Kind:         "wal_bypass",
		Trigger:      "wal_replay_default_binding",
		Phase:        "checkpoint_without_wal",
		BundleDir:    bundleDir,
		WALAsidePath: asidePath,
		CreatedAt:    time.Now().UTC(),
	}
	if err := store.recovery.writeMarker(marker); err != nil {
		t.Fatalf("write recovery marker: %v", err)
	}
	if err := store.recovery.recoverStartup(ctx); err != nil {
		t.Fatalf("resume WAL checkpoint: %v", err)
	}
	if _, err := os.Stat(asidePath); !os.IsNotExist(err) {
		t.Fatalf("expected aside WAL cleanup, got %v", err)
	}
}

func createRecoveryTestDatabase(t *testing.T, dir string) string {
	t.Helper()
	dbPath := filepath.Join(dir, "base.db")
	store := NewStore(dbPath, WithCheckpointInterval(0))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect recovery test database: %v", err)
	}
	if _, err := store.DB().Exec("CREATE TABLE recovery_base (id BIGINT); INSERT INTO recovery_base VALUES (1);"); err != nil {
		t.Fatalf("seed recovery test database: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close recovery test database: %v", err)
	}
	return dbPath
}

func knownWALReplayTestError() error {
	return errors.New(
		`INTERNAL Error: Failure while replaying WAL file: ` +
			`duckdb::WriteAheadLogDeserializer::ReplayAlter ` +
			`duckdb::Binder::BindDefaultValues ` +
			`duckdb::DatabaseManager::GetDefaultDatabase`,
	)
}
