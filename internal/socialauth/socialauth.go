package socialauth

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"golang.org/x/oauth2"

	"hitkeep/internal/appurl"
	"hitkeep/internal/config"
	"hitkeep/internal/security"
	"hitkeep/internal/sso"
)

const (
	ProviderGoogle    = "google"
	ProviderGitHub    = "github"
	ProviderMicrosoft = "microsoft"

	googleIssuer        = "https://accounts.google.com"
	githubAuthURL       = "https://github.com/login/oauth/authorize"
	githubTokenURL      = "https://github.com/login/oauth/access_token" //nolint:gosec // Public OAuth endpoint, not a credential.
	githubAPIBaseURL    = "https://api.github.com"
	microsoftIssuerBase = "https://login.microsoftonline.com"
	microsoftConsumerID = "9188040d-6c67-4c5b-b112-36a304b66dad"
	providerTimeout     = 10 * time.Second
	maxProviderBody     = int64(1 << 20)
)

var (
	ErrNotConfigured       = errors.New("social provider is not configured")
	ErrTokenExchange       = errors.New("social provider token exchange failed")
	ErrTokenValidation     = errors.New("social provider token validation failed")
	ErrIdentityClaims      = errors.New("social provider identity claims are invalid")
	ErrEmailNotVerified    = errors.New("social provider email is not verified")
	ErrProviderUnavailable = errors.New("social provider is unavailable")
)

type ProviderConfig struct {
	Provider        string
	DisplayName     string
	ClientID        string
	ClientSecret    string
	RedirectURL     string
	MicrosoftTenant string

	// Endpoint overrides are intentionally internal-facing so deterministic tests
	// can use local providers without changing production configuration.
	IssuerURL  string
	AuthURL    string
	TokenURL   string
	APIBaseURL string
}

type ProviderStatus struct {
	Provider    string
	DisplayName string
	Configured  bool
	Partial     bool
}

type FlowState struct {
	Nonce        string
	CodeVerifier string
}

type Identity struct {
	Provider      string
	Subject       string
	Email         string
	EmailVerified bool
}

type AuthorizationRequest struct {
	FlowState FlowState
	config    *oauth2.Config
	provider  string
}

func (r AuthorizationRequest) URL(state string) string {
	if r.config == nil {
		return ""
	}
	options := []oauth2.AuthCodeOption{oauth2.S256ChallengeOption(r.FlowState.CodeVerifier)}
	if r.provider != ProviderGitHub {
		options = append(options, oidc.Nonce(r.FlowState.Nonce))
	}
	return r.config.AuthCodeURL(state, options...)
}

type Client struct {
	oidcClient *sso.Client
	httpClient *http.Client
}

func NewClient(oidcClient *sso.Client) *Client {
	if oidcClient == nil {
		oidcClient = sso.NewClient(nil)
	}
	return &Client{oidcClient: oidcClient, httpClient: oidcClient.HTTPClient}
}

func ProviderStatuses(conf *config.Config) []ProviderStatus {
	if conf == nil {
		return nil
	}
	inputs := []struct {
		provider, name, id, secret string
	}{
		{ProviderGoogle, "Google", conf.SocialGoogleClientID, conf.SocialGoogleClientSecret},
		{ProviderGitHub, "GitHub", conf.SocialGitHubClientID, conf.SocialGitHubClientSecret},
		{ProviderMicrosoft, "Microsoft", conf.SocialMicrosoftClientID, conf.SocialMicrosoftClientSecret},
	}
	statuses := make([]ProviderStatus, 0, len(inputs))
	for _, input := range inputs {
		hasID := strings.TrimSpace(input.id) != ""
		hasSecret := strings.TrimSpace(input.secret) != ""
		validTenant := true
		if input.provider == ProviderMicrosoft {
			_, validTenant = parseMicrosoftTenant(conf.SocialMicrosoftTenant)
		}
		statuses = append(statuses, ProviderStatus{
			Provider: input.provider, DisplayName: input.name,
			Configured: hasID && hasSecret && validTenant,
			Partial:    hasID != hasSecret || (hasID && hasSecret && !validTenant),
		})
	}
	return statuses
}

