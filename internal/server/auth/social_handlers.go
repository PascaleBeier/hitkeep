package auth

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/appurl"
	"hitkeep/internal/controlstore"
	"hitkeep/internal/database"
	"hitkeep/internal/localization"
	"hitkeep/internal/mailables"
	"hitkeep/internal/mailer"
	"hitkeep/internal/security"
	"hitkeep/internal/server/shared"
	"hitkeep/internal/socialauth"
	"hitkeep/internal/sso"
)

const socialFlowTTL = 10 * time.Minute
const socialCompletionTTL = 5 * time.Minute
const socialStateCookiePrefix = "hk_social_state_"
const socialStateCookieRoute = "/api/auth/social/"

type socialStartRequest struct {
	Flow        string `json:"flow"`
	InviteToken string `json:"invite_token,omitempty"`
	ReturnURL   string `json:"return_url,omitempty"`
	RememberMe  bool   `json:"remember_me,omitempty"`
}

type socialCompletionRequest struct {
	CompletionToken string `json:"completion_token"`
	Email           string `json:"email,omitempty"`
}

type socialPreviewResponse struct {
	Provider                  string `json:"provider"`
	DisplayName               string `json:"display_name"`
	ObservedEmail             string `json:"observed_email,omitempty"`
	EmailVerified             bool   `json:"email_verified"`
	EmailConfirmationRequired bool   `json:"email_confirmation_required"`
	Flow                      string `json:"flow"`
}

type socialCompleteResponse struct {
	Status          string   `json:"status"`
	RedirectURL     string   `json:"redirect_url,omitempty"`
	CompletionToken string   `json:"completion_token,omitempty"`
	ChallengeToken  string   `json:"challenge_token,omitempty"`
	Factors         []string `json:"factors,omitempty"`
	Passkey         any      `json:"passkey,omitempty"`
}

type socialCloudSignupRequest struct {
	CompletionToken string `json:"completion_token"`
	Email           string `json:"email,omitempty"`
	TeamName        string `json:"team_name"`
	PlanCode        string `json:"plan_code"`
	BillingInterval string `json:"billing"`
	Jurisdiction    string `json:"jurisdiction"`
	Locale          string `json:"locale"`
	AcceptedTOS     bool   `json:"accepted_tos"`
}

func (h *handler) handleSocialProviders() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		providers := make([]api.SocialProvider, 0, 3)
		for _, status := range socialauth.ProviderStatuses(h.ctx.Config) {
			if status.Configured {
				providers = append(providers, api.SocialProvider{ID: status.Provider, DisplayName: status.DisplayName})
			}
		}
		writeSocialJSON(w, http.StatusOK, api.SocialProvidersResponse{
			Providers: providers, SignupEnabled: h.socialSignupEnabled(),
		})
	}
}

func (h *handler) handleSocialStart(linkFlow bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil || h.ctx.AuthState == nil || h.ctx.SocialAuth == nil {
			writeSocialError(w, http.StatusServiceUnavailable, "social_unavailable")
			return
		}
		providerName := strings.ToLower(strings.TrimSpace(r.PathValue("provider")))
		providerConfig, err := socialauth.ConfigForProvider(h.ctx.Config, providerName)
		if err != nil {
			writeSocialError(w, http.StatusNotFound, "social_provider_unavailable")
			return
		}

		var req socialStartRequest
		if !decodeSocialJSON(w, r, &req, true) {
			return
		}
		flow := strings.ToLower(strings.TrimSpace(req.Flow))
		userID := uuid.Nil
		if linkFlow {
			flow = "link"
			userID = shared.GetUserIDFromContext(r)
			if userID == uuid.Nil {
				writeSocialError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
		} else {
			switch flow {
			case "login":
			case "signup":
				if !h.socialSignupEnabled() {
					writeSocialError(w, http.StatusNotFound, "social_signup_disabled")
					return
				}
			case "invite":
				if strings.TrimSpace(req.InviteToken) == "" {
					writeSocialError(w, http.StatusBadRequest, "invite_token_required")
					return
				}
				if _, err := h.ctx.Store.ResolvePasswordResetEmail(r.Context(), req.InviteToken); err != nil {
					writeSocialError(w, http.StatusForbidden, "invite_invalid")
					return
				}
			default:
				writeSocialError(w, http.StatusBadRequest, "social_flow_invalid")
				return
			}
		}

		authorization, err := h.ctx.SocialAuth.Begin(r.Context(), providerConfig)
		if err != nil {
			slog.Warn("Social provider authorization could not be prepared", "provider", providerName, "category", socialErrorCategory(err))
			writeSocialError(w, http.StatusServiceUnavailable, "social_provider_unavailable")
			return
		}
		state := h.ctx.AuthState.CreateSocialOAuthState(shared.SocialOAuthState{
			Provider: providerName, Flow: flow, InviteToken: strings.TrimSpace(req.InviteToken),
			ReturnPath: sanitizeAuthReturnPath(req.ReturnURL), RememberMe: req.RememberMe, UserID: userID,
			Nonce: authorization.FlowState.Nonce, CodeVerifier: authorization.FlowState.CodeVerifier,
			ExpiresAt: time.Now().UTC().Add(socialFlowTTL),
		})
		h.setSocialStateCookie(w, state)
		writeSocialJSON(w, http.StatusOK, map[string]string{"auth_url": authorization.URL(state)})
	}
}

