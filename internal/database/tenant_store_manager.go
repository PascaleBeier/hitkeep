package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	lru "github.com/hashicorp/golang-lru/v2/expirable"

	"hitkeep/internal/api"
	"hitkeep/internal/controlstore"
)

const (
	siteSyncMemoSize = 16384
	siteSyncMemoTTL  = 30 * time.Second
)

// siteSyncKey memoizes per-site mirror syncs. The tenant is part of the key
// so a site transfer (new tenant) always syncs immediately.
type siteSyncKey struct {
	siteID   uuid.UUID
	tenantID uuid.UUID
}

// TenantStoreManager provides per-tenant database isolation. The default
// tenant file is the root of one shared tenant-data DuckDB instance and
// non-default files are attached as catalogs.
type TenantStoreManager struct {
	control      *controlstore.Store
	basePath     string
	storeOptions []StoreOption

	mu              sync.RWMutex
	stores          map[uuid.UUID]*Store
	attachedAliases map[uuid.UUID]string
	dataPlaneRoot   *Store

	// recentSyncs suppresses repeated mirror syncs on the ingest path;
	// explicit SyncSite calls bypass it and force a refresh.
	recentSyncs *lru.LRU[siteSyncKey, struct{}]

	defaultID uuid.UUID

	maintenanceCtx context.Context

	compaction  *CompactionOptions
	fatalErrors chan error
}

// TenantStoreManagerOption configures a TenantStoreManager.
type TenantStoreManagerOption func(*TenantStoreManager)

// WithTenantCompaction rewrites a tenant's database file on lazy open when
// its share of free blocks exceeds the given thresholds, returning space
// freed by retention and deletes to the operating system.
func WithTenantCompaction(opts CompactionOptions) TenantStoreManagerOption {
	return func(m *TenantStoreManager) {
		m.compaction = &opts
	}
}

// NewTenantStoreManager creates a tenant data-plane coordinator backed by the
// SQLite control store. It resolves and caches the default tenant ID there.
func NewTenantStoreManager(control *controlstore.Store, basePath string, storeOptions []StoreOption, opts ...TenantStoreManagerOption) *TenantStoreManager {
	mgr := &TenantStoreManager{
		control:         control,
		basePath:        basePath,
		storeOptions:    append([]StoreOption(nil), storeOptions...),
		stores:          make(map[uuid.UUID]*Store),
		attachedAliases: make(map[uuid.UUID]string),
		recentSyncs:     lru.NewLRU[siteSyncKey, struct{}](siteSyncMemoSize, nil, siteSyncMemoTTL),
		fatalErrors:     make(chan error, 1),
	}
	for _, opt := range opts {
		opt(mgr)
	}

	// Best-effort default tenant ID resolution. If the tenant table doesn't
	// exist yet (pre-migration) we'll resolve lazily.
	defaultID, err := control.GetDefaultTenantID(context.Background())
	if err != nil {
		slog.Debug("TenantStoreManager: could not resolve default tenant ID at init (will resolve lazily)", "error", err)
	} else {
		mgr.defaultID = defaultID
	}

	return mgr
}

// Control returns the control-plane store (hitkeep.db).
func (m *TenantStoreManager) Control() *controlstore.Store { return m.control }

// FatalErrors reports tenant database conditions that require a controlled
// process restart.
func (m *TenantStoreManager) FatalErrors() <-chan error {
	if m == nil {
		return nil
	}
	return m.fatalErrors
}

