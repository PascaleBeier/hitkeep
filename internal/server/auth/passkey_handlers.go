package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	webauthnlib "github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"

	"hitkeep/internal/database"
	appsecurity "hitkeep/internal/security"
)

const (
	passkeyLoginChallengeTTL = 5 * time.Minute
	maxAuthPayloadBytes      = 1 << 20
)

type passkeyLoginStartResponse struct {
	ChallengeToken string                                     `json:"challenge_token"`
	PublicKey      protocol.PublicKeyCredentialRequestOptions `json:"publicKey"`
}

type passkeyLoginFinishRequest struct {
	ChallengeToken string                               `json:"challenge_token"`
	Credential     protocol.CredentialAssertionResponse `json:"credential"`
	RememberMe     bool                                 `json:"remember_me"`
}

func (h *handler) handlePasskeyLoginStart() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !decodeAuthJSON(w, r, &struct{}{}, true) {
			return
		}

		webAuthn, err := appsecurity.NewWebAuthn(h.ctx.Config.PublicURL, r)
		if err != nil {
			slog.Error("Failed to configure passkey login", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		assertion, session, err := webAuthn.BeginDiscoverableLogin()
		if err != nil {
			slog.Error("Failed to begin passkey login", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		expiresAt := session.Expires
		if expiresAt.IsZero() {
			expiresAt = time.Now().UTC().Add(passkeyLoginChallengeTTL)
		}

		if h.ctx.AuthState == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}
		challengeID := h.ctx.AuthState.CreatePasskeyLoginChallenge(session.Challenge, database.CreateLoginChallengeInput{
			Flow: "passwordless",
		}, expiresAt, session)

		resp := passkeyLoginStartResponse{
			ChallengeToken: challengeID.String(),
			PublicKey:      assertion.Response,
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("Failed to encode passkey login start response", "error", err)
		}
	}
}

func (h *handler) handlePasskeyLoginFinish() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req passkeyLoginFinishRequest
		if !decodeAuthJSON(w, r, &req, false) {
			return
		}

		challengeID, err := uuid.Parse(strings.TrimSpace(req.ChallengeToken))
		if err != nil {
			http.Error(w, "Invalid challenge token", http.StatusBadRequest)
			return
		}

		parsedCredential, err := req.Credential.Parse()
		if err != nil {
			http.Error(w, "Invalid passkey assertion", http.StatusBadRequest)
			return
		}

		if h.ctx.AuthState == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}
		challenge, found := h.ctx.AuthState.GetPasskeyLoginChallenge(challengeID)
		if !found {
			http.Error(w, "Passkey login challenge not found", http.StatusConflict)
			return
		}
		h.ctx.AuthState.DeletePasskeyLoginChallenge(challengeID)
		if time.Now().UTC().After(challenge.ExpiresAt.UTC()) {
			http.Error(w, "Passkey login challenge expired", http.StatusGone)
			return
		}
		if challenge.Session == nil {
			http.Error(w, "Passkey login challenge is invalid", http.StatusForbidden)
			return
		}

		webAuthn, err := appsecurity.NewWebAuthn(h.ctx.Config.PublicURL, r)
		if err != nil {
			slog.Error("Failed to configure passkey login validation", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		var (
			userID              uuid.UUID
			validatedCredential *webauthnlib.Credential
		)

		flow := strings.TrimSpace(challenge.Flow)
		switch flow {
		case "mfa":
			if !challenge.HasUserID {
				http.Error(w, "Invalid MFA challenge context", http.StatusForbidden)
				return
			}

			user, err := h.loadPasskeyUserForAssertion(
				r.Context(),
				challenge.UserID,
				parsedCredential.RawID,
				parsedCredential.Response.AuthenticatorData.Flags,
			)
			if err != nil {
				slog.Error("Failed to load passkey user for MFA login", "error", err, "user_id", challenge.UserID)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			validatedCredential, err = webAuthn.ValidateLogin(user, *challenge.Session, parsedCredential)
			if err != nil {
				slog.Warn("Passkey assertion verification failed", "error", err)
				http.Error(w, "Invalid passkey assertion", http.StatusUnauthorized)
				return
			}

			userID = challenge.UserID
		default:
			var validatedUser webauthnlib.User
			validatedUser, validatedCredential, err = webAuthn.ValidatePasskeyLogin(
				func(rawID, userHandle []byte) (webauthnlib.User, error) {
					userID, err := appsecurity.ParseUserHandle(userHandle)
					if err != nil {
						return nil, err
					}
					return h.loadPasskeyUserForAssertion(r.Context(), userID, rawID, parsedCredential.Response.AuthenticatorData.Flags)
				},
				*challenge.Session,
				parsedCredential,
			)
			if err != nil {
				slog.Warn("Passkey assertion verification failed", "error", err)
				http.Error(w, "Invalid passkey assertion", http.StatusUnauthorized)
				return
			}

			passkeyUser, ok := validatedUser.(*appsecurity.WebAuthnUser)
			if !ok {
				slog.Error("Validated passkey user has unexpected type")
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			userID = passkeyUser.UserID
		}

		if err := h.persistValidatedPasskey(r.Context(), *validatedCredential); err != nil {
			slog.Error("Failed to persist validated passkey", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		rememberMe := req.RememberMe
		if flow == "mfa" {
			rememberMe = challenge.RememberMe
		}
		if err := h.issueLoginSession(r.Context(), w, userID, rememberMe); err != nil {
			slog.Error("Failed to issue login session after passkey verification", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if flow == "mfa" {
			h.appendAuthAuditForUserTeams(r, userID, "auth.mfa_succeeded", "success", "Passkey multi-factor authentication succeeded", true)
			h.appendAuthAuditForUserTeams(r, userID, "auth.login_succeeded", "success", "Login succeeded after multi-factor authentication", true)
		} else {
			h.appendAuthAuditForUserTeams(r, userID, "auth.login_succeeded", "success", "Passkey login succeeded", true)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
			slog.Error("Failed to encode passkey login response", "error", err, "user_id", userID)
		}
	}
}

func (h *handler) loadPasskeyUser(ctx context.Context, userID uuid.UUID) (*appsecurity.WebAuthnUser, error) {
	user, err := h.ctx.Store.GetUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load user: %w", err)
	}
	if user == nil {
		return nil, fmt.Errorf("user not found")
	}

	credentials, err := h.ctx.Store.ListUserPasskeyCredentials(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load passkey credentials: %w", err)
	}

	return appsecurity.NewWebAuthnUser(user, credentials), nil
}

func (h *handler) loadPasskeyUserForAssertion(
	ctx context.Context,
	userID uuid.UUID,
	rawCredentialID []byte,
	flags protocol.AuthenticatorFlags,
) (*appsecurity.WebAuthnUser, error) {
	user, err := h.loadPasskeyUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	user.Credentials = normalizeLegacyCredentialFlags(user.Credentials, rawCredentialID, flags)

	return user, nil
}

func normalizeLegacyCredentialFlags(
	credentials []webauthnlib.Credential,
	rawCredentialID []byte,
	flags protocol.AuthenticatorFlags,
) []webauthnlib.Credential {
	if len(credentials) == 0 {
		return credentials
	}

	normalized := append([]webauthnlib.Credential(nil), credentials...)
	for i, credential := range normalized {
		if !bytes.Equal(credential.ID, rawCredentialID) || strings.TrimSpace(credential.AttestationType) != "" {
			continue
		}

		normalized[i].Flags = webauthnlib.NewCredentialFlags(flags)
		normalized[i].AttestationType = "none"
		break
	}

	return normalized
}

func (h *handler) persistValidatedPasskey(ctx context.Context, credential webauthnlib.Credential) error {
	credentialID := appsecurity.EncodeCredentialID(credential.ID)
	passkey, err := h.ctx.Store.GetPasskeyByCredentialID(ctx, credentialID)
	if err != nil {
		return err
	}
	if passkey == nil {
		return fmt.Errorf("passkey credential %q not found", credentialID)
	}
	return h.ctx.Store.UpdatePasskeyCredential(ctx, passkey.ID, credential)
}

func decodeAuthJSON(w http.ResponseWriter, r *http.Request, dest any, allowEmpty bool) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAuthPayloadBytes))
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
