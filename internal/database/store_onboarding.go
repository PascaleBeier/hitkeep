package database

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

func (s *Store) GetUserOnboarding(ctx context.Context, userID uuid.UUID) (*api.UserOnboarding, error) {
	return s.GetUserOnboardingWithResolver(ctx, userID, func(context.Context, uuid.UUID) (*Store, error) {
		return s, nil
	})
}

// GetUserOnboardingWithResolver keeps control-plane onboarding metadata on the
// control store while resolving site activity from the owning tenant store.
func (s *Store) GetUserOnboardingWithResolver(ctx context.Context, userID uuid.UUID, resolve func(context.Context, uuid.UUID) (*Store, error)) (*api.UserOnboarding, error) {
	if resolve == nil {
		resolve = func(context.Context, uuid.UUID) (*Store, error) { return s, nil }
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
	firstSiteID := ""
	firstSiteDomain := ""
	if len(sites) > 0 {
		firstSiteID = sites[0].ID.String()
		firstSiteDomain = sites[0].Domain
	}

	receivedFirstHit, automaticEventSeen := false, false
	for _, site := range sites {
		analyticsStore, err := resolve(ctx, site.ID)
		if err != nil {
			return nil, fmt.Errorf("resolve onboarding analytics store: %w", err)
		}
		status, err := analyticsStore.GetSiteTrackingStatus(ctx, site.ID, nowUTC())
		if err != nil {
			return nil, err
		}
		if status != nil && status.FirstHitAt != nil {
			receivedFirstHit = true
			if firstSiteID == "" {
				firstSiteID = site.ID.String()
				firstSiteDomain = site.Domain
			}
		}
		if status != nil && status.LastAutomaticEventAt != nil {
			automaticEventSeen = true
		}
	}

	reportScheduled := false
	if err := s.db.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM report_definitions rd
				WHERE rd.status = 'active'
				  AND (
				      rd.owner_user_id = ?
				      OR EXISTS (
				          SELECT 1 FROM tenant_members tm
				          WHERE tm.tenant_id = rd.tenant_id AND tm.user_id = ?
				            AND lower(tm.role) IN ('owner', 'admin')
				      )
				  )
				  AND EXISTS (
				      SELECT 1 FROM report_recipients rr
				      WHERE rr.report_id = rd.id AND rr.opted_out_at IS NULL
				        AND (
				            rr.user_id IS NOT NULL
				            OR (rr.external_email IS NOT NULL AND rr.confirmed_at IS NOT NULL
				                AND rr.consent_version = rd.consent_version)
				        )
				  )
			)
	`, userID, userID).Scan(&reportScheduled); err != nil {
		return nil, fmt.Errorf("check scheduled report onboarding: %w", err)
	}

	steps := []api.OnboardingStep{
		{Key: "create_site", Complete: len(sites) > 0, Current: len(sites), Target: 1, SiteID: firstSiteID, SiteDomain: firstSiteDomain},
		{Key: "verify_tracking", Complete: receivedFirstHit, Current: boolInt(receivedFirstHit), Target: 1, SiteID: firstSiteID, SiteDomain: firstSiteDomain},
		{Key: "automatic_events", Complete: automaticEventSeen, Current: boolInt(automaticEventSeen), Target: 1, SiteID: firstSiteID, SiteDomain: firstSiteDomain},
		{Key: "invite_teammate", Complete: memberCount > 1 || pendingInviteCount > 0, Current: memberCount + pendingInviteCount, Target: 2},
		{Key: "schedule_report", Complete: reportScheduled, Current: boolInt(reportScheduled), Target: 1},
	}

	complete := true
	for _, step := range steps {
		if !step.Complete {
			complete = false
			break
		}
	}

	return &api.UserOnboarding{
		Dismissed: prefs != nil && prefs.DismissedOnboardingAt != nil,
		Complete:  complete,
		Steps:     steps,
	}, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nowUTC() time.Time {
	return time.Now().UTC()
}
