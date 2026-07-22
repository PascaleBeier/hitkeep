//go:build billing

package entitlements

import (
	"testing"

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
