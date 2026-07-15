package auth

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	webauthnlib "github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"

	"hitkeep/internal/database"
	"hitkeep/internal/security"
)

// beginUserLogin applies the same local MFA policy to every primary login
// method. It issues a session only when no configured local factor requires a
// challenge; callers remain responsible for method-specific audit details.
func (h *handler) beginUserLogin(r *http.Request, w http.ResponseWriter, userID uuid.UUID, rememberMe bool, authProvider string) (loginResponse, error) {
	totpEnabled, err := h.ctx.Store.HasEnabledTOTP(r.Context(), userID)
	if err != nil {
		return loginResponse{}, fmt.Errorf("check user TOTP status: %w", err)
	}
	passkeys, err := h.ctx.Store.ListUserPasskeys(r.Context(), userID)
	if err != nil {
		return loginResponse{}, fmt.Errorf("list user passkeys: %w", err)
	}
	hasPasskey := len(passkeys) > 0
	recoveryCodesRemaining, err := h.ctx.Store.CountActiveRecoveryCodes(r.Context(), userID)
	if err != nil {
		return loginResponse{}, fmt.Errorf("count recovery codes: %w", err)
	}

	if !totpEnabled && !hasPasskey {
		if err := h.issueLoginSession(r.Context(), w, userID, rememberMe); err != nil {
			return loginResponse{}, err
		}
		return loginResponse{Status: "ok"}, nil
	}
	if h.ctx.AuthState == nil {
		return loginResponse{}, errAuthStateUnavailable
	}

	var (
		challenge      string
		session        *webauthnlib.SessionData
		passkeyOptions *protocol.PublicKeyCredentialRequestOptions
	)
	if hasPasskey {
		passkeyUser, err := h.loadPasskeyUser(r.Context(), userID)
		if err != nil {
			return loginResponse{}, err
		}
		webAuthn, err := security.NewWebAuthn(h.ctx.Config.PublicURL, r)
		if err != nil {
			return loginResponse{}, err
		}
		assertion, beginSession, err := webAuthn.BeginLogin(passkeyUser)
		if err != nil {
			return loginResponse{}, err
		}
		challenge = beginSession.Challenge
		session = beginSession
		passkeyOptions = &assertion.Response
	} else {
		challenge, err = security.GenerateRandomChallenge(32)
		if err != nil {
			return loginResponse{}, err
		}
	}

	expiresAt := time.Now().UTC().Add(passkeyLoginChallengeTTL)
	if session != nil && !session.Expires.IsZero() {
		expiresAt = session.Expires
	}
	challengeID := h.ctx.AuthState.CreatePasskeyLoginChallenge(challenge, database.CreateLoginChallengeInput{
		UserID: &userID, RememberMe: rememberMe, Flow: "mfa", AuthProvider: authProvider,
	}, expiresAt, session)

	factors := make([]string, 0, 4)
	if totpEnabled {
		factors = append(factors, "totp")
	}
	if recoveryCodesRemaining > 0 {
		factors = append(factors, "recovery_code")
	}
	if h.ctx.Mailer != nil {
		factors = append(factors, "email_link")
	}
	response := loginResponse{Status: "mfa_required", ChallengeToken: challengeID.String(), Factors: factors}
	if hasPasskey {
		response.Factors = append(response.Factors, "passkey")
		response.Passkey = passkeyOptions
	}
	return response, nil
}

var errAuthStateUnavailable = fmt.Errorf("auth state is unavailable")
