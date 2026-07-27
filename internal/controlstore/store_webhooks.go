package controlstore

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/webhooks"
)

var ErrWebhookNotFound = errors.New("webhook not found")

func (s *Store) CreateWebhook(ctx context.Context, siteID *uuid.UUID, input api.WebhookInput) (*api.Webhook, string, error) {
	return s.createWebhook(ctx, siteID, input, nil)
}

func (s *Store) CreateWebhookWithAudit(ctx context.Context, siteID *uuid.UUID, input api.WebhookInput, audit AuditEntryParams) (*api.Webhook, string, error) {
	return s.createWebhook(ctx, siteID, input, &audit)
}

func (s *Store) createWebhook(ctx context.Context, siteID *uuid.UUID, input api.WebhookInput, audit *AuditEntryParams) (*api.Webhook, string, error) {
	input, err := normalizeWebhookInput(siteID, input)
	if err != nil {
		return nil, "", err
	}
	secret, err := generateWebhookSecret()
	if err != nil {
		return nil, "", err
	}

	id := uuid.New()
	now := time.Now().UTC()
	if err := s.transact(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO webhooks (id, site_id, name, description, destination_url, secret, enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, id, nullableUUIDPtr(siteID), input.Name, input.Description, input.URL, secret, input.Enabled, now, now); err != nil {
			return fmt.Errorf("create webhook: %w", err)
		}
		if err := insertWebhookSubscriptions(ctx, tx, id, input.Events); err != nil {
			return err
		}
		return appendWebhookAuditTx(ctx, tx, audit, id, input.Name)
	}); err != nil {
		return nil, "", err
	}

	created, err := s.GetWebhook(ctx, id, siteID)
	return created, secret, err
}

