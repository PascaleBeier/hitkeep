package database

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
)

// invalidatedDatabaseMarker is DuckDB's fatal-error signature after an
// unrecoverable in-memory failure (e.g. an out-of-memory during an index
// mutation). Once emitted, every operation on that database instance fails
// until the instance is reopened; the on-disk file itself recovers via WAL
// replay.
const invalidatedDatabaseMarker = "database has been invalidated"

// errDatabaseRecovering is returned to callers that hit the short window in
// which the invalidated instance is still draining its open connections.
var errDatabaseRecovering = errors.New("database is recovering from a fatal error, retry shortly")

func isInvalidatedDatabaseError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), invalidatedDatabaseMarker)
}

// duckdbConnUnwrapper exposes the raw DuckDB driver connection behind the
// reconnecting wrapper, for APIs that type-assert on it (the appender).
type duckdbConnUnwrapper interface {
	UnwrapDuckDBConn() driver.Conn
}

// reconnectingConnector wraps a DuckDB connector and transparently reopens
// the database after a fatal invalidation. DuckDB holds a file lock per
// instance, so a fresh instance can only be opened once every connection of
// the dead one is closed; until then new connections fail fast with
// errDatabaseRecovering. database/sql drains dead pooled connections
// naturally: every operation on them reports driver.ErrBadConn.
type reconnectingConnector struct {
	path    string
	factory func() (driver.Connector, error)

	mu        sync.Mutex
	inner     driver.Connector
	openConns int
	dead      bool
}

func newReconnectingConnector(path string, factory func() (driver.Connector, error)) *reconnectingConnector {
	return &reconnectingConnector{path: path, factory: factory}
}

func (c *reconnectingConnector) Connect(ctx context.Context) (driver.Conn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.dead {
		if c.openConns > 0 {
			return nil, fmt.Errorf("%s: %w", c.path, errDatabaseRecovering)
		}
		c.closeInnerLocked()
	}

	if c.inner == nil {
		inner, err := c.factory()
		if err != nil {
			return nil, fmt.Errorf("open database %s: %w", c.path, err)
		}
		c.inner = inner
	}

	conn, err := c.inner.Connect(ctx)
	if err != nil {
		return nil, err
	}
	c.openConns++
	return &reconnectingConn{inner: conn, connector: c}, nil
}

func (c *reconnectingConnector) Driver() driver.Driver { return nil }

// Close shuts down the current instance; called by sql.DB.Close.
func (c *reconnectingConnector) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	inner := c.inner
	c.inner = nil
	c.dead = false
	if closer, ok := inner.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// markDead flags the current instance after an invalidation error. The first
// caller logs the incident; the swap happens once the pool has drained.
func (c *reconnectingConnector) markDead() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dead {
		return
	}
	c.dead = true
	slog.Error("DuckDB database invalidated by a fatal error; reopening after connections drain",
		"path", c.path, "open_connections", c.openConns)
}

func (c *reconnectingConnector) isDead() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dead
}

// connClosed releases one pooled connection; the last one out closes the
// dead instance so the next Connect can reopen the file.
func (c *reconnectingConnector) connClosed() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.openConns > 0 {
		c.openConns--
	}
	if c.dead && c.openConns == 0 {
		c.closeInnerLocked()
	}
}

func (c *reconnectingConnector) closeInnerLocked() {
	if c.inner == nil {
		c.dead = false
		return
	}
	if closer, ok := c.inner.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			slog.Warn("Failed to close invalidated DuckDB instance", "path", c.path, "error", err)
		}
	}
	c.inner = nil
	c.dead = false
	slog.Info("Reopening DuckDB database after fatal invalidation", "path", c.path)
}

// observe inspects an operation error and flags the instance on invalidation.
func (c *reconnectingConnector) observe(err error) {
	if isInvalidatedDatabaseError(err) {
		c.markDead()
	}
}

// reconnectingConn wraps a DuckDB connection. Operations on a dead instance
// report driver.ErrBadConn so database/sql discards the connection and
// retries on a fresh one.
type reconnectingConn struct {
	inner     driver.Conn
	connector *reconnectingConnector
	closed    bool
}

func (c *reconnectingConn) UnwrapDuckDBConn() driver.Conn { return c.inner }

func (c *reconnectingConn) Prepare(query string) (driver.Stmt, error) {
	if c.connector.isDead() {
		return nil, driver.ErrBadConn
	}
	stmt, err := c.inner.Prepare(query)
	c.connector.observe(err)
	if err != nil {
		return nil, err
	}
	return &reconnectingStmt{inner: stmt, connector: c.connector}, nil
}

