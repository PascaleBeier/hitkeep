package admin

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	authcore "hitkeep/internal/auth"
	"hitkeep/internal/database"
	"hitkeep/internal/mailables"
	serverauth "hitkeep/internal/server/auth"
	"hitkeep/internal/server/shared"
)

func (h *handler) handleAdminListTeams() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		teams, err := h.ctx.Store.ListAllTeams(r.Context())
		if err != nil {
			slog.Error("Failed to list all teams", "error", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(teams); err != nil {
			slog.Error("Failed to encode teams response", "error", err)
		}
	}
}

func (h *handler) handleAdminArchiveTeam() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		teamID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
		if err != nil {
			http.Error(w, "Invalid team ID", http.StatusBadRequest)
			return
		}

		actorID := shared.GetUserIDFromContext(r)

		err = h.ctx.Store.AdminArchiveTenant(r.Context(), teamID, actorID)
		if err != nil {
			switch {
			case errors.Is(err, database.ErrTeamArchiveDefaultTenant):
				http.Error(w, "The default team cannot be archived", http.StatusBadRequest)
			case errors.Is(err, database.ErrTenantMembershipRequired):
				http.Error(w, "Team not found or already archived", http.StatusBadRequest)
			default:
				slog.Error("Failed to archive team", "error", err, "team_id", teamID)
				http.Error(w, "Internal error", http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
			slog.Error("Failed to encode archive team response", "error", err)
		}
	}
}

func (h *handler) handleAdminDeleteTeam() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.TenantStores == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		teamID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
		if err != nil {
			http.Error(w, "Invalid team ID", http.StatusBadRequest)
			return
		}

		force := r.URL.Query().Get("force") == "true"

		if force {
			actorID := shared.GetUserIDFromContext(r)

			sites, sitesErr := h.ctx.Store.ListSitesForTenant(r.Context(), teamID)
			if sitesErr != nil {
				slog.Error("Failed to list sites for team during force delete", "error", sitesErr, "team_id", teamID)
				http.Error(w, "Failed to delete team", http.StatusInternalServerError)
				return
			}
			for _, site := range sites {
				if h.ctx.TenantStores != nil {
					err = h.ctx.TenantStores.DeleteSite(r.Context(), site.ID)
				} else {
					err = h.ctx.Store.DeleteSite(r.Context(), site.ID)
				}
				if err != nil {
					slog.Error("Failed to delete site during force team delete", "error", err, "site_id", site.ID, "team_id", teamID)
					http.Error(w, "Failed to delete team", http.StatusInternalServerError)
					return
				}
			}

			archiveErr := h.ctx.Store.AdminArchiveTenant(r.Context(), teamID, actorID)
			if archiveErr != nil && !errors.Is(archiveErr, database.ErrTenantMembershipRequired) {
				if errors.Is(archiveErr, database.ErrTeamArchiveDefaultTenant) {
					http.Error(w, "The default team cannot be deleted", http.StatusBadRequest)
					return
				}
				slog.Error("Failed to archive team during force delete", "error", archiveErr, "team_id", teamID)
				http.Error(w, "Failed to delete team", http.StatusInternalServerError)
				return
			}
		}

		deleted, err := h.ctx.TenantStores.PurgeArchivedTenant(r.Context(), teamID)
		if err != nil {
			switch {
			case errors.Is(err, database.ErrTeamPurgeDefaultTenant):
				http.Error(w, "The default team cannot be deleted", http.StatusBadRequest)
			case errors.Is(err, database.ErrTeamPurgeNotArchived):
				http.Error(w, "Archive the team before deleting it", http.StatusBadRequest)
			case errors.Is(err, database.ErrTeamPurgeHasSites):
				http.Error(w, "Transfer or delete all sites before deleting the team", http.StatusBadRequest)
			default:
				slog.Error("Failed to purge archived team", "error", err, "team_id", teamID)
				http.Error(w, "Internal error", http.StatusInternalServerError)
			}
			return
		}
		if deleted == nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(api.AdminDeleteTeamResponse{
			Status: "ok",
			TeamID: deleted.ID,
			Name:   deleted.Name,
		}); err != nil {
			slog.Error("Failed to encode delete team response", "error", err, "team_id", teamID)
		}
	}
}

