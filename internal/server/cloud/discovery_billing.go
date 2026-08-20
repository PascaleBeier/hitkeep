//go:build billing

package cloud

import (
	"context"
	"net/http"
	"strings"

	"hitkeep/internal/appurl"
	"hitkeep/internal/config"
	json "hitkeep/internal/jsonapi"
	"hitkeep/internal/server/shared"
	"hitkeep/internal/server/system"
)

const discoveryCacheControl = "public, max-age=3600"

var openAPIDiscoveryPaths = []string{
	"/openapi.json",
	"/swagger.json",
	"/api/openapi.json",
	"/v1/openapi.json",
	"/api/schema/",
}

func (h *handler) registerDiscoveryRoutes(mux *http.ServeMux) {
	for _, path := range openAPIDiscoveryPaths {
		mux.HandleFunc("GET "+path, h.ctx.Handler(shared.HandlerConfig{
			RateLimiter: h.ctx.ApiLimiter,
		}, h.handleCloudOpenAPI()))
	}
	mux.HandleFunc("GET /.well-known/mcp/server-card.json", h.ctx.Handler(shared.HandlerConfig{
		RateLimiter: h.ctx.ApiLimiter,
	}, h.handleCloudMCPServerCard()))
	mux.HandleFunc("GET /.well-known/integrations.json", h.ctx.Handler(shared.HandlerConfig{
		RateLimiter: h.ctx.ApiLimiter,
	}, h.handleCloudIntegrations()))
	mux.HandleFunc("GET /.well-known/api-catalog", h.ctx.Handler(shared.HandlerConfig{
		RateLimiter: h.ctx.ApiLimiter,
	}, h.handleCloudAPICatalog()))
}

func (h *handler) handleCloudOpenAPI() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.discoveryEnabled(w) {
			return
		}
		writeDiscoveryJSON(r.Context(), w, system.OpenAPISpecV1(cloudPublicURL(h.ctx.Config)))
	}
}

func (h *handler) handleCloudMCPServerCard() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.discoveryEnabled(w) {
			return
		}
		if !h.ctx.Config.MCPEnabled {
			http.NotFound(w, r)
			return
		}
		writeDiscoveryJSON(r.Context(), w, h.mcpServerCard())
	}
}

func (h *handler) handleCloudIntegrations() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.discoveryEnabled(w) {
			return
		}
		writeDiscoveryJSON(r.Context(), w, h.integrationsDeclaration())
	}
}

func (h *handler) handleCloudAPICatalog() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.discoveryEnabled(w) {
			return
		}
		writeDiscoveryLinkset(r.Context(), w, h.apiCatalog())
	}
}

func (h *handler) discoveryEnabled(w http.ResponseWriter) bool {
	if h.ctx == nil || h.ctx.Config == nil || !h.ctx.Config.CloudHosted {
		http.Error(w, "Not found", http.StatusNotFound)
		return false
	}
	return true
}

func (h *handler) mcpServerCard() map[string]any {
	publicURL := cloudPublicURL(h.ctx.Config)
	mcpPath := normalizedMCPPath(h.ctx.Config.MCPPath)
	return map[string]any{
		"$schema":          "https://static.modelcontextprotocol.io/schemas/mcp-server-card/v1.json",
		"version":          "1.0",
		"protocolVersion":  "2025-06-18",
		"url":              appurl.Path(publicURL, mcpPath),
		"description":      "Read-only MCP server for aggregate HitKeep analytics and official documentation.",
		"documentationUrl": "https://hitkeep.com/use-cases/read-only-mcp-server-web-analytics/",
		"serverInfo": map[string]any{
			"name":    "hitkeep",
			"title":   "HitKeep MCP server",
			"version": strings.TrimSpace(h.ctx.Config.Version),
		},
		"transport": map[string]any{
			"type":     "streamable-http",
			"endpoint": mcpPath,
		},
		"capabilities": map[string]any{
			"tools":     map[string]any{},
			"resources": map[string]any{},
		},
		"authentication": map[string]any{
			"type":     "bearer",
			"required": true,
			"schemes":  []string{"bearer"},
		},
		"tools":     []string{"dynamic"},
		"resources": []string{"dynamic"},
	}
}

func (h *handler) integrationsDeclaration() map[string]any {
	publicURL := cloudPublicURL(h.ctx.Config)
	source := appurl.Path(publicURL, "/.well-known/integrations.json")
	credentialID := "hitkeep_api_client_token" // #nosec G101 -- manifest identifier naming the credential type, not a secret.
	surfaces := []map[string]any{
		h.restSurface(publicURL, source, credentialID),
	}
	if h.ctx.Config.MCPEnabled {
		surfaces = append(surfaces, h.mcpSurface(publicURL, source, credentialID))
	}
	return map[string]any{
		"version": 3,
		"summary": "HitKeep Cloud exposes a REST API and read-only MCP server for privacy-first analytics.",
		"credentials": map[string]any{
			credentialID: map[string]any{
				"type":        "bearer",
				"label":       "HitKeep API client token",
				"generateUrl": appurl.Path(publicURL, "/integration/api-clients"),
				"setup":       "In the HitKeep app, open Integration > API clients and create a personal or team API client. Send the issued token as an Authorization Bearer token.",
			},
		},
		"surfaces": surfaces,
	}
}