func (h *handler) handleSocialCallback() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		providerName := strings.ToLower(strings.TrimSpace(r.PathValue("provider")))
		if h.ctx.AuthState == nil || h.ctx.SocialAuth == nil {
			http.Redirect(w, r, h.loginErrorRedirectURL("social_unavailable"), http.StatusSeeOther)
			return
		}
		rawState := r.URL.Query().Get("state")
		if !h.hasSocialStateCookie(r, rawState) {
			http.Redirect(w, r, h.loginErrorRedirectURL("social_state_invalid"), http.StatusSeeOther)
			return
		}
		h.clearSocialStateCookie(w, rawState)
		state, ok := h.ctx.AuthState.ConsumeSocialOAuthState(rawState)
		if !ok || state.Provider != providerName {
			http.Redirect(w, r, h.loginErrorRedirectURL("social_state_invalid"), http.StatusSeeOther)
			return
		}
		if strings.TrimSpace(r.URL.Query().Get("error")) != "" {
			h.appendAuthAuditSystem(r, "auth.social_login_failed", "failure", providerName, "provider="+providerName+";reason=cancelled")
			http.Redirect(w, r, h.socialErrorRedirectURL(state, "social_provider_cancelled"), http.StatusSeeOther)
			return
		}
		providerConfig, err := socialauth.ConfigForProvider(h.ctx.Config, providerName)
		if err != nil {
			http.Redirect(w, r, h.socialErrorRedirectURL(state, "social_provider_unavailable"), http.StatusSeeOther)
			return
		}
		identity, err := h.ctx.SocialAuth.Complete(r.Context(), providerConfig, socialauth.FlowState{
			Nonce: state.Nonce, CodeVerifier: state.CodeVerifier,
		}, r.URL.Query().Get("code"))
		if err != nil {
			slog.Warn("Social provider callback failed", "provider", providerName, "category", socialErrorCategory(err))
			h.appendAuthAuditSystem(r, "auth.social_login_failed", "failure", providerName, "provider="+providerName+";reason="+socialErrorCategory(err))
			http.Redirect(w, r, h.socialErrorRedirectURL(state, socialCallbackErrorCode(err)), http.StatusSeeOther)
			return
		}

		if state.Flow == "link" {
			if err := h.completeAuthenticatedSocialLink(r, state.UserID, identity); err != nil {
				category := socialDatabaseErrorCategory(err)
				slog.Warn("Social identity link failed", "provider", providerName, "category", category, "user_id", state.UserID)
				h.appendAuthAuditForUserTeams(r, state.UserID, "auth.social_identity_link_failed", "failure", "provider="+providerName+";reason="+category, true)
				http.Redirect(w, r, h.socialErrorRedirectURL(state, "social_identity_conflict"), http.StatusSeeOther)
				return
			}
			h.appendAuthAuditForUserTeams(r, state.UserID, "auth.social_identity_linked", "success", "provider="+providerName+";reason=authenticated_session", true)
			http.Redirect(w, r, appurl.Path(h.ctx.Config.PublicURL, "/settings?tab=security&social=linked"), http.StatusSeeOther)
			return
		}

		completionToken := h.ctx.AuthState.CreateSocialCompletion(shared.SocialCompletion{
			Provider: identity.Provider, Subject: identity.Subject, ObservedEmail: identity.Email,
			EmailVerified: identity.EmailVerified, Flow: state.Flow,
			InviteToken: state.InviteToken, ReturnPath: state.ReturnPath, RememberMe: state.RememberMe,
			ExpiresAt: time.Now().UTC().Add(socialCompletionTTL),
		})
		//nolint:gosec // The redirect is constructed from the configured public URL and allowlisted local flow paths.
		http.Redirect(w, r, h.socialCompletionRedirectURL(state, completionToken), http.StatusSeeOther)
	}
}

func socialStateCookieName(rawState string) (string, string, bool) {
	stateID, err := uuid.Parse(strings.TrimSpace(rawState))
	if err != nil || stateID == uuid.Nil {
		return "", "", false
	}
	canonical := stateID.String()
	return socialStateCookiePrefix + canonical, canonical, true
}

