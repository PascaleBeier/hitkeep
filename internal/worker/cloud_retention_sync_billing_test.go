//go:build billing

package worker

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/config"
	"hitkeep/internal/database"
	"hitkeep/internal/entitlements"
)

func newCloudRetentionSyncEntitlementsService(store *database.Store) *entitlements.Service {
	return entitlements.NewService(store, entitlements.NewProvider(cloudRetentionSyncWorkerConfig()), cloudRetentionSyncWorkerConfig())
}

func cloudRetentionSyncWorkerConfig() *config.Config {
	return &config.Config{
		CloudHosted:           true,
		CloudMaxRetentionDays: 60,
	}
}

func getSiteRetentionDays(t *testing.T, store *database.Store, tenantID uuid.UUID) int {
	t.Helper()
	sites, err := store.ListSitesForTenant(context.Background(), tenantID)
	if err != nil {
		t.Fatalf("list sites for tenant: %v", err)
	}
	if len(sites) == 0 {
		t.Fatalf("no sites found for tenant %s", tenantID)
	}
	return sites[0].DataRetentionDays
}

func TestCloudRetentionSyncWorkerClampsFreePlanSiteToCap(t *testing.T) {
	store, mgr := setupCloudLifecycleWorkerStore(t)
	team := seedWorkerCloudLifecycleTeam(t, store, "free@example.com", "free.example.com", database.CloudPlanFree, database.CloudSubscriptionStatusFree, nil)

	// Simulate a stale value predating this feature.
	if _, err := store.DB().ExecContext(context.Background(),
		"UPDATE sites SET data_retention_days = 365 WHERE user_id = ?", team.UserID,
	); err != nil {
		t.Fatalf("seed stale retention: %v", err)
	}

	ent := newCloudRetentionSyncEntitlementsService(store)
	worker := NewCloudRetentionSyncWorker(mgr, ent, cloudRetentionSyncWorkerConfig())
	worker.RunAt(context.Background(), time.Now().UTC())

	if got := getSiteRetentionDays(t, store, team.TenantID); got != 60 {
		t.Fatalf("expected free plan site clamped to 60, got %d", got)
	}
}

func TestCloudRetentionSyncWorkerRaisesPlanManagedSiteToExactBusinessCap(t *testing.T) {
	store, mgr := setupCloudLifecycleWorkerStore(t)
	team := seedWorkerCloudLifecycleTeam(t, store, "biz@example.com", "biz.example.com", database.CloudPlanBusiness, database.CloudSubscriptionStatusActive, nil)

	// Plan-managed sites always sync to exactly the plan cap, in both
	// directions - a stale lower value must be raised, not left alone.
	if _, err := store.DB().ExecContext(context.Background(),
		"UPDATE sites SET data_retention_days = 200 WHERE user_id = ?", team.UserID,
	); err != nil {
		t.Fatalf("seed retention: %v", err)
	}

	ent := newCloudRetentionSyncEntitlementsService(store)
	worker := NewCloudRetentionSyncWorker(mgr, ent, cloudRetentionSyncWorkerConfig())
	worker.RunAt(context.Background(), time.Now().UTC())

	if got := getSiteRetentionDays(t, store, team.TenantID); got != 1095 {
		t.Fatalf("expected plan-managed business site raised to exactly 1095, got %d", got)
	}
}

func TestCloudRetentionSyncWorkerLeavesManuallyCustomizedSiteUnderCapUnchanged(t *testing.T) {
	store, mgr := setupCloudLifecycleWorkerStore(t)
	team := seedWorkerCloudLifecycleTeam(t, store, "biz2@example.com", "biz2.example.com", database.CloudPlanBusiness, database.CloudSubscriptionStatusActive, nil)

	if _, err := store.DB().ExecContext(context.Background(),
		"UPDATE sites SET data_retention_days = 200, retention_synced_from_plan = FALSE WHERE user_id = ?", team.UserID,
	); err != nil {
		t.Fatalf("seed retention: %v", err)
	}

	ent := newCloudRetentionSyncEntitlementsService(store)
	worker := NewCloudRetentionSyncWorker(mgr, ent, cloudRetentionSyncWorkerConfig())
	worker.RunAt(context.Background(), time.Now().UTC())

	if got := getSiteRetentionDays(t, store, team.TenantID); got != 200 {
		t.Fatalf("expected manually-customized site (200 <= 1095) unchanged, got %d", got)
	}
}

