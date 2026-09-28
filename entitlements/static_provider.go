package entitlements

import (
	"context"

	"github.com/google/uuid"
)

type StaticProvider struct {
	entitlements Entitlements
	plan         PlanInfo
}

func NewStaticProvider(ent Entitlements, plan PlanInfo) *StaticProvider {
	return &StaticProvider{
		entitlements: ent,
		plan:         plan,
	}
}

func (p *StaticProvider) ForTenant(_ context.Context, _ uuid.UUID) (*Entitlements, error) {
	ent := p.entitlements
	return &ent, nil
}

func (p *StaticProvider) DescribeTenant(_ context.Context, _ uuid.UUID) (*PlanInfo, error) {
	plan := p.plan
	return &plan, nil
}
