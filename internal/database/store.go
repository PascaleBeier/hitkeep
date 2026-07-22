package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	duckdb "github.com/duckdb/duckdb-go/v2"
)

const (
	walAutoCheckpointSize                = "64MB"
	defaultMaintenanceCheckpointInterval = 5 * time.Minute
)

var duckDBCoreExtensions = [...]string{"httpfs", "aws", "excel"}

type Store struct {
	db                  *sql.DB
	path                string
	memoryLimit         string
	threads             int
	checkpointInterval  time.Duration
	checkpointMu        sync.Mutex
	migrationMu         sync.Mutex
	status              *databaseStatusTracker
	recovery            *databaseRecovery
	recoveryOptions     recoveryOptions
	fatalErrors         chan error
	fatalReporter       func(error)
	recoveredOnConnect  bool
	closeMu             sync.Mutex
	closed              bool
	analyticsMu         sync.Mutex
	aiBudgetMu          sync.Mutex
	primaryAuthMu       sync.Mutex
	reportClaimMu       sync.Mutex
	analyticsStatements *analyticsStatements
	runtime             *runtimeCache

	socialConfirmationMu sync.Mutex

	maintenanceMu     sync.Mutex
	maintenanceCancel context.CancelFunc

	activityPruneMu   sync.Mutex
	lastActivityPrune time.Time

	hitColumnsMu sync.Mutex
	hitColumns   []string
}

// StoreOption configures a Store before Connect.
type StoreOption func(*Store)

// WithMemoryLimit caps DuckDB's memory usage for this database (e.g. "512MB").
// An empty value keeps the DuckDB default of 80% of system RAM.
func WithMemoryLimit(limit string) StoreOption {
	return func(s *Store) {
		s.memoryLimit = strings.TrimSpace(limit)
	}
}

// WithThreads sets the number of DuckDB worker threads for this database.
// Zero keeps the DuckDB default.
func WithThreads(threads int) StoreOption {
	return func(s *Store) {
		s.threads = threads
	}
}

// WithCheckpointInterval configures periodic checkpoint maintenance. A zero
// interval disables the periodic loop while preserving required checkpoints.
func WithCheckpointInterval(interval time.Duration) StoreOption {
	return func(s *Store) {
		s.checkpointInterval = interval
	}
}

// WithAutomaticRecovery enables narrowly classified DuckDB recovery procedures
// and configures where their permission-restricted bundles are retained.
func WithAutomaticRecovery(enabled bool, recoveryPath string) StoreOption {
	return func(s *Store) {
		s.recoveryOptions.enabled = enabled
		s.recoveryOptions.root = strings.TrimSpace(recoveryPath)
	}
}

// WithAutomaticWALRecovery opts into discarding a narrowly recognized,
// replay-failing WAL after retaining an exact recovery bundle. It is disabled
// by default because WAL-only committed changes are excluded from the recovered
// live database.
func WithAutomaticWALRecovery(enabled bool) StoreOption {
	return func(s *Store) {
		s.recoveryOptions.automaticWALRecovery = enabled
	}
}

func withFatalReporter(reporter func(error)) StoreOption {
	return func(s *Store) {
		s.fatalReporter = reporter
	}
}

// memoryLimitPattern accepts DuckDB memory sizes such as "512MB" or "1.5 GiB".
var memoryLimitPattern = regexp.MustCompile(`(?i)^[0-9]+(\.[0-9]+)?\s*(B|KB|MB|GB|TB|KiB|MiB|GiB|TiB)?$`)

func NewStore(path string, opts ...StoreOption) *Store {
	store := &Store{
		path:               path,
		checkpointInterval: defaultMaintenanceCheckpointInterval,
		runtime:            newRuntimeCache(),
		fatalErrors:        make(chan error, 1),
	}
	for _, opt := range opts {
		opt(store)
	}
	store.status = newDatabaseStatusTracker(
		store.recoveryOptions.enabled,
		store.recoveryOptions.automaticWALRecovery,
		store.checkpointInterval,
	)
	store.recovery = newDatabaseRecovery(store, store.recoveryOptions, store.status)
	return store
}

