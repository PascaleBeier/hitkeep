package admin

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	authcore "hitkeep/internal/auth"
	"hitkeep/internal/blocking"
	"hitkeep/internal/database"
	"hitkeep/internal/server/shared"
)

type handler struct {
	ctx *shared.Context
}

func Register(mux *http.ServeMux, ctx *shared.Context) {
	h := &handler{ctx: ctx}
	mux.HandleFunc("GET /api/admin/users", ctx.Handler(shared.HandlerConfig{
		InstancePerm: authcore.PermInstanceManageUsers,
		RateLimiter:  ctx.ApiLimiter,
	}, h.handleListUsers()))
	mux.HandleFunc("POST /api/admin/users/{id}/disable-2fa", ctx.Handler(shared.HandlerConfig{
		InstancePerm: authcore.PermInstanceManageUsers,
		RateLimiter:  ctx.ApiLimiter,
	}, h.handleDisableUser2FA()))
	mux.HandleFunc("POST /api/admin/users/{id}/role", ctx.Handler(shared.HandlerConfig{
		InstancePerm: authcore.PermInstanceManageUsers,
		RateLimiter:  ctx.ApiLimiter,
	}, h.handleUpdateUserRole()))
	mux.HandleFunc("DELETE /api/admin/users/{id}", ctx.Handler(shared.HandlerConfig{
		InstancePerm: authcore.PermInstanceManageUsers,
		RateLimiter:  ctx.ApiLimiter,
	}, h.handleDeleteUser()))
	mux.HandleFunc("GET /api/admin/sites", ctx.Handler(shared.HandlerConfig{
		InstancePerm: authcore.PermInstanceManageUsers,
		RateLimiter:  ctx.ApiLimiter,
	}, h.handleAdminListSites()))
	mux.HandleFunc("DELETE /api/admin/sites/{id}", ctx.Handler(shared.HandlerConfig{
		InstancePerm: authcore.PermInstanceManageUsers,
		RateLimiter:  ctx.ApiLimiter,
	}, h.handleAdminDeleteSite()))
	mux.HandleFunc("GET /api/admin/teams", ctx.Handler(shared.HandlerConfig{
		InstancePerm: authcore.PermInstanceManageUsers,
		RateLimiter:  ctx.ApiLimiter,
	}, h.handleAdminListTeams()))
	mux.HandleFunc("POST /api/admin/teams/{id}/archive", ctx.Handler(shared.HandlerConfig{
		InstancePerm: authcore.PermInstanceManageUsers,
		RateLimiter:  ctx.ApiLimiter,
	}, h.handleAdminArchiveTeam()))
	mux.HandleFunc("DELETE /api/admin/teams/{id}", ctx.Handler(shared.HandlerConfig{
		InstancePerm: authcore.PermInstanceManageUsers,
		RateLimiter:  ctx.ApiLimiter,
	}, h.handleAdminDeleteTeam()))
	mux.HandleFunc("GET /api/admin/exclusions", ctx.Handler(shared.HandlerConfig{
		InstancePerm: authcore.PermInstanceViewAllSites,
		RateLimiter:  ctx.ApiLimiter,
	}, h.handleListInstanceExclusions()))
	mux.HandleFunc("POST /api/admin/exclusions", ctx.Handler(shared.HandlerConfig{
		InstancePerm: authcore.PermInstanceViewAllSites,
		RateLimiter:  ctx.ApiLimiter,
	}, h.handleCreateInstanceExclusion()))
	mux.HandleFunc("DELETE /api/admin/exclusions/{ruleID}", ctx.Handler(shared.HandlerConfig{
		InstancePerm: authcore.PermInstanceViewAllSites,
		RateLimiter:  ctx.ApiLimiter,
	}, h.handleDeleteInstanceExclusion()))

	mux.HandleFunc("GET /api/sites/{id}/members", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetSiteMembers()))
	mux.HandleFunc("POST /api/sites/{id}/members", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteManageTeam,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleAddSiteMember()))
	mux.HandleFunc("DELETE /api/sites/{id}/members/{userId}", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteManageTeam,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleRemoveSiteMember()))
}

