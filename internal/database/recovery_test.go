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

func TestStoreRepairsOnlyKnownUnsafeIndexes(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "unsafe.db")

	source := NewStore(dbPath, WithCheckpointInterval(0))
	if err := source.Connect(); err != nil {
		t.Fatalf("connect source: %v", err)
	}
	if _, err := source.DB().ExecContext(ctx, `
		CREATE TABLE google_search_console_sync_state (
			site_id UUID PRIMARY KEY,
			team_id UUID NOT NULL,
			state VARCHAR NOT NULL,
			next_retry_at TIMESTAMPTZ
		);
		CREATE INDEX idx_gsc_sync_state_team_state
			ON google_search_console_sync_state(team_id, state);
		CREATE TABLE recovery_probe (id BIGINT, value VARCHAR);
		CREATE INDEX idx_recovery_probe_value ON recovery_probe(value);
		INSERT INTO google_search_console_sync_state
			VALUES (uuid(), uuid(), 'idle', NULL);
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
	if err := store.recovery.repairIndexes(ctx, "index_mutation_corruption"); err != nil {
		t.Fatalf("repair unsafe indexes: %v", err)
	}
	if err := store.Connect(); err != nil {
		t.Fatalf("connect recovered database: %v", err)
	}
	var unsafeCount, retainedCount int
	if err := store.DB().QueryRowContext(ctx,
		"SELECT count(*) FROM duckdb_indexes() WHERE index_name = 'idx_gsc_sync_state_team_state'").Scan(&unsafeCount); err != nil {
		t.Fatalf("count unsafe indexes: %v", err)
	}
	if err := store.DB().QueryRowContext(ctx,
		"SELECT count(*) FROM duckdb_indexes() WHERE index_name = 'idx_recovery_probe_value'").Scan(&retainedCount); err != nil {
		t.Fatalf("count retained indexes: %v", err)
	}
	if unsafeCount != 0 || retainedCount != 1 {
		t.Fatalf("unexpected recovered index inventory: unsafe=%d retained=%d", unsafeCount, retainedCount)
	}

	status := store.DatabaseStatus()
	if status.State != DatabaseStateHealthy || !status.RecoveryBundleAvailable || status.RemovedUnsafeIndexes != 1 {
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
	if !reopenedStatus.RecoveryBundleAvailable || reopenedStatus.LastRecoveryAt == nil || reopenedStatus.RemovedUnsafeIndexes != 1 {
		t.Fatalf("expected retained recovery history after restart, got %+v", reopenedStatus)
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
