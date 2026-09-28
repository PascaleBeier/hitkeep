package entitlements

import (
	"context"

	"github.com/google/uuid"
)

// DefaultProvider is the OSS entitlements provider.
// It returns unlimited entitlements for all tenants.
type DefaultProvider struct{}

// NewDefaultProvider creates an OSS entitlements provider with no limits.
func NewDefaultProvider() *DefaultProvider {
	return &DefaultProvider{}
}

// ForTenant returns unlimited entitlements (all zeros = unlimited, all bools = true).
func (p *DefaultProvider) ForTenant(_ context.Context, _ uuid.UUID) (*Entitlements, error) {
	return &Entitlements{
		AllowSSO:                      true,
		AllowCustomBranding:           true,
		AllowExternalReportRecipients: true,
	}, nil
}
