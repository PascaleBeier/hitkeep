package database

import (
	"context"
	"database/sql"
	"fmt"
)

// WithPinnedConn runs fn on a single physical database/sql connection so
// connection-local DuckDB state stays consistent across related statements.
func WithPinnedConn(ctx context.Context, db *sql.DB, fn func(*sql.Conn) error) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire duckdb connection: %w", err)
	}
	defer conn.Close()

	return fn(conn)
}
