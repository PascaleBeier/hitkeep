package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"

	duckdb "github.com/duckdb/duckdb-go/v2"
)

type rowAppender interface {
	AppendRow(args ...driver.Value) error
}

func (s *Store) withAppender(ctx context.Context, table string, fn func(rowAppender) error) error {
	return s.withAppenderColumns(ctx, table, nil, fn)
}

func (s *Store) withAppenderColumns(ctx context.Context, table string, columns []string, fn func(rowAppender) error) error {
	return s.WithDuckDBSession(ctx, DuckDBSessionOptions{}, func(conn *sql.Conn) error {
		return conn.Raw(func(driverConn any) error {
			if unwrapper, ok := driverConn.(duckdbConnUnwrapper); ok {
				driverConn = unwrapper.UnwrapDuckDBConn()
			}
			rawConn, ok := driverConn.(driver.Conn)
			if !ok {
				return fmt.Errorf("unexpected duckdb driver connection type %T", driverConn)
			}

			var (
				appender *duckdb.Appender
				err      error
			)
			if len(columns) == 0 {
				appender, err = duckdb.NewAppenderFromConn(rawConn, "", table)
			} else {
				appender, err = duckdb.NewAppenderWithColumns(rawConn, "", "", table, columns)
			}
			if err != nil {
				return fmt.Errorf("create appender for %s: %w", table, err)
			}

			if err := fn(appender); err != nil {
				_ = appender.Close()
				return err
			}

			if err := appender.Close(); err != nil {
				return fmt.Errorf("close appender for %s: %w", table, err)
			}

			return nil
		})
	})
}

func nullableStringPtr(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableIntPtr(value *int) any {
	if value == nil {
		return nil
	}
	return int64(*value)
}

func nullableBoolPtr(value *bool) any {
	if value == nil {
		return nil
	}
	return *value
}
