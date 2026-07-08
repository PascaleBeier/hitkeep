package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

const fakeInvalidationMessage = "FATAL Error: Failed: database has been invalidated because of a previous fatal error. The database must be restarted prior to being used again."

type fakeInstance struct {
	id          int
	invalidated atomic.Bool
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
	db := sql.OpenDB(newReconnectingConnector("fake.db", factory.new))
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

func TestReconnectingConnectorFailsFastWhileDraining(t *testing.T) {
	ctx := context.Background()
	factory := &fakeFactory{}
	db := sql.OpenDB(newReconnectingConnector("fake.db", factory.new))
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
