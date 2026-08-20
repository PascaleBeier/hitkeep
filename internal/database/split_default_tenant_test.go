package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/assetstore"
)

// defaultTenantSplitFixtureTables is the explicit migration acceptance
// registry. The schema-contract test below fails whenever a site-scoped tenant
// table is added without also extending the split fixture.
var defaultTenantSplitFixtureTables = []string{
	"ai_fetches",
	"events",
	"funnel_rollups_daily",
	"funnel_rollups_hourly",
	"funnel_rollups_monthly",
	"funnels",
	"goal_rollups_daily",
	"goal_rollups_hourly",
	"goal_rollups_monthly",
	"goals",
	"hit_rollups_daily",
	"hit_rollups_hourly",
	"hit_rollups_monthly",
	"hits",
	"imported_dimension_daily",
	"imported_event_daily",
	"imported_event_dimensions_daily",
	"imported_event_properties_daily",
	"imported_traffic_daily",
	"qr_code_opens",
	"rollup_dirty_buckets",
	"search_console_facts",
	"session_rollups_daily",
	"session_rollups_hourly",
	"session_rollups_monthly",
	"site_activity_hourly_counts",
	"site_activity_summary",
	"web_vitals",
}

func TestDefaultTenantSplitFixtureRegistryMatchesTenantSchema(t *testing.T) {
	ctx := context.Background()
	store := NewStore(filepath.Join(t.TempDir(), "tenant.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect tenant fixture schema: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.MigrateTenant(ctx); err != nil {
		t.Fatalf("migrate tenant fixture schema: %v", err)
	}
	rows, err := store.DB().QueryContext(ctx, `
		SELECT DISTINCT c.table_name
		FROM information_schema.columns c
		JOIN information_schema.tables t
		  ON t.table_catalog = c.table_catalog
		 AND t.table_schema = c.table_schema
		 AND t.table_name = c.table_name
		WHERE c.table_catalog = current_database()
		  AND c.table_schema = 'main'
		  AND c.column_name = 'site_id'
		  AND t.table_type = 'BASE TABLE'
		ORDER BY c.table_name`)
	if err != nil {
		t.Fatalf("list site-scoped tenant tables: %v", err)
	}
	defer rows.Close()
	var actual []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			t.Fatalf("scan tenant table: %v", err)
		}
		actual = append(actual, table)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate tenant tables: %v", err)
	}
	expected := slices.Clone(defaultTenantSplitFixtureTables)
	slices.Sort(expected)
	if !slices.Equal(actual, expected) {
		t.Fatalf("split fixture registry is stale\nactual:   %v\nexpected: %v", actual, expected)
	}
}

func TestDefaultTenantSplitSpaceRequirementsKeepControlHeadroomBounded(t *testing.T) {
	sharedPath := filepath.Join(t.TempDir(), "hitkeep.db")
	const sharedSize = int64(64 << 20)
	file, err := os.Create(sharedPath)
	if err != nil {
		t.Fatalf("create shared database fixture: %v", err)
	}
	if err := file.Truncate(sharedSize); err != nil {
		_ = file.Close()
		t.Fatalf("size shared database fixture: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close shared database fixture: %v", err)
	}

	dataRequired, controlRequired, err := defaultTenantSplitSpaceRequirements(sharedPath)
	if err != nil {
		t.Fatalf("calculate split space requirements: %v", err)
	}
	if want := sharedSize + defaultTenantSplitHeadroom; dataRequired != want {
		t.Fatalf("data space requirement = %d, want %d", dataRequired, want)
	}
	if controlRequired != defaultTenantSplitHeadroom {
		t.Fatalf("control space requirement = %d, want %d", controlRequired, defaultTenantSplitHeadroom)
	}
}

