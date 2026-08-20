package security

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	webauthnlib "github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"

	"hitkeep/internal/api"
)

const (
	passkeyRPDisplayName = "HitKeep"
	passkeyTimeout       = 5 * time.Minute
)

type WebAuthnUser struct {
	UserID      uuid.UUID
	Name        string
	DisplayName string
	Credentials []webauthnlib.Credential
}

func NewWebAuthn(publicURL string, r *http.Request) (*webauthnlib.WebAuthn, error) {
	rpID, origin := passkeyRelyingParty(publicURL, r)
	if rpID == "" || origin == "" {
		return nil, fmt.Errorf("could not determine passkey relying party")
	}

	return webauthnlib.New(&webauthnlib.Config{
		RPID:                  rpID,
		RPDisplayName:         passkeyRPDisplayName,
		RPOrigins:             []string{origin},
		AttestationPreference: protocol.PreferNoAttestation,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementRequired,
			UserVerification: protocol.VerificationRequired,
		},
		Timeouts: webauthnlib.TimeoutsConfig{
			Login: webauthnlib.TimeoutConfig{
				Enforce:    true,
				Timeout:    passkeyTimeout,
				TimeoutUVD: passkeyTimeout,
			},
			Registration: webauthnlib.TimeoutConfig{
				Enforce:    true,
				Timeout:    passkeyTimeout,
				TimeoutUVD: passkeyTimeout,
			},
		},
	})
}

func NewWebAuthnUser(user *api.User, credentials []webauthnlib.Credential) *WebAuthnUser {
	if user == nil {
		return nil
	}

	return &WebAuthnUser{
		UserID:      user.ID,
		Name:        user.Email,
		DisplayName: userDisplayName(user),
		Credentials: credentials,
	}
}

func (u *WebAuthnUser) WebAuthnID() []byte {
	if u == nil {
		return nil
	}
	out := make([]byte, len(u.UserID))
	copy(out, u.UserID[:])
	return out
}

func (u *WebAuthnUser) WebAuthnName() string {
	if u == nil {
		return ""
	}
	return u.Name
}

func (u *WebAuthnUser) WebAuthnDisplayName() string {
	if u == nil {
		return ""
	}
	return u.DisplayName
}

func (u *WebAuthnUser) WebAuthnCredentials() []webauthnlib.Credential {
	if u == nil {
		return nil
	}
	return u.Credentials
}

func EncodeCredentialID(id []byte) string {
	return base64.RawURLEncoding.EncodeToString(id)
}

func ParseUserHandle(userHandle []byte) (uuid.UUID, error) {
	if len(userHandle) == 0 {
		return uuid.Nil, fmt.Errorf("user handle is required")
	}
	userID, err := uuid.FromBytes(userHandle)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid user handle: %w", err)
	}
	return userID, nil
}

func userDisplayName(user *api.User) string {
	parts := make([]string, 0, 2)
	if given := strings.TrimSpace(user.GivenName); given != "" {
		parts = append(parts, given)
	}
	if last := strings.TrimSpace(user.LastName); last != "" {
		parts = append(parts, last)
	}
	if len(parts) > 0 {
		return strings.Join(parts, " ")
	}
	return displayNameFromEmail(user.Email)
}

func displayNameFromEmail(email string) string {
	local, _, _ := strings.Cut(strings.TrimSpace(email), "@")
	if local == "" {
		return "User"
	}
	parts := strings.FieldsFunc(local, func(r rune) bool {
		return r == '.' || r == '_' || r == '-'
	})
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	name := strings.TrimSpace(strings.Join(parts, " "))
	if name == "" {
		return "User"
	}
	return name
}

func passkeyRelyingParty(publicURL string, r *http.Request) (rpID string, origin string) {
	if parsed, err := url.Parse(strings.TrimSpace(publicURL)); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		return parsed.Hostname(), parsed.Scheme + "://" + parsed.Host
	}

	scheme := "http"
	if r != nil && r.TLS != nil {
		scheme = "https"
	}
	if r != nil {
		if forwardedProto := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0]); forwardedProto != "" {
			scheme = forwardedProto
		}
	}

	host := ""
	if r != nil {
		host = strings.TrimSpace(r.Host)
	}
	if host == "" {
		host = "localhost:8080"
	}

	return hostNameOnly(host), scheme + "://" + host
}

func hostNameOnly(host string) string {
	if parsed, err := url.Parse("http://" + host); err == nil && parsed.Hostname() != "" {
		return parsed.Hostname()
	}
	if name, _, err := net.SplitHostPort(host); err == nil && name != "" {
		return name
	}
	return host
}
