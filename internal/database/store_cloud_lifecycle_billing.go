//go:build billing

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/controlstore"
)

const (
	CloudLifecycleMessageWelcome               = "cloud_welcome"
	CloudLifecycleMessageFreeRetentionReminder = "cloud_free_retention_reminder"
	CloudLifecycleMessageFreeRetentionPreTrim  = "cloud_free_retention_pretrim"
	CloudLifecycleMessageFreeLimitReminder     = "cloud_free_limit_reminder"
	CloudLifecycleMessageStatusSent            = "sent"
	CloudLifecycleMessageStatusFailed          = "failed"
	CloudLifecycleMessageMaxAttempts           = 3

	// CloudFreePlanRetentionDays mirrors the free plan's MaxRetentionDays
	// entitlement so lifecycle eligibility can be computed in SQL.
	CloudFreePlanRetentionDays = 60
	// CloudRetentionPreTrimLeadDays is how many days before the first data
	// roll-off the pre-trim warning goes out.
	CloudRetentionPreTrimLeadDays = 7
)

var ErrCloudLifecycleMessageNotFound = errors.New("cloud lifecycle message not found")

type CloudLifecycleRecipient struct {
	TenantID           uuid.UUID
	TenantName         string
	UserID             uuid.UUID
	Email              string
	Locale             string
	SiteID             uuid.UUID
	SiteDomain         string
	FirstHitAt         time.Time
	PlanCode           string
	PlanName           string
	SubscriptionStatus string
	Attempts           int
}

