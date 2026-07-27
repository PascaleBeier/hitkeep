package controlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type sqlExecContext interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

type sqlQueryContext interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// queryRowOrNil executes a query expecting a single row.
// Returns nil error if no rows are found.
func (s *Store) queryRowOrNil(ctx context.Context, query string, dest []any, args ...any) error {
	err := s.db.QueryRowContext(ctx, query, args...).Scan(dest...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	return err
}

// queryList executes a query and iterates over rows, calling scanFn for each.
func (s *Store) queryList(ctx context.Context, query string, scanFn func(*sql.Rows) error, args ...any) error {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		if err := scanFn(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

// exec executes a statement and returns the error.
func (s *Store) exec(ctx context.Context, query string, args ...any) error {
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

// execRowsAffected executes a statement and returns an error if 0 rows were affected.
// Useful for DELETE/UPDATE where you expect something to exist.
func (s *Store) execRowsAffected(ctx context.Context, query string, args ...any) error {
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("no rows affected")
	}
	return nil
}

// transact handles the boilerplate of beginning a transaction, rolling it back on error/panic,
// and committing it on success.
func (s *Store) transact(ctx context.Context, fn func(*sql.Tx) error) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		} else if err != nil {
			_ = tx.Rollback()
		} else {
			err = tx.Commit()
		}
	}()

	return fn(tx)
}

func nullableStringPtr(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullStringValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func nullString(value sql.NullString) string { return nullStringValue(value) }
