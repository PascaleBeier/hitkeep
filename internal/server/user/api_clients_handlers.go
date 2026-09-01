package user

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	authcore "hitkeep/internal/auth"
	"hitkeep/internal/database"
	"hitkeep/internal/server/shared"
	json "hitkeep/jsonapi"
)

type apiClientSiteRoleInput struct {
	SiteID string `json:"site_id"`
	Role   string `json:"role"`
}

type createAPIClientRequest struct {
	Name         string                   `json:"name"`
	Description  string                   `json:"description"`
	InstanceRole string                   `json:"instance_role"`
	ExpiresAt    *time.Time               `json:"expires_at"`
	SiteRoles    []apiClientSiteRoleInput `json:"site_roles"`
}

type updateAPIClientRequest struct {
	Name         string                   `json:"name"`
	Description  string                   `json:"description"`
	InstanceRole string                   `json:"instance_role"`
	ExpiresAt    *time.Time               `json:"expires_at"`
	Revoked      *bool                    `json:"revoked"`
	SiteRoles    []apiClientSiteRoleInput `json:"site_roles"`
}

type apiClientTokenResponse struct {
	Client api.APIClient `json:"client"`
	Token  string        `json:"token"`
}

func (h *handler) handleListAPIClients() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		clients, err := h.ctx.Store.ListAPIClients(r.Context(), userID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to list api clients", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, clients); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode api clients response", "error", err, "user_id", userID)
		}
	}
}

func (h *handler) handleCreateAPIClient() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		var req createAPIClientRequest
		if err := decodeJSON(r, &req); err != nil {
			http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
			return
		}

		name := strings.TrimSpace(req.Name)
		if name == "" || len(name) > 120 {
			http.Error(w, "Name is required and must be <= 120 characters", http.StatusBadRequest)
			return
		}

		description := strings.TrimSpace(req.Description)
		if len(description) > 500 {
			http.Error(w, "Description must be <= 500 characters", http.StatusBadRequest)
			return
		}

		requestedInstanceRole := authcore.InstanceUser
		if strings.TrimSpace(req.InstanceRole) != "" {
			requestedInstanceRole = authcore.InstanceRole(strings.TrimSpace(req.InstanceRole))
		}
		if !authcore.IsValidInstanceRole(requestedInstanceRole) {
			http.Error(w, "Invalid instance role", http.StatusBadRequest)
			return
		}

		actorInstanceRole, err := h.ctx.Store.GetInstanceRole(r.Context(), userID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to get actor instance role", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if !authcore.CanAssignInstanceRole(actorInstanceRole, requestedInstanceRole) {
			http.Error(w, "Cannot delegate requested instance role", http.StatusForbidden)
			return
		}

		siteRoles, err := h.validateDelegatedSiteRoles(r, userID, actorInstanceRole, req.SiteRoles)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Warn("Invalid delegated site roles", "error", err, "user_id", userID)
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}

		client, token, err := h.ctx.Store.CreateAPIClient(
			r.Context(),
			userID,
			name,
			description,
			requestedInstanceRole,
			siteRoles,
			req.ExpiresAt,
		)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to create api client", "error", err, "user_id", userID)
			http.Error(w, "Failed to create api client", http.StatusInternalServerError)
			return
		}

		if err := h.appendAPIClientAudit(r, userID, uuid.Nil, "api_client.created", client); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to audit api client create", "error", err, "user_id", userID, "client_id", client.ID)
			http.Error(w, "Failed to audit api client action", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.MarshalWrite(w, apiClientTokenResponse{Client: *client, Token: token}); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode create api client response", "error", err, "user_id", userID)
		}
	}
}

