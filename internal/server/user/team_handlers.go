package user

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
	"hitkeep/internal/entitlements"
	"hitkeep/internal/mailables"
	"hitkeep/internal/server/shared"
)

func canManageTeam(role string) bool {
	switch strings.TrimSpace(strings.ToLower(role)) {
	case database.TenantRoleOwner, database.TenantRoleAdmin:
		return true
	default:
		return false
	}
}

func (h *handler) appendTeamAudit(r *http.Request, teamID, actorID uuid.UUID, action, details string, targetUserID *uuid.UUID) {
	targetType := "team"
	targetID := teamID.String()
	targetLabel := ""
	resolvedTargetUserID := uuid.Nil
	if team, err := h.ctx.Store.GetTenant(r.Context(), teamID); err == nil && team != nil {
		targetLabel = team.Name
	}
	if targetUserID != nil && *targetUserID != uuid.Nil {
		resolvedTargetUserID = *targetUserID
		targetID = (*targetUserID).String()
		if user, err := h.ctx.Store.GetUserByID(r.Context(), *targetUserID); err == nil && user != nil {
			targetLabel = user.Email
		}
	}
	if strings.HasPrefix(action, "member.") || strings.HasPrefix(action, "ownership.") {
		targetType = "user"
	}
	if strings.HasPrefix(action, "api_client.") {
		targetType = "api_client"
	}

	h.ctx.AppendAuditEvent(r.Context(), r, shared.AuditEvent{
		ActorID:      actorID,
		TeamID:       teamID,
		TargetUserID: resolvedTargetUserID,
		Action:       action,
		TargetType:   targetType,
		TargetID:     targetID,
		TargetLabel:  targetLabel,
		Outcome:      "success",
		Details:      details,
	})
}

func writeTeamActionError(w http.ResponseWriter, statusCode int, code string, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status":  "error",
		"code":    code,
		"message": message,
	}); err != nil {
		slog.Error("Failed to encode team action error", "error", err, "code", code)
	}
}

func resolveTeamEntitlements(ctx context.Context, store *database.Store, provider entitlements.Provider, teamID uuid.UUID) *api.TeamEntitlements {
	if override := resolveCloudBillingTeamEntitlements(ctx, store, teamID); override != nil {
		return override
	}

	defaults := &entitlements.Entitlements{
		AllowSSO:            true,
		AllowCustomBranding: true,
	}

	ent := defaults
	if provider != nil {
		if resolved, err := provider.ForTenant(ctx, teamID); err == nil && resolved != nil {
			ent = resolved
		}
	}

	return &api.TeamEntitlements{
		MaxSitesPerTeam:     ent.MaxSitesPerTeam,
		MaxTeamMembers:      ent.MaxTeamMembers,
		MaxRetentionDays:    ent.MaxRetentionDays,
		AllowSSO:            ent.AllowSSO,
		AllowCustomBranding: ent.AllowCustomBranding,
	}
}

func resolveTeamPlan(ctx context.Context, store *database.Store, provider entitlements.Provider, teamID uuid.UUID) *api.TeamPlan {
	if override := resolveCloudBillingTeamPlan(ctx, store, teamID); override != nil {
		return override
	}

	describer, ok := provider.(entitlements.Describer)
	if !ok || describer == nil {
		return nil
	}

	plan, err := describer.DescribeTenant(ctx, teamID)
	if err != nil || plan == nil {
		return nil
	}
	if strings.TrimSpace(plan.Name) == "" && strings.TrimSpace(plan.Code) == "" {
		return nil
	}

	return &api.TeamPlan{
		Code:       strings.TrimSpace(plan.Code),
		Name:       strings.TrimSpace(plan.Name),
		UpgradeURL: strings.TrimSpace(plan.UpgradeURL),
		SupportURL: strings.TrimSpace(plan.SupportURL),
	}
}