func (c *reconnectingConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if c.connector.isDead() {
		return nil, driver.ErrBadConn
	}
	preparer, ok := c.inner.(driver.ConnPrepareContext)
	if !ok {
		return c.Prepare(query)
	}
	stmt, err := preparer.PrepareContext(ctx, query)
	c.connector.observe(err)
	if err != nil {
		return nil, err
	}
	return &reconnectingStmt{inner: stmt, connector: c.connector}, nil
}

func (c *reconnectingConn) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true
	err := c.inner.Close()
	c.connector.connClosed()
	return err
}

func (c *reconnectingConn) Begin() (driver.Tx, error) {
	if c.connector.isDead() {
		return nil, driver.ErrBadConn
	}
	tx, err := c.inner.Begin() //nolint:staticcheck // driver.Conn interface compliance; BeginTx is preferred below.
	c.connector.observe(err)
	if err != nil {
		return nil, err
	}
	return &reconnectingTx{inner: tx, connector: c.connector}, nil
}

func (c *reconnectingConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if c.connector.isDead() {
		return nil, driver.ErrBadConn
	}
	beginner, ok := c.inner.(driver.ConnBeginTx)
	if !ok {
		return c.Begin()
	}
	tx, err := beginner.BeginTx(ctx, opts)
	c.connector.observe(err)
	if err != nil {
		return nil, err
	}
	return &reconnectingTx{inner: tx, connector: c.connector}, nil
}

func (c *reconnectingConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if c.connector.isDead() {
		return nil, driver.ErrBadConn
	}
	execer, ok := c.inner.(driver.ExecerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	result, err := execer.ExecContext(ctx, query, args)
	c.connector.observe(err)
	return result, err
}

func (c *reconnectingConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if c.connector.isDead() {
		return nil, driver.ErrBadConn
	}
	queryer, ok := c.inner.(driver.QueryerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	rows, err := queryer.QueryContext(ctx, query, args)
	c.connector.observe(err)
	return rows, err
}

func (c *reconnectingConn) CheckNamedValue(nv *driver.NamedValue) error {
	if checker, ok := c.inner.(driver.NamedValueChecker); ok {
		return checker.CheckNamedValue(nv)
	}
	return driver.ErrSkip
}

// IsValid lets database/sql discard dead connections when they return to the
// pool instead of parking them as idle.
func (c *reconnectingConn) IsValid() bool {
	if c.connector.isDead() {
		return false
	}
	if validator, ok := c.inner.(driver.Validator); ok {
		return validator.IsValid()
	}
	return true
}

func (c *reconnectingConn) ResetSession(ctx context.Context) error {
	if c.connector.isDead() {
		return driver.ErrBadConn
	}
	if resetter, ok := c.inner.(driver.SessionResetter); ok {
		return resetter.ResetSession(ctx)
	}
	return nil
}

type reconnectingStmt struct {
	inner     driver.Stmt
	connector *reconnectingConnector
}

func (s *reconnectingStmt) Close() error  { return s.inner.Close() }
func (s *reconnectingStmt) NumInput() int { return s.inner.NumInput() }

func (s *reconnectingStmt) Exec(args []driver.Value) (driver.Result, error) {
	result, err := s.inner.Exec(args) //nolint:staticcheck // driver.Stmt interface compliance.
	s.connector.observe(err)
	return result, err
}

func (s *reconnectingStmt) Query(args []driver.Value) (driver.Rows, error) {
	rows, err := s.inner.Query(args) //nolint:staticcheck // driver.Stmt interface compliance.
	s.connector.observe(err)
	return rows, err
}

func (s *reconnectingStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	execer, ok := s.inner.(driver.StmtExecContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	result, err := execer.ExecContext(ctx, args)
	s.connector.observe(err)
	return result, err
}

func (s *reconnectingStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	queryer, ok := s.inner.(driver.StmtQueryContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	rows, err := queryer.QueryContext(ctx, args)
	s.connector.observe(err)
	return rows, err
}

func (s *reconnectingStmt) CheckNamedValue(nv *driver.NamedValue) error {
	if checker, ok := s.inner.(driver.NamedValueChecker); ok {
		return checker.CheckNamedValue(nv)
	}
	return driver.ErrSkip
}

type reconnectingTx struct {
	inner     driver.Tx
	connector *reconnectingConnector
}

func (t *reconnectingTx) Commit() error {
	err := t.inner.Commit()
	t.connector.observe(err)
	return err
}

func (t *reconnectingTx) Rollback() error {
	err := t.inner.Rollback()
	t.connector.observe(err)
	return err
}