// duckDBOptions returns the DuckDB tuning options this store was created
// with, so per-tenant stores can inherit them.
func (s *Store) duckDBOptions() []StoreOption {
	var opts []StoreOption
	if s.memoryLimit != "" {
		opts = append(opts, WithMemoryLimit(s.memoryLimit))
	}
	if s.threads != 0 {
		opts = append(opts, WithThreads(s.threads))
	}
	opts = append(opts,
		WithCheckpointInterval(s.checkpointInterval),
		WithAutomaticRecovery(s.recoveryOptions.enabled, s.recoveryOptions.root),
		WithAutomaticWALRecovery(s.recoveryOptions.automaticWALRecovery),
	)
	return opts
}

func (s *Store) Connect() error {
	s.recoveredOnConnect = false
	if s.memoryLimit != "" && !memoryLimitPattern.MatchString(s.memoryLimit) {
		return fmt.Errorf("invalid DuckDB memory limit %q: expected a size such as 512MB or 1.5GiB", s.memoryLimit)
	}
	if s.threads < 0 {
		return fmt.Errorf("invalid DuckDB threads %d: must be zero or positive", s.threads)
	}

	ctx := context.Background()
	s.recovery.loadRecoveryHistory()
	recoveryBeforeConnect := s.DatabaseStatus().LastRecoveryAt
	migrationGuard, err := s.recovery.loadMigrationGuard()
	if err != nil {
		return fmt.Errorf("load migration WAL guard: %w", err)
	}
	if err := s.recovery.recoverStartup(ctx); err != nil {
		return fmt.Errorf("resume database recovery: %w", err)
	}

	slog.Info("Connecting to database...", "path", s.path)
	db, err := s.openReconnectingDB(ctx)
	if err != nil && isKnownWALReplayError(err) && s.recovery.available() {
		var recoveryErr error
		if migrationGuard != nil {
			recoveryErr = s.recovery.recoverMigrationWAL(ctx, err, migrationGuard)
		} else {
			recoveryErr = s.recovery.recoverWAL(ctx, err)
		}
		if recoveryErr != nil {
			return fmt.Errorf("recover database WAL: %w", recoveryErr)
		}
		db, err = s.openReconnectingDB(ctx)
	}
	if err != nil {
		return fmt.Errorf("could not connect to database: %w", err)
	}
	if err := s.recovery.checkpointAndClearMigrationGuard(ctx, db, migrationGuard); err != nil {
		_ = db.Close()
		return fmt.Errorf("complete interrupted database migration: %w", err)
	}

	s.db = db
	s.closeMu.Lock()
	s.closed = false
	s.closeMu.Unlock()
	s.recoveredOnConnect = recoveryTimestampChanged(recoveryBeforeConnect, s.DatabaseStatus().LastRecoveryAt)
	if err := s.bootstrapCoreExtensions(); err != nil {
		slog.Warn("DuckDB core extension bootstrap incomplete; XLSX exports and S3-backed flows may fail", "error", err)
	}
	slog.Debug("Database connection established successfully.")
	return nil
}

func recoveryTimestampChanged(before, after *time.Time) bool {
	if after == nil {
		return false
	}
	return before == nil || !before.Equal(*after)
}

