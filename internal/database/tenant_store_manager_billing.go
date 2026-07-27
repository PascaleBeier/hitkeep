//go:build billing

package database

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type lifecycleSiteCandidate struct {
	recipient   CloudLifecycleRecipient
	createdAt   time.Time
	clmStatus   string
	sent        bool
	siteCount   int
	memberCount int
}

// ListEligibleCloudLifecycleRecipients keeps billing metadata and lifecycle
// message state on the control plane while resolving activation timestamps
// from the owning tenant catalogs.
func (m *TenantStoreManager) ListEligibleCloudLifecycleRecipients(ctx context.Context, kind string, now time.Time, limit int) ([]CloudLifecycleRecipient, error) {
	if m == nil || m.control == nil {
		return nil, fmt.Errorf("tenant store manager is not configured")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if kind != CloudLifecycleMessageWelcome && kind != CloudLifecycleMessageFreeRetentionReminder && kind != CloudLifecycleMessageFreeRetentionPreTrim && kind != CloudLifecycleMessageFreeLimitReminder {
		return nil, fmt.Errorf("unsupported cloud lifecycle message kind %q", kind)
	}
	controlCandidates, err := m.control.ListCloudLifecycleControlCandidates(ctx, kind)
	if err != nil {
		return nil, err
	}
	selected := make(map[uuid.UUID]lifecycleSiteCandidate)
	for _, controlCandidate := range controlCandidates {
		candidate := lifecycleSiteCandidate{
			recipient: CloudLifecycleRecipient{
				TenantID: controlCandidate.Recipient.TenantID, TenantName: controlCandidate.Recipient.TenantName,
				UserID: controlCandidate.Recipient.UserID, Email: controlCandidate.Recipient.Email, Locale: controlCandidate.Recipient.Locale,
				SiteID: controlCandidate.Recipient.SiteID, SiteDomain: controlCandidate.Recipient.SiteDomain,
				PlanCode: controlCandidate.Recipient.PlanCode, PlanName: controlCandidate.Recipient.PlanName,
				SubscriptionStatus: controlCandidate.Recipient.SubscriptionStatus, Attempts: controlCandidate.Recipient.Attempts,
			},
			createdAt:   controlCandidate.SiteCreated,
			clmStatus:   controlCandidate.MessageStatus,
			sent:        controlCandidate.MessageSent,
			siteCount:   controlCandidate.SiteCount,
			memberCount: controlCandidate.MemberCount,
		}
		store, _, err := m.ResolveSiteStore(ctx, candidate.recipient.SiteID)
		if err != nil {
			return nil, fmt.Errorf("resolve lifecycle analytics store for site %s: %w", candidate.recipient.SiteID, err)
		}
		status, err := store.GetSiteTrackingStatus(ctx, candidate.recipient.SiteID, now)
		if err != nil {
			return nil, fmt.Errorf("load lifecycle activity for site %s: %w", candidate.recipient.SiteID, err)
		}
		if status == nil || status.FirstHitAt == nil {
			continue
		}
		candidate.recipient.FirstHitAt = *status.FirstHitAt
		if previous, ok := selected[candidate.recipient.TenantID]; ok && !lifecycleCandidateEarlier(candidate, previous) {
			continue
		}
		if candidate.sent || candidate.clmStatus == CloudLifecycleMessageStatusSent || candidate.recipient.Attempts >= CloudLifecycleMessageMaxAttempts {
			continue
		}
		if kind != CloudLifecycleMessageWelcome {
			if !CloudSubscriptionStatusIsFree(candidate.recipient.SubscriptionStatus) {
				continue
			}
		}
		if !eligibleLifecycleKind(kind, candidate.recipient.FirstHitAt, candidate.siteCount, candidate.memberCount, now) {
			continue
		}
		selected[candidate.recipient.TenantID] = candidate
	}
	result := make([]CloudLifecycleRecipient, 0, len(selected))
	for _, candidate := range selected {
		result = append(result, candidate.recipient)
	}
	sort.Slice(result, func(i, j int) bool {
		if !result[i].FirstHitAt.Equal(result[j].FirstHitAt) {
			return result[i].FirstHitAt.Before(result[j].FirstHitAt)
		}
		return strings.ToLower(result[i].Email) < strings.ToLower(result[j].Email)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func lifecycleCandidateEarlier(candidate, previous lifecycleSiteCandidate) bool {
	if !candidate.recipient.FirstHitAt.Equal(previous.recipient.FirstHitAt) {
		return candidate.recipient.FirstHitAt.Before(previous.recipient.FirstHitAt)
	}
	if !candidate.createdAt.Equal(previous.createdAt) {
		return candidate.createdAt.Before(previous.createdAt)
	}
	return candidate.recipient.SiteDomain < previous.recipient.SiteDomain
}

func eligibleLifecycleKind(kind string, firstHit time.Time, siteCount, memberCount int, now time.Time) bool {
	switch kind {
	case CloudLifecycleMessageFreeRetentionReminder:
		return !firstHit.After(now.AddDate(0, 0, -14))
	case CloudLifecycleMessageFreeRetentionPreTrim:
		return !firstHit.After(now.AddDate(0, 0, -(CloudFreePlanRetentionDays-CloudRetentionPreTrimLeadDays))) && firstHit.After(now.AddDate(0, 0, -CloudFreePlanRetentionDays))
	case CloudLifecycleMessageFreeLimitReminder:
		return siteCount >= CloudFreePlanSiteLimit || memberCount >= CloudFreePlanMemberLimit
	default:
		return true
	}
}
