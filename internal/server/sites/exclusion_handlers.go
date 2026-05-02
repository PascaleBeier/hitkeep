package sites

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"hitkeep/internal/blocking"
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
	type request struct {
		CIDR        string `json:"cidr"`
		Description string `json:"description"`
	}

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

		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		normalizedCIDR, _, err := blocking.NormalizeCIDR(req.CIDR)
		if err != nil {
			http.Error(w, "Invalid IP or CIDR", http.StatusBadRequest)
			return
		}

		description := strings.TrimSpace(req.Description)
		if len(description) > 255 {
			http.Error(w, "Description must be 255 characters or fewer", http.StatusBadRequest)
			return
		}

		rule, err := h.ctx.Store.CreateSiteExclusion(r.Context(), siteID, normalizedCIDR, description, userID)
		if err != nil {
			slog.Error("Failed to create site exclusion", "error", err, "site_id", siteID, "cidr", normalizedCIDR)
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
				TargetID:    rule.ID.String(),
				TargetLabel: normalizedCIDR,
				Outcome:     "success",
				Details:     fmt.Sprintf("Site exclusion %s created", normalizedCIDR),
			})
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(rule); err != nil {
			slog.Error("Failed to encode site exclusion response", "error", err, "site_id", siteID)
		}
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
