package user

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	webauthnlib "github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
	appsecurity "hitkeep/internal/security"
	"hitkeep/internal/server/shared"
)

const (
	maxSecurityPayloadBytes = 1 << 20
	totpSetupTTL            = 10 * time.Minute
	passkeyChallengeTTL     = 5 * time.Minute
)

type passkeyRegistrationStartRequest struct {
	Name string `json:"name"`
}

type passkeyRegistrationStartResponse struct {
	PublicKey protocol.PublicKeyCredentialCreationOptions `json:"publicKey"`
}

type verifyTOTPRequest struct {
	Code string `json:"code"`
}

type disableTOTPRequest struct {
	Code string `json:"code"`
}

func (h *handler) handleGetUserSecurityStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		status, err := h.getUserSecurityStatus(r.Context(), userID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to load user security status", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(status); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode user security status", "error", err, "user_id", userID)
		}
	}
}

func (h *handler) handleStartTOTPSetup() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		user, err := h.ctx.Store.GetUserByID(r.Context(), userID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to load user for totp setup", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if user == nil {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}

		secret, err := appsecurity.GenerateTOTPSecret()
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to generate totp secret", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if h.ctx.AuthState == nil {
			shared.LoggerFromContext(r.Context()).Error("TOTP auth state cache is not configured", "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		expiresAt := time.Now().UTC().Add(totpSetupTTL)
		h.ctx.AuthState.CreatePendingTOTPSetup(userID, secret, expiresAt)

		issuer := h.totpIssuer()
		resp := api.UserTOTPSetup{
			Secret:     secret,
			OTPAuthURL: appsecurity.BuildOTPAuthURL(issuer, user.Email, secret),
			ExpiresAt:  expiresAt,
		}

		w.Header().Set("Content-Type", "application/json")
		//nolint:gosec // TOTP bootstrap secret is intentionally returned to the authenticated user during setup.
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode pending totp setup", "error", err, "user_id", userID)
		}
	}
}

func (h *handler) handleVerifyTOTPSetup() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		var req verifyTOTPRequest
		if !decodeSecurityJSON(w, r, &req, false) {
			return
		}

		if h.ctx.AuthState == nil {
			shared.LoggerFromContext(r.Context()).Error("TOTP auth state cache is not configured", "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		secret, expiresAt, found := h.ctx.AuthState.GetPendingTOTPSetup(userID)
		if !found {
			http.Error(w, "No TOTP setup is pending", http.StatusConflict)
			return
		}
		if time.Now().UTC().After(expiresAt.UTC()) {
			h.ctx.AuthState.DeletePendingTOTPSetup(userID)
			http.Error(w, "TOTP setup expired", http.StatusGone)
			return
		}
		if !appsecurity.ValidateTOTPCode(secret, req.Code, time.Now().UTC()) {
			http.Error(w, "Invalid TOTP code", http.StatusBadRequest)
			return
		}

		if err := h.ctx.Store.EnableUserTOTP(r.Context(), userID, secret); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to enable totp", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		h.ctx.AuthState.DeletePendingTOTPSetup(userID)

		status, err := h.getUserSecurityStatus(r.Context(), userID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to load user security status after enabling totp", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(status); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode security status after enabling totp", "error", err, "user_id", userID)
		}
	}
}

func (h *handler) handleDisableTOTP() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		var req disableTOTPRequest
		if !decodeSecurityJSON(w, r, &req, false) {
			return
		}

		secret, found, err := h.ctx.Store.GetUserTOTPSecret(r.Context(), userID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to get user totp secret", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if !found {
			http.Error(w, "TOTP is not enabled", http.StatusNotFound)
			return
		}
		if !appsecurity.ValidateTOTPCode(secret, req.Code, time.Now().UTC()) {
			http.Error(w, "Invalid TOTP code", http.StatusBadRequest)
			return
		}

		if err := h.ctx.Store.DisableUserTOTP(r.Context(), userID); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to disable user totp", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		status, err := h.getUserSecurityStatus(r.Context(), userID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to load user security status after disabling totp", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(status); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode security status after disabling totp", "error", err, "user_id", userID)
		}
	}
}