func TestRunDefaultTenantSplitMovesDefaultScopedAnalytics(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sharedPath := filepath.Join(root, "hitkeep.db")
	dataPath := filepath.Join(root, "data")

	shared := NewStore(sharedPath)
	if err := shared.Connect(); err != nil {
		t.Fatalf("connect shared: %v", err)
	}
	if err := shared.Migrate(ctx); err != nil {
		_ = shared.Close()
		t.Fatalf("migrate shared: %v", err)
	}
	userID, err := shared.CreateUser(ctx, "split@example.com", "hashed")
	if err != nil {
		_ = shared.Close()
		t.Fatalf("create user: %v", err)
	}
	defaultSite, err := shared.CreateSite(ctx, userID, "default.split.test")
	if err != nil {
		_ = shared.Close()
		t.Fatalf("create default site: %v", err)
	}
	qr, _, err := shared.CreateQRCode(ctx, defaultSite.ID, userID, api.QRCodeCreateRequest{
		Name: "Migration poster", DestinationURL: "https://example.test/landing",
		UTMSource: "poster", UTMMedium: "print", UTMCampaign: "migration",
		CustomParams: map[string]string{"placement": "station"},
		Style:        map[string]any{"dots": "rounded"},
	})
	if err != nil {
		_ = shared.Close()
		t.Fatalf("create control-plane QR definition: %v", err)
	}
	qrAssetBytes := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a}
	qrAssets := assetstore.New(dataPath)
	qrAssetKey, err := qrAssets.PutQRCodeAsset(defaultSite.ID, qr.ID, "fixture-checksum", "fixture.png", "image/png", qrAssetBytes)
	if err != nil {
		_ = shared.Close()
		t.Fatalf("write control-plane QR asset file: %v", err)
	}
	if _, err := shared.UpsertQRCodeAsset(ctx, api.QRCodeAsset{
		QRCodeID: qr.ID, SiteID: defaultSite.ID, Filename: "fixture.png", ContentType: "image/png",
		ByteSize: int64(len(qrAssetBytes)), Width: 1, Height: 1, Checksum: "fixture-checksum", StorageKey: qrAssetKey,
	}); err != nil {
		_ = shared.Close()
		t.Fatalf("create control-plane QR asset row: %v", err)
	}
	if _, _, err := shared.CreateQRCodeShareLink(ctx, defaultSite.ID, qr.ID, userID); err != nil {
		_ = shared.Close()
		t.Fatalf("create control-plane QR share link: %v", err)
	}
	otherTenant := uuid.New()
	if _, err := shared.DB().ExecContext(ctx,
		"INSERT INTO tenants (id, name, is_default, created_at) VALUES (?, ?, FALSE, ?)",
		otherTenant, "Other Tenant", time.Now().UTC(),
	); err != nil {
		_ = shared.Close()
		t.Fatalf("create other tenant: %v", err)
	}
	otherSite, err := shared.CreateSite(ctx, userID, "other.split.test")
	if err != nil {
		_ = shared.Close()
		t.Fatalf("create other site: %v", err)
	}
	if _, err := shared.DB().ExecContext(ctx,
		"UPDATE site_tenants SET tenant_id = ?, created_at = ? WHERE site_id = ?",
		otherTenant, time.Now().UTC(), otherSite.ID,
	); err != nil {
		_ = shared.Close()
		t.Fatalf("map other site: %v", err)
	}
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	qrCodeID := qr.ID
	if err := shared.CreateHitsBulk(ctx, []*api.Hit{
		{
			ID: uuid.New(), SiteID: defaultSite.ID, SessionID: uuid.New(), PageID: uuid.New(), Timestamp: now, Path: "/default",
			Hostname: new("analytics.example"), Referrer: new("https://referrer.example/source"), UserAgent: new("fixture-agent"),
			ViewportWidth: new(1440), ViewportHeight: new(900), ScreenWidth: new(1920), ScreenHeight: new(1080),
			Language: new("de-DE"), CountryCode: new("DE"), Region: new("BE"), City: new("Berlin"), Provider: new("fixture-network"),
			ASN: new(64500), ASNOrg: new("Fixture ASN"), UTMSource: new("qr"), UTMMedium: new("offline"),
			UTMCampaign: new("launch"), UTMTerm: new("tenant"), UTMContent: new("poster"), QRCodeID: &qrCodeID, IsUnique: new(true),
		},
		{ID: uuid.New(), SiteID: defaultSite.ID, SessionID: uuid.New(), PageID: uuid.New(), Timestamp: now.Add(time.Minute), Path: "/optional-nulls"},
		{ID: uuid.New(), SiteID: otherSite.ID, SessionID: uuid.New(), PageID: uuid.New(), Timestamp: now, Path: "/other"},
	}); err != nil {
		_ = shared.Close()
		t.Fatalf("create hits: %v", err)
	}
	defaultTenantID, err := shared.GetDefaultTenantID(ctx)
	if err != nil {
		_ = shared.Close()
		t.Fatalf("resolve default tenant: %v", err)
	}
	if err := shared.CreateQRCodeOpen(ctx, &api.QRCodeOpen{
		ID: uuid.New(), SiteID: defaultSite.ID, QRCodeID: qrCodeID, Timestamp: now,
		Referrer: new("https://offline.example/poster"), UserAgent: new("qr-fixture-agent"),
		CountryCode: new("DE"), Region: new("BE"), City: new("Berlin"),
		Provider: new("fixture-network"), ASN: new(64500), ASNOrg: new("Fixture ASN"),
	}); err != nil {
		_ = shared.Close()
		t.Fatalf("create QR-open fixture: %v", err)
	}
	seedSplitFixtureTables(t, ctx, shared, defaultSite.ID, defaultTenantID, now, qrCodeID)
	seedSplitFixtureTables(t, ctx, shared, otherSite.ID, otherTenant, now.Add(24*time.Hour), uuid.New())
	if err := shared.CreateEvent(ctx, &api.Event{
		ID: uuid.New(), SiteID: defaultSite.ID, SessionID: uuid.New(), Name: "purchase",
		Properties: map[string]any{
			"transaction_id": "fixture-order", "value": 42.5, "currency": "EUR", "coupon": "QR",
			"items": []any{map[string]any{"item_id": "fixture-sku", "item_name": "Fixture", "quantity": 2, "price": 21.25}},
		},
		Timestamp: now.Add(2 * time.Minute),
	}); err != nil {
		_ = shared.Close()
		t.Fatalf("create ecommerce fixture event: %v", err)
	}
	expectedPath := filepath.Join(root, "expected-default-tenant.db")
	snapshotSplitFixtureRows(t, ctx, shared, expectedPath, defaultSite.ID)
	strayDir := filepath.Join(dataPath, "tenants", defaultTenantID.String())
	if err := os.MkdirAll(strayDir, 0o755); err != nil {
		_ = shared.Close()
		t.Fatalf("create tenant directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(strayDir, "hitkeep.db.split-work"), []byte("stale"), 0o600); err != nil {
		_ = shared.Close()
		t.Fatalf("write stale work artifact: %v", err)
	}
	if err := os.WriteFile(filepath.Join(strayDir, "hitkeep.db.split-work.wal"), []byte("stale"), 0o600); err != nil {
		_ = shared.Close()
		t.Fatalf("write stale WAL artifact: %v", err)
	}
	if err := os.WriteFile(filepath.Join(strayDir, "hitkeep.db"), []byte("untrusted"), 0o600); err != nil {
		_ = shared.Close()
		t.Fatalf("write stray final artifact: %v", err)
	}
	if err := shared.Close(); err != nil {
		t.Fatalf("close shared before split: %v", err)
	}

	if err := RunDefaultTenantSplit(ctx, sharedPath, dataPath); err != nil {
		t.Fatalf("run split: %v", err)
	}
	if _, err := os.Stat(filepath.Join(strayDir, "hitkeep.db.split-work")); !os.IsNotExist(err) {
		t.Fatalf("expected stale split work artifact to be removed, err=%v", err)
	}
	entries, err := os.ReadDir(strayDir)
	if err != nil {
		t.Fatalf("read tenant directory after split: %v", err)
	}
	strayFound := false
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "hitkeep.db.stray-") {
			strayFound = true
			break
		}
	}
	if !strayFound {
		t.Fatal("expected unsentinelled final tenant file to be preserved as a stray")
	}

	control := NewStore(sharedPath)
	if err := control.Connect(); err != nil {
		t.Fatalf("reopen control: %v", err)
	}
	t.Cleanup(func() { _ = control.Close() })
	if err := control.Migrate(ctx); err != nil {
		t.Fatalf("migrate control after split: %v", err)
	}
	if applied, err := control.HasDefaultTenantSplit(ctx); err != nil || !applied {
		t.Fatalf("expected split marker, applied=%v err=%v", applied, err)
	}
	if storedQR, err := control.GetQRCode(ctx, defaultSite.ID, qrCodeID); err != nil || storedQR == nil || storedQR.Name != "Migration poster" {
		t.Fatalf("control-plane QR definition changed: qr=%+v err=%v", storedQR, err)
	}
	if storedAsset, err := control.GetQRCodeAsset(ctx, defaultSite.ID, qrCodeID); err != nil || storedAsset == nil || storedAsset.StorageKey != qrAssetKey {
		t.Fatalf("control-plane QR asset row changed: asset=%+v err=%v", storedAsset, err)
	}
	if links, err := control.ListQRCodeShareLinks(ctx, defaultSite.ID, qrCodeID); err != nil || len(links) != 1 {
		t.Fatalf("control-plane QR share links changed: count=%d err=%v", len(links), err)
	}
	assetFile, err := qrAssets.Open(qrAssetKey)
	if err != nil {
		t.Fatalf("open QR asset after split: %v", err)
	}
	assetAfter, readErr := os.ReadFile(assetFile.Name())
	_ = assetFile.Close()
	if readErr != nil || !slices.Equal(assetAfter, qrAssetBytes) {
		t.Fatalf("control-plane QR asset file changed: err=%v", readErr)
	}
	var defaultSharedHits int
	if err := control.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM hits WHERE site_id = ?", defaultSite.ID,
	).Scan(&defaultSharedHits); err != nil {
		t.Fatalf("count default shared hits: %v", err)
	}
	if defaultSharedHits != 0 {
		t.Fatalf("expected default shared hits to be removed, got %d", defaultSharedHits)
	}
	var defaultSharedActivity int
	if err := control.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM site_activity_summary WHERE site_id = ?", defaultSite.ID,
	).Scan(&defaultSharedActivity); err != nil {
		t.Fatalf("count default shared activity: %v", err)
	}
	if defaultSharedActivity != 0 {
		t.Fatalf("expected default shared activity to be removed, got %d", defaultSharedActivity)
	}
	var otherSharedHits int
	if err := control.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM hits WHERE site_id = ?", otherSite.ID,
	).Scan(&otherSharedHits); err != nil {
		t.Fatalf("count other shared hits: %v", err)
	}
	if otherSharedHits != 1 {
		t.Fatalf("expected non-default shared hit to remain, got %d", otherSharedHits)
	}
	for _, table := range defaultTenantSplitFixtureTables {
		var defaultRows, retainedRows int
		query := fmt.Sprintf(`
			SELECT
				COUNT(*) FILTER (WHERE site_id = ?),
				COUNT(*) FILTER (WHERE site_id = ?)
			FROM %s`, quoteDuckDBIdentifier(table))
		if err := control.DB().QueryRowContext(ctx, query, defaultSite.ID, otherSite.ID).Scan(&defaultRows, &retainedRows); err != nil {
			t.Fatalf("verify control fixture table %s: %v", table, err)
		}
		if defaultRows != 0 || retainedRows == 0 {
			t.Fatalf("unexpected control rows for %s: default=%d retained=%d", table, defaultRows, retainedRows)
		}
	}

	defaultID, err := control.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}
	tenantPath := filepath.Join(dataPath, "tenants", defaultID.String(), "hitkeep.db")
	tenant := NewStore(tenantPath)
	if err := tenant.Connect(); err != nil {
		t.Fatalf("open default tenant file: %v", err)
	}
	t.Cleanup(func() { _ = tenant.Close() })
	if err := tenant.MigrateTenant(ctx); err != nil {
		t.Fatalf("migrate default tenant file: %v", err)
	}
	assertSplitFixtureRowsEqual(t, ctx, expectedPath, tenantPath)
	var tenantHits int
	if err := tenant.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM hits WHERE site_id = ?", defaultSite.ID).Scan(&tenantHits); err != nil {
		t.Fatalf("count tenant hits: %v", err)
	}
	if tenantHits != 2 {
		t.Fatalf("expected two moved tenant hits, got %d", tenantHits)
	}
	var tenantActivity int
	if err := tenant.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM site_activity_summary WHERE site_id = ?", defaultSite.ID).Scan(&tenantActivity); err != nil {
		t.Fatalf("count tenant activity summary: %v", err)
	}
	if tenantActivity != 1 {
		t.Fatalf("expected one moved tenant activity summary, got %d", tenantActivity)
	}
	var tenantSites int
	if err := tenant.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM sites WHERE id = ?", defaultSite.ID).Scan(&tenantSites); err != nil {
		t.Fatalf("count tenant site mirror: %v", err)
	}
	if tenantSites != 1 {
		t.Fatalf("expected moved tenant site mirror, got %d", tenantSites)
	}

	if err := RunDefaultTenantSplit(ctx, sharedPath, dataPath); err != nil {
		t.Fatalf("rerun split: %v", err)
	}
	if err := tenant.Close(); err != nil {
		t.Fatalf("close tenant before missing-final recovery check: %v", err)
	}
	if err := control.Close(); err != nil {
		t.Fatalf("close control before missing-final recovery check: %v", err)
	}
	if err := os.Remove(tenantPath); err != nil {
		t.Fatalf("remove tenant file for recovery check: %v", err)
	}
	if err := RunDefaultTenantSplit(ctx, sharedPath, dataPath); err == nil {
		t.Fatal("expected split marker with missing tenant file to fail startup")
	}
}