func (h *handler) setSocialStateCookie(w http.ResponseWriter, rawState string) {
	name, canonical, ok := socialStateCookieName(rawState)
	if !ok {
		return
	}
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure follows the configured public URL; local HTTP development intentionally uses an insecure cookie.
		Name: name, Value: canonical, Path: h.socialStateCookiePath(),
		Expires: time.Now().UTC().Add(socialFlowTTL), MaxAge: int(socialFlowTTL / time.Second),
		HttpOnly: true, Secure: strings.HasPrefix(strings.ToLower(strings.TrimSpace(h.ctx.Config.PublicURL)), "https://"),
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *handler) hasSocialStateCookie(r *http.Request, rawState string) bool {
	name, canonical, ok := socialStateCookieName(rawState)
	if !ok {
		return false
	}
	cookie, err := r.Cookie(name)
	return err == nil && subtle.ConstantTimeCompare([]byte(strings.TrimSpace(cookie.Value)), []byte(canonical)) == 1
}

func (h *handler) clearSocialStateCookie(w http.ResponseWriter, rawState string) {
	name, _, ok := socialStateCookieName(rawState)
	if !ok {
		return
	}
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure follows the configured public URL; local HTTP development intentionally uses an insecure cookie.
		Name: name, Value: "", Path: h.socialStateCookiePath(), Expires: time.Unix(0, 0), MaxAge: -1,
		HttpOnly: true, Secure: strings.HasPrefix(strings.ToLower(strings.TrimSpace(h.ctx.Config.PublicURL)), "https://"),
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *handler) socialStateCookiePath() string {
	if h == nil || h.ctx == nil || h.ctx.Config == nil {
		return socialStateCookieRoute
	}
	publicURL, err := url.Parse(strings.TrimSpace(h.ctx.Config.PublicURL))
	if err != nil {
		return socialStateCookieRoute
	}
	prefix := strings.TrimRight(publicURL.EscapedPath(), "/")
	if prefix == "" {
		return socialStateCookieRoute
	}
	return prefix + socialStateCookieRoute
}

func (h *handler) completeAuthenticatedSocialLink(r *http.Request, userID uuid.UUID, identity socialauth.Identity) error {
	if userID == uuid.Nil {
		return errors.New("authenticated user is required")
	}
	if identity.EmailVerified && identity.Email != "" {
		matched, err := h.ctx.Store.GetUserByEmail(r.Context(), identity.Email)
		if err != nil {
			return err
		}
		if matched != nil && matched.ID != userID {
			return controlstore.ErrSocialIdentityConflict
		}
	}
	_, err := h.ctx.Store.LinkSocialIdentity(r.Context(), database.LinkSocialIdentityInput{
		UserID: userID, Provider: identity.Provider, Subject: identity.Subject, ObservedEmail: identity.Email,
	})
	return err
}

func (h *handler) handleSocialPreview() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx == nil || h.ctx.AuthState == nil || h.ctx.Store == nil {
			writeSocialError(w, http.StatusServiceUnavailable, "social_unavailable")
			return
		}
		var req socialCompletionRequest
		if !decodeSocialJSON(w, r, &req, false) {
			return
		}
		completion, ok := h.ctx.AuthState.GetSocialCompletion(req.CompletionToken)
		if !ok {
			writeSocialError(w, http.StatusConflict, "social_completion_invalid")
			return
		}
		config, err := socialauth.ConfigForProvider(h.ctx.Config, completion.Provider)
		if err != nil {
			writeSocialError(w, http.StatusServiceUnavailable, "social_provider_unavailable")
			return
		}
		emailConfirmationRequired := false
		if completion.Provider == socialauth.ProviderMicrosoft {
			identity, err := h.ctx.Store.GetSocialIdentity(r.Context(), completion.Provider, completion.Subject)
			if err != nil {
				writeSocialError(w, http.StatusInternalServerError, "social_login_failed")
				return
			}
			emailConfirmationRequired = identity == nil
		}
		writeSocialJSON(w, http.StatusOK, socialPreviewResponse{
			Provider: completion.Provider, DisplayName: config.DisplayName, ObservedEmail: completion.ObservedEmail,
			EmailVerified: completion.EmailVerified, EmailConfirmationRequired: emailConfirmationRequired, Flow: completion.Flow,
		})
	}
}

func (h *handler) handleSocialComplete() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx == nil || h.ctx.AuthState == nil || h.ctx.Store == nil {
			writeSocialError(w, http.StatusServiceUnavailable, "social_unavailable")
			return
		}
		var req socialCompletionRequest
		if !decodeSocialJSON(w, r, &req, false) {
			return
		}
		completion, ok := h.ctx.AuthState.ConsumeSocialCompletion(req.CompletionToken)
		if !ok {
			writeSocialError(w, http.StatusConflict, "social_completion_invalid")
			return
		}
		switch completion.Flow {
		case "invite":
			h.completeSocialInvite(w, r, completion)
		case "login":
			h.completeSocialLogin(w, r, req, completion)
		default:
			writeSocialError(w, http.StatusBadRequest, "social_flow_invalid")
		}
	}
}

