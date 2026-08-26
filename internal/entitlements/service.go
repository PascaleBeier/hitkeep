package entitlements

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"hitkeep/config"
	"hitkeep/internal/database"
)

// PlanCodeFree is the managed-cloud free plan code. It mirrors
// database.CloudPlanFree, which is only compiled in billing builds.
const PlanCodeFree = "free"

// Managed-cloud limit violations reported by Service.
var (
	ErrTeamLimitReached           = errors.New("team limit reached")
	ErrTeamMemberLimitReached     = errors.New("team member limit reached")
	ErrTeamMembershipLimitReached = errors.New("team membership limit reached")
)

// Service owns managed-cloud plan and account limit policy. Handlers ask it
// the questions their gates need answered instead of re-deriving plan rules.
// Instance admins and owners bypass every limit it enforces.
type Service struct {
	store    *database.Store
	provider Provider
	cfg      *config.Config
}

// NewService assembles the limits policy service. All dependencies are read
// lazily, so callers may construct it per request from live configuration.
func NewService(store *database.Store, provider Provider, cfg *config.Config) *Service {
	return &Service{store: store, provider: provider, cfg: cfg}
}

func (s *Service) cloudHosted() bool {
	return s.cfg != nil && s.cfg.CloudHosted
}

// BypassesCloudLimits reports whether the user holds an instance role (admin
// or owner) that is exempt from managed-cloud limitations. Always false on
// self-hosted deployments, which have no such limits.
func (s *Service) BypassesCloudLimits(ctx context.Context, userID uuid.UUID) bool {
	if !s.cloudHosted() || s.store == nil || userID == uuid.Nil {
		return false
	}
	role, err := s.store.GetInstanceRole(ctx, userID)
	return err == nil && role.BypassesCloudLimits()
}

// TeamBypassesCloudLimits reports whether the team owner is an instance
// operator who is exempt from managed-cloud team limits. Unlike
// BypassesCloudLimits, this is intentionally actor-independent so public
// routes can apply the same team policy as authenticated dashboard routes.
func (s *Service) TeamBypassesCloudLimits(ctx context.Context, teamID uuid.UUID) bool {
	if !s.cloudHosted() || s.store == nil || teamID == uuid.Nil {
		return false
	}
	owned, err := s.store.IsTeamOwnedByOperator(ctx, teamID)
	return err == nil && owned
}

// TeamPlan resolves the team's current plan, preferring the cloud billing
// account over the entitlements provider. Returns nil when no plan applies.
func (s *Service) TeamPlan(ctx context.Context, teamID uuid.UUID) *PlanInfo {
	if plan := s.cloudBillingTeamPlan(ctx, teamID); plan != nil {
		return plan
	}

	describer, ok := s.provider.(Describer)
	if !ok || describer == nil {
		return nil
	}
	plan, err := describer.DescribeTenant(ctx, teamID)
	if err != nil || plan == nil {
		return nil
	}

	code := strings.TrimSpace(plan.Code)
	name := strings.TrimSpace(plan.Name)
	if code == "" && name == "" {
		return nil
	}
	return &PlanInfo{
		Code:       code,
		Name:       name,
		UpgradeURL: strings.TrimSpace(plan.UpgradeURL),
		SupportURL: strings.TrimSpace(plan.SupportURL),
	}
}

// TeamEntitlements resolves the team's effective entitlements, preferring the
// cloud billing account over the entitlements provider. Teams without either
// get permissive defaults.
func (s *Service) TeamEntitlements(ctx context.Context, teamID uuid.UUID) *Entitlements {
	if ent := s.cloudBillingTeamEntitlements(ctx, teamID); ent != nil {
		return ent
	}
	if s.provider != nil {
		if resolved, err := s.provider.ForTenant(ctx, teamID); err == nil && resolved != nil {
			return resolved
		}
	}
	return &Entitlements{
		AllowSSO:                      true,
		AllowCustomBranding:           true,
		AllowExternalReportRecipients: true,
	}
}

// AllowsCustomTrackingDomains reports whether the actor may manage custom
// tracking domains for the team. Free cloud teams must upgrade to Pro or
// higher; self-hosted deployments are never gated.
func (s *Service) AllowsCustomTrackingDomains(ctx context.Context, actorID, teamID uuid.UUID) bool {
	if !s.cloudHosted() {
		return true
	}
	if s.BypassesCloudLimits(ctx, actorID) {
		return true
	}
	plan := s.TeamPlan(ctx, teamID)
	return plan == nil || plan.Code != PlanCodeFree
}

