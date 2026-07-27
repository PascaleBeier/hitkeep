package controlstore

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

// GetUserOnboardingWithActivity keeps all metadata reads in SQLite while the
// caller resolves the two tenant-analytics activity signals.
func (s *Store) GetUserOnboardingWithActivity(
	ctx context.Context,
	userID uuid.UUID,
	resolve func(context.Context, uuid.UUID) (*api.SiteTrackingStatus, error),
) (*api.UserOnboarding, error) {
	if resolve == nil {
		return nil, fmt.Errorf("onboarding activity resolver is required")
	}
	prefs, err := s.GetUserPreferences(ctx, userID)
	if err != nil {
		return nil, err
	}
	sites, err := s.GetSites(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list onboarding sites: %w", err)
	}
	activeTenantID, err := s.GetActiveTenantID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("resolve onboarding active tenant: %w", err)
	}
	memberCount, err := s.CountTeamMembers(ctx, activeTenantID)
	if err != nil {
		return nil, err
	}
	pendingInviteCount, err := s.CountPendingTeamInvites(ctx, activeTenantID)
	if err != nil {
		return nil, err
	}
	firstSiteID, firstSiteDomain := "", ""
	if len(sites) > 0 {
		firstSiteID, firstSiteDomain = sites[0].ID.String(), sites[0].Domain
	}
	receivedFirstHit, automaticEventSeen := false, false
	for _, site := range sites {
		status, err := resolve(ctx, site.ID)
		if err != nil {
			return nil, fmt.Errorf("resolve onboarding analytics activity: %w", err)
		}
		if status != nil && status.FirstHitAt != nil {
			receivedFirstHit = true
		}
		if status != nil && status.LastAutomaticEventAt != nil {
			automaticEventSeen = true
		}
	}
	var reportScheduled bool
	if err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM report_definitions rd
			WHERE rd.status = 'active'
			  AND (rd.owner_user_id = ? OR EXISTS (
				  SELECT 1 FROM tenant_members tm WHERE tm.tenant_id = rd.tenant_id
				  AND tm.user_id = ? AND lower(tm.role) IN ('owner', 'admin')
			  ))
			  AND EXISTS (
				  SELECT 1 FROM report_recipients rr WHERE rr.report_id = rd.id
				  AND rr.opted_out_at IS NULL AND (rr.user_id IS NOT NULL OR
				  (rr.external_email IS NOT NULL AND rr.confirmed_at IS NOT NULL
				   AND rr.consent_version = rd.consent_version))
			  )
		)
	`, userID, userID).Scan(&reportScheduled); err != nil {
		return nil, fmt.Errorf("check scheduled report onboarding: %w", err)
	}
	steps := []api.OnboardingStep{
		{Key: "create_site", Complete: len(sites) > 0, Current: len(sites), Target: 1, SiteID: firstSiteID, SiteDomain: firstSiteDomain},
		{Key: "verify_tracking", Complete: receivedFirstHit, Current: boolToInt(receivedFirstHit), Target: 1, SiteID: firstSiteID, SiteDomain: firstSiteDomain},
		{Key: "automatic_events", Complete: automaticEventSeen, Current: boolToInt(automaticEventSeen), Target: 1, SiteID: firstSiteID, SiteDomain: firstSiteDomain},
		{Key: "invite_teammate", Complete: memberCount > 1 || pendingInviteCount > 0, Current: memberCount + pendingInviteCount, Target: 2},
		{Key: "schedule_report", Complete: reportScheduled, Current: boolToInt(reportScheduled), Target: 1},
	}
	complete := true
	for _, step := range steps {
		complete = complete && step.Complete
	}
	return &api.UserOnboarding{Dismissed: prefs != nil && prefs.DismissedOnboardingAt != nil, Complete: complete, Steps: steps}, nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
