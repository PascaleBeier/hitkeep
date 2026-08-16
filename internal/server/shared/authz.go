package shared

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"hitkeep/internal/auth"
	"hitkeep/internal/database"
)

// PermissionContext carries the resolved roles of the authenticated caller,
// set by the permission middleware.
type PermissionContext struct {
	UserID       uuid.UUID
	InstanceRole auth.InstanceRole
	SiteRole     auth.SiteRole // Only set if checking site permission.
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

			instanceRole, err := c.resolveInstanceRole(r.Context(), userID, apiClientAuth)
			if err != nil {
				LoggerFromContext(r.Context()).Error("Failed to get instance role", "error", err)
				http.Error(w, "Internal error", http.StatusInternalServerError)
				return
			}

			// API clients need an explicit site grant for every site-scoped route.
			if isSitePermission(perm) && apiClientAuth != nil {
				siteID, err := siteIDFromRequest(r)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}

				siteRole, err := c.resolveSiteRole(r.Context(), userID, apiClientAuth, siteID)
				if err != nil {
					http.Error(w, "Access denied", http.StatusForbidden)
					return
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

			// Check instance-level permission for human sessions and instance-scoped API routes.
			if instanceRole.HasPermission(perm) {
				ctx := context.WithValue(r.Context(), PermissionKey, PermissionContext{
					UserID:       userID,
					InstanceRole: instanceRole,
				})
				next(w, r.WithContext(ctx))
				return
			}

			// For site-level human-session permissions, check site role after instance role.
			if isSitePermission(perm) {
				siteID, err := siteIDFromRequest(r)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}

				siteRole, err := c.resolveSiteRole(r.Context(), userID, apiClientAuth, siteID)
				if err != nil {
					http.Error(w, "Access denied", http.StatusForbidden)
					return
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

func (c *Context) RequireTeamCapability(capability auth.Capability) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			userID := GetUserIDFromContext(r)
			if userID == uuid.Nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			teamID, ok := teamIDFromRequest(r, w)
			if !ok {
				return
			}

			if !c.userHasTeamCapability(r.Context(), teamID, userID, capability) {
				http.Error(w, "Access denied", http.StatusForbidden)
				return
			}

			next(w, r)
		}
	}
}

func teamIDFromRequest(r *http.Request, w http.ResponseWriter) (uuid.UUID, bool) {
	teamID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		http.Error(w, "Invalid team ID", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return teamID, true
}

func (c *Context) userHasTeamCapability(ctx context.Context, teamID, userID uuid.UUID, capability auth.Capability) bool {
	role, err := c.Store.GetTenantRole(ctx, teamID, userID)
	return err == nil && auth.TeamRoleHasCapability(role, capability)
}

func (c *Context) sitePermissionContext(
	ctx context.Context,
	userID uuid.UUID,
	apiClientAuth *database.APIClientAuth,
	instanceRole auth.InstanceRole,
	siteID uuid.UUID,
	perm auth.Permission,
) (context.Context, bool, error) {
	siteRole, err := c.resolveSiteRole(ctx, userID, apiClientAuth, siteID)
	if err != nil {
		return nil, false, err
	}
	if !siteRole.HasPermission(perm) {
		return nil, false, nil
	}
	return context.WithValue(ctx, PermissionKey, PermissionContext{
		UserID:       userID,
		InstanceRole: instanceRole,
		SiteRole:     siteRole,
	}), true, nil
}

// RequireSiteOrInstancePermission allows route-specific exceptions where a site
// permission can also be satisfied by a narrow instance-level permission.
func (c *Context) RequireSiteOrInstancePermission(sitePerm, instancePerm auth.Permission) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			userID := GetUserIDFromContext(r)
			apiClientAuth, _ := r.Context().Value(APIClientAuthKey).(*database.APIClientAuth)
			if userID == uuid.Nil && apiClientAuth == nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			instanceRole, err := c.resolveInstanceRole(r.Context(), userID, apiClientAuth)
			if err != nil {
				LoggerFromContext(r.Context()).Error("Failed to get instance role", "error", err)
				http.Error(w, "Internal error", http.StatusInternalServerError)
				return
			}

			if sitePerm != "" {
				siteID, err := siteIDFromRequest(r)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}

				siteCtx, ok, err := c.sitePermissionContext(r.Context(), userID, apiClientAuth, instanceRole, siteID, sitePerm)
				if err != nil {
					if apiClientAuth != nil {
						http.Error(w, "Access denied", http.StatusForbidden)
						return
					}
				}
				if ok {
					next(w, r.WithContext(siteCtx))
					return
				}
				if apiClientAuth != nil {
					http.Error(w, "Forbidden", http.StatusForbidden)
					return
				}
			}

			if instancePerm != "" && instanceRole.HasPermission(instancePerm) {
				ctx := context.WithValue(r.Context(), PermissionKey, PermissionContext{
					UserID:       userID,
					InstanceRole: instanceRole,
				})
				next(w, r.WithContext(ctx))
				return
			}

			http.Error(w, "Forbidden", http.StatusForbidden)
		}
	}
}

func isSitePermission(perm auth.Permission) bool {
	return strings.HasPrefix(string(perm), "site.")
}

func (c *Context) resolveInstanceRole(ctx context.Context, userID uuid.UUID, apiClientAuth *database.APIClientAuth) (auth.InstanceRole, error) {
	instanceRole := auth.InstanceUser
	if userID != uuid.Nil {
		var err error
		instanceRole, err = c.Store.GetInstanceRole(ctx, userID)
		if err != nil {
			return auth.InstanceUser, err
		}
	}
	if apiClientAuth != nil {
		instanceRole = auth.MinInstanceRole(instanceRole, apiClientAuth.InstanceRole)
	}
	return instanceRole, nil
}

func (c *Context) resolveSiteRole(ctx context.Context, userID uuid.UUID, apiClientAuth *database.APIClientAuth, siteID uuid.UUID) (auth.SiteRole, error) {
	var siteRole auth.SiteRole
	if userID != uuid.Nil {
		var err error
		siteRole, err = c.Store.GetSiteRole(ctx, userID, siteID)
		if err != nil {
			return "", err
		}
	}

	if apiClientAuth != nil {
		delegatedRole, ok := apiClientAuth.SiteRoles[siteID]
		if !ok {
			return "", fmt.Errorf("api client is not allowed for site %s", siteID)
		}
		if userID == uuid.Nil {
			siteRole = delegatedRole
		} else {
			siteRole = auth.MinSiteRole(siteRole, delegatedRole)
		}
	}

	return siteRole, nil
}

func siteIDFromRequest(r *http.Request) (uuid.UUID, error) {
	siteIDStr := r.PathValue("id")
	if siteIDStr == "" {
		return uuid.Nil, fmt.Errorf("site ID required")
	}

	siteID, err := uuid.Parse(siteIDStr)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid site ID")
	}

	return siteID, nil
}
