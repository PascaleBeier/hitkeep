package database

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

// seedFragmentedDatabase creates a file-backed database whose file holds a
// large share of free blocks: bulk-insert hits, checkpoint them into the
// file, delete them all, and checkpoint again so the row groups are freed.
func seedFragmentedDatabase(t *testing.T) (string, uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "compact.db")

	store := NewStore(path)
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	userID, err := store.CreateUser(ctx, "compact@example.com", "hashed_secret")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "compact.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	hits := make([]*api.Hit, 0, 200000)
	now := time.Now().UTC()
	for i := 0; i < cap(hits); i++ {
		hits = append(hits, &api.Hit{
			ID:        uuid.New(),
			SiteID:    site.ID,
			SessionID: uuid.New(),
			PageID:    uuid.New(),
			Timestamp: now,
			Path:      "/compaction-filler-page-with-a-reasonably-long-path/segment",
		})
	}
	if err := store.CreateHitsBulk(ctx, hits); err != nil {
		t.Fatalf("bulk insert hits: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx, "CHECKPOINT"); err != nil {
		t.Fatalf("checkpoint after insert: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx, "DELETE FROM hits"); err != nil {
		t.Fatalf("delete hits: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx, "CHECKPOINT"); err != nil {
		t.Fatalf("checkpoint after delete: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	return path, site.ID
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.Size()
}

func TestMaybeCompactDatabaseReclaimsSpaceAfterBulkDelete(t *testing.T) {
	ctx := context.Background()
	path, siteID := seedFragmentedDatabase(t)
	sizeBefore := fileSize(t, path)

	result, err := MaybeCompactDatabase(ctx, path, CompactionOptions{MinReclaimableBytes: 1, MinFreeRatio: 0}, PrepareSharedSchema)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}
	if !result.Compacted {
		t.Fatalf("expected compaction to run, got %+v", result)
	}
	sizeAfter := fileSize(t, path)
	if sizeAfter >= sizeBefore {
		t.Fatalf("expected file to shrink, before=%d after=%d", sizeBefore, sizeAfter)
	}
	if result.BytesBefore <= result.BytesAfter {
		t.Fatalf("expected result to report shrinkage, got %+v", result)
	}

	// No swap artifacts may remain.
	for _, leftover := range []string{path + ".compacting", path + ".pre-compact"} {
		if _, err := os.Stat(leftover); !os.IsNotExist(err) {
			t.Errorf("expected %s to be cleaned up", leftover)
		}
	}

	// The compacted database must keep data, constraints, and migrations.
	store := NewStore(path)
	if err := store.Connect(); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("re-migrate compacted db: %v", err)
	}

	site, err := store.GetSiteByID(ctx, siteID)
	if err != nil || site == nil || site.Domain != "compact.test" {
		t.Fatalf("expected site to survive compaction, got site=%+v err=%v", site, err)
	}
	var hitCount int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM hits").Scan(&hitCount); err != nil {
		t.Fatalf("count hits: %v", err)
	}
	if hitCount != 0 {
		t.Fatalf("expected zero hits after compaction, got %d", hitCount)
	}
	// Unique constraint on sites.domain must survive.
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO sites (id, user_id, domain, created_at) VALUES (?, ?, 'compact.test', now())",
		uuid.New(), site.UserID,
	); err == nil {
		t.Fatal("expected unique domain constraint to survive compaction")
	}
	// Foreign keys must survive.
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO goals (id, site_id, name, type, value, created_at) VALUES (?, ?, 'g', 'event', 'x', now())",
		uuid.New(), uuid.New(),
	); err == nil {
		t.Fatal("expected sites foreign key to survive compaction")
	}
}

func TestMaybeCompactDatabaseSkipsBelowThresholds(t *testing.T) {
	ctx := context.Background()
	path, _ := seedFragmentedDatabase(t)

	// Absurdly high thresholds: measurement happens, compaction does not.
	result, err := MaybeCompactDatabase(ctx, path, CompactionOptions{MinReclaimableBytes: 1 << 60, MinFreeRatio: 0.999999}, PrepareSharedSchema)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}
	if result.Compacted {
		t.Fatalf("expected compaction to be skipped, got %+v", result)
	}
	if result.ReclaimableBytes <= 0 || result.FreeRatio <= 0 {
		t.Fatalf("expected measurement to report reclaimable space, got %+v", result)
	}
}

func TestMaybeCompactDatabaseIgnoresMemoryAndMissingPaths(t *testing.T) {
	ctx := context.Background()
	for _, path := range []string{":memory:", filepath.Join(t.TempDir(), "does-not-exist.db")} {
		result, err := MaybeCompactDatabase(ctx, path, DefaultCompactionOptions(), PrepareSharedSchema)
		if err != nil {
			t.Fatalf("compact %s: %v", path, err)
		}
		if result.Compacted {
			t.Fatalf("expected no compaction for %s", path)
		}
	}
}

