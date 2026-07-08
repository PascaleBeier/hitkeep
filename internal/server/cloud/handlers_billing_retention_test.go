//go:build billing

package cloud

import (
	"context"
	"testing"

	"github.com/google/uuid"
	stripe "github.com/stripe/stripe-go/v84"

	"hitkeep/internal/database"
)

func setupCloudTestHandlerWithTenantStores(t *testing.T) (*handler, *database.Store) {
	t.Helper()
	h, store := setupCloudTestHandler(t)
	h.ctx.TenantStores = database.NewTenantStoreManager(store, t.TempDir())
	return h, store
}

func siteRetentionDaysForTeam(t *testing.T, store *database.Store, tenantID uuid.UUID) int {
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

func TestHandleStripeEventSyncsRetentionOnUpgrade(t *testing.T) {
	h, store := setupCloudTestHandlerWithTenantStores(t)
	defer store.Close()

	account, err := store.CreateManagedCloudAccount(context.Background(), database.CreateManagedCloudAccountInput{
		Email:          "retention-upgrade@example.com",
		HashedPassword: "hashed",
		TeamName:       "Retention Upgrade Team",
	})
	if err != nil {
		t.Fatalf("create managed account: %v", err)
	}
	if _, err := store.CreateSite(context.Background(), account.UserID, "retention-upgrade.example.com"); err != nil {
		t.Fatalf("create site: %v", err)
	}

	event := stripe.Event{
		ID:   "evt_upgrade_pro",
		Type: "checkout.session.completed",
		Data: &stripe.EventData{
			Raw: []byte(`{
				"metadata":{"tenant_id":"` + account.TenantID.String() + `","plan_code":"pro","plan_name":"Pro"},
				"customer":{"id":"cus_live"},
				"subscription":{"id":"sub_live"},
				"status":"complete"
			}`),
		},
	}
	if err := h.handleStripeEvent(context.Background(), event); err != nil {
		t.Fatalf("handle stripe event: %v", err)
	}

	if got := siteRetentionDaysForTeam(t, store, account.TenantID); got != 365 {
		t.Fatalf("expected retention raised to 365 (Pro cap) after upgrade, got %d", got)
	}
}

func TestHandleStripeEventSyncsRetentionOnDowngrade(t *testing.T) {
	h, store := setupCloudTestHandlerWithTenantStores(t)
	defer store.Close()

	account, err := store.CreateManagedCloudAccount(context.Background(), database.CreateManagedCloudAccountInput{
		Email:          "retention-downgrade@example.com",
		HashedPassword: "hashed",
		TeamName:       "Retention Downgrade Team",
	})
	if err != nil {
		t.Fatalf("create managed account: %v", err)
	}
	if _, err := store.CreateSite(context.Background(), account.UserID, "retention-downgrade.example.com"); err != nil {
		t.Fatalf("create site: %v", err)
	}
	if err := store.UpsertCloudBillingAccount(context.Background(), database.CloudBillingAccount{
		TenantID:           account.TenantID,
		PlanCode:           database.CloudPlanBusiness,
		PlanName:           "Business",
		SubscriptionStatus: database.CloudSubscriptionStatusActive,
	}); err != nil {
		t.Fatalf("seed business billing account: %v", err)
	}

	event := stripe.Event{
		ID:   "evt_downgrade",
		Type: "customer.subscription.deleted",
		Data: &stripe.EventData{
			Raw: []byte(`{
				"id":"sub_live",
				"metadata":{"tenant_id":"` + account.TenantID.String() + `","plan_code":"free","plan_name":"Free"},
				"customer":{"id":"cus_live"},
				"status":"canceled"
			}`),
		},
	}
	if err := h.handleStripeEvent(context.Background(), event); err != nil {
		t.Fatalf("handle stripe event: %v", err)
	}

	if got := siteRetentionDaysForTeam(t, store, account.TenantID); got != 60 {
		t.Fatalf("expected retention lowered to 60 (Free cap) after downgrade, got %d", got)
	}
}

func TestHandleStripeEventRetentionSyncPreservesManuallyCustomizedSite(t *testing.T) {
	h, store := setupCloudTestHandlerWithTenantStores(t)
	defer store.Close()

	account, err := store.CreateManagedCloudAccount(context.Background(), database.CreateManagedCloudAccountInput{
		Email:          "retention-manual@example.com",
		HashedPassword: "hashed",
		TeamName:       "Retention Manual Team",
	})
	if err != nil {
		t.Fatalf("create managed account: %v", err)
	}
	site, err := store.CreateSite(context.Background(), account.UserID, "retention-manual.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	if err := store.UpdateSiteRetention(context.Background(), site.ID, account.UserID, 30, false); err != nil {
		t.Fatalf("manually customize retention: %v", err)
	}
	if err := store.UpsertCloudBillingAccount(context.Background(), database.CloudBillingAccount{
		TenantID:           account.TenantID,
		PlanCode:           database.CloudPlanBusiness,
		PlanName:           "Business",
		SubscriptionStatus: database.CloudSubscriptionStatusActive,
	}); err != nil {
		t.Fatalf("seed business billing account: %v", err)
	}

	event := stripe.Event{
		ID:   "evt_downgrade_manual",
		Type: "customer.subscription.deleted",
		Data: &stripe.EventData{
			Raw: []byte(`{
				"id":"sub_live",
				"metadata":{"tenant_id":"` + account.TenantID.String() + `","plan_code":"free","plan_name":"Free"},
				"customer":{"id":"cus_live"},
				"status":"canceled"
			}`),
		},
	}
	if err := h.handleStripeEvent(context.Background(), event); err != nil {
		t.Fatalf("handle stripe event: %v", err)
	}

	if got := siteRetentionDaysForTeam(t, store, account.TenantID); got != 30 {
		t.Fatalf("expected manually-customized retention (30, under Free's 60 cap) preserved, got %d", got)
	}
}

func TestHandleStripeEventRetentionSyncHarmlessOnPaymentFailure(t *testing.T) {
	h, store := setupCloudTestHandlerWithTenantStores(t)
	defer store.Close()

	account, err := store.CreateManagedCloudAccount(context.Background(), database.CreateManagedCloudAccountInput{
		Email:          "retention-invoice@example.com",
		HashedPassword: "hashed",
		TeamName:       "Retention Invoice Team",
	})
	if err != nil {
		t.Fatalf("create managed account: %v", err)
	}
	if _, err := store.CreateSite(context.Background(), account.UserID, "retention-invoice.example.com"); err != nil {
		t.Fatalf("create site: %v", err)
	}
	if err := store.UpsertCloudBillingAccount(context.Background(), database.CloudBillingAccount{
		TenantID:           account.TenantID,
		PlanCode:           database.CloudPlanPro,
		PlanName:           "Pro",
		SubscriptionStatus: database.CloudSubscriptionStatusActive,
	}); err != nil {
		t.Fatalf("seed pro billing account: %v", err)
	}

	before := siteRetentionDaysForTeam(t, store, account.TenantID)

	event := stripe.Event{
		ID:   "evt_invoice_failed",
		Type: "invoice.payment_failed",
		Data: &stripe.EventData{
			Raw: []byte(`{
				"customer":{"id":"cus_live"},
				"subscription":{"id":"sub_live"},
				"metadata":{"tenant_id":"` + account.TenantID.String() + `"}
			}`),
		},
	}
	if err := h.handleStripeEvent(context.Background(), event); err != nil {
		t.Fatalf("handle stripe event: %v", err)
	}

	if got := siteRetentionDaysForTeam(t, store, account.TenantID); got != before {
		t.Fatalf("expected no retention mutation on payment failure, before=%d after=%d", before, got)
	}
}

func TestHandleStripeEventSucceedsWithoutTenantStores(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	defer store.Close()

	account, err := store.CreateManagedCloudAccount(context.Background(), database.CreateManagedCloudAccountInput{
		Email:          "retention-no-tenant-stores@example.com",
		HashedPassword: "hashed",
		TeamName:       "No Tenant Stores Team",
	})
	if err != nil {
		t.Fatalf("create managed account: %v", err)
	}

	event := stripe.Event{
		ID:   "evt_no_tenant_stores",
		Type: "checkout.session.completed",
		Data: &stripe.EventData{
			Raw: []byte(`{
				"metadata":{"tenant_id":"` + account.TenantID.String() + `","plan_code":"pro","plan_name":"Pro"},
				"customer":{"id":"cus_live"},
				"subscription":{"id":"sub_live"},
				"status":"complete"
			}`),
		},
	}

	if err := h.handleStripeEvent(context.Background(), event); err != nil {
		t.Fatalf("handle stripe event should succeed with nil TenantStores: %v", err)
	}
}
