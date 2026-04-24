package database

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	tenant "hitkeep/internal/database/migrations/tenant"
)

// MigrateTenant applies tenant-scoped analytics migrations.
// These run on per-tenant DuckDB files (not the shared hitkeep.db).
func (s *Store) MigrateTenant(ctx context.Context) error {
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
		return nil
	}

	slog.Info("Applying pending tenant migrations...", "count", len(pendingMigrations), "path", s.path)

	for _, fileName := range pendingMigrations {
		slog.Info("Applying tenant migration", "file", fileName, "path", s.path)

		fileContent, err := tenant.Fs.ReadFile(fileName)
		if err != nil {
			return fmt.Errorf("could not read tenant migration file %s: %w", fileName, err)
		}

		if _, err := s.db.ExecContext(ctx, string(fileContent)); err != nil {
			return fmt.Errorf("failed to apply tenant migration %s: %w", fileName, err)
		}

		if err := s.addTenantAppliedMigration(ctx, fileName); err != nil {
			return fmt.Errorf("failed to record tenant migration %s: %w", fileName, err)
		}
	}

	slog.Info("Successfully applied all tenant migrations.", "path", s.path)
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

func (s *Store) addTenantAppliedMigration(ctx context.Context, fileName string) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO migrations (migration, applied_at) VALUES (?, ?)", fileName, time.Now())
	return err
}
