package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/appurl"
	authcore "hitkeep/internal/auth"
	"hitkeep/internal/database"
	"hitkeep/internal/entitlements"
	"hitkeep/internal/mailables"
	serverauth "hitkeep/internal/server/auth"
	"hitkeep/internal/server/shared"
)

type resolvedSiteMemberUser struct {
	userID                uuid.UUID
	isTeamMember          bool
	inviteToken           string
	requiresPasswordSetup bool
}

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

		if h.ctx.Config.CloudHosted {
			http.Error(w, "Managed cloud teams cannot be archived", http.StatusForbidden)
			return
		}

		err = h.archiveTeam(r.Context(), teamID, actorID)
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
			if h.ctx.Config.CloudHosted {
				purgeable, purgeableErr := h.ctx.Store.GetPurgeableTenant(r.Context(), teamID)
				if purgeableErr != nil {
					if errors.Is(purgeableErr, database.ErrTeamPurgeNotArchived) {
						actorID := shared.GetUserIDFromContext(r)
						archived, archiveErr := h.archiveEmptyHostedCloudTeamForForceDelete(r.Context(), teamID, actorID)
						if archiveErr != nil {
							if errors.Is(archiveErr, database.ErrTeamArchiveDefaultTenant) {
								http.Error(w, "The default team cannot be deleted", http.StatusBadRequest)
								return
							}
							slog.Error("Failed to archive empty hosted cloud team during force delete", "error", archiveErr, "team_id", teamID)
							http.Error(w, "Failed to delete team", http.StatusInternalServerError)
							return
						}
						if !archived {
							http.Error(w, "Managed cloud teams cannot be force deleted", http.StatusForbidden)
							return
						}
					} else if errors.Is(purgeableErr, database.ErrTeamPurgeDefaultTenant) {
						http.Error(w, "The default team cannot be deleted", http.StatusBadRequest)
						return
					} else {
						slog.Error("Failed to check archived team before cloud force delete", "error", purgeableErr, "team_id", teamID)
						http.Error(w, "Failed to delete team", http.StatusInternalServerError)
						return
					}
				}
				if purgeableErr == nil && purgeable == nil {
					http.Error(w, "Managed cloud teams cannot be force deleted", http.StatusForbidden)
					return
				}
			} else {
				actorID := shared.GetUserIDFromContext(r)

				sites, sitesErr := h.ctx.Store.ListSitesForTenant(r.Context(), teamID)
				if sitesErr != nil {
					slog.Error("Failed to list sites for team during force delete", "error", sitesErr, "team_id", teamID)
					http.Error(w, "Failed to delete team", http.StatusInternalServerError)
					return
				}
				for _, site := range sites {
					err = h.deleteSite(r.Context(), site.ID)
					if err != nil {
						slog.Error("Failed to delete site during force team delete", "error", err, "site_id", site.ID, "team_id", teamID)
						http.Error(w, "Failed to delete team", http.StatusInternalServerError)
						return
					}
				}

				archiveErr := h.archiveTeam(r.Context(), teamID, actorID)
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

func (h *handler) archiveEmptyHostedCloudTeamForForceDelete(ctx context.Context, teamID, actorID uuid.UUID) (bool, error) {
	memberCount, err := h.ctx.Store.CountTeamMembers(ctx, teamID)
	if err != nil {
		return false, err
	}
	if memberCount > 0 {
		return false, nil
	}

	siteCount, err := h.ctx.Store.CountTeamSites(ctx, teamID)
	if err != nil {
		return false, err
	}
	if siteCount > 0 {
		return false, nil
	}

	if err := h.archiveTeam(ctx, teamID, actorID); err != nil {
		return false, err
	}
	return true, nil
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

		err = h.deleteSite(r.Context(), siteID)
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

		email, ok := normalizeSiteMemberEmail(w, req.Email)
		if !ok {
			return
		}

		actorID := shared.GetUserIDFromContext(r)
		teamID, teamErr := h.ctx.Store.GetSiteTenantID(r.Context(), siteID)
		if teamErr != nil {
			slog.Error("Failed to resolve site team", "error", teamErr, "site_id", siteID, "actor_id", actorID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		resolvedUser, err := h.resolveSiteMemberUser(r.Context(), teamID, actorID, email)
		if err != nil {
			if h.writeSiteInvitePreflightError(w, err, email, teamID, actorID) {
				return
			}
			slog.Error("Failed to resolve site member user", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		userID := resolvedUser.userID
		inviteToken := resolvedUser.inviteToken

		siteLabel := siteID.String()
		if site, siteErr := h.ctx.Store.GetSiteByID(r.Context(), siteID); siteErr == nil && site != nil && strings.TrimSpace(site.Domain) != "" {
			siteLabel = site.Domain
		}
		previousRole := ""
		if members, membersErr := h.ctx.Store.GetSiteMembers(r.Context(), siteID); membersErr == nil {
			for _, member := range members {
				if member.UserID == userID {
					previousRole = member.Role
					break
				}
			}
		}

		if resolvedUser.isTeamMember {
			err = h.ctx.Store.AddSiteMember(r.Context(), siteID, userID, authcore.SiteRole(req.Role), actorID)
		} else {
			err = h.ctx.Store.AddPendingSiteMemberInviteAccess(r.Context(), siteID, userID, authcore.SiteRole(req.Role), actorID)
		}
		if err != nil {
			slog.Error("Failed to add member", "error", err)
			http.Error(w, "Failed to add member", http.StatusInternalServerError)
			return
		}
		action := "permission.site_member_granted"
		details := fmt.Sprintf("Site member %s granted %s on %s", email, strings.TrimSpace(req.Role), siteLabel)
		if previousRole != "" {
			action = "permission.site_member_role_updated"
			details = fmt.Sprintf("Site member %s role changed from %s to %s on %s", email, previousRole, strings.TrimSpace(req.Role), siteLabel)
		}
		h.ctx.AppendAuditEvent(r.Context(), r, shared.AuditEvent{
			ActorID:      actorID,
			TeamID:       teamID,
			TargetUserID: userID,
			Action:       action,
			TargetType:   "permission",
			TargetID:     siteID.String(),
			TargetLabel:  siteLabel,
			Outcome:      "success",
			Details:      details,
		})

		if inviteToken != "" && h.ctx.Mailer != nil {
			site, err := h.ctx.Store.GetSiteByID(r.Context(), siteID)
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

			inviteLink := siteInviteLink(h.ctx.Config.PublicURL, inviteToken, resolvedUser.requiresPasswordSetup)
			err = h.ctx.Mailer.Send(email, mailables.NewUserInvite(inviteLink, siteName, inviterName, locale))
			if err != nil {
				slog.Warn("Failed to send invite email", "error", err, "email", email)
			}
		}

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}

func normalizeSiteMemberEmail(w http.ResponseWriter, value string) (string, bool) {
	email := strings.ToLower(strings.TrimSpace(value))
	parsedEmail, err := mail.ParseAddress(email)
	if err != nil || parsedEmail.Address != email {
		http.Error(w, "Invalid email", http.StatusBadRequest)
		return "", false
	}
	return email, true
}

func (h *handler) resolveSiteMemberUser(ctx context.Context, teamID, actorID uuid.UUID, email string) (resolvedSiteMemberUser, error) {
	user, err := h.ctx.Store.GetUserByEmail(ctx, email)
	if err != nil {
		return resolvedSiteMemberUser{}, fmt.Errorf("check user: %w", err)
	}

	result := resolvedSiteMemberUser{}
	if user != nil {
		result.userID = user.ID
	} else {
		tempPassword := uuid.New().String()
		hashedPassword, err := serverauth.HashPassword(tempPassword)
		if err != nil {
			return resolvedSiteMemberUser{}, fmt.Errorf("hash temporary password: %w", err)
		}

		if h.ctx.Config.CloudHosted {
			result.userID, err = h.ctx.Store.CreatePlaceholderUserWithoutDefaultTenant(ctx, email, hashedPassword)
		} else {
			result.userID, err = h.ctx.Store.CreatePlaceholderUser(ctx, email, hashedPassword)
		}
		if err != nil {
			return resolvedSiteMemberUser{}, fmt.Errorf("create user: %w", err)
		}
		result.requiresPasswordSetup = true
	}

	isTeamMember, err := h.ctx.Store.IsTenantMember(ctx, teamID, result.userID)
	if err != nil {
		return resolvedSiteMemberUser{}, fmt.Errorf("check tenant membership: %w", err)
	}
	result.isTeamMember = isTeamMember

	if !isTeamMember || result.requiresPasswordSetup {
		invite, err := h.findPendingTeamInviteForSite(ctx, teamID, email)
		if err != nil {
			return resolvedSiteMemberUser{}, err
		}
		if !isTeamMember && h.ctx.Config.CloudHosted {
			requireCapacity := invite == nil && !h.ctx.Limits().BypassesCloudLimits(ctx, actorID)
			if err := h.validateHostedCloudSiteInvitee(ctx, teamID, result.userID, email, requireCapacity); err != nil {
				return resolvedSiteMemberUser{}, err
			}
		}
		if invite == nil {
			invite, err = h.createSiteTeamInvite(ctx, teamID, actorID, result.userID, email, result.requiresPasswordSetup)
			if err != nil {
				return resolvedSiteMemberUser{}, err
			}
		}
		result.requiresPasswordSetup = invite.RequiresPasswordSetup
		result.inviteToken, err = h.ctx.Store.CreatePasswordResetToken(ctx, email)
		if err != nil {
			return resolvedSiteMemberUser{}, fmt.Errorf("create invite token: %w", err)
		}
	}

	return result, nil
}

func (h *handler) createSiteTeamInvite(ctx context.Context, teamID, actorID, userID uuid.UUID, email string, requiresPasswordSetup bool) (*api.TeamInvite, error) {
	invite, err := h.ctx.Store.CreateTeamInvite(ctx, teamID, email, database.TenantRoleMember, &userID, actorID, requiresPasswordSetup)
	if err == nil {
		return invite, nil
	}
	if !errors.Is(err, database.ErrTeamInviteAlreadyPending) {
		return nil, err
	}
	invite, err = h.findPendingTeamInviteForSite(ctx, teamID, email)
	if err != nil {
		return nil, err
	}
	if invite == nil {
		return nil, database.ErrTeamInviteAlreadyPending
	}
	return invite, nil
}

func (h *handler) findPendingTeamInviteForSite(ctx context.Context, teamID uuid.UUID, email string) (*api.TeamInvite, error) {
	pendingInvites, err := h.ctx.Store.ListPendingTeamInvitesByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	for idx := range pendingInvites {
		if pendingInvites[idx].TeamID == teamID {
			return &pendingInvites[idx], nil
		}
	}
	return nil, nil
}

func (h *handler) validateHostedCloudSiteInvitee(ctx context.Context, teamID, userID uuid.UUID, email string, requireCapacity bool) error {
	if requireCapacity {
		if err := h.ctx.Limits().RequireTeamMemberCapacity(ctx, teamID); err != nil {
			return err
		}
	}
	// Pending invites to other teams reserve membership slots so the invitee
	// cannot be promised more teams than their entitlement allows.
	pendingInvites, err := h.ctx.Store.ListPendingTeamInvitesByEmail(ctx, email)
	if err != nil {
		return fmt.Errorf("load pending cloud invites: %w", err)
	}
	pendingOutsideTeam := 0
	for _, pendingInvite := range pendingInvites {
		if pendingInvite.TeamID != teamID {
			pendingOutsideTeam++
		}
	}
	return h.ctx.Limits().RequireTeamMembershipCapacity(ctx, userID, 1+pendingOutsideTeam)
}

func (h *handler) writeSiteInvitePreflightError(w http.ResponseWriter, err error, email string, teamID, actorID uuid.UUID) bool {
	switch {
	case errors.Is(err, entitlements.ErrTeamMemberLimitReached):
		slog.Warn("Cloud team member limit reached for site invite", "error", err, "email", email, "team_id", teamID, "actor_id", actorID)
		http.Error(w, "Team member limit reached", http.StatusForbidden)
		return true
	case errors.Is(err, entitlements.ErrTeamMembershipLimitReached):
		http.Error(w, "Managed cloud accounts are limited to one team", http.StatusConflict)
		return true
	default:
		return false
	}
}

func siteInviteLink(publicURL, inviteToken string, requiresPasswordSetup bool) string {
	acceptPath := "/accept-invite?token=" + inviteToken
	if requiresPasswordSetup {
		return appurl.Path(publicURL, acceptPath)
	}
	return appurl.Path(publicURL, "/login?returnUrl="+url.QueryEscape(acceptPath))
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
		teamID, teamErr := h.ctx.Store.GetSiteTenantID(r.Context(), siteID)
		siteLabel := siteID.String()
		if site, siteErr := h.ctx.Store.GetSiteByID(r.Context(), siteID); siteErr == nil && site != nil && strings.TrimSpace(site.Domain) != "" {
			siteLabel = site.Domain
		}
		targetEmail := userID.String()
		previousRole := ""
		if members, membersErr := h.ctx.Store.GetSiteMembers(r.Context(), siteID); membersErr == nil {
			for _, member := range members {
				if member.UserID == userID {
					if strings.TrimSpace(member.Email) != "" {
						targetEmail = member.Email
					}
					previousRole = member.Role
					break
				}
			}
		}

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
		if teamErr == nil {
			details := fmt.Sprintf("Site member %s revoked from %s", targetEmail, siteLabel)
			if previousRole != "" {
				details = fmt.Sprintf("Site member %s role %s revoked from %s", targetEmail, previousRole, siteLabel)
			}
			h.ctx.AppendAuditEvent(r.Context(), r, shared.AuditEvent{
				ActorID:      actorID,
				TeamID:       teamID,
				TargetUserID: userID,
				Action:       "permission.site_member_revoked",
				TargetType:   "permission",
				TargetID:     siteID.String(),
				TargetLabel:  siteLabel,
				Outcome:      "success",
				Details:      details,
			})
		}

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}
