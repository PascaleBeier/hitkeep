package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"hitkeep/hklog"
	"hitkeep/internal/database/migrations"
)

type migrationRunOptions struct {
	guarded     bool
	afterCommit func()
	reopenHeavy bool
	morePending *bool
}

// OpenMigratedStore opens a shared database, applies all pending migrations
// before the Store can be published to other goroutines, and closes it again
// if startup migration fails.
func OpenMigratedStore(ctx context.Context, path string, opts ...StoreOption) (*Store, error) {
	if err := recoverCompactionSwap(path); err != nil {
		return nil, err
	}
	recovered := false
	preserveGuard := false
	for {
		storeOpts := append([]StoreOption(nil), opts...)
		if preserveGuard {
			storeOpts = append(storeOpts, withPreservedMigrationGuardOnConnect())
		}
		store := NewStore(path, storeOpts...)
		if err := store.Connect(); err != nil {
			return nil, err
		}
		recovered = recovered || store.RecoveredDuringConnect()
		morePending := false
		if err := store.migrate(ctx, migrationRunOptions{guarded: true, reopenHeavy: true, morePending: &morePending}); err != nil {
			_ = store.Close()
			return nil, err
		}
		if !morePending {
			store.recoveredOnConnect = recovered
			return store, nil
		}
		if err := store.closeForMigrationReopen(); err != nil {
			return nil, fmt.Errorf("reopen database between migrations: %w", err)
		}
		preserveGuard = true
	}
}

// OpenDefaultSplitControlStore avoids applying the remaining analytics-table
// rebuilds in-place on a legacy indexed control file. The mandatory split
// rewrites that file out of place before the store can be published.
func OpenDefaultSplitControlStore(ctx context.Context, path string, opts ...StoreOption) (*Store, error) {
	store := NewStore(path, opts...)
	if err := store.Connect(); err != nil {
		return nil, err
	}
	if store.RecoveredDuringConnect() {
		store.skipCheckpointOnClose = true
		return store, nil
	}
	applied, err := store.getAppliedMigrations(ctx)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	splitApplied, err := store.HasDefaultTenantSplit(ctx)
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	legacyIndexedControl := applied["2026_07_08_000000_drop_analytics_art_indexes.sql"] &&
		!applied["2026_07_08_000001_drop_events_art_indexes.sql"] &&
		!splitApplied
	if legacyIndexedControl {
		store.skipCheckpointOnClose = true
		return store, nil
	}
	if err := store.closeForMigrationReopen(); err != nil {
		return nil, err
	}
	return OpenMigratedStore(ctx, path, opts...)
}

// Migrate applies shared migrations to an already connected Store. It does not
// authorize automatic WAL bypass because the caller may already have published
// the Store; production startup should use OpenMigratedStore.
func (s *Store) Migrate(ctx context.Context) error {
	return s.migrate(ctx, migrationRunOptions{})
}

