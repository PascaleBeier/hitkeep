package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

const migrationWALGuardVersion = 1

const migrationCheckpointTable = "hitkeep_migration_checkpoints"

// migrationWALGuard records the migrations prepared before an unpublished
// startup store runs them. Recovery only treats the resulting WAL as a
// migration WAL after the WAL-less database proves it contains the matching
// checkpoint token, so unknown application writes are never discarded.
type migrationWALGuard struct {
	Version           int       `json:"version"`
	DatabaseID        string    `json:"database_id"`
	Scope             string    `json:"scope"`
	PendingMigrations []string  `json:"pending_migrations"`
	CheckpointToken   string    `json:"checkpoint_token"`
	DatabaseSHA256    string    `json:"database_sha256,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

func (r *databaseRecovery) migrationGuardPath() string {
	return r.markerPath() + ".migration"
}

func (r *databaseRecovery) writeMigrationGuard(scope string, pending []string, checkpointToken, databaseSHA256 string) error {
	if !r.available() {
		return nil
	}
	scope = strings.TrimSpace(scope)
	checkpointToken = strings.TrimSpace(checkpointToken)
	if scope == "" || len(pending) == 0 || (checkpointToken == "" && databaseSHA256 == "") {
		return errors.New("migration WAL guard requires a scope, pending migrations, and a checkpoint token or database checksum")
	}
	if _, err := r.loadMigrationGuard(); err != nil {
		return err
	} else if fileExists(r.migrationGuardPath()) {
		return errors.New("migration WAL guard already exists")
	}
	if err := os.MkdirAll(r.root(), 0o700); err != nil {
		return fmt.Errorf("create migration WAL guard directory: %w", err)
	}
	if err := os.Chmod(r.root(), 0o700); err != nil {
		return fmt.Errorf("secure migration WAL guard directory: %w", err)
	}
	guard := migrationWALGuard{
		Version:           migrationWALGuardVersion,
		DatabaseID:        r.databaseID(),
		Scope:             scope,
		PendingMigrations: append([]string(nil), pending...),
		CheckpointToken:   checkpointToken,
		DatabaseSHA256:    databaseSHA256,
		CreatedAt:         time.Now().UTC(),
	}
	if err := writeJSONFile(r.migrationGuardPath(), guard); err != nil {
		return fmt.Errorf("write migration WAL guard: %w", err)
	}
	return nil
}

func (r *databaseRecovery) loadMigrationGuard() (*migrationWALGuard, error) {
	if !r.available() {
		return nil, nil
	}
	data, err := os.ReadFile(r.migrationGuardPath())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read migration WAL guard: %w", err)
	}
	var guard migrationWALGuard
	if err := json.Unmarshal(data, &guard); err != nil {
		return nil, fmt.Errorf("decode migration WAL guard: %w", err)
	}
	if guard.Version != migrationWALGuardVersion || guard.DatabaseID != r.databaseID() {
		return nil, errors.New("migration WAL guard does not match this database")
	}
	if strings.TrimSpace(guard.Scope) == "" || len(guard.PendingMigrations) == 0 ||
		(strings.TrimSpace(guard.CheckpointToken) == "" && strings.TrimSpace(guard.DatabaseSHA256) == "") {
		return nil, errors.New("migration WAL guard is incomplete")
	}
	return &guard, nil
}

func (r *databaseRecovery) clearMigrationGuard() error {
	if !r.available() {
		return nil
	}
	if err := os.Remove(r.migrationGuardPath()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("clear migration WAL guard: %w", err)
	}
	if err := syncParentDirectory(r.migrationGuardPath()); err != nil {
		return fmt.Errorf("sync cleared migration WAL guard: %w", err)
	}
	return nil
}

func (r *databaseRecovery) checkpointMigrationGuard(ctx context.Context, db *sql.DB, guard *migrationWALGuard, clear bool) error {
	if guard == nil {
		return nil
	}
	if db == nil {
		return errors.New("cannot complete migration WAL guard without a database")
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserve guarded migration checkpoint connection: %w", err)
	}
	expandedLimit := migrationCheckpointMemoryLimit(r.store.memoryLimit)
	if expandedLimit != "" {
		if _, err := conn.ExecContext(ctx, "SET memory_limit='"+expandedLimit+"'"); err != nil {
			_ = conn.Close()
			return fmt.Errorf("expand guarded migration checkpoint memory: %w", err)
		}
	}
	_, checkpointErr := conn.ExecContext(ctx, "CHECKPOINT;")
	var restoreErr error
	if expandedLimit != "" {
		_, restoreErr = conn.ExecContext(context.Background(), "SET memory_limit='"+r.store.memoryLimit+"'")
	}
	closeErr := conn.Close()
	if checkpointErr != nil {
		return fmt.Errorf("checkpoint replayed migration WAL: %w", checkpointErr)
	}
	if restoreErr != nil {
		return fmt.Errorf("restore memory limit after guarded migration checkpoint: %w", restoreErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close guarded migration checkpoint connection: %w", closeErr)
	}
	if clear {
		if err := r.clearMigrationGuard(); err != nil {
			return err
		}
	}
	r.status.checkpointSucceeded(time.Now().UTC())
	message := "Checkpointed guarded DuckDB migration section"
	if clear {
		message = "Completed interrupted DuckDB migration checkpoint"
	}
	slog.Info(message,
		"database_id", guard.DatabaseID,
		"scope", guard.Scope,
		"pending_migration_count", len(guard.PendingMigrations))
	return nil
}

func (s *Store) ensureMigrationCheckpointTable(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS hitkeep_migration_checkpoints (
			checkpoint_token VARCHAR NOT NULL
		)`); err != nil {
		return fmt.Errorf("create migration checkpoint table: %w", err)
	}
	return nil
}

