package controlstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	SQLiteWorkSuffix        = ".sqlite-work"
	PreSQLiteEvidenceSuffix = ".pre-sqlite-2.13.0"
)

// PublicationState is the non-mutating classification of a configured
// control path and its two conversion artifacts.
type PublicationState uint8

const (
	PublicationNeedsConversion PublicationState = iota
	PublicationSQLiteReady
	PublicationCanComplete
	PublicationNeedsRebuild
	PublicationNeedsWorkRebuild
)

var errInvalidSQLiteWork = errors.New("invalid SQLite conversion work file")

func SQLiteWorkPath(path string) string        { return path + SQLiteWorkSuffix }
func PreSQLiteEvidencePath(path string) string { return path + PreSQLiteEvidenceSuffix }

// InspectPublication reports whether startup should convert, open SQLite, or
// complete the final work-file rename. Ambiguous states are errors.
func InspectPublication(ctx context.Context, path string) (PublicationState, error) {
	finalFormat, err := InspectFormat(path)
	if err != nil {
		return 0, err
	}
	evidencePath := PreSQLiteEvidencePath(path)
	workPath := SQLiteWorkPath(path)
	evidenceExists, err := regularFileExists(evidencePath)
	if err != nil {
		return 0, err
	}
	workExists, err := regularFileExists(workPath)
	if err != nil {
		return 0, err
	}

	switch finalFormat {
	case FileMissing:
		if evidenceExists {
			if !workExists {
				return PublicationNeedsRebuild, nil
			}
			if err := validateWorkAgainstEvidence(ctx, workPath, evidencePath); err != nil {
				if errors.Is(err, errInvalidSQLiteWork) {
					return PublicationNeedsRebuild, nil
				}
				return 0, err
			}
			return PublicationCanComplete, nil
		}
		if workExists {
			return 0, fmt.Errorf("SQLite work file exists without a configured database or retained DuckDB evidence; refusing ambiguous publication")
		}
		return PublicationSQLiteReady, nil
	case FileSQLite:
		if workExists {
			return 0, fmt.Errorf("configured SQLite control database and conversion work file both exist; refusing ambiguous publication")
		}
		if evidenceExists {
			if err := validateWorkAgainstEvidence(ctx, path, evidencePath); err != nil {
				return 0, fmt.Errorf("published SQLite database does not match retained DuckDB evidence: %w", err)
			}
		}
		return PublicationSQLiteReady, nil
	case FileDuckDB:
		if evidenceExists {
			return 0, fmt.Errorf("configured DuckDB source and retained pre-SQLite evidence both exist; refusing to overwrite either file")
		}
		if workExists {
			if err := validateWorkAgainstEvidence(ctx, workPath, path); err != nil {
				if errors.Is(err, errInvalidSQLiteWork) {
					return PublicationNeedsWorkRebuild, nil
				}
				return 0, err
			}
		}
		return PublicationNeedsConversion, nil
	default:
		return 0, fmt.Errorf("configured control database has an unknown or malformed format; refusing to overwrite it")
	}
}

// ResetInvalidLegacyWork removes only the recognized SQLite conversion work
// artifact after InspectPublication has proven that the retained DuckDB
// evidence is authoritative and the work file is missing or malformed.
func ResetInvalidLegacyWork(ctx context.Context, path string) error {
	state, err := InspectPublication(ctx, path)
	if err != nil {
		return err
	}
	if state != PublicationNeedsRebuild && state != PublicationNeedsWorkRebuild {
		return fmt.Errorf("SQLite conversion work reset is not valid in publication state %d", state)
	}
	workPath := SQLiteWorkPath(path)
	for _, candidate := range []string{workPath, workPath + "-wal", workPath + "-shm"} {
		if err := os.Remove(candidate); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove invalid SQLite conversion artifact %s: %w", candidate, err)
		}
	}
	return syncDirectory(path)
}

// PublishLegacyConversion atomically retains the DuckDB source and publishes
// a verified SQLite work file at the configured path. It never overwrites the
// retained evidence path.
func PublishLegacyConversion(ctx context.Context, path string) error {
	state, err := InspectPublication(ctx, path)
	if err != nil {
		return err
	}
	workPath := SQLiteWorkPath(path)
	evidencePath := PreSQLiteEvidencePath(path)
	switch state {
	case PublicationSQLiteReady:
		format, _ := InspectFormat(path)
		if format == FileSQLite {
			return nil
		}
		return errors.New("no DuckDB conversion is ready to publish")
	case PublicationCanComplete:
		if err := os.Rename(workPath, path); err != nil {
			return fmt.Errorf("complete SQLite control publication: %w", err)
		}
		return syncDirectory(path)
	case PublicationNeedsConversion:
		if _, err := os.Stat(workPath); err != nil {
			return fmt.Errorf("verified SQLite work file is required before publication: %w", err)
		}
		if _, err := os.Stat(path + ".wal"); err == nil {
			return fmt.Errorf("legacy DuckDB control WAL still exists at %s; checkpoint and close the source before conversion", path+".wal")
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect legacy DuckDB WAL: %w", err)
		}
		if err := os.Rename(path, evidencePath); err != nil {
			return fmt.Errorf("retain pre-SQLite DuckDB control database: %w", err)
		}
		if err := os.Chmod(evidencePath, 0o600); err != nil {
			return fmt.Errorf("restrict retained DuckDB control evidence: %w", err)
		}
		if err := syncDirectory(path); err != nil {
			return fmt.Errorf("sync retained DuckDB control evidence: %w", err)
		}
		if err := os.Rename(workPath, path); err != nil {
			return fmt.Errorf("publish SQLite control database (retained source is %s): %w", evidencePath, err)
		}
		if err := syncDirectory(path); err != nil {
			return fmt.Errorf("sync SQLite control publication: %w", err)
		}
		return nil
	default:
		return errors.New("unsupported SQLite publication state")
	}
}

type importRecord struct {
	SourceSHA256       string
	SourceSchemaSHA256 string
}

func legacyImportRecord(ctx context.Context, path string) (importRecord, error) {
	store, err := Open(ctx, path)
	if err != nil {
		return importRecord{}, err
	}
	defer store.Close()
	var record importRecord
	if err := store.db.QueryRowContext(ctx, `
		SELECT source_sha256, source_schema_sha256
		FROM control_imports WHERE name = ?
	`, DuckDBControlImportV1).Scan(&record.SourceSHA256, &record.SourceSchemaSHA256); err != nil {
		return importRecord{}, fmt.Errorf("read DuckDB control import marker: %w", err)
	}
	if err := validateDigest("import source checksum", record.SourceSHA256); err != nil {
		return importRecord{}, err
	}
	if err := validateDigest("import source schema digest", record.SourceSchemaSHA256); err != nil {
		return importRecord{}, err
	}
	return record, nil
}

func validateWorkAgainstEvidence(ctx context.Context, workPath, evidencePath string) error {
	record, err := legacyImportRecord(ctx, workPath)
	if err != nil {
		return fmt.Errorf("%w: %v", errInvalidSQLiteWork, err)
	}
	evidenceSHA, err := checksumFile(evidencePath)
	if err != nil {
		return err
	}
	if !strings.EqualFold(record.SourceSHA256, evidenceSHA) {
		return fmt.Errorf("SQLite work-file source checksum %s conflicts with DuckDB evidence checksum %s; restore a matching conversion pair", record.SourceSHA256, evidenceSHA)
	}
	return nil
}

func regularFileExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("expected regular file at %s", path)
	}
	return true, nil
}

func checksumFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s for checksum: %w", path, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("checksum %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func syncDirectory(path string) error {
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
