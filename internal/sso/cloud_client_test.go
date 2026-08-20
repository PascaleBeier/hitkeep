package sso

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestRuntimeClientUsesManagedCloudBoundaryOnlyInCloud(t *testing.T) {
	cloudClient := NewRuntimeClient(true)
	if cloudClient.HTTPClient == nil {
		t.Fatal("managed cloud client must use a hardened HTTP client")
	}
	if _, ok := cloudClient.HTTPClient.Transport.(*cloudRoundTripper); !ok {
		t.Fatalf("managed cloud client uses an unexpected transport %T", cloudClient.HTTPClient.Transport)
	}

	selfHostedClient := NewRuntimeClient(false)
	if selfHostedClient.HTTPClient != nil {
		t.Fatal("self-hosted client must retain the operator's default network access")
	}
}

func TestRuntimeCloudClientRejectsPrivateIssuer(t *testing.T) {
	server := httptest.NewTLSServer(http.NotFoundHandler())
	t.Cleanup(server.Close)

	_, err := NewRuntimeClient(true).Discover(t.Context(), server.URL)
	if !errors.Is(err, errUnsafeOIDCEndpoint) {
		t.Fatalf("expected private issuer rejection, got %v", err)
	}
}

func TestCloudNetworkPolicyRejectsNonPublicResolutions(t *testing.T) {
	for _, rawIP := range []string{
		"127.0.0.1",
		"10.0.0.1",
		"169.254.169.254",
		"100.64.0.1",
		"::1",
		"fd00::1",
		"64:ff9b::a9fe:a9fe",
	} {
		t.Run(rawIP, func(t *testing.T) {
			dialed := false
			policy := cloudNetworkPolicy{
				resolver: staticIPResolver{"issuer.example": {netip.MustParseAddr(rawIP)}},
				dialContext: func(context.Context, string, string) (net.Conn, error) {
					dialed = true
					return nil, errors.New("unexpected dial")
				},
			}

			_, err := policy.DialContext(t.Context(), "tcp", "issuer.example:443")
			if !errors.Is(err, errUnsafeOIDCEndpoint) {
				t.Fatalf("expected %s to be rejected, got %v", rawIP, err)
			}
			if dialed {
				t.Fatalf("dial attempted for rejected address %s", rawIP)
			}
		})
	}
}

func TestCloudNetworkPolicyDialsValidatedIPWithoutResolvingAgain(t *testing.T) {
	dialErr := errors.New("dial stopped by test")
	dialedAddress := ""
	policy := cloudNetworkPolicy{
		resolver: staticIPResolver{"issuer.example": {netip.MustParseAddr("93.184.216.34")}},
		dialContext: func(_ context.Context, _, address string) (net.Conn, error) {
			dialedAddress = address
			return nil, dialErr
		},
	}

	_, err := policy.DialContext(t.Context(), "tcp", "issuer.example:443")
	if !errors.Is(err, dialErr) {
		t.Fatalf("expected test dial error, got %v", err)
	}
	if dialedAddress != "93.184.216.34:443" {
		t.Fatalf("dial used unresolved or unexpected address %q", dialedAddress)
	}
}

func TestCloudRoundTripperRejectsNonHTTPSEndpoints(t *testing.T) {
	baseCalled := false
	transport := &cloudRoundTripper{base: roundTripFunc(func(*http.Request) (*http.Response, error) {
		baseCalled = true
		return nil, errors.New("unexpected round trip")
	})}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://public.example/token", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	resp, err := transport.RoundTrip(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if !errors.Is(err, errUnsafeOIDCEndpoint) {
		t.Fatalf("expected non-HTTPS endpoint rejection, got %v", err)
	}
	if baseCalled {
		t.Fatal("rejected endpoint reached the base transport")
	}
}

func TestCloudRedirectPolicyRejectsHTTPSDowngrade(t *testing.T) {
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://metadata.internal/latest", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	via, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://issuer.example/.well-known/openid-configuration", nil)
	if err != nil {
		t.Fatalf("build prior request: %v", err)
	}

	if err := cloudRedirectPolicy(req, []*http.Request{via}); !errors.Is(err, errUnsafeOIDCEndpoint) {
		t.Fatalf("expected HTTPS downgrade rejection, got %v", err)
	}
}

func TestCloudRoundTripperCapsResponseBodies(t *testing.T) {
	transport := &cloudRoundTripper{
		base: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode:    http.StatusOK,
				Body:          io.NopCloser(strings.NewReader("12345")),
				ContentLength: -1,
			}, nil
		}),
		maxResponseBytes: 4,
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://issuer.example/.well-known/openid-configuration", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if !errors.Is(err, errOIDCResponseTooLarge) {
		t.Fatalf("expected response limit error, body=%q err=%v", body, err)
	}
	if string(body) != "1234" {
		t.Fatalf("unexpected body before limit: %q", body)
	}
}

func TestLimitedOIDCResponseBodyDoesNotConsumeOnEmptyRead(t *testing.T) {
	body := &limitedOIDCResponseBody{
		body:      io.NopCloser(strings.NewReader("x")),
		remaining: 0,
	}
	defer body.Close()

	if n, err := body.Read(nil); n != 0 || err != nil {
		t.Fatalf("empty read consumed or rejected data: n=%d err=%v", n, err)
	}
	if _, err := body.Read(make([]byte, 1)); !errors.Is(err, errOIDCResponseTooLarge) {
		t.Fatalf("expected the next real read to enforce the limit, got %v", err)
	}
}

type staticIPResolver map[string][]netip.Addr

func (r staticIPResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	return r[host], nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
