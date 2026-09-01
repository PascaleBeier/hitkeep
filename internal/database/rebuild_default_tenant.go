package database

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/hklog"
)

// RebuildDefaultTenantFile recreates the default tenant's database file after
// it was lost without a restorable backup, explicitly accepting the loss of
// its analytics history. It refuses to touch a tenant file that still opens
// and sets an unreadable leftover aside instead of deleting it. The rebuilt
// file carries the current tenant schema and the default tenant's site
// mirrors so ingest and archive imports work immediately. HitKeep must be
// stopped: DuckDB allows only one writer.
func RebuildDefaultTenantFile(ctx context.Context, sharedPath, dataPath string, opts ...StoreOption) (string, error) {
	ctx = hklog.WithLoggerIfAbsent(ctx, storeLoggerFromOptions(opts))
	if strings.TrimSpace(sharedPath) == "" || strings.TrimSpace(dataPath) == "" {
		return "", errors.New("default tenant rebuild requires shared and data paths")
	}
	shared, err := openSplitDatabase(sharedPath, opts...)
	if err != nil {
		return "", fmt.Errorf("open shared database for default tenant rebuild: %w", err)
	}
	defaultID, splitApplied, _, err := readDefaultTenantSplitState(ctx, shared)
	_ = shared.Close()
	if err != nil {
		return "", err
	}
	if defaultID == uuid.Nil {
		return "", errors.New("default tenant rebuild requires a default tenant")
	}
	if !splitApplied {
		return "", errors.New("the shared database has no default tenant split marker; analytics still live in the shared database and there is nothing to rebuild")
	}

	tenantPath := filepath.Join(dataPath, "tenants", defaultID.String(), "hitkeep.db")
	if err := setAsideUnreadableTenantFile(ctx, tenantPath, opts); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(tenantPath), 0o755); err != nil {
		return "", fmt.Errorf("create default tenant directory: %w", err)
	}

	store := NewStore(tenantPath, opts...)
	if err := store.Connect(); err != nil {
		return "", fmt.Errorf("create rebuilt default tenant database: %w", err)
	}
	if err := store.migrateTenant(ctx, migrationRunOptions{guarded: true}); err != nil {
		_ = store.Close()
		return "", fmt.Errorf("prepare rebuilt default tenant schema: %w", err)
	}
	if err := copyDefaultTenantSiteMirrors(ctx, store, sharedPath, defaultID); err != nil {
		_ = store.Close()
		return "", err
	}
	if _, err := store.DB().ExecContext(ctx, "CHECKPOINT;"); err != nil {
		_ = store.Close()
		return "", fmt.Errorf("checkpoint rebuilt default tenant database: %w", err)
	}
	if err := store.Close(); err != nil {
		return "", fmt.Errorf("close rebuilt default tenant database: %w", err)
	}
	if err := syncParentDirectory(tenantPath); err != nil {
		return "", fmt.Errorf("sync rebuilt default tenant database: %w", err)
	}
	hklog.LoggerFromContext(ctx).Warn("Rebuilt an empty default tenant database; its analytics history is not restored", "tenant_id", defaultID, "tenant_path", tenantPath)
	return tenantPath, nil
}

// setAsideUnreadableTenantFile keeps the rebuild from destroying data. A
// tenant file that still opens is refused outright; only a file DuckDB
// rejects as invalid is renamed aside together with its WAL. Any other open
// failure (for example a held lock from a running HitKeep) aborts.
func setAsideUnreadableTenantFile(ctx context.Context, tenantPath string, opts []StoreOption) error {
	if _, err := os.Stat(tenantPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stat default tenant file %s: %w", tenantPath, err)
	}
	probe := NewStore(tenantPath, opts...)
	err := probe.Connect()
	if err == nil {
		_ = probe.Close()
		return fmt.Errorf("default tenant file %s exists and opens; refusing to rebuild over it", tenantPath)
	}
	if !strings.Contains(err.Error(), "not a valid DuckDB database file") {
		return fmt.Errorf("open existing default tenant file %s: %w", tenantPath, err)
	}
	suffix := fmt.Sprintf(".invalid-%d", time.Now().UTC().UnixNano())
	for _, path := range []string{tenantPath, tenantPath + ".wal"} {
		if _, statErr := os.Stat(path); statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("stat default tenant artifact %s: %w", path, statErr)
		}
		if renameErr := os.Rename(path, path+suffix); renameErr != nil {
			return fmt.Errorf("set aside unreadable default tenant artifact %s: %w", path, renameErr)
		}
		hklog.LoggerFromContext(ctx).Warn("Set aside unreadable default tenant artifact before rebuild", "path", path+suffix)
	}
	if err := syncParentDirectory(tenantPath); err != nil {
		return fmt.Errorf("sync set-aside default tenant artifacts: %w", err)
	}
	return nil
}

// copyDefaultTenantSiteMirrors seeds the rebuilt file with the same site
// mirror rows the split would have published, scoped exactly like the split
// scope query.
func copyDefaultTenantSiteMirrors(ctx context.Context, store *Store, sharedPath string, defaultID uuid.UUID) error {
	if _, err := store.DB().ExecContext(ctx, fmt.Sprintf("ATTACH '%s' AS rebuild_control (READ_ONLY);", escapeSQLString(sharedPath))); err != nil {
		return fmt.Errorf("attach control database for default tenant rebuild: %w", err)
	}
	defer func() { _, _ = store.DB().ExecContext(context.Background(), "DETACH rebuild_control;") }()
	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO sites (id, domain, data_retention_days)
		SELECT s.id, s.domain, s.data_retention_days
		FROM rebuild_control.sites s
		LEFT JOIN rebuild_control.site_tenants st ON st.site_id = s.id
		WHERE st.tenant_id = ? OR st.site_id IS NULL
	`, defaultID); err != nil {
		return fmt.Errorf("copy default tenant site mirrors during rebuild: %w", err)
	}
	return nil
}
