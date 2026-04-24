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

func (s *Store) Migrate(ctx context.Context) error {
	migrationTableSQL, err := migrations.Fs.ReadFile("0000_00_00_000000_create_migrations_table.sql")
	if err != nil {
		return fmt.Errorf("could not read migrations table schema: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, string(migrationTableSQL)); err != nil {
		return fmt.Errorf("could not create migrations table: %w", err)
	}

	appliedMigrations, err := s.getAppliedMigrations(ctx)
	if err != nil {
		return err
	}

	availableMigrations, err := migrations.Fs.ReadDir(".")
	if err != nil {
		return fmt.Errorf("could not read migrations directory: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not begin migration transaction: %w", err)
	}

	// Ignore rollback error here as we only care if commit fails.
	// If commit succeeds, rollback returns ErrTxDone, which is fine.
	defer func() { _ = tx.Rollback() }()

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
		return nil
	}

	slog.Info("Applying pending database migrations...", "count", len(pendingMigrations))

	for _, fileName := range pendingMigrations {
		slog.Info("Applying migration", "file", fileName)

		fileContent, err := migrations.Fs.ReadFile(fileName)
		if err != nil {
			return fmt.Errorf("could not read migration file %s: %w", fileName, err)
		}

		if _, err := tx.ExecContext(ctx, string(fileContent)); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", fileName, err)
		}

		if err := s.addAppliedMigration(ctx, tx, fileName); err != nil {
			return fmt.Errorf("failed to record migration %s: %w", fileName, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("could not commit migration transaction: %w", err)
	}

	slog.Info("Successfully applied all database migrations.")
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