func ConfigForProvider(conf *config.Config, provider string) (ProviderConfig, error) {
	if conf == nil {
		return ProviderConfig{}, ErrNotConfigured
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	callback := appurl.Path(conf.PublicURL, "/api/auth/social/"+provider+"/callback")
	result := ProviderConfig{Provider: provider, RedirectURL: callback}
	switch provider {
	case ProviderGoogle:
		result.DisplayName = "Google"
		result.ClientID = conf.SocialGoogleClientID
		result.ClientSecret = conf.SocialGoogleClientSecret
		result.IssuerURL = googleIssuer
	case ProviderGitHub:
		result.DisplayName = "GitHub"
		result.ClientID = conf.SocialGitHubClientID
		result.ClientSecret = conf.SocialGitHubClientSecret
		result.AuthURL = githubAuthURL
		result.TokenURL = githubTokenURL
		result.APIBaseURL = githubAPIBaseURL
	case ProviderMicrosoft:
		result.DisplayName = "Microsoft"
		result.ClientID = conf.SocialMicrosoftClientID
		result.ClientSecret = conf.SocialMicrosoftClientSecret
		var validTenant bool
		result.MicrosoftTenant, validTenant = parseMicrosoftTenant(conf.SocialMicrosoftTenant)
		if !validTenant {
			return ProviderConfig{}, ErrNotConfigured
		}
		result.IssuerURL = microsoftIssuerBase + "/" + url.PathEscape(result.MicrosoftTenant) + "/v2.0"
	default:
		return ProviderConfig{}, ErrNotConfigured
	}
	if strings.TrimSpace(result.ClientID) == "" || strings.TrimSpace(result.ClientSecret) == "" {
		return ProviderConfig{}, ErrNotConfigured
	}
	return result, nil
}

func (c *Client) Begin(ctx context.Context, providerConfig ProviderConfig) (AuthorizationRequest, error) {
	if err := validateProviderConfig(providerConfig); err != nil {
		return AuthorizationRequest{}, err
	}
	verifier := oauth2.GenerateVerifier()
	nonce, err := security.GenerateRandomChallenge(32)
	if err != nil {
		return AuthorizationRequest{}, err
	}

	var oauthConfig *oauth2.Config
	switch providerConfig.Provider {
	case ProviderGitHub:
		oauthConfig = &oauth2.Config{
			ClientID: providerConfig.ClientID, ClientSecret: providerConfig.ClientSecret,
			RedirectURL: providerConfig.RedirectURL,
			Endpoint:    oauth2.Endpoint{AuthURL: providerConfig.AuthURL, TokenURL: providerConfig.TokenURL},
			Scopes:      []string{"read:user", "user:email"},
		}
	case ProviderGoogle, ProviderMicrosoft:
		provider, err := c.discover(ctx, providerConfig)
		if err != nil {
			return AuthorizationRequest{}, fmt.Errorf("%w: discovery failed", ErrProviderUnavailable)
		}
		oauthConfig = &oauth2.Config{
			ClientID: providerConfig.ClientID, ClientSecret: providerConfig.ClientSecret,
			RedirectURL: providerConfig.RedirectURL, Endpoint: provider.Endpoint(),
			Scopes: oidcScopes(providerConfig.Provider),
		}
	}
	return AuthorizationRequest{
		FlowState: FlowState{Nonce: nonce, CodeVerifier: verifier},
		config:    oauthConfig, provider: providerConfig.Provider,
	}, nil
}

func (c *Client) Complete(ctx context.Context, providerConfig ProviderConfig, state FlowState, code string) (Identity, error) {
	if err := validateProviderConfig(providerConfig); err != nil {
		return Identity{}, err
	}
	if strings.TrimSpace(state.CodeVerifier) == "" || strings.TrimSpace(code) == "" || (providerConfig.Provider != ProviderGitHub && strings.TrimSpace(state.Nonce) == "") {
		return Identity{}, ErrTokenExchange
	}

	requestCtx, cancel := context.WithTimeout(ctx, providerTimeout)
	defer cancel()
	if c.httpClient != nil {
		requestCtx = context.WithValue(requestCtx, oauth2.HTTPClient, c.httpClient)
	}

	switch providerConfig.Provider {
	case ProviderGitHub:
		return c.completeGitHub(requestCtx, providerConfig, state, code)
	case ProviderGoogle, ProviderMicrosoft:
		return c.completeOIDC(requestCtx, providerConfig, state, code)
	default:
		return Identity{}, ErrNotConfigured
	}
}

func (c *Client) completeOIDC(ctx context.Context, cfg ProviderConfig, state FlowState, code string) (Identity, error) {
	provider, err := c.discover(ctx, cfg)
	if err != nil {
		return Identity{}, fmt.Errorf("%w: discovery failed", ErrProviderUnavailable)
	}
	token, err := (&oauth2.Config{
		ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, RedirectURL: cfg.RedirectURL,
		Endpoint: provider.Endpoint(), Scopes: oidcScopes(cfg.Provider),
	}).Exchange(ctx, strings.TrimSpace(code), oauth2.VerifierOption(state.CodeVerifier))
	if err != nil {
		return Identity{}, ErrTokenExchange
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || strings.TrimSpace(rawIDToken) == "" {
		return Identity{}, ErrTokenValidation
	}
	verifierConfig := &oidc.Config{ClientID: cfg.ClientID}
	if cfg.Provider == ProviderGoogle || cfg.Provider == ProviderMicrosoft {
		verifierConfig.SkipIssuerCheck = true
	}
	idToken, err := provider.VerifierContext(ctx, verifierConfig).Verify(ctx, rawIDToken)
	if err != nil || subtle.ConstantTimeCompare([]byte(idToken.Nonce), []byte(state.Nonce)) != 1 {
		return Identity{}, ErrTokenValidation
	}

	claims := make(map[string]any)
	if err := idToken.Claims(&claims); err != nil {
		return Identity{}, ErrIdentityClaims
	}
	if cfg.Provider == ProviderMicrosoft {
		if err := c.validateMicrosoftSigningKeyIssuer(ctx, provider, rawIDToken, idToken.Issuer, stringClaim(claims, "tid")); err != nil {
			return Identity{}, ErrTokenValidation
		}
		return microsoftIdentity(idToken, claims, cfg)
	}
	return googleIdentity(idToken, claims, cfg)
}

func oidcScopes(provider string) []string {
	scopes := []string{oidc.ScopeOpenID, "email"}
	if provider == ProviderMicrosoft {
		// Microsoft requires profile consent before it emits the immutable oid
		// claim used together with tid as HitKeep's account key.
		scopes = append(scopes, "profile")
	}
	return scopes
}

func googleIdentity(idToken *oidc.IDToken, claims map[string]any, cfg ProviderConfig) (Identity, error) {
	if idToken == nil || strings.TrimSpace(idToken.Subject) == "" {
		return Identity{}, ErrIdentityClaims
	}
	if !googleIssuerMatches(idToken.Issuer, cfg.IssuerURL) {
		return Identity{}, ErrTokenValidation
	}
	verified, ok := claims["email_verified"].(bool)
	if !ok || !verified {
		return Identity{}, ErrEmailNotVerified
	}
	email, _, err := sso.NormalizeEmail(stringClaim(claims, "email"))
	if err != nil {
		return Identity{}, ErrIdentityClaims
	}
	return Identity{
		Provider: ProviderGoogle, Subject: strings.TrimSpace(idToken.Subject), Email: email,
		EmailVerified: true,
	}, nil
}

func googleIssuerMatches(tokenIssuer, configuredIssuer string) bool {
	tokenIssuer = strings.TrimSpace(tokenIssuer)
	configuredIssuer = strings.TrimSpace(configuredIssuer)
	if configuredIssuer == googleIssuer {
		return tokenIssuer == googleIssuer || tokenIssuer == "accounts.google.com"
	}
	return tokenIssuer != "" && tokenIssuer == configuredIssuer
}

func microsoftIdentity(idToken *oidc.IDToken, claims map[string]any, cfg ProviderConfig) (Identity, error) {
	tenantID := strings.TrimSpace(stringClaim(claims, "tid"))
	objectID := strings.TrimSpace(stringClaim(claims, "oid"))
	if idToken == nil || tenantID == "" || objectID == "" {
		return Identity{}, ErrIdentityClaims
	}
	parsedTenantID, err := uuid.Parse(tenantID)
	if err != nil {
		return Identity{}, ErrIdentityClaims
	}
	parsedObjectID, err := uuid.Parse(objectID)
	if err != nil {
		return Identity{}, ErrIdentityClaims
	}
	expectedIssuer := microsoftIssuerBase + "/" + tenantID + "/v2.0"
	if idToken.Issuer != expectedIssuer {
		return Identity{}, ErrTokenValidation
	}
	selector, validSelector := parseMicrosoftTenant(cfg.MicrosoftTenant)
	if !validSelector {
		return Identity{}, ErrTokenValidation
	}
	switch selector {
	case "consumers":
		if parsedTenantID.String() != microsoftConsumerID {
			return Identity{}, ErrTokenValidation
		}
	case "organizations":
		if parsedTenantID.String() == microsoftConsumerID {
			return Identity{}, ErrTokenValidation
		}
	case "common":
	default:
		if selector != parsedTenantID.String() {
			return Identity{}, ErrTokenValidation
		}
	}

	email := ""
	for _, claim := range []string{"email", "preferred_username"} {
		if normalized, _, err := sso.NormalizeEmail(stringClaim(claims, claim)); err == nil {
			email = normalized
			break
		}
	}
	return Identity{
		Provider: ProviderMicrosoft, Subject: parsedTenantID.String() + ":" + parsedObjectID.String(),
		Email: email, EmailVerified: false,
	}, nil
}

func (c *Client) validateMicrosoftSigningKeyIssuer(ctx context.Context, provider *oidc.Provider, rawToken, tokenIssuer, tenantID string) error {
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 {
		return ErrTokenValidation
	}
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return ErrTokenValidation
	}
	var header struct {
		KeyID string `json:"kid"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil || strings.TrimSpace(header.KeyID) == "" {
		return ErrTokenValidation
	}
	var metadata struct {
		JWKSURL string `json:"jwks_uri"`
	}
	if provider == nil || provider.Claims(&metadata) != nil || strings.TrimSpace(metadata.JWKSURL) == "" {
		return ErrTokenValidation
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, metadata.JWKSURL, nil)
	if err != nil {
		return ErrTokenValidation
	}
	client := c.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return ErrTokenValidation
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxProviderBody))
		return ErrTokenValidation
	}
	var keySet struct {
		Keys []struct {
			KeyID  string `json:"kid"`
			Issuer string `json:"issuer"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxProviderBody)).Decode(&keySet); err != nil {
		return ErrTokenValidation
	}
	foundKey := false
	for _, key := range keySet.Keys {
		if key.KeyID != header.KeyID {
			continue
		}
		foundKey = true
		if !microsoftKeyIssuerMatches(key.Issuer, tokenIssuer, tenantID) {
			return ErrTokenValidation
		}
	}
	if foundKey {
		return nil
	}
	return ErrTokenValidation
}