// AllowsSSO reports whether the actor may use SSO for the team. Managed cloud
// plans expose this entitlement only on Business; self-hosted deployments are
// never gated. Passing uuid.Nil omits the actor bypass while still honoring an
// operator-owned team's team-level exemption, as required by public login
// routes.
func (s *Service) AllowsSSO(ctx context.Context, actorID, teamID uuid.UUID) bool {
	if !s.cloudHosted() {
		return true
	}
	if s.BypassesCloudLimits(ctx, actorID) || s.TeamBypassesCloudLimits(ctx, teamID) {
		return true
	}
	ent := s.TeamEntitlements(ctx, teamID)
	return ent != nil && ent.AllowSSO
}

// AllowsExternalReportRecipients reports whether the actor may invite and
// deliver scheduled reports to addresses outside the team. Managed Cloud
// exposes this entitlement on Pro and Business; self-hosted deployments are
// never gated. Passing uuid.Nil enforces the team entitlement without an
// instance-role bypass, as required by the background delivery worker.
func (s *Service) AllowsExternalReportRecipients(ctx context.Context, actorID, teamID uuid.UUID) bool {
	if !s.cloudHosted() {
		return true
	}
	if s.BypassesCloudLimits(ctx, actorID) {
		return true
	}
	ent := s.TeamEntitlements(ctx, teamID)
	return ent != nil && ent.AllowExternalReportRecipients
}

// RequireTeamMemberCapacity returns ErrTeamMemberLimitReached when the team's
// members plus pending invites have reached the plan's member limit.
func (s *Service) RequireTeamMemberCapacity(ctx context.Context, teamID uuid.UUID) error {
	if s.TeamBypassesCloudLimits(ctx, teamID) {
		return nil
	}
	ent := s.TeamEntitlements(ctx, teamID)
	if ent == nil || ent.MaxTeamMembers <= 0 {
		return nil
	}

	memberCount, err := s.store.CountTeamMembers(ctx, teamID)
	if err != nil {
		return err
	}
	pendingInviteCount, err := s.store.CountPendingTeamInvites(ctx, teamID)
	if err != nil {
		return err
	}
	if memberCount+pendingInviteCount >= ent.MaxTeamMembers {
		return ErrTeamMemberLimitReached
	}
	return nil
}

// CanCreateTeam reports whether the actor may create another team, enforcing
// the per-user MaxTeams entitlement (ErrTeamLimitReached). Resolution
// problems fail open, matching the tolerant behavior of the handlers this
// policy replaced.
func (s *Service) CanCreateTeam(ctx context.Context, actorID uuid.UUID) error {
	if s.BypassesCloudLimits(ctx, actorID) {
		return nil
	}
	maxTeams := s.userMaxTeams(ctx, actorID)
	if maxTeams <= 0 {
		return nil
	}
	teams, _, err := s.store.ListUserTeams(ctx, actorID)
	if err != nil {
		return nil
	}
	if len(teams) >= maxTeams {
		return ErrTeamLimitReached
	}
	return nil
}

// RequireTeamMembershipCapacity returns ErrTeamMembershipLimitReached when the
// user's non-default team memberships plus `additional` new ones would exceed
// the per-user MaxTeams entitlement. Only managed cloud caps memberships;
// self-hosted deployments and instance staff are never capped.
func (s *Service) RequireTeamMembershipCapacity(ctx context.Context, userID uuid.UUID, additional int) error {
	if !s.cloudHosted() {
		return nil
	}
	if s.BypassesCloudLimits(ctx, userID) {
		return nil
	}
	maxTeams := s.userMaxTeams(ctx, userID)
	if maxTeams <= 0 {
		return nil
	}
	teamCount, err := s.store.CountUserNonDefaultTeams(ctx, userID)
	if err != nil {
		return fmt.Errorf("count user teams: %w", err)
	}
	if teamCount+additional > maxTeams {
		return ErrTeamMembershipLimitReached
	}
	return nil
}

// userMaxTeams resolves the per-user team cap from the deployment-level
// provider. Deliberately NOT the team's billing-plan override: billing is per
// team, so no single team's plan can govern how many teams a user may have.
// Zero means unlimited.
func (s *Service) userMaxTeams(ctx context.Context, userID uuid.UUID) int {
	if s.provider == nil {
		return 0
	}
	activeTenantID, err := s.store.GetActiveTenantID(ctx, userID)
	if err != nil {
		return 0
	}
	ent, err := s.provider.ForTenant(ctx, activeTenantID)
	if err != nil || ent == nil {
		return 0
	}
	return ent.MaxTeams
}