func (h *handler) hydrateTeamSummaries(r *http.Request, teams []api.Team) []api.Team {
	if len(teams) == 0 {
		return teams
	}

	enriched := make([]api.Team, len(teams))
	copy(enriched, teams)
	for idx, team := range enriched {
		enriched[idx].Entitlements = resolveTeamEntitlements(r.Context(), h.ctx.Store, h.ctx.Entitlements, team.ID)
		enriched[idx].Plan = resolveTeamPlan(r.Context(), h.ctx.Store, h.ctx.Entitlements, team.ID)

		analyticsStore := h.ctx.Store
		if h.ctx.TenantStores != nil {
			store, err := h.ctx.TenantStores.ForTenant(r.Context(), team.ID)
			if err != nil {
				slog.Warn("Failed to resolve analytics store for team usage", "error", err, "team_id", team.ID)
				continue
			}
			analyticsStore = store
		}

		usage, err := h.ctx.Store.BuildTeamUsageSummary(r.Context(), team.ID, analyticsStore)
		if err != nil {
			slog.Warn("Failed to build team usage summary", "error", err, "team_id", team.ID)
			continue
		}
		enriched[idx].Usage = usage
	}

	return enriched
}

func (h *handler) sendTeamInviteEmail(r *http.Request, teamID, actorID uuid.UUID, invite *api.TeamInvite) {
	if h.ctx.Mailer == nil || invite == nil {
		return
	}

	inviteToken, err := h.ctx.Store.CreatePasswordResetToken(r.Context(), invite.Email)
	if err != nil {
		slog.Warn("Failed to create invite token for team invite", "error", err, "email", invite.Email, "team_id", teamID)
		return
	}

	teamName := "HitKeep Team"
	if team, err := h.ctx.Store.GetTenant(r.Context(), teamID); err == nil && team != nil {
		teamName = team.Name
	}
	inviterName := "Someone"
	if inviter, err := h.ctx.Store.GetUserByID(r.Context(), actorID); err == nil && inviter != nil {
		inviterName = inviter.Email
	}
	locale := "en"
	if recipient, err := h.ctx.Store.GetUserByEmail(r.Context(), invite.Email); err == nil && recipient != nil {
		if resolvedLocale, err := h.ctx.Store.GetUserLocale(r.Context(), recipient.ID); err == nil {
			locale = resolvedLocale
		}
	} else if actorID != uuid.Nil {
		if resolvedLocale, err := h.ctx.Store.GetUserLocale(r.Context(), actorID); err == nil && strings.TrimSpace(resolvedLocale) != "" {
			locale = resolvedLocale
		}
	}

	inviteLink := strings.TrimRight(h.ctx.Config.PublicURL, "/") + "/accept-invite?token=" + inviteToken
	if err := h.ctx.Mailer.Send(invite.Email, mailables.NewTeamInvite(inviteLink, teamName, inviterName, invite.Role, true, locale)); err != nil {
		slog.Warn("Failed to send team invite email", "error", err, "email", invite.Email, "team_id", teamID)
	}
}