func TestDefaultTenantSplitRestoresPreCompactBackupWhenTenantFileMissing(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sharedPath := filepath.Join(root, "hitkeep.db")
	dataPath := filepath.Join(root, "data")

	shared := NewStore(sharedPath)
	if err := shared.Connect(); err != nil {
		t.Fatalf("connect shared: %v", err)
	}
	if err := shared.Migrate(ctx); err != nil {
		_ = shared.Close()
		t.Fatalf("migrate shared: %v", err)
	}
	userID, err := shared.CreateUser(ctx, "split-backup-recovery@example.test", "hashed")
	if err != nil {
		_ = shared.Close()
		t.Fatalf("create user: %v", err)
	}
	site, err := shared.CreateSite(ctx, userID, "split-backup-recovery.example.test")
	if err != nil {
		_ = shared.Close()
		t.Fatalf("create site: %v", err)
	}
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	if err := shared.CreateHitsBulk(ctx, []*api.Hit{
		{ID: uuid.New(), SiteID: site.ID, SessionID: uuid.New(), PageID: uuid.New(), Timestamp: now, Path: "/recovered"},
	}); err != nil {
		_ = shared.Close()
		t.Fatalf("create hit: %v", err)
	}
	if err := shared.Close(); err != nil {
		t.Fatalf("close shared before split: %v", err)
	}

	// Abort immediately after the rewritten control file (with the split
	// marker) is published but before the pre-compaction backup is removed.
	abort := errors.New("abort after control publication")
	err = runDefaultTenantSplitWithFaults(ctx, sharedPath, dataPath, nil, nil, func(point string) error {
		if point == "after-control-target-rename" {
			return abort
		}
		return nil
	})
	if !errors.Is(err, abort) {
		t.Fatalf("expected aborted split, got %v", err)
	}
	if _, err := os.Stat(sharedPath + ".pre-compact"); err != nil {
		t.Fatalf("expected pre-compaction backup after aborted split: %v", err)
	}

	// Simulate a tenant data directory that was not on persistent storage,
	// e.g. a recreated Docker container whose HITKEEP_DATA_PATH was off-volume.
	if err := os.RemoveAll(dataPath); err != nil {
		t.Fatalf("remove tenant data directory: %v", err)
	}

	if err := RunDefaultTenantSplit(ctx, sharedPath, dataPath); err != nil {
		t.Fatalf("expected split to recover from pre-compaction backup: %v", err)
	}
	if _, err := os.Stat(sharedPath + ".pre-compact"); !os.IsNotExist(err) {
		t.Fatalf("expected pre-compaction backup to be removed after recovery, err=%v", err)
	}

	control := NewStore(sharedPath)
	if err := control.Connect(); err != nil {
		t.Fatalf("connect control after recovery: %v", err)
	}
	defer control.Close()
	if complete, err := control.DefaultTenantSplitComplete(ctx); err != nil || !complete {
		t.Fatalf("expected complete split markers after recovery, complete=%v err=%v", complete, err)
	}
	var controlHits int
	if err := control.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM hits WHERE site_id = ?", site.ID).Scan(&controlHits); err != nil {
		t.Fatalf("count control hits: %v", err)
	}
	if controlHits != 0 {
		t.Fatalf("expected control hits to be moved out, got %d", controlHits)
	}
	defaultID, err := control.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}
	tenantPath := filepath.Join(dataPath, "tenants", defaultID.String(), "hitkeep.db")
	tenant := NewStore(tenantPath)
	if err := tenant.Connect(); err != nil {
		t.Fatalf("open recovered tenant file: %v", err)
	}
	defer tenant.Close()
	var tenantHits int
	if err := tenant.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM hits WHERE site_id = ?", site.ID).Scan(&tenantHits); err != nil {
		t.Fatalf("count recovered tenant hits: %v", err)
	}
	if tenantHits != 1 {
		t.Fatalf("expected recovered tenant hit, got %d", tenantHits)
	}
}

