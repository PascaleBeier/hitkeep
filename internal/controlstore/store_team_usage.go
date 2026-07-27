package controlstore

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

func (s *Store) CountTeamMembers(ctx context.Context, tenantID uuid.UUID) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM tenant_members
		WHERE tenant_id = ?
	`, tenantID).Scan(&count); err != nil {
		return 0, fmt.Errorf("could not count team members: %w", err)
	}
	return count, nil
}

func (s *Store) CountPendingTeamInvites(ctx context.Context, tenantID uuid.UUID) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM team_invites
		WHERE tenant_id = ? AND status = ?
	`, tenantID, TeamInviteStatusPending).Scan(&count); err != nil {
		return 0, fmt.Errorf("could not count pending team invites: %w", err)
	}
	return count, nil
}

func (s *Store) BuildTeamUsageSummary(ctx context.Context, tenantID uuid.UUID) (*api.TeamUsageSummary, error) {
	sites, err := s.ListSitesForTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	memberCount, err := s.CountTeamMembers(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	pendingInviteCount, err := s.CountPendingTeamInvites(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	return &api.TeamUsageSummary{
		CurrentSites:          len(sites),
		CurrentMembers:        memberCount,
		CurrentPendingInvites: pendingInviteCount,
	}, nil
}