func microsoftKeyIssuerMatches(keyIssuer, tokenIssuer, tenantID string) bool {
	keyIssuer = strings.TrimSpace(keyIssuer)
	placeholder := "{tenantid}"
	index := strings.Index(strings.ToLower(keyIssuer), placeholder)
	if index >= 0 {
		keyIssuer = keyIssuer[:index] + tenantID + keyIssuer[index+len(placeholder):]
	}
	return keyIssuer != "" && keyIssuer == tokenIssuer
}

func (c *Client) completeGitHub(ctx context.Context, cfg ProviderConfig, state FlowState, code string) (Identity, error) {
	token, err := (&oauth2.Config{
		ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, RedirectURL: cfg.RedirectURL,
		Endpoint: oauth2.Endpoint{AuthURL: cfg.AuthURL, TokenURL: cfg.TokenURL},
		Scopes:   []string{"read:user", "user:email"},
	}).Exchange(ctx, strings.TrimSpace(code), oauth2.VerifierOption(state.CodeVerifier))
	if err != nil || strings.TrimSpace(token.AccessToken) == "" {
		return Identity{}, ErrTokenExchange
	}

	var user struct {
		ID int64 `json:"id"`
	}
	if err := c.githubGetJSON(ctx, cfg, token.AccessToken, "/user", &user); err != nil || user.ID <= 0 {
		return Identity{}, ErrIdentityClaims
	}
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := c.githubGetJSON(ctx, cfg, token.AccessToken, "/user/emails", &emails); err != nil {
		return Identity{}, ErrProviderUnavailable
	}
	verifiedEmail := ""
	for _, candidate := range emails {
		if !candidate.Primary || !candidate.Verified {
			continue
		}
		normalized, _, err := sso.NormalizeEmail(candidate.Email)
		if err == nil {
			verifiedEmail = normalized
			break
		}
	}
	if verifiedEmail == "" {
		return Identity{}, ErrEmailNotVerified
	}
	return Identity{
		Provider: ProviderGitHub, Subject: strconv.FormatInt(user.ID, 10), Email: verifiedEmail,
		EmailVerified: true,
	}, nil
}

