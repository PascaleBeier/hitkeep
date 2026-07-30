package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	authcore "hitkeep/internal/auth"
	"hitkeep/internal/config"
	"hitkeep/internal/database"
	"hitkeep/internal/server/shared"
)

const (
	serverName           = "hitkeep"
	mcpListCacheTTL      = 5 * time.Minute
	mcpStaticResourceTTL = time.Hour
)

type authContextKey struct{}

type service struct {
	conf         *config.Config
	store        *database.Store
	tenantStores *database.TenantStoreManager
	docs         *docsClient
	apiLimiter   *shared.IPRateLimiter
	logger       *slog.Logger
	mcp          *mcp.Server
}

func Register(mux *http.ServeMux, ctx *shared.Context, logger *slog.Logger) {
	mux.Handle(ctx.Config.MCPPath, NewHandler(ctx.Config, ctx.Store, ctx.TenantStores, ctx.ApiLimiter, logger))
}

func NewHandler(conf *config.Config, store *database.Store, tenantStores *database.TenantStoreManager, apiLimiter *shared.IPRateLimiter, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	svc := &service{
		conf:         conf,
		store:        store,
		tenantStores: tenantStores,
		logger:       logger,
	}
	if conf.MCPDocsEnabled {
		svc.docs = newDocsClient(conf.MCPDocsURL, time.Duration(conf.MCPDocsCacheMinutes)*time.Minute)
	}
	svc.apiLimiter = apiLimiter
	svc.mcp = svc.newMCPServer()

	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return svc.mcp
	}, &mcp.StreamableHTTPOptions{
		Stateless:                  true,
		JSONResponse:               true,
		Logger:                     logger,
		DisableLocalhostProtection: true,
	})
	return svc.hostValidationMiddleware(svc.authMiddleware(handler))
}

func (s *service) newMCPServer() *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:        serverName,
		Title:       "HitKeep Analytics MCP",
		Description: "Read-only aggregate HitKeep analytics and official documentation over a stateless MCP endpoint.",
		Version:     s.conf.Version,
	}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{
			Tools:     &mcp.ToolCapabilities{ListChanged: false},
			Resources: &mcp.ResourceCapabilities{ListChanged: false, Subscribe: false},
		},
	})
	server.AddReceivingMiddleware(s.logMiddleware(), s.cacheMiddleware())
	s.registerTools(server)
	s.registerResources(server)
	return server
}

func (s *service) cacheMiddleware() mcp.Middleware {
	docsTTL := time.Duration(s.conf.MCPDocsCacheMinutes) * time.Minute
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			result, err := next(ctx, method, req)
			if err != nil || result == nil {
				return result, err
			}

			setCache := func(cache *mcp.Cacheable, ttl time.Duration) {
				if cache == nil {
					return
				}
				if ttl < 0 {
					ttl = 0
				}
				cache.TTLMs = int(ttl / time.Millisecond)
				cache.CacheScope = "private"
			}

			switch typed := result.(type) {
			case *mcp.DiscoverResult:
				setCache(&typed.Cacheable, mcpListCacheTTL)
			case *mcp.ListToolsResult:
				setCache(&typed.Cacheable, mcpListCacheTTL)
			case *mcp.ListResourcesResult:
				setCache(&typed.Cacheable, mcpListCacheTTL)
			case *mcp.ListResourceTemplatesResult:
				setCache(&typed.Cacheable, mcpListCacheTTL)
			case *mcp.ReadResourceResult:
				ttl := time.Duration(0)
				if request, ok := req.(*mcp.ReadResourceRequest); ok && request.Params != nil {
					switch request.Params.URI {
					case helpMCPURI, helpMetricsURI:
						ttl = mcpStaticResourceTTL
					default:
						if s.docs != nil {
							ttl = docsTTL
						}
					}
				}
				setCache(&typed.Cacheable, ttl)
			}
			return result, nil
		}
	}
}

