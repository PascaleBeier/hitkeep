package webhooks

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	authcore "hitkeep/internal/auth"
	"hitkeep/internal/database"
	"hitkeep/internal/server/shared"
	webhookcore "hitkeep/internal/webhooks"
)

type handler struct {
	ctx *shared.Context
}

func Register(mux *http.ServeMux, ctx *shared.Context) {
	h := &handler{ctx: ctx}

	instanceConfig := shared.HandlerConfig{
		InstancePerm: authcore.PermInstanceManageWebhooks,
		HumanOnly:    true,
		RateLimiter:  ctx.ApiLimiter,
	}
	siteConfig := shared.HandlerConfig{
		SitePerm:    authcore.PermSiteManageWebhooks,
		HumanOnly:   true,
		RateLimiter: ctx.ApiLimiter,
	}

	mux.HandleFunc("GET /api/admin/webhooks/events", ctx.Handler(instanceConfig, h.handleCatalog(webhookcore.ScopeInstance)))
	mux.HandleFunc("GET /api/admin/webhooks", ctx.Handler(instanceConfig, h.handleList(nil)))
	mux.HandleFunc("POST /api/admin/webhooks", ctx.Handler(instanceConfig, h.handleCreate(nil)))
	mux.HandleFunc("PUT /api/admin/webhooks/{webhookID}", ctx.Handler(instanceConfig, h.handleUpdate(nil)))
	mux.HandleFunc("POST /api/admin/webhooks/{webhookID}/rotate", ctx.Handler(instanceConfig, h.handleRotate(nil)))
	mux.HandleFunc("POST /api/admin/webhooks/{webhookID}/test", ctx.Handler(instanceConfig, h.handleTest(nil)))
	mux.HandleFunc("GET /api/admin/webhooks/{webhookID}/deliveries", ctx.Handler(instanceConfig, h.handleDeliveries(nil)))
	mux.HandleFunc("DELETE /api/admin/webhooks/{webhookID}", ctx.Handler(instanceConfig, h.handleDelete(nil)))

	mux.HandleFunc("GET /api/sites/{id}/webhooks/events", ctx.Handler(siteConfig, h.siteScoped(func(siteID *uuid.UUID) http.HandlerFunc {
		return h.handleCatalog(webhookcore.ScopeSite)
	})))
	mux.HandleFunc("GET /api/sites/{id}/webhooks", ctx.Handler(siteConfig, h.siteScoped(h.handleList)))
	mux.HandleFunc("POST /api/sites/{id}/webhooks", ctx.Handler(siteConfig, h.siteScoped(h.handleCreate)))
	mux.HandleFunc("PUT /api/sites/{id}/webhooks/{webhookID}", ctx.Handler(siteConfig, h.siteScoped(h.handleUpdate)))
	mux.HandleFunc("POST /api/sites/{id}/webhooks/{webhookID}/rotate", ctx.Handler(siteConfig, h.siteScoped(h.handleRotate)))
	mux.HandleFunc("POST /api/sites/{id}/webhooks/{webhookID}/test", ctx.Handler(siteConfig, h.siteScoped(h.handleTest)))
	mux.HandleFunc("GET /api/sites/{id}/webhooks/{webhookID}/deliveries", ctx.Handler(siteConfig, h.siteScoped(h.handleDeliveries)))
	mux.HandleFunc("DELETE /api/sites/{id}/webhooks/{webhookID}", ctx.Handler(siteConfig, h.siteScoped(h.handleDelete)))
}

func (h *handler) siteScoped(build func(*uuid.UUID) http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
		if err != nil {
			http.Error(w, "Invalid site ID", http.StatusBadRequest)
			return
		}
		build(&siteID).ServeHTTP(w, r)
	}
}

func (h *handler) handleCatalog(scope webhookcore.Scope) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, webhookcore.Catalog(scope))
	}
}

func (h *handler) handleList(siteID *uuid.UUID) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := h.ctx.Store.ListWebhooks(r.Context(), siteID)
		if err != nil {
			slog.Error("Failed to list webhooks", "error", err, "site_id", nullableSiteLogValue(siteID))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, items)
	}
}

