package user

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"hitkeep/internal/database"
	json "hitkeep/internal/jsonapi"
	"hitkeep/internal/server/shared"
)

func (h *handler) handleListTeamExclusions() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		teamID, ok := teamExclusionID(w, r)
		if !ok {
			return
		}
		effective, err := teamEffectiveExclusionsQuery(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var rules any
		if effective {
			rules, err = h.ctx.Store.ListEffectiveTeamExclusions(r.Context(), teamID)
		} else {
			rules, err = h.ctx.Store.ListTeamExclusions(r.Context(), teamID)
		}
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to list team exclusions", "error", err, "team_id", teamID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, rules); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode team exclusions response", "error", err, "team_id", teamID)
		}
	}
}

func (h *handler) handleCreateTeamExclusion() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		teamID, ok := teamExclusionID(w, r)
		if !ok {
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
		rule, err := h.ctx.Store.CreateTeamTrafficExclusion(r.Context(), teamID, database.TrafficExclusionValues{
			Type:        input.Type,
			CIDR:        input.CIDR,
			CountryCode: input.CountryCode,
			UserAgent:   input.UserAgent,
			Path:        input.Path,
			Description: input.Description,
		}, userID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to create team exclusion", "error", err, "team_id", teamID, "type", input.Type)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		h.refreshTeamTrafficExclusions(r)
		h.ctx.AppendAuditEvent(r.Context(), r, shared.AuditEvent{
			ActorID:     userID,
			TeamID:      teamID,
			Action:      "site.exclusion_created",
			TargetType:  "site_exclusion",
			TargetID:    rule.ID.String(),
			TargetLabel: input.Label,
			Outcome:     "success",
			Details:     fmt.Sprintf("Traffic exclusion created (scope=team, type=%s, value=%s)", input.Type, input.Label),
		})

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.MarshalWrite(w, rule); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode team exclusion response", "error", err, "team_id", teamID)
		}
	}
}

func (h *handler) handleDeleteTeamExclusion() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		teamID, ok := teamExclusionID(w, r)
		if !ok {
			return
		}
		ruleID, err := uuid.Parse(strings.TrimSpace(r.PathValue("ruleID")))
		if err != nil {
			http.Error(w, "Invalid rule_id", http.StatusBadRequest)
			return
		}

		deleted, err := h.ctx.Store.DeleteTeamExclusion(r.Context(), teamID, ruleID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to delete team exclusion", "error", err, "team_id", teamID, "rule_id", ruleID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if !deleted {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		h.refreshTeamTrafficExclusions(r)
		h.ctx.AppendAuditEvent(r.Context(), r, shared.AuditEvent{
			ActorID:     shared.GetUserIDFromContext(r),
			TeamID:      teamID,
			Action:      "site.exclusion_deleted",
			TargetType:  "site_exclusion",
			TargetID:    ruleID.String(),
			TargetLabel: ruleID.String(),
			Outcome:     "success",
			Details:     fmt.Sprintf("Traffic exclusion deleted (scope=team, rule_id=%s)", ruleID),
		})
		w.WriteHeader(http.StatusNoContent)
	}
}

func teamExclusionID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	teamID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		http.Error(w, "Invalid team_id", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return teamID, true
}

func teamEffectiveExclusionsQuery(r *http.Request) (bool, error) {
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

func (h *handler) refreshTeamTrafficExclusions(r *http.Request) {
	if h.ctx.IPFilter == nil {
		return
	}
	if err := h.ctx.IPFilter.Refresh(r.Context()); err != nil {
		shared.LoggerFromContext(r.Context()).Warn("Failed to refresh traffic exclusions after team write", "error", err)
	}
}
