//go:build billing

package entitlements

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"hitkeep/internal/database"
)

const (
	subscriptionStatusPendingCheckout = "pending_checkout"
	subscriptionStatusCanceled        = "canceled"
)

// EffectiveCloudPlan resolves the billing account's effective plan code and
// name. Accounts without an active subscription fall back to the free plan.
func EffectiveCloudPlan(account *database.CloudBillingAccount) (string, string) {
	if account == nil {
		return "", ""
	}

	switch strings.TrimSpace(account.SubscriptionStatus) {
	case "", database.CloudSubscriptionStatusFree, subscriptionStatusPendingCheckout, subscriptionStatusCanceled, database.CloudSubscriptionStatusChargebackLost:
		return database.CloudPlanFree, CloudPlanName(database.CloudPlanFree)
	default:
		return strings.TrimSpace(account.PlanCode), strings.TrimSpace(account.PlanName)
	}
}

// CloudPlanName returns the display name for a managed-cloud plan code.
func CloudPlanName(code string) string {
	switch code {
	case database.CloudPlanBusiness:
		return "Business"
	case database.CloudPlanPro:
		return "Pro"
	default:
		return "Free"
	}
}

// CloudPlanEntitlements is the canonical plan-to-entitlements table for
// managed-cloud plans. Returns nil for unknown codes.
func CloudPlanEntitlements(code string) *Entitlements {
	switch code {
	case database.CloudPlanBusiness:
		return &Entitlements{
			MaxSitesPerTeam:     50,
			MaxTeamMembers:      20,
			MaxRetentionDays:    1095,
			AllowSSO:            true,
			AllowCustomBranding: true,
		}
	case database.CloudPlanPro:
		return &Entitlements{
			MaxSitesPerTeam:  10,
			MaxTeamMembers:   5,
			MaxRetentionDays: 365,
		}
	case database.CloudPlanFree:
		return &Entitlements{
			MaxSitesPerTeam:  3,
			MaxTeamMembers:   3,
			MaxRetentionDays: 60,
		}
	default:
		return nil
	}
}

func (s *Service) cloudBillingAccount(ctx context.Context, teamID uuid.UUID) *database.CloudBillingAccount {
	if s.store == nil {
		return nil
	}
	account, err := s.store.GetCloudBillingAccount(ctx, teamID)
	if err != nil {
		return nil
	}
	return account
}

func (s *Service) cloudBillingTeamPlan(ctx context.Context, teamID uuid.UUID) *PlanInfo {
	account := s.cloudBillingAccount(ctx, teamID)
	if account == nil {
		return nil
	}
	code, name := EffectiveCloudPlan(account)
	return &PlanInfo{Code: code, Name: name}
}

func (s *Service) cloudBillingTeamEntitlements(ctx context.Context, teamID uuid.UUID) *Entitlements {
	account := s.cloudBillingAccount(ctx, teamID)
	if account == nil {
		return nil
	}
	code, _ := EffectiveCloudPlan(account)
	return CloudPlanEntitlements(code)
}