func (s *Store) openReconnectingDB(ctx context.Context) (*sql.DB, error) {
	connector := newReconnectingConnector(s.path, func() (driver.Connector, error) {
		return duckdb.NewConnector(s.path, s.initConnection)
	}, func(ctx context.Context, trigger error) error {
		err := s.recovery.recoverInvalidation(ctx, trigger)
		if err != nil {
			s.reportFatal(fmt.Errorf("automatic database recovery failed: %w", err))
		}
		return err
	})
	connector.configureDrainWatchdog(10*time.Second, s.reportDrainTimeout)
	connector.configureInvalidationObserver(func(trigger error) {
		recoveryTrigger := "fatal_invalidation"
		if isIndexMutationCorruption(trigger) {
			recoveryTrigger = "index_mutation_corruption"
		}
		s.status.recovering("drain_connections", recoveryTrigger)
	})
	db := sql.OpenDB(connector)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func (s *Store) reportDrainTimeout(err error) {
	if s == nil || err == nil {
		return
	}
	s.status.recoveryFailed("connection_drain_timeout", "drain_connections")
	s.reportFatal(err)
}

func (s *Store) reportFatal(err error) {
	if s == nil || err == nil {
		return
	}
	select {
	case s.fatalErrors <- err:
	default:
	}
	if s.fatalReporter != nil {
		s.fatalReporter(err)
	}
}

func (s *Store) openRawDB(ctx context.Context) (*sql.DB, error) {
	connector, err := duckdb.NewConnector(s.path, s.initConnection)
	if err != nil {
		return nil, err
	}
	db := sql.OpenDB(connector)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func (s *Store) initConnection(execer driver.ExecerContext) error {
	if _, err := execer.ExecContext(context.Background(), "SET TimeZone = 'UTC';", nil); err != nil {
		return fmt.Errorf("set database timezone: %w", err)
	}
	// Everything HitKeep needs is statically linked or installed explicitly
	// at bootstrap; implicit extension fetching would mean silent network
	// egress to the DuckDB extension repository at query time. Community
	// extensions are never used, so that repository is locked out entirely
	// (the setting is one-way until restart by design).
	if _, err := execer.ExecContext(context.Background(), "SET autoinstall_known_extensions=false; SET autoload_known_extensions=false; SET allow_community_extensions=false;", nil); err != nil {
		return fmt.Errorf("disable implicit extension fetching: %w", err)
	}
	// Insertion-order preservation buffers whole results and parallel insert
	// batches in memory; every user-visible ordering in HitKeep is an
	// explicit ORDER BY, so trade the implicit order for lower memory use.
	if _, err := execer.ExecContext(context.Background(), "SET preserve_insertion_order=false;", nil); err != nil {
		return fmt.Errorf("disable insertion order preservation: %w", err)
	}
	if _, err := execer.ExecContext(context.Background(), fmt.Sprintf("PRAGMA wal_autocheckpoint='%s';", walAutoCheckpointSize), nil); err != nil {
		slog.Warn("Failed to set wal_autocheckpoint", "size", walAutoCheckpointSize, "error", err)
	}
	if s.memoryLimit != "" {
		if _, err := execer.ExecContext(context.Background(), fmt.Sprintf("SET memory_limit='%s';", s.memoryLimit), nil); err != nil {
			return fmt.Errorf("set database memory limit %q: %w", s.memoryLimit, err)
		}
	}
	if s.threads > 0 {
		if _, err := execer.ExecContext(context.Background(), fmt.Sprintf("SET threads=%d;", s.threads), nil); err != nil {
			return fmt.Errorf("set database threads %d: %w", s.threads, err)
		}
	}
	for _, extension := range duckDBCoreExtensions {
		s.loadInstalledExtension(context.Background(), execer, extension)
	}
	return nil
}

func (s *Store) bootstrapCoreExtensions() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return s.WithDuckDBSession(ctx, DuckDBSessionOptions{}, func(conn *sql.Conn) error {
		for _, extension := range duckDBCoreExtensions {
			if err := EnsureCoreExtension(ctx, conn, extension); err != nil {
				return fmt.Errorf("bootstrap %s extension: %w", extension, err)
			}
		}
		return nil
	})
}

func (s *Store) loadInstalledExtension(ctx context.Context, execer driver.ExecerContext, name string) {
	query := fmt.Sprintf("LOAD %s;", name)
	if _, err := execer.ExecContext(ctx, query, nil); err != nil {
		slog.Debug("DuckDB core extension not yet available on new connection", "extension", name, "error", err)
	}
}

func (s *Store) StartMaintenance(ctx context.Context) {
	if s.db == nil {
		slog.Warn("Skipping database maintenance loop because database is not connected")
		return
	}
	if s.checkpointInterval <= 0 {
		return
	}

	s.maintenanceMu.Lock()
	defer s.maintenanceMu.Unlock()
	if s.maintenanceCancel != nil {
		return
	}
	maintenanceCtx, cancel := context.WithCancel(ctx)
	s.maintenanceCancel = cancel

	ticker := time.NewTicker(s.checkpointInterval)
	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-maintenanceCtx.Done():
				return
			case <-ticker.C:
				if err := s.Checkpoint(maintenanceCtx, "periodic"); err != nil && !errors.Is(err, context.Canceled) {
					slog.Error("Periodic database checkpoint failed", "error", err)
					retry := time.NewTimer(30 * time.Second)
					select {
					case <-maintenanceCtx.Done():
						if !retry.Stop() {
							<-retry.C
						}
						return
					case <-retry.C:
						if retryErr := s.Checkpoint(maintenanceCtx, "periodic_retry"); retryErr != nil && !errors.Is(retryErr, context.Canceled) {
							slog.Error("Periodic database checkpoint retry failed", "error", retryErr)
						}
					}
				}
			}
		}
	}()
}