func (s *service) hostValidationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestHost, allowLoopbackHost := mcpRequestHost(r, s.conf.GetTrustedProxyNetworks())
		if !isAllowedMCPHost(requestHost, s.conf.PublicURL, allowLoopbackHost) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *service) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}
		if s.apiLimiter != nil {
			ip := shared.GetRealIP(r, s.conf.GetTrustedProxyNetworks())
			limiter := s.apiLimiter.GetLimiter(ip)
			if !limiter.Allow() {
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}
		}

		token := extractBearerToken(r)
		if token == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		authz, err := s.store.GetAPIClientAuth(r.Context(), token)
		if err != nil {
			s.logger.Error("Failed to validate MCP API client token", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if authz == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), authContextKey{}, authz)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func mcpRequestHost(r *http.Request, trustedProxies []netip.Prefix) (string, bool) {
	if isTrustedMCPProxyRequest(r, trustedProxies) && !trustsAllMCPProxies(trustedProxies) {
		if host := forwardedMCPHost(r.Header); host != "" {
			return host, false
		}
		return r.Host, false
	}
	return r.Host, isLoopbackMCPRemote(r.RemoteAddr)
}

func isTrustedMCPProxyRequest(r *http.Request, trustedProxies []netip.Prefix) bool {
	directIP := shared.RemoteIPFromAddr(r.RemoteAddr)
	parsedDirectIP, ok := shared.ParseAddr(directIP)
	return ok && shared.IsTrustedProxy(parsedDirectIP, trustedProxies)
}

func trustsAllMCPProxies(trustedProxies []netip.Prefix) bool {
	for _, prefix := range trustedProxies {
		if prefix.Bits() == 0 {
			return true
		}
	}
	return false
}

func isLoopbackMCPRemote(remoteAddr string) bool {
	directIP := shared.RemoteIPFromAddr(remoteAddr)
	parsedDirectIP, ok := shared.ParseAddr(directIP)
	return ok && parsedDirectIP.IsLoopback()
}

func forwardedMCPHost(header http.Header) string {
	if host := firstMCPHeaderToken(header.Values("X-Forwarded-Host")); host != "" {
		return host
	}

	for _, entry := range mcpHeaderTokens(header.Values("Forwarded")) {
		for part := range strings.SplitSeq(entry, ";") {
			key, value, ok := strings.Cut(part, "=")
			if !ok || !strings.EqualFold(strings.TrimSpace(key), "host") {
				continue
			}
			return strings.Trim(strings.TrimSpace(value), `"`)
		}
	}
	return ""
}

func firstMCPHeaderToken(values []string) string {
	tokens := mcpHeaderTokens(values)
	if len(tokens) == 0 {
		return ""
	}
	return tokens[0]
}

func mcpHeaderTokens(values []string) []string {
	var tokens []string
	for _, value := range values {
		for part := range strings.SplitSeq(value, ",") {
			token := strings.TrimSpace(part)
			if token != "" {
				tokens = append(tokens, token)
			}
		}
	}
	return tokens
}

type mcpHost struct {
	name string
	port string
}

func isAllowedMCPHost(requestHost, publicURL string, allowLoopbackHost bool) bool {
	requested, ok := parseMCPHost(requestHost)
	if !ok {
		return false
	}
	if allowLoopbackHost && isLoopbackMCPHost(requested.name) {
		return true
	}

	public, defaultPort, ok := publicMCPHost(publicURL)
	if !ok || requested.name != public.name {
		return false
	}
	if requested.port == "" {
		return public.port == defaultPort
	}
	return requested.port == public.port
}

func parseMCPHost(raw string) (mcpHost, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return mcpHost{}, false
	}
	parsed, err := url.Parse("//" + trimmed)
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return mcpHost{}, false
	}
	host := normalizeMCPHost(parsed.Hostname())
	if host == "" {
		return mcpHost{}, false
	}
	return mcpHost{name: host, port: parsed.Port()}, true
}

func publicMCPHost(publicURL string) (mcpHost, string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(publicURL))
	if err != nil || parsed.Hostname() == "" {
		return mcpHost{}, "", false
	}
	defaultPort := defaultMCPPort(parsed.Scheme)
	if defaultPort == "" {
		return mcpHost{}, "", false
	}
	port := parsed.Port()
	if port == "" {
		port = defaultPort
	}
	host := normalizeMCPHost(parsed.Hostname())
	return mcpHost{name: host, port: port}, defaultPort, host != ""
}

