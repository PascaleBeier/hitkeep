package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"hitkeep/internal/database/migrations"
)

type migrationRunOptions struct {
	guarded     bool
	afterCommit func()
}

// OpenMigratedStore opens a shared database, applies all pending migrations
// before the Store can be published to other goroutines, and closes it again
// if startup migration fails.
func OpenMigratedStore(ctx context.Context, path string, opts ...StoreOption) (*Store, error) {
	store := NewStore(path, opts...)
	if err := store.Connect(); err != nil {
		return nil, err
	}
	if err := store.migrate(ctx, migrationRunOptions{guarded: true}); err != nil {
		_ = store.Close()
		return nil, err
	}
	return store, nil
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

	if len(pendingMigrations) == 0 {
		slog.Info("Database schema is up to date. No migrations to apply.")
		return validateCleanupPlans(ctx, s.db, siteDeleteSpec, tenantPurgeSpec)
	}

	slog.Info("Applying pending database migrations...", "count", len(pendingMigrations))
	if opts.guarded {
		if err := s.prepareGuardedMigration(ctx, "shared", pendingMigrations); err != nil {
			return err
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return s.rollbackMigration(nil, fmt.Errorf("could not begin migration transaction: %w", err), opts.guarded)
	}

	for _, fileName := range pendingMigrations {
		slog.Info("Applying migration", "file", fileName)

		fileContent, err := migrations.Fs.ReadFile(fileName)
		if err != nil {
			return s.rollbackMigration(tx, fmt.Errorf("could not read migration file %s: %w", fileName, err), opts.guarded)
		}

		if _, err := tx.ExecContext(ctx, string(fileContent)); err != nil {
			return s.rollbackMigration(tx, fmt.Errorf("failed to apply migration %s: %w", fileName, err), opts.guarded)
		}

		if err := s.addAppliedMigration(ctx, tx, fileName); err != nil {
			return s.rollbackMigration(tx, fmt.Errorf("failed to record migration %s: %w", fileName, err), opts.guarded)
		}
	}

	if err := tx.Commit(); err != nil {
		// When guarded, the commit outcome is uncertain. Keep the guard so the
		// next startup can resolve either a replayed or unreplayable migration WAL.
		return fmt.Errorf("could not commit migration transaction: %w", err)
	}
	if opts.afterCommit != nil {
		opts.afterCommit()
	}

	slog.Info("Successfully applied all database migrations.")
	if opts.guarded {
		if err := s.completeGuardedMigration(ctx, "shared"); err != nil {
			return err
		}
	} else if err := s.Checkpoint(ctx, "shared_migrations"); err != nil {
		return fmt.Errorf("checkpoint shared migrations: %w", err)
	}
	if err := validateCleanupPlans(ctx, s.db, siteDeleteSpec, tenantPurgeSpec); err != nil {
		return err
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