func (s *Store) migrate(ctx context.Context, opts migrationRunOptions) error {
	s.migrationMu.Lock()
	defer s.migrationMu.Unlock()
	cleanBaseForChecksumGuard := !fileExists(s.path + ".wal")

	migrationTableSQL, err := migrations.Fs.ReadFile("0000_00_00_000000_create_migrations_table.sql")
	if err != nil {
		return fmt.Errorf("could not read migrations table schema: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, string(migrationTableSQL)); err != nil {
		return fmt.Errorf("could not create migrations table: %w", err)
	}
	if err := s.ensureMigrationCheckpointTable(ctx); err != nil {
		return err
	}

	appliedMigrations, err := s.getAppliedMigrations(ctx)
	if err != nil {
		return err
	}

	availableMigrations, err := migrations.Fs.ReadDir(".")
	if err != nil {
		return fmt.Errorf("could not read migrations directory: %w", err)
	}

	pendingMigrations := []string{}
	for _, entry := range availableMigrations {
		fileName := entry.Name()
		if _, applied := appliedMigrations[fileName]; !applied && fileName != "embed.go" && fileName != "0000_00_00_000000_create_migrations_table.sql" {
			pendingMigrations = append(pendingMigrations, fileName)
		}
	}

	sort.Strings(pendingMigrations)
	if opts.morePending != nil {
		*opts.morePending = false
	}

	if len(pendingMigrations) == 0 {
		hklog.LoggerFromContextOr(ctx, s.logger).Info("Database schema is up to date. No migrations to apply.")
		if opts.guarded {
			if guard, err := s.recovery.loadMigrationGuard(); err != nil {
				return err
			} else if guard != nil {
				if err := s.completeGuardedMigration(ctx, "shared"); err != nil {
					return err
				}
			}
		}
		return s.afterSchemaCurrent(ctx)
	}
	guardPendingMigrations := append([]string(nil), pendingMigrations...)

	batchEnd := len(pendingMigrations)
	if opts.reopenHeavy {
		for i, fileName := range pendingMigrations {
			if migrationNeedsNativeReopen(fileName) {
				batchEnd = i + 1
				break
			}
		}
	}
	morePending := batchEnd < len(pendingMigrations)
	if morePending {
		pendingMigrations = pendingMigrations[:batchEnd]
		if opts.morePending != nil {
			*opts.morePending = true
		}
	}
	hklog.LoggerFromContextOr(ctx, s.logger).Info("Applying pending database migrations...", "count", len(pendingMigrations))
	if opts.guarded {
		if err := s.prepareGuardedMigration(ctx, "shared", guardPendingMigrations, cleanBaseForChecksumGuard); err != nil {
			return err
		}
	}
	// Do not retain the native connection that performed a table rebuild. Its
	// transaction-local allocator can otherwise keep the old and rebuilt ART
	// state resident until after CHECKPOINT, pushing large upgrades over their
	// memory limit. The next statement opens a fresh connection to the same
	// unpublished database instance.
	s.db.SetMaxIdleConns(0)
	if s.catalog == "" {
		defer s.db.SetMaxIdleConns(2)
	}

	for _, fileName := range pendingMigrations {
		hklog.LoggerFromContextOr(ctx, s.logger).Info("Applying migration", "file", fileName)

		fileContent, err := migrations.Fs.ReadFile(fileName)
		if err != nil {
			return fmt.Errorf("could not read migration file %s: %w", fileName, err)
		}

		if err := s.runMigrationStep(ctx, fileName, string(fileContent), s.addAppliedMigration); err != nil {
			return err
		}
		if opts.afterCommit != nil {
			opts.afterCommit()
		}
		reopenNative := opts.reopenHeavy && fileName == pendingMigrations[len(pendingMigrations)-1] && migrationNeedsNativeReopen(fileName)
		if reopenNative {
			morePending = true
			if opts.morePending != nil {
				*opts.morePending = true
			}
		} else if err := s.checkpointMigrationWithTransientMargin(ctx, "shared_migration"); err != nil {
			return fmt.Errorf("checkpoint shared migration %s: %w", fileName, err)
		}
	}

	hklog.LoggerFromContextOr(ctx, s.logger).Info("Successfully applied all database migrations.")
	if morePending {
		// Keep the guard while OpenMigratedStore releases the native allocator and
		// continues the remaining migrations on a fresh instance.
		return nil
	}
	if opts.guarded {
		if err := s.completeGuardedMigration(ctx, "shared"); err != nil {
			return err
		}
	} else if err := s.Checkpoint(ctx, "shared_migrations"); err != nil {
		return fmt.Errorf("checkpoint shared migrations: %w", err)
	}
	return s.afterSchemaCurrent(ctx)
}

const migrationCheckpointHeadroom = int64(256 << 20)
const migrationCheckpointTransientMargin = int64(256 << 20)

// runMigrationStep leaves part of the configured DuckDB budget unused by a
// physical table rebuild, then restores the full configured limit before the
// mandatory checkpoint. DuckDB otherwise allows a rebuild to consume the last
// allocator block and cannot allocate even the checkpoint's small working
// buffer. The process still runs within the operator's configured limit.
func (s *Store) runMigrationStep(
	ctx context.Context,
	fileName string,
	statement string,
	record func(context.Context, *sql.Tx, string) error,
) (retErr error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("could not reserve migration connection for %s: %w", fileName, err)
	}
	reducedLimit := ""
	if strings.TrimSpace(statement) != "" && migrationNeedsNativeReopen(fileName) {
		reducedLimit = migrationMemoryLimitWithHeadroom(s.memoryLimit)
		if reducedLimit != "" {
			if _, err := conn.ExecContext(ctx, "SET memory_limit='"+reducedLimit+"'"); err != nil {
				_ = conn.Close()
				return fmt.Errorf("reserve checkpoint memory for migration %s: %w", fileName, err)
			}
		}
	}
	defer func() {
		if reducedLimit != "" {
			if _, err := conn.ExecContext(context.Background(), "SET memory_limit='"+s.memoryLimit+"'"); err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("restore memory limit after migration %s: %w", fileName, err))
			}
		}
		if err := conn.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("close migration connection for %s: %w", fileName, err))
		}
	}()

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not begin migration transaction for %s: %w", fileName, err)
	}
	if strings.TrimSpace(statement) != "" {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return rollbackMigrationStep(tx, fmt.Errorf("failed to apply migration %s: %w", fileName, err))
		}
	}
	if err := record(ctx, tx, fileName); err != nil {
		return rollbackMigrationStep(tx, fmt.Errorf("failed to record migration %s: %w", fileName, err))
	}
	if err := tx.Commit(); err != nil {
		// The commit outcome can be uncertain. The migration guard remains in
		// place so the next startup can safely resolve the WAL before resuming.
		return fmt.Errorf("could not commit migration %s: %w", fileName, err)
	}
	return nil
}

