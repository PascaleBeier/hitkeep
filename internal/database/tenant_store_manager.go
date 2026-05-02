package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

// TenantStoreManager provides per-tenant database isolation.
//
// The default tenant's analytics data stays in the shared hitkeep.db
// (backwards compatible for self-hosters). Non-default tenants get their
// own DuckDB file at {basePath}/tenants/{tenantID}/hitkeep.db.
type TenantStoreManager struct {
	shared   *Store
	basePath string

	mu     sync.RWMutex
	stores map[uuid.UUID]*Store

	defaultID uuid.UUID
}

var siteAnalyticsTransferTables = []string{
	"imported_event_properties_daily",
	"imported_event_dimensions_daily",
	"imported_event_daily",
	"imported_dimension_daily",
	"imported_traffic_daily",
	"goal_rollups_hourly",
	"goal_rollups_daily",
	"goal_rollups_monthly",
	"funnel_rollups_hourly",
	"funnel_rollups_daily",
	"funnel_rollups_monthly",
	"session_rollups_hourly",
	"session_rollups_daily",
	"session_rollups_monthly",
	"hit_rollups_hourly",
	"hit_rollups_daily",
	"hit_rollups_monthly",
	"goals",
	"funnels",
	"events",
	"hits",
}

// NewTenantStoreManager creates a TenantStoreManager that wraps the shared store.
// It resolves and caches the default tenant ID from the shared database.
func NewTenantStoreManager(shared *Store, basePath string) *TenantStoreManager {
	mgr := &TenantStoreManager{
		shared:   shared,
		basePath: basePath,
		stores:   make(map[uuid.UUID]*Store),
	}

	// Best-effort default tenant ID resolution. If the tenant table doesn't
	// exist yet (pre-migration) we'll resolve lazily.
	defaultID, err := shared.GetDefaultTenantID(context.Background())
	if err != nil {
		slog.Debug("TenantStoreManager: could not resolve default tenant ID at init (will resolve lazily)", "error", err)
	} else {
		mgr.defaultID = defaultID
	}

	return mgr
}

// Shared returns the main shared store (identity tables, default tenant data).
func (m *TenantStoreManager) Shared() *Store {
	return m.shared
}