func (c *Client) githubGetJSON(ctx context.Context, cfg ProviderConfig, accessToken, path string, dest any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(cfg.APIBaseURL, "/")+path, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-Github-Api-Version", "2026-03-10")
	request.Header.Set("User-Agent", "HitKeep")
	client := c.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxProviderBody))
		return ErrProviderUnavailable
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxProviderBody))
	if err := decoder.Decode(dest); err != nil {
		return ErrProviderUnavailable
	}
	return nil
}

func (c *Client) discover(ctx context.Context, cfg ProviderConfig) (*oidc.Provider, error) {
	if cfg.Provider == ProviderMicrosoft {
		discoveryCtx, cancel := context.WithTimeout(ctx, providerTimeout)
		defer cancel()
		if c.httpClient != nil {
			discoveryCtx = oidc.ClientContext(discoveryCtx, c.httpClient)
		}
		// Microsoft advertises a tenant-template issuer from multi-tenant
		// discovery. Signature/audience checks stay in go-oidc and the concrete
		// tenant issuer is validated explicitly after token verification.
		expectedIssuer := cfg.IssuerURL
		tenant, _ := parseMicrosoftTenant(cfg.MicrosoftTenant)
		switch tenant {
		case "common", "organizations", "consumers":
			expectedIssuer = microsoftIssuerBase + "/{tenantid}/v2.0"
		}
		discoveryCtx = oidc.InsecureIssuerURLContext(discoveryCtx, expectedIssuer)
		return oidc.NewProvider(discoveryCtx, cfg.IssuerURL)
	}
	return c.oidcClient.Discover(ctx, cfg.IssuerURL)
}

