package sites

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"hitkeep/internal/database"
	"hitkeep/internal/server/shared"
	json "hitkeep/jsonapi"
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

		effective, err := parseEffectiveExclusionsQuery(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var rules any
		if effective {
			teamID, resolveErr := h.ctx.Store.GetSiteTenantID(r.Context(), siteID)
			if resolveErr != nil {
				shared.LoggerFromContext(r.Context()).Error("Failed to resolve site team for exclusions", "error", resolveErr, "site_id", siteID)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			rules, err = h.ctx.Store.ListEffectiveSiteExclusions(r.Context(), teamID, siteID)
		} else {
			rules, err = h.ctx.Store.ListSiteExclusions(r.Context(), siteID)
		}
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to list site exclusions", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, rules); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode site exclusions response", "error", err, "site_id", siteID)
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
		createdRule, createErr := h.ctx.Store.CreateSiteTrafficExclusion(r.Context(), siteID, database.TrafficExclusionValues{
			Type:        input.Type,
			CIDR:        input.CIDR,
			CountryCode: input.CountryCode,
			UserAgent:   input.UserAgent,
			Path:        input.Path,
			Description: input.Description,
		}, userID)
		if createErr != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to create site exclusion", "error", createErr, "site_id", siteID, "type", input.Type)
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
				TargetID:    createdRule.ID.String(),
				TargetLabel: input.Label,
				Outcome:     "success",
				Details:     fmt.Sprintf("Traffic exclusion created (scope=site, type=%s, value=%s)", input.Type, input.Label),
			})
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.MarshalWrite(w, createdRule); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode site exclusion response", "error", err, "site_id", siteID)
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
			shared.LoggerFromContext(r.Context()).Error("Failed to delete site exclusion", "error", err, "site_id", siteID, "rule_id", ruleID)
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
				Details:     fmt.Sprintf("Traffic exclusion deleted (scope=site, rule_id=%s)", ruleID),
			})
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func parseEffectiveExclusionsQuery(r *http.Request) (bool, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("effective"))
	if raw == "" {
		return false, nil
	}
	effective, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("invalid effective value")
	}
	return effective, nil
}

func (h *handler) refreshIPFilter(ctx context.Context) {
	if h.ctx.IPFilter == nil {
		return
	}
	if err := h.ctx.IPFilter.Refresh(ctx); err != nil {
		shared.LoggerFromContext(ctx).Warn("Failed to refresh IP filter after exclusion write", "error", err)
	}
}
