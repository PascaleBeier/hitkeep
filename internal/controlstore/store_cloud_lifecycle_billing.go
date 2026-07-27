//go:build billing

package controlstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
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

// CloudLifecycleControlCandidate contains only control-plane metadata. The
// tenant manager resolves FirstHitAt from the owning DuckDB catalog before it
// applies lifecycle eligibility policy.
type CloudLifecycleControlCandidate struct {
	Recipient     CloudLifecycleRecipient
	SiteCreated   time.Time
	MessageSent   bool
	MessageStatus string
	SiteCount     int
	MemberCount   int
}

func (s *Store) ListCloudLifecycleControlCandidates(ctx context.Context, kind string) ([]CloudLifecycleControlCandidate, error) {
	rows, err := s.db.QueryContext(ctx, `
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
		  AND COALESCE(clm.attempts, 0) < ?`,
		kind, CloudLifecycleMessageStatusSent, CloudLifecycleMessageMaxAttempts)
	if err != nil {
		return nil, fmt.Errorf("query cloud lifecycle control metadata: %w", err)
	}
	defer rows.Close()
	var candidates []CloudLifecycleControlCandidate
	for rows.Next() {
		var candidate CloudLifecycleControlCandidate
		var sentAt *time.Time
		if err := rows.Scan(
			&candidate.Recipient.TenantID, &candidate.Recipient.TenantName,
			&candidate.Recipient.UserID, &candidate.Recipient.Email, &candidate.Recipient.Locale,
			&candidate.Recipient.SiteID, &candidate.Recipient.SiteDomain, &candidate.SiteCreated,
			&candidate.Recipient.PlanCode, &candidate.Recipient.PlanName,
			&candidate.Recipient.SubscriptionStatus, &candidate.Recipient.Attempts,
			&candidate.MessageStatus, &sentAt, &candidate.SiteCount, &candidate.MemberCount,
		); err != nil {
			return nil, fmt.Errorf("scan cloud lifecycle control metadata: %w", err)
		}
		candidate.MessageSent = sentAt != nil
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read cloud lifecycle control metadata: %w", err)
	}
	return candidates, nil
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

type CloudLifecycleMessageUpdate struct {
	TenantID uuid.UUID
	UserID   uuid.UUID
	Kind     string
	Error    string
	Now      time.Time
}

func (s *Store) MarkCloudLifecycleMessageSent(ctx context.Context, update CloudLifecycleMessageUpdate) error {
	now := cloudLifecycleNow(update.Now)
	return s.exec(ctx, `
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
	return s.exec(ctx, `
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
