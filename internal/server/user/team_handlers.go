package user

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/appurl"
	"hitkeep/internal/api"
	"hitkeep/internal/database"
	"hitkeep/internal/entitlements"
	json "hitkeep/internal/jsonapi"
	"hitkeep/internal/mailables"
	"hitkeep/internal/mailer"
	"hitkeep/internal/server/shared"
	"hitkeep/internal/webhooks"
)

const (
	operatorCloudPlanCode = "operator"
	operatorCloudPlanName = "Operator"
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

	eventType := ""
	switch action {
	case "team.created":
		eventType = webhooks.EventTeamCreated
	case "team.updated":
		eventType = webhooks.EventTeamUpdated
	case "team.archived":
		eventType = webhooks.EventTeamArchived
	case "member.removed", "member.left":
		eventType = webhooks.EventTeamMemberRemoved
	}
	if eventType != "" {
		data := map[string]any{"team_id": teamID.String()}
		if resolvedTargetUserID != uuid.Nil {
			data["user_id"] = resolvedTargetUserID.String()
		}
		h.ctx.EmitWebhookEvent(r.Context(), webhooks.Event{Type: eventType, Data: data})
	}
}

func writeTeamActionError(ctx context.Context, w http.ResponseWriter, statusCode int, code string, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.MarshalWrite(w, map[string]string{
		"status":  "error",
		"code":    code,
		"message": message,
	}); err != nil {
		shared.LoggerFromContext(ctx).Error("Failed to encode team action error", "error", err, "code", code)
	}
}

func operatorTeamEntitlements() *api.TeamEntitlements {
	return &api.TeamEntitlements{
		AllowSSO:                      true,
		AllowCustomBranding:           true,
		AllowExternalReportRecipients: true,
	}
}

func operatorTeamPlan() *api.TeamPlan {
	return &api.TeamPlan{
		Code: operatorCloudPlanCode,
		Name: operatorCloudPlanName,
	}
}

func teamEntitlementsResponse(ent *entitlements.Entitlements) *api.TeamEntitlements {
	if ent == nil {
		return nil
	}
	return &api.TeamEntitlements{
		MaxSitesPerTeam:               ent.MaxSitesPerTeam,
		MaxTeamMembers:                ent.MaxTeamMembers,
		MaxRetentionDays:              ent.MaxRetentionDays,
		AllowSSO:                      ent.AllowSSO,
		AllowCustomBranding:           ent.AllowCustomBranding,
		AllowExternalReportRecipients: ent.AllowExternalReportRecipients,
	}
}

func teamPlanResponse(plan *entitlements.PlanInfo) *api.TeamPlan {
	if plan == nil {
		return nil
	}
	return &api.TeamPlan{
		Code:       plan.Code,
		Name:       plan.Name,
		UpgradeURL: plan.UpgradeURL,
		SupportURL: plan.SupportURL,
	}
}

