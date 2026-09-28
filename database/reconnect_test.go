package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const fakeInvalidationMessage = "FATAL Error: Failed: database has been invalidated because of a previous fatal error. The database must be restarted prior to being used again."

func reconnectingTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestReconnectingConnectorRequiresLogger(t *testing.T) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected a nil logger to panic")
		}
	}()
	newReconnectingConnector("fake.db", nil, nil, nil)
}

type fakeInstance struct {
	id          int
	invalidated atomic.Bool
	indexFatal  atomic.Bool
	closed      atomic.Bool
	execs       atomic.Int64
}

func (f *fakeInstance) Connect(context.Context) (driver.Conn, error) {
	return &fakeConn{instance: f}, nil
}

func (f *fakeInstance) Driver() driver.Driver { return nil }

func (f *fakeInstance) Close() error {
	f.closed.Store(true)
	return nil
}

type fakeConn struct {
	instance *fakeInstance
}

func (c *fakeConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("fake driver does not prepare")
}

func (c *fakeConn) Close() error { return nil }

func (c *fakeConn) Begin() (driver.Tx, error) {
	return nil, errors.New("fake driver does not begin")
}

func (c *fakeConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	if c.instance.indexFatal.Swap(false) {
		return nil, errors.New("FATAL Error: Invalid Input Error: Failed to delete all rows from index. Only deleted 0 out of 1 rows")
	}
	if c.instance.invalidated.Load() {
		return nil, errors.New(fakeInvalidationMessage)
	}
	c.instance.execs.Add(1)
	return driver.RowsAffected(1), nil
}

type fakeFactory struct {
	mu        sync.Mutex
	instances []*fakeInstance
}

func (f *fakeFactory) new() (driver.Connector, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	instance := &fakeInstance{id: len(f.instances) + 1}
	f.instances = append(f.instances, instance)
	return instance, nil
}

func (f *fakeFactory) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.instances)
}

func (f *fakeFactory) instance(i int) *fakeInstance {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.instances[i]
}

func TestReconnectingConnectorHealsAfterInvalidation(t *testing.T) {
	ctx := context.Background()
	factory := &fakeFactory{}
	db := sql.OpenDB(newReconnectingConnector("fake.db", factory.new, nil, reconnectingTestLogger()))
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.ExecContext(ctx, "SELECT 1"); err != nil {
		t.Fatalf("healthy exec: %v", err)
	}

	factory.instance(0).invalidated.Store(true)

	// The incident surfaces once with the original error for observability.
	if _, err := db.ExecContext(ctx, "SELECT 1"); err == nil || !strings.Contains(err.Error(), "database has been invalidated") {
		t.Fatalf("expected the invalidation error to surface, got %v", err)
	}

	// Subsequent use heals transparently on a fresh instance.
	if _, err := db.ExecContext(ctx, "SELECT 1"); err != nil {
		t.Fatalf("expected exec to heal after invalidation, got %v", err)
	}
	if factory.count() != 2 {
		t.Fatalf("expected a second instance to be opened, got %d", factory.count())
	}
	if !factory.instance(0).closed.Load() {
		t.Fatal("expected the invalidated instance to be closed (releasing the file lock)")
	}
	if factory.instance(1).execs.Load() == 0 {
		t.Fatal("expected the healed exec to run on the new instance")
	}
}

func TestReconnectingConnectorRunsRecoveryAfterConnectionsDrain(t *testing.T) {
	ctx := context.Background()
	factory := &fakeFactory{}
	var recoveryCalls atomic.Int64
	db := sql.OpenDB(newReconnectingConnector("fake.db", factory.new, func(_ context.Context, trigger error) error {
		if trigger == nil || !strings.Contains(trigger.Error(), "database has been invalidated") {
			t.Fatalf("unexpected recovery trigger: %v", trigger)
		}
		recoveryCalls.Add(1)
		return nil
	}, reconnectingTestLogger()))
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.ExecContext(ctx, "SELECT 1"); err != nil {
		t.Fatalf("healthy exec: %v", err)
	}
	factory.instance(0).invalidated.Store(true)
	if _, err := db.ExecContext(ctx, "SELECT 1"); err == nil {
		t.Fatal("expected invalidation to surface")
	}
	if _, err := db.ExecContext(ctx, "SELECT 1"); err != nil {
		t.Fatalf("expected recovery followed by a fresh connection, got %v", err)
	}
	if recoveryCalls.Load() != 1 {
		t.Fatalf("expected one recovery callback, got %d", recoveryCalls.Load())
	}
}

