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
