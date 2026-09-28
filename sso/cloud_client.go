package sso

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

const maxCloudOIDCResponseBytes int64 = 1 << 20

var (
	errUnsafeOIDCEndpoint   = errors.New("unsafe OIDC endpoint")
	errOIDCResponseTooLarge = errors.New("OIDC response exceeds the allowed size")
	errOIDCRedirectLimit    = errors.New("OIDC redirect limit exceeded")
)

var blockedCloudOIDCPrefixes = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("2001::/32"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
}

type ipResolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

type dialContextFunc func(context.Context, string, string) (net.Conn, error)

type cloudNetworkPolicy struct {
	resolver    ipResolver
	dialContext dialContextFunc
}

// NewRuntimeClient keeps private-network identity providers available to
// self-hosted operators while applying the managed-cloud outbound boundary.
func NewRuntimeClient(cloudHosted bool) *Client {
	if !cloudHosted {
		return NewClient(nil)
	}
	return NewClient(newCloudHTTPClient())
}

func newCloudHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Environment proxies would resolve and fetch the tenant-controlled target
	// outside this process, bypassing the dial-time address checks below.
	transport.Proxy = nil
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	policy := cloudNetworkPolicy{
		resolver:    net.DefaultResolver,
		dialContext: dialer.DialContext,
	}
	transport.DialContext = policy.DialContext

	return &http.Client{
		Transport: &cloudRoundTripper{
			base:             transport,
			maxResponseBytes: maxCloudOIDCResponseBytes,
		},
		CheckRedirect: cloudRedirectPolicy,
	}
}

func (p cloudNetworkPolicy) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid network address", errUnsafeOIDCEndpoint)
	}

	var addresses []netip.Addr
	if literal, parseErr := netip.ParseAddr(host); parseErr == nil {
		addresses = []netip.Addr{literal}
	} else {
		resolver := p.resolver
		if resolver == nil {
			resolver = net.DefaultResolver
		}
		addresses, err = resolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, fmt.Errorf("resolve OIDC endpoint: %w", err)
		}
	}
	if len(addresses) == 0 {
		return nil, errors.New("OIDC endpoint did not resolve to an address")
	}
	for _, address := range addresses {
		if !isPublicCloudOIDCAddress(address) {
			return nil, fmt.Errorf("%w: endpoint resolved to a non-public address", errUnsafeOIDCEndpoint)
		}
	}

	dial := p.dialContext
	if dial == nil {
		dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
		dial = dialer.DialContext
	}
	var lastErr error
	for _, address := range addresses {
		connection, err := dial(ctx, network, net.JoinHostPort(address.String(), port))
		if err == nil {
			return connection, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func isPublicCloudOIDCAddress(address netip.Addr) bool {
	address = address.Unmap()
	if !address.IsValid() || address.Zone() != "" || !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsUnspecified() {
		return false
	}
	for _, prefix := range blockedCloudOIDCPrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

type cloudRoundTripper struct {
	base             http.RoundTripper
	maxResponseBytes int64
}

func (t *cloudRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil || !strings.EqualFold(req.URL.Scheme, "https") || req.URL.User != nil {
		return nil, fmt.Errorf("%w: endpoints must use HTTPS without credentials", errUnsafeOIDCEndpoint)
	}
	if t == nil || t.base == nil {
		return nil, errors.New("OIDC HTTP transport is unavailable")
	}
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	limit := t.maxResponseBytes
	if limit <= 0 {
		limit = maxCloudOIDCResponseBytes
	}
	if resp.ContentLength > limit {
		_ = resp.Body.Close()
		return nil, errOIDCResponseTooLarge
	}
	if resp.Body != nil {
		resp.Body = &limitedOIDCResponseBody{body: resp.Body, remaining: limit}
	}
	return resp, nil
}

func cloudRedirectPolicy(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errOIDCRedirectLimit
	}
	if req == nil || req.URL == nil || !strings.EqualFold(req.URL.Scheme, "https") || req.URL.User != nil {
		return fmt.Errorf("%w: redirects must use HTTPS without credentials", errUnsafeOIDCEndpoint)
	}
	return nil
}

type limitedOIDCResponseBody struct {
	body      io.ReadCloser
	remaining int64
}

func (b *limitedOIDCResponseBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if b.remaining <= 0 {
		var probe [1]byte
		n, err := b.body.Read(probe[:])
		if n > 0 {
			return 0, errOIDCResponseTooLarge
		}
		return 0, err
	}
	if int64(len(p)) > b.remaining {
		p = p[:b.remaining]
	}
	n, err := b.body.Read(p)
	b.remaining -= int64(n)
	return n, err
}

func (b *limitedOIDCResponseBody) Close() error {
	return b.body.Close()
}
