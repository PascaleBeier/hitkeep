package user

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	authcore "hitkeep/internal/auth"
	"hitkeep/internal/server/shared"
)

func (h *handler) handleGetUserBootstrap() http.HandlerFunc {
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

		bootstrap, err := h.userBootstrapResponse(r, userID)
		if err != nil {
			if errors.Is(err, errBootstrapSessionUnavailable) {
				http.Error(w, "Session unavailable", http.StatusUnauthorized)
				return
			}
			if errors.Is(err, errBootstrapUserNotFound) {
				http.Error(w, "User not found", http.StatusNotFound)
				return
			}
			slog.Error("Failed to build user bootstrap", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(bootstrap); err != nil {
			slog.Error("Failed to encode user bootstrap", "error", err, "user_id", userID)
		}
	}
}

var (
	errBootstrapSessionUnavailable = errors.New("session unavailable")
	errBootstrapUserNotFound       = errors.New("user not found")
)

func (h *handler) userBootstrapResponse(r *http.Request, userID uuid.UUID) (api.UserBootstrap, error) {
	session, err := h.authSessionResponse(r, userID)
	if err != nil {
		return api.UserBootstrap{}, err
	}
	profile, err := h.userProfileResponse(r.Context(), userID)
	if err != nil {
		return api.UserBootstrap{}, err
	}
	preferences, err := h.userPreferencesResponse(r, userID)
	if err != nil {
		return api.UserBootstrap{}, err
	}
	teams, err := h.userTeamsResponse(r, userID)
	if err != nil {
		return api.UserBootstrap{}, err
	}
	sites, err := h.userSitesResponse(r.Context(), userID)
	if err != nil {
		return api.UserBootstrap{}, err
	}
	permissions, err := h.userPermissionContext(r.Context(), userID, sites)
	if err != nil {
		return api.UserBootstrap{}, err
	}
	status, err := h.ctx.SystemStatusResponse(r.Context())
	if err != nil {
		return api.UserBootstrap{}, err
	}

	return api.UserBootstrap{
		Session:     session,
		Profile:     profile,
		Preferences: preferences,
		Teams:       teams,
		Permissions: permissions,
		Sites:       sites,
		Status:      status,
	}, nil
}

func (h *handler) authSessionResponse(r *http.Request, userID uuid.UUID) (api.AuthSession, error) {
	session, ok := r.Context().Value(shared.AuthSessionKey).(shared.AuthSessionContext)
	if !ok || session.ExpiresAt.IsZero() {
		return api.AuthSession{}, errBootstrapSessionUnavailable
	}

	return h.ctx.AuthSessionResponseForRequest(r, userID, session), nil
}

func (h *handler) userProfileResponse(ctx context.Context, userID uuid.UUID) (api.UserProfile, error) {
	user, err := h.ctx.Store.GetUserByID(ctx, userID)
	if err != nil {
		return api.UserProfile{}, fmt.Errorf("load user profile: %w", err)
	}
	if user == nil {
		return api.UserProfile{}, errBootstrapUserNotFound
	}

	return api.UserProfile{
		ID:          user.ID,
		Email:       user.Email,
		GivenName:   user.GivenName,
		LastName:    user.LastName,
		DisplayName: displayNameForUser(user),
		AvatarURL:   "/api/user/avatar?s=96",
	}, nil
}

func (h *handler) userPreferencesResponse(r *http.Request, userID uuid.UUID) (api.UserPreferences, error) {
	prefs, err := h.ctx.Store.GetUserPreferences(r.Context(), userID)
	if err != nil {
		return api.UserPreferences{}, fmt.Errorf("load user preferences: %w", err)
	}

	if prefs == nil {
		return defaultPreferencesFromHeader(r.Header.Get("Accept-Language")), nil
	}
	normalized := normalizeLocaleTag(prefs.DefaultLocale)
	if normalized == "" {
		return defaultPreferencesFromHeader(r.Header.Get("Accept-Language")), nil
	}
	prefs.DefaultLocale = normalized
	return *prefs, nil
}

func (h *handler) userTeamsResponse(r *http.Request, userID uuid.UUID) (api.UserTeamsResponse, error) {
	teams, activeTeamID, err := h.ctx.Store.ListUserTeams(r.Context(), userID)
	if err != nil {
		return api.UserTeamsResponse{}, fmt.Errorf("list user teams: %w", err)
	}

	return api.UserTeamsResponse{
		ActiveTeamID:  activeTeamID,
		RecentTeamIDs: orderedRecentTeamIDs(teams, activeTeamID),
		Teams:         h.hydrateTeamSummaries(r, teams),
	}, nil
}

func (h *handler) userPermissionContext(ctx context.Context, userID uuid.UUID, sites []api.Site) (api.PermissionContext, error) {
	instanceRole, err := h.ctx.Store.GetInstanceRole(ctx, userID)
	if err != nil {
		return api.PermissionContext{}, fmt.Errorf("get instance role: %w", err)
	}

	siteRoles := map[string]string{}
	for _, site := range sites {
		role, err := h.ctx.Store.GetSiteRole(ctx, userID, site.ID)
		if err != nil {
			if !instanceRole.HasPermission(authcore.PermInstanceViewAllSites) {
				return api.PermissionContext{}, fmt.Errorf("resolve site role %s: %w", site.ID, err)
			}
			continue
		}
		siteRoles[site.ID.String()] = string(role)
	}

	instancePermissions := instanceRole.Permissions()
	permissions := make([]string, 0, len(instancePermissions))
	for _, permission := range instancePermissions {
		permissions = append(permissions, string(permission))
	}

	return api.PermissionContext{
		InstanceRole:        string(instanceRole),
		Permissions:         siteRoles,
		InstancePermissions: permissions,
	}, nil
}

func (h *handler) userSitesResponse(ctx context.Context, userID uuid.UUID) ([]api.Site, error) {
	sites, err := h.ctx.Store.GetSites(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list user sites: %w", err)
	}
	if sites == nil {
		return []api.Site{}, nil
	}
	return sites, nil
}