func (h *handler) restSurface(publicURL, source, credentialID string) map[string]any {
	return map[string]any{
		"slug": "hitkeep-rest-api",
		"name": "HitKeep REST API",
		"type": "http",
		"docs": "https://hitkeep.com/api/",
		"spec": appurl.Path(publicURL, "/openapi.json"),
		"url":  appurl.Path(publicURL, "/api"),
		"basis": map[string]any{
			"via":    "declared",
			"source": source,
		},
		"auth": requiredBearerAuth(source, credentialID),
	}
}

func (h *handler) mcpSurface(publicURL, source, credentialID string) map[string]any {
	return map[string]any{
		"slug":       "hitkeep-mcp-server",
		"name":       "HitKeep MCP server",
		"type":       "mcp",
		"docs":       "https://hitkeep.com/use-cases/read-only-mcp-server-web-analytics/",
		"url":        appurl.Path(publicURL, normalizedMCPPath(h.ctx.Config.MCPPath)),
		"transports": []string{"streamable-http"},
		"basis": map[string]any{
			"via":    "declared",
			"source": source,
		},
		"auth": requiredBearerAuth(source, credentialID),
	}
}

func requiredBearerAuth(source, credentialID string) map[string]any {
	basis := map[string]any{
		"via":    "declared",
		"source": source,
	}
	return map[string]any{
		"status": "required",
		"entries": []map[string]any{
			{
				"use": []map[string]any{
					{
						"id": credentialID,
						"mechanics": map[string]any{
							"source":     "http",
							"in":         "header",
							"headerName": "Authorization",
							"scheme":     "Bearer",
						},
					},
				},
				"basis": basis,
			},
		},
	}
}

func (h *handler) apiCatalog() map[string]any {
	publicURL := cloudPublicURL(h.ctx.Config)
	linkset := []map[string]any{
		{
			"anchor": appurl.Path(publicURL, "/api"),
			"service-desc": []map[string]any{
				{
					"href":  appurl.Path(publicURL, "/openapi.json"),
					"type":  "application/json",
					"title": "HitKeep REST API OpenAPI",
				},
			},
			"service-doc": []map[string]any{
				{
					"href": "https://hitkeep.com/api/",
					"type": "text/html",
				},
			},
			"status": []map[string]any{
				{
					"href": appurl.Path(publicURL, "/healthz"),
					"type": "text/plain",
				},
			},
		},
	}
	if h.ctx.Config.MCPEnabled {
		linkset = append(linkset, map[string]any{
			"anchor": appurl.Path(publicURL, normalizedMCPPath(h.ctx.Config.MCPPath)),
			"service-desc": []map[string]any{
				{
					"href":  appurl.Path(publicURL, "/.well-known/mcp/server-card.json"),
					"type":  "application/json",
					"title": "HitKeep MCP server card",
				},
			},
			"service-doc": []map[string]any{
				{
					"href": "https://hitkeep.com/use-cases/read-only-mcp-server-web-analytics/",
					"type": "text/html",
				},
			},
			"status": []map[string]any{
				{
					"href": appurl.Path(publicURL, "/readyz"),
					"type": "text/plain",
				},
			},
		})
	}
	return map[string]any{"linkset": linkset}
}

func cloudPublicURL(conf *config.Config) string {
	publicURL := strings.TrimRight(strings.TrimSpace(conf.PublicURL), "/")
	if publicURL == "" {
		return "http://localhost:8080"
	}
	return publicURL
}

func normalizedMCPPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || path == "/" {
		return "/mcp"
	}
	if !strings.HasPrefix(path, "/") {
		return "/" + path
	}
	return path
}

func writeDiscoveryJSON(ctx context.Context, w http.ResponseWriter, body any) {
	writeDiscoveryHeaders(w, "application/json")
	if err := json.MarshalWrite(w, body); err != nil {
		shared.LoggerFromContext(ctx).Error("Failed to encode cloud discovery response", "error", err)
	}
}

func writeDiscoveryLinkset(ctx context.Context, w http.ResponseWriter, body any) {
	writeDiscoveryHeaders(w, "application/linkset+json")
	if err := json.MarshalWrite(w, body); err != nil {
		shared.LoggerFromContext(ctx).Error("Failed to encode cloud discovery response", "error", err)
	}
}

func writeDiscoveryHeaders(w http.ResponseWriter, contentType string) {
	header := w.Header()
	header.Set("Content-Type", contentType)
	header.Set("Cache-Control", discoveryCacheControl)
	header.Set("Access-Control-Allow-Origin", "*")
	header.Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	header.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	header.Set("X-Content-Type-Options", "nosniff")
}
