package database

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

var defaultTenantSplitFaultPoints = []string{
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

type defaultTenantSplitFaultFixture struct {
	base            []byte
	siteID          uuid.UUID
	defaultTenantID uuid.UUID
}

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
	fixture := newDefaultTenantSplitFaultFixture(t)

	for _, point := range defaultTenantSplitFaultPoints {
		t.Run(point, func(t *testing.T) {
			root := t.TempDir()
			sharedPath := filepath.Join(root, "hitkeep.db")
			if err := os.WriteFile(sharedPath, fixture.base, 0o600); err != nil {
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
			assertDefaultTenantSplitFaultFixture(t, sharedPath, dataPath, fixture)
		})
	}
}

func TestDefaultTenantSplitFaultBoundariesSurviveProcessTermination(t *testing.T) {
	requireDefaultTenantMigrationAcceptance(t)
	ctx := context.Background()
	fixture := newDefaultTenantSplitFaultFixture(t)

	for _, point := range defaultTenantSplitFaultPoints {
		t.Run(point, func(t *testing.T) {
			root := t.TempDir()
			sharedPath := filepath.Join(root, "hitkeep.db")
			if err := os.WriteFile(sharedPath, fixture.base, 0o600); err != nil {
				t.Fatal(err)
			}
			dataPath := filepath.Join(root, "data")
			terminateDefaultTenantSplitAtFault(t, sharedPath, dataPath, point)
			if err := RunDefaultTenantSplit(ctx, sharedPath, dataPath); err != nil {
				t.Fatalf("resume after process termination at %s: %v", point, err)
			}
			assertDefaultTenantSplitFaultFixture(t, sharedPath, dataPath, fixture)
		})
	}
}

func TestDefaultTenantSplitFaultBoundaryProcess(t *testing.T) {
	if os.Getenv("HITKEEP_DEFAULT_TENANT_SPLIT_FAULT_CHILD") != "1" {
		return
	}
	point := os.Getenv("HITKEEP_DEFAULT_TENANT_SPLIT_FAULT_POINT")
	sharedPath := os.Getenv("HITKEEP_DEFAULT_TENANT_SPLIT_SHARED_PATH")
	dataPath := os.Getenv("HITKEEP_DEFAULT_TENANT_SPLIT_DATA_PATH")
	if point == "" || sharedPath == "" || dataPath == "" {
		t.Fatal("missing split fault child configuration")
	}
	err := runDefaultTenantSplitWithFaults(context.Background(), sharedPath, dataPath, nil, nil, func(got string) error {
		if got != point {
			return nil
		}
		if _, err := fmt.Fprintln(os.Stdout, got); err != nil {
			return fmt.Errorf("report split fault %q: %w", got, err)
		}
		_, err := os.Stdin.Read(make([]byte, 1))
		return err
	})
	t.Fatalf("split process returned after fault %q: %v", point, err)
}

func newDefaultTenantSplitFaultFixture(t *testing.T) defaultTenantSplitFaultFixture {
	t.Helper()
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
	defaultTenantID, err := base.GetDefaultTenantID(ctx)
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
	return defaultTenantSplitFaultFixture{
		base:            baseBytes,
		siteID:          site.ID,
		defaultTenantID: defaultTenantID,
	}
}

func terminateDefaultTenantSplitAtFault(t *testing.T, sharedPath, dataPath, point string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestDefaultTenantSplitFaultBoundaryProcess$")
	cmd.Env = append(os.Environ(),
		"HITKEEP_DEFAULT_TENANT_SPLIT_FAULT_CHILD=1",
		"HITKEEP_DEFAULT_TENANT_SPLIT_FAULT_POINT="+point,
		"HITKEEP_DEFAULT_TENANT_SPLIT_SHARED_PATH="+sharedPath,
		"HITKEEP_DEFAULT_TENANT_SPLIT_DATA_PATH="+dataPath,
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("open split process fault input %q: %v", point, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("open split process fault output %q: %v", point, err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start split process fault %q: %v", point, err)
	}
	waited := false
	defer func() {
		if !waited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			_ = stdin.Close()
		}
	}()

	got, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		waitErr := cmd.Wait()
		_ = stdin.Close()
		waited = true
		t.Fatalf("split process exited before fault %q: read=%v wait=%v stderr=%s", point, err, waitErr, stderr.String())
	}
	if got = strings.TrimSpace(got); got != point {
		t.Fatalf("split process reported fault %q, want %q", got, point)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("terminate split process at fault %q: %v", point, err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatalf("split process exited successfully after fault %q", point)
	}
	_ = stdin.Close()
	waited = true
}

func assertDefaultTenantSplitFaultFixture(t *testing.T, sharedPath, dataPath string, fixture defaultTenantSplitFaultFixture) {
	t.Helper()
	ctx := context.Background()
	control := NewStore(sharedPath)
	if err := control.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.Close() })
	complete, err := control.DefaultTenantSplitComplete(ctx)
	if err != nil || !complete {
		t.Fatalf("split incomplete: complete=%v err=%v", complete, err)
	}
	var controlSites int
	if err := control.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM sites WHERE id = ?", fixture.siteID).Scan(&controlSites); err != nil || controlSites != 1 {
		t.Fatalf("control site state: count=%d err=%v", controlSites, err)
	}
	var controlHits int
	if err := control.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM hits WHERE site_id = ?", fixture.siteID).Scan(&controlHits); err != nil || controlHits != 0 {
		t.Fatalf("control hit state: count=%d err=%v", controlHits, err)
	}
	tenant := NewStore(filepath.Join(dataPath, "tenants", fixture.defaultTenantID.String(), "hitkeep.db"))
	if err := tenant.Connect(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tenant.Close() })
	var tenantHits int
	if err := tenant.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM hits WHERE site_id = ?", fixture.siteID).Scan(&tenantHits); err != nil || tenantHits != 1 {
		t.Fatalf("tenant hit state: count=%d err=%v", tenantHits, err)
	}
}

func requireDefaultTenantMigrationAcceptance(t *testing.T) {
	t.Helper()
	if os.Getenv("HITKEEP_DEFAULT_TENANT_MIGRATION_ACCEPTANCE") != "1" {
		t.Skip("run through the default-tenant-migration-acceptance QA gate")
	}
}