func migrationMemoryLimitWithHeadroom(configured string) string {
	total, ok := parseMemoryLimitBytes(configured)
	if !ok || total <= 16<<20 {
		return ""
	}
	reserve := migrationCheckpointHeadroom
	if fraction := total / 3; fraction < reserve {
		reserve = fraction
	}
	return fmt.Sprintf("%dB", total-reserve)
}

func migrationCheckpointMemoryLimit(configured string) string {
	total, ok := parseMemoryLimitBytes(configured)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%dB", total+migrationCheckpointTransientMargin)
}

func (s *Store) checkpointMigrationWithTransientMargin(ctx context.Context, reason string) error {
	limit := migrationCheckpointMemoryLimit(s.memoryLimit)
	if limit == "" {
		return s.Checkpoint(ctx, reason)
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserve migration checkpoint connection: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "SET memory_limit='"+limit+"'"); err != nil {
		_ = conn.Close()
		return fmt.Errorf("expand migration checkpoint memory: %w", err)
	}
	_, checkpointErr := conn.ExecContext(ctx, "CHECKPOINT;")
	_, restoreErr := conn.ExecContext(context.Background(), "SET memory_limit='"+s.memoryLimit+"'")
	closeErr := conn.Close()
	if checkpointErr != nil {
		s.status.checkpointFailed()
		return fmt.Errorf("checkpoint database (%s): %w", strings.TrimSpace(reason), checkpointErr)
	}
	if restoreErr != nil {
		return fmt.Errorf("restore memory limit after migration checkpoint: %w", restoreErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close migration checkpoint connection: %w", closeErr)
	}
	s.status.checkpointSucceeded(time.Now().UTC())
	return nil
}

func migrationNeedsNativeReopen(fileName string) bool {
	switch fileName {
	case "2026_07_08_000000_drop_analytics_art_indexes.sql",
		"2026_07_08_000001_drop_events_art_indexes.sql",
		"2026_07_08_000002_drop_web_vitals_art_indexes.sql",
		"2026_07_30_010000_drop_search_console_fact_art_indexes.sql",
		"0013_drop_analytics_art_indexes.sql",
		"0013a_drop_events_art_indexes.sql",
		"0013b_drop_web_vitals_art_indexes.sql",
		"0016_drop_search_console_fact_art_indexes.sql":
		return true
	default:
		return false
	}
}

// rollbackMigrationStep rolls back only the active migration. A guarded run
// deliberately keeps its durable guard until every pending migration has
// committed and checkpointed; reconnect clears or recovers that guard before
// the remaining migrations resume.
func rollbackMigrationStep(tx *sql.Tx, cause error) error {
	if tx == nil {
		return cause
	}
	if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
		return fmt.Errorf("%w; rollback migration step: %v", cause, err)
	}
	return cause
}

// afterSchemaCurrent runs the steps that must hold once the shared schema is
// current, whether this process applied migrations or found none pending.
func (s *Store) afterSchemaCurrent(ctx context.Context) error {
	if err := s.ensureAIClassificationMacros(ctx); err != nil {
		return err
	}
	if err := validateCleanupPlans(ctx, s.db, siteDeleteSpec, tenantPurgeSpec); err != nil {
		return err
	}
	if err := s.EnsureReportNextRuns(ctx, time.Now().UTC()); err != nil {
		return fmt.Errorf("initialize report schedules: %w", err)
	}
	return nil
}

func (s *Store) getAppliedMigrations(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT migration FROM migrations")
	if err != nil {
		if strings.Contains(err.Error(), "Table with name migrations does not exist") {
			return make(map[string]bool), nil
		}
		return nil, fmt.Errorf("could not query migrations table: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var migration string
		if err := rows.Scan(&migration); err != nil {
			return nil, fmt.Errorf("could not scan migration row: %w", err)
		}
		applied[migration] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("could not read migration rows: %w", err)
	}

	return applied, nil
}

func (s *Store) addAppliedMigration(ctx context.Context, tx *sql.Tx, fileName string) error {
	_, err := tx.ExecContext(ctx, "INSERT INTO migrations (migration, applied_at) VALUES (?, ?)", fileName, time.Now())
	return err
}
