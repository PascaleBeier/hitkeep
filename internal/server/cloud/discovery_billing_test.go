//go:build billing

package cloud

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDiscoveryOpenAPIAliases(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	defer store.Close()

	mux := http.NewServeMux()
	h.registerDiscoveryRoutes(mux)

	for _, path := range openAPIDiscoveryPaths {
		t.Run(path, func(t *testing.T) {
			rec := getCloudDiscovery(t, mux, path)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d body %q", rec.Code, rec.Body.String())
			}
			assertDiscoveryHeaders(t, rec, "application/json")

			var spec map[string]any
			decodeJSONBody(t, rec, &spec)
			if got := spec["openapi"]; got != "3.1.0" {
				t.Fatalf("expected OpenAPI 3.1.0, got %v", got)
			}
			servers, ok := spec["servers"].([]any)
			if !ok || len(servers) != 1 {
				t.Fatalf("expected one server entry, got %#v", spec["servers"])
			}
			server, ok := servers[0].(map[string]any)
			if !ok || server["url"] != "https://cloud.hitkeep.eu" {
				t.Fatalf("unexpected server entry %#v", servers[0])
			}
		})
	}
}

func TestRegisterExposesDiscoveryRoutesInCloudBuild(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	defer store.Close()

	mux := http.NewServeMux()
	Register(mux, h.ctx)

	rec := getCloudDiscovery(t, mux, "/openapi.json")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body %q", rec.Code, rec.Body.String())
	}
	assertDiscoveryHeaders(t, rec, "application/json")
}

func TestDiscoveryMCPServerCard(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	defer store.Close()
	h.ctx.Config.MCPEnabled = true
	h.ctx.Config.MCPPath = "mcp"
	h.ctx.Config.Version = "v-test"

	mux := http.NewServeMux()
	h.registerDiscoveryRoutes(mux)

	rec := getCloudDiscovery(t, mux, "/.well-known/mcp/server-card.json")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body %q", rec.Code, rec.Body.String())
	}
	assertDiscoveryHeaders(t, rec, "application/json")

	var card map[string]any
	decodeJSONBody(t, rec, &card)
	if card["url"] != "https://cloud.hitkeep.eu/mcp" {
		t.Fatalf("unexpected MCP URL %v", card["url"])
	}
	transport := requireJSONMap(t, card, "transport")
	if transport["type"] != "streamable-http" || transport["endpoint"] != "/mcp" {
		t.Fatalf("unexpected transport %#v", transport)
	}
	auth := requireJSONMap(t, card, "authentication")
	if auth["type"] != "bearer" || auth["required"] != true {
		t.Fatalf("unexpected auth %#v", auth)
	}
	info := requireJSONMap(t, card, "serverInfo")
	if info["version"] != "v-test" {
		t.Fatalf("unexpected server version %v", info["version"])
	}
}

func TestDiscoveryMCPServerCardRequiresEnabledMCP(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	defer store.Close()
	h.ctx.Config.MCPEnabled = false

	mux := http.NewServeMux()
	h.registerDiscoveryRoutes(mux)

	rec := getCloudDiscovery(t, mux, "/.well-known/mcp/server-card.json")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d body %q", rec.Code, rec.Body.String())
	}
}

func TestDiscoveryIntegrationsDeclaration(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	defer store.Close()
	h.ctx.Config.MCPEnabled = true
	h.ctx.Config.MCPPath = "/mcp"

	mux := http.NewServeMux()
	h.registerDiscoveryRoutes(mux)

	rec := getCloudDiscovery(t, mux, "/.well-known/integrations.json")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body %q", rec.Code, rec.Body.String())
	}
	assertDiscoveryHeaders(t, rec, "application/json")

	var declaration map[string]any
	decodeJSONBody(t, rec, &declaration)
	if declaration["version"] != float64(3) {
		t.Fatalf("expected version 3, got %#v", declaration["version"])
	}
	credentials := requireJSONMap(t, declaration, "credentials")
	if _, ok := credentials["hitkeep_api_client_token"]; !ok {
		t.Fatalf("expected shared API client credential, got %#v", credentials)
	}
	surfaces, ok := declaration["surfaces"].([]any)
	if !ok || len(surfaces) != 2 {
		t.Fatalf("expected REST and MCP surfaces, got %#v", declaration["surfaces"])
	}

	for _, raw := range surfaces {
		surface, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("expected surface map, got %T", raw)
		}
		basis := requireJSONMap(t, surface, "basis")
		if basis["source"] != "https://cloud.hitkeep.eu/.well-known/integrations.json" {
			t.Fatalf("unexpected basis %#v", basis)
		}
		auth := requireJSONMap(t, surface, "auth")
		if auth["status"] != "required" {
			t.Fatalf("expected required auth for %#v", surface)
		}
	}
}

func TestDiscoveryAPICatalog(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	defer store.Close()
	h.ctx.Config.MCPEnabled = true

	mux := http.NewServeMux()
	h.registerDiscoveryRoutes(mux)

	rec := getCloudDiscovery(t, mux, "/.well-known/api-catalog")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body %q", rec.Code, rec.Body.String())
	}
	assertDiscoveryHeaders(t, rec, "application/linkset+json")

	var catalog map[string]any
	decodeJSONBody(t, rec, &catalog)
	linkset, ok := catalog["linkset"].([]any)
	if !ok || len(linkset) != 2 {
		t.Fatalf("expected REST and MCP linkset entries, got %#v", catalog["linkset"])
	}
	rest, ok := linkset[0].(map[string]any)
	if !ok || rest["anchor"] != "https://cloud.hitkeep.eu/api" {
		t.Fatalf("unexpected REST linkset entry %#v", linkset[0])
	}
	mcp, ok := linkset[1].(map[string]any)
	if !ok || mcp["anchor"] != "https://cloud.hitkeep.eu/mcp" {
		t.Fatalf("unexpected MCP linkset entry %#v", linkset[1])
	}
}

func TestDiscoveryRoutesRequireCloudHosted(t *testing.T) {
	h, store := setupCloudTestHandler(t)
	defer store.Close()
	h.ctx.Config.CloudHosted = false
	h.ctx.Config.MCPEnabled = true

	mux := http.NewServeMux()
	h.registerDiscoveryRoutes(mux)

	for _, path := range []string{"/openapi.json", "/.well-known/mcp/server-card.json", "/.well-known/integrations.json", "/.well-known/api-catalog"} {
		t.Run(path, func(t *testing.T) {
			rec := getCloudDiscovery(t, mux, path)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("expected status 404, got %d body %q", rec.Code, rec.Body.String())
			}
		})
	}
}

func getCloudDiscovery(t *testing.T, mux *http.ServeMux, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func assertDiscoveryHeaders(t *testing.T, rec *httptest.ResponseRecorder, contentType string) {
	t.Helper()
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, contentType) {
		t.Fatalf("expected content type %q, got %q", contentType, got)
	}
	if got := rec.Header().Get("Cache-Control"); got != discoveryCacheControl {
		t.Fatalf("expected cache-control %q, got %q", discoveryCacheControl, got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("expected wildcard CORS, got %q", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("expected nosniff, got %q", got)
	}
}

func decodeJSONBody(t *testing.T, rec *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), target); err != nil {
		t.Fatalf("decode JSON body: %v\n%s", err, rec.Body.String())
	}
}

func requireJSONMap(t *testing.T, parent map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := parent[key].(map[string]any)
	if !ok {
		t.Fatalf("expected %s map, got %#v", key, parent[key])
	}
	return value
}