func (h *handler) handleCreate(siteID *uuid.UUID) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		input, ok := h.decodeAndValidateInput(w, r, siteID)
		if !ok {
			return
		}
		created, secret, err := h.ctx.Store.CreateWebhook(r.Context(), siteID, input)
		if err != nil {
			slog.Error("Failed to create webhook", "error", err, "site_id", nullableSiteLogValue(siteID))
			http.Error(w, "Failed to create webhook", http.StatusInternalServerError)
			return
		}
		if err := h.appendAudit(r, siteID, "webhook.created", created); err != nil {
			slog.Error("Failed to audit webhook create", "error", err, "webhook_id", created.ID)
			http.Error(w, "Failed to audit webhook action", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, api.WebhookSecretResponse{Webhook: *created, Secret: secret})
	}
}

func (h *handler) handleUpdate(siteID *uuid.UUID) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		webhookID, ok := parseWebhookID(w, r)
		if !ok {
			return
		}
		input, ok := h.decodeAndValidateInput(w, r, siteID)
		if !ok {
			return
		}
		updated, err := h.ctx.Store.UpdateWebhook(r.Context(), webhookID, siteID, input)
		if errors.Is(err, database.ErrWebhookNotFound) {
			http.Error(w, "Webhook not found", http.StatusNotFound)
			return
		}
		if err != nil {
			slog.Error("Failed to update webhook", "error", err, "webhook_id", webhookID)
			http.Error(w, "Failed to update webhook", http.StatusInternalServerError)
			return
		}
		if err := h.appendAudit(r, siteID, "webhook.updated", updated); err != nil {
			slog.Error("Failed to audit webhook update", "error", err, "webhook_id", webhookID)
			http.Error(w, "Failed to audit webhook action", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, updated)
	}
}

func (h *handler) handleRotate(siteID *uuid.UUID) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		webhookID, ok := parseWebhookID(w, r)
		if !ok {
			return
		}
		updated, secret, err := h.ctx.Store.RotateWebhookSecret(r.Context(), webhookID, siteID)
		if errors.Is(err, database.ErrWebhookNotFound) {
			http.Error(w, "Webhook not found", http.StatusNotFound)
			return
		}
		if err != nil {
			slog.Error("Failed to rotate webhook secret", "error", err, "webhook_id", webhookID)
			http.Error(w, "Failed to rotate webhook secret", http.StatusInternalServerError)
			return
		}
		if err := h.appendAudit(r, siteID, "webhook.secret_rotated", updated); err != nil {
			slog.Error("Failed to audit webhook rotation", "error", err, "webhook_id", webhookID)
			http.Error(w, "Failed to audit webhook action", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, api.WebhookSecretResponse{Webhook: *updated, Secret: secret})
	}
}