func (h *handler) completeSocialLogin(w http.ResponseWriter, r *http.Request, req socialCompletionRequest, completion shared.SocialCompletion) {
	user, identity, err := h.ctx.Store.GetUserBySocialIdentity(r.Context(), completion.Provider, completion.Subject)
	if err != nil {
		writeSocialError(w, http.StatusInternalServerError, "social_login_failed")
		return
	}
	if identity == nil && completion.EmailVerified && completion.Provider != socialauth.ProviderMicrosoft {
		user, err = h.ctx.Store.GetUserByEmail(r.Context(), completion.ObservedEmail)
		if err != nil {
			writeSocialError(w, http.StatusInternalServerError, "social_login_failed")
			return
		}
		if user != nil {
			identity, err = h.ctx.Store.LinkSocialIdentity(r.Context(), database.LinkSocialIdentityInput{
				UserID: user.ID, Provider: completion.Provider, Subject: completion.Subject,
				ObservedEmail: completion.ObservedEmail, MarkUsed: true,
			})
			if err != nil {
				h.appendAuthAuditSystem(r, "auth.social_login_failed", "failure", completion.Provider, "provider="+completion.Provider+";reason=identity_conflict")
				writeSocialError(w, http.StatusConflict, "social_identity_conflict")
				return
			}
			h.appendAuthAuditForUserTeams(r, user.ID, "auth.social_identity_linked", "success", "provider="+completion.Provider+";reason=verified_email_match", true)
		}
	}
	if identity != nil && user != nil {
		_, err = h.ctx.Store.LinkSocialIdentity(r.Context(), database.LinkSocialIdentityInput{
			UserID: user.ID, Provider: completion.Provider, Subject: completion.Subject,
			ObservedEmail: completion.ObservedEmail, MarkUsed: true,
		})
		if err != nil {
			writeSocialError(w, http.StatusConflict, "social_identity_conflict")
			return
		}
		h.writeCompletedSocialLogin(w, r, user.ID, completion)
		return
	}

	if completion.Provider == socialauth.ProviderMicrosoft {
		targetEmail := strings.TrimSpace(req.Email)
		if targetEmail == "" {
			targetEmail = completion.ObservedEmail
		}
		targetEmail, _, err = sso.NormalizeEmail(targetEmail)
		if err != nil {
			writeSocialError(w, http.StatusBadRequest, "social_email_required")
			return
		}
		targetUser, err := h.ctx.Store.GetUserByEmail(r.Context(), targetEmail)
		if err != nil {
			writeSocialError(w, http.StatusInternalServerError, "social_login_failed")
			return
		}
		if targetUser == nil {
			if h.socialSignupEnabled() {
				writeSocialJSON(w, http.StatusOK, h.socialSignupRequiredResponse(completion))
				return
			}
			h.appendAuthAuditSystem(r, "auth.social_login_failed", "failure", completion.Provider, "provider="+completion.Provider+";reason=account_not_found")
			writeSocialError(w, http.StatusForbidden, "social_account_not_found")
			return
		}
		if err := h.sendSocialConfirmation(r, completion, targetEmail, &targetUser.ID, socialCloudSignupRequest{}); err != nil {
			writeSocialError(w, http.StatusBadGateway, "social_confirmation_failed")
			return
		}
		writeSocialJSON(w, http.StatusOK, socialCompleteResponse{Status: "verification_sent"})
		return
	}

	if h.socialSignupEnabled() {
		writeSocialJSON(w, http.StatusOK, h.socialSignupRequiredResponse(completion))
		return
	}
	writeSocialError(w, http.StatusForbidden, "social_account_not_found")
	h.appendAuthAuditSystem(r, "auth.social_login_failed", "failure", completion.Provider, "provider="+completion.Provider+";reason=account_not_found")
}

func (h *handler) completeSocialInvite(w http.ResponseWriter, r *http.Request, completion shared.SocialCompletion) {
	email, err := h.ctx.Store.ResolvePasswordResetEmail(r.Context(), completion.InviteToken)
	if err != nil {
		writeSocialError(w, http.StatusBadRequest, "invite_invalid")
		return
	}
	if completion.Provider != socialauth.ProviderMicrosoft {
		providerEmail, _, normalizeErr := sso.NormalizeEmail(completion.ObservedEmail)
		if normalizeErr != nil || !completion.EmailVerified || !strings.EqualFold(providerEmail, email) {
			h.appendAuthAuditSystem(r, "auth.social_login_failed", "failure", completion.Provider, "provider="+completion.Provider+";reason=invite_email_mismatch")
			writeSocialError(w, http.StatusForbidden, "social_invite_email_mismatch")
			return
		}
	}
	user, err := h.ctx.Store.GetUserByEmail(r.Context(), email)
	if err != nil || user == nil {
		writeSocialError(w, http.StatusBadRequest, "invite_invalid")
		return
	}
	pendingInvites, err := h.ctx.Store.ListPendingTeamInvitesByEmail(r.Context(), email)
	if err != nil || len(pendingInvites) == 0 {
		writeSocialError(w, http.StatusBadRequest, "invite_invalid")
		return
	}
	if err := h.validateCloudInviteAcceptance(r.Context(), email, user.ID, pendingInvites); err != nil {
		h.writeInviteAcceptanceError(w, err, email, user.ID)
		return
	}
	if _, err := h.ctx.Store.LinkSocialIdentity(r.Context(), database.LinkSocialIdentityInput{
		UserID: user.ID, Provider: completion.Provider, Subject: completion.Subject,
		ObservedEmail: completion.ObservedEmail, MarkUsed: true,
	}); err != nil {
		writeSocialError(w, http.StatusConflict, "social_identity_conflict")
		return
	}
	h.appendAuthAuditForUserTeams(r, user.ID, "auth.social_identity_linked", "success", "provider="+completion.Provider+";reason=valid_invite", true)
	acceptedEmail, acceptedInvites, err := h.ctx.Store.AcceptInviteForAuthenticatedUser(r.Context(), completion.InviteToken, user.ID)
	if err != nil {
		h.writeInviteAcceptanceError(w, err, email, user.ID)
		return
	}
	h.appendInviteAcceptedAuditEvents(r, user.ID, acceptedEmail, acceptedInvites)
	h.writeCompletedSocialLogin(w, r, user.ID, completion)
}

