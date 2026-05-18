package sites

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"hitkeep/internal/server/shared"
)

func (h *handler) handleListSiteExclusions() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		siteID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
		if err != nil {
			http.Error(w, "Invalid site_id", http.StatusBadRequest)
			return
		}

		rules, err := h.ctx.Store.ListSiteExclusions(r.Context(), siteID)
		if err != nil {
			slog.Error("Failed to list site exclusions", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(rules); err != nil {
			slog.Error("Failed to encode site exclusions response", "error", err, "site_id", siteID)
		}
	}
}

func (h *handler) handleCreateSiteExclusion() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		siteID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
		if err != nil {
			http.Error(w, "Invalid site_id", http.StatusBadRequest)
			return
		}

		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		input, message, status, ok := shared.DecodeTrafficExclusionRequest(r)
		if !ok {
			http.Error(w, message, status)
			return
		}
		ruleID, createdRule, createErr := h.createSiteTrafficExclusion(r.Context(), siteID, userID, input)
		if createErr != nil {
			slog.Error("Failed to create site exclusion", "error", createErr, "site_id", siteID, "type", input.Type, "label", input.Label)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		h.refreshIPFilter(r.Context())
		if teamID, err := h.ctx.Store.GetSiteTenantID(r.Context(), siteID); err == nil {
			h.ctx.AppendAuditEvent(r.Context(), r, shared.AuditEvent{
				ActorID:     userID,
				TeamID:      teamID,
				Action:      "site.exclusion_created",
				TargetType:  "site_exclusion",
				TargetID:    ruleID,
				TargetLabel: input.Label,
				Outcome:     "success",
				Details:     fmt.Sprintf("Site exclusion %s created", input.Label),
			})
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(createdRule); err != nil {
			slog.Error("Failed to encode site exclusion response", "error", err, "site_id", siteID)
		}
	}
}

func (h *handler) createSiteTrafficExclusion(ctx context.Context, siteID uuid.UUID, userID uuid.UUID, input shared.TrafficExclusionInput) (string, any, error) {
	switch input.Type {
	case shared.ExclusionRuleTypeCIDR:
		rule, err := h.ctx.Store.CreateSiteExclusion(ctx, siteID, input.CIDR, input.Description, userID)
		if err != nil {
			return "", nil, err
		}
		return rule.ID.String(), rule, nil
	case shared.ExclusionRuleTypeCountry:
		rule, err := h.ctx.Store.CreateSiteCountryExclusion(ctx, siteID, input.CountryCode, input.Description, userID)
		if err != nil {
			return "", nil, err
		}
		return rule.ID.String(), rule, nil
	default:
		return "", nil, fmt.Errorf("unsupported exclusion type %q", input.Type)
	}
}

func (h *handler) handleDeleteSiteExclusion() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		siteID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
		if err != nil {
			http.Error(w, "Invalid site_id", http.StatusBadRequest)
			return
		}

		ruleID, err := uuid.Parse(strings.TrimSpace(r.PathValue("ruleID")))
		if err != nil {
			http.Error(w, "Invalid rule_id", http.StatusBadRequest)
			return
		}

		deleted, err := h.ctx.Store.DeleteSiteExclusion(r.Context(), siteID, ruleID)
		if err != nil {
			slog.Error("Failed to delete site exclusion", "error", err, "site_id", siteID, "rule_id", ruleID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if !deleted {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		h.refreshIPFilter(r.Context())
		if teamID, err := h.ctx.Store.GetSiteTenantID(r.Context(), siteID); err == nil {
			h.ctx.AppendAuditEvent(r.Context(), r, shared.AuditEvent{
				ActorID:     shared.GetUserIDFromContext(r),
				TeamID:      teamID,
				Action:      "site.exclusion_deleted",
				TargetType:  "site_exclusion",
				TargetID:    ruleID.String(),
				TargetLabel: ruleID.String(),
				Outcome:     "success",
				Details:     fmt.Sprintf("Site exclusion %s deleted", ruleID),
			})
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *handler) refreshIPFilter(ctx context.Context) {
	if h.ctx.IPFilter == nil {
		return
	}
	if err := h.ctx.IPFilter.Refresh(ctx); err != nil {
		slog.Warn("Failed to refresh IP filter after exclusion write", "error", err)
	}
}
