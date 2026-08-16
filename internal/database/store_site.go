package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/auth"
	"hitkeep/internal/hklog"
)

// FindSiteByDomain resolves a site by its tracked domain. It runs once per
// ingested browser hit, so results — including misses, which bot traffic
// produces in bulk — are held in a short-TTL cache with singleflight
// collapsing concurrent lookups; domain writers must call invalidateSiteDomain.
func (s *Store) FindSiteByDomain(ctx context.Context, domain string) (*api.Site, error) {
	if site, ok := s.getCachedSiteByDomain(domain); ok {
		return site, nil
	}
	if s.runtime == nil {
		return s.querySiteByDomain(ctx, domain)
	}

	result, err, _ := s.runtime.siteDomainSF.Do(domain, func() (any, error) {
		site, err := s.querySiteByDomain(ctx, domain)
		if err != nil {
			return nil, err
		}
		s.cacheSiteByDomain(domain, site)
		return site, nil
	})
	if err != nil {
		return nil, err
	}
	return cloneSite(result.(*api.Site)), nil
}

func (s *Store) querySiteByDomain(ctx context.Context, domain string) (*api.Site, error) {
	var site api.Site
	err := s.db.QueryRowContext(ctx, "SELECT id, user_id, domain, created_at FROM sites WHERE domain = ?", domain).Scan(&site.ID, &site.UserID, &site.Domain, &site.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("could not query for site: %w", err)
	}
	return &site, nil
}