func TestDefaultTenantSplitEmitsEveryPerTableFaultBoundary(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sharedPath := filepath.Join(root, "hitkeep.db")
	shared := NewStore(sharedPath)
	if err := shared.Connect(); err != nil {
		t.Fatal(err)
	}
	if err := shared.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	userID, err := shared.CreateUser(ctx, "split-boundary-registry@example.test", "hashed")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := shared.CreateSite(ctx, userID, "split-boundary-registry.example.test"); err != nil {
		t.Fatal(err)
	}
	if err := shared.Close(); err != nil {
		t.Fatal(err)
	}

	seen := make(map[string]bool)
	if err := runDefaultTenantSplitWithFaults(ctx, sharedPath, filepath.Join(root, "data"), nil, nil, func(point string) error {
		seen[point] = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, table := range defaultTenantSplitFixtureTables {
		for _, prefix := range []string{
			"after-copy:",
			"after-source-fingerprint:",
			"after-target-fingerprint:",
			"after-copy-checkpoint:",
			"after-delete:",
			"after-cleanup-fingerprint:",
		} {
			point := prefix + table
			if !seen[point] {
				t.Errorf("missing split fault boundary %q", point)
			}
		}
	}
}

type splitFixtureColumn struct {
	Name       string
	DataType   string
	Nullable   string
	DefaultSQL sql.NullString
}

func seedSplitFixtureTables(t *testing.T, ctx context.Context, store *Store, siteID, tenantID uuid.UUID, at time.Time, qrCodeID uuid.UUID) {
	t.Helper()
	goalID := uuid.New()
	funnelID := uuid.New()
	seedOrder := []string{"goals", "funnels"}
	for _, table := range defaultTenantSplitFixtureTables {
		if table != "hits" && table != "goals" && table != "funnels" {
			seedOrder = append(seedOrder, table)
		}
	}
	for _, table := range seedOrder {
		var existing int
		if err := store.DB().QueryRowContext(ctx,
			"SELECT COUNT(*) FROM "+quoteDuckDBIdentifier(table)+" WHERE site_id = ?", siteID).Scan(&existing); err != nil {
			t.Fatalf("count existing split fixture table %s: %v", table, err)
		}
		if existing > 0 {
			continue
		}
		columns, err := splitFixtureColumns(ctx, store, table)
		if err != nil {
			t.Fatalf("inspect fixture table %s: %v", table, err)
		}
		var names, expressions []string
		var args []any
		for _, column := range columns {
			if column.Nullable == "YES" {
				// Optional fields are deliberately left NULL. Hits and the explicit
				// ecommerce event above provide populated optional-field coverage.
				names = append(names, quoteDuckDBIdentifier(column.Name))
				expressions = append(expressions, "NULL")
				continue
			}
			value, expression, include := splitFixtureValue(column, siteID, tenantID, goalID, funnelID, qrCodeID, at)
			if !include {
				continue
			}
			names = append(names, quoteDuckDBIdentifier(column.Name))
			expressions = append(expressions, expression)
			if expression == "?" {
				args = append(args, value)
			}
		}
		query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
			quoteDuckDBIdentifier(table), strings.Join(names, ", "), strings.Join(expressions, ", "))
		if _, err := store.DB().ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("seed split fixture table %s: %v", table, err)
		}
	}
}