func (h *handler) handleUpdateAPIClient() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		clientID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
		if err != nil {
			http.Error(w, "Invalid client ID", http.StatusBadRequest)
			return
		}

		existing, err := h.ctx.Store.GetAPIClient(r.Context(), userID, clientID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to load api client before update", "error", err, "user_id", userID, "client_id", clientID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if existing == nil {
			http.Error(w, "API client not found", http.StatusNotFound)
			return
		}

		var req updateAPIClientRequest
		if err := decodeJSON(r, &req); err != nil {
			http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
			return
		}

		name := strings.TrimSpace(req.Name)
		if name == "" || len(name) > 120 {
			http.Error(w, "Name is required and must be <= 120 characters", http.StatusBadRequest)
			return
		}

		description := strings.TrimSpace(req.Description)
		if len(description) > 500 {
			http.Error(w, "Description must be <= 500 characters", http.StatusBadRequest)
			return
		}

		requestedInstanceRole := authcore.InstanceRole(strings.TrimSpace(req.InstanceRole))
		if !authcore.IsValidInstanceRole(requestedInstanceRole) {
			http.Error(w, "Invalid instance role", http.StatusBadRequest)
			return
		}

		actorInstanceRole, err := h.ctx.Store.GetInstanceRole(r.Context(), userID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to get actor instance role", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if !authcore.CanAssignInstanceRole(actorInstanceRole, requestedInstanceRole) {
			http.Error(w, "Cannot delegate requested instance role", http.StatusForbidden)
			return
		}

		siteRoles, err := h.validateDelegatedSiteRoles(r, userID, actorInstanceRole, req.SiteRoles)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Warn("Invalid delegated site roles for update", "error", err, "user_id", userID, "client_id", clientID)
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}

		revoked := existing.RevokedAt != nil
		if req.Revoked != nil {
			revoked = *req.Revoked
		}

		updated, err := h.ctx.Store.UpdateAPIClient(
			r.Context(),
			userID,
			clientID,
			name,
			description,
			requestedInstanceRole,
			siteRoles,
			req.ExpiresAt,
			revoked,
		)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to update api client", "error", err, "user_id", userID, "client_id", clientID)
			http.Error(w, "Failed to update api client", http.StatusInternalServerError)
			return
		}
		if updated == nil {
			http.Error(w, "API client not found", http.StatusNotFound)
			return
		}

		if err := h.appendAPIClientAudit(r, userID, uuid.Nil, apiClientUpdateAuditAction(existing, updated), updated); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to audit api client update", "error", err, "user_id", userID, "client_id", clientID)
			http.Error(w, "Failed to audit api client action", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, updated); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode api client update response", "error", err, "user_id", userID, "client_id", clientID)
		}
	}
}

func (h *handler) handleRotateAPIClient() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		clientID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
		if err != nil {
			http.Error(w, "Invalid client ID", http.StatusBadRequest)
			return
		}

		client, token, err := h.ctx.Store.RotateAPIClient(r.Context(), userID, clientID)
		if err != nil {
			if errors.Is(err, database.ErrAPIClientNotFound) {
				http.Error(w, "API client not found", http.StatusNotFound)
				return
			}
			if errors.Is(err, database.ErrAPIClientInactive) {
				http.Error(w, "API client is revoked or expired", http.StatusConflict)
				return
			}
			shared.LoggerFromContext(r.Context()).Error("Failed to rotate api client", "error", err, "user_id", userID, "client_id", clientID)
			http.Error(w, "Failed to rotate api client", http.StatusInternalServerError)
			return
		}
		if client == nil {
			http.Error(w, "API client not found", http.StatusNotFound)
			return
		}

		if err := h.appendAPIClientAudit(r, userID, uuid.Nil, "api_client.rotated", client); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to audit api client rotation", "error", err, "user_id", userID, "client_id", clientID)
			http.Error(w, "Failed to audit api client action", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, apiClientTokenResponse{Client: *client, Token: token}); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode rotate api client response", "error", err, "user_id", userID, "client_id", clientID)
		}
	}
}

func (h *handler) handleDeleteAPIClient() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		clientID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
		if err != nil {
			http.Error(w, "Invalid client ID", http.StatusBadRequest)
			return
		}

		existing, err := h.ctx.Store.GetAPIClient(r.Context(), userID, clientID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to load api client before delete", "error", err, "user_id", userID, "client_id", clientID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if existing == nil {
			http.Error(w, "API client not found", http.StatusNotFound)
			return
		}

		err = h.ctx.Store.DeleteAPIClient(r.Context(), userID, clientID)
		if err != nil {
			if errors.Is(err, database.ErrAPIClientNotFound) {
				http.Error(w, "API client not found", http.StatusNotFound)
				return
			}
			shared.LoggerFromContext(r.Context()).Error("Failed to delete api client", "error", err, "user_id", userID, "client_id", clientID)
			http.Error(w, "Failed to delete api client", http.StatusInternalServerError)
			return
		}

		if err := h.appendAPIClientAudit(r, userID, uuid.Nil, "api_client.deleted", existing); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to audit api client delete", "error", err, "user_id", userID, "client_id", clientID)
			http.Error(w, "Failed to audit api client action", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *handler) handleListTeamAPIClients() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actorID, teamID, ok := h.resolveTeamAPIClientScope(w, r)
		if !ok {
			return
		}

		clients, err := h.ctx.Store.ListTeamAPIClients(r.Context(), teamID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to list team api clients", "error", err, "team_id", teamID, "actor_id", actorID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, clients); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode team api clients response", "error", err, "team_id", teamID, "actor_id", actorID)
		}
	}
}

