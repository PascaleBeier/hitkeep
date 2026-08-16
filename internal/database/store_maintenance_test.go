package database

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCheckpointUsesStoreLoggerWithoutContextLogger(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	store := NewStore(":memory:", WithLogger(logger), WithCheckpointInterval(0))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer store.Close()
	logs.Reset()

	if err := store.Checkpoint(context.Background(), "test"); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	if !strings.Contains(logs.String(), "msg=\"Database checkpoint completed\"") {
		t.Fatalf("checkpoint did not use the injected store logger: %s", logs.String())
	}
}

func maintenanceRunning(s *Store) bool {
	s.maintenanceMu.Lock()
	defer s.maintenanceMu.Unlock()
	return s.maintenanceCancel != nil
}

func insertTestTenant(t *testing.T, store *Store) uuid.UUID {
	t.Helper()
	tenantID := uuid.New()
	if _, err := store.DB().ExecContext(context.Background(),
		"INSERT INTO tenants (id, name, created_at) VALUES (?, ?, ?)",
		tenantID, "Maintenance Tenant", time.Now().UTC(),
	); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	return tenantID
}

func TestTenantStoreOpenedAfterStartMaintenanceRunsCheckpoints(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	shared := newSharedMaintenanceTestStore(t)
	mgr := NewTenantStoreManager(shared, t.TempDir())
	t.Cleanup(func() { _ = mgr.Close() })
	mgr.StartMaintenance(ctx)

	tenantStore, err := mgr.ForTenant(ctx, insertTestTenant(t, shared))
	if err != nil {
		t.Fatalf("ForTenant: %v", err)
	}
	if !maintenanceRunning(tenantStore) {
		t.Fatal("expected tenant store opened after StartMaintenance to run checkpoint maintenance")
	}
}

func TestTenantStoreOpenedBeforeStartMaintenanceRunsCheckpoints(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	shared := newSharedMaintenanceTestStore(t)
	mgr := NewTenantStoreManager(shared, t.TempDir())
	t.Cleanup(func() { _ = mgr.Close() })

	tenantStore, err := mgr.ForTenant(ctx, insertTestTenant(t, shared))
	if err != nil {
		t.Fatalf("ForTenant: %v", err)
	}
	if maintenanceRunning(tenantStore) {
		t.Fatal("expected no maintenance before StartMaintenance")
	}

	mgr.StartMaintenance(ctx)
	if !maintenanceRunning(tenantStore) {
		t.Fatal("expected StartMaintenance to cover already-open tenant stores")
	}
}

func TestCloseStopsMaintenance(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "maint.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	store.StartMaintenance(context.Background())
	if !maintenanceRunning(store) {
		t.Fatal("expected maintenance to be running after StartMaintenance")
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if maintenanceRunning(store) {
		t.Fatal("expected Close to stop maintenance")
	}
}

func TestCloseIsIdempotentAndQueriesReturnClosedError(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "closed.db"), WithCheckpointInterval(0))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if _, err := store.DB().ExecContext(context.Background(), "SELECT 1"); err == nil {
		t.Fatal("expected a query against the closed store to return an error")
	}
}

func TestTenantStoreManagerForwardsFatalErrorsAndAvailability(t *testing.T) {
	shared := newSharedTestStore(t)
	mgr := NewTenantStoreManager(shared, t.TempDir())
	t.Cleanup(func() { _ = mgr.Close() })

	tenantID := uuid.New()
	tenantStore := NewStore(":memory:", withFatalReporter(mgr.tenantFatalReporter(tenantID)))
	mgr.mu.Lock()
	mgr.stores[tenantID] = tenantStore
	mgr.mu.Unlock()

	tenantStore.reportDrainTimeout(errors.New("synthetic drain timeout"))

	err := <-mgr.FatalErrors()
	if err == nil || !strings.Contains(err.Error(), "synthetic drain timeout") {
		t.Fatalf("unexpected forwarded fatal error: %v", err)
	}

	status, unavailable := mgr.UnavailableDatabaseStatus()
	if !unavailable || status.State != DatabaseStateNeedsAttention {
		t.Fatalf("expected unavailable tenant database status, got unavailable=%v status=%+v", unavailable, status)
	}
}

func TestTenantStoreInvalidationRequestsControlledRestart(t *testing.T) {
	reported := make(chan error, 1)
	store := NewStore(":memory:", withFatalReporter(func(err error) {
		reported <- err
	}))
	trigger := errors.New("synthetic database has been invalidated")
	store.observeInvalidation(trigger)

	err := <-reported
	if err == nil || !strings.Contains(err.Error(), "controlled restart required") || !errors.Is(err, trigger) {
		t.Fatalf("unexpected controlled restart error: %v", err)
	}
	if status := store.DatabaseStatus(); status.State != DatabaseStateRecovering {
		t.Fatalf("expected recovering tenant status, got %+v", status)
	}
}

func TestTenantStoreInvalidationDefersRecoveryUntilRestart(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "tenant.db"),
		WithAutomaticRecovery(true, t.TempDir()),
		withFatalReporter(func(error) {}),
	)
	trigger := errors.New("synthetic database has been invalidated")

	err := store.recoverAfterInvalidation(context.Background(), trigger)
	if err == nil || !strings.Contains(err.Error(), "controlled restart") || !errors.Is(err, trigger) {
		t.Fatalf("expected deferred tenant recovery error, got %v", err)
	}
	if _, statErr := os.Stat(store.recovery.markerPath()); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("tenant invalidation created an in-process recovery marker: %v", statErr)
	}
}

func TestTenantStoreOpenRecoveryFailureRequestsControlledRestart(t *testing.T) {
	ctx := context.Background()
	shared := newSharedTestStore(t)
	basePath := t.TempDir()
	tenantID := insertTestTenant(t, shared)
	dbPath := filepath.Join(basePath, "tenants", tenantID.String(), "hitkeep.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatalf("create tenant database directory: %v", err)
	}

	seed := NewStore(dbPath, WithCheckpointInterval(0))
	if err := seed.Connect(); err != nil {
		t.Fatalf("connect tenant seed database: %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("close tenant seed database: %v", err)
	}
	if err := os.WriteFile(dbPath+".wal", []byte("synthetic unreplayable WAL"), 0o600); err != nil {
		t.Fatalf("write tenant WAL: %v", err)
	}

	pending := NewStore(dbPath, WithCheckpointInterval(0))
	bundleDir := filepath.Join(pending.recovery.root(), "retained-bundle")
	if err := os.MkdirAll(bundleDir, 0o700); err != nil {
		t.Fatalf("create retained bundle directory: %v", err)
	}
	if err := pending.recovery.writeMarker(&recoveryMarker{
		Version:      recoveryMarkerVersion,
		DatabaseID:   pending.recovery.databaseID(),
		Kind:         "wal_bypass",
		Trigger:      "wal_replay_default_binding",
		Phase:        "awaiting_operator",
		BundleDir:    bundleDir,
		WALAsidePath: dbPath + ".wal.recovery-test",
		CreatedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("write pending tenant recovery marker: %v", err)
	}

	mgr := NewTenantStoreManager(shared, basePath)
	t.Cleanup(func() { _ = mgr.Close() })
	if _, err := mgr.ForTenant(ctx, tenantID); !errors.Is(err, errAutomaticWALRecoveryDisabled) {
		t.Fatalf("expected tenant WAL recovery opt-in error, got %v", err)
	}

	select {
	case err := <-mgr.FatalErrors():
		if err == nil || !strings.Contains(err.Error(), tenantID.String()) {
			t.Fatalf("unexpected controlled restart error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for tenant controlled restart request")
	}
}