func (h *handler) writeCompletedSocialLogin(w http.ResponseWriter, r *http.Request, userID uuid.UUID, completion shared.SocialCompletion) {
	response, err := h.beginUserLogin(r, w, userID, completion.RememberMe, completion.Provider)
	if err != nil {
		slog.Error("Failed to complete social login", "error", err, "provider", completion.Provider, "user_id", userID)
		writeSocialError(w, http.StatusInternalServerError, "social_login_failed")
		return
	}
	if response.Status == "mfa_required" {
		h.appendAuthAuditForUserTeams(r, userID, "auth.mfa_required", "success", "provider="+completion.Provider+";reason=local_mfa_enabled", true)
	} else {
		h.appendAuthAuditForUserTeams(r, userID, "auth.social_login_succeeded", "success", "provider="+completion.Provider+";reason=success", true)
	}
	writeSocialJSON(w, http.StatusOK, socialCompleteResponse{
		Status: response.Status, RedirectURL: completion.ReturnPath, ChallengeToken: response.ChallengeToken,
		Factors: response.Factors, Passkey: response.Passkey,
	})
}

func (h *handler) handleSocialCloudSignupComplete() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx == nil || h.ctx.AuthState == nil || h.ctx.Store == nil {
			writeSocialError(w, http.StatusServiceUnavailable, "social_unavailable")
			return
		}
		if !h.socialSignupEnabled() {
			writeSocialError(w, http.StatusNotFound, "social_signup_disabled")
			return
		}
		var req socialCloudSignupRequest
		if !decodeSocialJSON(w, r, &req, false) {
			return
		}
		completion, ok := h.ctx.AuthState.GetSocialCompletion(req.CompletionToken)
		if !ok || (completion.Flow != "signup" && completion.Flow != "login") {
			writeSocialError(w, http.StatusConflict, "social_completion_invalid")
			return
		}

		req.TeamName = strings.TrimSpace(req.TeamName)
		req.Locale = mailer.NormalizeLocale(req.Locale)
		req.PlanCode = normalizeSocialPlan(req.PlanCode)
		req.BillingInterval = normalizeSocialBilling(req.BillingInterval)
		req.Jurisdiction = strings.ToUpper(strings.TrimSpace(req.Jurisdiction))
		if req.TeamName == "" {
			req.TeamName = localization.DefaultTeamName(req.Locale, "")
		}
		if !req.AcceptedTOS || req.TeamName == "" || req.PlanCode == "" || req.BillingInterval == "" || (req.Jurisdiction != "EU" && req.Jurisdiction != "US") {
			writeSocialError(w, http.StatusBadRequest, "social_signup_invalid")
			return
		}
		configuredJurisdiction := strings.ToUpper(strings.TrimSpace(h.ctx.Config.CloudJurisdiction))
		if configuredJurisdiction != "" && req.Jurisdiction != "" && configuredJurisdiction != req.Jurisdiction {
			writeSocialError(w, http.StatusBadRequest, "jurisdiction_mismatch")
			return
		}

		targetEmail := completion.ObservedEmail
		if completion.Provider == socialauth.ProviderMicrosoft {
			targetEmail = req.Email
		}
		var err error
		targetEmail, _, err = sso.NormalizeEmail(targetEmail)
		if err != nil {
			writeSocialError(w, http.StatusBadRequest, "social_email_required")
			return
		}
		existing, err := h.ctx.Store.GetUserByEmail(r.Context(), targetEmail)
		if err != nil {
			writeSocialError(w, http.StatusInternalServerError, "social_signup_failed")
			return
		}
		if existing != nil {
			writeSocialError(w, http.StatusConflict, "social_account_exists")
			return
		}
		consumed, ok := h.ctx.AuthState.ConsumeSocialCompletion(req.CompletionToken)
		if !ok {
			writeSocialError(w, http.StatusConflict, "social_completion_invalid")
			return
		}
		completion = consumed

		if completion.Provider == socialauth.ProviderMicrosoft {
			if err := h.sendSocialConfirmation(r, completion, targetEmail, nil, req); err != nil {
				writeSocialError(w, http.StatusBadGateway, "social_confirmation_failed")
				return
			}
			writeSocialJSON(w, http.StatusCreated, map[string]any{
				"status": "verification_sent", "plan_code": req.PlanCode, "billing": req.BillingInterval,
			})
			return
		}

		account, err := h.createSocialCloudAccount(r, completion, targetEmail, req)
		if err != nil {
			status := http.StatusInternalServerError
			code := "social_signup_failed"
			if errors.Is(err, controlstore.ErrUserEmailAlreadyExists) || errors.Is(err, controlstore.ErrSocialIdentityConflict) {
				status = http.StatusConflict
				code = "social_account_exists"
			}
			writeSocialError(w, status, code)
			return
		}
		if err := h.issueLoginSession(r.Context(), w, account.UserID, false); err != nil {
			writeSocialError(w, http.StatusInternalServerError, "social_signup_failed")
			return
		}
		h.appendAuthAuditForUserTeams(r, account.UserID, "auth.social_signup_succeeded", "success", "provider="+completion.Provider+";reason=verified_provider_email", true)
		writeSocialJSON(w, http.StatusCreated, map[string]any{
			"status": "ok", "plan_code": req.PlanCode, "billing": req.BillingInterval,
			"redirect_url": socialSignupRedirect(req.PlanCode, req.BillingInterval),
		})
	}
}