// DefaultTenantID returns the cached default tenant ID, resolving lazily if needed.
func (m *TenantStoreManager) DefaultTenantID(ctx context.Context) (uuid.UUID, error) {
	if m.defaultID != uuid.Nil {
		return m.defaultID, nil
	}

	defaultID, err := m.shared.GetDefaultTenantID(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	m.defaultID = defaultID
	return defaultID, nil
}

// ForTenant returns the Store for the given tenant.
//
// For the default tenant, it returns the shared store directly (no separate DB).
// For non-default tenants, it lazily opens and migrates a per-tenant DuckDB file.
func (m *TenantStoreManager) ForTenant(ctx context.Context, tenantID uuid.UUID) (*Store, error) {
	if tenantID == uuid.Nil {
		return m.shared, nil
	}

	defaultID, err := m.DefaultTenantID(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not resolve default tenant: %w", err)
	}
	if tenantID == defaultID {
		return m.shared, nil
	}

	// Fast path: check cache with read lock.
	m.mu.RLock()
	if store, ok := m.stores[tenantID]; ok {
		m.mu.RUnlock()
		return store, nil
	}
	m.mu.RUnlock()

	// Slow path: create with write lock.
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock.
	if store, ok := m.stores[tenantID]; ok {
		return store, nil
	}

	store, err := m.openTenantStore(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	m.stores[tenantID] = store
	return store, nil
}

// ResolveTenantStore resolves the active tenant for a user and returns
// the tenant's store along with the tenant ID.
func (m *TenantStoreManager) ResolveTenantStore(ctx context.Context, userID uuid.UUID) (*Store, uuid.UUID, error) {
	tenantID, err := m.shared.GetActiveTenantID(ctx, userID)
	if err != nil {
		return nil, uuid.Nil, fmt.Errorf("could not resolve active tenant for user %s: %w", userID, err)
	}

	store, err := m.ForTenant(ctx, tenantID)
	if err != nil {
		return nil, uuid.Nil, err
	}
	return store, tenantID, nil
}

// ResolveSiteStore resolves the tenant for a site, ensures the tenant-local
// mirror/config bridge is in place, and returns the analytics store.
func (m *TenantStoreManager) ResolveSiteStore(ctx context.Context, siteID uuid.UUID) (*Store, uuid.UUID, error) {
	tenantID, err := m.shared.GetSiteTenantID(ctx, siteID)
	if err != nil {
		return nil, uuid.Nil, fmt.Errorf("resolve tenant for site %s: %w", siteID, err)
	}

	store, err := m.ForTenant(ctx, tenantID)
	if err != nil {
		return nil, uuid.Nil, err
	}

	if err := m.SyncSite(ctx, siteID); err != nil {
		return nil, uuid.Nil, err
	}

	return store, tenantID, nil
}

// SyncSite mirrors the site's metadata into the tenant-local store and
// backfills legacy shared goals/funnels for bridge-release compatibility.
func (m *TenantStoreManager) SyncSite(ctx context.Context, siteID uuid.UUID) error {
	tenantID, err := m.shared.GetSiteTenantID(ctx, siteID)
	if err != nil {
		return fmt.Errorf("resolve tenant for site %s: %w", siteID, err)
	}
	return m.syncSiteTenantData(ctx, siteID, tenantID)
}

// SyncAllTenants eagerly syncs all known sites into their tenant-local stores.
func (m *TenantStoreManager) SyncAllTenants(ctx context.Context) error {
	sites, err := m.shared.ListAllSites(ctx)
	if err != nil {
		return fmt.Errorf("list sites for tenant sync: %w", err)
	}
	for _, site := range sites {
		if err := m.SyncSite(ctx, site.ID); err != nil {
			return err
		}
	}
	return nil
}

// DeleteSite removes tenant-local analytics data first, then deletes the
// shared control-plane records.
func (m *TenantStoreManager) DeleteSite(ctx context.Context, siteID uuid.UUID) error {
	analyticsStore, _, err := m.ResolveSiteStore(ctx, siteID)
	if err != nil {
		return err
	}

	if analyticsStore != m.shared {
		if err := analyticsStore.DeleteSite(ctx, siteID); err != nil {
			return fmt.Errorf("delete tenant analytics site %s: %w", siteID, err)
		}
	}

	if err := m.shared.DeleteSite(ctx, siteID); err != nil {
		return fmt.Errorf("delete shared site %s: %w", siteID, err)
	}
	return nil
}

// PurgeArchivedTenant removes the per-tenant analytics database directory and
// deletes archived control-plane records for a non-default tenant.
func (m *TenantStoreManager) PurgeArchivedTenant(ctx context.Context, tenantID uuid.UUID) (*api.Team, error) {
	team, err := m.shared.GetPurgeableTenant(ctx, tenantID)
	if err != nil || team == nil {
		return team, err
	}

	if err := m.closeTenantStore(tenantID); err != nil {
		return nil, err
	}

	if err := os.RemoveAll(m.tenantDataDir(tenantID)); err != nil {
		return nil, fmt.Errorf("remove tenant data directory: %w", err)
	}

	deleted, err := m.shared.DeleteArchivedTenantMetadata(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if deleted != nil {
		slog.Info("Purged archived tenant", "tenant_id", tenantID, "name", deleted.Name)
	}

	return deleted, nil
}

// TransferSite copies site-scoped analytics into the destination tenant store,
// updates the shared site->tenant mapping, and removes stale analytics from the
// previous tenant's data plane.
func (m *TenantStoreManager) TransferSite(ctx context.Context, siteID, destinationTenantID uuid.UUID) error {
	sourceTenantID, err := m.shared.GetSiteTenantID(ctx, siteID)
	if err != nil {
		return fmt.Errorf("resolve source tenant for site %s: %w", siteID, err)
	}
	if sourceTenantID == destinationTenantID {
		return nil
	}

	site, err := m.shared.GetSiteByID(ctx, siteID)
	if err != nil {
		return fmt.Errorf("load site %s for transfer: %w", siteID, err)
	}
	if site == nil {
		return fmt.Errorf("site %s not found", siteID)
	}

	sourceStore, err := m.ForTenant(ctx, sourceTenantID)
	if err != nil {
		return err
	}
	destinationStore, err := m.ForTenant(ctx, destinationTenantID)
	if err != nil {
		return err
	}

	if sourceStore != destinationStore {
		if err := copySiteAnalyticsBetweenStores(ctx, sourceStore, destinationStore, siteID); err != nil {
			return err
		}
		if err := deleteSiteAnalyticsOnly(ctx, sourceStore, siteID, sourceStore != m.shared); err != nil {
			return err
		}
	}

	if err := m.shared.UpdateSiteTenant(ctx, siteID, destinationTenantID); err != nil {
		return fmt.Errorf("update shared site tenant mapping: %w", err)
	}

	defaultID, err := m.DefaultTenantID(ctx)
	if err != nil {
		return fmt.Errorf("resolve default tenant after site transfer: %w", err)
	}
	if destinationTenantID != defaultID {
		if err := destinationStore.UpsertSiteMirror(ctx, site); err != nil {
			return fmt.Errorf("upsert destination site mirror: %w", err)
		}
	}
	if err := m.SyncSite(ctx, siteID); err != nil {
		return err
	}

	return nil
}

// Close closes all per-tenant stores. The caller is responsible for
// closing the shared store separately.
func (m *TenantStoreManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var firstErr error
	for id, store := range m.stores {
		if err := store.Close(); err != nil {
			slog.Error("Failed to close tenant store", "tenant_id", id, "error", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	m.stores = make(map[uuid.UUID]*Store)
	return firstErr
}

// openTenantStore creates the directory structure and opens a new DuckDB
// connection for a non-default tenant. Must be called with m.mu held.
func (m *TenantStoreManager) openTenantStore(ctx context.Context, tenantID uuid.UUID) (*Store, error) {
	dir := m.tenantDataDir(tenantID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("could not create tenant data directory %s: %w", dir, err)
	}

	dbPath := filepath.Join(dir, "hitkeep.db")
	store := NewStore(dbPath)
	if err := store.Connect(); err != nil {
		return nil, fmt.Errorf("could not connect to tenant database %s: %w", dbPath, err)
	}

	if err := store.MigrateTenant(ctx); err != nil {
		store.Close()
		return nil, fmt.Errorf("could not migrate tenant database %s: %w", dbPath, err)
	}

	slog.Info("Opened per-tenant database", "tenant_id", tenantID, "path", dbPath)
	return store, nil
}

func (m *TenantStoreManager) tenantDataDir(tenantID uuid.UUID) string {
	return filepath.Join(m.basePath, "tenants", tenantID.String())
}

func (m *TenantStoreManager) closeTenantStore(tenantID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	store, ok := m.stores[tenantID]
	if !ok {
		return nil
	}
	delete(m.stores, tenantID)

	if err := store.Close(); err != nil {
		return fmt.Errorf("close tenant store %s: %w", tenantID, err)
	}
	return nil
}

func (m *TenantStoreManager) syncSiteTenantData(ctx context.Context, siteID, tenantID uuid.UUID) error {
	defaultID, err := m.DefaultTenantID(ctx)
	if err != nil {
		return fmt.Errorf("resolve default tenant for site sync: %w", err)
	}
	if tenantID == uuid.Nil || tenantID == defaultID {
		return nil
	}

	tenantStore, err := m.ForTenant(ctx, tenantID)
	if err != nil {
		return err
	}

	site, err := m.shared.GetSiteByID(ctx, siteID)
	if err != nil {
		return fmt.Errorf("load site %s for tenant sync: %w", siteID, err)
	}
	if site == nil {
		return fmt.Errorf("site %s not found", siteID)
	}

	if err := tenantStore.UpsertSiteMirror(ctx, site); err != nil {
		return fmt.Errorf("mirror site %s into tenant %s: %w", siteID, tenantID, err)
	}

	if err := m.backfillLegacyGoals(ctx, tenantStore, siteID, tenantID); err != nil {
		return err
	}
	if err := m.backfillLegacyFunnels(ctx, tenantStore, siteID, tenantID); err != nil {
		return err
	}
	return nil
}

func copySiteAnalyticsBetweenStores(ctx context.Context, sourceStore, destinationStore *Store, siteID uuid.UUID) error {
	if sourceStore == nil || destinationStore == nil {
		return fmt.Errorf("source and destination stores are required")
	}
	if sourceStore.db == destinationStore.db {
		return nil
	}

	for _, table := range siteAnalyticsTransferTables {
		if !isSafeIdentifier(table) {
			return fmt.Errorf("unsafe analytics transfer table %q", table)
		}
		if err := copySiteAnalyticsTable(ctx, sourceStore.db, destinationStore.db, table, siteID); err != nil {
			return fmt.Errorf("copy site analytics table %s: %w", table, err)
		}
	}

	return nil
}

func copySiteAnalyticsTable(ctx context.Context, sourceDB, destinationDB *sql.DB, table string, siteID uuid.UUID) error {
	// #nosec G201 -- table is validated via isSafeIdentifier before formatting.
	query := fmt.Sprintf("SELECT * FROM %s WHERE site_id = ?", table)
	rows, err := sourceDB.QueryContext(ctx, query, siteID)
	if err != nil {
		return err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("list columns: %w", err)
	}
	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		return fmt.Errorf("list column types: %w", err)
	}
	if len(columns) == 0 {
		return nil
	}
	for _, column := range columns {
		if !isSafeIdentifier(column) {
			return fmt.Errorf("unsafe analytics transfer column %q", column)
		}
	}

	// #nosec G201 -- table and columns are validated via isSafeIdentifier before formatting.
	insertSQL := fmt.Sprintf(
		"INSERT OR REPLACE INTO %s (%s) VALUES (%s)",
		table,
		strings.Join(columns, ", "),
		placeholders(len(columns)),
	)

	values := make([]any, len(columns))
	scanTargets := make([]any, len(columns))
	for i := range values {
		scanTargets[i] = &values[i]
	}

	for rows.Next() {
		if err := rows.Scan(scanTargets...); err != nil {
			return fmt.Errorf("scan row: %w", err)
		}
		for i := range values {
			values[i], err = normalizeAnalyticsTransferValue(values[i], columnTypes[i])
			if err != nil {
				return fmt.Errorf("normalize %s.%s: %w", table, columns[i], err)
			}
		}
		if _, err := destinationDB.ExecContext(ctx, insertSQL, values...); err != nil {
			return fmt.Errorf("insert row: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate rows: %w", err)
	}

	return nil
}

func normalizeAnalyticsTransferValue(value any, columnType *sql.ColumnType) (any, error) {
	if value == nil || columnType == nil {
		return value, nil
	}

	if strings.EqualFold(columnType.DatabaseTypeName(), "JSON") {
		switch typed := value.(type) {
		case string, []byte:
			return typed, nil
		default:
			encoded, err := json.Marshal(typed)
			if err != nil {
				return nil, fmt.Errorf("marshal json value: %w", err)
			}
			return string(encoded), nil
		}
	}

	return value, nil
}

func placeholders(count int) string {
	if count <= 0 {
		return ""
	}

	values := make([]string, count)
	for i := range count {
		values[i] = "?"
	}
	return strings.Join(values, ", ")
}

func (m *TenantStoreManager) backfillLegacyGoals(ctx context.Context, tenantStore *Store, siteID, tenantID uuid.UUID) error {
	goals, err := tenantStore.GetGoals(ctx, siteID)
	if err != nil {
		return fmt.Errorf("list tenant goals for site %s: %w", siteID, err)
	}
	if len(goals) > 0 {
		return nil
	}

	legacyGoals, err := m.shared.GetGoals(ctx, siteID)
	if err != nil {
		return fmt.Errorf("list shared goals for site %s: %w", siteID, err)
	}
	for _, goal := range legacyGoals {
		goalCopy := goal
		if err := tenantStore.UpsertGoal(ctx, &goalCopy); err != nil {
			return fmt.Errorf("backfill goal %s into tenant %s: %w", goal.ID, tenantID, err)
		}
	}
	if len(legacyGoals) > 0 {
		slog.Info("Backfilled legacy goals into tenant analytics store", "tenant_id", tenantID, "site_id", siteID, "count", len(legacyGoals))
	}
	return nil
}

func (m *TenantStoreManager) backfillLegacyFunnels(ctx context.Context, tenantStore *Store, siteID, tenantID uuid.UUID) error {
	funnels, err := tenantStore.GetFunnels(ctx, siteID)
	if err != nil {
		return fmt.Errorf("list tenant funnels for site %s: %w", siteID, err)
	}
	if len(funnels) > 0 {
		return nil
	}

	legacyFunnels, err := m.shared.GetFunnels(ctx, siteID)
	if err != nil {
		return fmt.Errorf("list shared funnels for site %s: %w", siteID, err)
	}
	for _, funnel := range legacyFunnels {
		funnelCopy := funnel
		if err := tenantStore.UpsertFunnel(ctx, &funnelCopy); err != nil {
			return fmt.Errorf("backfill funnel %s into tenant %s: %w", funnel.ID, tenantID, err)
		}
	}
	if len(legacyFunnels) > 0 {
		slog.Info("Backfilled legacy funnels into tenant analytics store", "tenant_id", tenantID, "site_id", siteID, "count", len(legacyFunnels))
	}
	return nil
}

func IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}
