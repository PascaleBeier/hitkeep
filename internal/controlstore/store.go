// Package controlstore owns HitKeep's pure-Go SQLite control plane.
//
// It intentionally has no dependency on internal/database or DuckDB. Tenant
// analytics remain in the DuckDB data plane and must be resolved separately.
package controlstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"hitkeep/internal/controlstore/migrations"
)

const (
	maxOpenConnections = 8
	busyTimeoutMillis  = 5000
	walSizeLimitBytes  = 64 << 20
)

var (
	sqliteHeader = []byte("SQLite format 3\x00")
	duckDBMagic  = []byte("DUCK")
)

// FileFormat is the storage engine detected at the configured control path.
type FileFormat uint8

const (
	FileMissing FileFormat = iota
	FileSQLite
	FileDuckDB
	FileUnknown
)

func (f FileFormat) String() string {
	switch f {
	case FileMissing:
		return "missing"
	case FileSQLite:
		return "sqlite"
	case FileDuckDB:
		return "duckdb"
	default:
		return "unknown"
	}
}

// InspectFormat reads only the database header and never modifies path.
func InspectFormat(path string) (FileFormat, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return FileMissing, nil
	}
	if err != nil {
		return FileUnknown, fmt.Errorf("open control database header: %w", err)
	}
	defer f.Close()

	header := make([]byte, len(sqliteHeader))
	n, err := io.ReadFull(f, header)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return FileUnknown, fmt.Errorf("read control database header: %w", err)
	}
	header = header[:n]
	if len(header) >= len(sqliteHeader) && string(header[:len(sqliteHeader)]) == string(sqliteHeader) {
		return FileSQLite, nil
	}
	if len(header) >= 12 && string(header[8:12]) == string(duckDBMagic) {
		return FileDuckDB, nil
	}
	return FileUnknown, nil
}

// Store is the bounded SQLite control-plane handle. The raw database pool is
// deliberately private so analytics code cannot accidentally cross planes.
type Store struct {
	db   *sql.DB
	path string

	aiBudgetMu           sync.Mutex
	primaryAuthMu        sync.Mutex
	reportClaimMu        sync.Mutex
	socialConfirmationMu sync.Mutex
	runtime              *runtimeCache

	closeOnce sync.Once
	closeErr  error

	maintenanceMu     sync.Mutex
	maintenanceCancel context.CancelFunc
}

// Open opens or creates a SQLite control database and applies the checksummed
// SQLite-only migration stream. DuckDB inputs are rejected for the offline
// legacy converter to handle.
func Open(ctx context.Context, path string) (*Store, error) {
	format, err := InspectFormat(path)
	if err != nil {
		return nil, err
	}
	if format == FileDuckDB {
		return nil, fmt.Errorf("control database %q is DuckDB: legacy conversion required", path)
	}
	if format == FileUnknown {
		return nil, fmt.Errorf("control database %q has an unknown or malformed format; refusing to overwrite it", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create control database directory: %w", err)
	}

	db, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		return nil, fmt.Errorf("open SQLite control database: %w", err)
	}
	db.SetMaxOpenConns(maxOpenConnections)
	db.SetMaxIdleConns(maxOpenConnections)
	db.SetConnMaxIdleTime(5 * time.Minute)

	store := &Store{db: db, path: path, runtime: newRuntimeCache()}
	if err := store.Ping(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := restrictSQLiteArtifacts(path); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.validate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := restrictSQLiteArtifacts(path); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func restrictSQLiteArtifacts(path string) error {
	for _, candidate := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Chmod(candidate, 0o600); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("restrict SQLite control artifact %s: %w", candidate, err)
		}
	}
	return nil
}

func sqliteDSN(path string) string {
	values := url.Values{}
	for _, pragma := range []string{
		fmt.Sprintf("busy_timeout(%d)", busyTimeoutMillis),
		"foreign_keys(ON)",
		"journal_mode(WAL)",
		"synchronous(FULL)",
		"temp_store(FILE)",
		"mmap_size(0)",
		fmt.Sprintf("journal_size_limit(%d)", walSizeLimitBytes),
		"cache_size(-8192)",
		"auto_vacuum(INCREMENTAL)",
	} {
		values.Add("_pragma", pragma)
	}
	values.Set("_txlock", "immediate")
	return "file:" + url.PathEscape(path) + "?" + values.Encode()
}

