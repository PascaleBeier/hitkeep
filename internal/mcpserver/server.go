package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
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
	serverName = "hitkeep"
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
		Stateless:    true,
		JSONResponse: true,
		Logger:       logger,
	})
	return svc.authMiddleware(handler)
}

func (s *service) newMCPServer() *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    serverName,
		Version: s.conf.Version,
	}, nil)
	server.AddReceivingMiddleware(s.logMiddleware())
	s.registerTools(server)
	s.registerResources(server)
	return server
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
	if authz.InstanceRole.HasPermission(authcore.PermSiteView) {
		return authz, nil
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