func (h *handler) handleDelete(siteID *uuid.UUID) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		webhookID, ok := parseWebhookID(w, r)
		if !ok {
			return
		}
		existing, err := h.ctx.Store.GetWebhook(r.Context(), webhookID, siteID)
		if err != nil {
			slog.Error("Failed to load webhook before delete", "error", err, "webhook_id", webhookID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if existing == nil {
			http.Error(w, "Webhook not found", http.StatusNotFound)
			return
		}
		if err := h.ctx.Store.DeleteWebhook(r.Context(), webhookID, siteID); err != nil {
			slog.Error("Failed to delete webhook", "error", err, "webhook_id", webhookID)
			http.Error(w, "Failed to delete webhook", http.StatusInternalServerError)
			return
		}
		if err := h.appendAudit(r, siteID, "webhook.deleted", existing); err != nil {
			slog.Error("Failed to audit webhook delete", "error", err, "webhook_id", webhookID)
			http.Error(w, "Failed to audit webhook action", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *handler) handleTest(siteID *uuid.UUID) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		webhookID, ok := parseWebhookID(w, r)
		if !ok {
			return
		}
		configured, err := h.ctx.Store.GetWebhook(r.Context(), webhookID, siteID)
		if err != nil {
			slog.Error("Failed to load webhook before test", "error", err, "webhook_id", webhookID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if configured == nil {
			http.Error(w, "Webhook not found", http.StatusNotFound)
			return
		}
		if h.ctx.Webhooks == nil {
			http.Error(w, "Webhook delivery is unavailable", http.StatusServiceUnavailable)
			return
		}
		emission, err := h.ctx.Webhooks.Emit(r.Context(), webhookcore.Event{
			Type:            webhookcore.EventWebhookTest,
			SiteID:          siteID,
			TargetWebhookID: &webhookID,
			Data: map[string]any{
				"webhook_id": webhookID.String(),
				"scope":      configured.Scope,
			},
		})
		if err != nil {
			slog.Error("Failed to create webhook test delivery", "error", err, "webhook_id", webhookID)
			http.Error(w, "Failed to create webhook test delivery", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusAccepted, api.WebhookTestResponse{EventID: emission.EventID, DeliveryIDs: emission.DeliveryIDs})
	}
}

func (h *handler) handleDeliveries(siteID *uuid.UUID) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		webhookID, ok := parseWebhookID(w, r)
		if !ok {
			return
		}
		items, err := h.ctx.Store.ListWebhookDeliveries(r.Context(), webhookID, siteID, 100)
		if errors.Is(err, database.ErrWebhookNotFound) {
			http.Error(w, "Webhook not found", http.StatusNotFound)
			return
		}
		if err != nil {
			slog.Error("Failed to list webhook deliveries", "error", err, "webhook_id", webhookID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, items)
	}
}

func (h *handler) decodeAndValidateInput(w http.ResponseWriter, r *http.Request, siteID *uuid.UUID) (api.WebhookInput, bool) {
	var input api.WebhookInput
	decoder := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return input, false
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.URL = strings.TrimSpace(input.URL)
	if input.Name == "" || len(input.Name) > 120 {
		http.Error(w, "Name is required and must be <= 120 characters", http.StatusBadRequest)
		return input, false
	}
	if len(input.Description) > 500 {
		http.Error(w, "Description must be <= 500 characters", http.StatusBadRequest)
		return input, false
	}
	scope := webhookcore.ScopeForSiteID(siteID != nil)
	if err := webhookcore.ValidateEventSelection(scope, input.Events); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return input, false
	}
	allowDevelopment := h.ctx.Config != nil && h.ctx.Config.WebhookAllowDevelopmentTargets
	destination, err := webhookcore.ValidateDestination(r.Context(), input.URL, allowDevelopment, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return input, false
	}
	input.URL = destination.String()
	return input, true
}

func (h *handler) appendAudit(r *http.Request, siteID *uuid.UUID, action string, webhook *api.Webhook) error {
	if webhook == nil {
		return nil
	}
	teamID := uuid.Nil
	if siteID != nil {
		var err error
		teamID, err = h.ctx.Store.GetSiteTenantID(r.Context(), *siteID)
		if err != nil {
			return fmt.Errorf("resolve webhook team for audit: %w", err)
		}
	}
	return h.ctx.AppendAuditEventChecked(r.Context(), r, shared.AuditEvent{
		ActorID:     shared.GetUserIDFromContext(r),
		TeamID:      teamID,
		Action:      action,
		TargetType:  "webhook",
		TargetID:    webhook.ID.String(),
		TargetLabel: webhook.Name,
		Outcome:     "success",
		Details:     fmt.Sprintf("Webhook %q %s (scope=%s, webhook_id=%s)", webhook.Name, strings.TrimPrefix(action, "webhook."), webhook.Scope, webhook.ID),
	})
}

func parseWebhookID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	webhookID, err := uuid.Parse(strings.TrimSpace(r.PathValue("webhookID")))
	if err != nil {
		http.Error(w, "Invalid webhook ID", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return webhookID, true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("Failed to encode webhook response", "error", err)
	}
}

func nullableSiteLogValue(siteID *uuid.UUID) any {
	if siteID == nil {
		return nil
	}
	return *siteID
}
