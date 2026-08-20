package sso

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/coreos/go-oidc/v3/oidc/oidctest"

	json "hitkeep/internal/jsonapi"
)

func TestNormalizeIssuerURLPreservesExactIssuerIdentifier(t *testing.T) {
	got, err := NormalizeIssuerURL(" https://ID.Example.com/realms/acme/ ")
	if err != nil {
		t.Fatalf("normalize issuer: %v", err)
	}
	if got != "https://ID.Example.com/realms/acme/" {
		t.Fatalf("unexpected issuer %q", got)
	}
}

func TestDiscoverCachesProviderByExactIssuer(t *testing.T) {
	providerHandler := &oidctest.Server{}
	discoveryRequests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			discoveryRequests++
		}
		providerHandler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	providerHandler.SetIssuer(server.URL)

	client := NewClient(server.Client())
	first, err := client.Discover(t.Context(), server.URL)
	if err != nil {
		t.Fatalf("first discovery: %v", err)
	}
	second, err := client.Discover(t.Context(), server.URL)
	if err != nil {
		t.Fatalf("second discovery: %v", err)
	}

	if first != second {
		t.Fatal("expected exact issuer discovery to reuse the cached provider")
	}
	if discoveryRequests != 1 {
		t.Fatalf("expected one discovery request, got %d", discoveryRequests)
	}
}

func TestDiscoverCoalescesConcurrentRequestsForExactIssuer(t *testing.T) {
	providerHandler := &oidctest.Server{}
	var discoveryRequests atomic.Int32
	discoveryStarted := make(chan struct{})
	releaseDiscovery := make(chan struct{})
	var startedOnce sync.Once
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			discoveryRequests.Add(1)
			startedOnce.Do(func() { close(discoveryStarted) })
			<-releaseDiscovery
		}
		providerHandler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	providerHandler.SetIssuer(server.URL)

	client := NewClient(server.Client())
	results := make(chan *oidc.Provider, 2)
	errors := make(chan error, 2)
	start := make(chan struct{})
	for range 2 {
		go func() {
			<-start
			provider, err := client.Discover(t.Context(), server.URL)
			results <- provider
			errors <- err
		}()
	}
	close(start)
	<-discoveryStarted
	close(releaseDiscovery)

	first, second := <-results, <-results
	if err := <-errors; err != nil {
		t.Fatalf("concurrent discovery: %v", err)
	}
	if err := <-errors; err != nil {
		t.Fatalf("concurrent discovery: %v", err)
	}
	if first == nil || first != second {
		t.Fatal("expected concurrent discovery to share one provider")
	}
	if got := discoveryRequests.Load(); got != 1 {
		t.Fatalf("expected one in-flight discovery request, got %d", got)
	}
}

func TestDiscoverSupportsPathBasedIssuerWithTrailingSlash(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		issuer := server.URL + "/realms/acme/"
		if r.URL.Path != "/realms/acme/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.MarshalWrite(w, map[string]any{
			"issuer":                                issuer,
			"authorization_endpoint":                server.URL + "/authorize",
			"token_endpoint":                        server.URL + "/token",
			"jwks_uri":                              server.URL + "/keys",
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	}))
	t.Cleanup(server.Close)

	if _, err := NewClient(server.Client()).Discover(t.Context(), server.URL+"/realms/acme/"); err != nil {
		t.Fatalf("discover exact path-based issuer: %v", err)
	}
}

func TestDiscoverRejectsTrailingSlashIssuerMismatch(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.MarshalWrite(w, map[string]any{
			"issuer":                                server.URL,
			"authorization_endpoint":                server.URL + "/authorize",
			"token_endpoint":                        server.URL + "/token",
			"jwks_uri":                              server.URL + "/keys",
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	}))
	t.Cleanup(server.Close)

	if _, err := NewClient(server.Client()).Discover(t.Context(), server.URL+"/"); err == nil {
		t.Fatal("expected discovery to reject an issuer that differs by its trailing slash")
	}
}

func TestNormalizeIssuerURLRejectsUnsafeURLs(t *testing.T) {
	for _, raw := range []string{
		"http://id.example.com",
		"https://user:secret@id.example.com",
		"https://id.example.com?tenant=acme",
		"https://id.example.com?",
		"https://id.example.com#",
		"//id.example.com",
	} {
		if _, err := NormalizeIssuerURL(raw); err == nil {
			t.Fatalf("expected %q to be rejected", raw)
		}
	}
}
