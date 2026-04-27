package shared

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nsqio/go-nsq"

	"hitkeep/internal/auth"
	"hitkeep/internal/blocking"
	"hitkeep/internal/cluster"
	"hitkeep/internal/config"
	"hitkeep/internal/database"
	"hitkeep/internal/entitlements"
	"hitkeep/internal/mailer"
	"hitkeep/internal/takeout"
)

type contextKey string

const UserIDKey contextKey = "user_id"
const PermissionKey contextKey = "permissions"
const APIClientAuthKey contextKey = "api_client_auth"
const AuthSessionKey contextKey = "auth_session"

type PermissionContext struct {
	UserID       uuid.UUID
	InstanceRole auth.InstanceRole
	SiteRole     auth.SiteRole // Only set if checking site permission.
}

type AuthSessionContext struct {
	ExpiresAt time.Time
	IssuedAt  time.Time
}

type HandlerConfig struct {
	RequireAuth  bool
	InstancePerm auth.Permission
	SitePerm     auth.Permission
	AllowAPIKey  bool
	RateLimiter  *IPRateLimiter
}

type Context struct {
	Store          *database.Store
	TenantStores   *database.TenantStoreManager
	Cluster        *cluster.Manager
	Producer       *nsq.Producer
	Mailer         *mailer.Mailer
	Config         *config.Config
	Takeout        *takeout.TakeoutService
	Entitlements   entitlements.Provider
	IngestLimiter  *IPRateLimiter
	ApiLimiter     *IPRateLimiter
	AuthLimiter    *IPRateLimiter
	WebhookLimiter *IPRateLimiter
	AuthState      *AuthStateStore
	IPFilter       *blocking.IPFilter
	SpamFilter     *blocking.SpamFilter
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
	handler := fn

	// Apply site permission check if needed.
	if config.SitePerm != "" {
		handler = c.RequirePermission(config.SitePerm)(handler)
	}

	// Apply instance permission check if needed.
	if config.InstancePerm != "" {
		handler = c.RequirePermission(config.InstancePerm)(handler)
	}

	// Apply auth if needed.
	if config.RequireAuth || config.InstancePerm != "" || config.SitePerm != "" {
		allowAPIKey := config.AllowAPIKey || config.InstancePerm != "" || config.SitePerm != ""
		handler = c.RequireAuth(allowAPIKey, handler)
	}

	// Apply rate limiting.
	if config.RateLimiter != nil {
		handler = c.WithRateLimit(config.RateLimiter, handler)
	}

	return handler
}

// RequirePermission checks if user has the required permission.
func (c *Context) RequirePermission(perm auth.Permission) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			userID := GetUserIDFromContext(r)
			apiClientAuth, _ := r.Context().Value(APIClientAuthKey).(*database.APIClientAuth)
			if userID == uuid.Nil && apiClientAuth == nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			instanceRole := auth.InstanceUser
			if userID != uuid.Nil {
				var err error
				instanceRole, err = c.Store.GetInstanceRole(r.Context(), userID)
				if err != nil {
					slog.Error("Failed to get instance role", "error", err)
					http.Error(w, "Internal error", http.StatusInternalServerError)
					return
				}
			}
			if apiClientAuth != nil {
				instanceRole = auth.MinInstanceRole(instanceRole, apiClientAuth.InstanceRole)
			}

			// Check instance-level permission.
			if instanceRole.HasPermission(perm) {
				ctx := context.WithValue(r.Context(), PermissionKey, PermissionContext{
					UserID:       userID,
					InstanceRole: instanceRole,
				})
				next(w, r.WithContext(ctx))
				return
			}

			// For site-level permissions, check site role.
			if strings.HasPrefix(string(perm), "site.") {
				siteIDStr := r.PathValue("id")
				if siteIDStr == "" {
					http.Error(w, "Site ID required", http.StatusBadRequest)
					return
				}

				siteID, err := uuid.Parse(siteIDStr)
				if err != nil {
					http.Error(w, "Invalid site ID", http.StatusBadRequest)
					return
				}

				var siteRole auth.SiteRole
				if userID != uuid.Nil {
					var err error
					siteRole, err = c.Store.GetSiteRole(r.Context(), userID, siteID)
					if err != nil {
						http.Error(w, "Access denied", http.StatusForbidden)
						return
					}
				}

				if apiClientAuth != nil {
					delegatedRole, ok := apiClientAuth.SiteRoles[siteID]
					if !ok {
						http.Error(w, "Forbidden", http.StatusForbidden)
						return
					}
					if userID == uuid.Nil {
						siteRole = delegatedRole
					} else {
						siteRole = auth.MinSiteRole(siteRole, delegatedRole)
					}
				}

				if siteRole.HasPermission(perm) {
					ctx := context.WithValue(r.Context(), PermissionKey, PermissionContext{
						UserID:       userID,
						InstanceRole: instanceRole,
						SiteRole:     siteRole,
					})
					next(w, r.WithContext(ctx))
					return
				}
			}

			http.Error(w, "Forbidden", http.StatusForbidden)
		}
	}
}