func validateProviderConfig(cfg ProviderConfig) error {
	if strings.TrimSpace(cfg.ClientID) == "" || strings.TrimSpace(cfg.ClientSecret) == "" || strings.TrimSpace(cfg.RedirectURL) == "" {
		return ErrNotConfigured
	}
	switch cfg.Provider {
	case ProviderGoogle, ProviderMicrosoft:
		if strings.TrimSpace(cfg.IssuerURL) == "" {
			return ErrNotConfigured
		}
		if cfg.Provider == ProviderMicrosoft {
			if _, valid := parseMicrosoftTenant(cfg.MicrosoftTenant); !valid {
				return ErrNotConfigured
			}
		}
	case ProviderGitHub:
		if strings.TrimSpace(cfg.AuthURL) == "" || strings.TrimSpace(cfg.TokenURL) == "" || strings.TrimSpace(cfg.APIBaseURL) == "" {
			return ErrNotConfigured
		}
	default:
		return ErrNotConfigured
	}
	return nil
}

func parseMicrosoftTenant(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", "common":
		return "common", true
	case "organizations", "consumers":
		return value, true
	default:
		if tenantID, err := uuid.Parse(value); err == nil {
			return strings.ToLower(tenantID.String()), true
		}
		return "", false
	}
}

func stringClaim(claims map[string]any, key string) string {
	value, _ := claims[key].(string)
	return strings.TrimSpace(value)
}