// UnavailableDatabaseStatus returns one non-healthy tenant database status.
// Callers use it to make readiness and request availability conservative while
// a tenant database is recovering.
func (m *TenantStoreManager) UnavailableDatabaseStatus() (DatabaseStatus, bool) {
	if m == nil {
		return DatabaseStatus{}, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, store := range m.stores {
		if status := store.DatabaseStatus(); status.State != DatabaseStateHealthy {
			return status, true
		}
	}
	return DatabaseStatus{}, false
}

// StartMaintenance runs periodic checkpoint maintenance on every per-tenant
// store: those already open and those opened later. The shared store's
// maintenance is managed by its owner.
func (m *TenantStoreManager) StartMaintenance(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maintenanceCtx = ctx
	for _, store := range m.stores {
		store.StartMaintenance(ctx)
	}
}

// DefaultTenantID returns the cached default tenant ID, resolving lazily if needed.
func (m *TenantStoreManager) DefaultTenantID(ctx context.Context) (uuid.UUID, error) {
	if m.defaultID != uuid.Nil {
		return m.defaultID, nil
	}

	defaultID, err := m.control.GetDefaultTenantID(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	m.defaultID = defaultID
	return defaultID, nil
}

// ForTenant returns the Store for the given tenant. uuid.Nil resolves to the
// default tenant. In split mode the default file is the root catalog and
// non-default files are attached lazily under deterministic aliases.
func (m *TenantStoreManager) ForTenant(ctx context.Context, tenantID uuid.UUID) (*Store, error) {
	defaultID, err := m.DefaultTenantID(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not resolve default tenant: %w", err)
	}
	if tenantID == uuid.Nil {
		tenantID = defaultID
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

	if tenantID == defaultID {
		store, err := m.openDefaultTenantStore(ctx, defaultID)
		if err != nil {
			return nil, err
		}
		m.stores[tenantID] = store
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
	tenantID, err := m.control.GetActiveTenantID(ctx, userID)
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
	tenantID, err := m.control.GetSiteTenantID(ctx, siteID)
	if err != nil {
		return nil, uuid.Nil, fmt.Errorf("resolve tenant for site %s: %w", siteID, err)
	}

	store, err := m.ForTenant(ctx, tenantID)
	if err != nil {
		return nil, uuid.Nil, err
	}

	// Ingest resolves the same sites continuously; memoize the mirror sync
	// instead of re-running it per message.
	key := siteSyncKey{siteID: siteID, tenantID: tenantID}
	if _, ok := m.recentSyncs.Get(key); !ok {
		if err := m.syncSiteTenantData(ctx, siteID, tenantID); err != nil {
			return nil, uuid.Nil, err
		}
		m.recentSyncs.Add(key, struct{}{})
	}

	return store, tenantID, nil
}

// GetUserOnboarding keeps account and team progress on the control plane but
// reads first-hit and automatic-event activity from each owning tenant store.
func (m *TenantStoreManager) GetUserOnboarding(ctx context.Context, userID uuid.UUID) (*api.UserOnboarding, error) {
	if m == nil || m.control == nil {
		return nil, fmt.Errorf("tenant store manager is not configured")
	}
	return m.control.GetUserOnboardingWithActivity(ctx, userID, func(ctx context.Context, siteID uuid.UUID) (*api.SiteTrackingStatus, error) {
		store, _, err := m.ResolveSiteStore(ctx, siteID)
		if err != nil {
			return nil, err
		}
		return store.GetSiteTrackingStatus(ctx, siteID, time.Now().UTC())
	})
}

// SyncSite mirrors the site's metadata into its tenant catalog. It always
// performs the sync; ResolveSiteStore memoizes it for ingest.
func (m *TenantStoreManager) SyncSite(ctx context.Context, siteID uuid.UUID) error {
	tenantID, err := m.control.GetSiteTenantID(ctx, siteID)
	if err != nil {
		return fmt.Errorf("resolve tenant for site %s: %w", siteID, err)
	}
	if err := m.syncSiteTenantData(ctx, siteID, tenantID); err != nil {
		return err
	}
	m.recentSyncs.Add(siteSyncKey{siteID: siteID, tenantID: tenantID}, struct{}{})
	return nil
}

// SyncAllTenants eagerly syncs all known sites into their tenant-local stores.
func (m *TenantStoreManager) SyncAllTenants(ctx context.Context) error {
	sites, err := m.control.ListAllSites(ctx)
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

	if err := analyticsStore.DeleteSite(ctx, siteID); err != nil {
		return fmt.Errorf("delete tenant analytics site %s: %w", siteID, err)
	}

	if err := m.control.DeleteSite(ctx, siteID); err != nil {
		return fmt.Errorf("delete shared site %s: %w", siteID, err)
	}
	return nil
}

func (m *TenantStoreManager) DeleteSiteWithWebhookEvent(ctx context.Context, siteID uuid.UUID, event WebhookEventInput) ([]WebhookDeliveryJob, error) {
	analyticsStore, _, err := m.ResolveSiteStore(ctx, siteID)
	if err != nil {
		return nil, err
	}
	if err := analyticsStore.DeleteSite(ctx, siteID); err != nil {
		return nil, fmt.Errorf("delete tenant analytics site %s: %w", siteID, err)
	}
	jobs, err := m.control.DeleteSiteWithWebhookEvent(ctx, siteID, toControlWebhookEvent(event))
	if err != nil {
		return nil, fmt.Errorf("delete shared site %s with webhook event: %w", siteID, err)
	}
	return fromControlWebhookJobs(jobs), nil
}

func toControlWebhookEvent(input WebhookEventInput) controlstore.WebhookEventInput {
	return input
}

func fromControlWebhookJobs(input []controlstore.WebhookDeliveryJob) []WebhookDeliveryJob {
	return input
}

func (m *TenantStoreManager) ResetSiteStats(ctx context.Context, siteID uuid.UUID) (api.SiteStatsResetResponse, error) {
	analyticsStore, _, err := m.ResolveSiteStore(ctx, siteID)
	if err != nil {
		return api.SiteStatsResetResponse{Status: "reset"}, err
	}

	result, err := analyticsStore.resetSiteAnalyticsMeasurements(ctx, siteID)
	if err != nil {
		return result, fmt.Errorf("reset tenant analytics stats for site %s: %w", siteID, err)
	}
	sharedResult, err := m.control.ResetSiteControlMeasurements(ctx, siteID)
	if err != nil {
		return result, fmt.Errorf("reset shared stats for site %s: %w", siteID, err)
	}
	mergeSiteStatsResetResult(&result, sharedResult)
	return result, nil
}

// PurgeArchivedTenant removes the per-tenant analytics database directory and
// deletes archived control-plane records for a non-default tenant.
func (m *TenantStoreManager) PurgeArchivedTenant(ctx context.Context, tenantID uuid.UUID) (*api.Team, error) {
	team, err := m.control.GetPurgeableTenant(ctx, tenantID)
	if err != nil || team == nil {
		return team, err
	}

	if err := m.closeTenantStore(tenantID); err != nil {
		return nil, err
	}

	if err := os.RemoveAll(m.tenantDataDir(tenantID)); err != nil {
		return nil, fmt.Errorf("remove tenant data directory: %w", err)
	}

	deleted, err := m.control.DeleteArchivedTenantMetadata(ctx, tenantID)
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
func (m *TenantStoreManager) TransferSite(ctx context.Context, siteID, destinationTenantID uuid.UUID, auditEntries ...AuditEntryParams) error {
	sourceTenantID, err := m.control.GetSiteTenantID(ctx, siteID)
	if err != nil {
		return fmt.Errorf("resolve source tenant for site %s: %w", siteID, err)
	}
	if sourceTenantID == destinationTenantID {
		return nil
	}

	site, err := m.control.GetSiteByID(ctx, siteID)
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
	searchConsoleMapping, err := m.control.GetGoogleSearchConsoleSiteMappingForTeam(ctx, siteID, sourceTenantID)
	if err != nil {
		return fmt.Errorf("load Search Console mapping before site transfer: %w", err)
	}
	if !hasSiteTransferAudit(auditEntries, siteID, sourceTenantID, destinationTenantID) {
		return fmt.Errorf("site transfer audit is required")
	}
	if searchConsoleMapping != nil && !hasSearchConsoleTransferAudit(auditEntries, siteID, sourceTenantID) {
		return fmt.Errorf("search console transfer audit is required")
	}

	// Mirror the site into the destination before copying analytics: tenant
	// schemas keep foreign keys on sites (e.g. ai_fetches), so the parent row
	// must exist first.
	if err := destinationStore.UpsertSiteMirror(ctx, site); err != nil {
		return fmt.Errorf("upsert destination site mirror: %w", err)
	}

	if sourceStore != destinationStore {
		if err := m.control.AppendAuditEntry(ctx, toControlAudit(siteTransferDataMovePreparedAudit(site, sourceTenantID, destinationTenantID, auditEntries))); err != nil {
			return fmt.Errorf("append site transfer data move audit: %w", err)
		}
		copiedTables, err := copySiteAnalyticsBetweenStores(ctx, sourceStore, destinationStore, siteID)
		if err != nil {
			return err
		}
		if err := deleteSiteAnalyticsOnly(ctx, sourceStore, siteID, copiedTables, true); err != nil {
			return err
		}
	}

	if err := m.control.TransferSiteTeamWithAudit(ctx, siteID, destinationTenantID, searchConsoleMapping != nil, toControlAudits(auditEntries)); err != nil {
		return fmt.Errorf("update shared site transfer records: %w", err)
	}
	if err := m.SyncSite(ctx, siteID); err != nil {
		return err
	}

	return nil
}

func toControlAudit(input AuditEntryParams) controlstore.AuditEntryParams {
	return input
}

func toControlAudits(input []AuditEntryParams) []controlstore.AuditEntryParams {
	result := make([]controlstore.AuditEntryParams, len(input))
	for i := range input {
		result[i] = toControlAudit(input[i])
	}
	return result
}

func siteTransferDataMovePreparedAudit(site *api.Site, sourceTenantID, destinationTenantID uuid.UUID, audits []AuditEntryParams) AuditEntryParams {
	entry := AuditEntryParams{
		TeamID:      sourceTenantID,
		Action:      "site.transfer_data_move_prepared",
		TargetType:  "site",
		TargetID:    site.ID.String(),
		TargetLabel: site.Domain,
		Outcome:     "success",
		Details:     fmt.Sprintf("outcome=prepared;destination_team_id=%s", destinationTenantID),
	}
	if len(audits) == 0 {
		return entry
	}
	base := audits[0]
	entry.ActorID = base.ActorID
	entry.ActorEmail = base.ActorEmail
	entry.ActorRole = base.ActorRole
	entry.IPAddress = base.IPAddress
	entry.IPCountryCode = base.IPCountryCode
	entry.UserAgent = base.UserAgent
	entry.RequestID = base.RequestID
	return entry
}

func hasSiteTransferAudit(audits []AuditEntryParams, siteID, sourceTeamID, destinationTeamID uuid.UUID) bool {
	var hasTransferredOut bool
	var hasTransferredIn bool
	for _, audit := range audits {
		if audit.TargetType != "site" || audit.TargetID != siteID.String() || audit.Outcome != "success" {
			continue
		}
		if audit.TeamID == sourceTeamID && audit.Action == "site.transferred_out" {
			hasTransferredOut = true
		}
		if audit.TeamID == destinationTeamID && audit.Action == "site.transferred_in" {
			hasTransferredIn = true
		}
	}
	return hasTransferredOut && hasTransferredIn
}

func hasSearchConsoleTransferAudit(audits []AuditEntryParams, siteID, teamID uuid.UUID) bool {
	for _, audit := range audits {
		if audit.TeamID == teamID &&
			audit.Action == "google_search_console.property_unmapped" &&
			audit.TargetType == "site" &&
			audit.TargetID == siteID.String() &&
			audit.Outcome == "success" {
			return true
		}
	}
	return false
}

// Close closes all per-tenant stores and detaches shared data-plane catalogs.
// The caller is responsible for closing the control store separately.
func (m *TenantStoreManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var firstErr error
	for id, store := range m.stores {
		if id == m.defaultID {
			continue
		}
		if err := store.Close(); err != nil {
			slog.Error("Failed to close tenant store", "tenant_id", id, "error", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	if m.dataPlaneRoot != nil {
		for id, alias := range m.attachedAliases {
			if _, err := m.dataPlaneRoot.DB().ExecContext(context.Background(), "DETACH "+safeCatalogIdentifier(alias)); err != nil {
				slog.Warn("Failed to detach tenant catalog during shutdown", "tenant_id", id, "error", err)
			}
		}
		if err := m.dataPlaneRoot.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		m.dataPlaneRoot = nil
	}
	m.stores = make(map[uuid.UUID]*Store)
	m.attachedAliases = make(map[uuid.UUID]string)
	return firstErr
}

func (m *TenantStoreManager) openDefaultTenantStore(ctx context.Context, tenantID uuid.UUID) (*Store, error) {
	if m.dataPlaneRoot != nil {
		return m.dataPlaneRoot, nil
	}
	dir := m.tenantDataDir(tenantID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("could not create default tenant data directory %s: %w", dir, err)
	}
	dbPath := filepath.Join(dir, "hitkeep.db")
	if err := recoverCompactionSwap(dbPath); err != nil {
		return nil, err
	}
	if _, err := os.Stat(dbPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			complete, markerErr := m.control.DefaultTenantSplitComplete(ctx)
			if markerErr != nil {
				return nil, fmt.Errorf("inspect default tenant split marker: %w", markerErr)
			}
			imported, importErr := m.control.LegacyControlImportComplete(ctx)
			if importErr != nil {
				return nil, fmt.Errorf("inspect legacy control import marker: %w", importErr)
			}
			if complete && imported {
				return nil, fmt.Errorf("default tenant split marker exists but tenant file is missing: %s", dbPath)
			}
			fresh := NewStore(dbPath, m.tenantStoreOptions(tenantID)...)
			if err := fresh.Connect(); err != nil {
				return nil, fmt.Errorf("create default tenant database %s: %w", dbPath, err)
			}
			if err := fresh.migrateTenant(ctx, migrationRunOptions{guarded: true}); err != nil {
				_ = fresh.Close()
				return nil, fmt.Errorf("migrate fresh default tenant database %s: %w", dbPath, err)
			}
			m.dataPlaneRoot = fresh
			return fresh, nil
		}
		return nil, fmt.Errorf("stat default tenant database %s: %w", dbPath, err)
	}
	if m.compaction != nil {
		if result, err := MaybeCompactDatabase(ctx, dbPath, *m.compaction, PrepareTenantSchema); err != nil {
			slog.Warn("Skipping default tenant database compaction", "path", dbPath, "error", err)
		} else if result.Compacted {
			slog.Info("Compacted default tenant database", "path", dbPath, "bytes_before", result.BytesBefore, "bytes_after", result.BytesAfter)
		}
	}
	options := m.tenantStoreOptions(tenantID)
	store := NewStore(dbPath, options...)
	if err := store.Connect(); err != nil {
		return nil, fmt.Errorf("could not connect to default tenant database %s: %w", dbPath, err)
	}
	if err := store.migrateTenant(ctx, migrationRunOptions{guarded: true}); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("could not migrate default tenant database %s: %w", dbPath, err)
	}
	if m.maintenanceCtx != nil {
		store.StartMaintenance(m.maintenanceCtx)
	}
	m.dataPlaneRoot = store
	slog.Info("Opened shared tenant data-plane root", "tenant_id", tenantID, "path", dbPath)
	return store, nil
}

// openTenantStore creates the directory structure and opens a tenant. In
// split mode the physical file is prepared standalone, then attached to the
// default root and exposed through a catalog-specific logical Store.
// Must be called with m.mu held.
func (m *TenantStoreManager) openTenantStore(ctx context.Context, tenantID uuid.UUID) (*Store, error) {
	dir := m.tenantDataDir(tenantID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("could not create tenant data directory %s: %w", dir, err)
	}

	dbPath := filepath.Join(dir, "hitkeep.db")
	if err := recoverCompactionSwap(dbPath); err != nil {
		return nil, err
	}

	// Compaction remains best effort. A replay-failing WAL makes measurement
	// fail without rewriting the file; the subsequent Store.Connect call then
	// retains a recovery bundle and follows the configured recovery policy.
	if m.compaction != nil {
		if result, err := MaybeCompactDatabase(ctx, dbPath, *m.compaction, PrepareTenantSchema); err != nil {
			slog.Warn("Skipping tenant database compaction", "tenant_id", tenantID, "path", dbPath, "error", err)
		} else if result.Compacted {
			slog.Info("Compacted tenant database", "tenant_id", tenantID, "path", dbPath, "bytes_before", result.BytesBefore, "bytes_after", result.BytesAfter)
		}
	}

	storeOptions := m.tenantStoreOptions(tenantID)
	if _, err := m.openDefaultTenantStore(ctx, m.defaultID); err != nil {
		return nil, err
	}
	standalone := NewStore(dbPath, storeOptions...)
	if err := standalone.Connect(); err != nil {
		fatal := fmt.Errorf("could not connect to tenant database %s: %w", dbPath, err)
		m.tenantFatalReporter(tenantID)(fatal)
		return nil, fatal
	}
	if err := standalone.migrateTenant(ctx, migrationRunOptions{guarded: true}); err != nil {
		_ = standalone.Close()
		fatal := fmt.Errorf("could not migrate tenant database %s: %w", dbPath, err)
		m.tenantFatalReporter(tenantID)(fatal)
		return nil, fatal
	}
	if err := standalone.Close(); err != nil {
		return nil, fmt.Errorf("close tenant database before attach %s: %w", dbPath, err)
	}
	alias := "tenant_" + strings.ReplaceAll(tenantID.String(), "-", "")
	if _, err := m.dataPlaneRoot.DB().ExecContext(ctx, fmt.Sprintf("ATTACH '%s' AS %s;", escapeSQLString(dbPath), safeCatalogIdentifier(alias))); err != nil {
		fatal := fmt.Errorf("attach tenant database %s: %w", tenantID, err)
		m.tenantFatalReporter(tenantID)(fatal)
		return nil, fatal
	}
	store := newAttachedStore(m.dataPlaneRoot.path, dbPath, alias, storeOptions...)
	// Set the shared gates before Connect builds the reconnecting
	// connector. Assigning them after Connect would leave the pool using
	// its private semaphore and defeat the process-wide native connection
	// cap required by the shared data plane.
	store.checkpointGate = m.dataPlaneRoot.checkpointGate
	store.connectionGate = m.dataPlaneRoot.connectionGate
	if err := store.Connect(); err != nil {
		_, _ = m.dataPlaneRoot.DB().ExecContext(ctx, "DETACH "+safeCatalogIdentifier(alias))
		fatal := fmt.Errorf("connect attached tenant database %s: %w", tenantID, err)
		m.tenantFatalReporter(tenantID)(fatal)
		return nil, fatal
	}
	if m.maintenanceCtx != nil {
		store.StartMaintenance(m.maintenanceCtx)
	}
	m.attachedAliases[tenantID] = alias
	slog.Info("Attached tenant database", "tenant_id", tenantID, "catalog", alias, "path", dbPath)
	return store, nil
}

// tenantStoreOptions keeps every tenant connection aligned with the shared
// store's DuckDB settings while adding tenant-specific routing and failure
// reporting.
func (m *TenantStoreManager) tenantStoreOptions(tenantID uuid.UUID) []StoreOption {
	return append(append([]StoreOption(nil), m.storeOptions...),
		withTenantID(tenantID),
		withFatalReporter(m.tenantFatalReporter(tenantID)),
	)
}

func (m *TenantStoreManager) tenantFatalReporter(tenantID uuid.UUID) func(error) {
	return func(err error) {
		if m == nil || err == nil {
			return
		}
		select {
		case m.fatalErrors <- fmt.Errorf("tenant %s database requires controlled restart: %w", tenantID, err):
		default:
		}
	}
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

	if err := store.Close(); err != nil {
		return fmt.Errorf("close tenant store %s: %w", tenantID, err)
	}
	if alias, ok := m.attachedAliases[tenantID]; ok && m.dataPlaneRoot != nil {
		if _, err := m.dataPlaneRoot.DB().ExecContext(context.Background(), "DETACH "+safeCatalogIdentifier(alias)); err != nil {
			return fmt.Errorf("detach tenant store %s: %w", tenantID, err)
		}
		delete(m.attachedAliases, tenantID)
	}
	delete(m.stores, tenantID)
	return nil
}

func (m *TenantStoreManager) syncSiteTenantData(ctx context.Context, siteID, tenantID uuid.UUID) error {
	if tenantID == uuid.Nil {
		return nil
	}

	tenantStore, err := m.ForTenant(ctx, tenantID)
	if err != nil {
		return err
	}
	site, err := m.control.GetSiteByID(ctx, siteID)
	if err != nil {
		return fmt.Errorf("load site %s for tenant sync: %w", siteID, err)
	}
	if site == nil {
		return fmt.Errorf("site %s not found", siteID)
	}

	if err := tenantStore.UpsertSiteMirror(ctx, site); err != nil {
		return fmt.Errorf("mirror site %s into tenant %s: %w", siteID, tenantID, err)
	}

	return nil
}

// copySiteAnalyticsBetweenStores copies the site's rows for every site-scoped
// table that exists in both schemas (derived from the live schemas, parents
// before foreign-key children) and returns the copied tables so the caller
// can clean exactly the same set from the source.
func copySiteAnalyticsBetweenStores(ctx context.Context, sourceStore, destinationStore *Store, siteID uuid.UUID) ([]string, error) {
	if sourceStore == nil || destinationStore == nil {
		return nil, fmt.Errorf("source and destination stores are required")
	}
	if sourceStore.db == destinationStore.db {
		return nil, nil
	}

	tables, err := listScopedCopyTables(ctx, sourceStore.db, destinationStore.db, "site_id", "sites", siteExtraEdges)
	if err != nil {
		return nil, err
	}
	for _, table := range tables {
		if err := copySiteAnalyticsTable(ctx, sourceStore.db, destinationStore.db, table, siteID); err != nil {
			return nil, fmt.Errorf("copy site analytics table %s: %w", table, err)
		}
	}

	return tables, nil
}

func copySiteAnalyticsTable(ctx context.Context, sourceDB, destinationDB *sql.DB, table string, siteID uuid.UUID) error {
	// Replace the site's rows wholesale: the analytics tables carry no unique
	// constraints (their ART indexes were dropped for memory and ingest
	// performance), so conflict-based upserts are not available here.
	// #nosec G201 -- table is validated via isSafeIdentifier before formatting.
	deleteQuery := fmt.Sprintf("DELETE FROM %s WHERE site_id = ?", table)
	if _, err := destinationDB.ExecContext(ctx, deleteQuery, siteID); err != nil {
		return fmt.Errorf("clear destination rows: %w", err)
	}

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
		"INSERT INTO %s (%s) VALUES (%s)",
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

func IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}