// RequireAuth wraps a handler and ensures the user is authenticated.
// It sets the UserIDKey in the context.
func (c *Context) RequireAuth(allowAPIKey bool, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var userID uuid.UUID
		var err error
		var apiClientAuth *database.APIClientAuth
		var sessionCtx *AuthSessionContext

		// 1. Try to validate the short-lived JWT.
		cookie, err := r.Cookie(auth.CookieName)
		if err == nil {
			var claims *auth.Claims
			claims, err = auth.ValidateTokenClaims(cookie.Value, c.Config.JWTSecret, c.Config.PublicURL)
			if err == nil {
				userID = claims.UserID
				sessionCtx = authSessionContextFromClaims(claims)
			}
		}

		// 2. If JWT is missing or invalid, try the Remember Me token.
		if err != nil || userID == uuid.Nil {
			rememberCookie, err := r.Cookie(auth.RememberMeCookieName)
			if err == nil {
				userID, err = c.Store.ValidateRememberMeToken(r.Context(), rememberCookie.Value)
				if err == nil && userID != uuid.Nil {
					// Valid remember me token! Issue a new JWT.
					duration := c.Config.AuthSessionDuration()
					newToken, expiresAt, err := auth.GenerateTokenWithDuration(c.Config.JWTSecret, c.Config.PublicURL, userID, duration)
					if err == nil {
						isSecure := strings.HasPrefix(c.Config.PublicURL, "https://")
						auth.SetTokenCookieWithDuration(w, newToken, isSecure, duration)
						sessionCtx = &AuthSessionContext{ExpiresAt: expiresAt.UTC(), IssuedAt: time.Now().UTC()}
					}
				}
			}
		}

		if allowAPIKey && userID == uuid.Nil {
			token := extractAPIClientToken(r)
			if token != "" {
				apiClientAuth, err = c.Store.GetAPIClientAuth(r.Context(), token)
				if err != nil {
					slog.Error("Failed to validate api client token", "error", err)
				} else if apiClientAuth != nil {
					userID = apiClientAuth.UserID
				}
			}
		}

		if userID == uuid.Nil && apiClientAuth == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), UserIDKey, userID)
		if apiClientAuth != nil {
			ctx = context.WithValue(ctx, APIClientAuthKey, apiClientAuth)
		}
		if sessionCtx != nil {
			ctx = context.WithValue(ctx, AuthSessionKey, *sessionCtx)
		}
		next(w, r.WithContext(ctx))
	}
}

func authSessionContextFromClaims(claims *auth.Claims) *AuthSessionContext {
	if claims == nil || claims.ExpiresAt == nil {
		return nil
	}

	session := &AuthSessionContext{ExpiresAt: claims.ExpiresAt.UTC()}
	if claims.IssuedAt != nil {
		session.IssuedAt = claims.IssuedAt.UTC()
	}
	return session
}

func extractAPIClientToken(r *http.Request) string {
	authorization := strings.TrimSpace(r.Header.Get("Authorization"))
	if authorization != "" {
		parts := strings.SplitN(authorization, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			token := strings.TrimSpace(parts[1])
			if token != "" {
				return token
			}
		}
	}

	return strings.TrimSpace(r.Header.Get("X-Api-Key"))
}

// WithRateLimit wraps a handler with IP-based rate limiting.
func (c *Context) WithRateLimit(limiter *IPRateLimiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := GetRealIP(r, c.Config.GetTrustedProxyNetworks())
		l := limiter.GetLimiter(ip)
		if !l.Allow() {
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		next(w, r)
	}
}