func (s *Store) GetSiteByID(ctx context.Context, siteID uuid.UUID) (*api.Site, error) {
	var site api.Site
	err := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, domain, data_retention_days, created_at
		FROM sites
		WHERE id = ?`,
		siteID,
	).Scan(&site.ID, &site.UserID, &site.Domain, &site.DataRetentionDays, &site.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("could not query site by id: %w", err)
	}
	return &site, nil
}

func (s *Store) GetSite(ctx context.Context, siteID uuid.UUID, userID uuid.UUID) (*api.Site, error) {
	activeTenantID, err := s.GetActiveTenantID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("could not resolve active tenant: %w", err)
	}
	defaultTenantID, err := s.GetDefaultTenantID(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not resolve default tenant: %w", err)
	}

	var site api.Site
	err = s.db.QueryRowContext(ctx, `
		SELECT id, user_id, domain, created_at
		FROM sites s
		LEFT JOIN site_tenants st ON st.site_id = s.id
		WHERE s.id = ?
			AND s.user_id = ?
			AND COALESCE(st.tenant_id, ?) = ?
	`, siteID, userID, defaultTenantID, activeTenantID).Scan(&site.ID, &site.UserID, &site.Domain, &site.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("could not query for site: %w", err)
	}
	return &site, nil
}

func (s *Store) CreateSite(ctx context.Context, userID uuid.UUID, domain string) (*api.Site, error) {
	id := uuid.New()
	now := time.Now()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := ensureDefaultTenantTx(ctx, tx, defaultTenantName, false); err != nil {
		return nil, err
	}

	tenantID, err := getActiveTenantID(ctx, tx, userID)
	if err != nil {
		return nil, fmt.Errorf("could not resolve site tenant: %w", err)
	}

	if err := ensureTenantMemberTx(ctx, tx, tenantID, userID, TenantRoleOwner, userID); err != nil {
		return nil, err
	}

	_, err = tx.ExecContext(ctx,
		"INSERT INTO sites (id, user_id, domain, created_at) VALUES (?, ?, ?, ?)",
		id, userID, domain, now,
	)
	if err != nil {
		return nil, fmt.Errorf("could not create site: %w", err)
	}

	_, err = tx.ExecContext(ctx,
		"INSERT INTO site_tenants (site_id, tenant_id, created_at) VALUES (?, ?, ?)",
		id, tenantID, now,
	)
	if err != nil {
		return nil, fmt.Errorf("could not create site tenant mapping: %w", err)
	}

	// Add creator as site owner
	_, err = tx.ExecContext(ctx,
		"INSERT INTO site_members (site_id, user_id, role, added_by) VALUES (?, ?, 'owner', ?)",
		id, userID, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("could not add site owner: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}
	s.invalidateSiteDomain(domain)

	return &api.Site{
		ID:        id,
		UserID:    userID,
		Domain:    domain,
		CreatedAt: now,
	}, nil
}

func (s *Store) UpdateSiteTenant(ctx context.Context, siteID, tenantID uuid.UUID) error {
	if err := updateSiteTenant(ctx, s.db, siteID, tenantID); err != nil {
		return err
	}
	s.invalidateSiteTenantID(siteID)
	return nil
}

func updateSiteTenant(ctx context.Context, exec sqlExecContext, siteID, tenantID uuid.UUID) error {
	now := time.Now().UTC()
	_, err := exec.ExecContext(ctx, `
		INSERT INTO site_tenants (site_id, tenant_id, created_at)
		VALUES (?, ?, ?)
		ON CONFLICT (site_id) DO UPDATE SET
			tenant_id = excluded.tenant_id
	`, siteID, tenantID, now)
	if err != nil {
		return fmt.Errorf("could not update site tenant mapping: %w", err)
	}
	return nil
}

func (s *Store) GetSites(ctx context.Context, userID uuid.UUID) ([]api.Site, error) {
	instanceRole, err := s.GetInstanceRole(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("could not get instance role: %w", err)
	}
	activeTenantID, err := s.GetActiveTenantID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("could not resolve active tenant: %w", err)
	}
	defaultTenantID, err := s.GetDefaultTenantID(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not resolve default tenant: %w", err)
	}

	canManageActiveTenantSites := false
	if tenantRole, err := s.GetTenantRole(ctx, activeTenantID, userID); err == nil {
		_, canManageActiveTenantSites = tenantRoleSiteRole(tenantRole)
	}

	var rows *sql.Rows
	if instanceRole.HasPermission(auth.PermInstanceViewAllSites) || canManageActiveTenantSites {
		rows, err = s.db.QueryContext(ctx, `
			SELECT s.id, s.user_id, s.domain, s.data_retention_days, s.created_at
			FROM sites s
			LEFT JOIN site_tenants st ON st.site_id = s.id
			WHERE COALESCE(st.tenant_id, ?) = ?
			ORDER BY s.created_at DESC
		`, defaultTenantID, activeTenantID)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT DISTINCT s.id, s.user_id, s.domain, s.data_retention_days, s.created_at
			FROM sites s
			LEFT JOIN site_tenants st ON st.site_id = s.id
			LEFT JOIN site_members sm ON sm.site_id = s.id AND sm.user_id = ?
			WHERE COALESCE(st.tenant_id, ?) = ?
				AND (s.user_id = ? OR sm.user_id IS NOT NULL)
			ORDER BY s.created_at DESC
		`,
			userID, defaultTenantID, activeTenantID, userID,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sites := []api.Site{}
	for rows.Next() {
		var site api.Site
		if err := rows.Scan(&site.ID, &site.UserID, &site.Domain, &site.DataRetentionDays, &site.CreatedAt); err != nil {
			return nil, err
		}
		sites = append(sites, site)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read site rows: %w", err)
	}

	return sites, nil
}

