package database

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func newMigratedStore(t *testing.T) *Store {
	t.Helper()
	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return store
}

func planStepIndex(steps []scopedDeleteStep, table string) int {
	for i, step := range steps {
		if step.table == table {
			return i
		}
	}
	return -1
}

func TestBuildScopedDeletePlanCoversAllSiteScopedTables(t *testing.T) {
	ctx := context.Background()
	store := newMigratedStore(t)

	steps, err := buildScopedDeletePlan(ctx, store.DB(), siteDeleteSpec)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}

	siteTables, err := listSiteIDTables(ctx, store.DB())
	if err != nil {
		t.Fatalf("list site tables: %v", err)
	}
	for _, table := range siteTables {
		if table == "sites" {
			continue
		}
		if planStepIndex(steps, table) < 0 {
			t.Errorf("expected plan to cover site-scoped table %s", table)
		}
	}
	if planStepIndex(steps, "sites") >= 0 {
		t.Error("plan must not delete the sites root table")
	}

	// site_import_files carries no site_id column and no declared FK; it must
	// be deleted through its parent before site_imports rows disappear.
	fileIdx := planStepIndex(steps, "site_import_files")
	importIdx := planStepIndex(steps, "site_imports")
	if fileIdx < 0 || importIdx < 0 || fileIdx > importIdx {
		t.Errorf("expected site_import_files (%d) before site_imports (%d)", fileIdx, importIdx)
	}
	fileStep := steps[fileIdx]
	if !strings.Contains(fileStep.query, "IN (SELECT id FROM site_imports WHERE site_id = ?)") {
		t.Errorf("unexpected derived delete query: %s", fileStep.query)
	}

	// Declared FK children must be deleted before their parents.
	qrIdx := planStepIndex(steps, "qr_codes")
	for _, child := range []string{"qr_code_assets", "qr_code_share_links"} {
		childIdx := planStepIndex(steps, child)
		if childIdx < 0 || qrIdx < 0 || childIdx > qrIdx {
			t.Errorf("expected %s (%d) before qr_codes (%d)", child, childIdx, qrIdx)
		}
	}
}

func TestBuildScopedDeletePlanFailsOnUnhandledRootReference(t *testing.T) {
	ctx := context.Background()
	store := newMigratedStore(t)

	if _, err := store.DB().ExecContext(ctx, `
		CREATE TABLE rogue_site_refs (
			id UUID PRIMARY KEY,
			origin_site UUID NOT NULL REFERENCES sites(id)
		)`); err != nil {
		t.Fatalf("create rogue table: %v", err)
	}

	_, err := buildScopedDeletePlan(ctx, store.DB(), siteDeleteSpec)
	if err == nil || !strings.Contains(err.Error(), "rogue_site_refs") {
		t.Fatalf("expected plan to fail on rogue_site_refs, got err=%v", err)
	}

	spec := siteDeleteSpec
	spec.policyTables = append([]string{"rogue_site_refs"}, siteDeleteSpec.policyTables...)
	steps, err := buildScopedDeletePlan(ctx, store.DB(), spec)
	if err != nil {
		t.Fatalf("build plan with policy table: %v", err)
	}
	if planStepIndex(steps, "rogue_site_refs") >= 0 {
		t.Error("policy tables must be excluded from the generated plan")
	}
}

func TestScopedDeletePlanExecutesForSeededSite(t *testing.T) {
	ctx := context.Background()
	store := newMigratedStore(t)

	userID, err := store.CreateUser(ctx, "fk-cleanup@example.com", "hashed_secret")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "fk-cleanup.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	importID := uuid.New()
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO site_imports (id, site_id, provider, status, source_hash, bytes_total, bytes_received, rows_scanned, rows_imported, created_by, created_at, updated_at) VALUES (?, ?, 'plausible', ?, 'h', 1, 1, 0, 0, ?, now(), now())",
		importID, site.ID, ImportStatusCompleted, userID,
	); err != nil {
		t.Fatalf("insert import: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO site_import_files (import_id, file_id, filename, relative_path, size_bytes, status, created_at, updated_at) VALUES (?, ?, 'f.csv', 'f.csv', 1, 'uploaded', now(), now())",
		importID, uuid.New(),
	); err != nil {
		t.Fatalf("insert import file: %v", err)
	}

	tx, err := store.DB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := deleteSiteChildren(ctx, tx, site.ID); err != nil {
		t.Fatalf("delete site children: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	var fileCount int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM site_import_files WHERE import_id = ?", importID).Scan(&fileCount); err != nil {
		t.Fatalf("count import files: %v", err)
	}
	if fileCount != 0 {
		t.Fatalf("expected import files deleted, got %d", fileCount)
	}
	if _, err := store.DB().ExecContext(ctx, "DELETE FROM sites WHERE id = ?", site.ID); err != nil {
		t.Fatalf("expected site row deletable after children cleanup: %v", err)
	}
}

func TestListScopedCopyTablesMatchesTenantAnalyticsSchema(t *testing.T) {
	ctx := context.Background()
	store := newSharedTestStore(t)
	control := newControlTestStore(t)
	mgr := NewTenantStoreManager(control, t.TempDir(), nil)
	t.Cleanup(func() { _ = mgr.Close() })

	tenantID := newManagerTestTenant(t, control, "Copy Tables Team")
	tenantStore, err := mgr.ForTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("open tenant store: %v", err)
	}

	tables, err := listScopedCopyTables(ctx, store.DB(), tenantStore.DB(), "site_id", "sites", siteExtraEdges)
	if err != nil {
		t.Fatalf("list copy tables: %v", err)
	}

	got := map[string]struct{}{}
	for _, table := range tables {
		got[table] = struct{}{}
	}
	if _, ok := got["sites"]; ok {
		t.Error("copy tables must not include the sites root table")
	}

	// Every site-scoped base table of the tenant schema must be covered, so
	// new tenant analytics tables are transferred without registration.
	tenantSiteTables, err := listSiteIDTables(ctx, tenantStore.DB())
	if err != nil {
		t.Fatalf("list tenant site tables: %v", err)
	}
	tenantBaseTables, err := listTables(ctx, tenantStore.DB())
	if err != nil {
		t.Fatalf("list tenant base tables: %v", err)
	}
	expected := 0
	for _, table := range tenantSiteTables {
		if table == "sites" {
			continue
		}
		if _, ok := tenantBaseTables[table]; !ok {
			continue
		}
		expected++
		if _, ok := got[table]; !ok {
			t.Errorf("expected copy tables to include tenant analytics table %s", table)
		}
	}
	if len(tables) != expected {
		t.Errorf("expected %d copy tables, got %d (%v)", expected, len(tables), tables)
	}
}

