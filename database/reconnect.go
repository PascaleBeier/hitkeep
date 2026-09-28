package database

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"
)

// invalidatedDatabaseMarker is DuckDB's fatal-error signature after an
// unrecoverable in-memory failure (e.g. an out-of-memory during an index
// mutation). Once emitted, every operation on that database instance fails
// until the instance is reopened; the on-disk file itself recovers via WAL
// replay.
const invalidatedDatabaseMarker = "database has been invalidated"

// ErrDatabaseRecovering is returned to callers that hit the short window in
// which the invalidated instance is still draining its open connections.
var ErrDatabaseRecovering = errors.New("database is recovering from a fatal error, retry shortly")

var mutationTablePattern = regexp.MustCompile(`(?i)\b(?:INSERT\s+INTO|UPDATE|DELETE\s+FROM)\s+(?:(?:"?[a-z_][a-z0-9_]*"?)\s*\.\s*)?("?[a-z_][a-z0-9_]*"?)`)

type databaseOperationError struct {
	err   error
	table string
}

func (e *databaseOperationError) Error() string { return e.err.Error() }
func (e *databaseOperationError) Unwrap() error { return e.err }

func repairTableFromError(err error) string {
	var targeted *databaseOperationError
	if !errors.As(err, &targeted) || targeted == nil {
		return ""
	}
	return targeted.table
}

func mutationTable(query string) string {
	match := mutationTablePattern.FindStringSubmatch(query)
	if len(match) != 2 {
		return ""
	}
	table := strings.Trim(match[1], `"`)
	if table == "" {
		return ""
	}
	return table
}

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
	logger  *slog.Logger
	factory func() (driver.Connector, error)
	recover func(context.Context, error) error

	mu             sync.Mutex
	inner          driver.Connector
	connectionGate *connectionGate
	openConns      int
	dead           bool
	recovering     bool
	trigger        error
	drainTimer     *time.Timer

	drainTimeout   time.Duration
	onDrainTimeout func(error)
	onInvalidated  func(error)
}

type connectionGate struct {
	sem chan struct{}
}

func newConnectionGate(limit int) *connectionGate {
	if limit <= 0 {
		limit = 16
	}
	return &connectionGate{sem: make(chan struct{}, limit)}
}

func (g *connectionGate) acquire(ctx context.Context) error {
	if g == nil {
		return nil
	}
	select {
	case g.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (g *connectionGate) release() {
	if g == nil {
		return
	}
	select {
	case <-g.sem:
	default:
	}
}

func newReconnectingConnector(path string, factory func() (driver.Connector, error), recover func(context.Context, error) error, logger *slog.Logger) *reconnectingConnector {
	if logger == nil {
		panic("database: reconnecting connector logger is required")
	}
	return &reconnectingConnector{path: path, logger: logger, factory: factory, recover: recover}
}

func (c *reconnectingConnector) Connect(ctx context.Context) (driver.Conn, error) {
	if err := c.connectionGate.acquire(ctx); err != nil {
		return nil, err
	}
	release := true
	defer func() {
		if release {
			c.connectionGate.release()
		}
	}()
	c.mu.Lock()

	if c.dead {
		if c.openConns > 0 || c.recovering {
			c.mu.Unlock()
			return nil, fmt.Errorf("%s: %w", c.path, ErrDatabaseRecovering)
		}
		c.closeInnerLocked()
		c.recovering = true
		trigger := c.trigger
		c.mu.Unlock()

		var recoveryErr error
		if c.recover != nil {
			recoveryErr = c.recover(ctx, trigger)
		}

		c.mu.Lock()
		c.recovering = false
		if recoveryErr != nil {
			c.mu.Unlock()
			return nil, fmt.Errorf("%s: recovery failed: %w", c.path, recoveryErr)
		}
		c.dead = false
		c.trigger = nil
	}

	if c.inner == nil {
		inner, err := c.factory()
		if err != nil {
			c.mu.Unlock()
			return nil, fmt.Errorf("open database %s: %w", c.path, err)
		}
		c.inner = inner
	}

	conn, err := c.inner.Connect(ctx)
	if err != nil {
		c.mu.Unlock()
		return nil, err
	}
	c.openConns++
	c.mu.Unlock()
	release = false
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
	c.recovering = false
	c.trigger = nil
	if c.drainTimer != nil {
		c.drainTimer.Stop()
		c.drainTimer = nil
	}
	if closer, ok := inner.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// markDead flags the current instance after an invalidation error. The first
// caller logs the incident; the swap happens once the pool has drained.
func (c *reconnectingConnector) markDead(trigger error) {
	c.mu.Lock()
	if c.dead {
		if c.trigger == nil || (repairTableFromError(c.trigger) == "" && repairTableFromError(trigger) != "") {
			c.trigger = trigger
		}
		c.mu.Unlock()
		return
	}
	c.dead = true
	c.trigger = trigger
	observer := c.onInvalidated
	c.logger.Error("DuckDB database invalidated by a fatal error; reopening after connections drain",
		"path", c.path, "open_connections", c.openConns)
	c.startDrainWatchdogLocked()
	c.mu.Unlock()
	if observer != nil {
		observer(trigger)
	}
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
		c.connectionGate.release()
	}
	if c.dead && c.openConns == 0 {
		if c.drainTimer != nil {
			c.drainTimer.Stop()
			c.drainTimer = nil
		}
		c.closeInnerLocked()
	}
}

func (c *reconnectingConnector) configureDrainWatchdog(timeout time.Duration, callback func(error)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.drainTimeout = timeout
	c.onDrainTimeout = callback
}

func (c *reconnectingConnector) configureInvalidationObserver(callback func(error)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onInvalidated = callback
}

func (c *reconnectingConnector) startDrainWatchdogLocked() {
	if c.openConns == 0 || c.drainTimeout <= 0 || c.onDrainTimeout == nil || c.drainTimer != nil {
		return
	}
	c.drainTimer = time.AfterFunc(c.drainTimeout, func() {
		c.mu.Lock()
		if !c.dead || c.openConns == 0 {
			c.drainTimer = nil
			c.mu.Unlock()
			return
		}
		openConnections := c.openConns
		callback := c.onDrainTimeout
		c.drainTimer = nil
		c.mu.Unlock()
		callback(fmt.Errorf("database recovery drain timed out with %d open connection(s)", openConnections))
	})
}

func (c *reconnectingConnector) closeInnerLocked() {
	if c.inner == nil {
		return
	}
	if closer, ok := c.inner.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			c.logger.Warn("Failed to close invalidated DuckDB instance", "path", c.path, "error", err)
		}
	}
	c.inner = nil
	c.logger.Info("Closed invalidated DuckDB database before recovery", "path", c.path)
}