func (h *handler) handleCreateTeamAPIClient() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actorID, teamID, ok := h.resolveTeamAPIClientScope(w, r)
		if !ok {
			return
		}

		var req createAPIClientRequest
		if err := decodeJSON(r, &req); err != nil {
			http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
			return
		}

		name := strings.TrimSpace(req.Name)
		if name == "" || len(name) > 120 {
			http.Error(w, "Name is required and must be <= 120 characters", http.StatusBadRequest)
			return
		}

		description := strings.TrimSpace(req.Description)
		if len(description) > 500 {
			http.Error(w, "Description must be <= 500 characters", http.StatusBadRequest)
			return
		}

		siteRoles, err := h.validateTeamDelegatedSiteRoles(r, teamID, req.SiteRoles)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Warn("Invalid delegated team site roles", "error", err, "team_id", teamID, "actor_id", actorID)
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}

		client, token, err := h.ctx.Store.CreateTeamAPIClient(
			r.Context(),
			teamID,
			name,
			description,
			siteRoles,
			req.ExpiresAt,
		)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to create team api client", "error", err, "team_id", teamID, "actor_id", actorID)
			http.Error(w, "Failed to create api client", http.StatusInternalServerError)
			return
		}

		if err := h.appendAPIClientAudit(r, actorID, teamID, "api_client.created", client); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to audit team api client create", "error", err, "team_id", teamID, "actor_id", actorID, "client_id", client.ID)
			http.Error(w, "Failed to audit api client action", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.MarshalWrite(w, apiClientTokenResponse{Client: *client, Token: token}); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode create team api client response", "error", err, "team_id", teamID, "actor_id", actorID)
		}
	}
}

func (h *handler) handleUpdateTeamAPIClient() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actorID, teamID, ok := h.resolveTeamAPIClientScope(w, r)
		if !ok {
			return
		}

		clientID, err := uuid.Parse(strings.TrimSpace(r.PathValue("clientId")))
		if err != nil {
			http.Error(w, "Invalid client ID", http.StatusBadRequest)
			return
		}

		existing, err := h.ctx.Store.GetTeamAPIClient(r.Context(), teamID, clientID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to load team api client before update", "error", err, "team_id", teamID, "actor_id", actorID, "client_id", clientID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if existing == nil {
			http.Error(w, "API client not found", http.StatusNotFound)
			return
		}

		var req updateAPIClientRequest
		if err := decodeJSON(r, &req); err != nil {
			http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
			return
		}

		name := strings.TrimSpace(req.Name)
		if name == "" || len(name) > 120 {
			http.Error(w, "Name is required and must be <= 120 characters", http.StatusBadRequest)
			return
		}

		description := strings.TrimSpace(req.Description)
		if len(description) > 500 {
			http.Error(w, "Description must be <= 500 characters", http.StatusBadRequest)
			return
		}

		siteRoles, err := h.validateTeamDelegatedSiteRoles(r, teamID, req.SiteRoles)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Warn("Invalid delegated team site roles for update", "error", err, "team_id", teamID, "actor_id", actorID, "client_id", clientID)
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}

		revoked := existing.RevokedAt != nil
		if req.Revoked != nil {
			revoked = *req.Revoked
		}

		updated, err := h.ctx.Store.UpdateTeamAPIClient(
			r.Context(),
			teamID,
			clientID,
			name,
			description,
			siteRoles,
			req.ExpiresAt,
			revoked,
		)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to update team api client", "error", err, "team_id", teamID, "actor_id", actorID, "client_id", clientID)
			http.Error(w, "Failed to update api client", http.StatusInternalServerError)
			return
		}
		if updated == nil {
			http.Error(w, "API client not found", http.StatusNotFound)
			return
		}

		if err := h.appendAPIClientAudit(r, actorID, teamID, apiClientUpdateAuditAction(existing, updated), updated); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to audit team api client update", "error", err, "team_id", teamID, "actor_id", actorID, "client_id", clientID)
			http.Error(w, "Failed to audit api client action", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, updated); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode team api client update response", "error", err, "team_id", teamID, "actor_id", actorID, "client_id", clientID)
		}
	}
}