type CloudLifecycleMessage struct {
	ID              uuid.UUID
	TenantID        uuid.UUID
	UserID          uuid.UUID
	Kind            string
	Status          string
	Attempts        int
	ProcessingError string
	SentAt          *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type CloudLifecycleMessageUpdate = controlstore.CloudLifecycleMessageUpdate

func (s *Store) ListEligibleCloudLifecycleRecipients(ctx context.Context, kind string, now time.Time, limit int) ([]CloudLifecycleRecipient, error) {
	kind = strings.TrimSpace(kind)
	if kind != CloudLifecycleMessageWelcome && kind != CloudLifecycleMessageFreeRetentionReminder && kind != CloudLifecycleMessageFreeRetentionPreTrim && kind != CloudLifecycleMessageFreeLimitReminder {
		return nil, fmt.Errorf("unsupported cloud lifecycle message kind %q", kind)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	// Free-status filter shared by every free-plan lifecycle kind, built from
	// the canonical list so eligibility can't drift from entitlements.
	freeStatuses := CloudFreeSubscriptionStatuses()
	freeStatusIn := "COALESCE(NULLIF(cba.subscription_status, ''), ?) IN (?" + strings.Repeat(", ?", len(freeStatuses)-1) + ")"
	freeStatusArgs := make([]any, 0, len(freeStatuses)+1)
	freeStatusArgs = append(freeStatusArgs, CloudSubscriptionStatusFree)
	for _, status := range freeStatuses {
		freeStatusArgs = append(freeStatusArgs, status)
	}

	extraWhere := ""
	args := []any{
		kind,
		CloudLifecycleMessageStatusSent,
		CloudLifecycleMessageMaxAttempts,
	}
	if kind == CloudLifecycleMessageFreeRetentionReminder {
		extraWhere = `
			AND activated.first_hit_at <= ?
			AND ` + freeStatusIn + `
		`
		args = append(args, now.UTC().AddDate(0, 0, -14))
		args = append(args, freeStatusArgs...)
	}
	if kind == CloudLifecycleMessageFreeRetentionPreTrim {
		// Warn only inside the lead window: old enough that roll-off is
		// imminent, but not so old that data is already being removed.
		extraWhere = `
			AND activated.first_hit_at <= ?
			AND activated.first_hit_at > ?
			AND ` + freeStatusIn + `
		`
		args = append(args,
			now.UTC().AddDate(0, 0, -(CloudFreePlanRetentionDays-CloudRetentionPreTrimLeadDays)),
			now.UTC().AddDate(0, 0, -CloudFreePlanRetentionDays),
		)
		args = append(args, freeStatusArgs...)
	}
	if kind == CloudLifecycleMessageFreeLimitReminder {
		extraWhere = `
			AND ` + freeStatusIn + `
			AND (
				(SELECT COUNT(*) FROM site_tenants st2 WHERE st2.tenant_id = activated.tenant_id) >= ?
				OR (SELECT COUNT(*) FROM tenant_members tm2 WHERE tm2.tenant_id = activated.tenant_id) >= ?
			)
		`
		args = append(args, freeStatusArgs...)
		args = append(args,
			CloudFreePlanSiteLimit,
			CloudFreePlanMemberLimit,
		)
	}
	args = append(args, limit)

	query := fmt.Sprintf(`
		WITH activated_sites AS (
			SELECT tenant_id, site_id, domain, first_hit_at
			FROM (
				SELECT
					st.tenant_id,
					s.id AS site_id,
					s.domain,
					sas.first_hit_at,
					ROW_NUMBER() OVER (
						PARTITION BY st.tenant_id
						ORDER BY sas.first_hit_at ASC, s.created_at ASC, s.domain ASC
					) AS rn
				FROM site_tenants st
				JOIN sites s ON s.id = st.site_id
				JOIN site_activity_summary sas ON sas.site_id = s.id
				WHERE sas.first_hit_at IS NOT NULL
			) ranked
			WHERE rn = 1
		)
		SELECT
			activated.tenant_id,
			t.name,
			tm.user_id,
			u.email,
			COALESCE(up.default_locale, ''),
			activated.site_id,
			activated.domain,
			activated.first_hit_at,
			COALESCE(cba.plan_code, ''),
			COALESCE(cba.plan_name, ''),
			COALESCE(cba.subscription_status, ''),
			COALESCE(clm.attempts, 0)
		FROM activated_sites activated
		JOIN tenants t ON t.id = activated.tenant_id
		JOIN cloud_billing_accounts cba ON cba.tenant_id = activated.tenant_id
		JOIN tenant_members tm ON tm.tenant_id = activated.tenant_id AND tm.role = 'owner'
		JOIN users u ON u.id = tm.user_id
		LEFT JOIN user_preferences up ON up.user_id = tm.user_id
		LEFT JOIN tenant_archives ta ON ta.tenant_id = activated.tenant_id
		LEFT JOIN cloud_lifecycle_messages clm
			ON clm.tenant_id = activated.tenant_id
			AND clm.user_id = tm.user_id
			AND clm.kind = ?
		WHERE ta.tenant_id IS NULL
			AND COALESCE(clm.status, '') <> ?
			AND clm.sent_at IS NULL
			AND COALESCE(clm.attempts, 0) < ?
			%s
		ORDER BY activated.first_hit_at ASC, u.email ASC
		LIMIT ?
	`, extraWhere)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query eligible cloud lifecycle recipients: %w", err)
	}
	defer rows.Close()

	recipients := make([]CloudLifecycleRecipient, 0)
	for rows.Next() {
		var recipient CloudLifecycleRecipient
		if err := rows.Scan(
			&recipient.TenantID,
			&recipient.TenantName,
			&recipient.UserID,
			&recipient.Email,
			&recipient.Locale,
			&recipient.SiteID,
			&recipient.SiteDomain,
			&recipient.FirstHitAt,
			&recipient.PlanCode,
			&recipient.PlanName,
			&recipient.SubscriptionStatus,
			&recipient.Attempts,
		); err != nil {
			return nil, fmt.Errorf("scan cloud lifecycle recipient: %w", err)
		}
		recipients = append(recipients, recipient)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cloud lifecycle recipients: %w", err)
	}

	return recipients, nil
}

func (s *Store) MarkCloudLifecycleMessageSent(ctx context.Context, update CloudLifecycleMessageUpdate) error {
	now := cloudLifecycleNow(update.Now)
	return s.Exec(ctx, `
		INSERT INTO cloud_lifecycle_messages (
			id,
			tenant_id,
			user_id,
			kind,
			status,
			attempts,
			processing_error,
			sent_at,
			created_at,
			updated_at
		)
		VALUES (?, ?, ?, ?, ?, 1, NULL, ?, ?, ?)
		ON CONFLICT (tenant_id, user_id, kind) DO UPDATE SET
			status = excluded.status,
			attempts = cloud_lifecycle_messages.attempts + 1,
			processing_error = NULL,
			sent_at = excluded.sent_at,
			updated_at = excluded.updated_at
	`, uuid.New(), update.TenantID, update.UserID, strings.TrimSpace(update.Kind), CloudLifecycleMessageStatusSent, now, now, now)
}

func (s *Store) MarkCloudLifecycleMessageFailed(ctx context.Context, update CloudLifecycleMessageUpdate) error {
	now := cloudLifecycleNow(update.Now)
	return s.Exec(ctx, `
		INSERT INTO cloud_lifecycle_messages (
			id,
			tenant_id,
			user_id,
			kind,
			status,
			attempts,
			processing_error,
			sent_at,
			created_at,
			updated_at
		)
		VALUES (?, ?, ?, ?, ?, 1, NULLIF(?, ''), NULL, ?, ?)
		ON CONFLICT (tenant_id, user_id, kind) DO UPDATE SET
			status = excluded.status,
			attempts = cloud_lifecycle_messages.attempts + 1,
			processing_error = excluded.processing_error,
			updated_at = excluded.updated_at
	`, uuid.New(), update.TenantID, update.UserID, strings.TrimSpace(update.Kind), CloudLifecycleMessageStatusFailed, truncateCloudLifecycleError(update.Error), now, now)
}

func (s *Store) GetCloudLifecycleMessage(ctx context.Context, tenantID, userID uuid.UUID, kind string) (*CloudLifecycleMessage, error) {
	var message CloudLifecycleMessage
	var processingError sql.NullString
	var sentAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, user_id, kind, status, attempts, COALESCE(processing_error, ''), sent_at, created_at, updated_at
		FROM cloud_lifecycle_messages
		WHERE tenant_id = ? AND user_id = ? AND kind = ?
	`, tenantID, userID, strings.TrimSpace(kind)).Scan(
		&message.ID,
		&message.TenantID,
		&message.UserID,
		&message.Kind,
		&message.Status,
		&message.Attempts,
		&processingError,
		&sentAt,
		&message.CreatedAt,
		&message.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCloudLifecycleMessageNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query cloud lifecycle message: %w", err)
	}
	message.ProcessingError = strings.TrimSpace(processingError.String)
	if sentAt.Valid {
		sent := sentAt.Time
		message.SentAt = &sent
	}
	return &message, nil
}

func cloudLifecycleNow(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now().UTC()
	}
	return now.UTC()
}

func truncateCloudLifecycleError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 1000 {
		return value[:1000]
	}
	return value
}
