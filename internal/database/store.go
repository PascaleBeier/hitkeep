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
	walAutoCheckpointSize         = "64MB"
	maintenanceCheckpointInterval = 15 * time.Minute
)

type Store struct {
	db                  *sql.DB
	path                string
	memoryLimit         string
	threads             int
	analyticsMu         sync.Mutex
	aiBudgetMu          sync.Mutex
	analyticsStatements *analyticsStatements
	runtime             *runtimeCache

	maintenanceMu     sync.Mutex
	maintenanceCancel context.CancelFunc
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

// memoryLimitPattern accepts DuckDB memory sizes such as "512MB" or "1.5 GiB".
var memoryLimitPattern = regexp.MustCompile(`(?i)^[0-9]+(\.[0-9]+)?\s*(B|KB|MB|GB|TB|KiB|MiB|GiB|TiB)?$`)

func NewStore(path string, opts ...StoreOption) *Store {
	store := &Store{
		path:    path,
		runtime: newRuntimeCache(),
	}
	for _, opt := range opts {
		opt(store)
	}
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
	return opts
}

func (s *Store) Connect() error {
	if s.memoryLimit != "" && !memoryLimitPattern.MatchString(s.memoryLimit) {
		return fmt.Errorf("invalid DuckDB memory limit %q: expected a size such as 512MB or 1.5GiB", s.memoryLimit)
	}
	if s.threads < 0 {
		return fmt.Errorf("invalid DuckDB threads %d: must be zero or positive", s.threads)
	}

	slog.Info("Connecting to database...", "path", s.path)
	connector := newReconnectingConnector(s.path, func() (driver.Connector, error) {
		return duckdb.NewConnector(s.path, s.initConnection)
	})
	db := sql.OpenDB(connector)

	if err := db.PingContext(context.Background()); err != nil {
		_ = db.Close()
		return fmt.Errorf("could not connect to database: %w", err)
	}

	s.db = db
	if err := s.bootstrapCoreExtensions(); err != nil {
		slog.Warn("DuckDB core extension bootstrap incomplete; XLSX exports and S3-backed flows may fail", "error", err)
	}
	slog.Debug("Database connection established successfully.")
	return nil
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
	s.loadInstalledExtension(context.Background(), execer, "httpfs")
	s.loadInstalledExtension(context.Background(), execer, "excel")
	return nil
}

func (s *Store) bootstrapCoreExtensions() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return s.WithDuckDBSession(ctx, DuckDBSessionOptions{}, func(conn *sql.Conn) error {
		if err := EnsureCoreExtension(ctx, conn, "httpfs"); err != nil {
			return fmt.Errorf("bootstrap httpfs extension: %w", err)
		}
		if err := EnsureCoreExtension(ctx, conn, "excel"); err != nil {
			return fmt.Errorf("bootstrap excel extension: %w", err)
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

	s.maintenanceMu.Lock()
	defer s.maintenanceMu.Unlock()
	if s.maintenanceCancel != nil {
		return
	}
	maintenanceCtx, cancel := context.WithCancel(ctx)
	s.maintenanceCancel = cancel

	ticker := time.NewTicker(maintenanceCheckpointInterval)
	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-maintenanceCtx.Done():
				return
			case <-ticker.C:
				slog.Debug("Running database checkpoint...", "path", s.path)
				if _, err := s.db.ExecContext(maintenanceCtx, "CHECKPOINT;"); err != nil && !errors.Is(err, context.Canceled) {
					slog.Error("Checkpoint failed", "path", s.path, "error", err)
				}
			}
		}
	}()
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
	slog.Debug("Closing database connection...")
	s.stopMaintenance()
	s.analyticsMu.Lock()
	if s.analyticsStatements != nil {
		_ = s.analyticsStatements.close()
		s.analyticsStatements = nil
	}
	s.analyticsMu.Unlock()
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

func (s *Store) DB() *sql.DB {
	return s.db
}
