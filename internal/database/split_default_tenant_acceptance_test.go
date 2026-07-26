package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

func TestSplitFingerprintSurvivesLargeParquetRoundTrip(t *testing.T) {
	requireDefaultTenantMigrationAcceptance(t)
	ctx := context.Background()
	source := NewStore(filepath.Join(t.TempDir(), "source.db"), WithThreads(1))
	if err := source.Connect(); err != nil {
		t.Fatalf("connect fingerprint source: %v", err)
	}
	t.Cleanup(func() { _ = source.Close() })
	if _, err := source.DB().ExecContext(ctx, `
		CREATE TABLE fingerprint_rows (id UUID PRIMARY KEY);
		INSERT INTO fingerprint_rows SELECT uuid() FROM range(300000);
	`); err != nil {
		t.Fatalf("seed fingerprint source: %v", err)
	}
	var sourceCatalog string
	if err := source.DB().QueryRowContext(ctx, "SELECT current_database()").Scan(&sourceCatalog); err != nil {
		t.Fatalf("resolve fingerprint source catalog: %v", err)
	}
	exportPath := filepath.Join(t.TempDir(), "export")
	if _, err := source.DB().ExecContext(ctx, fmt.Sprintf("EXPORT DATABASE '%s' (FORMAT PARQUET)", escapeSQLString(exportPath))); err != nil {
		t.Fatalf("export fingerprint source: %v", err)
	}

	restored := NewStore(filepath.Join(t.TempDir(), "restored.db"), WithThreads(1))
	if err := restored.Connect(); err != nil {
		t.Fatalf("connect fingerprint restore: %v", err)
	}
	t.Cleanup(func() { _ = restored.Close() })
	if _, err := restored.DB().ExecContext(ctx, fmt.Sprintf("IMPORT DATABASE '%s'", escapeSQLString(exportPath))); err != nil {
		t.Fatalf("import fingerprint restore: %v", err)
	}
	var restoredCatalog string
	if err := restored.DB().QueryRowContext(ctx, "SELECT current_database()").Scan(&restoredCatalog); err != nil {
		t.Fatalf("resolve fingerprint restore catalog: %v", err)
	}

	expected, err := fingerprintSplitTable(ctx, source.DB(), sourceCatalog, "fingerprint_rows", []string{"id"}, "")
	if err != nil {
		t.Fatalf("fingerprint source rows: %v", err)
	}
	actual, err := fingerprintSplitTable(ctx, restored.DB(), restoredCatalog, "fingerprint_rows", []string{"id"}, "")
	if err != nil {
		t.Fatalf("fingerprint restored rows: %v", err)
	}
	if expected != actual {
		t.Fatalf("fingerprint changed across Parquet round trip: source_count=%d restored_count=%d", expected.Count, actual.Count)
	}
}

func TestDefaultTenantSplitFaultBoundariesResume(t *testing.T) {
	requireDefaultTenantMigrationAcceptance(t)
	ctx := context.Background()
	basePath := filepath.Join(t.TempDir(), "base.db")
	base := NewStore(basePath)
	if err := base.Connect(); err != nil {
		t.Fatal(err)
	}
	if err := base.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	userID, err := base.CreateUser(ctx, "split-faults@example.test", "hashed")
	if err != nil {
		t.Fatal(err)
	}
	site, err := base.CreateSite(ctx, userID, "split-faults.example.test")
	if err != nil {
		t.Fatal(err)
	}
	if err := base.CreateHit(ctx, &api.Hit{
		ID: uuid.New(), SiteID: site.ID, SessionID: uuid.New(), PageID: uuid.New(),
		Timestamp: time.Now().UTC(), Path: "/fault-boundary",
	}); err != nil {
		t.Fatal(err)
	}
	defaultID, err := base.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := base.Close(); err != nil {
		t.Fatal(err)
	}
	baseBytes, err := os.ReadFile(basePath)
	if err != nil {
		t.Fatal(err)
	}

	points := []string{
		"after-target-preparation",
		"after-copy:hits",
		"after-source-fingerprint:hits",
		"after-target-fingerprint:hits",
		"after-copy-checkpoint:hits",
		"after-target-checkpoint",
		"after-target-rename",
		"after-delete:hits",
		"after-cleanup-fingerprint:hits",
		"before-split-marker",
		"after-split-marker-commit",
		"after-split-marker-checkpoint",
		"before-control-rewrite",
		"after-control-source-rename",
		"after-control-target-rename",
		"after-control-rewrite",
		"before-cleanup-marker",
		"after-cleanup-marker",
		"after-cleanup-marker-checkpoint",
	}
	for _, point := range points {
		t.Run(point, func(t *testing.T) {
			root := t.TempDir()
			sharedPath := filepath.Join(root, "hitkeep.db")
			if err := os.WriteFile(sharedPath, baseBytes, 0o600); err != nil {
				t.Fatal(err)
			}
			dataPath := filepath.Join(root, "data")
			injected := false
			err := runDefaultTenantSplitWithFaults(ctx, sharedPath, dataPath, nil, nil, func(got string) error {
				if got == point && !injected {
					injected = true
					return fmt.Errorf("injected test failure")
				}
				return nil
			})
			if !injected || err == nil || !strings.Contains(err.Error(), point) {
				t.Fatalf("fault %q was not injected: injected=%v err=%v", point, injected, err)
			}
			if err := RunDefaultTenantSplit(ctx, sharedPath, dataPath); err != nil {
				t.Fatalf("resume after %s: %v", point, err)
			}

			control := NewStore(sharedPath)
			if err := control.Connect(); err != nil {
				t.Fatal(err)
			}
			defer control.Close()
			complete, err := control.DefaultTenantSplitComplete(ctx)
			if err != nil || !complete {
				t.Fatalf("split incomplete after %s: complete=%v err=%v", point, complete, err)
			}
			var controlHits int
			if err := control.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM hits WHERE site_id = ?", site.ID).Scan(&controlHits); err != nil || controlHits != 0 {
				t.Fatalf("control hit state after %s: count=%d err=%v", point, controlHits, err)
			}
			tenant := NewStore(filepath.Join(dataPath, "tenants", defaultID.String(), "hitkeep.db"))
			if err := tenant.Connect(); err != nil {
				t.Fatal(err)
			}
			defer tenant.Close()
			var tenantHits int
			if err := tenant.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM hits WHERE site_id = ?", site.ID).Scan(&tenantHits); err != nil || tenantHits != 1 {
				t.Fatalf("tenant hit state after %s: count=%d err=%v", point, tenantHits, err)
			}
		})
	}
}

func requireDefaultTenantMigrationAcceptance(t *testing.T) {
	t.Helper()
	if os.Getenv("HITKEEP_DEFAULT_TENANT_MIGRATION_ACCEPTANCE") != "1" {
		t.Skip("run through the default-tenant-migration-acceptance QA gate")
	}
}