func (h *handler) handleStartPasskeyRegistration() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		var req passkeyRegistrationStartRequest
		if !decodeSecurityJSON(w, r, &req, true) {
			return
		}

		passkeyUser, err := h.loadPasskeyUser(r.Context(), userID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to load passkey user for registration", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		webAuthn, err := appsecurity.NewWebAuthn(h.ctx.Config.PublicURL, r)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to configure passkey registration", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		creation, session, err := webAuthn.BeginRegistration(passkeyUser, webauthnlib.WithExclusions(passkeyCredentialDescriptors(passkeyUser.Credentials)))
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to begin passkey registration", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		expiresAt := session.Expires
		if expiresAt.IsZero() {
			expiresAt = time.Now().UTC().Add(passkeyChallengeTTL)
		}
		if h.ctx.AuthState == nil {
			shared.LoggerFromContext(r.Context()).Error("Passkey auth state cache is not configured", "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		h.ctx.AuthState.CreatePasskeyChallenge(userID, session.Challenge, strings.TrimSpace(req.Name), expiresAt, session)

		resp := passkeyRegistrationStartResponse{
			PublicKey: creation.Response,
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode passkey registration start response", "error", err, "user_id", userID)
		}
	}
}

func (h *handler) handleFinishPasskeyRegistration() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		var req protocol.CredentialCreationResponse
		if !decodeSecurityJSON(w, r, &req, false) {
			return
		}

		parsedCredential, err := req.Parse()
		if err != nil {
			http.Error(w, "Invalid passkey registration payload", http.StatusBadRequest)
			return
		}

		if h.ctx.AuthState == nil {
			shared.LoggerFromContext(r.Context()).Error("Passkey auth state cache is not configured", "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		_, requestedName, expiresAt, session, found := h.ctx.AuthState.GetPasskeyChallenge(userID)
		if !found {
			http.Error(w, "No passkey registration is pending", http.StatusConflict)
			return
		}
		if time.Now().UTC().After(expiresAt.UTC()) {
			h.ctx.AuthState.DeletePasskeyChallenge(userID)
			http.Error(w, "Passkey registration challenge expired", http.StatusGone)
			return
		}
		if session == nil {
			http.Error(w, "Passkey registration challenge is invalid", http.StatusForbidden)
			return
		}

		passkeyUser, err := h.loadPasskeyUser(r.Context(), userID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to load passkey user for registration completion", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		webAuthn, err := appsecurity.NewWebAuthn(h.ctx.Config.PublicURL, r)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to configure passkey registration validation", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		credential, err := webAuthn.CreateCredential(passkeyUser, *session, parsedCredential)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Warn("Passkey registration verification failed", "error", err, "user_id", userID)
			http.Error(w, "Invalid passkey registration", http.StatusBadRequest)
			return
		}

		name := strings.TrimSpace(requestedName)
		if name == "" {
			name = "Passkey " + time.Now().Format("2006-01-02")
		}

		if _, err := h.ctx.Store.CreateUserPasskeyCredential(r.Context(), userID, name, *credential); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to create user passkey", "error", err, "user_id", userID)
			http.Error(w, "Failed to save passkey", http.StatusBadRequest)
			return
		}

		h.ctx.AuthState.DeletePasskeyChallenge(userID)

		status, err := h.getUserSecurityStatus(r.Context(), userID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to load user security status after passkey registration", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(status); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode security status after passkey registration", "error", err, "user_id", userID)
		}
	}
}

