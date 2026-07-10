package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/webhooks"
)

const (
	WebhookDeliveryPending    = "pending"
	WebhookDeliveryProcessing = "processing"
	WebhookDeliveryRetrying   = "retrying"
	WebhookDeliverySucceeded  = "succeeded"
	WebhookDeliveryFailed     = "failed"
)

type WebhookEventInput struct {
	ID                        uuid.UUID
	SiteID                    *uuid.UUID
	TargetWebhookID           *uuid.UUID
	EventType                 string
	APIVersion                string
	OccurredAt                time.Time
	Data                      map[string]any
	PreserveAfterSiteDeletion bool
}

type WebhookDeliveryJob struct {
	DeliveryID uuid.UUID `json:"delivery_id"`
	EventID    uuid.UUID `json:"event_id"`
	WebhookID  uuid.UUID `json:"webhook_id"`
	Payload    []byte    `json:"-"`
}

type WebhookDeliveryRecord struct {
	ID               uuid.UUID
	EventID          uuid.UUID
	WebhookID        uuid.UUID
	SiteID           *uuid.UUID
	EventType        string
	WebhookName      string
	DestinationURL   string
	SigningSecret    string
	Payload          []byte
	Status           string
	AttemptCount     int
	NextAttemptAt    *time.Time
	LastAttemptAt    *time.Time
	CompletedAt      *time.Time
	ResponseStatus   int
	LastErrorCode    string
	LastErrorMessage string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type WebhookAttemptResult struct {
	Status         string
	ResponseStatus int
	ErrorCode      string
	ErrorMessage   string
	StartedAt      time.Time
	CompletedAt    time.Time
	NextAttemptAt  *time.Time
}

type WebhookRetentionResult struct {
	Attempts   int64
	Deliveries int64
	Events     int64
}

type webhookSubscriber struct {
	ID           uuid.UUID
	Name         string
	URL          string
	Secret       string
	ConfiguredAt time.Time
}

type webhookPayload struct {
	APIVersion string         `json:"api_version"`
	ID         uuid.UUID      `json:"id"`
	DeliveryID *uuid.UUID     `json:"delivery_id,omitempty"`
	Type       string         `json:"type"`
	CreatedAt  time.Time      `json:"created_at"`
	Data       map[string]any `json:"data"`
}

func (s *Store) EnqueueWebhookEvent(ctx context.Context, input WebhookEventInput) ([]WebhookDeliveryJob, error) {
	if !webhooks.EventAllowedForScope(input.EventType, webhooks.ScopeInstance) {
		return nil, fmt.Errorf("unsupported webhook event type %q", input.EventType)
	}
	if input.APIVersion == "" {
		return nil, fmt.Errorf("webhook API version is required")
	}

	subscribers, err := s.enabledWebhookSubscribers(ctx, input.SiteID, input.TargetWebhookID, input.EventType)
	if err != nil {
		return nil, err
	}
	if len(subscribers) == 0 {
		return []WebhookDeliveryJob{}, nil
	}

	now := time.Now().UTC()
	if input.ID == uuid.Nil {
		input.ID = uuid.New()
	}
	if input.OccurredAt.IsZero() {
		input.OccurredAt = now
	} else {
		input.OccurredAt = input.OccurredAt.UTC()
	}
	if input.Data == nil {
		input.Data = map[string]any{}
	}

	eventBody, err := json.Marshal(webhookPayload{
		APIVersion: input.APIVersion,
		ID:         input.ID,
		Type:       input.EventType,
		CreatedAt:  input.OccurredAt,
		Data:       input.Data,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal webhook event payload: %w", err)
	}

	storedSiteID := input.SiteID
	if input.PreserveAfterSiteDeletion {
		storedSiteID = nil
	}
	jobs := make([]WebhookDeliveryJob, 0, len(subscribers))
	err = s.Transact(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO webhook_events (id, site_id, event_type, api_version, payload_json, occurred_at, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, input.ID, nullableUUIDPtr(storedSiteID), input.EventType, input.APIVersion, string(eventBody), input.OccurredAt, now); err != nil {
			return fmt.Errorf("create webhook event: %w", err)
		}

		for _, subscriber := range subscribers {
			deliveryID := uuid.New()
			payload, err := json.Marshal(webhookPayload{
				APIVersion: input.APIVersion,
				ID:         input.ID,
				DeliveryID: &deliveryID,
				Type:       input.EventType,
				CreatedAt:  input.OccurredAt,
				Data:       input.Data,
			})
			if err != nil {
				return fmt.Errorf("marshal webhook delivery payload: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO webhook_deliveries (
					id, event_id, webhook_id, site_id, event_type, webhook_name,
					destination_url, signing_secret, payload_json, status, attempt_count,
					next_attempt_at, created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?)
			`, deliveryID, input.ID, subscriber.ID, nullableUUIDPtr(storedSiteID), input.EventType, subscriber.Name,
				subscriber.URL, subscriber.Secret, string(payload), WebhookDeliveryPending, now, now, now); err != nil {
				return fmt.Errorf("create webhook delivery: %w", err)
			}
			jobs = append(jobs, WebhookDeliveryJob{
				DeliveryID: deliveryID,
				EventID:    input.ID,
				WebhookID:  subscriber.ID,
				Payload:    payload,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return jobs, nil
}

func (s *Store) enabledWebhookSubscribers(ctx context.Context, siteID, targetWebhookID *uuid.UUID, eventType string) ([]webhookSubscriber, error) {
	where := "w.site_id IS NULL"
	args := make([]any, 0, 3)
	if siteID != nil {
		where = "(w.site_id IS NULL OR w.site_id = ?)"
		args = append(args, *siteID)
	}
	if targetWebhookID != nil {
		where += " AND w.id = ?"
		args = append(args, *targetWebhookID)
	}
	query := `
		SELECT w.id, w.name, w.destination_url, w.secret, w.created_at
		FROM webhooks w
		JOIN webhook_event_subscriptions subscription ON subscription.webhook_id = w.id
		WHERE subscription.event_type = ? AND w.enabled = TRUE AND ` + where + `
		ORDER BY w.created_at, w.id
	`
	queryArgs := append([]any{eventType}, args...)
	if targetWebhookID != nil && eventType == webhooks.EventWebhookTest {
		query = `
			SELECT w.id, w.name, w.destination_url, w.secret, w.created_at
			FROM webhooks w
			WHERE w.enabled = TRUE AND ` + where + `
			ORDER BY w.created_at, w.id
		`
		queryArgs = args
	}
	rows, err := s.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("list enabled webhook subscribers: %w", err)
	}
	defer rows.Close()

	result := make([]webhookSubscriber, 0)
	for rows.Next() {
		var subscriber webhookSubscriber
		if err := rows.Scan(&subscriber.ID, &subscriber.Name, &subscriber.URL, &subscriber.Secret, &subscriber.ConfiguredAt); err != nil {
			return nil, fmt.Errorf("scan enabled webhook subscriber: %w", err)
		}
		result = append(result, subscriber)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate enabled webhook subscribers: %w", err)
	}
	return result, nil
}

func (s *Store) ListDueWebhookDeliveryJobs(ctx context.Context, dueAt time.Time, limit int) ([]WebhookDeliveryJob, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, event_id, webhook_id, CAST(payload_json AS VARCHAR)
		FROM webhook_deliveries
		WHERE status IN (?, ?) AND next_attempt_at <= ?
		ORDER BY next_attempt_at, created_at
		LIMIT ?
	`, WebhookDeliveryPending, WebhookDeliveryRetrying, dueAt.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("list due webhook deliveries: %w", err)
	}
	defer rows.Close()

	jobs := make([]WebhookDeliveryJob, 0)
	for rows.Next() {
		var job WebhookDeliveryJob
		var payload string
		if err := rows.Scan(&job.DeliveryID, &job.EventID, &job.WebhookID, &payload); err != nil {
			return nil, fmt.Errorf("scan due webhook delivery: %w", err)
		}
		job.Payload = []byte(payload)
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate due webhook deliveries: %w", err)
	}
	sort.SliceStable(jobs, func(i, j int) bool { return jobs[i].DeliveryID.String() < jobs[j].DeliveryID.String() })
	return jobs, nil
}

func (s *Store) GetWebhookDelivery(ctx context.Context, deliveryID uuid.UUID) (*WebhookDeliveryRecord, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT
			id, event_id, webhook_id, CAST(site_id AS VARCHAR), event_type, webhook_name,
			destination_url, signing_secret, CAST(payload_json AS VARCHAR), status, attempt_count,
			next_attempt_at, last_attempt_at, completed_at, response_status,
			last_error_code, last_error_message, created_at, updated_at
		FROM webhook_deliveries
		WHERE id = ?
	`, deliveryID)
	result, err := scanWebhookDelivery(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get webhook delivery: %w", err)
	}
	return &result, nil
}

func (s *Store) ClaimWebhookDelivery(ctx context.Context, deliveryID uuid.UUID, now time.Time) (*WebhookDeliveryRecord, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE webhook_deliveries
		SET status = ?, last_attempt_at = ?, updated_at = ?
		WHERE id = ?
			AND status IN (?, ?)
			AND (next_attempt_at IS NULL OR next_attempt_at <= ?)
	`, WebhookDeliveryProcessing, now.UTC(), now.UTC(), deliveryID, WebhookDeliveryPending, WebhookDeliveryRetrying, now.UTC())
	if err != nil {
		return nil, fmt.Errorf("claim webhook delivery: %w", err)
	}
	if affected, ok := rowsAffected(result); ok && affected == 0 {
		return nil, nil
	}
	delivery, err := s.GetWebhookDelivery(ctx, deliveryID)
	if err != nil {
		return nil, err
	}
	if delivery == nil || delivery.Status != WebhookDeliveryProcessing {
		return nil, nil
	}
	return delivery, nil
}

func (s *Store) RecordWebhookDeliveryAttempt(ctx context.Context, deliveryID uuid.UUID, result WebhookAttemptResult) error {
	delivery, err := s.GetWebhookDelivery(ctx, deliveryID)
	if err != nil {
		return err
	}
	if delivery == nil {
		return fmt.Errorf("webhook delivery not found")
	}
	if result.Status != WebhookDeliverySucceeded && result.Status != WebhookDeliveryRetrying && result.Status != WebhookDeliveryFailed {
		return fmt.Errorf("invalid webhook delivery result status %q", result.Status)
	}
	if result.StartedAt.IsZero() {
		result.StartedAt = time.Now().UTC()
	}
	if result.CompletedAt.IsZero() {
		result.CompletedAt = time.Now().UTC()
	}
	attemptNumber := delivery.AttemptCount + 1
	var responseStatus any
	if result.ResponseStatus > 0 {
		responseStatus = result.ResponseStatus
	}
	var completedAt any
	if result.Status == WebhookDeliverySucceeded || result.Status == WebhookDeliveryFailed {
		completedAt = result.CompletedAt.UTC()
	}

	return s.Transact(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO webhook_delivery_attempts (
				id, delivery_id, site_id, attempt_number, status, response_status,
				error_code, error_message, started_at, completed_at, next_attempt_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, uuid.New(), deliveryID, nullableUUIDPtr(delivery.SiteID), attemptNumber, result.Status, responseStatus,
			result.ErrorCode, result.ErrorMessage, result.StartedAt.UTC(), result.CompletedAt.UTC(), result.NextAttemptAt); err != nil {
			return fmt.Errorf("create webhook delivery attempt: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE webhook_deliveries
			SET status = ?, attempt_count = ?, next_attempt_at = ?, completed_at = ?,
				response_status = ?, last_error_code = ?, last_error_message = ?, updated_at = ?
			WHERE id = ?
		`, result.Status, attemptNumber, result.NextAttemptAt, completedAt, responseStatus,
			result.ErrorCode, result.ErrorMessage, result.CompletedAt.UTC(), deliveryID); err != nil {
			return fmt.Errorf("update webhook delivery attempt result: %w", err)
		}
		return nil
	})
}

type webhookDeliveryScanner interface {
	Scan(dest ...any) error
}

func scanWebhookDelivery(scanner webhookDeliveryScanner) (WebhookDeliveryRecord, error) {
	var result WebhookDeliveryRecord
	var siteID sql.NullString
	var payload string
	var nextAttempt, lastAttempt, completed sql.NullTime
	var responseStatus sql.NullInt64
	if err := scanner.Scan(
		&result.ID, &result.EventID, &result.WebhookID, &siteID, &result.EventType, &result.WebhookName,
		&result.DestinationURL, &result.SigningSecret, &payload, &result.Status, &result.AttemptCount,
		&nextAttempt, &lastAttempt, &completed, &responseStatus,
		&result.LastErrorCode, &result.LastErrorMessage, &result.CreatedAt, &result.UpdatedAt,
	); err != nil {
		return result, err
	}
	if siteID.Valid && siteID.String != "" {
		parsed, err := uuid.Parse(siteID.String)
		if err != nil {
			return result, fmt.Errorf("parse webhook delivery site ID: %w", err)
		}
		result.SiteID = &parsed
	}
	result.Payload = []byte(payload)
	if nextAttempt.Valid {
		value := nextAttempt.Time.UTC()
		result.NextAttemptAt = &value
	}
	if lastAttempt.Valid {
		value := lastAttempt.Time.UTC()
		result.LastAttemptAt = &value
	}
	if completed.Valid {
		value := completed.Time.UTC()
		result.CompletedAt = &value
	}
	if responseStatus.Valid {
		result.ResponseStatus = int(responseStatus.Int64)
	}
	return result, nil
}

func (s *Store) RecoverStaleWebhookDeliveries(ctx context.Context, staleBefore, now time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE webhook_deliveries
		SET status = ?, next_attempt_at = ?, last_error_code = ?, last_error_message = ?, updated_at = ?
		WHERE status = ? AND last_attempt_at < ?
	`, WebhookDeliveryRetrying, now.UTC(), "worker_interrupted", "delivery worker stopped before recording an outcome", now.UTC(), WebhookDeliveryProcessing, staleBefore.UTC())
	if err != nil {
		return 0, fmt.Errorf("recover stale webhook deliveries: %w", err)
	}
	affected, _ := rowsAffected(result)
	return affected, nil
}

func (s *Store) DeleteWebhookHistoryBefore(ctx context.Context, cutoff time.Time) (WebhookRetentionResult, error) {
	var result WebhookRetentionResult
	attempts, err := s.db.ExecContext(ctx, `
		DELETE FROM webhook_delivery_attempts WHERE completed_at < ?
	`, cutoff.UTC())
	if err != nil {
		return result, fmt.Errorf("delete expired webhook delivery attempts: %w", err)
	}
	result.Attempts, _ = rowsAffected(attempts)

	deliveries, err := s.db.ExecContext(ctx, `
		DELETE FROM webhook_deliveries
		WHERE status IN (?, ?) AND completed_at < ?
	`, WebhookDeliverySucceeded, WebhookDeliveryFailed, cutoff.UTC())
	if err != nil {
		return result, fmt.Errorf("delete expired webhook deliveries: %w", err)
	}
	result.Deliveries, _ = rowsAffected(deliveries)

	events, err := s.db.ExecContext(ctx, `
		DELETE FROM webhook_events
		WHERE created_at < ?
			AND id NOT IN (SELECT event_id FROM webhook_deliveries)
	`, cutoff.UTC())
	if err != nil {
		return result, fmt.Errorf("delete expired webhook events: %w", err)
	}
	result.Events, _ = rowsAffected(events)
	return result, nil
}

func (s *Store) ListWebhookDeliveries(ctx context.Context, webhookID uuid.UUID, siteID *uuid.UUID, limit int) ([]api.WebhookDelivery, error) {
	configured, err := s.GetWebhook(ctx, webhookID, siteID)
	if err != nil {
		return nil, err
	}
	if configured == nil {
		return nil, ErrWebhookNotFound
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			id, event_id, webhook_id, CAST(site_id AS VARCHAR), event_type, status, attempt_count,
			next_attempt_at, last_attempt_at, completed_at, response_status,
			last_error_code, last_error_message, created_at
		FROM webhook_deliveries
		WHERE webhook_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, webhookID, limit)
	if err != nil {
		return nil, fmt.Errorf("list webhook deliveries: %w", err)
	}
	defer rows.Close()

	result := make([]api.WebhookDelivery, 0)
	for rows.Next() {
		var item api.WebhookDelivery
		var rawSiteID sql.NullString
		var nextAttempt, lastAttempt, completed sql.NullTime
		var responseStatus sql.NullInt64
		if err := rows.Scan(
			&item.ID, &item.EventID, &item.WebhookID, &rawSiteID, &item.EventType, &item.Status, &item.AttemptCount,
			&nextAttempt, &lastAttempt, &completed, &responseStatus,
			&item.LastErrorCode, &item.LastErrorMessage, &item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan webhook delivery log: %w", err)
		}
		if rawSiteID.Valid && rawSiteID.String != "" {
			parsed, err := uuid.Parse(rawSiteID.String)
			if err != nil {
				return nil, fmt.Errorf("parse webhook delivery log site ID: %w", err)
			}
			item.SiteID = &parsed
		}
		item.NextAttemptAt = nullableTimeValue(nextAttempt)
		item.LastAttemptAt = nullableTimeValue(lastAttempt)
		item.CompletedAt = nullableTimeValue(completed)
		if responseStatus.Valid {
			item.ResponseStatus = int(responseStatus.Int64)
		}
		item.Attempts, err = s.listWebhookDeliveryAttempts(ctx, item.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate webhook delivery logs: %w", err)
	}
	return result, nil
}

func (s *Store) listWebhookDeliveryAttempts(ctx context.Context, deliveryID uuid.UUID) ([]api.WebhookDeliveryAttempt, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, attempt_number, status, response_status, error_code, error_message, started_at, completed_at, next_attempt_at
		FROM webhook_delivery_attempts
		WHERE delivery_id = ?
		ORDER BY attempt_number DESC
	`, deliveryID)
	if err != nil {
		return nil, fmt.Errorf("list webhook delivery attempts: %w", err)
	}
	defer rows.Close()
	result := make([]api.WebhookDeliveryAttempt, 0)
	for rows.Next() {
		var item api.WebhookDeliveryAttempt
		var responseStatus sql.NullInt64
		var nextAttempt sql.NullTime
		if err := rows.Scan(&item.ID, &item.AttemptNumber, &item.Status, &responseStatus, &item.ErrorCode, &item.ErrorMessage, &item.StartedAt, &item.CompletedAt, &nextAttempt); err != nil {
			return nil, fmt.Errorf("scan webhook delivery attempt: %w", err)
		}
		if responseStatus.Valid {
			item.ResponseStatus = int(responseStatus.Int64)
		}
		item.NextAttemptAt = nullableTimeValue(nextAttempt)
		result = append(result, item)
	}
	return result, rows.Err()
}
