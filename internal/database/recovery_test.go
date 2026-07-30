package database

import (
	"context"
	"errors"
	"os"
	"os/exec"
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

func TestGuardedTenantMigrationSurvivesAbruptRestart(t *testing.T) {
	const (
		helperDBEnv       = "HITKEEP_TEST_ABRUPT_TENANT_MIGRATION_DB"
		helperRecoveryEnv = "HITKEEP_TEST_ABRUPT_TENANT_MIGRATION_RECOVERY"
		migrationFile     = "0013_drop_analytics_art_indexes.sql"
		eventsMigration   = "0013a_drop_events_art_indexes.sql"
		vitalsMigration   = "0013b_drop_web_vitals_art_indexes.sql"
		searchMigration   = "0016_drop_search_console_fact_art_indexes.sql"
	)
	if dbPath := os.Getenv(helperDBEnv); dbPath != "" {
		store := NewStore(dbPath,
			WithCheckpointInterval(0),
			WithAutomaticRecovery(false, os.Getenv(helperRecoveryEnv)),
		)
		if err := store.Connect(); err != nil {
			t.Fatalf("connect abrupt migration helper: %v", err)
		}
		committedHeavyMigrations := 0
		if err := store.migrateTenant(context.Background(), migrationRunOptions{
			guarded: true,
			afterCommit: func() {
				committedHeavyMigrations++
				if committedHeavyMigrations < 4 {
					return
				}
				// Deliberately skip the post-migration checkpoint and clean close so
				// the committed Search Console rebuild remains in the WAL.
				os.Exit(0)
			},
		}); err != nil {
			t.Fatalf("run abrupt tenant migration: %v", err)
		}
		t.Fatal("abrupt migration hook did not exit")
	}

	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tenant-migration-replay.db")
	recoveryRoot := filepath.Join(dir, "recovery")

	seed := NewStore(dbPath, WithCheckpointInterval(0))
	if err := seed.Connect(); err != nil {
		t.Fatalf("connect migration seed: %v", err)
	}
	if _, err := seed.DB().ExecContext(ctx, `
		CREATE TABLE migrations (migration VARCHAR NOT NULL, applied_at TIMESTAMPTZ NOT NULL);
		INSERT INTO migrations (migration, applied_at) VALUES (?, now()), (?, now()), (?, now()), (?, now())`,
		migrationFile, eventsMigration, vitalsMigration, searchMigration); err != nil {
		t.Fatalf("hold back replay migration: %v", err)
	}
	if err := seed.MigrateTenant(ctx); err != nil {
		t.Fatalf("prepare pre-migration tenant schema: %v", err)
	}
	if _, err := seed.DB().ExecContext(ctx, `
		INSERT INTO sites (id, domain, data_retention_days) VALUES (uuid(), 'migration.test', 365);
		INSERT INTO hits (id, site_id, session_id, page_id, timestamp, path)
		SELECT uuid(), id, uuid(), uuid(), now(), '/before-migration' FROM sites;
		INSERT INTO search_console_facts (site_id, property_uri, date, query, clicks, imported_at)
		SELECT id, 'sc-domain:migration.test', current_date, 'before migration', 7, now() FROM sites;
		DELETE FROM migrations WHERE migration IN (?, ?, ?, ?)`, migrationFile, eventsMigration, vitalsMigration, searchMigration); err != nil {
		t.Fatalf("seed checkpointed tenant data: %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("close migration seed: %v", err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestGuardedTenantMigrationSurvivesAbruptRestart$")
	cmd.Env = append(os.Environ(), helperDBEnv+"="+dbPath, helperRecoveryEnv+"="+recoveryRoot)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create replay-failing WAL: %v\n%s", err, output)
	}
	if _, err := os.Stat(dbPath + ".wal"); err != nil {
		t.Fatalf("expected committed migration WAL: %v", err)
	}

	store, err := OpenMigratedTenantStore(ctx, dbPath,
		WithCheckpointInterval(0),
		WithAutomaticRecovery(false, recoveryRoot),
	)
	if err != nil {
		t.Fatalf("replay and resume guarded migration WAL without global opt-in: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	var rows, searchRows, appliedMigrations, remainingIndexes, remainingSearchIndexes int
	if err := store.DB().QueryRowContext(ctx, "SELECT count(*) FROM hits WHERE path = '/before-migration'").Scan(&rows); err != nil {
		t.Fatalf("read base data after migration WAL recovery: %v", err)
	}
	if err := store.DB().QueryRowContext(ctx,
		"SELECT count(*) FROM migrations WHERE migration IN (?, ?, ?, ?)",
		migrationFile, eventsMigration, vitalsMigration, searchMigration).Scan(&appliedMigrations); err != nil {
		t.Fatalf("count reapplied migration: %v", err)
	}
	if err := store.DB().QueryRowContext(ctx,
		"SELECT count(*) FROM search_console_facts WHERE query = 'before migration' AND clicks = 7").Scan(&searchRows); err != nil {
		t.Fatalf("read Search Console data after migration WAL recovery: %v", err)
	}
	if err := store.DB().QueryRowContext(ctx, `
		SELECT count(*) FROM duckdb_indexes()
		WHERE table_name IN ('hits', 'events', 'web_vitals') AND NOT is_unique`).Scan(&remainingIndexes); err != nil {
		t.Fatalf("inspect migrated tenant indexes: %v", err)
	}
	if err := store.DB().QueryRowContext(ctx,
		"SELECT count(*) FROM duckdb_indexes() WHERE table_name = 'search_console_facts'").Scan(&remainingSearchIndexes); err != nil {
		t.Fatalf("inspect migrated Search Console indexes: %v", err)
	}
	if rows != 1 || searchRows != 1 || appliedMigrations != 4 || remainingIndexes != 0 || remainingSearchIndexes != 0 {
		t.Fatalf("unexpected recovered migration state: rows=%d search_rows=%d migrations=%d indexes=%d search_indexes=%d",
			rows, searchRows, appliedMigrations, remainingIndexes, remainingSearchIndexes)
	}
	if _, err := os.Stat(store.recovery.migrationGuardPath()); !os.IsNotExist(err) {
		t.Fatalf("expected completed migration guard to be cleared, got %v", err)
	}
	status := store.DatabaseStatus()
	if status.State != DatabaseStateHealthy {
		t.Fatalf("unexpected guarded migration status: %+v", status)
	}
	if store.RecoveredDuringConnect() {
		t.Fatalf("replay-safe guarded migration should checkpoint normally without WAL bypass recovery: %+v", status)
	}
	if status.RecoveryBundleAvailable {
		t.Fatal("checksum-verified migration WAL recovery must not duplicate the database into a recovery bundle")
	}
}

func TestConnectCheckpointsAndClearsStaleMigrationGuard(t *testing.T) {
	dir := t.TempDir()
	dbPath := createRecoveryTestDatabase(t, dir)
	store := NewStore(dbPath,
		WithCheckpointInterval(0),
		WithAutomaticRecovery(false, filepath.Join(dir, "recovery")),
	)
	if err := store.Connect(); err != nil {
		t.Fatalf("connect guard seed: %v", err)
	}
	if err := store.ensureMigrationCheckpointTable(context.Background()); err != nil {
		t.Fatalf("create guard checkpoint table: %v", err)
	}
	if err := store.prepareGuardedMigration(context.Background(), "shared", []string{"already_replayed.sql"}); err != nil {
		t.Fatalf("prepare stale migration guard: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close guard seed: %v", err)
	}

	reopened := NewStore(dbPath,
		WithCheckpointInterval(0),
		WithAutomaticRecovery(false, filepath.Join(dir, "recovery")),
	)
	if err := reopened.Connect(); err != nil {
		t.Fatalf("connect with replayable migration guard: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if _, err := os.Stat(reopened.recovery.migrationGuardPath()); !os.IsNotExist(err) {
		t.Fatalf("expected replayed migration guard to be cleared, got %v", err)
	}
	if reopened.DatabaseStatus().RecoveryBundleAvailable {
		t.Fatal("successful WAL replay must not create a recovery bundle")
	}
}

func TestConnectRejectsInvalidMigrationGuard(t *testing.T) {
	dir := t.TempDir()
	dbPath := createRecoveryTestDatabase(t, dir)
	store := NewStore(dbPath,
		WithCheckpointInterval(0),
		WithAutomaticRecovery(false, filepath.Join(dir, "recovery")),
	)
	if err := os.MkdirAll(store.recovery.root(), 0o700); err != nil {
		t.Fatalf("create recovery directory: %v", err)
	}
	if err := os.WriteFile(store.recovery.migrationGuardPath(), []byte(`{"version":1,"database_id":"wrong"}`), 0o600); err != nil {
		t.Fatalf("write invalid migration guard: %v", err)
	}

	err := store.Connect()
	if err == nil || !strings.Contains(err.Error(), "migration WAL guard does not match this database") {
		t.Fatalf("expected invalid migration guard to fail closed, got %v", err)
	}
}

func TestMigrationCheckpointMismatchRestoresApplicationWAL(t *testing.T) {
	const helperEnv = "HITKEEP_TEST_CREATE_APPLICATION_WAL"
	if dbPath := os.Getenv(helperEnv); dbPath != "" {
		db, err := openDuckDBFile(dbPath)
		if err != nil {
			t.Fatalf("open application WAL helper: %v", err)
		}
		if _, err := db.Exec("INSERT INTO application_writes VALUES (1)"); err != nil {
			t.Fatalf("write application WAL row: %v", err)
		}
		os.Exit(0)
	}

	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "checkpoint-mismatch.db")
	recoveryRoot := filepath.Join(dir, "recovery")

	seed := NewStore(dbPath,
		WithCheckpointInterval(0),
		WithAutomaticRecovery(false, recoveryRoot),
	)
	if err := seed.Connect(); err != nil {
		t.Fatalf("connect checkpoint mismatch seed: %v", err)
	}
	if _, err := seed.DB().ExecContext(ctx, "CREATE TABLE application_writes (id BIGINT)"); err != nil {
		t.Fatalf("create application write table: %v", err)
	}
	if err := seed.ensureMigrationCheckpointTable(ctx); err != nil {
		t.Fatalf("create migration checkpoint table: %v", err)
	}
	if err := seed.prepareGuardedMigration(ctx, "shared", []string{"stale.sql"}); err != nil {
		t.Fatalf("prepare migration checkpoint: %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("close checkpoint mismatch seed: %v", err)
	}

	guard, err := seed.recovery.loadMigrationGuard()
	if err != nil {
		t.Fatalf("load migration guard: %v", err)
	}
	guard.CheckpointToken = "restored-database-token"
	if err := writeJSONFile(seed.recovery.migrationGuardPath(), guard); err != nil {
		t.Fatalf("replace migration checkpoint identity: %v", err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestMigrationCheckpointMismatchRestoresApplicationWAL$")
	cmd.Env = append(os.Environ(), helperEnv+"="+dbPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create application WAL: %v\n%s", err, output)
	}

	recovering := NewStore(dbPath,
		WithCheckpointInterval(0),
		WithAutomaticRecovery(false, recoveryRoot),
	)
	err = recovering.recovery.recoverMigrationWAL(ctx, knownWALReplayTestError(), guard)
	if err == nil || !strings.Contains(err.Error(), "migration checkpoint identity did not match") {
		t.Fatalf("expected checkpoint mismatch to fail closed, got %v", err)
	}
	if _, err := os.Stat(dbPath + ".wal"); err != nil {
		t.Fatalf("expected application WAL to be restored unchanged: %v", err)
	}
	marker, err := recovering.recovery.loadMarker()
	if err != nil {
		t.Fatalf("load rejected recovery marker: %v", err)
	}
	if marker == nil || marker.MigrationWAL || marker.Phase != "awaiting_operator" {
		t.Fatalf("unexpected rejected migration recovery marker: %+v", marker)
	}

	if err := os.Remove(recovering.recovery.markerPath()); err != nil {
		t.Fatalf("clear rejected recovery marker for WAL verification: %v", err)
	}
	if err := recovering.recovery.clearMigrationGuard(); err != nil {
		t.Fatalf("clear stale migration guard for WAL verification: %v", err)
	}
	if err := recovering.Connect(); err != nil {
		t.Fatalf("replay restored application WAL: %v", err)
	}
	t.Cleanup(func() { _ = recovering.Close() })
	var rows int
	if err := recovering.DB().QueryRowContext(ctx, "SELECT count(*) FROM application_writes").Scan(&rows); err != nil {
		t.Fatalf("read restored application WAL row: %v", err)
	}
	if rows != 1 {
		t.Fatalf("expected application WAL row to survive, got %d", rows)
	}
}
