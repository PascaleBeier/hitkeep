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
	if m == nil || m.shared == nil || !m.dataPlaneEnabled {
		if m == nil || m.shared == nil {
			return nil, fmt.Errorf("tenant store manager is not configured")
		}
		return m.shared.ListEligibleCloudLifecycleRecipients(ctx, kind, now, limit)
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
	rows, err := m.shared.DB().QueryContext(ctx, `
		SELECT
			st.tenant_id, t.name, tm.user_id, u.email,
			COALESCE(up.default_locale, ''), s.id, s.domain, s.created_at,
			COALESCE(cba.plan_code, ''), COALESCE(cba.plan_name, ''),
			COALESCE(cba.subscription_status, ''), COALESCE(clm.attempts, 0),
			COALESCE(clm.status, ''), clm.sent_at,
			(SELECT COUNT(*) FROM site_tenants st2 WHERE st2.tenant_id = st.tenant_id),
			(SELECT COUNT(*) FROM tenant_members tm2 WHERE tm2.tenant_id = st.tenant_id)
		FROM site_tenants st
		JOIN sites s ON s.id = st.site_id
		JOIN tenants t ON t.id = st.tenant_id
		JOIN tenant_members tm ON tm.tenant_id = st.tenant_id AND tm.role = 'owner'
		JOIN users u ON u.id = tm.user_id
		LEFT JOIN user_preferences up ON up.user_id = tm.user_id
		JOIN cloud_billing_accounts cba ON cba.tenant_id = st.tenant_id
		LEFT JOIN tenant_archives ta ON ta.tenant_id = st.tenant_id
		LEFT JOIN cloud_lifecycle_messages clm
			ON clm.tenant_id = st.tenant_id AND clm.user_id = tm.user_id AND clm.kind = ?
		WHERE ta.tenant_id IS NULL
		  AND COALESCE(clm.status, '') <> ?
		  AND clm.sent_at IS NULL
		  AND COALESCE(clm.attempts, 0) < ?
	`, kind, CloudLifecycleMessageStatusSent, CloudLifecycleMessageMaxAttempts)
	if err != nil {
		return nil, fmt.Errorf("query cloud lifecycle control metadata: %w", err)
	}
	defer rows.Close()
	selected := make(map[uuid.UUID]lifecycleSiteCandidate)
	for rows.Next() {
		var candidate lifecycleSiteCandidate
		var sentAt *time.Time
		if err := rows.Scan(
			&candidate.recipient.TenantID, &candidate.recipient.TenantName,
			&candidate.recipient.UserID, &candidate.recipient.Email, &candidate.recipient.Locale,
			&candidate.recipient.SiteID, &candidate.recipient.SiteDomain, &candidate.createdAt,
			&candidate.recipient.PlanCode, &candidate.recipient.PlanName,
			&candidate.recipient.SubscriptionStatus, &candidate.recipient.Attempts,
			&candidate.clmStatus, &sentAt, &candidate.siteCount, &candidate.memberCount,
		); err != nil {
			return nil, fmt.Errorf("scan cloud lifecycle control metadata: %w", err)
		}
		candidate.sent = sentAt != nil
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read cloud lifecycle control metadata: %w", err)
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
