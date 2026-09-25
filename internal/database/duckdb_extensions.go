package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"

	"hitkeep/internal/duckdbextensions"
)

type duckdbExtensionExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// coreExtensionQuery only loads the signed artifact shipped with this binary.
func coreExtensionQuery(name string) (string, error) {
	path, err := duckdbextensions.Path(name)
	if err != nil {
		return "", fmt.Errorf("prepare bundled %s extension: %w", name, err)
	}
	return "LOAD '" + escapeSQLString(path) + "';", nil
}

func initializeCoreExtensions(ctx context.Context, exec driver.ExecerContext) error {
	if _, err := exec.ExecContext(ctx, "SET autoinstall_known_extensions=false; SET autoload_known_extensions=false; SET allow_community_extensions=false;", nil); err != nil {
		return fmt.Errorf("disable implicit extension fetching: %w", err)
	}
	for _, name := range duckDBCoreExtensions {
		query, err := coreExtensionQuery(name)
		if err != nil {
			return err
		}
		if _, err := exec.ExecContext(ctx, query, nil); err != nil {
			return fmt.Errorf("load bundled %s extension: %w", name, err)
		}
	}
	return nil
}

// initializeSessionExtensions covers standalone backup connections as well as
// Store connections. Secrets remain scoped to the caller's pinned session.
func initializeSessionExtensions(ctx context.Context, conn *sql.Conn) error {
	return conn.Raw(func(raw any) error {
		exec, ok := raw.(driver.ExecerContext)
		if !ok {
			return fmt.Errorf("DuckDB connection does not support extension initialization")
		}
		return initializeCoreExtensions(ctx, exec)
	})
}
