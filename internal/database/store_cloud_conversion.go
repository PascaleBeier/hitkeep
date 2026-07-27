package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/controlstore"
)

const (
	CloudConversionSignupVerified        = "signup_verified"
	CloudConversionFirstSiteCreated      = "first_site_created"
	CloudConversionFirstHitReceived      = "first_hit_received"
	CloudConversionCheckoutStarted       = "checkout_started"
	CloudConversionSubscriptionActivated = "subscription_activated"
)

var cloudConversionEventNames = map[string]struct{}{
	CloudConversionSignupVerified:        {},
	CloudConversionFirstSiteCreated:      {},
	CloudConversionFirstHitReceived:      {},
	CloudConversionCheckoutStarted:       {},
	CloudConversionSubscriptionActivated: {},
}

type CloudConversionEvent = controlstore.CloudConversionEvent

// RecordCloudConversionEvent records a privacy-safe product milestone. Blank
// plan and interval values inherit the tenant's current billing account. A
// blank dedupe key makes the milestone one-time per tenant.
func (s *Store) RecordCloudConversionEvent(ctx context.Context, event CloudConversionEvent) (bool, error) {
	event.EventName = strings.TrimSpace(strings.ToLower(event.EventName))
	if _, ok := cloudConversionEventNames[event.EventName]; !ok {
		return false, fmt.Errorf("unsupported cloud conversion event %q", event.EventName)
	}
	if event.TenantID == uuid.Nil {
		return false, fmt.Errorf("cloud conversion tenant id is required")
	}
	if strings.TrimSpace(event.DedupeKey) == "" {
		event.DedupeKey = event.TenantID.String() + ":" + event.EventName
		if s.runtime != nil && s.runtime.cloudConversionMilestones != nil {
			if _, ok := s.runtime.cloudConversionMilestones.Get(event.DedupeKey); ok {
				return false, nil
			}
		}
	} else {
		event.DedupeKey = strings.TrimSpace(event.DedupeKey)
	}

	var accountPlan, accountInterval string
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(plan_code, ''), COALESCE(billing_interval, '')
		FROM cloud_billing_accounts
		WHERE tenant_id = ?
	`, event.TenantID).Scan(&accountPlan, &accountInterval)
	if err == sql.ErrNoRows {
		if s.runtime != nil && s.runtime.cloudConversionMilestones != nil {
			s.runtime.cloudConversionMilestones.Add(event.DedupeKey, false)
		}
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("resolve cloud conversion billing context: %w", err)
	}

	event.PlanCode = normalizeCloudConversionPlan(event.PlanCode, accountPlan)
	event.BillingInterval = normalizeCloudConversionInterval(event.BillingInterval, accountInterval)
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}

	result, err := s.db.ExecContext(ctx, `
		INSERT INTO cloud_conversion_events (
			id, tenant_id, event_name, plan_code, billing_interval,
			dedupe_key, occurred_at, created_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (dedupe_key) DO NOTHING
	`, event.ID, event.TenantID, event.EventName, event.PlanCode, event.BillingInterval, event.DedupeKey, event.OccurredAt.UTC(), event.CreatedAt.UTC())
	if err != nil {
		return false, fmt.Errorf("record cloud conversion event: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read cloud conversion insert result: %w", err)
	}
	if s.runtime != nil && s.runtime.cloudConversionMilestones != nil {
		s.runtime.cloudConversionMilestones.Add(event.DedupeKey, true)
	}
	return rows > 0, nil
}

func (s *Store) ListCloudConversionEvents(ctx context.Context, tenantID uuid.UUID) ([]CloudConversionEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, event_name, plan_code, billing_interval, dedupe_key, occurred_at, created_at
		FROM cloud_conversion_events
		WHERE tenant_id = ?
		ORDER BY occurred_at ASC, event_name ASC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list cloud conversion events: %w", err)
	}
	defer rows.Close()

	events := make([]CloudConversionEvent, 0)
	for rows.Next() {
		var event CloudConversionEvent
		if err := rows.Scan(&event.ID, &event.TenantID, &event.EventName, &event.PlanCode, &event.BillingInterval, &event.DedupeKey, &event.OccurredAt, &event.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan cloud conversion event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cloud conversion events: %w", err)
	}
	return events, nil
}

func normalizeCloudConversionPlan(requested, fallback string) string {
	for _, value := range []string{requested, fallback} {
		switch strings.TrimSpace(strings.ToLower(value)) {
		case "pro":
			return "pro"
		case "business":
			return "business"
		case "free":
			return "free"
		}
	}
	return "free"
}

func normalizeCloudConversionInterval(requested, fallback string) string {
	for _, value := range []string{requested, fallback} {
		switch strings.TrimSpace(strings.ToLower(value)) {
		case "annual":
			return "annual"
		case "monthly":
			return "monthly"
		}
	}
	return "monthly"
}
