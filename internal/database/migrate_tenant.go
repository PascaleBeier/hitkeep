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
		if err := store.migrateTenant(ctx, migrationRunOptions{guarded: true, reopenHeavy: true, morePending: &morePending}); err != nil {
			_ = store.Close()
			return nil, err
		}
		if !morePending {
			store.recoveredOnConnect = recovered
			return store, nil
		}
		if err := store.closeForMigrationReopen(); err != nil {
			return nil, fmt.Errorf("reopen tenant database between migrations: %w", err)
		}
		preserveGuard = true
	}
}

func (s *Store) migrateTenant(ctx context.Context, opts migrationRunOptions) error {
	s.migrationMu.Lock()
	defer s.migrationMu.Unlock()
	cleanBaseForChecksumGuard := !fileExists(s.path + ".wal")

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
	if opts.morePending != nil {
		*opts.morePending = false
	}

	if len(pendingMigrations) == 0 {
		slog.Debug("Tenant database schema is up to date.")
		if opts.guarded {
			if guard, err := s.recovery.loadMigrationGuard(); err != nil {
				return err
			} else if guard != nil {
				if err := s.completeGuardedMigration(ctx, "tenant"); err != nil {
					return err
				}
			}
		}
		return s.afterTenantSchemaCurrent(ctx)
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
	slog.Info("Applying pending tenant migrations...", "count", len(pendingMigrations), "path", s.path)
	if opts.guarded {
		if err := s.prepareGuardedMigration(ctx, "tenant", guardPendingMigrations, cleanBaseForChecksumGuard); err != nil {
			return err
		}
	}
	s.db.SetMaxIdleConns(0)
	if s.catalog == "" {
		defer s.db.SetMaxIdleConns(2)
	}

	for _, fileName := range pendingMigrations {
		slog.Info("Applying tenant migration", "file", fileName, "path", s.path)

		fileContent, err := tenant.Fs.ReadFile(fileName)
		if err != nil {
			return fmt.Errorf("could not read tenant migration file %s: %w", fileName, err)
		}

		if err := s.runMigrationStep(ctx, fileName, string(fileContent), s.addTenantAppliedMigration); err != nil {
			return fmt.Errorf("tenant migration: %w", err)
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
		} else if err := s.Checkpoint(ctx, "tenant_migration"); err != nil {
			return fmt.Errorf("checkpoint tenant migration %s: %w", fileName, err)
		}
	}

	slog.Info("Successfully applied all tenant migrations.", "path", s.path)
	if morePending {
		return nil
	}
	if opts.guarded {
		if err := s.completeGuardedMigration(ctx, "tenant"); err != nil {
			return err
		}
	} else if err := s.Checkpoint(ctx, "tenant_migrations"); err != nil {
		return fmt.Errorf("checkpoint tenant migrations: %w", err)
	}
	return s.afterTenantSchemaCurrent(ctx)
}

// afterTenantSchemaCurrent runs the steps that must hold once a tenant schema is
// current, whether this process applied migrations or found none pending.
func (s *Store) afterTenantSchemaCurrent(ctx context.Context) error {
	if err := s.ensureAIClassificationMacros(ctx); err != nil {
		return err
	}
	return validateCleanupPlans(ctx, s.db, siteDeleteSpec)
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