func (s *Store) ListAccessibleSitesForTakeout(ctx context.Context, userID uuid.UUID) ([]api.Site, error) {
	instanceRole, err := s.GetInstanceRole(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("could not get instance role: %w", err)
	}
	defaultTenantID, err := s.GetDefaultTenantID(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not resolve default tenant: %w", err)
	}

	var rows *sql.Rows
	if instanceRole.HasPermission(auth.PermInstanceViewAllSites) {
		rows, err = s.db.QueryContext(ctx, `
			SELECT s.id, s.user_id, s.domain, s.data_retention_days, s.created_at
			FROM sites s
			LEFT JOIN site_tenants st ON st.site_id = s.id
			LEFT JOIN tenant_archives ta ON ta.tenant_id = COALESCE(st.tenant_id, ?)
			WHERE ta.tenant_id IS NULL
			ORDER BY s.created_at DESC
		`, defaultTenantID)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT DISTINCT s.id, s.user_id, s.domain, s.data_retention_days, s.created_at
			FROM sites s
			LEFT JOIN site_tenants st ON st.site_id = s.id
			LEFT JOIN tenant_archives ta ON ta.tenant_id = COALESCE(st.tenant_id, ?)
			LEFT JOIN site_members sm ON sm.site_id = s.id AND sm.user_id = ?
			LEFT JOIN tenant_members tm ON tm.tenant_id = COALESCE(st.tenant_id, ?)
				AND tm.user_id = ?
				AND LOWER(TRIM(tm.role)) IN (?, ?)
			WHERE ta.tenant_id IS NULL
				AND (s.user_id = ? OR sm.user_id IS NOT NULL OR tm.user_id IS NOT NULL)
			ORDER BY s.created_at DESC
		`,
			defaultTenantID,
			userID,
			defaultTenantID,
			userID,
			TenantRoleOwner,
			TenantRoleAdmin,
			userID,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("could not list takeout sites: %w", err)
	}
	defer rows.Close()

	sites := make([]api.Site, 0)
	for rows.Next() {
		var site api.Site
		if err := rows.Scan(&site.ID, &site.UserID, &site.Domain, &site.DataRetentionDays, &site.CreatedAt); err != nil {
			return nil, fmt.Errorf("could not scan takeout site: %w", err)
		}
		sites = append(sites, site)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("could not read takeout sites: %w", err)
	}

	return sites, nil
}

func (s *Store) ListSitesForTenant(ctx context.Context, tenantID uuid.UUID) ([]api.Site, error) {
	defaultTenantID, err := s.GetDefaultTenantID(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not resolve default tenant: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT s.id, s.user_id, s.domain, s.data_retention_days, s.retention_synced_from_plan, s.created_at
		FROM sites s
		LEFT JOIN site_tenants st ON st.site_id = s.id
		LEFT JOIN tenant_archives ta ON ta.tenant_id = COALESCE(st.tenant_id, ?)
		WHERE COALESCE(st.tenant_id, ?) = ?
			AND ta.tenant_id IS NULL
		ORDER BY s.created_at DESC
	`, defaultTenantID, defaultTenantID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("could not list tenant sites: %w", err)
	}
	defer rows.Close()

	sites := make([]api.Site, 0)
	for rows.Next() {
		var site api.Site
		if err := rows.Scan(&site.ID, &site.UserID, &site.Domain, &site.DataRetentionDays, &site.RetentionSyncedFromPlan, &site.CreatedAt); err != nil {
			return nil, fmt.Errorf("could not scan tenant site: %w", err)
		}
		sites = append(sites, site)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("could not read tenant sites: %w", err)
	}

	return sites, nil
}

// UpdateSiteRetention updates a site's retention policy on behalf of its
// owning user. syncedFromPlan should be false for user-initiated changes
// (protects the value from being raised automatically by the plan-based
// retention sync) and true when applying a deployment-wide default that
// remains eligible for plan-based sync.
func (s *Store) UpdateSiteRetention(ctx context.Context, siteID uuid.UUID, userID uuid.UUID, days int, syncedFromPlan bool) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE sites SET data_retention_days = ?, retention_synced_from_plan = ? WHERE id = ? AND user_id = ?",
		days, syncedFromPlan, siteID, userID,
	)
	if err != nil {
		return fmt.Errorf("could not update site retention: %w", err)
	}
	return nil
}

// SetSiteRetentionDaysSystem updates a site's retention policy without a
// user-ownership check. Used only by the plan-entitlement retention sync
// (internal/database/tenant_store_manager_retention.go); user-initiated
// requests must keep going through UpdateSiteRetention.
func (s *Store) SetSiteRetentionDaysSystem(ctx context.Context, siteID uuid.UUID, days int, syncedFromPlan bool) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE sites SET data_retention_days = ?, retention_synced_from_plan = ? WHERE id = ?",
		days, syncedFromPlan, siteID,
	)
	if err != nil {
		return fmt.Errorf("could not set site retention: %w", err)
	}
	return nil
}

// listSiteFKReferences discovers every table column that declares a foreign
// key on sites(id), so domain updates keep working when new site-scoped
// tables are added.
func listSiteFKReferences(ctx context.Context, q queryer) ([]fkEdge, error) {
	edges, err := listFKEdges(ctx, q)
	if err != nil {
		return nil, err
	}
	refs := make([]fkEdge, 0, len(edges))
	for _, edge := range edges {
		if edge.referencedTable != "sites" || edge.table == "sites" {
			continue
		}
		if !isSafeIdentifier(edge.table) || !isSafeIdentifier(edge.column) {
			continue
		}
		refs = append(refs, edge)
	}
	return refs, nil
}

