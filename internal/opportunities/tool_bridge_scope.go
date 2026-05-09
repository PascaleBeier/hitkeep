package opportunities

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	hitai "hitkeep/internal/ai"
	"hitkeep/internal/auth"
)

type toolBridgeScope struct {
	config ToolBridgeConfig
}

func newToolBridgeScope(config ToolBridgeConfig) toolBridgeScope {
	return toolBridgeScope{config: config}
}

func (s toolBridgeScope) authorize(ctx context.Context) error {
	if s.config.Shared == nil || s.config.Analytics == nil {
		return fmt.Errorf("tool bridge unavailable")
	}
	if err := s.authorizeConfiguredSiteScope(ctx); err != nil {
		return err
	}
	switch s.config.ActorType {
	case "ai_scheduler":
		return s.authorizeScheduler()
	case "api_client":
		return s.authorizeAPIClient()
	default:
		return s.authorizeUser()
	}
}

func (s toolBridgeScope) authorizeConfiguredSiteScope(ctx context.Context) error {
	siteTeamID, err := s.config.Shared.GetSiteTenantID(ctx, s.config.SiteID)
	if err != nil || siteTeamID != s.config.TeamID {
		return hitai.ErrAccessDenied
	}
	return nil
}

func (s toolBridgeScope) authorizeScheduler() error {
	if s.config.SchedulerTeamID != s.config.TeamID || s.config.SchedulerSiteID != s.config.SiteID {
		return hitai.ErrAccessDenied
	}
	return nil
}

func (s toolBridgeScope) authorizeUser() error {
	if s.config.ActorID == uuid.Nil {
		return hitai.ErrAccessDenied
	}
	if s.config.APIClientAuth != nil {
		return hitai.ErrAccessDenied
	}
	if s.config.EffectiveUserID != s.config.ActorID {
		return hitai.ErrAccessDenied
	}
	if !s.hasEffectiveSiteView() {
		return hitai.ErrAccessDenied
	}
	return nil
}

func (s toolBridgeScope) authorizeAPIClient() error {
	apiClient := s.config.APIClientAuth
	if apiClient == nil || apiClient.ClientID == uuid.Nil || apiClient.ClientID != s.config.ActorID {
		return hitai.ErrAccessDenied
	}
	if apiClient.TenantID != uuid.Nil && apiClient.TenantID != s.config.TeamID {
		return hitai.ErrAccessDenied
	}
	if apiClient.UserID != s.config.EffectiveUserID {
		return hitai.ErrAccessDenied
	}
	if !s.hasEffectiveSiteView() {
		return hitai.ErrAccessDenied
	}
	delegatedRole, hasSiteDelegation := apiClient.SiteRoles[s.config.SiteID]
	if hasSiteDelegation && delegatedRole.HasPermission(auth.PermSiteView) {
		return nil
	}
	if !hasSiteDelegation && apiClient.InstanceRole.HasPermission(auth.PermSiteView) {
		return nil
	}
	return hitai.ErrAccessDenied
}

func (s toolBridgeScope) hasEffectiveSiteView() bool {
	return s.config.EffectiveInstanceRole.HasPermission(auth.PermSiteView) ||
		s.config.EffectiveSiteRole.HasPermission(auth.PermSiteView)
}
