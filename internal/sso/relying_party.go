package sso

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"hitkeep/internal/security"
)

const providerTimeout = 10 * time.Second

var (
	ErrTokenExchange     = errors.New("OIDC token exchange failed")
	ErrIDTokenMissing    = errors.New("OIDC provider omitted ID token")
	ErrIDTokenValidation = errors.New("OIDC ID token validation failed")
	ErrIdentityClaims    = errors.New("OIDC identity claims are invalid")
)

// TeamConfig contains the OIDC protocol settings required for one team.
// Team membership, entitlements, provisioning, sessions, and auditing remain
// the responsibility of the caller.
type TeamConfig struct {
	IssuerURL        string
	ClientID         string
	ClientSecret     string
	RedirectURL      string
	EmailClaim       string
	DisplayNameClaim string
}

// FlowState is the protocol state that must be bound to the caller's opaque
// OAuth state value and supplied exactly once to Complete.
type FlowState struct {
	Nonce        string
	CodeVerifier string
}

type Identity struct {
	Subject     string
	Email       string
	Domain      string
	DisplayName string
}

// AuthorizationRequest holds the generated OIDC flow state and prepares the
// authorization URL once the application has persisted its opaque state.
type AuthorizationRequest struct {
	FlowState FlowState
	config    *oauth2.Config
}

func (r AuthorizationRequest) URL(state string) string {
	if r.config == nil {
		return ""
	}
	return r.config.AuthCodeURL(
		state,
		oidc.Nonce(r.FlowState.Nonce),
		oauth2.S256ChallengeOption(r.FlowState.CodeVerifier),
	)
}

// RelyingParty owns the OIDC and OAuth protocol exchange while leaving
// HitKeep product policy and account lifecycle decisions to HTTP callers.
type RelyingParty struct {
	client *Client
}

func NewRelyingParty(client *Client) *RelyingParty {
	if client == nil {
		client = NewClient(nil)
	}
	return &RelyingParty{client: client}
}

func (r *RelyingParty) Begin(ctx context.Context, config TeamConfig) (AuthorizationRequest, error) {
	if err := validateTeamConfig(config); err != nil {
		return AuthorizationRequest{}, err
	}
	provider, err := r.discover(ctx, config.IssuerURL)
	if err != nil {
		return AuthorizationRequest{}, err
	}
	nonce, err := security.GenerateRandomChallenge(32)
	if err != nil {
		return AuthorizationRequest{}, err
	}
	return AuthorizationRequest{
		FlowState: FlowState{
			Nonce:        nonce,
			CodeVerifier: oauth2.GenerateVerifier(),
		},
		config: oauthConfig(provider, config),
	}, nil
}

func (r *RelyingParty) Complete(ctx context.Context, config TeamConfig, state FlowState, code string) (Identity, error) {
	if err := validateTeamConfig(config); err != nil {
		return Identity{}, err
	}
	if strings.TrimSpace(state.Nonce) == "" || strings.TrimSpace(state.CodeVerifier) == "" || strings.TrimSpace(code) == "" {
		return Identity{}, fmt.Errorf("%w: complete authorization response is required", ErrTokenExchange)
	}
	provider, err := r.discover(ctx, config.IssuerURL)
	if err != nil {
		return Identity{}, err
	}
	requestCtx, cancel := context.WithTimeout(r.client.Context(ctx), providerTimeout)
	defer cancel()
	token, err := oauthConfig(provider, config).Exchange(requestCtx, strings.TrimSpace(code), oauth2.VerifierOption(state.CodeVerifier))
	if err != nil {
		return Identity{}, fmt.Errorf("%w: %v", ErrTokenExchange, err)
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || strings.TrimSpace(rawIDToken) == "" {
		return Identity{}, ErrIDTokenMissing
	}
	verifier := provider.VerifierContext(requestCtx, &oidc.Config{ClientID: config.ClientID})
	idToken, err := verifier.Verify(requestCtx, rawIDToken)
	if err != nil || subtle.ConstantTimeCompare([]byte(idTokenNonce(idToken)), []byte(state.Nonce)) != 1 {
		return Identity{}, fmt.Errorf("%w: signature, claims, or nonce mismatch", ErrIDTokenValidation)
	}
	identity, err := extractIdentity(idToken, config)
	if err != nil {
		return Identity{}, fmt.Errorf("%w: %v", ErrIdentityClaims, err)
	}
	return identity, nil
}

func (r *RelyingParty) discover(ctx context.Context, issuerURL string) (*oidc.Provider, error) {
	discoveryCtx, cancel := context.WithTimeout(ctx, providerTimeout)
	defer cancel()
	return r.client.Discover(discoveryCtx, issuerURL)
}

func validateTeamConfig(config TeamConfig) error {
	if strings.TrimSpace(config.IssuerURL) == "" || strings.TrimSpace(config.ClientID) == "" || strings.TrimSpace(config.RedirectURL) == "" {
		return errors.New("complete OIDC team configuration is required")
	}
	return nil
}

func oauthConfig(provider *oidc.Provider, config TeamConfig) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  config.RedirectURL,
		Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
	}
}

func extractIdentity(idToken *oidc.IDToken, config TeamConfig) (Identity, error) {
	if idToken == nil || strings.TrimSpace(idToken.Subject) == "" {
		return Identity{}, errors.New("OIDC subject is required")
	}
	claims := make(map[string]any)
	if err := idToken.Claims(&claims); err != nil {
		return Identity{}, errors.New("could not decode OIDC claims")
	}
	verified, ok := claims["email_verified"].(bool)
	if !ok || !verified {
		return Identity{}, errors.New("OIDC email is not verified")
	}
	emailClaim := strings.TrimSpace(config.EmailClaim)
	if emailClaim == "" {
		emailClaim = "email"
	}
	emailValue, ok := claimString(claims, emailClaim)
	if !ok {
		return Identity{}, errors.New("OIDC email claim is missing")
	}
	email, domain, err := NormalizeEmail(emailValue)
	if err != nil {
		return Identity{}, err
	}
	displayNameClaim := strings.TrimSpace(config.DisplayNameClaim)
	if displayNameClaim == "" {
		displayNameClaim = "name"
	}
	displayName, _ := claimString(claims, displayNameClaim)
	return Identity{
		Subject:     strings.TrimSpace(idToken.Subject),
		Email:       email,
		Domain:      domain,
		DisplayName: strings.TrimSpace(displayName),
	}, nil
}

func claimString(claims map[string]any, path string) (string, bool) {
	var current any = claims
	for segment := range strings.SplitSeq(strings.TrimSpace(path), ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return "", false
		}
		current, ok = object[segment]
		if !ok {
			return "", false
		}
	}
	value, ok := current.(string)
	return strings.TrimSpace(value), ok && strings.TrimSpace(value) != ""
}

func NormalizeEmail(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	address, err := mail.ParseAddress(raw)
	if err != nil || !strings.EqualFold(address.Address, raw) || strings.Count(address.Address, "@") != 1 {
		return "", "", errors.New("valid email is required")
	}
	email := strings.ToLower(address.Address)
	local, domain, ok := strings.CutLast(email, "@")
	if !ok || local == "" || domain == "" {
		return "", "", errors.New("valid email is required")
	}
	domain = strings.TrimSuffix(domain, ".")
	if domain == "" {
		return "", "", errors.New("valid email is required")
	}
	return local + "@" + domain, domain, nil
}

func idTokenNonce(idToken *oidc.IDToken) string {
	if idToken == nil {
		return ""
	}
	return idToken.Nonce
}
