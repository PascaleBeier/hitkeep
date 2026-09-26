//go:build billing

package entitlements

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"hitkeep/config"
	"hitkeep/internal/database"
)

func TestEffectiveCloudPlanMapsSubscriptionStatuses(t *testing.T) {
	freeStatuses := []string{
		"",
		database.CloudSubscriptionStatusFree,
		database.CloudSubscriptionStatusPendingCheckout,
		database.CloudSubscriptionStatusCanceled,
		database.CloudSubscriptionStatusChargebackLost,
		database.CloudSubscriptionStatusUnpaid,
		database.CloudSubscriptionStatusPaused,
		database.CloudSubscriptionStatusIncomplete,
		database.CloudSubscriptionStatusIncompleteExpired,
	}
	paidStatuses := []string{
		database.CloudSubscriptionStatusActive,
		"trialing",
		database.CloudSubscriptionStatusPastDue,
		database.CloudSubscriptionStatusDisputed,
		// checkout.session.completed stores the session status verbatim.
		"complete",
		// Unknown statuses must keep the paid plan: misclassifying a paying
		// team as Free would destructively trim its retained data.
		"some_future_stripe_status",
	}

	for _, status := range freeStatuses {
		account := &database.CloudBillingAccount{
			PlanCode:           database.CloudPlanBusiness,
			PlanName:           "Business",
			SubscriptionStatus: status,
		}
		if code, name := EffectiveCloudPlan(account); code != database.CloudPlanFree || name != "Free" {
			t.Errorf("status %q: expected free plan, got code=%q name=%q", status, code, name)
		}
	}
	for _, status := range paidStatuses {
		account := &database.CloudBillingAccount{
			PlanCode:           database.CloudPlanBusiness,
			PlanName:           "Business",
			SubscriptionStatus: status,
		}
		if code, name := EffectiveCloudPlan(account); code != database.CloudPlanBusiness || name != "Business" {
			t.Errorf("status %q: expected business plan kept, got code=%q name=%q", status, code, name)
		}
	}
}

func TestCloudPlanEntitlementsAskAIDailyLimits(t *testing.T) {
	for _, tc := range []struct {
		code  string
		limit int
	}{{database.CloudPlanFree, 1}, {database.CloudPlanPro, 100}, {database.CloudPlanBusiness, 500}} {
		t.Run(tc.code, func(t *testing.T) {
			ent := CloudPlanEntitlements(tc.code)
			if ent == nil || ent.MaxAskAIAnswersPerDay != tc.limit {
				t.Fatalf("expected Ask AI daily limit %d, got %+v", tc.limit, ent)
			}
		})
	}
}

func TestCloudProviderAskAIFallbackLimits(t *testing.T) {
	for _, tc := range []struct {
		code  string
		limit int
	}{{"", 1}, {database.CloudPlanFree, 1}, {database.CloudPlanPro, 100}, {database.CloudPlanBusiness, 500}, {"unknown", 1}} {
		t.Run(tc.code, func(t *testing.T) {
			provider := NewProvider(&config.Config{CloudHosted: true, CloudPlanCode: tc.code})
			ent, err := provider.ForTenant(context.Background(), uuid.Nil)
			if err != nil || ent == nil || ent.MaxAskAIAnswersPerDay != tc.limit {
				t.Fatalf("expected provider Ask AI daily limit %d, got %+v, err=%v", tc.limit, ent, err)
			}
		})
	}
}

func TestCloudPlanEntitlementsGateSSOToBusiness(t *testing.T) {
	free := CloudPlanEntitlements(database.CloudPlanFree)
	pro := CloudPlanEntitlements(database.CloudPlanPro)
	business := CloudPlanEntitlements(database.CloudPlanBusiness)

	if free == nil || free.AllowSSO {
		t.Fatalf("expected Free to exclude SSO, got %+v", free)
	}
	if pro == nil || pro.AllowSSO {
		t.Fatalf("expected Pro to exclude SSO, got %+v", pro)
	}
	if business == nil || !business.AllowSSO {
		t.Fatalf("expected Business to include SSO, got %+v", business)
	}
}

func TestCloudPlanEntitlementsGateExternalReportRecipientsToPaidPlans(t *testing.T) {
	free := CloudPlanEntitlements(database.CloudPlanFree)
	pro := CloudPlanEntitlements(database.CloudPlanPro)
	business := CloudPlanEntitlements(database.CloudPlanBusiness)

	if free == nil || free.AllowExternalReportRecipients {
		t.Fatalf("expected Free to exclude external report recipients, got %+v", free)
	}
	if pro == nil || !pro.AllowExternalReportRecipients {
		t.Fatalf("expected Pro to include external report recipients, got %+v", pro)
	}
	if business == nil || !business.AllowExternalReportRecipients {
		t.Fatalf("expected Business to include external report recipients, got %+v", business)
	}
}