func normalizeMCPHost(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	return strings.TrimSuffix(host, ".")
}

func defaultMCPPort(scheme string) string {
	switch strings.ToLower(strings.TrimSpace(scheme)) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

func isLoopbackMCPHost(host string) bool {
	host = normalizeMCPHost(host)
	if host == "localhost" {
		return true
	}
	addr, err := netip.ParseAddr(host)
	return err == nil && addr.IsLoopback()
}

func extractBearerToken(r *http.Request) string {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func apiAuth(ctx context.Context) (*database.APIClientAuth, error) {
	authz, _ := ctx.Value(authContextKey{}).(*database.APIClientAuth)
	if authz == nil {
		return nil, errors.New("unauthorized")
	}
	return authz, nil
}

func (s *service) requireSiteView(ctx context.Context, siteID uuid.UUID) (*database.APIClientAuth, error) {
	authz, err := apiAuth(ctx)
	if err != nil {
		return nil, err
	}
	role, ok := authz.SiteRoles[siteID]
	if !ok || !role.HasPermission(authcore.PermSiteView) {
		return nil, errors.New("forbidden")
	}
	return authz, nil
}

func (s *service) analyticsStore(ctx context.Context, siteID uuid.UUID) (*database.Store, error) {
	if s.tenantStores == nil {
		return s.store, nil
	}
	store, _, err := s.tenantStores.ResolveSiteStore(ctx, siteID)
	return store, err
}

func (s *service) logMiddleware() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			start := time.Now()
			result, err := next(ctx, method, req)
			authz, _ := apiAuth(ctx)
			attrs := []any{
				"method", method,
				"duration_ms", time.Since(start).Milliseconds(),
			}
			if authz != nil {
				attrs = append(attrs, "api_client_id", authz.ClientID)
			}
			if call, ok := req.(*mcp.CallToolRequest); ok && call.Params != nil {
				attrs = append(attrs, "tool", call.Params.Name)
				if siteID := rawSiteID(call.Params.Arguments); siteID != "" {
					attrs = append(attrs, "site_id", siteID)
				}
			}
			if err != nil {
				attrs = append(attrs, "outcome", "protocol_error", "error", err)
				s.logger.Warn("MCP request failed", attrs...)
				return result, err
			}
			if toolResult, ok := result.(*mcp.CallToolResult); ok && toolResult.IsError {
				attrs = append(attrs, "outcome", "tool_error")
				if toolErr := toolResult.GetError(); toolErr != nil {
					attrs = append(attrs, "error", toolErr)
				}
				s.logger.Warn("MCP request completed with tool error", attrs...)
				return result, nil
			}
			attrs = append(attrs, "outcome", "success")
			s.logger.Info("MCP request completed", attrs...)
			return result, nil
		}
	}
}

func rawSiteID(raw json.RawMessage) string {
	var payload struct {
		SiteID string `json:"site_id"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.SiteID)
}

func toMCPSearchConsoleSyncStatus(state *database.GoogleSearchConsoleSyncState) *mcpSearchConsoleSyncStatus {
	if state == nil {
		return nil
	}
	return &mcpSearchConsoleSyncStatus{
		State:             state.State,
		ImportedStartDate: formatOptionalMCPDate(state.ImportedStartDate),
		ImportedEndDate:   formatOptionalMCPDate(state.ImportedEndDate),
		LastSuccessAt:     formatOptionalMCPTime(state.LastSuccessAt),
		LastAttemptAt:     formatOptionalMCPTime(state.LastAttemptAt),
		LastErrorCategory: state.LastErrorCategory,
		NextRetryAt:       formatOptionalMCPTime(state.NextRetryAt),
		Manual:            state.Manual,
	}
}

func formatOptionalMCPDate(ts *time.Time) string {
	if ts == nil {
		return ""
	}
	return formatMCPDate(*ts)
}

func formatOptionalMCPTime(ts *time.Time) string {
	if ts == nil {
		return ""
	}
	return formatMCPTime(*ts)
}