func TestCloudRetentionSyncWorkerAppliesStaticCapForLegacyTeamWithoutBillingAccount(t *testing.T) {
	store, mgr := setupCloudLifecycleWorkerStore(t)

	ctx := context.Background()
	account, err := store.CreateManagedCloudAccount(ctx, database.CreateManagedCloudAccountInput{
		Email:          "legacy@example.com",
		HashedPassword: "hashed",
		TeamName:       "legacy@example.com",
		Locale:         "en",
	})
	if err != nil {
		t.Fatalf("create managed cloud account: %v", err)
	}
	site, err := store.CreateSite(ctx, account.UserID, "legacy.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx,
		"UPDATE sites SET data_retention_days = 3650 WHERE id = ?", site.ID,
	); err != nil {
		t.Fatalf("seed retention: %v", err)
	}

	ent := newCloudRetentionSyncEntitlementsService(store)
	worker := NewCloudRetentionSyncWorker(mgr, ent, cloudRetentionSyncWorkerConfig())
	worker.RunAt(ctx, time.Now().UTC())

	if got := getSiteRetentionDays(t, store, account.TenantID); got != 60 {
		t.Fatalf("expected legacy team clamped to static config default 60, got %d", got)
	}
}

func TestCloudRetentionSyncWorkerNoOpWhenNotCloudHosted(t *testing.T) {
	store, mgr := setupCloudLifecycleWorkerStore(t)
	team := seedWorkerCloudLifecycleTeam(t, store, "free2@example.com", "free2.example.com", database.CloudPlanFree, database.CloudSubscriptionStatusFree, nil)
	if _, err := store.DB().ExecContext(context.Background(),
		"UPDATE sites SET data_retention_days = 365 WHERE user_id = ?", team.UserID,
	); err != nil {
		t.Fatalf("seed retention: %v", err)
	}

	conf := &config.Config{CloudHosted: false}
	ent := entitlements.NewService(store, entitlements.NewProvider(conf), conf)
	worker := NewCloudRetentionSyncWorker(mgr, ent, conf)
	worker.RunAt(context.Background(), time.Now().UTC())

	if got := getSiteRetentionDays(t, store, team.TenantID); got != 365 {
		t.Fatalf("expected no-op when not cloud hosted, got %d", got)
	}
}

func TestCloudRetentionSyncWorkerIdempotentReRun(t *testing.T) {
	store, mgr := setupCloudLifecycleWorkerStore(t)
	team := seedWorkerCloudLifecycleTeam(t, store, "idem@example.com", "idem.example.com", database.CloudPlanFree, database.CloudSubscriptionStatusFree, nil)
	if _, err := store.DB().ExecContext(context.Background(),
		"UPDATE sites SET data_retention_days = 365 WHERE user_id = ?", team.UserID,
	); err != nil {
		t.Fatalf("seed retention: %v", err)
	}

	ent := newCloudRetentionSyncEntitlementsService(store)
	worker := NewCloudRetentionSyncWorker(mgr, ent, cloudRetentionSyncWorkerConfig())
	now := time.Now().UTC()
	worker.RunAt(context.Background(), now)
	worker.RunAt(context.Background(), now)

	if got := getSiteRetentionDays(t, store, team.TenantID); got != 60 {
		t.Fatalf("expected idempotent clamp to 60, got %d", got)
	}
}