func moveSiteForeignKeys(ctx context.Context, tx *sql.Tx, refs []fkEdge, fromSiteID, toSiteID uuid.UUID) error {
	for _, ref := range refs {
		// #nosec G201 -- table and column names come from duckdb_constraints and pass isSafeIdentifier.
		query := fmt.Sprintf("UPDATE %s SET %s = ? WHERE %s = ?", ref.table, ref.column, ref.column)
		if _, err := tx.ExecContext(ctx, query, toSiteID, fromSiteID); err != nil {
			return fmt.Errorf("could not update site foreign key %s.%s: %w", ref.table, ref.column, err)
		}
	}
	return nil
}

// UpdateSiteDomain renames a site's tracked domain. Callers must normalize
// and validate the domain and enforce authorization before calling.
//
// DuckDB rewrites the whole sites row when the unique-indexed domain column
// changes and rejects the rewrite while other tables hold foreign keys on the
// site. Mirror the shadow-row sequence from UpdateUserProfile: park the
// references on a shadow site, update the domain, then move them back.
func (s *Store) UpdateSiteDomain(ctx context.Context, siteID uuid.UUID, domain string) error {
	var userID uuid.UUID
	var previousDomain string
	var createdAt time.Time
	if err := s.db.QueryRowContext(ctx, "SELECT user_id, domain, created_at FROM sites WHERE id = ?", siteID).Scan(&userID, &previousDomain, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("site %s not found", siteID)
		}
		return fmt.Errorf("could not load site for domain update: %w", err)
	}
	defer func() {
		s.invalidateSiteDomain(previousDomain)
		s.invalidateSiteDomain(domain)
	}()

	refs, err := listSiteFKReferences(ctx, s.db)
	if err != nil {
		return err
	}

	shadowSiteID := uuid.New()
	shadowDomain := fmt.Sprintf("__shadow_%s.hitkeep.invalid", strings.ReplaceAll(shadowSiteID.String(), "-", ""))

	if err := runStoreTx(ctx, s.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO sites (id, user_id, domain, created_at) VALUES (?, ?, ?, ?)",
			shadowSiteID, userID, shadowDomain, createdAt,
		); err != nil {
			return fmt.Errorf("could not create shadow site for domain update: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	deleteShadow := func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM sites WHERE id = ?", shadowSiteID); err != nil {
			return fmt.Errorf("could not cleanup shadow site for domain update: %w", err)
		}
		return nil
	}

	if err := runStoreTx(ctx, s.db, func(tx *sql.Tx) error {
		return moveSiteForeignKeys(ctx, tx, refs, siteID, shadowSiteID)
	}); err != nil {
		_ = runStoreTx(ctx, s.db, func(tx *sql.Tx) error {
			return moveSiteForeignKeys(ctx, tx, refs, shadowSiteID, siteID)
		})
		_ = runStoreTx(ctx, s.db, deleteShadow)
		return err
	}

	if err := runStoreTx(ctx, s.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "UPDATE sites SET domain = ? WHERE id = ?", domain, siteID); err != nil {
			return fmt.Errorf("could not update site domain: %w", err)
		}
		return moveSiteForeignKeys(ctx, tx, refs, shadowSiteID, siteID)
	}); err != nil {
		// Best-effort rollback to keep references on the original site if the update sequence fails.
		_ = runStoreTx(ctx, s.db, func(tx *sql.Tx) error {
			return moveSiteForeignKeys(ctx, tx, refs, shadowSiteID, siteID)
		})
		_ = runStoreTx(ctx, s.db, deleteShadow)
		return err
	}

	// DuckDB's foreign key check still sees the just-moved references inside
	// the same transaction, so the shadow row is removed on its own.
	return runStoreTx(ctx, s.db, deleteShadow)
}

