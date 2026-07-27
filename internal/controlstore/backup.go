package controlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
)

// BackupInfo describes a validated, compact SQLite control snapshot. The
// caller owns compression and local/S3 publication.
type BackupInfo struct {
	Path   string
	Bytes  int64
	SHA256 string
}

// Backup creates a transactionally consistent compact snapshot with SQLite's
// online VACUUM INTO primitive. It never overwrites destination.
func (s *Store) Backup(ctx context.Context, destination string) (BackupInfo, error) {
	if _, err := os.Stat(destination); err == nil {
		return BackupInfo{}, fmt.Errorf("control snapshot destination %q already exists", destination)
	} else if !errors.Is(err, os.ErrNotExist) {
		return BackupInfo{}, fmt.Errorf("inspect control snapshot destination: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(PASSIVE)"); err != nil {
		return BackupInfo{}, fmt.Errorf("checkpoint SQLite control database before backup: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, "VACUUM INTO ?", destination); err != nil {
		return BackupInfo{}, fmt.Errorf("create SQLite control snapshot: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(destination)
		}
	}()
	if err := os.Chmod(destination, 0o600); err != nil {
		return BackupInfo{}, fmt.Errorf("restrict SQLite control snapshot permissions: %w", err)
	}
	if err := validateReadOnlySnapshot(ctx, destination); err != nil {
		return BackupInfo{}, err
	}
	info, err := os.Stat(destination)
	if err != nil {
		return BackupInfo{}, err
	}
	digest, err := checksumFile(destination)
	if err != nil {
		return BackupInfo{}, err
	}
	cleanup = false
	return BackupInfo{Path: destination, Bytes: info.Size(), SHA256: digest}, nil
}

func validateReadOnlySnapshot(ctx context.Context, path string) error {
	values := url.Values{}
	values.Set("mode", "ro")
	values.Add("_pragma", "foreign_keys(ON)")
	db, err := sql.Open("sqlite", "file:"+url.PathEscape(path)+"?"+values.Encode())
	if err != nil {
		return err
	}
	defer db.Close()
	var quick string
	if err := db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&quick); err != nil {
		return fmt.Errorf("validate SQLite control snapshot: %w", err)
	}
	if quick != "ok" {
		return fmt.Errorf("SQLite control snapshot quick_check failed: %s", quick)
	}
	rows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return fmt.Errorf("validate SQLite control snapshot foreign keys: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		return errors.New("SQLite control snapshot contains a foreign-key violation")
	}
	return rows.Err()
}

// ValidateSnapshot verifies a restored SQLite control snapshot without
// migrating or otherwise modifying it.
func ValidateSnapshot(ctx context.Context, path string) error {
	format, err := InspectFormat(path)
	if err != nil {
		return err
	}
	if format != FileSQLite {
		return fmt.Errorf("control snapshot is %s, expected sqlite", format)
	}
	return validateReadOnlySnapshot(ctx, path)
}
