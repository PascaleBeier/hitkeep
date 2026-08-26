package database

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/webhooks"
	json "hitkeep/jsonapi"
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
	Deduplicate               bool
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
	jobsByEvent, err := s.EnqueueWebhookEvents(ctx, []WebhookEventInput{input})
	if err != nil {
		return nil, err
	}
	return jobsByEvent[0], nil
}

type preparedWebhookEvent struct {
	index        int
	input        WebhookEventInput
	storedSiteID *uuid.UUID
	eventBody    []byte
	subscribers  []webhookSubscriber
}

func (s *Store) EnqueueWebhookEvents(ctx context.Context, inputs []WebhookEventInput) ([][]WebhookDeliveryJob, error) {
	jobsByEvent := make([][]WebhookDeliveryJob, len(inputs))
	if len(inputs) == 0 {
		return jobsByEvent, nil
	}

	now := time.Now().UTC()
	prepared := make([]preparedWebhookEvent, 0, len(inputs))
	existingByEvent, err := s.existingWebhookDeliveryJobs(ctx, inputs)
	if err != nil {
		return nil, err
	}
	for index := range inputs {
		if existing, ok := existingByEvent[inputs[index].ID]; ok {
			jobsByEvent[index] = existing
			continue
		}
		input, eventBody, storedSiteID, err := prepareWebhookEventInput(inputs[index], now)
		if err != nil {
			return nil, err
		}
		subscribers, err := s.enabledWebhookSubscribers(ctx, input.SiteID, input.TargetWebhookID, input.EventType)
		if err != nil {
			return nil, err
		}
		if len(subscribers) == 0 {
			jobsByEvent[index] = []WebhookDeliveryJob{}
			continue
		}
		inputs[index] = input
		prepared = append(prepared, preparedWebhookEvent{index: index, input: input, storedSiteID: storedSiteID, eventBody: eventBody, subscribers: subscribers})
	}
	if len(prepared) == 0 {
		return jobsByEvent, nil
	}

	err = s.Transact(ctx, func(tx *sql.Tx) error {
		for _, event := range prepared {
			jobs, err := insertWebhookEventTx(ctx, tx, event.input, event.storedSiteID, event.eventBody, event.subscribers, now)
			if err != nil {
				return err
			}
			jobsByEvent[event.index] = jobs
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return jobsByEvent, nil
}

func (s *Store) existingWebhookDeliveryJobs(ctx context.Context, inputs []WebhookEventInput) (map[uuid.UUID][]WebhookDeliveryJob, error) {
	ids := make([]uuid.UUID, 0, len(inputs))
	seen := make(map[uuid.UUID]struct{}, len(inputs))
	for _, input := range inputs {
		if !input.Deduplicate || input.ID == uuid.Nil {
			continue
		}
		if _, ok := seen[input.ID]; ok {
			continue
		}
		seen[input.ID] = struct{}{}
		ids = append(ids, input.ID)
	}
	result := make(map[uuid.UUID][]WebhookDeliveryJob, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for index, id := range ids {
		args[index] = id
	}
	// placeholders is generated from the input count and all IDs remain bound parameters.
	//nolint:gosec
	rows, err := s.db.QueryContext(ctx, `
		SELECT CAST(event.id AS VARCHAR), CAST(delivery.id AS VARCHAR),
			CAST(delivery.webhook_id AS VARCHAR), CAST(delivery.payload_json AS VARCHAR)
		FROM webhook_events event
		LEFT JOIN webhook_deliveries delivery ON delivery.event_id = event.id
		WHERE event.id IN (`+placeholders+`)
		ORDER BY event.id, delivery.id
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("load deduplicated webhook deliveries: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var eventID string
		var deliveryID, webhookID, payload sql.NullString
		if err := rows.Scan(&eventID, &deliveryID, &webhookID, &payload); err != nil {
			return nil, fmt.Errorf("scan deduplicated webhook delivery: %w", err)
		}
		parsedEventID, err := uuid.Parse(eventID)
		if err != nil {
			return nil, fmt.Errorf("parse deduplicated webhook event ID: %w", err)
		}
		if _, ok := result[parsedEventID]; !ok {
			result[parsedEventID] = []WebhookDeliveryJob{}
		}
		if !deliveryID.Valid {
			continue
		}
		parsedDeliveryID, err := uuid.Parse(deliveryID.String)
		if err != nil {
			return nil, fmt.Errorf("parse deduplicated webhook delivery ID: %w", err)
		}
		parsedWebhookID, err := uuid.Parse(webhookID.String)
		if err != nil {
			return nil, fmt.Errorf("parse deduplicated webhook ID: %w", err)
		}
		result[parsedEventID] = append(result[parsedEventID], WebhookDeliveryJob{
			DeliveryID: parsedDeliveryID, EventID: parsedEventID, WebhookID: parsedWebhookID, Payload: []byte(payload.String),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate deduplicated webhook deliveries: %w", err)
	}
	return result, nil
}

func prepareWebhookEventInput(input WebhookEventInput, now time.Time) (WebhookEventInput, []byte, *uuid.UUID, error) {
	if !webhooks.EventAllowedForScope(input.EventType, webhooks.ScopeInstance) {
		return input, nil, nil, fmt.Errorf("unsupported webhook event type %q", input.EventType)
	}
	if input.APIVersion == "" {
		return input, nil, nil, fmt.Errorf("webhook API version is required")
	}
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
		return input, nil, nil, fmt.Errorf("marshal webhook event payload: %w", err)
	}
	storedSiteID := input.SiteID
	if input.PreserveAfterSiteDeletion {
		storedSiteID = nil
	}
	return input, eventBody, storedSiteID, nil
}

func (s *Store) HasEnabledWebhookSubscribers(ctx context.Context, siteID *uuid.UUID, eventType string) (bool, error) {
	subscribers, err := s.enabledWebhookSubscribers(ctx, siteID, nil, eventType)
	if err != nil {
		return false, err
	}
	return len(subscribers) > 0, nil
}

func insertWebhookEventTx(ctx context.Context, tx *sql.Tx, input WebhookEventInput, storedSiteID *uuid.UUID, eventBody []byte, subscribers []webhookSubscriber, now time.Time) ([]WebhookDeliveryJob, error) {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO webhook_events (id, site_id, event_type, api_version, payload_json, occurred_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, input.ID, nullableUUIDPtr(storedSiteID), input.EventType, input.APIVersion, string(eventBody), input.OccurredAt, now); err != nil {
		return nil, fmt.Errorf("create webhook event: %w", err)
	}

	jobs := make([]WebhookDeliveryJob, 0, len(subscribers))
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
			return nil, fmt.Errorf("marshal webhook delivery payload: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO webhook_deliveries (
				id, event_id, webhook_id, site_id, event_type, webhook_name,
				destination_url, signing_secret, payload_json, status, attempt_count,
				next_attempt_at, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?)
		`, deliveryID, input.ID, subscriber.ID, nullableUUIDPtr(storedSiteID), input.EventType, subscriber.Name,
			subscriber.URL, subscriber.Secret, string(payload), WebhookDeliveryPending, now, now, now); err != nil {
			return nil, fmt.Errorf("create webhook delivery: %w", err)
		}
		jobs = append(jobs, WebhookDeliveryJob{DeliveryID: deliveryID, EventID: input.ID, WebhookID: subscriber.ID, Payload: payload})
	}
	return jobs, nil
}

func (s *Store) stageSiteDeletionWebhookEvent(ctx context.Context, siteID uuid.UUID, input WebhookEventInput, eventBody []byte, subscribers []webhookSubscriber, now time.Time) error {
	return s.Transact(ctx, func(tx *sql.Tx) error {
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
				return fmt.Errorf("marshal final site webhook delivery: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO site_deletion_webhook_outbox (
					delivery_id, event_id, source_site_id, webhook_id, event_type, api_version,
					webhook_name, destination_url, signing_secret, event_payload_json, payload_json,
					occurred_at, created_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`, deliveryID, input.ID, siteID, subscriber.ID, input.EventType, input.APIVersion,
				subscriber.Name, subscriber.URL, subscriber.Secret, string(eventBody), string(payload), input.OccurredAt, now); err != nil {
				return fmt.Errorf("stage final site webhook delivery: %w", err)
			}
		}
		return nil
	})
}

func (s *Store) cancelStagedSiteDeletionWebhookEvent(ctx context.Context, eventID uuid.UUID) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM site_deletion_webhook_outbox WHERE event_id = ?", eventID); err != nil {
		return fmt.Errorf("cancel staged site deletion webhook event: %w", err)
	}
	return nil
}

func (s *Store) CommitStagedSiteDeletionWebhookEvent(ctx context.Context, eventID uuid.UUID, now time.Time) ([]WebhookDeliveryJob, error) {
	type stagedDelivery struct {
		deliveryID, webhookID                           uuid.UUID
		eventType, apiVersion, webhookName, destination string
		secret, eventPayload, payload                   string
		occurredAt                                      time.Time
	}
	staged, err := func() ([]stagedDelivery, error) {
		rows, err := s.db.QueryContext(ctx, `
			SELECT delivery_id, webhook_id, event_type, api_version, webhook_name, destination_url,
				signing_secret, CAST(event_payload_json AS VARCHAR), CAST(payload_json AS VARCHAR), occurred_at
			FROM site_deletion_webhook_outbox
			WHERE event_id = ?
			ORDER BY delivery_id
		`, eventID)
		if err != nil {
			return nil, fmt.Errorf("load staged site deletion webhook event: %w", err)
		}
		defer rows.Close()
		result := make([]stagedDelivery, 0)
		for rows.Next() {
			var item stagedDelivery
			if err := rows.Scan(&item.deliveryID, &item.webhookID, &item.eventType, &item.apiVersion, &item.webhookName,
				&item.destination, &item.secret, &item.eventPayload, &item.payload, &item.occurredAt); err != nil {
				return nil, fmt.Errorf("scan staged site deletion webhook event: %w", err)
			}
			result = append(result, item)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("iterate staged site deletion webhook events: %w", err)
		}
		return result, nil
	}()
	if err != nil {
		return nil, err
	}
	if len(staged) == 0 {
		return []WebhookDeliveryJob{}, nil
	}

	jobs := make([]WebhookDeliveryJob, 0, len(staged))
	err = s.Transact(ctx, func(tx *sql.Tx) error {
		first := staged[0]
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO webhook_events (id, site_id, event_type, api_version, payload_json, occurred_at, created_at)
			VALUES (?, NULL, ?, ?, ?, ?, ?)
		`, eventID, first.eventType, first.apiVersion, first.eventPayload, first.occurredAt, now.UTC()); err != nil {
			return fmt.Errorf("materialize site deletion webhook event: %w", err)
		}
		for _, item := range staged {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO webhook_deliveries (
					id, event_id, webhook_id, site_id, event_type, webhook_name,
					destination_url, signing_secret, payload_json, status, attempt_count,
					next_attempt_at, created_at, updated_at
				) VALUES (?, ?, ?, NULL, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?)
			`, item.deliveryID, eventID, item.webhookID, item.eventType, item.webhookName, item.destination,
				item.secret, item.payload, WebhookDeliveryPending, now.UTC(), now.UTC(), now.UTC()); err != nil {
				return fmt.Errorf("materialize site deletion webhook delivery: %w", err)
			}
			jobs = append(jobs, WebhookDeliveryJob{DeliveryID: item.deliveryID, EventID: eventID, WebhookID: item.webhookID, Payload: []byte(item.payload)})
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM site_deletion_webhook_outbox WHERE event_id = ?", eventID); err != nil {
			return fmt.Errorf("clear staged site deletion webhook event: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return jobs, nil
}

func (s *Store) RecoverStagedSiteDeletionWebhookEvents(ctx context.Context, now time.Time) ([][]WebhookDeliveryJob, error) {
	ids, err := func() ([]uuid.UUID, error) {
		rows, err := s.db.QueryContext(ctx, `
			SELECT DISTINCT staged.event_id
			FROM site_deletion_webhook_outbox staged
			WHERE NOT EXISTS (SELECT 1 FROM sites WHERE sites.id = staged.source_site_id)
			ORDER BY staged.event_id
		`)
		if err != nil {
			return nil, fmt.Errorf("list recoverable site deletion webhook events: %w", err)
		}
		defer rows.Close()
		result := make([]uuid.UUID, 0)
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				return nil, fmt.Errorf("scan recoverable site deletion webhook event: %w", err)
			}
			result = append(result, id)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("iterate recoverable site deletion webhook events: %w", err)
		}
		return result, nil
	}()
	if err != nil {
		return nil, err
	}
	result := make([][]WebhookDeliveryJob, 0, len(ids))
	for _, id := range ids {
		jobs, err := s.CommitStagedSiteDeletionWebhookEvent(ctx, id, now)
		if err != nil {
			return nil, err
		}
		result = append(result, jobs)
	}
	return result, nil
}

func (s *Store) DeleteAbandonedStagedSiteDeletionWebhookEvents(ctx context.Context, preparedBefore time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM site_deletion_webhook_outbox AS staged
		WHERE staged.created_at < ?
			AND EXISTS (SELECT 1 FROM sites WHERE sites.id = staged.source_site_id)
	`, preparedBefore.UTC())
	if err != nil {
		return 0, fmt.Errorf("delete abandoned site deletion webhook events: %w", err)
	}
	affected, _ := rowsAffected(result)
	return affected, nil
}

func (s *Store) enabledWebhookSubscribers(ctx context.Context, siteID, targetWebhookID *uuid.UUID, eventType string) ([]webhookSubscriber, error) {
	return enabledWebhookSubscribersWith(ctx, s.db, siteID, targetWebhookID, eventType)
}

func enabledWebhookSubscribersWith(ctx context.Context, queryer sqlQueryContext, siteID, targetWebhookID *uuid.UUID, eventType string) ([]webhookSubscriber, error) {
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
	rows, err := queryer.QueryContext(ctx, query, queryArgs...)
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
	return s.listWebhookDeliveryJobs(ctx, dueAt, nil, limit)
}

func (s *Store) ListDispatchableWebhookDeliveryJobs(ctx context.Context, dueAt, queuedBefore time.Time, limit int) ([]WebhookDeliveryJob, error) {
	return s.listWebhookDeliveryJobs(ctx, dueAt, &queuedBefore, limit)
}

func (s *Store) listWebhookDeliveryJobs(ctx context.Context, dueAt time.Time, queuedBefore *time.Time, limit int) ([]WebhookDeliveryJob, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	queuedWhere := ""
	args := []any{WebhookDeliveryPending, WebhookDeliveryRetrying, dueAt.UTC()}
	if queuedBefore != nil {
		queuedWhere = " AND (dispatch_queued_at IS NULL OR dispatch_queued_at <= ?)"
		args = append(args, queuedBefore.UTC())
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, event_id, webhook_id, CAST(payload_json AS VARCHAR)
		FROM webhook_deliveries
		WHERE status IN (?, ?) AND next_attempt_at <= ?`+queuedWhere+`
		ORDER BY next_attempt_at, created_at
		LIMIT ?
	`, args...)
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
	slices.SortStableFunc(jobs, func(left, right WebhookDeliveryJob) int {
		return cmp.Compare(left.DeliveryID.String(), right.DeliveryID.String())
	})
	return jobs, nil
}

func (s *Store) MarkWebhookDeliveryQueued(ctx context.Context, deliveryID uuid.UUID, queuedAt time.Time) error {
	if _, err := s.db.ExecContext(ctx, `
		UPDATE webhook_deliveries SET dispatch_queued_at = ?, updated_at = ?
		WHERE id = ? AND status IN (?, ?)
	`, queuedAt.UTC(), queuedAt.UTC(), deliveryID, WebhookDeliveryPending, WebhookDeliveryRetrying); err != nil {
		return fmt.Errorf("mark webhook delivery queued: %w", err)
	}
	return nil
}

func (s *Store) ClearWebhookDeliveryQueued(ctx context.Context, deliveryID uuid.UUID, updatedAt time.Time) error {
	if _, err := s.db.ExecContext(ctx, `
		UPDATE webhook_deliveries SET dispatch_queued_at = NULL, updated_at = ?
		WHERE id = ? AND status IN (?, ?)
	`, updatedAt.UTC(), deliveryID, WebhookDeliveryPending, WebhookDeliveryRetrying); err != nil {
		return fmt.Errorf("clear webhook delivery queue marker: %w", err)
	}
	return nil
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
		SET status = ?, last_attempt_at = ?, dispatch_queued_at = NULL, updated_at = ?
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
				response_status = ?, last_error_code = ?, last_error_message = ?, dispatch_queued_at = NULL, updated_at = ?
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
		SET status = ?, next_attempt_at = ?, last_error_code = ?, last_error_message = ?, dispatch_queued_at = NULL, updated_at = ?
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
