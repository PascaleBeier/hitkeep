package shared

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	hitai "hitkeep/internal/ai"
	"hitkeep/internal/auth"
	"hitkeep/internal/blocking"
	"hitkeep/internal/config"
	"hitkeep/internal/database"
	"hitkeep/internal/entitlements"
	"hitkeep/internal/mailer"
	"hitkeep/internal/realtime"
	"hitkeep/internal/searchconsole"
	"hitkeep/internal/sso"
	"hitkeep/internal/takeout"
	"hitkeep/internal/webhooks"
)

type contextKey string

const UserIDKey contextKey = "user_id"
const PermissionKey contextKey = "permissions"
const APIClientAuthKey contextKey = "api_client_auth"
const AuthSessionKey contextKey = "auth_session"

type HandlerConfig struct {
	RequireAuth   bool
	InstancePerm  auth.Permission
	SitePerm      auth.Permission
	TeamCap       auth.Capability
	AllowAPIKey   bool
	APIClientOnly bool
	HumanOnly     bool
	RateLimiter   *IPRateLimiter
}

type MessageProducer interface {
	Publish(topic string, body []byte) error
	Ping() error
}

type WebhookEventEmitter interface {
	Emit(ctx context.Context, event webhooks.Event) (webhooks.Emission, error)
}

type ClusterState interface {
	IsLeader() bool
	GetLeaderAddr() string
}

// Context carries the server's shared dependencies. It is a plain container:
// policy and behavior belong to the packages the dependencies come from
// (see Limits), and request middleware lives in authn.go and authz.go.
type Context struct {
	Store          *database.Store
	TenantStores   *database.TenantStoreManager
	Cluster        ClusterState
	Producer       MessageProducer
	Mailer         *mailer.Mailer
	Config         *config.Config
	Takeout        *takeout.TakeoutService
	Entitlements   entitlements.Provider
	IngestLimiter  *IPRateLimiter
	ApiLimiter     *IPRateLimiter
	AuthLimiter    *IPRateLimiter
	WebhookLimiter *IPRateLimiter
	AuthState      *AuthStateStore
	SearchConsole  searchconsole.Client
	SSO            *sso.Client
	AI             hitai.Client
	Realtime       *realtime.Broker
	IPFilter       *blocking.IPFilter
	SpamFilter     *blocking.SpamFilter
	Webhooks       WebhookEventEmitter

	// Runtime system monitoring
	StartedAt                time.Time
	SystemCounters           *database.SystemCounter
	BackupStatus             *database.BackupStatusTracker
	ImportStageCleanupStatus *database.ImportStageCleanupStatusTracker
	MailTestTracker          *database.MailTestTracker
}

// AnalyticsStore resolves the tenant-specific store that holds analytics data for the given site.
// It falls back to the shared store if TenantStores is nil (single-tenant / follower node).
func (c *Context) AnalyticsStore(ctx context.Context, siteID uuid.UUID) (*database.Store, error) {
	if c.TenantStores == nil {
		return c.Store, nil
	}

	store, _, err := c.TenantStores.ResolveSiteStore(ctx, siteID)
	if err != nil {
		return nil, fmt.Errorf("resolve analytics store for site %s: %w", siteID, err)
	}

	return store, nil
}

// Limits returns the managed-cloud limits policy service assembled from the
// context's live dependencies. Policy decisions live in the entitlements
// package; the context only carries the dependencies.
func (c *Context) Limits() *entitlements.Service {
	return entitlements.NewService(c.Store, c.Entitlements, c.Config)
}

// GetUserIDFromContext extracts the user ID from context (set by auth middleware).
func GetUserIDFromContext(r *http.Request) uuid.UUID {
	// First check PermissionContext (new RBAC).
	if val := r.Context().Value(PermissionKey); val != nil {
		if perms, ok := val.(PermissionContext); ok {
			return perms.UserID
		}
	}

	// Fallback to legacy UserIDKey.
	val := r.Context().Value(UserIDKey)
	if val == nil {
		return uuid.Nil
	}
	id, ok := val.(uuid.UUID)
	if !ok {
		return uuid.Nil
	}
	return id
}

// Handler wraps common middleware patterns.
func (c *Context) Handler(config HandlerConfig, fn http.HandlerFunc) http.HandlerFunc {
	handler := c.applyAccessChecks(config, fn)
	handler = c.applyAuthentication(config, handler)

	// Apply rate limiting.
	if config.RateLimiter != nil {
		handler = c.WithRateLimit(config.RateLimiter, handler)
	}

	return handler
}

func (c *Context) applyAccessChecks(config HandlerConfig, handler http.HandlerFunc) http.HandlerFunc {
	if config.SitePerm != "" {
		handler = c.RequirePermission(config.SitePerm)(handler)
	}

	if config.TeamCap != "" {
		handler = c.RequireTeamCapability(config.TeamCap)(handler)
	}

	// Apply instance permission check if needed.
	if config.InstancePerm != "" {
		handler = c.RequirePermission(config.InstancePerm)(handler)
	}

	return handler
}

func (c *Context) applyAuthentication(config HandlerConfig, handler http.HandlerFunc) http.HandlerFunc {
	if config.APIClientOnly {
		return c.RequireAPIClientAuth(handler)
	}
	if config.requiresUserAuth() {
		return c.RequireAuth(config.allowsAPIKey(), handler)
	}
	return handler
}

func (config HandlerConfig) requiresUserAuth() bool {
	return config.RequireAuth || config.InstancePerm != "" || config.SitePerm != "" || config.TeamCap != ""
}

func (config HandlerConfig) allowsAPIKey() bool {
	if config.HumanOnly {
		return false
	}
	return config.AllowAPIKey || config.InstancePerm != "" || config.SitePerm != ""
}