func (h *handler) hydrateTeamSummaries(r *http.Request, teams []api.Team) []api.Team {
	if len(teams) == 0 {
		return teams
	}

	enriched := make([]api.Team, len(teams))
	copy(enriched, teams)
	actorID := shared.GetUserIDFromContext(r)
	limits := h.ctx.Limits()
	isOperator := limits.BypassesCloudLimits(r.Context(), actorID)
	for idx, team := range enriched {
		if isOperator && team.Role == database.TenantRoleOwner {
			enriched[idx].Entitlements = operatorTeamEntitlements()
			enriched[idx].Plan = operatorTeamPlan()
		} else {
			enriched[idx].Entitlements = teamEntitlementsResponse(limits.TeamEntitlements(r.Context(), team.ID))
			enriched[idx].Plan = teamPlanResponse(limits.TeamPlan(r.Context(), team.ID))
		}

		analyticsStore := h.ctx.Store
		if h.ctx.TenantStores != nil {
			store, err := h.ctx.TenantStores.ForTenant(r.Context(), team.ID)
			if err != nil {
				shared.LoggerFromContext(r.Context()).Warn("Failed to resolve analytics store for team usage", "error", err, "team_id", team.ID)
				continue
			}
			analyticsStore = store
		}

		usage, err := h.ctx.Store.BuildTeamUsageSummary(r.Context(), team.ID, analyticsStore)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Warn("Failed to build team usage summary", "error", err, "team_id", team.ID)
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
		shared.LoggerFromContext(r.Context()).Warn("Failed to create invite token for team invite", "error", err, "team_id", teamID)
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

	acceptPath := "/accept-invite?token=" + inviteToken
	inviteLink := appurl.Path(h.ctx.Config.PublicURL, acceptPath)
	if !invite.RequiresPasswordSetup {
		inviteLink = appurl.Path(h.ctx.Config.PublicURL, "/login?returnUrl="+url.QueryEscape(acceptPath))
	}
	if err := h.ctx.Mailer.Send(invite.Email, mailables.NewTeamInvite(inviteLink, teamName, inviterName, invite.Role, invite.RequiresPasswordSetup, locale)); err != nil {
		details := mailer.DescribeError(err)
		shared.LoggerFromContext(r.Context()).Warn("Failed to send team invite email", "error_code", "smtp_send_failed", "error_stage", details.Stage, "error_kind", details.Kind, "error_message", details.Message, "smtp_code", details.SMTPCode, "team_id", teamID)
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
		if err := json.UnmarshalReadStrict(http.MaxBytesReader(w, r.Body, 1<<20), &req); err != nil {
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

		if err := h.ctx.Limits().CanCreateTeam(r.Context(), actorID); err != nil {
			if errors.Is(err, entitlements.ErrTeamLimitReached) {
				http.Error(w, "Team limit reached", http.StatusForbidden)
			} else {
				shared.LoggerFromContext(r.Context()).Error("Failed to check team creation limit", "error", err, "actor_id", actorID)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}

		team, err := h.ctx.Store.CreateTenant(r.Context(), actorID, name, logoURL)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to create team", "error", err, "actor_id", actorID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if err := h.ctx.Store.SetActiveTenantID(r.Context(), actorID, team.ID); err != nil {
			shared.LoggerFromContext(r.Context()).Warn("Failed to auto-activate new team", "error", err, "team_id", team.ID, "actor_id", actorID)
		}
		h.appendTeamAudit(r, team.ID, actorID, "team.created", fmt.Sprintf("Team %q created", team.Name), nil)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.MarshalWrite(w, map[string]any{"team": team}); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode create team response", "error", err, "actor_id", actorID)
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

		resp, err := h.userTeamsResponse(r, userID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to list teams", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, resp); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode teams response", "error", err, "user_id", userID)
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
		if err := json.UnmarshalReadStrict(http.MaxBytesReader(w, r.Body, 1<<20), &req); err != nil {
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
			shared.LoggerFromContext(r.Context()).Error("Failed to set active team", "error", err, "user_id", userID, "team_id", teamID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		h.appendTeamAudit(r, teamID, userID, "team.active_changed", fmt.Sprintf("Active team changed to %s", teamID), nil)

		teams, activeTeamID, teamsErr := h.ctx.Store.ListUserTeams(r.Context(), userID)
		if teamsErr != nil {
			shared.LoggerFromContext(r.Context()).Warn("Failed to load active team after active team update", "error", teamsErr, "user_id", userID, "team_id", teamID)
			teams = nil
			activeTeamID = teamID
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, map[string]any{
			"status":          "ok",
			"active_team_id":  activeTeamID,
			"recent_team_ids": orderedRecentTeamIDs(teams, activeTeamID),
		}); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode active team response", "error", err, "user_id", userID)
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
			shared.LoggerFromContext(r.Context()).Error("Failed to list team members", "error", err, "user_id", userID, "team_id", teamID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, members); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode team members response", "error", err, "user_id", userID, "team_id", teamID)
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
			shared.LoggerFromContext(r.Context()).Error("Failed to list team invites", "error", err, "user_id", userID, "team_id", teamID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, invites); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode team invites response", "error", err, "user_id", userID, "team_id", teamID)
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
			shared.LoggerFromContext(r.Context()).Error("Failed to list team audit entries", "error", err, "user_id", userID, "team_id", teamID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, api.TeamAuditListResponse{
			Entries: entries,
			Total:   total,
			Limit:   normalizedTeamAuditLimit(filter.Limit),
			Offset:  filter.Offset,
			HasMore: filter.Offset+len(entries) < total,
			Action:  filter.Action,
		}); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode team audit response", "error", err, "user_id", userID, "team_id", teamID)
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