func (h *handler) handleRotateTeamAPIClient() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actorID, teamID, ok := h.resolveTeamAPIClientScope(w, r)
		if !ok {
			return
		}

		clientID, err := uuid.Parse(strings.TrimSpace(r.PathValue("clientId")))
		if err != nil {
			http.Error(w, "Invalid client ID", http.StatusBadRequest)
			return
		}

		client, token, err := h.ctx.Store.RotateTeamAPIClient(r.Context(), teamID, clientID)
		if err != nil {
			if errors.Is(err, database.ErrAPIClientNotFound) {
				http.Error(w, "API client not found", http.StatusNotFound)
				return
			}
			if errors.Is(err, database.ErrAPIClientInactive) {
				http.Error(w, "API client is revoked or expired", http.StatusConflict)
				return
			}
			shared.LoggerFromContext(r.Context()).Error("Failed to rotate team api client", "error", err, "team_id", teamID, "actor_id", actorID, "client_id", clientID)
			http.Error(w, "Failed to rotate api client", http.StatusInternalServerError)
			return
		}
		if client == nil {
			http.Error(w, "API client not found", http.StatusNotFound)
			return
		}

		if err := h.appendAPIClientAudit(r, actorID, teamID, "api_client.rotated", client); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to audit team api client rotation", "error", err, "team_id", teamID, "actor_id", actorID, "client_id", clientID)
			http.Error(w, "Failed to audit api client action", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, apiClientTokenResponse{Client: *client, Token: token}); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode rotate team api client response", "error", err, "team_id", teamID, "actor_id", actorID, "client_id", clientID)
		}
	}
}