func (h *handler) handleListUsers() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		users, err := h.ctx.Store.ListUsers(r.Context())
		if err != nil {
			slog.Error("Failed to list users", "error", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(users); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}

func (h *handler) handleListInstanceExclusions() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		rules, err := h.ctx.Store.ListInstanceExclusions(r.Context())
		if err != nil {
			slog.Error("Failed to list instance exclusions", "error", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(rules); err != nil {
			slog.Error("Failed to encode instance exclusions response", "error", err)
		}
	}
}

func (h *handler) handleCreateInstanceExclusion() http.HandlerFunc {
	type request struct {
		CIDR        string `json:"cidr"`
		Description string `json:"description"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
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

		rule, err := h.ctx.Store.CreateInstanceExclusion(r.Context(), normalizedCIDR, description, userID)
		if err != nil {
			slog.Error("Failed to create instance exclusion", "error", err, "cidr", normalizedCIDR)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		h.refreshIPFilter(r.Context())

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(rule); err != nil {
			slog.Error("Failed to encode instance exclusion response", "error", err)
		}
	}
}

func (h *handler) handleDeleteInstanceExclusion() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		ruleID, err := uuid.Parse(strings.TrimSpace(r.PathValue("ruleID")))
		if err != nil {
			http.Error(w, "Invalid rule ID", http.StatusBadRequest)
			return
		}

		deleted, err := h.ctx.Store.DeleteInstanceExclusion(r.Context(), ruleID)
		if err != nil {
			slog.Error("Failed to delete instance exclusion", "error", err, "rule_id", ruleID)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
		if !deleted {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		h.refreshIPFilter(r.Context())

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

func (h *handler) deleteSite(ctx context.Context, siteID uuid.UUID) error {
	if h.ctx.TenantStores != nil {
		return h.ctx.TenantStores.DeleteSite(ctx, siteID)
	}
	return h.ctx.Store.DeleteSite(ctx, siteID)
}

func (h *handler) actorInstanceRole(r *http.Request) (authcore.InstanceRole, error) {
	if permissionCtx, ok := r.Context().Value(shared.PermissionKey).(shared.PermissionContext); ok && permissionCtx.InstanceRole != "" {
		return permissionCtx.InstanceRole, nil
	}

	actorID := shared.GetUserIDFromContext(r)
	if actorID == uuid.Nil {
		return authcore.InstanceUser, nil
	}

	return h.ctx.Store.GetInstanceRole(r.Context(), actorID)
}

func (h *handler) handleDisableUser2FA() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		targetUserID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
		if err != nil {
			http.Error(w, "Invalid user ID", http.StatusBadRequest)
			return
		}

		actorRole, err := h.actorInstanceRole(r)
		if err != nil {
			slog.Error("Failed to resolve actor role for disable-2fa", "error", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
		if actorRole != authcore.InstanceOwner {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		targetUser, err := h.ctx.Store.GetUserByID(r.Context(), targetUserID)
		if err != nil {
			slog.Error("Failed to load target user for disable-2fa", "error", err, "target_user_id", targetUserID)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
		if targetUser == nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		result, err := h.ctx.Store.DisableUserMFA(r.Context(), targetUserID)
		if err != nil {
			slog.Error("Failed to disable user MFA", "error", err, "target_user_id", targetUserID)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
		if h.ctx.AuthState != nil {
			h.ctx.AuthState.ClearUser(targetUserID)
		}

		actorID := shared.GetUserIDFromContext(r)
		slog.Info("Admin disabled user MFA",
			"actor_user_id", actorID,
			"target_user_id", targetUserID,
			"target_email", targetUser.Email,
			"totp_disabled", result.TOTPDisabled,
			"passkeys_deleted", result.PasskeysDeleted,
			"sessions_invalidated", result.SessionsInvalidated,
		)

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(api.AdminDisableUserMFAResponse{
			Status:              "ok",
			TOTPDisabled:        result.TOTPDisabled,
			PasskeysDeleted:     result.PasskeysDeleted,
			SessionsInvalidated: result.SessionsInvalidated,
		}); err != nil {
			slog.Error("Failed to encode disable user MFA response", "error", err, "target_user_id", targetUserID)
		}
	}
}

func (h *handler) handleUpdateUserRole() http.HandlerFunc {
	type request struct {
		Role string `json:"role"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		targetUserIDStr := r.PathValue("id")
		targetUserID, err := uuid.Parse(targetUserIDStr)
		if err != nil {
			http.Error(w, "Invalid user ID", http.StatusBadRequest)
			return
		}

		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		actorID := shared.GetUserIDFromContext(r)

		err = h.ctx.Store.UpdateInstanceRole(r.Context(), targetUserID, authcore.InstanceRole(req.Role), actorID)
		if err != nil {
			slog.Error("Failed to update role", "error", err)
			http.Error(w, "Failed to update role", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}

func (h *handler) handleDeleteUser() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		targetUserIDStr := r.PathValue("id")
		targetUserID, err := uuid.Parse(targetUserIDStr)
		if err != nil {
			http.Error(w, "Invalid user ID", http.StatusBadRequest)
			return
		}

		actorID := shared.GetUserIDFromContext(r)

		if actorID == targetUserID {
			http.Error(w, "Cannot delete yourself", http.StatusBadRequest)
			return
		}

		force := r.URL.Query().Get("force") == "true"

		if force {
			soleTeams, listErr := h.ctx.Store.ListSoleOwnerTeams(r.Context(), targetUserID)
			if listErr != nil {
				slog.Error("Failed to list sole-owner teams for force delete", "error", listErr, "target_user_id", targetUserID)
				http.Error(w, "Failed to delete user", http.StatusInternalServerError)
				return
			}
			for _, team := range soleTeams {
				sites, sitesErr := h.ctx.Store.ListSitesForTenant(r.Context(), team.ID)
				if sitesErr != nil {
					slog.Error("Failed to list sites for team during force delete", "error", sitesErr, "team_id", team.ID)
					http.Error(w, "Failed to delete user", http.StatusInternalServerError)
					return
				}
				for _, site := range sites {
					if delErr := h.deleteSite(r.Context(), site.ID); delErr != nil {
						slog.Error("Failed to delete site during force delete", "error", delErr, "site_id", site.ID, "team_id", team.ID)
						http.Error(w, "Failed to delete user", http.StatusInternalServerError)
						return
					}
				}

				if archiveErr := h.ctx.Store.AdminArchiveTenant(r.Context(), team.ID, actorID); archiveErr != nil {
					slog.Error("Failed to archive team during force delete", "error", archiveErr, "team_id", team.ID, "target_user_id", targetUserID)
					http.Error(w, "Failed to delete user", http.StatusInternalServerError)
					return
				}
			}
		}

		if h.ctx.TenantStores != nil {
			blockingTeams, listErr := h.ctx.Store.ListSoleOwnerTeams(r.Context(), targetUserID)
			if listErr != nil {
				slog.Error("Failed to list sole-owner teams before delete", "error", listErr, "target_user_id", targetUserID)
				http.Error(w, "Failed to delete user", http.StatusInternalServerError)
				return
			}
			if len(blockingTeams) > 0 {
				writeDeleteUserBlocked(w, targetUserID, blockingTeams)
				return
			}

			siteIDs, listErr := h.ctx.Store.ListUserSiteIDs(r.Context(), targetUserID)
			if listErr != nil {
				slog.Error("Failed to list owned sites before delete", "error", listErr, "target_user_id", targetUserID)
				http.Error(w, "Failed to delete user", http.StatusInternalServerError)
				return
			}
			for _, siteID := range siteIDs {
				if delErr := h.ctx.TenantStores.DeleteSite(r.Context(), siteID); delErr != nil {
					slog.Error("Failed to delete owned site before user delete", "error", delErr, "site_id", siteID, "target_user_id", targetUserID)
					http.Error(w, "Failed to delete user", http.StatusInternalServerError)
					return
				}
			}
		}

		err = h.ctx.Store.DeleteUser(r.Context(), targetUserID)
		if err != nil {
			var ownsTeamsErr *database.UserOwnsTeamsError
			if errors.As(err, &ownsTeamsErr) {
				writeDeleteUserBlocked(w, targetUserID, ownsTeamsErr.Teams)
				return
			}
			slog.Error("Failed to delete user", "error", err)
			http.Error(w, "Failed to delete user", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}

func writeDeleteUserBlocked(w http.ResponseWriter, targetUserID uuid.UUID, teams []api.Team) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	if encodeErr := json.NewEncoder(w).Encode(api.AdminDeleteUserBlockedResponse{
		Status:  "error",
		Code:    "user_owns_teams",
		Message: "Transfer ownership before deleting this user, or use ?force=true to archive their teams.",
		Teams:   teams,
	}); encodeErr != nil {
		slog.Error("Failed to encode delete user blocked response", "error", encodeErr, "target_user_id", targetUserID)
	}
}
