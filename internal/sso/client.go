package sso

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
)

type Client struct {
	HTTPClient *http.Client
}

func NewClient(httpClient *http.Client) *Client {
	return &Client{HTTPClient: httpClient}
}

func NormalizeIssuerURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("OIDC issuer must be an absolute URL")
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return "", errors.New("OIDC issuer must use HTTPS")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("OIDC issuer cannot contain credentials, a query, or a fragment")
	}
	parsed.Scheme = "https"
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	return parsed.String(), nil
}

func (c *Client) Discover(ctx context.Context, issuerURL string) (*oidc.Provider, error) {
	issuerURL, err := NormalizeIssuerURL(issuerURL)
	if err != nil {
		return nil, err
	}
	if c != nil && c.HTTPClient != nil {
		ctx = oidc.ClientContext(ctx, c.HTTPClient)
	}
	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, fmt.Errorf("discover OIDC provider: %w", err)
	}
	endpoint := provider.Endpoint()
	if endpoint.AuthURL == "" || endpoint.TokenURL == "" {
		return nil, errors.New("OIDC provider discovery omitted required endpoints")
	}
	return provider, nil
}

func (c *Client) Context(ctx context.Context) context.Context {
	if c != nil && c.HTTPClient != nil {
		return oidc.ClientContext(ctx, c.HTTPClient)
	}
	return ctx
}