func (h *handler) handleDeleteUserPasskey() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		passkeyID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			http.Error(w, "Invalid passkey ID", http.StatusBadRequest)
			return
		}

		if err := h.ctx.Store.DeleteUserPasskey(r.Context(), userID, passkeyID); err != nil {
			if strings.Contains(err.Error(), "no rows affected") {
				http.Error(w, "Passkey not found", http.StatusNotFound)
				return
			}
			if errors.Is(err, database.ErrLastPrimaryLoginMethod) {
				http.Error(w, "Passkey is the last usable login method", http.StatusConflict)
				return
			}
			shared.LoggerFromContext(r.Context()).Error("Failed to delete user passkey", "error", err, "user_id", userID, "passkey_id", passkeyID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *handler) handleRegenerateRecoveryCodes() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		status, err := h.getUserSecurityStatus(r.Context(), userID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to load user security status before regenerating recovery codes", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if !status.TOTPEnabled && len(status.Passkeys) == 0 {
			http.Error(w, "Enable TOTP or register a passkey before generating recovery codes", http.StatusConflict)
			return
		}

		codes, err := appsecurity.GenerateRecoveryCodes()
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to generate recovery codes", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		hashes := make([]string, 0, len(codes))
		for _, code := range codes {
			hash, err := appsecurity.HashRecoveryCode(code)
			if err != nil {
				shared.LoggerFromContext(r.Context()).Error("Failed to hash recovery code", "error", err, "user_id", userID)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			if hash == "" {
				shared.LoggerFromContext(r.Context()).Error("Failed to hash recovery code", "user_id", userID)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			hashes = append(hashes, hash)
		}
		if err := h.ctx.Store.ReplaceUserRecoveryCodes(r.Context(), userID, hashes); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to persist recovery codes", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(api.UserRecoveryCodesResponse{
			Codes:     codes,
			Remaining: len(codes),
		}); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode recovery code response", "error", err, "user_id", userID)
		}
	}
}

func (h *handler) getUserSecurityStatus(ctx context.Context, userID uuid.UUID) (api.UserSecurityStatus, error) {
	user, err := h.ctx.Store.GetUserByID(ctx, userID)
	if err != nil {
		return api.UserSecurityStatus{}, err
	}
	if user == nil {
		return api.UserSecurityStatus{}, fmt.Errorf("user not found")
	}
	totpEnabled, err := h.ctx.Store.HasEnabledTOTP(ctx, userID)
	if err != nil {
		return api.UserSecurityStatus{}, err
	}
	totpPending := false
	if h.ctx.AuthState != nil {
		totpPending = h.ctx.AuthState.HasPendingTOTPSetup(userID)
	}
	passkeys, err := h.ctx.Store.ListUserPasskeys(ctx, userID)
	if err != nil {
		return api.UserSecurityStatus{}, err
	}
	recoveryStatus, err := h.ctx.Store.GetRecoveryCodeStatus(ctx, userID)
	if err != nil {
		return api.UserSecurityStatus{}, err
	}
	socialIdentities, err := h.ctx.Store.ListUserSocialIdentities(ctx, userID)
	if err != nil {
		return api.UserSecurityStatus{}, err
	}
	return api.UserSecurityStatus{
		TOTPEnabled:            totpEnabled,
		TOTPPending:            totpPending,
		Passkeys:               passkeys,
		RecoveryCodesGenerated: recoveryStatus.Generated,
		RecoveryCodesRemaining: recoveryStatus.Remaining,
		PasswordLoginEnabled:   user.PasswordLoginEnabled,
		SocialIdentities:       socialIdentities,
	}, nil
}

func (h *handler) totpIssuer() string {
	parsed, err := url.Parse(h.ctx.Config.PublicURL)
	if err == nil && parsed.Hostname() != "" {
		return parsed.Hostname()
	}
	return "HitKeep"
}

func (h *handler) loadPasskeyUser(ctx context.Context, userID uuid.UUID) (*appsecurity.WebAuthnUser, error) {
	user, err := h.ctx.Store.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, fmt.Errorf("user not found")
	}

	credentials, err := h.ctx.Store.ListUserPasskeyCredentials(ctx, userID)
	if err != nil {
		return nil, err
	}

	return appsecurity.NewWebAuthnUser(user, credentials), nil
}

func passkeyCredentialDescriptors(credentials []webauthnlib.Credential) []protocol.CredentialDescriptor {
	if len(credentials) == 0 {
		return nil
	}

	descriptors := make([]protocol.CredentialDescriptor, 0, len(credentials))
	for _, credential := range credentials {
		descriptors = append(descriptors, credential.Descriptor())
	}
	return descriptors
}

func decodeSecurityJSON(w http.ResponseWriter, r *http.Request, dest any, allowEmpty bool) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxSecurityPayloadBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dest); err != nil {
		if allowEmpty && err == io.EOF {
			return true
		}
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return false
	}
	return true
}