// Ping verifies that SQLite can serve a connection from the bounded pool.
func (s *Store) Ping(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errors.New("control database is not open")
	}
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping SQLite control database: %w", err)
	}
	return nil
}

// Path reports the configured local SQLite path without exposing the pool.
func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// Close checkpoints the WAL before closing the bounded pool.
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		s.stopMaintenance()
		if s.db == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
			s.closeErr = fmt.Errorf("checkpoint SQLite control database: %w", err)
		}
		if err := s.db.Close(); err != nil && s.closeErr == nil {
			s.closeErr = fmt.Errorf("close SQLite control database: %w", err)
		}
	})
	return s.closeErr
}

// StartMaintenance periodically performs a passive WAL checkpoint, planner
// optimization, and bounded incremental vacuuming. It is safe to call more
// than once; the first active loop wins.
func (s *Store) StartMaintenance(ctx context.Context, interval time.Duration) {
	if s == nil || s.db == nil || interval <= 0 {
		return
	}
	s.maintenanceMu.Lock()
	defer s.maintenanceMu.Unlock()
	if s.maintenanceCancel != nil {
		return
	}
	maintenanceCtx, cancel := context.WithCancel(ctx)
	s.maintenanceCancel = cancel
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-maintenanceCtx.Done():
				return
			case <-ticker.C:
				if err := s.Optimize(maintenanceCtx); err != nil && !errors.Is(err, context.Canceled) {
					slog.Error("SQLite control maintenance failed", "error", err)
					continue
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

func (s *Store) migrate(ctx context.Context) error {
	entries, err := fs.Glob(migrations.Files, "*.sql")
	if err != nil {
		return fmt.Errorf("list SQLite control migrations: %w", err)
	}
	sort.Strings(entries)
	for _, name := range entries {
		body, err := migrations.Files.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read SQLite control migration %s: %w", name, err)
		}
		checksum := sha256.Sum256(body)
		checksumHex := hex.EncodeToString(checksum[:])

		var existing string
		err = s.db.QueryRowContext(ctx,
			"SELECT checksum FROM control_migrations WHERE name = ?", name,
		).Scan(&existing)
		if err == nil {
			if existing != checksumHex {
				return fmt.Errorf("SQLite control migration %s checksum mismatch: database=%s binary=%s", name, existing, checksumHex)
			}
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) && !isMissingMigrationTable(err) {
			return fmt.Errorf("inspect SQLite control migration %s: %w", name, err)
		}

		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin SQLite control migration %s: %w", name, err)
		}
		if err := execMigration(ctx, tx, string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply SQLite control migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO control_migrations(name, checksum, applied_at) VALUES (?, ?, ?)",
			name, checksumHex, time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record SQLite control migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit SQLite control migration %s: %w", name, err)
		}
	}
	return nil
}

func execMigration(ctx context.Context, tx *sql.Tx, body string) error {
	// SQLite control migrations deliberately contain plain DDL only (no
	// triggers or semicolons inside literals), so statement-at-a-time execution
	// is deterministic across database/sql drivers.
	for _, statement := range strings.Split(body, ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func isMissingMigrationTable(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "no such table: control_migrations")
}

func (s *Store) validate(ctx context.Context) error {
	var quick string
	if err := s.db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&quick); err != nil {
		return fmt.Errorf("run SQLite quick_check: %w", err)
	}
	if quick != "ok" {
		return fmt.Errorf("SQLite quick_check failed: %s", quick)
	}

	rows, err := s.db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return fmt.Errorf("run SQLite foreign_key_check: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		return errors.New("SQLite foreign_key_check reported a violation")
	}
	return rows.Err()
}

// Optimize performs bounded control-plane maintenance. It is safe to call
// periodically; passive checkpoints never block active readers.
func (s *Store) Optimize(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(PASSIVE)"); err != nil {
		return fmt.Errorf("passive SQLite WAL checkpoint: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, "PRAGMA optimize"); err != nil {
		return fmt.Errorf("optimize SQLite control database: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, "PRAGMA incremental_vacuum"); err != nil {
		return fmt.Errorf("incremental vacuum SQLite control database: %w", err)
	}
	return nil
}
