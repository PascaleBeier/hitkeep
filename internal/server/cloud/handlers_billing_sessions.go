//go:build billing

package cloud

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"hitkeep/internal/config"
	"hitkeep/internal/database"
	"hitkeep/internal/server/shared"
)

func (h *handler) handleStripeWebhook() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.ctx.Config.CloudHosted {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		if strings.TrimSpace(h.ctx.Config.StripeWebhookSecret) == "" {
			http.Error(w, "Webhook signing secret not configured", http.StatusNotImplemented)
			return
		}

		payload, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Unable to read request body", http.StatusBadRequest)
			return
		}

		event, err := h.webhooks.ConstructEvent(payload, r.Header.Get("Stripe-Signature"), h.ctx.Config.StripeWebhookSecret)
		if err != nil {
			http.Error(w, "Invalid webhook signature", http.StatusBadRequest)
			return
		}

		if err := h.handleStripeEvent(r.Context(), event); err != nil {
			slog.Error("Failed to process Stripe webhook", "error", err, "type", event.Type, "event_id", event.ID)
			http.Error(w, "Webhook processing failed", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

func (h *handler) handleCreateBillingPortalSession() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.ctx.Config.CloudHosted {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		activeTenantID, err := h.ctx.Store.GetActiveTenantID(r.Context(), userID)
		if err != nil {
			http.Error(w, "Unable to resolve active team", http.StatusBadRequest)
			return
		}

		account, err := h.ctx.Store.GetCloudBillingAccount(r.Context(), activeTenantID)
		if errors.Is(err, database.ErrCloudBillingAccountNotFound) || account == nil {
			newAccount := database.CloudBillingAccount{
				TenantID:           activeTenantID,
				PlanCode:           database.CloudPlanFree,
				PlanName:           planNameForCode(database.CloudPlanFree),
				SubscriptionStatus: database.CloudSubscriptionStatusFree,
			}
			if upsertErr := h.ctx.Store.UpsertCloudBillingAccount(r.Context(), newAccount); upsertErr != nil {
				slog.Error("Failed to auto-create cloud billing account", "error", upsertErr, "team_id", activeTenantID)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			account = &newAccount
		}
		if err != nil && !errors.Is(err, database.ErrCloudBillingAccountNotFound) {
			slog.Error("Failed to load cloud billing account", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if strings.TrimSpace(account.StripeCustomerID) == "" {
			http.Error(w, "Stripe customer is not configured", http.StatusConflict)
			return
		}
		if strings.TrimSpace(h.ctx.Config.StripeSecretKey) == "" {
			http.Error(w, "Stripe secret key is not configured", http.StatusNotImplemented)
			return
		}

		var req billingPortalSessionRequest
		if r.Body != nil {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
				http.Error(w, "Invalid request body", http.StatusBadRequest)
				return
			}
		}

		session, err := h.stripe.CreatePortalSession(r.Context(), createPortalSessionInput{
			CustomerID:      account.StripeCustomerID,
			ConfigurationID: strings.TrimSpace(h.ctx.Config.StripePortalConfigurationID),
			ReturnURL:       billingPortalReturnURL(h.ctx.Config),
			Locale:          normalizeStripeLocale(req.Locale),
		})
		if err != nil {
			slog.Error("Failed to create Stripe billing portal session", "error", err)
			http.Error(w, "Unable to start billing portal", http.StatusBadGateway)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(billingPortalSessionResponse{URL: session.URL}); err != nil {
			slog.Error("Failed to encode billing portal session response", "error", err)
		}
	}
}

func (h *handler) handleCreateBillingCheckoutSession() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.ctx.Config.CloudHosted {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		activeTenantID, err := h.ctx.Store.GetActiveTenantID(r.Context(), userID)
		if err != nil {
			http.Error(w, "Unable to resolve active team", http.StatusBadRequest)
			return
		}

		account, err := h.ctx.Store.GetCloudBillingAccount(r.Context(), activeTenantID)
		if errors.Is(err, database.ErrCloudBillingAccountNotFound) || account == nil {
			// Auto-create a free billing account for legacy users who signed up
			// before the billing account table existed.
			newAccount := database.CloudBillingAccount{
				TenantID:           activeTenantID,
				PlanCode:           database.CloudPlanFree,
				PlanName:           planNameForCode(database.CloudPlanFree),
				SubscriptionStatus: database.CloudSubscriptionStatusFree,
			}
			if upsertErr := h.ctx.Store.UpsertCloudBillingAccount(r.Context(), newAccount); upsertErr != nil {
				slog.Error("Failed to auto-create cloud billing account", "error", upsertErr, "team_id", activeTenantID)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			account = &newAccount
		}
		if err != nil && !errors.Is(err, database.ErrCloudBillingAccountNotFound) {
			slog.Error("Failed to load cloud billing account", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if strings.TrimSpace(h.ctx.Config.StripeSecretKey) == "" {
			http.Error(w, "Stripe secret key is not configured", http.StatusNotImplemented)
			return
		}

		var req billingCheckoutSessionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		req.PlanCode = normalizePlanCode(req.PlanCode)
		req.Locale = normalizeStripeLocale(req.Locale)
		if req.PlanCode != database.CloudPlanPro && req.PlanCode != database.CloudPlanBusiness {
			http.Error(w, "Paid plan code is required", http.StatusBadRequest)
			return
		}

		currentPlanCode, _ := effectivePlanCode(account)
		if currentPlanCode != database.CloudPlanFree {
			http.Error(w, "Use billing portal to manage an existing paid plan", http.StatusConflict)
			return
		}

		user, err := h.ctx.Store.GetUserByID(r.Context(), userID)
		if err != nil || user == nil {
			slog.Error("Failed to load user for billing checkout", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		team, err := h.ctx.Store.GetTenant(r.Context(), activeTenantID)
		if err != nil || team == nil {
			slog.Error("Failed to load team for billing checkout", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		customerID := strings.TrimSpace(account.StripeCustomerID)
		if customerID == "" {
			customerID, err = h.stripe.CreateCustomer(r.Context(), createCustomerInput{
				Email:          strings.TrimSpace(strings.ToLower(user.Email)),
				Name:           stripeCustomerName(user, team.Name),
				UserID:         user.ID,
				TenantID:       activeTenantID,
				PlanCode:       req.PlanCode,
				Jurisdiction:   strings.TrimSpace(strings.ToUpper(h.ctx.Config.CloudJurisdiction)),
				IdempotencyKey: stripeCustomerCreateIdempotencyKey(user.ID, activeTenantID, user.Email),
			})
			if err != nil {
				slog.Error("Failed to create Stripe customer for billing checkout", "error", err)
				http.Error(w, "Unable to start checkout", http.StatusBadGateway)
				return
			}
		}

		priceID := priceIDForPlan(h.ctx.Config, req.PlanCode)
		if priceID == "" {
			http.Error(w, "Plan is not configured for checkout", http.StatusBadRequest)
			return
		}

		session, err := h.stripe.CreateCheckoutSession(r.Context(), createCheckoutSessionInput{
			CustomerID:   customerID,
			PriceID:      priceID,
			SuccessURL:   checkoutSuccessURL(h.ctx.Config),
			CancelURL:    checkoutCancelURL(h.ctx.Config),
			Locale:       req.Locale,
			UserID:       user.ID,
			TenantID:     activeTenantID,
			PlanCode:     req.PlanCode,
			PlanName:     planNameForCode(req.PlanCode),
			Jurisdiction: strings.TrimSpace(strings.ToUpper(h.ctx.Config.CloudJurisdiction)),
			Email:        strings.TrimSpace(strings.ToLower(user.Email)),
		})
		if err != nil {
			slog.Error("Failed to create Stripe upgrade checkout session", "error", err)
			http.Error(w, "Unable to start checkout", http.StatusBadGateway)
			return
		}

		if err := h.ctx.Store.UpsertCloudBillingAccount(r.Context(), database.CloudBillingAccount{
			TenantID:             activeTenantID,
			PlanCode:             database.CloudPlanFree,
			PlanName:             planNameForCode(database.CloudPlanFree),
			SubscriptionStatus:   subscriptionStatusPending,
			StripeCustomerID:     customerID,
			StripeSubscriptionID: "",
			StripePriceID:        priceID,
		}); err != nil {
			slog.Error("Failed to persist Stripe upgrade checkout metadata", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(billingCheckoutSessionResponse{URL: session.URL}); err != nil {
			slog.Error("Failed to encode billing checkout session response", "error", err)
		}
	}
}

func billingPortalReturnURL(conf *config.Config) string {
	return strings.TrimRight(conf.PublicURL, "/") + "/admin/team"
}

func normalizeStripeLocale(raw string) string {
	trimmed := strings.TrimSpace(strings.ReplaceAll(raw, "_", "-"))
	if trimmed == "" {
		return "auto"
	}

	parts := strings.Split(trimmed, "-")
	normalized := make([]string, 0, len(parts))
	for index, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return "auto"
		}
		if index == 0 {
			normalized = append(normalized, strings.ToLower(part))
			continue
		}
		if len(part) == 2 {
			normalized = append(normalized, strings.ToUpper(part))
			continue
		}
		normalized = append(normalized, strings.ToLower(part))
	}
	tag := strings.Join(normalized, "-")

	if supported, ok := supportedStripeLocale(tag); ok {
		return supported
	}
	if base, _, found := strings.Cut(tag, "-"); found {
		if supported, ok := supportedStripeLocale(base); ok {
			return supported
		}
	}
	return "auto"
}