func TestReconnectingConnectorRecoversInitialIndexFatalWithMutationTable(t *testing.T) {
	ctx := context.Background()
	factory := &fakeFactory{}
	var recoveryCalls atomic.Int64
	var repairTable string
	db := sql.OpenDB(newReconnectingConnector("fake.db", factory.new, func(_ context.Context, trigger error) error {
		recoveryCalls.Add(1)
		repairTable = repairTableFromError(trigger)
		return nil
	}, reconnectingTestLogger()))
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.ExecContext(ctx, "SELECT 1"); err != nil {
		t.Fatalf("healthy exec: %v", err)
	}
	factory.instance(0).indexFatal.Store(true)
	if _, err := db.ExecContext(ctx, "INSERT INTO site_activity_summary (site_id) VALUES (?) ON CONFLICT (site_id) DO UPDATE SET updated_at = now()", "site-id"); err == nil || !strings.Contains(strings.ToLower(err.Error()), "failed to delete all rows from index") {
		t.Fatalf("expected original index failure to surface, got %v", err)
	}

	if _, err := db.ExecContext(ctx, "SELECT 1"); err != nil {
		t.Fatalf("expected recovery followed by a fresh connection, got %v", err)
	}
	if recoveryCalls.Load() != 1 {
		t.Fatalf("expected one recovery callback, got %d", recoveryCalls.Load())
	}
	if repairTable != "site_activity_summary" {
		t.Fatalf("expected site_activity_summary repair target, got %q", repairTable)
	}
}

func TestReconnectingConnectorPrefersTargetedTriggerAfterGenericInvalidation(t *testing.T) {
	connector := newReconnectingConnector("fake.db", nil, nil, reconnectingTestLogger())
	connector.markDead(errors.New(fakeInvalidationMessage))
	connector.markDead(&databaseOperationError{
		err:   errors.New("FATAL Error: Invalid Input Error: Failed to delete all rows from index. Only deleted 0 out of 1 rows"),
		table: "site_activity_summary",
	})

	if got := repairTableFromError(connector.trigger); got != "site_activity_summary" {
		t.Fatalf("expected richer mutation target to replace generic trigger, got %q", got)
	}
}

func TestReconnectingConnectorDoesNotReopenAfterRecoveryFailure(t *testing.T) {
	ctx := context.Background()
	factory := &fakeFactory{}
	recoveryErr := errors.New("synthetic recovery failure")
	db := sql.OpenDB(newReconnectingConnector("fake.db", factory.new, func(context.Context, error) error {
		return recoveryErr
	}, reconnectingTestLogger()))
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.ExecContext(ctx, "SELECT 1"); err != nil {
		t.Fatalf("healthy exec: %v", err)
	}
	factory.instance(0).invalidated.Store(true)
	if _, err := db.ExecContext(ctx, "SELECT 1"); err == nil {
		t.Fatal("expected invalidation to surface")
	}
	if _, err := db.ExecContext(ctx, "SELECT 1"); !errors.Is(err, recoveryErr) {
		t.Fatalf("expected recovery failure to surface, got %v", err)
	}
	if factory.count() != 1 {
		t.Fatalf("expected no fresh instance after failed recovery, got %d", factory.count())
	}
}