func (s *Store) prepareGuardedMigration(ctx context.Context, scope string, pending []string, cleanBase ...bool) error {
	if existing, err := s.recovery.loadMigrationGuard(); err != nil {
		return err
	} else if existing != nil {
		if existing.Scope != strings.TrimSpace(scope) {
			return fmt.Errorf("existing migration WAL guard has scope %q, want %q", existing.Scope, scope)
		}
		// The unpublished runner checkpoints every migration before reopening
		// the native instance. Retain the original guard until the complete
		// pending set succeeds instead of replacing it between heavy sections.
		return nil
	}
	// A clean file-backed database can use its exact pre-migration checksum as
	// the WAL-bypass proof. This avoids loading a large legacy ART index merely
	// to checkpoint a token before the migration that removes that index.
	checksumGuardAllowed := !fileExists(s.path + ".wal")
	if len(cleanBase) > 0 {
		checksumGuardAllowed = cleanBase[0]
	}
	if s.recovery.available() && checksumGuardAllowed {
		databaseSHA256, err := databaseFileSHA256(s.path)
		if err != nil {
			return fmt.Errorf("checksum database before %s migrations: %w", scope, err)
		}
		if err := s.recovery.writeMigrationGuard(scope, pending, "", databaseSHA256); err != nil {
			return fmt.Errorf("write %s migration checksum guard: %w", scope, err)
		}
		return nil
	}
	checkpointToken := uuid.NewString()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin %s migration checkpoint token: %w", scope, err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM "+migrationCheckpointTable); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("clear %s migration checkpoint token: %w", scope, err)
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO "+migrationCheckpointTable+" (checkpoint_token) VALUES (?)",
		checkpointToken,
	); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("write %s migration checkpoint token: %w", scope, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit %s migration checkpoint token: %w", scope, err)
	}
	if err := s.Checkpoint(ctx, "before_"+scope+"_migrations"); err != nil {
		return fmt.Errorf("checkpoint before %s migrations: %w", scope, err)
	}
	if err := s.recovery.writeMigrationGuard(scope, pending, checkpointToken, ""); err != nil {
		return fmt.Errorf("write %s migration WAL guard: %w", scope, err)
	}
	return nil
}

func (r *databaseRecovery) verifyMigrationCheckpoint(ctx context.Context, db *sql.DB, expectedToken string) error {
	expectedToken = strings.TrimSpace(expectedToken)
	if expectedToken == "" {
		return errors.New("migration recovery marker has no checkpoint token")
	}
	var count int
	var actualToken string
	if err := db.QueryRowContext(ctx, `
		SELECT count(*), COALESCE(max(checkpoint_token), '')
		FROM hitkeep_migration_checkpoints`).Scan(&count, &actualToken); err != nil {
		return fmt.Errorf("read migration checkpoint token: %w", err)
	}
	if count != 1 || actualToken != expectedToken {
		return fmt.Errorf("migration checkpoint token mismatch")
	}
	return nil
}

func (s *Store) completeGuardedMigration(ctx context.Context, scope string) error {
	if err := s.Checkpoint(ctx, scope+"_migrations"); err != nil {
		return fmt.Errorf("checkpoint %s migrations: %w", scope, err)
	}
	if err := s.recovery.clearMigrationGuard(); err != nil {
		return fmt.Errorf("clear %s migration WAL guard: %w", scope, err)
	}
	return nil
}
