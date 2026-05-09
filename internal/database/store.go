package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"log/slog"
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
	analyticsMu         sync.Mutex
	aiBudgetMu          sync.Mutex
	analyticsStatements *analyticsStatements
	runtime             *runtimeCache
}

func NewStore(path string) *Store {
	return &Store{
		path:    path,
		runtime: newRuntimeCache(),
	}
}

func (s *Store) Connect() error {
	slog.Info("Connecting to database...", "path", s.path)
	connector, err := duckdb.NewConnector(s.path, s.initConnection)
	if err != nil {
		return fmt.Errorf("could not create database connector: %w", err)
	}
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
	if _, err := execer.ExecContext(context.Background(), fmt.Sprintf("PRAGMA wal_autocheckpoint='%s';", walAutoCheckpointSize), nil); err != nil {
		slog.Warn("Failed to set wal_autocheckpoint", "size", walAutoCheckpointSize, "error", err)
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

	ticker := time.NewTicker(maintenanceCheckpointInterval)
	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				slog.Debug("Running database checkpoint...")
				if _, err := s.db.ExecContext(ctx, "CHECKPOINT;"); err != nil && !errors.Is(err, context.Canceled) {
					slog.Error("Checkpoint failed", "error", err)
				}
			}
		}
	}()
}

func (s *Store) Close() error {
	slog.Debug("Closing database connection...")
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