func (h *handler) handleAdminListSites() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sites, err := h.ctx.Store.ListAllSites(r.Context())
		if err != nil {
			slog.Error("Failed to list all sites", "error", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(sites); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}

func (h *handler) handleAdminDeleteSite() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteIDStr := r.PathValue("id")
		siteID, err := uuid.Parse(siteIDStr)
		if err != nil {
			http.Error(w, "Invalid site ID", http.StatusBadRequest)
			return
		}

		if h.ctx.TenantStores != nil {
			err = h.ctx.TenantStores.DeleteSite(r.Context(), siteID)
		} else {
			err = h.ctx.Store.DeleteSite(r.Context(), siteID)
		}
		if err != nil {
			slog.Error("Failed to delete site", "error", err)
			http.Error(w, "Failed to delete site", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}

func (h *handler) handleGetSiteMembers() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteIDStr := r.PathValue("id")
		siteID, err := uuid.Parse(siteIDStr)
		if err != nil {
			http.Error(w, "Invalid site ID", http.StatusBadRequest)
			return
		}

		members, err := h.ctx.Store.GetSiteMembers(r.Context(), siteID)
		if err != nil {
			slog.Error("Failed to get members", "error", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(members); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}

func (h *handler) handleAddSiteMember() http.HandlerFunc {
	type request struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		siteIDStr := r.PathValue("id")
		siteID, err := uuid.Parse(siteIDStr)
		if err != nil {
			http.Error(w, "Invalid site ID", http.StatusBadRequest)
			return
		}

		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		user, err := h.ctx.Store.GetUserByEmail(r.Context(), req.Email)
		if err != nil {
			slog.Error("Database error checking user", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		var userID uuid.UUID
		var isNewUser bool
		var inviteToken string

		if user == nil {
			tempPassword := uuid.New().String()
			hashedPassword, err := serverauth.HashPassword(tempPassword)
			if err != nil {
				slog.Error("Failed to hash password", "error", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			userID, err = h.ctx.Store.CreateUser(r.Context(), req.Email, hashedPassword)
			if err != nil {
				slog.Error("Failed to create user", "error", err)
				http.Error(w, "Failed to create user", http.StatusInternalServerError)
				return
			}
			isNewUser = true

			inviteToken, err = h.ctx.Store.CreatePasswordResetToken(r.Context(), req.Email)
			if err != nil {
				slog.Error("Failed to create invite token", "error", err)
			}
		} else {
			userID = user.ID
		}

		actorID := shared.GetUserIDFromContext(r)

		err = h.ctx.Store.AddSiteMember(r.Context(), siteID, userID, authcore.SiteRole(req.Role), actorID)
		if err != nil {
			slog.Error("Failed to add member", "error", err)
			http.Error(w, "Failed to add member", http.StatusInternalServerError)
			return
		}

		if isNewUser && inviteToken != "" {
			site, err := h.ctx.Store.GetSite(r.Context(), siteID, actorID)
			siteName := "Unknown Site"
			if err == nil && site != nil {
				siteName = site.Domain
			}

			inviter, err := h.ctx.Store.GetUserByID(r.Context(), actorID)
			inviterName := "Someone"
			if err == nil && inviter != nil {
				inviterName = inviter.Email
			}

			locale := "en"
			if actorID != uuid.Nil {
				if resolvedLocale, err := h.ctx.Store.GetUserLocale(r.Context(), actorID); err == nil && strings.TrimSpace(resolvedLocale) != "" {
					locale = resolvedLocale
				}
			}

			inviteLink := h.ctx.Config.PublicURL + "/accept-invite?token=" + inviteToken
			err = h.ctx.Mailer.Send(req.Email, mailables.NewUserInvite(inviteLink, siteName, inviterName, locale))
			if err != nil {
				slog.Warn("Failed to send invite email", "error", err, "email", req.Email)
			}
		}

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}

func (h *handler) handleRemoveSiteMember() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteIDStr := r.PathValue("id")
		siteID, err := uuid.Parse(siteIDStr)
		if err != nil {
			http.Error(w, "Invalid site ID", http.StatusBadRequest)
			return
		}

		userIDStr := r.PathValue("userId")
		userID, err := uuid.Parse(userIDStr)
		if err != nil {
			http.Error(w, "Invalid user ID", http.StatusBadRequest)
			return
		}

		actorID := shared.GetUserIDFromContext(r)

		if actorID == userID {
			role, _ := h.ctx.Store.GetSiteRole(r.Context(), userID, siteID)
			if role == authcore.SiteOwner {
				owners, _ := h.ctx.Store.CountSiteOwners(r.Context(), siteID)
				if owners <= 1 {
					http.Error(w, "Cannot remove the last owner", http.StatusBadRequest)
					return
				}
			}
		}

		err = h.ctx.Store.RemoveSiteMember(r.Context(), siteID, userID, actorID)
		if err != nil {
			slog.Error("Failed to remove member", "error", err)
			http.Error(w, "Failed to remove member", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}