func splitFixtureColumns(ctx context.Context, store *Store, table string) ([]splitFixtureColumn, error) {
	rows, err := store.DB().QueryContext(ctx, `
		SELECT column_name, data_type, is_nullable, column_default
		FROM information_schema.columns
		WHERE table_catalog = current_database() AND table_schema = 'main' AND table_name = ?
		ORDER BY ordinal_position`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var columns []splitFixtureColumn
	for rows.Next() {
		var column splitFixtureColumn
		if err := rows.Scan(&column.Name, &column.DataType, &column.Nullable, &column.DefaultSQL); err != nil {
			return nil, err
		}
		columns = append(columns, column)
	}
	return columns, rows.Err()
}

func splitFixtureValue(column splitFixtureColumn, siteID, tenantID, goalID, funnelID, qrCodeID uuid.UUID, at time.Time) (any, string, bool) {
	switch column.Name {
	case "site_id":
		return siteID, "?", true
	case "tenant_id", "team_id":
		return tenantID, "?", true
	case "goal_id":
		return goalID, "?", true
	case "funnel_id":
		return funnelID, "?", true
	case "qr_code_id":
		return qrCodeID, "?", true
	case "id":
		if column.DefaultSQL.Valid {
			return nil, "", false
		}
		return uuid.New(), "?", true
	}
	upperType := strings.ToUpper(column.DataType)
	switch {
	case upperType == "UUID":
		return uuid.New(), "?", true
	case strings.Contains(upperType, "TIMESTAMP"):
		return at, "?", true
	case upperType == "DATE":
		return at, "?", true
	case upperType == "BOOLEAN":
		return true, "?", true
	case upperType == "JSON":
		return `{"fixture":true}`, "?", true
	case strings.HasSuffix(upperType, "[]"):
		return nil, "[]::" + upperType, true
	case upperType == "INTERVAL":
		return nil, "INTERVAL '1 day'", true
	case strings.Contains(upperType, "INT") || strings.Contains(upperType, "DECIMAL") || upperType == "DOUBLE" || upperType == "FLOAT" || upperType == "REAL":
		return int64(1), "?", true
	case upperType == "BLOB":
		return []byte("fixture"), "?", true
	case strings.Contains(upperType, "CHAR") || upperType == "VARCHAR":
		value := "fixture"
		switch column.Name {
		case "path":
			value = "/fixture"
		case "metric":
			value = "LCP"
		case "rating":
			value = "good"
		case "bucket_type", "granularity":
			value = "hourly"
		case "bucket_unit":
			value = "hour"
		case "rollup_type":
			value = "hit"
		case "event_name", "name":
			value = "purchase"
		case "type":
			value = "event"
		case "operator":
			value = "equals"
		case "dimension", "dimension_name", "dimension_type":
			value = "country"
		case "country_code":
			value = "DE"
		}
		return value, "?", true
	default:
		if column.DefaultSQL.Valid {
			return nil, "", false
		}
		return "fixture", "?", true
	}
}

func snapshotSplitFixtureRows(t *testing.T, ctx context.Context, source *Store, expectedPath string, siteID uuid.UUID) {
	t.Helper()
	expected := NewStore(expectedPath)
	if err := expected.Connect(); err != nil {
		t.Fatalf("connect expected tenant fixture: %v", err)
	}
	if err := expected.MigrateTenant(ctx); err != nil {
		_ = expected.Close()
		t.Fatalf("migrate expected tenant fixture: %v", err)
	}
	if err := expected.Close(); err != nil {
		t.Fatalf("close expected tenant fixture: %v", err)
	}
	if _, err := source.DB().ExecContext(ctx,
		fmt.Sprintf("ATTACH '%s' AS split_expected", escapeSQLString(expectedPath))); err != nil {
		t.Fatalf("attach expected tenant fixture: %v", err)
	}
	if _, err := source.DB().ExecContext(ctx, `
		INSERT INTO split_expected.sites (id, domain, data_retention_days)
		SELECT id, domain, data_retention_days FROM sites WHERE id = ?`, siteID); err != nil {
		t.Fatalf("snapshot split fixture site mirror: %v", err)
	}
	copyOrder := []string{"goals", "funnels"}
	for _, table := range defaultTenantSplitFixtureTables {
		if table != "goals" && table != "funnels" {
			copyOrder = append(copyOrder, table)
		}
	}
	for _, table := range copyOrder {
		query := fmt.Sprintf("INSERT INTO split_expected.%s SELECT * FROM %s WHERE site_id = ?",
			quoteDuckDBIdentifier(table), quoteDuckDBIdentifier(table))
		if _, err := source.DB().ExecContext(ctx, query, siteID); err != nil {
			t.Fatalf("snapshot split fixture table %s: %v", table, err)
		}
	}
	if _, err := source.DB().ExecContext(ctx, "CHECKPOINT split_expected; DETACH split_expected;"); err != nil {
		t.Fatalf("checkpoint expected tenant fixture: %v", err)
	}
}

func assertSplitFixtureRowsEqual(t *testing.T, ctx context.Context, expectedPath, actualPath string) {
	t.Helper()
	worker, err := openDuckDBFile(":memory:")
	if err != nil {
		t.Fatalf("open split fixture comparison worker: %v", err)
	}
	defer worker.Close()
	for alias, path := range map[string]string{"expected": expectedPath, "actual": actualPath} {
		if _, err := worker.ExecContext(ctx,
			fmt.Sprintf("ATTACH '%s' AS %s (READ_ONLY)", escapeSQLString(path), alias)); err != nil {
			t.Fatalf("attach %s split fixture: %v", alias, err)
		}
	}
	for _, table := range defaultTenantSplitFixtureTables {
		quoted := quoteDuckDBIdentifier(table)
		query := fmt.Sprintf(`
			SELECT COUNT(*) FROM (
				(SELECT * FROM expected.%s EXCEPT ALL SELECT * FROM actual.%s)
				UNION ALL
				(SELECT * FROM actual.%s EXCEPT ALL SELECT * FROM expected.%s)
			) differences`, quoted, quoted, quoted, quoted)
		var differences int
		if err := worker.QueryRowContext(ctx, query).Scan(&differences); err != nil {
			t.Fatalf("compare complete rows for %s: %v", table, err)
		}
		if differences != 0 {
			t.Fatalf("split changed complete row contents for %s: differences=%d", table, differences)
		}
	}
}

func TestTenantStoreManagerUsesOneAttachedDataPlane(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sharedPath := filepath.Join(root, "hitkeep.db")
	dataPath := filepath.Join(root, "data")
	shared := NewStore(sharedPath)
	if err := shared.Connect(); err != nil {
		t.Fatal(err)
	}
	if err := shared.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	userID, err := shared.CreateUser(ctx, "attached@example.com", "hashed")
	if err != nil {
		t.Fatal(err)
	}
	defaultSite, err := shared.CreateSite(ctx, userID, "attached-default.test")
	if err != nil {
		t.Fatal(err)
	}
	otherTenant := uuid.New()
	if _, err := shared.DB().ExecContext(ctx, "INSERT INTO tenants (id, name, is_default, created_at) VALUES (?, ?, FALSE, ?)", otherTenant, "Attached Other", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := shared.Close(); err != nil {
		t.Fatal(err)
	}
	if err := RunDefaultTenantSplit(ctx, sharedPath, dataPath); err != nil {
		t.Fatal(err)
	}
	control := NewStore(sharedPath)
	if err := control.Connect(); err != nil {
		t.Fatal(err)
	}
	tenants, err := control.GetTenantList(ctx)
	defaultCount := 0
	for _, tenant := range tenants {
		if tenant.IsDefault {
			defaultCount++
		}
	}
	if err != nil || len(tenants) != 2 || defaultCount != 1 {
		_ = control.Close()
		t.Fatalf("expected active enumeration to include the default tenant exactly once: tenants=%+v err=%v", tenants, err)
	}
	mgr := NewTenantStoreManager(control, dataPath, WithTenantDataPlane(true))
	defaultStore, err := mgr.ForTenant(ctx, uuid.Nil)
	if err != nil {
		_ = control.Close()
		t.Fatal(err)
	}
	otherStore, err := mgr.ForTenant(ctx, otherTenant)
	if err != nil {
		_ = mgr.Close()
		_ = control.Close()
		t.Fatal(err)
	}
	if mgr.dataPlaneRoot != defaultStore {
		t.Fatal("expected default tenant store to be the data-plane root")
	}
	if otherStore.connectorPath != defaultStore.path {
		t.Fatalf("expected attached tenant connector root %q, got %q", defaultStore.path, otherStore.connectorPath)
	}
	if otherStore.connectionGate != defaultStore.connectionGate {
		t.Fatal("expected attached tenant handles to share the data-plane connection gate")
	}
	if otherStore.checkpointGate != defaultStore.checkpointGate {
		t.Fatal("expected attached tenant handles to share the data-plane checkpoint gate")
	}
	if otherStore.catalog != "tenant_"+strings.ReplaceAll(otherTenant.String(), "-", "") {
		t.Fatalf("unexpected tenant catalog %q", otherStore.catalog)
	}
	otherSite := *defaultSite
	otherSite.ID = uuid.New()
	otherSite.Domain = "attached-other.test"
	if err := otherStore.UpsertSiteMirror(ctx, &otherSite); err != nil {
		t.Fatalf("mirror attached tenant site: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	for store, site := range map[*Store]*api.Site{defaultStore: defaultSite, otherStore: &otherSite} {
		if err := store.CreateHit(ctx, &api.Hit{
			ID: uuid.New(), SiteID: site.ID, SessionID: uuid.New(), PageID: uuid.New(),
			Timestamp: now, Path: "/appender-catalog",
		}); err != nil {
			t.Fatalf("append catalog-scoped hit: %v", err)
		}
		var hits, dirty int
		if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM hits WHERE site_id = ?", site.ID).Scan(&hits); err != nil || hits != 1 {
			t.Fatalf("catalog-scoped appender routed incorrectly: hits=%d err=%v", hits, err)
		}
		if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM rollup_dirty_buckets WHERE site_id = ?", site.ID).Scan(&dirty); err != nil || dirty == 0 {
			t.Fatalf("catalog-scoped dirty buckets routed incorrectly: dirty=%d err=%v", dirty, err)
		}
	}
	var crossedHits int
	if err := defaultStore.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM hits WHERE site_id = ?", otherSite.ID).Scan(&crossedHits); err != nil || crossedHits != 0 {
		t.Fatalf("attached appender leaked into default catalog: hits=%d err=%v", crossedHits, err)
	}
	for _, store := range []*Store{defaultStore, otherStore} {
		if _, err := store.DB().ExecContext(ctx, "CREATE TABLE IF NOT EXISTS attachment_probe (site_id UUID, value VARCHAR)"); err != nil {
			_ = mgr.Close()
			_ = control.Close()
			t.Fatal(err)
		}
	}
	prepared, err := otherStore.DB().PrepareContext(ctx, "INSERT INTO attachment_probe VALUES (?, ?)")
	if err != nil {
		_ = mgr.Close()
		_ = control.Close()
		t.Fatal(err)
	}
	defer func() { _ = prepared.Close() }()
	if _, err := prepared.ExecContext(ctx, defaultSite.ID, "other-prepared"); err != nil {
		_ = prepared.Close()
		_ = mgr.Close()
		_ = control.Close()
		t.Fatal(err)
	}
	if err := prepared.Close(); err != nil {
		t.Fatal(err)
	}
	tx, err := defaultStore.DB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO attachment_probe VALUES (?, ?)", defaultSite.ID, "default-transaction"); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	errCh := make(chan error, 20)
	for index := range 20 {
		wait.Go(func() {
			store := defaultStore
			prefix := "default"
			if index%2 == 1 {
				store = otherStore
				prefix = "other"
			}
			_, err := store.DB().ExecContext(ctx, "INSERT INTO attachment_probe VALUES (?, ?)", defaultSite.ID, fmt.Sprintf("%s-%d", prefix, index))
			if err != nil {
				errCh <- err
			}
		})
	}
	wait.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("parallel catalog write: %v", err)
	}
	var rootProbe int
	if err := defaultStore.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM "+otherStore.catalog+".attachment_probe").Scan(&rootProbe); err != nil {
		_ = mgr.Close()
		_ = control.Close()
		t.Fatal(err)
	}
	if rootProbe != 11 {
		t.Fatalf("expected attached catalog rows visible from root, got %d", rootProbe)
	}
	var defaultProbe int
	if err := defaultStore.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM attachment_probe").Scan(&defaultProbe); err != nil || defaultProbe != 11 {
		t.Fatalf("default catalog was not isolated: count=%d err=%v", defaultProbe, err)
	}
	if err := defaultStore.Checkpoint(ctx, "attachment test default"); err != nil {
		t.Fatal(err)
	}
	if err := otherStore.Checkpoint(ctx, "attachment test other"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Close(); err != nil {
		_ = control.Close()
		t.Fatal(err)
	}
	if err := control.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestTenantStoreManagerActivationReadsTenantActivity(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sharedPath := filepath.Join(root, "hitkeep.db")
	dataPath := filepath.Join(root, "data")
	shared := NewStore(sharedPath)
	if err := shared.Connect(); err != nil {
		t.Fatal(err)
	}
	if err := shared.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	userID, err := shared.CreateUser(ctx, "activation-split@example.com", "hashed")
	if err != nil {
		t.Fatal(err)
	}
	site, err := shared.CreateSite(ctx, userID, "activation-split.test")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := shared.RecordHitActivity(ctx, []*api.Hit{{SiteID: site.ID, Timestamp: now}}); err != nil {
		t.Fatal(err)
	}
	if err := shared.Close(); err != nil {
		t.Fatal(err)
	}
	if err := RunDefaultTenantSplit(ctx, sharedPath, dataPath); err != nil {
		t.Fatal(err)
	}
	control := NewStore(sharedPath)
	if err := control.Connect(); err != nil {
		t.Fatal(err)
	}
	manager := NewTenantStoreManager(control, dataPath, WithTenantDataPlane(true))
	defer func() {
		_ = manager.Close()
		_ = control.Close()
	}()
	tenantStore, err := manager.ForTenant(ctx, uuid.Nil)
	if err != nil {
		t.Fatalf("open default tenant data plane: %v", err)
	}
	if err := tenantStore.RecordHitActivity(ctx, []*api.Hit{{SiteID: site.ID, Timestamp: now.Add(time.Minute)}}); err != nil {
		t.Fatalf("record tenant-local activity without control billing tables: %v", err)
	}
	response, err := manager.ListSystemActivation(ctx, ActivationQuery{Now: now.Add(time.Minute), Limit: 10})
	if err != nil {
		t.Fatalf("list split activation: %v", err)
	}
	if len(response.Rows) != 1 || response.Rows[0].Status != api.TrackingStatusLive || response.Rows[0].HitsLast24h != 2 {
		t.Fatalf("expected tenant-local activation row, got %+v", response.Rows)
	}
}
