package database

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/database/migrations"
)

func TestTenantMigrationUpgradeWithExistingSiteReferences(t *testing.T) {
	ctx := context.Background()

	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := applyMigrationsThrough(t, store, "2026_04_01_000000_create_report_subscriptions.sql"); err != nil {
		t.Fatalf("apply baseline migrations: %v", err)
	}

	now := time.Now().UTC()
	userID := uuid.New()
	siteID := uuid.New()
	hitID := uuid.New()
	sessionID := uuid.New()
	pageID := uuid.New()

	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO users (id, email, password, created_at) VALUES (?, ?, ?, ?)",
		userID, "upgrade@tenant.test", "hash", now,
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO sites (id, user_id, domain, created_at) VALUES (?, ?, ?, ?)",
		siteID, userID, "upgrade-tenant.test", now,
	); err != nil {
		t.Fatalf("insert site: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO hits (
			id, site_id, session_id, page_id, timestamp, path
		) VALUES (?, ?, ?, ?, ?, ?)
	`, hitID, siteID, sessionID, pageID, now, "/"); err != nil {
		t.Fatalf("insert hit: %v", err)
	}

	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate store with tenant migrations: %v", err)
	}

	var tenantMappingCount int
	if err := store.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM site_tenants WHERE site_id = ?",
		siteID,
	).Scan(&tenantMappingCount); err != nil {
		t.Fatalf("query site_tenants: %v", err)
	}
	if tenantMappingCount != 1 {
		t.Fatalf("expected 1 site_tenants row for site, got %d", tenantMappingCount)
	}

	var tenantMemberCount int
	if err := store.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM tenant_members WHERE user_id = ?",
		userID,
	).Scan(&tenantMemberCount); err != nil {
		t.Fatalf("query tenant_members: %v", err)
	}
	if tenantMemberCount != 1 {
		t.Fatalf("expected 1 tenant_members row for user, got %d", tenantMemberCount)
	}
}

func TestCentralAuditMigrationBackfillsTeamAuditView(t *testing.T) {
	ctx := context.Background()
	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := applyMigrationsThrough(t, store, "2026_05_22_000000_normalize_imported_property_event_names.sql"); err != nil {
		t.Fatalf("apply baseline migrations: %v", err)
	}

	actorID, err := store.CreateUser(ctx, "legacy-audit-actor@example.com", "hash")
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}
	targetID, err := store.CreateUser(ctx, "legacy-audit-target@example.com", "hash")
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	tenantID, err := store.GetDefaultTenantID(ctx)
	if err != nil {
		t.Fatalf("get default tenant: %v", err)
	}
	legacyAuditID := uuid.New()
	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO team_audit_log (id, tenant_id, actor_id, target_user_id, action, details)
		VALUES (?, ?, ?, ?, ?, ?)
	`, legacyAuditID, tenantID, actorID, targetID, "member.added", "legacy row"); err != nil {
		t.Fatalf("insert legacy team audit row: %v", err)
	}

	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate central audit: %v", err)
	}

	var centralCount int
	if err := store.DB().QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM instance_audit_log
		WHERE id = ? AND team_id = ? AND target_user_id = ? AND action = ?
	`, legacyAuditID, tenantID, targetID, "member.added").Scan(&centralCount); err != nil {
		t.Fatalf("count central backfill rows: %v", err)
	}
	if centralCount != 1 {
		t.Fatalf("expected one central backfill row, got %d", centralCount)
	}

	entries, total, err := store.ListTeamAuditEntries(ctx, tenantID, "member.added", 10, 0)
	if err != nil {
		t.Fatalf("list team audit view entries: %v", err)
	}
	if total != 1 || len(entries) != 1 {
		t.Fatalf("expected one projected team audit row, total=%d len=%d", total, len(entries))
	}
	if entries[0].ID != legacyAuditID {
		t.Fatalf("expected projected legacy audit id %s, got %s", legacyAuditID, entries[0].ID)
	}
}

func applyMigrationsThrough(t *testing.T, store *Store, maxFileName string) error {
	t.Helper()

	ctx := context.Background()
	migrationTableSQL, err := migrations.Fs.ReadFile("0000_00_00_000000_create_migrations_table.sql")
	if err != nil {
		return err
	}
	if _, err := store.DB().ExecContext(ctx, string(migrationTableSQL)); err != nil {
		return err
	}

	entries, err := migrations.Fs.ReadDir(".")
	if err != nil {
		return err
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		if name == "0000_00_00_000000_create_migrations_table.sql" {
			continue
		}
		files = append(files, name)
	}
	sort.Strings(files)

	for _, name := range files {
		if name > maxFileName {
			break
		}
		path := filepath.Join(".", name)
		contents, err := migrations.Fs.ReadFile(path)
		if err != nil {
			return err
		}
		if _, err := store.DB().ExecContext(ctx, string(contents)); err != nil {
			return err
		}
		if _, err := store.DB().ExecContext(ctx, "INSERT INTO migrations (migration, applied_at) VALUES (?, ?)", name, time.Now().UTC()); err != nil {
			return err
		}
	}

	return nil
}