func (h *handler) handleCreateTeam() http.HandlerFunc {
	type request struct {
		Name    string `json:"name"`
		LogoURL string `json:"logo_url"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		actorID := shared.GetUserIDFromContext(r)
		if actorID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		var req request
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
			return
		}

		name := strings.TrimSpace(req.Name)
		if name == "" {
			http.Error(w, "Team name is required", http.StatusBadRequest)
			return
		}
		if len(name) > 120 {
			http.Error(w, "Team name must be 120 characters or fewer", http.StatusBadRequest)
			return
		}

		logoURL := strings.TrimSpace(req.LogoURL)
		if logoURL != "" {
			if len(logoURL) > 2048 {
				http.Error(w, "Logo URL must be 2048 characters or fewer", http.StatusBadRequest)
				return
			}
			if _, err := url.ParseRequestURI(logoURL); err != nil {
				http.Error(w, "Invalid logo URL", http.StatusBadRequest)
				return
			}
		}

		if h.ctx.Config.CloudHosted {
			http.Error(w, "Managed cloud accounts are limited to one team", http.StatusForbidden)
			return
		}

		if h.ctx.Entitlements != nil {
			activeTenantID, entErr := h.ctx.Store.GetActiveTenantID(r.Context(), actorID)
			if entErr == nil {
				ent, entErr := h.ctx.Entitlements.ForTenant(r.Context(), activeTenantID)
				if entErr == nil && ent.MaxTeams > 0 {
					teams, _, _ := h.ctx.Store.ListUserTeams(r.Context(), actorID)
					if len(teams) >= ent.MaxTeams {
						http.Error(w, "Team limit reached", http.StatusForbidden)
						return
					}
				}
			}
		}

		team, err := h.ctx.Store.CreateTenant(r.Context(), actorID, name, logoURL)
		if err != nil {
			slog.Error("Failed to create team", "error", err, "actor_id", actorID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if err := h.ctx.Store.SetActiveTenantID(r.Context(), actorID, team.ID); err != nil {
			slog.Warn("Failed to auto-activate new team", "error", err, "team_id", team.ID, "actor_id", actorID)
		}
		h.appendTeamAudit(r, team.ID, actorID, "team.created", fmt.Sprintf("Team %q created", team.Name), nil)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(map[string]any{"team": team}); err != nil {
			slog.Error("Failed to encode create team response", "error", err, "actor_id", actorID)
		}
	}
}

func (h *handler) handleGetTeams() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		teams, activeTeamID, err := h.ctx.Store.ListUserTeams(r.Context(), userID)
		if err != nil {
			slog.Error("Failed to list teams", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		resp := struct {
			ActiveTeamID  uuid.UUID   `json:"active_team_id"`
			RecentTeamIDs []uuid.UUID `json:"recent_team_ids"`
			Teams         []api.Team  `json:"teams"`
		}{
			ActiveTeamID:  activeTeamID,
			RecentTeamIDs: orderedRecentTeamIDs(teams, activeTeamID),
			Teams:         h.hydrateTeamSummaries(r, teams),
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("Failed to encode teams response", "error", err, "user_id", userID)
		}
	}
}

func (h *handler) handleSetActiveTeam() http.HandlerFunc {
	type request struct {
		TeamID string `json:"team_id"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		var req request
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
			return
		}

		teamID, err := uuid.Parse(strings.TrimSpace(req.TeamID))
		if err != nil {
			http.Error(w, "Invalid team ID", http.StatusBadRequest)
			return
		}

		if err := h.ctx.Store.SetActiveTenantID(r.Context(), userID, teamID); err != nil {
			if errors.Is(err, database.ErrTenantMembershipRequired) {
				http.Error(w, "Access denied", http.StatusForbidden)
				return
			}
			slog.Error("Failed to set active team", "error", err, "user_id", userID, "team_id", teamID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		h.appendTeamAudit(r, teamID, userID, "team.active_changed", fmt.Sprintf("Active team changed to %s", teamID), nil)

		teams, activeTeamID, teamsErr := h.ctx.Store.ListUserTeams(r.Context(), userID)
		if teamsErr != nil {
			slog.Warn("Failed to load active team after active team update", "error", teamsErr, "user_id", userID, "team_id", teamID)
			teams = nil
			activeTeamID = teamID
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"status":          "ok",
			"active_team_id":  activeTeamID,
			"recent_team_ids": orderedRecentTeamIDs(teams, activeTeamID),
		}); err != nil {
			slog.Error("Failed to encode active team response", "error", err, "user_id", userID)
		}
	}
}

