package permissions

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	authcore "hitkeep/internal/auth"
	"hitkeep/internal/server/shared"
)

type handler struct {
	ctx *shared.Context
}

func Register(mux *http.ServeMux, ctx *shared.Context) {
	h := &handler{ctx: ctx}
	mux.HandleFunc("GET /api/user/permissions", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetUserPermissions()))
}

func (h *handler) handleGetUserPermissions() http.HandlerFunc {
	type response struct {
		InstanceRole        authcore.InstanceRole        `json:"instance_role"`
		Permissions         map[string]authcore.SiteRole `json:"permissions"`
		InstancePermissions []authcore.Permission        `json:"instance_permissions"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		instanceRole, err := h.ctx.Store.GetInstanceRole(r.Context(), userID)
		if err != nil {
			//nolint:gosec // user_id is sourced from authenticated context; structured logging is intentional.
			slog.Error("Failed to get instance role", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		siteRoles := map[string]authcore.SiteRole{}
		sites, err := h.ctx.Store.GetSites(r.Context(), userID)
		if err != nil {
			//nolint:gosec // user_id is sourced from authenticated context; structured logging is intentional.
			slog.Error("Failed to list sites for permission context", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		for _, site := range sites {
			role, err := h.ctx.Store.GetSiteRole(r.Context(), userID, site.ID)
			if err != nil {
				if !instanceRole.HasPermission(authcore.PermInstanceViewAllSites) {
					slog.Error("Failed to resolve site role for permission context", "error", err, "user_id", userID, "site_id", site.ID)
					http.Error(w, "Internal server error", http.StatusInternalServerError)
					return
				}
				continue
			}
			siteRoles[site.ID.String()] = role
		}

		resp := response{
			InstanceRole:        instanceRole,
			Permissions:         siteRoles,
			InstancePermissions: instanceRole.Permissions(),
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}
