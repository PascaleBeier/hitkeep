package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

// exportRetentionArchiveFixture mirrors the retention worker's export shape:
// one parquet per site with a _source discriminator over UNION BY NAME.
func exportRetentionArchiveFixture(t *testing.T, ctx context.Context, tenantPath string, siteID uuid.UUID, file string) {
	t.Helper()
	tenant := NewStore(tenantPath)
	if err := tenant.Connect(); err != nil {
		t.Fatalf("connect tenant for archive export: %v", err)
	}
	defer tenant.Close()
	query := fmt.Sprintf(`COPY (
		SELECT 'hits' AS _source, * FROM hits WHERE site_id = '%s'
		UNION BY NAME
		SELECT 'events' AS _source, * FROM events WHERE site_id = '%s'
	) TO '%s' (FORMAT PARQUET, COMPRESSION 'SNAPPY');`, siteID, siteID, escapeSQLString(file))
	if _, err := tenant.DB().ExecContext(ctx, query); err != nil {
		t.Fatalf("export archive fixture: %v", err)
	}
}

func TestImportRetentionArchivesRestoresRebuiltTenant(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sharedPath := filepath.Join(root, "hitkeep.db")
	dataPath := filepath.Join(root, "data")
	siteID, _, tenantPath := buildSplitTenantFixture(t, ctx, sharedPath, dataPath)

	archiveFile := filepath.Join(root, fmt.Sprintf("site_%s_1.parquet", siteID))
	exportRetentionArchiveFixture(t, ctx, tenantPath, siteID, archiveFile)

	// Lose the tenant file, rebuild it empty, then import the archive.
	if err := os.Remove(tenantPath); err != nil {
		t.Fatalf("remove tenant file: %v", err)
	}
	if _, err := RebuildDefaultTenantFile(ctx, sharedPath, dataPath); err != nil {
		t.Fatalf("rebuild default tenant: %v", err)
	}
	summary, err := ImportRetentionArchives(ctx, tenantPath, []string{archiveFile})
	if err != nil {
		t.Fatalf("import retention archives: %v", err)
	}
	if summary.Files != 1 || summary.Imported["hits"] != 1 {
		t.Fatalf("unexpected import summary: %+v", summary)
	}

	tenant := NewStore(tenantPath)
	if err := tenant.Connect(); err != nil {
		t.Fatalf("open tenant after import: %v", err)
	}
	defer tenant.Close()
	var hits int
	if err := tenant.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM hits WHERE site_id = ? AND path = '/fixture'", siteID).Scan(&hits); err != nil {
		t.Fatalf("count imported hits: %v", err)
	}
	if hits != 1 {
		t.Fatalf("expected imported fixture hit, got %d", hits)
	}
	if err := tenant.Close(); err != nil {
		t.Fatalf("close tenant before idempotency check: %v", err)
	}

	// A second import of the same file must not duplicate rows.
	summary, err = ImportRetentionArchives(ctx, tenantPath, []string{archiveFile})
	if err != nil {
		t.Fatalf("re-import retention archives: %v", err)
	}
	if summary.Imported["hits"] != 0 || summary.Skipped["hits"] != 1 {
		t.Fatalf("expected idempotent re-import, got %+v", summary)
	}
}

func TestImportRetentionArchivesSkipsForeignSites(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sharedPath := filepath.Join(root, "hitkeep.db")
	dataPath := filepath.Join(root, "data")
	siteID, _, tenantPath := buildSplitTenantFixture(t, ctx, sharedPath, dataPath)

	// Rewrite the archive rows onto a site this tenant does not own.
	foreignFile := filepath.Join(root, "site_foreign_1.parquet")
	tenant := NewStore(tenantPath)
	if err := tenant.Connect(); err != nil {
		t.Fatalf("connect tenant for foreign export: %v", err)
	}
	query := fmt.Sprintf(`COPY (
		SELECT 'hits' AS _source, * REPLACE (CAST('%s' AS UUID) AS site_id) FROM hits WHERE site_id = '%s'
	) TO '%s' (FORMAT PARQUET, COMPRESSION 'SNAPPY');`, uuid.New(), siteID, escapeSQLString(foreignFile))
	if _, err := tenant.DB().ExecContext(ctx, query); err != nil {
		_ = tenant.Close()
		t.Fatalf("export foreign-site fixture: %v", err)
	}
	if err := tenant.Close(); err != nil {
		t.Fatalf("close tenant: %v", err)
	}

	summary, err := ImportRetentionArchives(ctx, tenantPath, []string{foreignFile})
	if err != nil {
		t.Fatalf("import foreign-site archive: %v", err)
	}
	if summary.Imported["hits"] != 0 || summary.Skipped["hits"] != 1 {
		t.Fatalf("expected foreign-site row to be skipped, got %+v", summary)
	}
}

func TestDiscoverLocalRetentionArchivesFindsBothLayouts(t *testing.T) {
	root := t.TempDir()
	tenantID := uuid.New()
	siteID := uuid.New()
	nestedDir := filepath.Join(root, "tenants", tenantID.String(), "sites", siteID.String())
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(nestedDir, fmt.Sprintf("site_%s_2.parquet", siteID))
	flat := filepath.Join(root, fmt.Sprintf("site_%s_1.parquet", siteID))
	other := filepath.Join(root, "tenants", uuid.NewString(), "sites", siteID.String())
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{nested, flat, filepath.Join(other, "site_x_3.parquet")} {
		if err := os.WriteFile(path, []byte("parquet"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	files, err := DiscoverLocalRetentionArchives(root, tenantID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0] != flat || files[1] != nested {
		t.Fatalf("unexpected default-tenant discovery: %v", files)
	}
	files, err = DiscoverLocalRetentionArchives(root, tenantID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != nested {
		t.Fatalf("unexpected non-default discovery: %v", files)
	}
}