func (h *handler) sendSocialConfirmation(r *http.Request, completion shared.SocialCompletion, targetEmail string, targetUserID *uuid.UUID, signup socialCloudSignupRequest) error {
	if h.ctx.Mailer == nil {
		return errors.New("mailer is unavailable")
	}
	entry := database.PendingSocialConfirmation{
		Provider: completion.Provider, Subject: completion.Subject, ObservedEmail: completion.ObservedEmail,
		TargetEmail: targetEmail, TargetUserID: targetUserID, TeamName: signup.TeamName,
		Jurisdiction: signup.Jurisdiction, Locale: signup.Locale, PlanCode: signup.PlanCode,
		BillingInterval: signup.BillingInterval, ReturnPath: completion.ReturnPath,
		RememberMe: completion.RememberMe,
	}
	if signup.AcceptedTOS {
		entry.AcceptedTosAt = time.Now().UTC()
	}
	token, err := h.ctx.Store.CreatePendingSocialConfirmation(r.Context(), entry)
	if err != nil {
		return err
	}
	locale := signup.Locale
	if targetUserID != nil {
		locale = h.preferredMailLocale(r, *targetUserID)
	}
	confirmURL := appurl.Path(h.ctx.Config.PublicURL, "/api/auth/social/confirm?token="+url.QueryEscape(token))
	if err := h.ctx.Mailer.Send(targetEmail, mailables.NewSocialConfirmation(confirmURL, providerDisplayName(completion.Provider), locale, 30)); err != nil {
		return err
	}
	return nil
}

func providerDisplayName(provider string) string {
	switch provider {
	case socialauth.ProviderGoogle:
		return "Google"
	case socialauth.ProviderGitHub:
		return "GitHub"
	case socialauth.ProviderMicrosoft:
		return "Microsoft"
	default:
		return "the selected provider"
	}
}

func (h *handler) socialSignupRequiredResponse(completion shared.SocialCompletion) socialCompleteResponse {
	completion.Flow = "signup"
	completion.ExpiresAt = time.Now().UTC().Add(socialCompletionTTL)
	return socialCompleteResponse{
		Status: "signup_required", RedirectURL: "/signup/social/complete",
		CompletionToken: h.ctx.AuthState.CreateSocialCompletion(completion),
	}
}

