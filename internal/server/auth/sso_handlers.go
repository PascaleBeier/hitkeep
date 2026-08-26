package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/appurl"
	"hitkeep/internal/api"
	"hitkeep/internal/database"
	"hitkeep/internal/security"
	"hitkeep/internal/server/shared"
	"hitkeep/internal/sso"
	"hitkeep/internal/webhooks"
	json "hitkeep/jsonapi"
)

const ssoFlowTTL = 10 * time.Minute

type ssoStartRequest struct {
	Email       string `json:"email"`
	InviteToken string `json:"invite_token"`
	ReturnURL   string `json:"return_url"`
	RememberMe  bool   `json:"remember_me"`
}

var errSSOAccessDenied = errors.New("SSO access denied")

type ssoAccessMode uint8

const (
	ssoAccessExistingMember ssoAccessMode = iota + 1
	ssoAccessInvitation
	ssoAccessAutoProvision
)

type ssoAccessDecision struct {
	Mode   ssoAccessMode
	UserID uuid.UUID
}

func (h *handler) handleSSOStart() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil || h.ctx.AuthState == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}
		var req ssoStartRequest
		if err := json.UnmarshalReadStrict(http.MaxBytesReader(w, r.Body, 1<<20), &req); err != nil {
			writeSSOStartError(r.Context(), w, http.StatusBadRequest, "invalid_email")
			return
		}
		email, err := h.resolveSSORequestEmail(r.Context(), req.Email, req.InviteToken)
		if err != nil {
			if strings.TrimSpace(req.InviteToken) != "" {
				writeSSOStartError(r.Context(), w, http.StatusForbidden, "sso_access_denied")
			} else {
				writeSSOStartError(r.Context(), w, http.StatusBadRequest, "invalid_email")
			}
			return
		}
		_, domain, _ := sso.NormalizeEmail(email)
		config, err := h.ctx.Store.GetEnabledTeamSSOConfigByDomain(r.Context(), domain)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to resolve SSO provider", "error_kind", ssoErrorKind(err, "provider_lookup_failed"))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if config == nil {
			h.appendSSOAudit(r, "auth.sso_login_failed", "failure", uuid.Nil, uuid.Nil, email, ssoAuditFlowStart, ssoReasonProviderUnresolved, 0, "SSO login could not resolve an enabled provider")
			writeSSOStartError(r.Context(), w, http.StatusBadRequest, "sso_unavailable")
			return
		}
		if !h.ctx.Limits().AllowsSSO(r.Context(), uuid.Nil, config.TeamID) {
			h.appendSSOAudit(r, "auth.sso_login_failed", "failure", config.TeamID, uuid.Nil, email, ssoAuditFlowStart, ssoReasonEntitlementDenied, 0, "SSO login is not entitled for the configured team")
			writeSSOStartError(r.Context(), w, http.StatusBadRequest, "sso_unavailable")
			return
		}
		decision, err := h.authorizeSSOAccess(r.Context(), config, email, req.InviteToken)
		if err != nil {
			if errors.Is(err, errSSOAccessDenied) {
				h.appendSSOAudit(r, "auth.sso_login_failed", "failure", config.TeamID, uuid.Nil, email, ssoAuditFlowStart, ssoReasonAccessDenied, 0, "SSO login was not authorized for the configured team")
				writeSSOStartError(r.Context(), w, http.StatusForbidden, "sso_access_denied")
				return
			}
			shared.LoggerFromContext(r.Context()).Error("Failed to authorize SSO login", "error_kind", ssoErrorKind(err, "access_authorization_failed"), "team_id", config.TeamID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if err := h.requireSSOAccessCapacity(r.Context(), config.TeamID, decision); err != nil {
			h.appendSSOAudit(r, "auth.sso_login_failed", "failure", config.TeamID, decision.UserID, email, ssoAuditFlowStart, ssoReasonCapacityDenied, decision.Mode, "SSO login exceeded a managed-cloud team or seat limit")
			writeSSOStartError(r.Context(), w, http.StatusForbidden, "sso_access_denied")
			return
		}

		teamConfig, err := h.teamSSOConfig(config)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Warn("Failed to decrypt configured SSO provider", "team_id", config.TeamID)
			h.appendSSOAudit(r, "auth.sso_login_failed", "failure", config.TeamID, decision.UserID, email, ssoAuditFlowStart, ssoReasonProviderUnavailable, decision.Mode, "SSO provider configuration was unavailable")
			writeSSOStartError(r.Context(), w, http.StatusServiceUnavailable, "sso_unavailable")
			return
		}
		authorization, err := h.ssoRelyingParty().Begin(r.Context(), teamConfig)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Warn("Failed to prepare configured SSO provider", "team_id", config.TeamID)
			h.appendSSOAudit(r, "auth.sso_login_failed", "failure", config.TeamID, decision.UserID, email, ssoAuditFlowStart, ssoReasonProviderUnavailable, decision.Mode, "SSO provider configuration was unavailable")
			writeSSOStartError(r.Context(), w, http.StatusServiceUnavailable, "sso_unavailable")
			return
		}
		state := h.ctx.AuthState.CreateSSOOAuthState(shared.SSOOAuthState{
			TeamID:       config.TeamID,
			IssuerURL:    config.IssuerURL,
			ClientID:     config.ClientID,
			Email:        email,
			InviteToken:  strings.TrimSpace(req.InviteToken),
			Nonce:        authorization.FlowState.Nonce,
			CodeVerifier: authorization.FlowState.CodeVerifier,
			ReturnPath:   sanitizeAuthReturnPath(req.ReturnURL),
			RememberMe:   req.RememberMe,
			ExpiresAt:    time.Now().UTC().Add(ssoFlowTTL),
		})

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, api.SSOStartResponse{AuthURL: authorization.URL(state)}); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode SSO start response", "error", err)
		}
	}
}

