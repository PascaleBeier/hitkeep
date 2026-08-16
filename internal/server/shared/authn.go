package shared

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/auth"
	"hitkeep/internal/database"
)

// AuthSessionContext carries the authenticated session's lifetime, set by
// RequireAuth for handlers that report session state.
type AuthSessionContext struct {
	ExpiresAt time.Time
	IssuedAt  time.Time
}

// RequireAPIClientAuth wraps a handler and accepts only API client tokens.
func (c *Context) RequireAPIClientAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := extractAPIClientToken(r)
		if token == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		apiClientAuth, err := c.Store.GetAPIClientAuth(r.Context(), token)
		if err != nil {
			LoggerFromContext(r.Context()).Error("Failed to validate api client token", "error", err)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if apiClientAuth == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), UserIDKey, apiClientAuth.UserID)
		ctx = context.WithValue(ctx, APIClientAuthKey, apiClientAuth)
		ctx = WithLoggerAttrs(ctx, "user_id", apiClientAuth.UserID.String())
		next(w, r.WithContext(ctx))
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
					LoggerFromContext(r.Context()).Error("Failed to validate api client token", "error", err)
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
		ctx = WithLoggerAttrs(ctx, "user_id", userID.String())
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
