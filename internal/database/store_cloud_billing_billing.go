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
	CloudPlanFree                            = "free"
	CloudPlanPro                             = "pro"
	CloudPlanBusiness                        = "business"
	CloudBillingIntervalMonthly              = "monthly"
	CloudBillingIntervalAnnual               = "annual"
	CloudFreePlanSiteLimit                   = 3
	CloudFreePlanMemberLimit                 = 3
	CloudSubscriptionStatusFree              = "free"
	CloudSubscriptionStatusPendingCheckout   = "pending_checkout"
	CloudSubscriptionStatusActive            = "active"
	CloudSubscriptionStatusPastDue           = "past_due"
	CloudSubscriptionStatusCanceled          = "canceled"
	CloudSubscriptionStatusUnpaid            = "unpaid"
	CloudSubscriptionStatusPaused            = "paused"
	CloudSubscriptionStatusIncomplete        = "incomplete"
	CloudSubscriptionStatusIncompleteExpired = "incomplete_expired"
	CloudSubscriptionStatusDisputed          = "disputed"
	CloudSubscriptionStatusChargebackLost    = "chargeback_lost"
	CloudBillingEventStatusSeen              = "seen"
	CloudBillingEventStatusDone              = "processed"
	CloudBillingEventStatusErrored           = "failed"
)

var ErrCloudBillingAccountNotFound = errors.New("cloud billing account not found")
var ErrCloudBillingEventNotFound = errors.New("cloud billing event not found")

// CloudFreeSubscriptionStatuses is the canonical list of subscription
// statuses that grant only Free-plan entitlements: the local pre-payment
// states plus every Stripe state in which the customer is no longer paying.
// Statuses absent here (active, trialing, past_due, disputed, checkout's
// "complete", and anything Stripe introduces later) keep the paid plan on
// purpose — misclassifying a paying team as Free would destructively trim
// its retained data.
func CloudFreeSubscriptionStatuses() []string {
	return []string{
		CloudSubscriptionStatusFree,
		CloudSubscriptionStatusPendingCheckout,
		CloudSubscriptionStatusCanceled,
		CloudSubscriptionStatusUnpaid,
		CloudSubscriptionStatusPaused,
		CloudSubscriptionStatusIncomplete,
		CloudSubscriptionStatusIncompleteExpired,
		CloudSubscriptionStatusChargebackLost,
	}
}

// CloudSubscriptionStatusIsFree reports whether the status grants only
// Free-plan entitlements. Blank statuses count as free: they belong to
// accounts that never started a checkout.
func CloudSubscriptionStatusIsFree(status string) bool {
	status = strings.TrimSpace(status)
	if status == "" {
		return true
	}
	for _, free := range CloudFreeSubscriptionStatuses() {
		if status == free {
			return true
		}
	}
	return false
}

type CreateManagedCloudAccountInput = controlstore.CreateManagedCloudAccountInput

type ManagedCloudAccount struct {
	UserID   uuid.UUID
	TenantID uuid.UUID
}

type CloudBillingAccount = controlstore.CloudBillingAccount

type CloudBillingEvent = controlstore.CloudBillingEvent

