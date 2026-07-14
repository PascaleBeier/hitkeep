package sso

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/coreos/go-oidc/v3/oidc"
)

type Client struct {
	HTTPClient *http.Client

	providersMu sync.Mutex
	providers   map[string]*oidc.Provider
	discoveries map[string]*providerDiscovery
}

func NewClient(httpClient *http.Client) *Client {
	return &Client{
		HTTPClient:  httpClient,
		providers:   make(map[string]*oidc.Provider),
		discoveries: make(map[string]*providerDiscovery),
	}
}

type providerDiscovery struct {
	done     chan struct{}
	provider *oidc.Provider
	err      error
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
	if parsed.User != nil || parsed.ForceQuery || parsed.RawQuery != "" || strings.Contains(raw, "#") {
		return "", errors.New("OIDC issuer cannot contain credentials, a query, or a fragment")
	}
	// The issuer is an OIDC identifier, not merely a fetch URL. Providers and
	// ID-token verifiers compare it exactly, so validation must not rewrite its
	// host casing, escaped path, or trailing slash.
	return raw, nil
}

func (c *Client) Discover(ctx context.Context, issuerURL string) (*oidc.Provider, error) {
	issuerURL, err := NormalizeIssuerURL(issuerURL)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return discoverProvider(ctx, nil, issuerURL)
	}

	c.providersMu.Lock()
	if provider := c.providers[issuerURL]; provider != nil {
		c.providersMu.Unlock()
		return provider, nil
	}
	if discovery := c.discoveries[issuerURL]; discovery != nil {
		c.providersMu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-discovery.done:
			return discovery.provider, discovery.err
		}
	}
	discovery := &providerDiscovery{done: make(chan struct{})}
	c.discoveries[issuerURL] = discovery
	c.providersMu.Unlock()

	provider, err := discoverProvider(ctx, c.HTTPClient, issuerURL)

	c.providersMu.Lock()
	discovery.provider = provider
	discovery.err = err
	if err == nil {
		c.providers[issuerURL] = provider
	}
	delete(c.discoveries, issuerURL)
	close(discovery.done)
	c.providersMu.Unlock()
	return provider, err
}

func discoverProvider(ctx context.Context, httpClient *http.Client, issuerURL string) (*oidc.Provider, error) {
	if httpClient != nil {
		ctx = oidc.ClientContext(ctx, httpClient)
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