func (s *Store) UpsertSiteMirror(ctx context.Context, site *api.Site) error {
	if site == nil {
		return fmt.Errorf("site is required")
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sites (id, domain, data_retention_days)
		VALUES (?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			domain = excluded.domain,
			data_retention_days = excluded.data_retention_days`,
		site.ID, site.Domain, site.DataRetentionDays,
	)
	if err != nil {
		return fmt.Errorf("could not upsert site mirror: %w", err)
	}
	return nil
}

func (s *Store) ListAllSites(ctx context.Context) ([]api.Site, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT s.id, s.user_id, s.domain, s.created_at, COALESCE(u.email, '') AS owner_email
		 FROM sites s
		 LEFT JOIN users u ON u.id = s.user_id
		 ORDER BY s.created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sites := []api.Site{}
	for rows.Next() {
		var site api.Site
		if err := rows.Scan(&site.ID, &site.UserID, &site.Domain, &site.CreatedAt, &site.OwnerEmail); err != nil {
			return nil, err
		}
		sites = append(sites, site)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read all site rows: %w", err)
	}

	return sites, nil
}

func (s *Store) DeleteSite(ctx context.Context, siteID uuid.UUID) error {
	// Best-effort domain read for cache invalidation; tenant-store mirrors
	// carry a reduced sites schema, so only the domain column is portable.
	var domain string
	_ = s.db.QueryRowContext(ctx, "SELECT domain FROM sites WHERE id = ?", siteID).Scan(&domain)

	if err := s.deleteSiteData(ctx, siteID); err != nil {
		return err
	}
	if err := s.deleteSiteRow(ctx, siteID); err != nil {
		return err
	}
	s.invalidateSiteTenantID(siteID)
	s.invalidateSiteDomain(domain)
	return nil
}

// DeleteSiteWithWebhookEvent coordinates DuckDB's required multi-transaction
// site cleanup with a durable staged outbox. The final deliveries cannot become
// dispatchable until the site deletion succeeds, while a recovery sweep can
// finish materializing them after a process interruption.
func (s *Store) DeleteSiteWithWebhookEvent(ctx context.Context, siteID uuid.UUID, event WebhookEventInput) ([]WebhookDeliveryJob, error) {
	event.SiteID = &siteID
	event.PreserveAfterSiteDeletion = true
	now := time.Now().UTC()
	prepared, eventBody, _, err := prepareWebhookEventInput(event, now)
	if err != nil {
		return nil, err
	}
	subscribers, err := s.enabledWebhookSubscribers(ctx, &siteID, event.TargetWebhookID, event.EventType)
	if err != nil {
		return nil, err
	}
	if len(subscribers) == 0 {
		return []WebhookDeliveryJob{}, s.DeleteSite(ctx, siteID)
	}

	if err := s.stageSiteDeletionWebhookEvent(ctx, siteID, prepared, eventBody, subscribers, now); err != nil {
		return nil, err
	}
	if err := s.DeleteSite(ctx, siteID); err != nil {
		_ = s.cancelStagedSiteDeletionWebhookEvent(ctx, prepared.ID)
		return nil, err
	}
	jobs, err := s.CommitStagedSiteDeletionWebhookEvent(ctx, prepared.ID, now)
	if err != nil {
		hklog.LoggerFromContextOr(ctx, s.logger).Warn("Site deleted with final webhook materialization deferred", "error", err, "site_id", siteID, "event_id", prepared.ID)
		return []WebhookDeliveryJob{}, nil
	}
	return jobs, nil
}

func (s *Store) deleteSiteData(ctx context.Context, siteID uuid.UUID) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := deleteSiteChildren(ctx, tx, siteID); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("could not commit transaction: %w", err)
	}
	return nil
}

func (s *Store) deleteSiteRow(ctx context.Context, siteID uuid.UUID) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM sites WHERE id = ?", siteID); err != nil {
		refs, refErr := findSiteReferences(ctx, s.db, siteID, s.logger)
		if refErr != nil {
			return fmt.Errorf("could not delete site: %w (failed to resolve references: %v)", err, refErr)
		}
		if len(refs) > 0 {
			return fmt.Errorf("could not delete site: %w (still referenced by: %s)", err, strings.Join(refs, ", "))
		}
		return fmt.Errorf("could not delete site: %w", err)
	}
	return nil
}