func (h *handler) handleSSOInviteAvailability() http.HandlerFunc {
	type request struct {
		InviteToken string `json:"invite_token"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}
		var req request
		if err := json.UnmarshalReadStrict(http.MaxBytesReader(w, r.Body, 1<<20), &req); err != nil || strings.TrimSpace(req.InviteToken) == "" {
			writeSSOAvailability(r.Context(), w, false)
			return
		}

		email, err := h.resolveSSORequestEmail(r.Context(), "", req.InviteToken)
		if err != nil {
			writeSSOAvailability(r.Context(), w, false)
			return
		}
		_, domain, _ := sso.NormalizeEmail(email)
		config, err := h.ctx.Store.GetEnabledTeamSSOConfigByDomain(r.Context(), domain)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to resolve invitation SSO provider", "error_kind", ssoErrorKind(err, "provider_lookup_failed"))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if config == nil || !h.ctx.Limits().AllowsSSO(r.Context(), uuid.Nil, config.TeamID) {
			writeSSOAvailability(r.Context(), w, false)
			return
		}
		decision, err := h.authorizeSSOAccess(r.Context(), config, email, req.InviteToken)
		if err != nil {
			if errors.Is(err, errSSOAccessDenied) {
				writeSSOAvailability(r.Context(), w, false)
				return
			}
			shared.LoggerFromContext(r.Context()).Error("Failed to authorize invitation SSO", "error_kind", ssoErrorKind(err, "access_authorization_failed"), "team_id", config.TeamID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		writeSSOAvailability(r.Context(), w, decision.Mode == ssoAccessInvitation)
	}
}

func (h *handler) resolveSSORequestEmail(ctx context.Context, requestedEmail, inviteToken string) (string, error) {
	inviteToken = strings.TrimSpace(inviteToken)
	if inviteToken == "" {
		email, _, err := sso.NormalizeEmail(requestedEmail)
		return email, err
	}

	inviteEmail, err := h.ctx.Store.ResolvePasswordResetEmail(ctx, inviteToken)
	if err != nil {
		return "", errSSOAccessDenied
	}
	inviteEmail, _, err = sso.NormalizeEmail(inviteEmail)
	if err != nil {
		return "", errSSOAccessDenied
	}
	if strings.TrimSpace(requestedEmail) == "" {
		return inviteEmail, nil
	}
	requestedEmail, _, err = sso.NormalizeEmail(requestedEmail)
	if err != nil || !strings.EqualFold(requestedEmail, inviteEmail) {
		return "", errSSOAccessDenied
	}
	return inviteEmail, nil
}

func (h *handler) authorizeSSOAccess(ctx context.Context, config *database.TeamSSOConfig, email, inviteToken string) (ssoAccessDecision, error) {
	if config == nil || config.TeamID == uuid.Nil {
		return ssoAccessDecision{}, errSSOAccessDenied
	}
	if strings.TrimSpace(inviteToken) != "" {
		inviteEmail, err := h.ctx.Store.ResolvePasswordResetEmail(ctx, inviteToken)
		if err != nil || !strings.EqualFold(strings.TrimSpace(inviteEmail), email) {
			return ssoAccessDecision{}, errSSOAccessDenied
		}
	}

	user, err := h.ctx.Store.GetUserByEmail(ctx, email)
	if err != nil {
		return ssoAccessDecision{}, err
	}
	if strings.TrimSpace(inviteToken) == "" && user != nil {
		member, err := h.ctx.Store.IsTenantMember(ctx, config.TeamID, user.ID)
		if err != nil {
			return ssoAccessDecision{}, err
		}
		if member {
			return ssoAccessDecision{Mode: ssoAccessExistingMember, UserID: user.ID}, nil
		}
	}

	invites, err := h.ctx.Store.ListPendingTeamInvitesByEmail(ctx, email)
	if err != nil {
		return ssoAccessDecision{}, err
	}
	now := time.Now().UTC()
	for _, invite := range invites {
		if invite.TeamID == config.TeamID && invite.ExpiresAt.After(now) {
			decision := ssoAccessDecision{Mode: ssoAccessInvitation}
			if user != nil {
				decision.UserID = user.ID
			}
			return decision, nil
		}
	}
	if strings.TrimSpace(inviteToken) != "" {
		return ssoAccessDecision{}, errSSOAccessDenied
	}
	if config.AutoProvision {
		decision := ssoAccessDecision{Mode: ssoAccessAutoProvision}
		if user != nil {
			decision.UserID = user.ID
		}
		return decision, nil
	}
	return ssoAccessDecision{}, errSSOAccessDenied
}

func (h *handler) requireSSOAccessCapacity(ctx context.Context, teamID uuid.UUID, decision ssoAccessDecision) error {
	if h.ctx.Config == nil || !h.ctx.Config.CloudHosted || decision.Mode == ssoAccessExistingMember {
		return nil
	}
	if decision.Mode == ssoAccessAutoProvision {
		if err := h.ctx.Limits().RequireTeamMemberCapacity(ctx, teamID); err != nil {
			return err
		}
	}
	if decision.UserID != uuid.Nil {
		return h.ctx.Limits().RequireTeamMembershipCapacity(ctx, decision.UserID, 1)
	}
	return nil
}

func writeSSOAvailability(ctx context.Context, w http.ResponseWriter, enabled bool) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.MarshalWrite(w, api.SSOAvailability{Enabled: enabled}); err != nil {
		shared.LoggerFromContext(ctx).Error("Failed to encode SSO availability", "error", err)
	}
}

func (h *handler) handleSSOCallback() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil || h.ctx.AuthState == nil {
			http.Redirect(w, r, h.loginErrorRedirectURL("sso_failed"), http.StatusSeeOther)
			return
		}
		state, ok := h.ctx.AuthState.ConsumeSSOOAuthState(r.URL.Query().Get("state"))
		if !ok {
			http.Redirect(w, r, h.loginErrorRedirectURL("sso_failed"), http.StatusSeeOther)
			return
		}
		if !h.ctx.Limits().AllowsSSO(r.Context(), uuid.Nil, state.TeamID) {
			h.appendSSOAudit(r, "auth.sso_login_failed", "failure", state.TeamID, uuid.Nil, state.Email, ssoAuditFlowCallback, ssoReasonEntitlementDenied, 0, "SSO entitlement changed during login")
			http.Redirect(w, r, h.ssoErrorRedirectURL(state, "sso_unavailable"), http.StatusSeeOther)
			return
		}
		if providerError := strings.TrimSpace(r.URL.Query().Get("error")); providerError != "" {
			h.appendSSOAudit(r, "auth.sso_login_failed", "failure", state.TeamID, uuid.Nil, state.Email, ssoAuditFlowCallback, ssoReasonProviderDenied, 0, "SSO provider denied or cancelled login")
			http.Redirect(w, r, h.ssoErrorRedirectURL(state, "sso_provider_error"), http.StatusSeeOther)
			return
		}
		code := strings.TrimSpace(r.URL.Query().Get("code"))
		if code == "" {
			h.appendSSOAudit(r, "auth.sso_login_failed", "failure", state.TeamID, uuid.Nil, state.Email, ssoAuditFlowCallback, ssoReasonAuthorizationMissing, 0, "SSO callback omitted authorization code")
			http.Redirect(w, r, h.ssoErrorRedirectURL(state, "sso_failed"), http.StatusSeeOther)
			return
		}

		config, err := h.ctx.Store.GetTeamSSOConfig(r.Context(), state.TeamID)
		if err != nil || !ssoStateMatchesConfig(state, config) {
			h.appendSSOAudit(r, "auth.sso_login_failed", "failure", state.TeamID, uuid.Nil, state.Email, ssoAuditFlowCallback, ssoReasonConfigurationChanged, 0, "SSO configuration changed during login")
			http.Redirect(w, r, h.ssoErrorRedirectURL(state, "sso_unavailable"), http.StatusSeeOther)
			return
		}
		teamConfig, err := h.teamSSOConfig(config)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Warn("Failed to decrypt SSO provider during callback", "team_id", state.TeamID)
			h.appendSSOAudit(r, "auth.sso_login_failed", "failure", state.TeamID, uuid.Nil, state.Email, ssoAuditFlowCallback, ssoReasonProviderUnavailable, 0, "SSO provider configuration was unavailable")
			http.Redirect(w, r, h.ssoErrorRedirectURL(state, "sso_unavailable"), http.StatusSeeOther)
			return
		}
		identity, err := h.ssoRelyingParty().Complete(r.Context(), teamConfig, sso.FlowState{
			Nonce:        state.Nonce,
			CodeVerifier: state.CodeVerifier,
		}, code)
		if err != nil {
			h.handleSSOCompletionError(w, r, state, err)
			return
		}
		if !strings.EqualFold(identity.Email, state.Email) || !slices.Contains(config.AllowedDomains, identity.Domain) {
			h.appendSSOAudit(r, "auth.sso_login_failed", "failure", config.TeamID, uuid.Nil, identity.Email, ssoAuditFlowCallback, ssoReasonEmailNotAllowed, 0, "SSO email did not match the requested account or allowed domain")
			http.Redirect(w, r, h.ssoErrorRedirectURL(state, "sso_email_not_allowed"), http.StatusSeeOther)
			return
		}
		decision, err := h.authorizeSSOAccess(r.Context(), config, identity.Email, state.InviteToken)
		if err != nil {
			if !errors.Is(err, errSSOAccessDenied) {
				shared.LoggerFromContext(r.Context()).Error("Failed to re-authorize SSO login", "error_kind", ssoErrorKind(err, "access_reauthorization_failed"), "team_id", config.TeamID)
			}
			h.appendSSOAudit(r, "auth.sso_login_failed", "failure", config.TeamID, decision.UserID, identity.Email, ssoAuditFlowCallback, ssoReasonAccessDenied, decision.Mode, "SSO team access was removed during login")
			http.Redirect(w, r, h.ssoErrorRedirectURL(state, "sso_access_denied"), http.StatusSeeOther)
			return
		}
		if err := h.requireSSOAccessCapacity(r.Context(), config.TeamID, decision); err != nil {
			h.appendSSOAudit(r, "auth.sso_login_failed", "failure", config.TeamID, decision.UserID, identity.Email, ssoAuditFlowCallback, ssoReasonCapacityDenied, decision.Mode, "SSO login exceeded a managed-cloud team or seat limit")
			http.Redirect(w, r, h.ssoErrorRedirectURL(state, "sso_access_denied"), http.StatusSeeOther)
			return
		}

		randomPassword, err := security.GenerateRandomChallenge(48)
		if err != nil {
			h.appendSSOAudit(r, "auth.sso_login_failed", "failure", config.TeamID, decision.UserID, identity.Email, ssoAuditFlowCallback, ssoReasonInternalError, decision.Mode, "SSO login could not create a local credential")
			http.Redirect(w, r, h.ssoErrorRedirectURL(state, "sso_failed"), http.StatusSeeOther)
			return
		}
		passwordHash, err := HashPassword(randomPassword)
		if err != nil {
			h.appendSSOAudit(r, "auth.sso_login_failed", "failure", config.TeamID, decision.UserID, identity.Email, ssoAuditFlowCallback, ssoReasonInternalError, decision.Mode, "SSO login could not create a local credential")
			http.Redirect(w, r, h.ssoErrorRedirectURL(state, "sso_failed"), http.StatusSeeOther)
			return
		}
		givenName, lastName := splitSSODisplayName(identity.DisplayName)
		resolved, err := h.ctx.Store.ResolveSSOUser(r.Context(), database.ResolveSSOUserInput{
			TeamID:         config.TeamID,
			IssuerURL:      config.IssuerURL,
			Subject:        identity.Subject,
			Email:          identity.Email,
			GivenName:      givenName,
			LastName:       lastName,
			PasswordHash:   passwordHash,
			ExpectedUserID: decision.UserID,
		})
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to resolve SSO user", "error_kind", ssoErrorKind(err, "identity_resolution_failed"), "team_id", config.TeamID)
			h.appendSSOAudit(r, "auth.sso_login_failed", "failure", config.TeamID, decision.UserID, identity.Email, ssoAuditFlowCallback, ssoReasonIdentityLinkFailed, decision.Mode, "SSO identity could not be linked")
			http.Redirect(w, r, h.ssoErrorRedirectURL(state, "sso_failed"), http.StatusSeeOther)
			return
		}
		if decision.UserID != uuid.Nil && decision.UserID != resolved.UserID {
			h.appendSSOAudit(r, "auth.sso_login_failed", "failure", config.TeamID, resolved.UserID, identity.Email, ssoAuditFlowCallback, ssoReasonAccessDenied, decision.Mode, "SSO identity resolved to a different authorized user")
			http.Redirect(w, r, h.ssoErrorRedirectURL(state, "sso_access_denied"), http.StatusSeeOther)
			return
		}
		if err := h.completeSSOTeamAccess(r, config, decision, resolved.UserID, identity.Email, state.InviteToken); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to complete SSO team access", "error_kind", ssoErrorKind(err, "membership_update_failed"), "team_id", config.TeamID, "user_id", resolved.UserID)
			h.appendSSOAudit(r, "auth.sso_login_failed", "failure", config.TeamID, resolved.UserID, identity.Email, ssoAuditFlowCallback, ssoReasonMembershipFailed, decision.Mode, "SSO team access could not be completed")
			http.Redirect(w, r, h.ssoErrorRedirectURL(state, "sso_access_denied"), http.StatusSeeOther)
			return
		}
		if err := h.issueLoginSession(r.Context(), w, resolved.UserID, state.RememberMe); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to issue SSO login session", "error_kind", ssoErrorKind(err, "session_issue_failed"), "user_id", resolved.UserID)
			h.appendSSOAudit(r, "auth.sso_login_failed", "failure", config.TeamID, resolved.UserID, identity.Email, ssoAuditFlowCallback, ssoReasonSessionFailed, decision.Mode, "SSO login session could not be issued")
			http.Redirect(w, r, h.ssoErrorRedirectURL(state, "sso_failed"), http.StatusSeeOther)
			return
		}
		details := "SSO login succeeded"
		if resolved.Created {
			details = "SSO login succeeded and created a user"
		}
		h.appendSSOAuditForUserTeams(r, resolved.UserID, "auth.sso_login_succeeded", "success", identity.Email, config.TeamID, ssoAuditFlowCallback, ssoReasonLoginSucceeded, decision.Mode, details)
		http.Redirect(w, r, h.publicRedirectURL(state.ReturnPath), http.StatusSeeOther)
	}
}

func (h *handler) completeSSOTeamAccess(r *http.Request, config *database.TeamSSOConfig, decision ssoAccessDecision, userID uuid.UUID, email, inviteToken string) error {
	switch decision.Mode {
	case ssoAccessExistingMember:
		return h.ctx.Store.SetActiveTenantID(r.Context(), userID, config.TeamID)
	case ssoAccessInvitation:
		if err := h.requireSSOAccessCapacity(r.Context(), config.TeamID, ssoAccessDecision{Mode: decision.Mode, UserID: userID}); err != nil {
			return err
		}
		invite, err := h.ctx.Store.AcceptTeamInviteForSSO(r.Context(), inviteToken, config.TeamID, userID)
		if err != nil {
			return err
		}
		h.appendInviteAcceptedAuditEvents(r, userID, email, []api.TeamInvite{invite})
		return nil
	case ssoAccessAutoProvision:
		member, err := h.ctx.Store.IsTenantMember(r.Context(), config.TeamID, userID)
		if err != nil {
			return err
		}
		if member {
			return h.ctx.Store.SetActiveTenantID(r.Context(), userID, config.TeamID)
		}
		if err := h.requireSSOAccessCapacity(r.Context(), config.TeamID, ssoAccessDecision{Mode: decision.Mode, UserID: userID}); err != nil {
			return err
		}
		if err := h.ctx.Store.AddTeamMember(r.Context(), config.TeamID, userID, database.TenantRoleMember, uuid.Nil); err != nil {
			return err
		}
		if err := h.ctx.Store.SetActiveTenantID(r.Context(), userID, config.TeamID); err != nil {
			return err
		}
		h.ctx.AppendAuditEvent(r.Context(), r, shared.AuditEvent{
			ActorID:      userID,
			TeamID:       config.TeamID,
			TargetUserID: userID,
			Action:       "member.added",
			TargetType:   "user",
			TargetID:     userID.String(),
			TargetLabel:  email,
			Outcome:      "success",
			Details:      "Member automatically provisioned through SSO",
		})
		h.ctx.EmitWebhookEvent(r.Context(), webhooks.Event{
			Type: webhooks.EventTeamMemberAdded,
			Data: map[string]any{
				"team_id": config.TeamID.String(),
				"user_id": userID.String(),
			},
		})
		return nil
	default:
		return errSSOAccessDenied
	}
}

func (h *handler) ssoErrorRedirectURL(state shared.SSOOAuthState, code string) string {
	if strings.TrimSpace(state.InviteToken) == "" {
		return h.loginErrorRedirectURL(code)
	}
	query := url.Values{
		"error": {code},
		"token": {state.InviteToken},
	}
	return h.publicRedirectURL("/accept-invite?" + query.Encode())
}

func (h *handler) teamSSOConfig(config *database.TeamSSOConfig) (sso.TeamConfig, error) {
	if config == nil || h.ctx.Config == nil {
		return sso.TeamConfig{}, errors.New("SSO configuration is unavailable")
	}
	box, err := sso.NewSecretBox(h.ctx.Config.JWTSecret)
	if err != nil {
		return sso.TeamConfig{}, err
	}
	clientSecret, err := box.Open(config.ClientSecretEncrypted)
	if err != nil {
		return sso.TeamConfig{}, err
	}
	return sso.TeamConfig{
		IssuerURL:        config.IssuerURL,
		ClientID:         config.ClientID,
		ClientSecret:     clientSecret,
		RedirectURL:      appurl.Path(h.ctx.Config.PublicURL, "/api/auth/sso/callback"),
		EmailClaim:       config.EmailClaim,
		DisplayNameClaim: config.DisplayNameClaim,
	}, nil
}

func ssoStateMatchesConfig(state shared.SSOOAuthState, config *database.TeamSSOConfig) bool {
	return config != nil && config.Enabled && state.TeamID == config.TeamID && subtle.ConstantTimeCompare([]byte(state.IssuerURL), []byte(config.IssuerURL)) == 1 && subtle.ConstantTimeCompare([]byte(state.ClientID), []byte(config.ClientID)) == 1
}

func splitSSODisplayName(displayName string) (string, string) {
	parts := strings.Fields(displayName)
	if len(parts) == 0 {
		return "", ""
	}
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], strings.Join(parts[1:], " ")
}

func writeSSOStartError(ctx context.Context, w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.MarshalWrite(w, map[string]string{
		"status":  "error",
		"code":    code,
		"message": "SSO login could not be started for this email address",
	}); err != nil {
		shared.LoggerFromContext(ctx).Error("Failed to encode SSO start error response", "error", err)
	}
}

func (h *handler) handleSSOCompletionError(w http.ResponseWriter, r *http.Request, state shared.SSOOAuthState, err error) {
	redirectCode := "sso_failed"
	reason := ssoReasonInternalError
	switch {
	case errors.Is(err, sso.ErrTokenExchange):
		shared.LoggerFromContext(r.Context()).Warn("SSO token exchange failed", "team_id", state.TeamID)
		reason = ssoReasonTokenExchangeFailed
	case errors.Is(err, sso.ErrIDTokenMissing):
		reason = ssoReasonIDTokenMissing
	case errors.Is(err, sso.ErrIDTokenValidation):
		reason = ssoReasonIDTokenInvalid
	case errors.Is(err, sso.ErrIdentityClaims):
		reason = ssoReasonEmailUnverified
		redirectCode = "sso_email_unverified"
	default:
		shared.LoggerFromContext(r.Context()).Warn("Failed to prepare SSO provider during callback", "team_id", state.TeamID)
		redirectCode = "sso_unavailable"
	}
	h.appendSSOAudit(r, "auth.sso_login_failed", "failure", state.TeamID, uuid.Nil, state.Email, ssoAuditFlowCallback, reason, 0, "SSO provider completion failed")
	http.Redirect(w, r, h.ssoErrorRedirectURL(state, redirectCode), http.StatusSeeOther)
}

func (h *handler) ssoRelyingParty() *sso.RelyingParty {
	return sso.NewRelyingParty(h.ssoClient())
}

func ssoErrorKind(err error, fallback string) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	default:
		return fallback
	}
}

func (h *handler) ssoClient() *sso.Client {
	if h.ctx.SSO != nil {
		return h.ctx.SSO
	}
	return sso.NewRuntimeClient(h.ctx.Config != nil && h.ctx.Config.CloudHosted)
}
