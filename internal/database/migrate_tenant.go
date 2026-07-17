package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	tenant "hitkeep/internal/database/migrations/tenant"
)

// MigrateTenant applies tenant-scoped analytics migrations to an already
// connected Store. It does not authorize automatic WAL bypass because the
// caller may already have published the Store; production startup should use
// OpenMigratedTenantStore or the tenant manager's unpublished open path.
func (s *Store) MigrateTenant(ctx context.Context) error {
	return s.migrateTenant(ctx, migrationRunOptions{})
}

// OpenMigratedTenantStore opens a tenant database and applies all pending
// migrations before the Store can be published to other goroutines.
func OpenMigratedTenantStore(ctx context.Context, path string, opts ...StoreOption) (*Store, error) {
	store := NewStore(path, opts...)
	if err := store.Connect(); err != nil {
		return nil, err
	}
	if err := store.migrateTenant(ctx, migrationRunOptions{guarded: true}); err != nil {
		_ = store.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) migrateTenant(ctx context.Context, opts migrationRunOptions) error {
	s.migrationMu.Lock()
	defer s.migrationMu.Unlock()

	if err := s.ensureMigrationCheckpointTable(ctx); err != nil {
		return err
	}

	appliedMigrations, err := s.getTenantAppliedMigrations(ctx)
	if err != nil {
		return err
	}

	availableMigrations, err := tenant.Fs.ReadDir(".")
	if err != nil {
		return fmt.Errorf("could not read tenant migrations directory: %w", err)
	}

	pendingMigrations := []string{}
	for _, entry := range availableMigrations {
		fileName := entry.Name()
		if _, applied := appliedMigrations[fileName]; !applied && fileName != "embed.go" {
			pendingMigrations = append(pendingMigrations, fileName)
		}
	}

	sort.Strings(pendingMigrations)

	if len(pendingMigrations) == 0 {
		slog.Debug("Tenant database schema is up to date.")
		return validateCleanupPlans(ctx, s.db, siteDeleteSpec)
	}

	slog.Info("Applying pending tenant migrations...", "count", len(pendingMigrations), "path", s.path)
	if opts.guarded {
		if err := s.prepareGuardedMigration(ctx, "tenant", pendingMigrations); err != nil {
			return err
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return s.rollbackMigration(nil, fmt.Errorf("could not begin tenant migration transaction: %w", err), opts.guarded)
	}

	for _, fileName := range pendingMigrations {
		slog.Info("Applying tenant migration", "file", fileName, "path", s.path)

		fileContent, err := tenant.Fs.ReadFile(fileName)
		if err != nil {
			return s.rollbackMigration(tx, fmt.Errorf("could not read tenant migration file %s: %w", fileName, err), opts.guarded)
		}

		if _, err := tx.ExecContext(ctx, string(fileContent)); err != nil {
			return s.rollbackMigration(tx, fmt.Errorf("failed to apply tenant migration %s: %w", fileName, err), opts.guarded)
		}

		if err := s.addTenantAppliedMigration(ctx, tx, fileName); err != nil {
			return s.rollbackMigration(tx, fmt.Errorf("failed to record tenant migration %s: %w", fileName, err), opts.guarded)
		}
	}
	if err := tx.Commit(); err != nil {
		// When guarded, keep the guard because the commit outcome may be uncertain.
		return fmt.Errorf("could not commit tenant migration transaction: %w", err)
	}
	if opts.afterCommit != nil {
		opts.afterCommit()
	}

	slog.Info("Successfully applied all tenant migrations.", "path", s.path)
	if opts.guarded {
		if err := s.completeGuardedMigration(ctx, "tenant"); err != nil {
			return err
		}
	} else if err := s.Checkpoint(ctx, "tenant_migrations"); err != nil {
		return fmt.Errorf("checkpoint tenant migrations: %w", err)
	}
	if err := validateCleanupPlans(ctx, s.db, siteDeleteSpec); err != nil {
		return err
	}
	return nil
}

func (s *Store) getTenantAppliedMigrations(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT migration FROM migrations")
	if err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			return make(map[string]bool), nil
		}
		return nil, fmt.Errorf("could not query tenant migrations table: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var migration string
		if err := rows.Scan(&migration); err != nil {
			return nil, fmt.Errorf("could not scan tenant migration row: %w", err)
		}
		applied[migration] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("could not read tenant migration rows: %w", err)
	}

	return applied, nil
}

func (s *Store) addTenantAppliedMigration(ctx context.Context, tx *sql.Tx, fileName string) error {
	_, err := tx.ExecContext(ctx, "INSERT INTO migrations (migration, applied_at) VALUES (?, ?)", fileName, time.Now())
	return err
}