// Checkpoint durably flushes the current database WAL. Checkpoints are
// serialized because DuckDB cannot run more than one checkpoint per database.
func (s *Store) Checkpoint(ctx context.Context, reason string) error {
	if s == nil || s.db == nil {
		return errors.New("database is not connected")
	}
	s.checkpointMu.Lock()
	defer s.checkpointMu.Unlock()

	if _, err := s.db.ExecContext(ctx, "CHECKPOINT;"); err != nil {
		s.status.checkpointFailed()
		return fmt.Errorf("checkpoint database (%s): %w", strings.TrimSpace(reason), err)
	}
	s.status.checkpointSucceeded(time.Now().UTC())
	slog.Debug("Database checkpoint completed", "reason", strings.TrimSpace(reason))
	return nil
}

func (s *Store) stopMaintenance() {
	s.maintenanceMu.Lock()
	defer s.maintenanceMu.Unlock()
	if s.maintenanceCancel != nil {
		s.maintenanceCancel()
		s.maintenanceCancel = nil
	}
}

func (s *Store) Close() error {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	if s.closed {
		return nil
	}

	slog.Debug("Closing database connection...")
	s.stopMaintenance()
	s.analyticsMu.Lock()
	if s.analyticsStatements != nil {
		_ = s.analyticsStatements.close()
		s.analyticsStatements = nil
	}
	s.analyticsMu.Unlock()
	if s.db != nil {
		db := s.db
		var checkpointErr error
		if s.DatabaseStatus().State == DatabaseStateHealthy {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			checkpointErr = s.Checkpoint(ctx, "shutdown")
			cancel()
		}
		closeErr := db.Close()
		s.closed = true
		return errors.Join(checkpointErr, closeErr)
	}
	s.closed = true
	return nil
}

func (s *Store) DB() *sql.DB {
	return s.db
}

// DatabaseStatus returns a sanitized snapshot for health and operator APIs.
func (s *Store) DatabaseStatus() DatabaseStatus {
	if s == nil {
		return DatabaseStatus{State: DatabaseStateFailed}
	}
	return s.status.snapshot()
}

// RecoveredDuringConnect reports whether this Store.Connect call completed an
// automatic recovery. It intentionally excludes recovery history loaded from
// retained bundles.
func (s *Store) RecoveredDuringConnect() bool {
	return s != nil && s.recoveredOnConnect
}

// FatalErrors reports database conditions that require a controlled process
// restart, such as a fatal instance that cannot release pinned connections.
func (s *Store) FatalErrors() <-chan error {
	if s == nil {
		return nil
	}
	return s.fatalErrors
}
