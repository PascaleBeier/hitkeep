package entitlements

import (
	"context"

	"github.com/google/uuid"
)

// Entitlements describes what a tenant is allowed to do.
// Zero values mean unlimited for integer fields.
type Entitlements struct {
	MaxTeams                      int // 0 = unlimited
	MaxSitesPerTeam               int // 0 = unlimited
	MaxRetentionDays              int // 0 = unlimited
	MaxTeamMembers                int // 0 = unlimited
	AllowSSO                      bool
	AllowCustomBranding           bool
	AllowExternalReportRecipients bool
}

type PlanInfo struct {
	Code       string
	Name       string
	UpgradeURL string
	SupportURL string
}

// Provider resolves entitlements for a given tenant.
// The OSS build uses DefaultProvider (unlimited). A cloud build tag can
// swap in a provider that reads from a billing/subscription backend.
type Provider interface {
	ForTenant(ctx context.Context, tenantID uuid.UUID) (*Entitlements, error)
}

type Describer interface {
	DescribeTenant(ctx context.Context, tenantID uuid.UUID) (*PlanInfo, error)
}