func (h *handler) handleSocialConfirmation() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimSpace(r.URL.Query().Get("token"))
		entry, err := h.ctx.Store.ConsumePendingSocialConfirmation(r.Context(), token)
		if err != nil {
			http.Redirect(w, r, h.loginErrorRedirectURL("social_confirmation_invalid"), http.StatusSeeOther)
			return
		}
		if entry.TargetUserID != nil {
			user, err := h.ctx.Store.GetUserByID(r.Context(), *entry.TargetUserID)
			if err != nil || user == nil || !strings.EqualFold(user.Email, entry.TargetEmail) {
				http.Redirect(w, r, h.loginErrorRedirectURL("social_confirmation_invalid"), http.StatusSeeOther)
				return
			}
			if _, err := h.ctx.Store.LinkSocialIdentity(r.Context(), database.LinkSocialIdentityInput{
				UserID: user.ID, Provider: entry.Provider, Subject: entry.Subject, ObservedEmail: entry.ObservedEmail,
			}); err != nil {
				http.Redirect(w, r, h.loginErrorRedirectURL("social_identity_conflict"), http.StatusSeeOther)
				return
			}
			h.appendAuthAuditForUserTeams(r, user.ID, "auth.social_identity_linked", "success", "provider="+entry.Provider+";reason=hitkeep_email_confirmed", true)
			completionToken := h.ctx.AuthState.CreateSocialCompletion(shared.SocialCompletion{
				Provider: entry.Provider, Subject: entry.Subject, ObservedEmail: entry.ObservedEmail,
				Flow: "login", ReturnPath: sanitizeAuthReturnPath(entry.ReturnPath), RememberMe: entry.RememberMe,
				ExpiresAt: time.Now().UTC().Add(socialCompletionTTL),
			})
			query := url.Values{"info": {"social_confirmed"}, "email": {user.Email}}
			redirectURL := appurl.Path(h.ctx.Config.PublicURL, "/login?"+query.Encode()) + "#social_token=" + url.QueryEscape(completionToken)
			http.Redirect(w, r, redirectURL, http.StatusSeeOther)
			return
		}
		if !h.socialSignupEnabled() || entry.AcceptedTosAt.IsZero() {
			http.Redirect(w, r, appurl.Path(h.ctx.Config.PublicURL, "/signup?error=social_confirmation_invalid"), http.StatusSeeOther)
			return
		}
		completion := shared.SocialCompletion{
			Provider: entry.Provider, Subject: entry.Subject, ObservedEmail: entry.ObservedEmail,
		}
		signup := socialCloudSignupRequest{
			TeamName: entry.TeamName, PlanCode: entry.PlanCode, BillingInterval: entry.BillingInterval,
			Jurisdiction: entry.Jurisdiction, Locale: entry.Locale, AcceptedTOS: true,
		}
		account, err := h.createSocialCloudAccount(r, completion, entry.TargetEmail, signup)
		if err != nil {
			http.Redirect(w, r, appurl.Path(h.ctx.Config.PublicURL, "/signup?error=social_confirmation_invalid"), http.StatusSeeOther)
			return
		}
		if err := h.issueLoginSession(r.Context(), w, account.UserID, entry.RememberMe); err != nil {
			http.Redirect(w, r, appurl.Path(h.ctx.Config.PublicURL, "/signup?error=social_confirmation_invalid"), http.StatusSeeOther)
			return
		}
		h.appendAuthAuditForUserTeams(r, account.UserID, "auth.social_signup_succeeded", "success", "provider="+entry.Provider+";reason=hitkeep_email_confirmed", true)
		http.Redirect(w, r, appurl.Path(h.ctx.Config.PublicURL, socialSignupRedirect(entry.PlanCode, entry.BillingInterval)), http.StatusSeeOther)
	}
}

func (h *handler) createSocialCloudAccount(r *http.Request, completion shared.SocialCompletion, email string, req socialCloudSignupRequest) (*database.ManagedSocialAccount, error) {
	randomPassword, err := security.GenerateRandomChallenge(48)
	if err != nil {
		return nil, err
	}
	passwordHash, err := HashPassword(randomPassword)
	if err != nil {
		return nil, err
	}
	account, err := h.ctx.Store.CreateManagedSocialAccount(r.Context(), database.CreateManagedSocialAccountInput{
		Email: email, HashedPassword: passwordHash, TeamName: req.TeamName, Locale: req.Locale,
		Provider: completion.Provider, Subject: completion.Subject, ObservedEmail: completion.ObservedEmail,
		PlanCode: req.PlanCode, BillingInterval: req.BillingInterval,
	})
	if err != nil {
		return nil, err
	}
	if _, err := h.ctx.Store.RecordCloudConversionEvent(r.Context(), database.CloudConversionEvent{
		TenantID: account.TenantID, EventName: database.CloudConversionSignupVerified,
		PlanCode: req.PlanCode, BillingInterval: req.BillingInterval,
	}); err != nil {
		slog.Warn("Failed to record social signup conversion", "error", err, "team_id", account.TenantID, "provider", completion.Provider)
	}
	return account, nil
}