func (h *handler) handleDeleteTeamAPIClient() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actorID, teamID, ok := h.resolveTeamAPIClientScope(w, r)
		if !ok {
			return
		}

		clientID, err := uuid.Parse(strings.TrimSpace(r.PathValue("clientId")))
		if err != nil {
			http.Error(w, "Invalid client ID", http.StatusBadRequest)
			return
		}

		existing, err := h.ctx.Store.GetTeamAPIClient(r.Context(), teamID, clientID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to load team api client before delete", "error", err, "team_id", teamID, "actor_id", actorID, "client_id", clientID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if existing == nil {
			http.Error(w, "API client not found", http.StatusNotFound)
			return
		}

		err = h.ctx.Store.DeleteTeamAPIClient(r.Context(), teamID, clientID)
		if err != nil {
			if errors.Is(err, database.ErrAPIClientNotFound) {
				http.Error(w, "API client not found", http.StatusNotFound)
				return
			}
			shared.LoggerFromContext(r.Context()).Error("Failed to delete team api client", "error", err, "team_id", teamID, "actor_id", actorID, "client_id", clientID)
			http.Error(w, "Failed to delete api client", http.StatusInternalServerError)
			return
		}

		if err := h.appendAPIClientAudit(r, actorID, teamID, "api_client.deleted", existing); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to audit team api client delete", "error", err, "team_id", teamID, "actor_id", actorID, "client_id", clientID)
			http.Error(w, "Failed to audit api client action", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func apiClientUpdateAuditAction(existing, updated *api.APIClient) string {
	if existing != nil && existing.RevokedAt == nil && updated != nil && updated.RevokedAt != nil {
		return "api_client.revoked"
	}
	if existing != nil && existing.RevokedAt != nil && updated != nil && updated.RevokedAt == nil {
		return "api_client.reactivated"
	}
	return "api_client.updated"
}

func (h *handler) appendAPIClientAudit(r *http.Request, actorID, teamID uuid.UUID, action string, client *api.APIClient) error {
	if client == nil {
		return nil
	}
	ownerType := client.OwnerType
	if ownerType == "" {
		ownerType = database.APIClientOwnerPersonal
		if client.TenantID != nil {
			ownerType = database.APIClientOwnerTeam
		}
	}
	details := fmt.Sprintf("API client %q %s (owner=%s, client_id=%s)", client.Name, strings.TrimPrefix(action, "api_client."), ownerType, client.ID)
	return h.ctx.AppendAuditEventChecked(r.Context(), r, shared.AuditEvent{
		ActorID:     actorID,
		TeamID:      teamID,
		Action:      action,
		TargetType:  "api_client",
		TargetID:    client.ID.String(),
		TargetLabel: client.Name,
		Outcome:     "success",
		Details:     details,
	})
}

func (h *handler) validateDelegatedSiteRoles(r *http.Request, userID uuid.UUID, actorInstanceRole authcore.InstanceRole, roles []apiClientSiteRoleInput) (map[uuid.UUID]authcore.SiteRole, error) {
	if len(roles) == 0 {
		return map[uuid.UUID]authcore.SiteRole{}, nil
	}
	if len(roles) > 100 {
		return nil, errors.New("too many delegated site roles")
	}

	result := make(map[uuid.UUID]authcore.SiteRole, len(roles))
	for _, item := range roles {
		siteID, err := uuid.Parse(strings.TrimSpace(item.SiteID))
		if err != nil {
			return nil, errors.New("invalid delegated site role site_id")
		}

		requestedRole := authcore.SiteRole(strings.TrimSpace(item.Role))
		if !authcore.IsValidSiteRole(requestedRole) {
			return nil, errors.New("invalid delegated site role")
		}

		maxRole, err := h.maxDelegableSiteRole(r, userID, actorInstanceRole, siteID)
		if err != nil {
			return nil, err
		}
		if !authcore.CanAssignSiteRole(maxRole, requestedRole) {
			return nil, errors.New("cannot delegate requested site role")
		}

		result[siteID] = requestedRole
	}

	return result, nil
}

func (h *handler) maxDelegableSiteRole(r *http.Request, userID uuid.UUID, actorInstanceRole authcore.InstanceRole, siteID uuid.UUID) (authcore.SiteRole, error) {
	if actorInstanceRole == authcore.InstanceOwner {
		return authcore.SiteOwner, nil
	}

	role, err := h.ctx.Store.GetSiteRole(r.Context(), userID, siteID)
	if err != nil {
		return "", errors.New("cannot delegate site role for site without access")
	}
	return role, nil
}

func (h *handler) validateTeamDelegatedSiteRoles(r *http.Request, teamID uuid.UUID, roles []apiClientSiteRoleInput) (map[uuid.UUID]authcore.SiteRole, error) {
	if len(roles) == 0 {
		return map[uuid.UUID]authcore.SiteRole{}, nil
	}
	if len(roles) > 100 {
		return nil, errors.New("too many delegated site roles")
	}

	teamSites, err := h.ctx.Store.ListSitesForTenant(r.Context(), teamID)
	if err != nil {
		return nil, errors.New("cannot load team sites for delegation")
	}
	allowedSites := make(map[uuid.UUID]struct{}, len(teamSites))
	for _, site := range teamSites {
		allowedSites[site.ID] = struct{}{}
	}

	result := make(map[uuid.UUID]authcore.SiteRole, len(roles))
	for _, item := range roles {
		siteID, err := uuid.Parse(strings.TrimSpace(item.SiteID))
		if err != nil {
			return nil, errors.New("invalid delegated site role site_id")
		}
		if _, ok := allowedSites[siteID]; !ok {
			return nil, errors.New("cannot delegate site role outside the selected team")
		}

		requestedRole := authcore.SiteRole(strings.TrimSpace(item.Role))
		if !authcore.IsValidSiteRole(requestedRole) {
			return nil, errors.New("invalid delegated site role")
		}

		result[siteID] = requestedRole
	}

	return result, nil
}

func (h *handler) resolveTeamAPIClientScope(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	actorID := shared.GetUserIDFromContext(r)
	if actorID == uuid.Nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return uuid.Nil, uuid.Nil, false
	}

	teamID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		http.Error(w, "Invalid team ID", http.StatusBadRequest)
		return uuid.Nil, uuid.Nil, false
	}

	role, err := h.ctx.Store.GetTenantRole(r.Context(), teamID, actorID)
	if err != nil || !authcore.TeamRoleHasCapability(role, authcore.CapTeamManageAPIClients) {
		http.Error(w, "Access denied", http.StatusForbidden)
		return uuid.Nil, uuid.Nil, false
	}

	return actorID, teamID, true
}

func decodeJSON(r *http.Request, dst any) error {
	return json.UnmarshalReadStrict(io.LimitReader(r.Body, 1<<20), dst)
}