func TestListScopedCopyTablesUsesExtraEdges(t *testing.T) {
	ctx := context.Background()
	newCopyStore := func() *Store {
		t.Helper()
		store := NewStore(":memory:")
		if err := store.Connect(); err != nil {
			t.Fatalf("connect: %v", err)
		}
		t.Cleanup(func() { _ = store.Close() })
		if _, err := store.DB().ExecContext(ctx, `
			CREATE TABLE sites (id UUID PRIMARY KEY);
			CREATE TABLE qr_codes (id UUID PRIMARY KEY, site_id UUID NOT NULL);
			CREATE TABLE qr_code_assets (qr_code_id UUID PRIMARY KEY, site_id UUID NOT NULL);
			CREATE TABLE qr_code_share_links (id UUID PRIMARY KEY, qr_code_id UUID NOT NULL, site_id UUID NOT NULL);
		`); err != nil {
			t.Fatalf("create copy schema: %v", err)
		}
		return store
	}

	source := newCopyStore()
	destination := newCopyStore()
	tables, err := listScopedCopyTables(ctx, source.DB(), destination.DB(), "site_id", "sites", siteExtraEdges)
	if err != nil {
		t.Fatalf("list copy tables: %v", err)
	}

	qrIdx := slices.Index(tables, "qr_codes")
	for _, child := range []string{"qr_code_assets", "qr_code_share_links"} {
		childIdx := slices.Index(tables, child)
		if qrIdx < 0 || childIdx < 0 || qrIdx > childIdx {
			t.Errorf("expected qr_codes (%d) before %s (%d)", qrIdx, child, childIdx)
		}
	}
}

func TestMigrateFailsFastOnUnhandledSiteReference(t *testing.T) {
	ctx := context.Background()
	store := newMigratedStore(t)

	// Re-running Migrate on a healthy, up-to-date schema must keep passing.
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("re-migrate healthy schema: %v", err)
	}

	if _, err := store.DB().ExecContext(ctx, `
		CREATE TABLE rogue_migrate_refs (
			id UUID PRIMARY KEY,
			origin_site UUID NOT NULL REFERENCES sites(id)
		)`); err != nil {
		t.Fatalf("create rogue table: %v", err)
	}

	err := store.Migrate(ctx)
	if err == nil || !strings.Contains(err.Error(), "rogue_migrate_refs") {
		t.Fatalf("expected migrate to fail fast on rogue_migrate_refs, got err=%v", err)
	}
}

func TestMigrateTenantFailsFastOnUnhandledSiteReference(t *testing.T) {
	ctx := context.Background()
	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.MigrateTenant(ctx); err != nil {
		t.Fatalf("migrate tenant: %v", err)
	}
	if err := store.MigrateTenant(ctx); err != nil {
		t.Fatalf("re-migrate healthy tenant schema: %v", err)
	}

	if _, err := store.DB().ExecContext(ctx, `
		CREATE TABLE rogue_tenant_refs (
			id UUID PRIMARY KEY,
			origin_site UUID NOT NULL REFERENCES sites(id)
		)`); err != nil {
		t.Fatalf("create rogue table: %v", err)
	}

	err := store.MigrateTenant(ctx)
	if err == nil || !strings.Contains(err.Error(), "rogue_tenant_refs") {
		t.Fatalf("expected tenant migrate to fail fast on rogue_tenant_refs, got err=%v", err)
	}
}

func TestListScopedCopyTablesRejectsIdentityEntangledPairs(t *testing.T) {
	ctx := context.Background()
	source := newMigratedStore(t)
	destination := newMigratedStore(t)

	// Two full shared schemas intersect on identity-entangled tables such as
	// site_tenants (FK to tenants); a derived copy plan must refuse them
	// rather than produce foreign-key violations.
	_, err := listScopedCopyTables(ctx, source.DB(), destination.DB(), "site_id", "sites", siteExtraEdges)
	if err == nil || !strings.Contains(err.Error(), "references") {
		t.Fatalf("expected copy plan to reject identity-entangled schema pair, got err=%v", err)
	}
}
