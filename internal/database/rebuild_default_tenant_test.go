package database

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

// buildSplitTenantFixture migrates a shared database with one user, one
// default-tenant site, and one hit, runs the full split, and returns the
// site ID, default tenant ID, and tenant file path.
func buildSplitTenantFixture(t *testing.T, ctx context.Context, sharedPath, dataPath string) (uuid.UUID, uuid.UUID, string) {
	t.Helper()
	shared := NewStore(sharedPath)
	if err := shared.Connect(); err != nil {
		t.Fatalf("connect shared: %v", err)
	}
	if err := shared.Migrate(ctx); err != nil {
		_ = shared.Close()
		t.Fatalf("migrate shared: %v", err)
	}
	userID, err := shared.CreateUser(ctx, "rebuild-fixture@example.test", "hashed")
	if err != nil {
		_ = shared.Close()
		t.Fatalf("create user: %v", err)
	}
	site, err := shared.CreateSite(ctx, userID, "rebuild-fixture.example.test")
	if err != nil {
		_ = shared.Close()
		t.Fatalf("create site: %v", err)
	}
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	if err := shared.CreateHitsBulk(ctx, []*api.Hit{
		{ID: uuid.New(), SiteID: site.ID, SessionID: uuid.New(), PageID: uuid.New(), Timestamp: now, Path: "/fixture"},
	}); err != nil {
		_ = shared.Close()
		t.Fatalf("create hit: %v", err)
	}
	defaultID, err := shared.GetDefaultTenantID(ctx)
	if err != nil {
		_ = shared.Close()
		t.Fatalf("resolve default tenant: %v", err)
	}
	if err := shared.Close(); err != nil {
		t.Fatalf("close shared before split: %v", err)
	}
	if err := RunDefaultTenantSplit(ctx, sharedPath, dataPath); err != nil {
		t.Fatalf("run split: %v", err)
	}
	return site.ID, defaultID, filepath.Join(dataPath, "tenants", defaultID.String(), "hitkeep.db")
}

func TestRebuildDefaultTenantFileRebuildsMissingFile(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sharedPath := filepath.Join(root, "hitkeep.db")
	dataPath := filepath.Join(root, "data")
	siteID, defaultID, tenantPath := buildSplitTenantFixture(t, ctx, sharedPath, dataPath)

	if err := os.Remove(tenantPath); err != nil {
		t.Fatalf("remove tenant file: %v", err)
	}

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	rebuilt, err := RebuildDefaultTenantFile(ctx, sharedPath, dataPath, WithLogger(logger))
	if err != nil {
		t.Fatalf("rebuild default tenant file: %v", err)
	}
	if !strings.Contains(logs.String(), "Rebuilt an empty default tenant database") {
		t.Fatalf("expected injected logger to receive rebuild event, got %q", logs.String())
	}
	if rebuilt != tenantPath {
		t.Fatalf("rebuilt path = %s, want %s", rebuilt, tenantPath)
	}

	control := NewStore(sharedPath)
	if err := control.Connect(); err != nil {
		t.Fatalf("connect control after rebuild: %v", err)
	}
	defer control.Close()
	mgr := NewTenantStoreManager(control, dataPath, WithTenantDataPlane(true))
	defer mgr.Close()
	store, err := mgr.ForTenant(ctx, defaultID)
	if err != nil {
		t.Fatalf("open rebuilt tenant data plane: %v", err)
	}
	var mirrors, hits int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM sites WHERE id = ?", siteID).Scan(&mirrors); err != nil {
		t.Fatalf("count rebuilt site mirrors: %v", err)
	}
	if mirrors != 1 {
		t.Fatalf("expected rebuilt site mirror, got %d", mirrors)
	}
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM hits").Scan(&hits); err != nil {
		t.Fatalf("count rebuilt hits: %v", err)
	}
	if hits != 0 {
		t.Fatalf("expected empty rebuilt analytics, got %d hits", hits)
	}
}

func TestRebuildDefaultTenantFileRefusesValidFile(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sharedPath := filepath.Join(root, "hitkeep.db")
	dataPath := filepath.Join(root, "data")
	_, _, tenantPath := buildSplitTenantFixture(t, ctx, sharedPath, dataPath)

	_, err := RebuildDefaultTenantFile(ctx, sharedPath, dataPath)
	if err == nil || !strings.Contains(err.Error(), "refusing to rebuild") {
		t.Fatalf("expected refusal over valid tenant file %s, got %v", tenantPath, err)
	}
}

func TestRebuildDefaultTenantFileSetsAsideInvalidFile(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sharedPath := filepath.Join(root, "hitkeep.db")
	dataPath := filepath.Join(root, "data")
	_, _, tenantPath := buildSplitTenantFixture(t, ctx, sharedPath, dataPath)

	if err := os.WriteFile(tenantPath, []byte("not a database"), 0o600); err != nil {
		t.Fatalf("write invalid tenant file: %v", err)
	}
	if _, err := RebuildDefaultTenantFile(ctx, sharedPath, dataPath); err != nil {
		t.Fatalf("rebuild over invalid tenant file: %v", err)
	}
	setAside, err := filepath.Glob(tenantPath + ".invalid-*")
	if err != nil || len(setAside) != 1 {
		t.Fatalf("expected one set-aside invalid tenant file, got %v (err=%v)", setAside, err)
	}
	tenant := NewStore(tenantPath)
	if err := tenant.Connect(); err != nil {
		t.Fatalf("open rebuilt tenant file: %v", err)
	}
	_ = tenant.Close()
}

func TestRebuildDefaultTenantFileRequiresSplitMarker(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	sharedPath := filepath.Join(root, "hitkeep.db")
	shared := NewStore(sharedPath)
	if err := shared.Connect(); err != nil {
		t.Fatal(err)
	}
	if err := shared.Migrate(ctx); err != nil {
		_ = shared.Close()
		t.Fatal(err)
	}
	if err := shared.Close(); err != nil {
		t.Fatal(err)
	}
	_, err := RebuildDefaultTenantFile(ctx, sharedPath, filepath.Join(root, "data"))
	if err == nil || !strings.Contains(err.Error(), "no default tenant split marker") {
		t.Fatalf("expected split marker requirement, got %v", err)
	}
}