func TestMaybeCompactDatabaseRemovesStaleArtifacts(t *testing.T) {
	ctx := context.Background()
	path, _ := seedFragmentedDatabase(t)
	if err := os.WriteFile(path+".compacting", []byte("stale"), 0o600); err != nil {
		t.Fatalf("write stale artifact: %v", err)
	}

	if _, err := MaybeCompactDatabase(ctx, path, CompactionOptions{MinReclaimableBytes: 1, MinFreeRatio: 0}, PrepareSharedSchema); err != nil {
		t.Fatalf("compact with stale artifact: %v", err)
	}
	if _, err := os.Stat(path + ".compacting"); !os.IsNotExist(err) {
		t.Error("expected stale compacting artifact to be removed")
	}
}

func TestRecoverCompactionSwapRestoresMissingLiveFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shared.db")
	backup := path + ".pre-compact"
	work := path + ".compacting"
	if err := os.WriteFile(backup, []byte("authoritative"), 0o600); err != nil {
		t.Fatalf("write backup: %v", err)
	}
	if err := os.WriteFile(work, []byte("incomplete"), 0o600); err != nil {
		t.Fatalf("write work file: %v", err)
	}
	if err := recoverCompactionSwap(path); err != nil {
		t.Fatalf("recover compaction swap: %v", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read restored database: %v", err)
	}
	if string(contents) != "authoritative" {
		t.Fatalf("restored wrong contents: %q", contents)
	}
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Fatalf("expected backup to be consumed, got %v", err)
	}
	if _, err := os.Stat(work); !os.IsNotExist(err) {
		t.Fatalf("expected incomplete work file removed, got %v", err)
	}
}

func TestTenantStoreManagerCompactsTenantDatabaseOnOpen(t *testing.T) {
	ctx := context.Background()
	shared := newSharedTestStore(t)
	basePath := t.TempDir()

	userID, err := shared.CreateUser(ctx, "tenant-compact@test.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	team, err := shared.CreateTenant(ctx, userID, "Compaction Team", "")
	if err != nil {
		t.Fatalf("create team: %v", err)
	}

	// Fragment the tenant database, then close every tenant store.
	seeder := NewTenantStoreManager(shared, basePath)
	tenantStore, err := seeder.ForTenant(ctx, team.ID)
	if err != nil {
		t.Fatalf("open tenant store: %v", err)
	}
	site, err := shared.CreateSite(ctx, userID, "tenant-compact.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	if err := tenantStore.UpsertSiteMirror(ctx, site); err != nil {
		t.Fatalf("mirror site: %v", err)
	}
	hits := make([]*api.Hit, 0, 50000)
	now := time.Now().UTC()
	for i := 0; i < cap(hits); i++ {
		hits = append(hits, &api.Hit{
			ID: uuid.New(), SiteID: site.ID, SessionID: uuid.New(), PageID: uuid.New(),
			Timestamp: now, Path: "/tenant-compaction-filler/segment",
		})
	}
	if err := tenantStore.CreateHitsBulk(ctx, hits); err != nil {
		t.Fatalf("bulk insert tenant hits: %v", err)
	}
	if _, err := tenantStore.DB().ExecContext(ctx, "CHECKPOINT"); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	if _, err := tenantStore.DB().ExecContext(ctx, "DELETE FROM hits"); err != nil {
		t.Fatalf("delete tenant hits: %v", err)
	}
	if _, err := tenantStore.DB().ExecContext(ctx, "CHECKPOINT"); err != nil {
		t.Fatalf("checkpoint after delete: %v", err)
	}
	if err := seeder.Close(); err != nil {
		t.Fatalf("close seeder manager: %v", err)
	}
	dbPath := filepath.Join(basePath, "tenants", team.ID.String(), "hitkeep.db")
	sizeBefore := fileSize(t, dbPath)

	// A manager with compaction enabled must shrink the file on lazy open.
	mgr := NewTenantStoreManager(shared, basePath, WithTenantCompaction(CompactionOptions{MinReclaimableBytes: 1, MinFreeRatio: 0}))
	t.Cleanup(func() { _ = mgr.Close() })
	reopened, err := mgr.ForTenant(ctx, team.ID)
	if err != nil {
		t.Fatalf("reopen tenant store with compaction: %v", err)
	}
	if sizeAfter := fileSize(t, dbPath); sizeAfter >= sizeBefore {
		t.Fatalf("expected tenant database to shrink on open, before=%d after=%d", sizeBefore, sizeAfter)
	}

	var mirrorDomain string
	if err := reopened.DB().QueryRowContext(ctx, "SELECT domain FROM sites WHERE id = ?", site.ID).Scan(&mirrorDomain); err != nil {
		t.Fatalf("read mirrored site after compaction: %v", err)
	}
	if mirrorDomain != "tenant-compact.test" {
		t.Fatalf("expected site mirror to survive compaction, got %q", mirrorDomain)
	}
}