func TestReconnectingConnectorFailsFastWhileDraining(t *testing.T) {
	ctx := context.Background()
	factory := &fakeFactory{}
	db := sql.OpenDB(newReconnectingConnector("fake.db", factory.new, nil, reconnectingTestLogger()))
	t.Cleanup(func() { _ = db.Close() })

	pinned, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("pin conn: %v", err)
	}

	factory.instance(0).invalidated.Store(true)
	if _, err := pinned.ExecContext(ctx, "SELECT 1"); err == nil {
		t.Fatal("expected pinned exec to fail after invalidation")
	}

	// While the dead instance still has an open connection, new connections
	// must fail fast instead of colliding with the held file lock.
	if _, err := db.ExecContext(ctx, "SELECT 1"); err == nil {
		t.Fatal("expected exec to fail while the dead instance is draining")
	}
	if factory.count() != 1 {
		t.Fatalf("expected no new instance while draining, got %d", factory.count())
	}

	if err := pinned.Close(); err != nil && !errors.Is(err, driver.ErrBadConn) {
		t.Fatalf("close pinned conn: %v", err)
	}

	if _, err := db.ExecContext(ctx, "SELECT 1"); err != nil {
		t.Fatalf("expected exec to heal once drained, got %v", err)
	}
	if !factory.instance(0).closed.Load() {
		t.Fatal("expected drained instance to be closed")
	}
}

func TestReconnectingConnectorReportsDrainTimeout(t *testing.T) {
	ctx := context.Background()
	factory := &fakeFactory{}
	connector := newReconnectingConnector("fake.db", factory.new, nil, reconnectingTestLogger())
	timedOut := make(chan error, 1)
	connector.configureDrainWatchdog(10*time.Millisecond, func(err error) {
		timedOut <- err
	})
	db := sql.OpenDB(connector)
	t.Cleanup(func() { _ = db.Close() })

	pinned, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("pin conn: %v", err)
	}
	t.Cleanup(func() { _ = pinned.Close() })
	factory.instance(0).invalidated.Store(true)
	if _, err := pinned.ExecContext(ctx, "SELECT 1"); err == nil {
		t.Fatal("expected pinned exec to invalidate the database")
	}

	select {
	case err := <-timedOut:
		if err == nil || !strings.Contains(err.Error(), "drain timed out") {
			t.Fatalf("unexpected drain timeout error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for drain watchdog")
	}
}

func TestIsInvalidatedDatabaseError(t *testing.T) {
	if !isInvalidatedDatabaseError(errors.New(fakeInvalidationMessage)) {
		t.Fatal("expected the real DuckDB message to match")
	}
	if isInvalidatedDatabaseError(errors.New("out of memory")) {
		t.Fatal("expected unrelated errors not to match")
	}
	if isInvalidatedDatabaseError(nil) {
		t.Fatal("expected nil not to match")
	}
}

func TestMutationTableExtraction(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{
			name:  "upsert keeps insert target rather than conflict keyword",
			query: "INSERT INTO site_activity_summary (site_id) VALUES (?) ON CONFLICT (site_id) DO UPDATE SET updated_at = now()",
			want:  "site_activity_summary",
		},
		{
			name:  "quoted schema-qualified insert",
			query: `INSERT INTO "main"."site_activity_summary" (site_id) VALUES (?)`,
			want:  "site_activity_summary",
		},
		{
			name:  "update",
			query: "UPDATE webhook_deliveries SET status = ? WHERE id = ?",
			want:  "webhook_deliveries",
		},
		{
			name:  "delete",
			query: "DELETE FROM rollup_dirty_buckets WHERE site_id = ?",
			want:  "rollup_dirty_buckets",
		},
		{
			name:  "read-only query",
			query: "SELECT * FROM site_activity_summary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mutationTable(tt.query)
			if tt.want == "" {
				if got != "" {
					t.Fatalf("expected no mutation target, got %q", got)
				}
				return
			}
			if got != tt.want {
				t.Fatalf("expected mutation target %q, got %q", tt.want, got)
			}
		})
	}
}