// observe inspects an operation error and flags the instance on invalidation.
// The first index-mutation fatal does not always include DuckDB's generic
// invalidation marker, so retain its mutation target before later operations
// can replace it with a context-free "previous fatal error" message.
func (c *reconnectingConnector) observe(err error, query string) {
	if isIndexMutationCorruption(err) {
		c.markDead(&databaseOperationError{err: err, table: mutationTable(query)})
		return
	}
	if isInvalidatedDatabaseError(err) {
		c.markDead(err)
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
	c.connector.observe(err, query)
	if err != nil {
		return nil, err
	}
	return &reconnectingStmt{inner: stmt, connector: c.connector, query: query}, nil
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
	c.connector.observe(err, query)
	if err != nil {
		return nil, err
	}
	return &reconnectingStmt{inner: stmt, connector: c.connector, query: query}, nil
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
	//lint:ignore SA1019 driver.Conn interface compliance; BeginTx is preferred below.
	tx, err := c.inner.Begin() //nolint:staticcheck // driver.Conn interface compliance; BeginTx is preferred below.
	c.connector.observe(err, "")
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
	c.connector.observe(err, "")
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
	c.connector.observe(err, query)
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
	c.connector.observe(err, query)
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
	query     string
}

func (s *reconnectingStmt) Close() error  { return s.inner.Close() }
func (s *reconnectingStmt) NumInput() int { return s.inner.NumInput() }

func (s *reconnectingStmt) Exec(args []driver.Value) (driver.Result, error) {
	//lint:ignore SA1019 driver.Stmt interface compliance.
	result, err := s.inner.Exec(args) //nolint:staticcheck // driver.Stmt interface compliance.
	s.connector.observe(err, s.query)
	return result, err
}

func (s *reconnectingStmt) Query(args []driver.Value) (driver.Rows, error) {
	//lint:ignore SA1019 driver.Stmt interface compliance.
	rows, err := s.inner.Query(args) //nolint:staticcheck // driver.Stmt interface compliance.
	s.connector.observe(err, s.query)
	return rows, err
}

func (s *reconnectingStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	execer, ok := s.inner.(driver.StmtExecContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	result, err := execer.ExecContext(ctx, args)
	s.connector.observe(err, s.query)
	return result, err
}

func (s *reconnectingStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	queryer, ok := s.inner.(driver.StmtQueryContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	rows, err := queryer.QueryContext(ctx, args)
	s.connector.observe(err, s.query)
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
	t.connector.observe(err, "")
	return err
}

func (t *reconnectingTx) Rollback() error {
	err := t.inner.Rollback()
	t.connector.observe(err, "")
	return err
}