func (s *Store) CreateManagedCloudAccount(ctx context.Context, input CreateManagedCloudAccountInput) (*ManagedCloudAccount, error) {
	email := strings.TrimSpace(strings.ToLower(input.Email))
	givenName := strings.TrimSpace(input.GivenName)
	lastName := strings.TrimSpace(input.LastName)
	teamName := strings.TrimSpace(input.TeamName)
	locale := strings.TrimSpace(input.Locale)
	if locale == "" {
		locale = defaultLocaleCode
	}

	var duplicateCount int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE lower(email) = lower(?)", email).Scan(&duplicateCount); err != nil {
		return nil, fmt.Errorf("could not check duplicate cloud signup email: %w", err)
	}
	if duplicateCount > 0 {
		return nil, ErrUserEmailAlreadyExists
	}

	var result ManagedCloudAccount
	err := s.Transact(ctx, func(tx *sql.Tx) error {
		if err := ensureDefaultTenantTx(ctx, tx, defaultTenantName, false); err != nil {
			return err
		}

		now := time.Now().UTC()
		result.UserID = uuid.New()
		result.TenantID = uuid.New()

		if _, err := tx.ExecContext(ctx,
			"INSERT INTO users (id, email, password, given_name, last_name, created_at) VALUES (?, ?, ?, ?, ?, ?)",
			result.UserID, email, input.HashedPassword, nullableProfileName(givenName), nullableProfileName(lastName), now,
		); err != nil {
			return fmt.Errorf("could not create managed cloud user: %w", err)
		}

		if _, err := tx.ExecContext(ctx,
			"INSERT INTO tenants (id, name, created_at) VALUES (?, ?, ?)",
			result.TenantID, teamName, now,
		); err != nil {
			return fmt.Errorf("could not create managed cloud team: %w", err)
		}

		if _, err := tx.ExecContext(ctx,
			"INSERT INTO tenant_members (tenant_id, user_id, role, added_by, added_at) VALUES (?, ?, ?, ?, ?)",
			result.TenantID, result.UserID, TenantRoleOwner, result.UserID, now,
		); err != nil {
			return fmt.Errorf("could not create managed cloud team owner: %w", err)
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO user_preferences (user_id, default_locale, updated_at, active_tenant_id)
			VALUES (?, ?, ?, ?)
		`, result.UserID, locale, now, result.TenantID); err != nil {
			return fmt.Errorf("could not initialize managed cloud user preferences: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func (s *Store) UpsertCloudBillingAccount(ctx context.Context, account CloudBillingAccount) error {
	now := time.Now().UTC()
	account.BillingInterval = normalizeCloudBillingInterval(account.BillingInterval)
	if account.CreatedAt.IsZero() {
		account.CreatedAt = now
	}
	account.UpdatedAt = now

	return s.Exec(ctx, `
		INSERT INTO cloud_billing_accounts (
			tenant_id,
			plan_code,
				plan_name,
				billing_interval,
				subscription_status,
			stripe_customer_id,
			stripe_subscription_id,
			stripe_price_id,
			created_at,
			updated_at
		)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (tenant_id) DO UPDATE SET
				plan_code = excluded.plan_code,
				plan_name = excluded.plan_name,
				billing_interval = excluded.billing_interval,
			subscription_status = excluded.subscription_status,
			stripe_customer_id = excluded.stripe_customer_id,
			stripe_subscription_id = excluded.stripe_subscription_id,
			stripe_price_id = excluded.stripe_price_id,
			updated_at = excluded.updated_at
	`,
		account.TenantID,
		account.PlanCode,
		account.PlanName,
		account.BillingInterval,
		account.SubscriptionStatus,
		nullIfBlank(account.StripeCustomerID),
		nullIfBlank(account.StripeSubscriptionID),
		nullIfBlank(account.StripePriceID),
		account.CreatedAt,
		account.UpdatedAt,
	)
}

func normalizeCloudBillingInterval(interval string) string {
	if strings.EqualFold(strings.TrimSpace(interval), CloudBillingIntervalAnnual) {
		return CloudBillingIntervalAnnual
	}
	return CloudBillingIntervalMonthly
}

func (s *Store) GetCloudBillingAccount(ctx context.Context, tenantID uuid.UUID) (*CloudBillingAccount, error) {
	var account CloudBillingAccount
	var customerID sql.NullString
	var subscriptionID sql.NullString
	var priceID sql.NullString

	err := s.db.QueryRowContext(ctx, `
			SELECT tenant_id, plan_code, plan_name, billing_interval, subscription_status,
		       COALESCE(stripe_customer_id, ''), COALESCE(stripe_subscription_id, ''), COALESCE(stripe_price_id, ''),
		       created_at, updated_at
		FROM cloud_billing_accounts
		WHERE tenant_id = ?
	`, tenantID).Scan(
		&account.TenantID,
		&account.PlanCode,
		&account.PlanName,
		&account.BillingInterval,
		&account.SubscriptionStatus,
		&customerID,
		&subscriptionID,
		&priceID,
		&account.CreatedAt,
		&account.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCloudBillingAccountNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("could not query cloud billing account: %w", err)
	}

	account.StripeCustomerID = strings.TrimSpace(customerID.String)
	account.StripeSubscriptionID = strings.TrimSpace(subscriptionID.String)
	account.StripePriceID = strings.TrimSpace(priceID.String)
	return &account, nil
}

func (s *Store) GetCloudBillingAccountByStripeCustomerID(ctx context.Context, customerID string) (*CloudBillingAccount, error) {
	return s.getCloudBillingAccountByField(ctx, "stripe_customer_id", customerID)
}

func (s *Store) GetCloudBillingAccountByStripeSubscriptionID(ctx context.Context, subscriptionID string) (*CloudBillingAccount, error) {
	return s.getCloudBillingAccountByField(ctx, "stripe_subscription_id", subscriptionID)
}

func (s *Store) CreateCloudBillingEvent(ctx context.Context, event CloudBillingEvent) (bool, error) {
	now := time.Now().UTC()
	if event.CreatedAt.IsZero() {
		event.CreatedAt = now
	}
	if event.UpdatedAt.IsZero() {
		event.UpdatedAt = event.CreatedAt
	}
	if strings.TrimSpace(event.ProcessingStatus) == "" {
		event.ProcessingStatus = CloudBillingEventStatusSeen
	}

	result, err := s.db.ExecContext(ctx, `
		INSERT INTO cloud_billing_events (
			stripe_event_id,
			tenant_id,
			event_type,
			livemode,
			payload,
			processing_status,
			processing_error,
			processed_at,
			created_at,
			updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (stripe_event_id) DO NOTHING
	`,
		strings.TrimSpace(event.StripeEventID),
		nullCloudBillingUUID(event.TenantID),
		strings.TrimSpace(event.EventType),
		event.Livemode,
		nullIfBlank(event.Payload),
		strings.TrimSpace(event.ProcessingStatus),
		nullIfBlank(event.ProcessingError),
		event.ProcessedAt,
		event.CreatedAt,
		event.UpdatedAt,
	)
	if err != nil {
		return false, fmt.Errorf("could not create cloud billing event: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("could not read cloud billing event rows affected: %w", err)
	}

	return rows > 0, nil
}

func (s *Store) UpdateCloudBillingEventStatus(ctx context.Context, event CloudBillingEvent) error {
	now := time.Now().UTC()
	if event.UpdatedAt.IsZero() {
		event.UpdatedAt = now
	}

	return s.Exec(ctx, `
		UPDATE cloud_billing_events
		SET tenant_id = ?,
			processing_status = ?,
			processing_error = ?,
			processed_at = ?,
			updated_at = ?
		WHERE stripe_event_id = ?
	`,
		nullCloudBillingUUID(event.TenantID),
		strings.TrimSpace(event.ProcessingStatus),
		nullIfBlank(event.ProcessingError),
		event.ProcessedAt,
		event.UpdatedAt,
		strings.TrimSpace(event.StripeEventID),
	)
}

func (s *Store) GetCloudBillingEvent(ctx context.Context, stripeEventID string) (*CloudBillingEvent, error) {
	var event CloudBillingEvent
	var tenantID sql.NullString
	var payload sql.NullString
	var processingError sql.NullString
	var processedAt sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT stripe_event_id,
		       CAST(tenant_id AS VARCHAR),
		       event_type,
		       livemode,
		       COALESCE(payload, ''),
		       processing_status,
		       COALESCE(processing_error, ''),
		       processed_at,
		       created_at,
		       updated_at
		FROM cloud_billing_events
		WHERE stripe_event_id = ?
	`, strings.TrimSpace(stripeEventID)).Scan(
		&event.StripeEventID,
		&tenantID,
		&event.EventType,
		&event.Livemode,
		&payload,
		&event.ProcessingStatus,
		&processingError,
		&processedAt,
		&event.CreatedAt,
		&event.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCloudBillingEventNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("could not query cloud billing event: %w", err)
	}

	if tenantID.Valid && strings.TrimSpace(tenantID.String) != "" {
		parsedTenantID, parseErr := uuid.Parse(tenantID.String)
		if parseErr != nil {
			return nil, fmt.Errorf("invalid cloud billing event tenant id %q: %w", tenantID.String, parseErr)
		}
		event.TenantID = parsedTenantID
	}
	event.Payload = strings.TrimSpace(payload.String)
	event.ProcessingError = strings.TrimSpace(processingError.String)
	if processedAt.Valid {
		processed := processedAt.Time
		event.ProcessedAt = &processed
	}

	return &event, nil
}

func nullIfBlank(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

func nullCloudBillingUUID(value uuid.UUID) any {
	if value == uuid.Nil {
		return nil
	}
	return value
}

func (s *Store) getCloudBillingAccountByField(ctx context.Context, field string, value string) (*CloudBillingAccount, error) {
	trimmedValue := strings.TrimSpace(value)
	if trimmedValue == "" {
		return nil, ErrCloudBillingAccountNotFound
	}

	var queryField string
	switch field {
	case "stripe_customer_id", "stripe_subscription_id":
		queryField = field
	default:
		return nil, fmt.Errorf("unsupported cloud billing lookup field %q", field)
	}

	var account CloudBillingAccount
	var customerID sql.NullString
	var subscriptionID sql.NullString
	var priceID sql.NullString

	// #nosec G201 -- queryField is restricted to the column whitelist in the switch above.
	query := fmt.Sprintf(`
			SELECT tenant_id, plan_code, plan_name, billing_interval, subscription_status,
		       COALESCE(stripe_customer_id, ''), COALESCE(stripe_subscription_id, ''), COALESCE(stripe_price_id, ''),
		       created_at, updated_at
		FROM cloud_billing_accounts
		WHERE %s = ?
	`, queryField)

	err := s.db.QueryRowContext(ctx, query, trimmedValue).Scan(
		&account.TenantID,
		&account.PlanCode,
		&account.PlanName,
		&account.BillingInterval,
		&account.SubscriptionStatus,
		&customerID,
		&subscriptionID,
		&priceID,
		&account.CreatedAt,
		&account.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCloudBillingAccountNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("could not query cloud billing account by %s: %w", queryField, err)
	}

	account.StripeCustomerID = strings.TrimSpace(customerID.String)
	account.StripeSubscriptionID = strings.TrimSpace(subscriptionID.String)
	account.StripePriceID = strings.TrimSpace(priceID.String)
	return &account, nil
}