func (s *Store) ListWebhooks(ctx context.Context, siteID *uuid.UUID) ([]api.Webhook, error) {
	where, args := webhookScopeWhere(siteID)
	// where comes from webhookScopeWhere and contains only fixed predicates.
	//nolint:gosec
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, CAST(site_id AS VARCHAR), name, description, destination_url, enabled, created_at, updated_at
		FROM webhooks
		WHERE `+where+`
		ORDER BY created_at DESC
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list webhooks: %w", err)
	}
	defer rows.Close()

	result := make([]api.Webhook, 0)
	for rows.Next() {
		webhook, err := scanWebhook(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, webhook)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate webhooks: %w", err)
	}
	for i := range result {
		result[i].Events, err = s.listWebhookSubscriptions(ctx, result[i].ID)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (s *Store) GetWebhook(ctx context.Context, webhookID uuid.UUID, siteID *uuid.UUID) (*api.Webhook, error) {
	where, args := webhookScopeWhere(siteID)
	queryArgs := append([]any{webhookID}, args...)
	// where comes from webhookScopeWhere and contains only fixed predicates.
	//nolint:gosec
	row := s.db.QueryRowContext(ctx, `
		SELECT id, CAST(site_id AS VARCHAR), name, description, destination_url, enabled, created_at, updated_at
		FROM webhooks
		WHERE id = ? AND `+where,
		queryArgs...,
	)
	webhook, err := scanWebhook(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	webhook.Events, err = s.listWebhookSubscriptions(ctx, webhook.ID)
	if err != nil {
		return nil, err
	}
	return &webhook, nil
}

func (s *Store) UpdateWebhook(ctx context.Context, webhookID uuid.UUID, siteID *uuid.UUID, input api.WebhookInput) (*api.Webhook, error) {
	return s.updateWebhook(ctx, webhookID, siteID, input, nil)
}

func (s *Store) UpdateWebhookWithAudit(ctx context.Context, webhookID uuid.UUID, siteID *uuid.UUID, input api.WebhookInput, audit AuditEntryParams) (*api.Webhook, error) {
	return s.updateWebhook(ctx, webhookID, siteID, input, &audit)
}

func (s *Store) updateWebhook(ctx context.Context, webhookID uuid.UUID, siteID *uuid.UUID, input api.WebhookInput, audit *AuditEntryParams) (*api.Webhook, error) {
	input, err := normalizeWebhookInput(siteID, input)
	if err != nil {
		return nil, err
	}
	existing, err := s.GetWebhook(ctx, webhookID, siteID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrWebhookNotFound
	}
	where, args := webhookScopeWhere(siteID)
	now := time.Now().UTC()

	err = s.transact(ctx, func(tx *sql.Tx) error {
		queryArgs := make([]any, 0, 6+len(args))
		queryArgs = append(queryArgs, input.Name, input.Description, input.URL, input.Enabled, now, webhookID)
		queryArgs = append(queryArgs, args...)
		// where comes from webhookScopeWhere and contains only fixed predicates.
		//nolint:gosec
		result, err := tx.ExecContext(ctx, `
			UPDATE webhooks
			SET name = ?, description = ?, destination_url = ?, enabled = ?, updated_at = ?
			WHERE id = ? AND `+where,
			queryArgs...,
		)
		if err != nil {
			return fmt.Errorf("update webhook: %w", err)
		}
		if affected, ok := rowsAffected(result); ok && affected == 0 {
			return ErrWebhookNotFound
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM webhook_event_subscriptions WHERE webhook_id = ?", webhookID); err != nil {
			return fmt.Errorf("replace webhook event subscriptions: %w", err)
		}
		if err := insertWebhookSubscriptions(ctx, tx, webhookID, input.Events); err != nil {
			return err
		}
		return appendWebhookAuditTx(ctx, tx, audit, webhookID, input.Name)
	})
	if err != nil {
		return nil, err
	}

	updated, err := s.GetWebhook(ctx, webhookID, siteID)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, ErrWebhookNotFound
	}
	return updated, nil
}

func (s *Store) RotateWebhookSecret(ctx context.Context, webhookID uuid.UUID, siteID *uuid.UUID) (*api.Webhook, string, error) {
	return s.rotateWebhookSecret(ctx, webhookID, siteID, nil)
}

func (s *Store) RotateWebhookSecretWithAudit(ctx context.Context, webhookID uuid.UUID, siteID *uuid.UUID, audit AuditEntryParams) (*api.Webhook, string, error) {
	return s.rotateWebhookSecret(ctx, webhookID, siteID, &audit)
}

func (s *Store) rotateWebhookSecret(ctx context.Context, webhookID uuid.UUID, siteID *uuid.UUID, audit *AuditEntryParams) (*api.Webhook, string, error) {
	existing, err := s.GetWebhook(ctx, webhookID, siteID)
	if err != nil {
		return nil, "", err
	}
	if existing == nil {
		return nil, "", ErrWebhookNotFound
	}
	secret, err := generateWebhookSecret()
	if err != nil {
		return nil, "", err
	}
	where, args := webhookScopeWhere(siteID)
	now := time.Now().UTC()
	err = s.transact(ctx, func(tx *sql.Tx) error {
		queryArgs := make([]any, 0, 3+len(args))
		queryArgs = append(queryArgs, secret, now, webhookID)
		queryArgs = append(queryArgs, args...)
		// where comes from webhookScopeWhere and contains only fixed predicates.
		//nolint:gosec
		result, err := tx.ExecContext(ctx, `
			UPDATE webhooks SET secret = ?, updated_at = ? WHERE id = ? AND `+where,
			queryArgs...,
		)
		if err != nil {
			return fmt.Errorf("rotate webhook secret: %w", err)
		}
		if affected, ok := rowsAffected(result); ok && affected == 0 {
			return ErrWebhookNotFound
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE webhook_deliveries
			SET signing_secret = ?, updated_at = ?
			WHERE webhook_id = ? AND status IN (?, ?, ?)
		`, secret, now, webhookID, WebhookDeliveryPending, WebhookDeliveryRetrying, WebhookDeliveryProcessing); err != nil {
			return fmt.Errorf("rotate outstanding webhook delivery secrets: %w", err)
		}
		return appendWebhookAuditTx(ctx, tx, audit, webhookID, existing.Name)
	})
	if err != nil {
		return nil, "", err
	}
	updated, err := s.GetWebhook(ctx, webhookID, siteID)
	if err != nil {
		return nil, "", err
	}
	if updated == nil {
		return nil, "", ErrWebhookNotFound
	}
	return updated, secret, nil
}

func (s *Store) DeleteWebhook(ctx context.Context, webhookID uuid.UUID, siteID *uuid.UUID) error {
	return s.deleteWebhook(ctx, webhookID, siteID, nil)
}

func (s *Store) DeleteWebhookWithAudit(ctx context.Context, webhookID uuid.UUID, siteID *uuid.UUID, audit AuditEntryParams) error {
	return s.deleteWebhook(ctx, webhookID, siteID, &audit)
}

func (s *Store) deleteWebhook(ctx context.Context, webhookID uuid.UUID, siteID *uuid.UUID, audit *AuditEntryParams) error {
	found, err := s.GetWebhook(ctx, webhookID, siteID)
	if err != nil {
		return err
	}
	if found == nil {
		return ErrWebhookNotFound
	}
	where, args := webhookScopeWhere(siteID)
	queryArgs := append([]any{webhookID}, args...)
	return s.transact(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM webhook_event_subscriptions WHERE webhook_id = ?", webhookID); err != nil {
			return fmt.Errorf("delete webhook subscriptions: %w", err)
		}
		// where comes from webhookScopeWhere and contains only fixed predicates.
		//nolint:gosec
		if _, err := tx.ExecContext(ctx, "DELETE FROM webhooks WHERE id = ? AND "+where, queryArgs...); err != nil {
			return fmt.Errorf("delete webhook: %w", err)
		}
		return appendWebhookAuditTx(ctx, tx, audit, webhookID, found.Name)
	})
}

func appendWebhookAuditTx(ctx context.Context, tx *sql.Tx, audit *AuditEntryParams, webhookID uuid.UUID, webhookName string) error {
	if audit == nil {
		return nil
	}
	audit.TargetType = "webhook"
	audit.TargetID = webhookID.String()
	audit.TargetLabel = webhookName
	audit.Details = fmt.Sprintf("Webhook %q %s (webhook_id=%s)", webhookName, strings.TrimPrefix(audit.Action, "webhook."), webhookID)
	if err := appendAuditEntryTx(ctx, tx, *audit); err != nil {
		return fmt.Errorf("append webhook audit entry: %w", err)
	}
	return nil
}

func normalizeWebhookInput(siteID *uuid.UUID, input api.WebhookInput) (api.WebhookInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.URL = strings.TrimSpace(input.URL)
	if input.Name == "" {
		return input, fmt.Errorf("webhook name is required")
	}
	if input.URL == "" {
		return input, fmt.Errorf("webhook URL is required")
	}
	scope := webhooks.ScopeForSiteID(siteID != nil)
	if err := webhooks.ValidateEventSelection(scope, input.Events); err != nil {
		return input, err
	}
	input.Events = append([]string(nil), input.Events...)
	sort.Strings(input.Events)
	return input, nil
}

func insertWebhookSubscriptions(ctx context.Context, tx *sql.Tx, webhookID uuid.UUID, events []string) error {
	for _, eventType := range events {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO webhook_event_subscriptions (webhook_id, event_type) VALUES (?, ?)
		`, webhookID, eventType); err != nil {
			return fmt.Errorf("create webhook event subscription: %w", err)
		}
	}
	return nil
}

func (s *Store) listWebhookSubscriptions(ctx context.Context, webhookID uuid.UUID) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT event_type FROM webhook_event_subscriptions WHERE webhook_id = ? ORDER BY event_type
	`, webhookID)
	if err != nil {
		return nil, fmt.Errorf("list webhook event subscriptions: %w", err)
	}
	defer rows.Close()

	events := make([]string, 0)
	for rows.Next() {
		var eventType string
		if err := rows.Scan(&eventType); err != nil {
			return nil, fmt.Errorf("scan webhook event subscription: %w", err)
		}
		events = append(events, eventType)
	}
	return events, rows.Err()
}

func webhookScopeWhere(siteID *uuid.UUID) (string, []any) {
	if siteID == nil {
		return "site_id IS NULL", nil
	}
	return "site_id = ?", []any{*siteID}
}

type webhookScanner interface {
	Scan(dest ...any) error
}

func scanWebhook(scanner webhookScanner) (api.Webhook, error) {
	var result api.Webhook
	var siteID sql.NullString
	if err := scanner.Scan(
		&result.ID,
		&siteID,
		&result.Name,
		&result.Description,
		&result.URL,
		&result.Enabled,
		&result.CreatedAt,
		&result.UpdatedAt,
	); err != nil {
		return result, err
	}
	if siteID.Valid && strings.TrimSpace(siteID.String) != "" {
		parsed, err := uuid.Parse(siteID.String)
		if err != nil {
			return result, fmt.Errorf("parse webhook site ID: %w", err)
		}
		result.SiteID = &parsed
		result.Scope = string(webhooks.ScopeSite)
	} else {
		result.Scope = string(webhooks.ScopeInstance)
	}
	result.Events = []string{}
	return result, nil
}

func generateWebhookSecret() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate webhook secret: %w", err)
	}
	return "whsec_" + hex.EncodeToString(bytes), nil
}