func (h *handler) handleSocialUnlink() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		provider := strings.ToLower(strings.TrimSpace(r.PathValue("provider")))
		var req struct {
			CurrentPassword string `json:"current_password"`
		}
		if !decodeSocialJSON(w, r, &req, true) {
			return
		}

		passwordConfirmed := false
		if req.CurrentPassword != "" {
			user, err := h.ctx.Store.GetUserByID(r.Context(), userID)
			if err != nil || user == nil {
				writeSocialError(w, http.StatusInternalServerError, "social_unlink_failed")
				return
			}
			if user.PasswordLoginEnabled {
				match, verifyErr := verifyPassword(req.CurrentPassword, user.Password)
				passwordConfirmed = verifyErr == nil && match
			}
		}

		if err := h.ctx.Store.DeleteSocialIdentityWithGuard(r.Context(), userID, provider, passwordConfirmed); err != nil {
			reason := "storage"
			switch {
			case errors.Is(err, controlstore.ErrSocialIdentityNotFound):
				reason = "identity_not_found"
				writeSocialError(w, http.StatusNotFound, "social_identity_not_found")
			case errors.Is(err, controlstore.ErrSocialLastLoginMethod):
				reason = "last_login_method"
				writeSocialError(w, http.StatusConflict, "social_last_login_method")
			default:
				writeSocialError(w, http.StatusInternalServerError, "social_unlink_failed")
			}
			h.appendAuthAuditForUserTeams(r, userID, "auth.social_identity_unlink_failed", "failure", "provider="+provider+";reason="+reason, true)
			return
		}
		h.appendAuthAuditForUserTeams(r, userID, "auth.social_identity_unlinked", "success", "provider="+provider+";reason=alternative_method_available", true)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *handler) socialSignupEnabled() bool {
	return h.ctx != nil && h.ctx.Config != nil && h.ctx.Config.CloudHosted && h.ctx.Config.CloudSignupEnabled && h.ctx.Config.SocialSignupEnabled
}

func (h *handler) socialCompletionRedirectURL(state shared.SocialOAuthState, token string) string {
	fragment := "#social_token=" + url.QueryEscape(token)
	switch state.Flow {
	case "signup":
		path := state.ReturnPath
		if !strings.HasPrefix(path, "/signup/social/complete") {
			path = "/signup/social/complete"
		}
		return appurl.Path(h.ctx.Config.PublicURL, path) + fragment
	case "invite":
		return appurl.Path(h.ctx.Config.PublicURL, "/accept-invite?token="+url.QueryEscape(state.InviteToken)) + fragment
	default:
		return appurl.Path(h.ctx.Config.PublicURL, "/login") + fragment
	}
}

func (h *handler) socialErrorRedirectURL(state shared.SocialOAuthState, code string) string {
	switch state.Flow {
	case "link":
		return appurl.Path(h.ctx.Config.PublicURL, "/settings?tab=security&social="+url.QueryEscape(code))
	case "signup":
		return appurl.Path(h.ctx.Config.PublicURL, "/signup?error="+url.QueryEscape(code))
	case "invite":
		query := url.Values{"token": {state.InviteToken}, "error": {code}}
		return appurl.Path(h.ctx.Config.PublicURL, "/accept-invite?"+query.Encode())
	default:
		return h.loginErrorRedirectURL(code)
	}
}

func normalizeSocialPlan(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "free", "pro", "business":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func normalizeSocialBilling(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "monthly", "annual":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func socialSignupRedirect(planCode, billing string) string {
	if planCode == "pro" || planCode == "business" {
		query := url.Values{"plan": {planCode}, "billing": {normalizeSocialBilling(billing)}}
		return "/signup/verified?" + query.Encode()
	}
	return "/dashboard"
}

func socialCallbackErrorCode(err error) string {
	switch {
	case errors.Is(err, socialauth.ErrEmailNotVerified):
		return "social_email_unverified"
	case errors.Is(err, socialauth.ErrProviderUnavailable), errors.Is(err, socialauth.ErrNotConfigured):
		return "social_provider_unavailable"
	default:
		return "social_login_failed"
	}
}

func socialErrorCategory(err error) string {
	switch {
	case errors.Is(err, socialauth.ErrTokenExchange):
		return "token_exchange"
	case errors.Is(err, socialauth.ErrTokenValidation):
		return "token_validation"
	case errors.Is(err, socialauth.ErrEmailNotVerified):
		return "email_unverified"
	case errors.Is(err, socialauth.ErrIdentityClaims):
		return "identity_claims"
	case errors.Is(err, socialauth.ErrNotConfigured):
		return "not_configured"
	default:
		return "provider_unavailable"
	}
}

func socialDatabaseErrorCategory(err error) string {
	switch {
	case errors.Is(err, controlstore.ErrSocialIdentityConflict):
		return "subject_conflict"
	case errors.Is(err, controlstore.ErrSocialProviderAlreadyLinked):
		return "provider_conflict"
	default:
		return "storage"
	}
}

func writeSocialError(w http.ResponseWriter, status int, code string) {
	writeSocialJSON(w, status, map[string]string{"status": "error", "code": code})
}

func writeSocialJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("Failed to encode social auth response", "error", err)
	}
}

func decodeSocialJSON(w http.ResponseWriter, r *http.Request, dest any, allowEmpty bool) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAuthPayloadBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dest); err != nil {
		if allowEmpty && err == io.EOF {
			return true
		}
		writeSocialError(w, http.StatusBadRequest, "social_request_invalid")
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeSocialError(w, http.StatusBadRequest, "social_request_invalid")
		return false
	}
	return true
}