func (h *handler) handleGetTeamMembers() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		teamID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
		if err != nil {
			http.Error(w, "Invalid team ID", http.StatusBadRequest)
			return
		}

		if _, err := h.ctx.Store.GetTenantRole(r.Context(), teamID, userID); err != nil {
			http.Error(w, "Access denied", http.StatusForbidden)
			return
		}

		members, err := h.ctx.Store.ListTeamMembers(r.Context(), teamID)
		if err != nil {
			slog.Error("Failed to list team members", "error", err, "user_id", userID, "team_id", teamID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(members); err != nil {
			slog.Error("Failed to encode team members response", "error", err, "user_id", userID, "team_id", teamID)
		}
	}
}

func (h *handler) handleGetTeamInvites() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		teamID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
		if err != nil {
			http.Error(w, "Invalid team ID", http.StatusBadRequest)
			return
		}

		role, err := h.ctx.Store.GetTenantRole(r.Context(), teamID, userID)
		if err != nil || !canManageTeam(role) {
			http.Error(w, "Access denied", http.StatusForbidden)
			return
		}

		invites, err := h.ctx.Store.ListTeamInvites(r.Context(), teamID)
		if err != nil {
			slog.Error("Failed to list team invites", "error", err, "user_id", userID, "team_id", teamID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(invites); err != nil {
			slog.Error("Failed to encode team invites response", "error", err, "user_id", userID, "team_id", teamID)
		}
	}
}

func (h *handler) handleGetTeamAudit() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		teamID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
		if err != nil {
			http.Error(w, "Invalid team ID", http.StatusBadRequest)
			return
		}

		role, err := h.ctx.Store.GetTenantRole(r.Context(), teamID, userID)
		if err != nil || !canManageTeam(role) {
			http.Error(w, "Access denied", http.StatusForbidden)
			return
		}

		filter, err := parseTeamAuditFilter(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		entries, total, err := h.ctx.Store.ListTeamAuditEntriesFiltered(r.Context(), teamID, filter)
		if err != nil {
			slog.Error("Failed to list team audit entries", "error", err, "user_id", userID, "team_id", teamID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(api.TeamAuditListResponse{
			Entries: entries,
			Total:   total,
			Limit:   normalizedTeamAuditLimit(filter.Limit),
			Offset:  filter.Offset,
			HasMore: filter.Offset+len(entries) < total,
			Action:  filter.Action,
		}); err != nil {
			slog.Error("Failed to encode team audit response", "error", err, "user_id", userID, "team_id", teamID)
		}
	}
}

func parseTeamAuditFilter(r *http.Request) (database.TeamAuditFilter, error) {
	q := r.URL.Query()
	filter := database.TeamAuditFilter{
		Action:     strings.TrimSpace(q.Get("action")),
		TargetType: strings.TrimSpace(q.Get("target_type")),
		Outcome:    strings.TrimSpace(q.Get("outcome")),
		Query:      strings.TrimSpace(q.Get("query")),
		Limit:      database.DefaultTeamAuditListLimit,
	}
	if rawLimit := strings.TrimSpace(q.Get("limit")); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil || limit < 0 {
			return filter, fmt.Errorf("invalid limit")
		}
		filter.Limit = normalizedTeamAuditLimit(limit)
	}
	if rawOffset := strings.TrimSpace(q.Get("offset")); rawOffset != "" {
		offset, err := strconv.Atoi(rawOffset)
		if err != nil || offset < 0 {
			return filter, fmt.Errorf("invalid offset")
		}
		filter.Offset = offset
	}
	if fromStr := strings.TrimSpace(q.Get("from")); fromStr != "" {
		from, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			return filter, fmt.Errorf("invalid from date, expected RFC3339")
		}
		filter.From = from
	}
	if toStr := strings.TrimSpace(q.Get("to")); toStr != "" {
		to, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			return filter, fmt.Errorf("invalid to date, expected RFC3339")
		}
		filter.To = to
	}
	return filter, nil
}

func normalizedTeamAuditLimit(limit int) int {
	if limit <= 0 || limit > database.MaxTeamAuditListLimit {
		return database.DefaultTeamAuditListLimit
	}
	return limit
}
