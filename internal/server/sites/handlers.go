package sites

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	authcore "hitkeep/internal/auth"
	"hitkeep/internal/database"
	"hitkeep/internal/server/shared"
)

type handler struct {
	ctx *shared.Context
}

var faviconProxyTransport = newFaviconProxyTransport(5 * time.Second)

func Register(mux *http.ServeMux, ctx *shared.Context) {
	h := &handler{ctx: ctx}
	mux.HandleFunc("GET /api/sites", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		AllowAPIKey: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetSites()))
	mux.HandleFunc("POST /api/sites", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleCreateSite()))
	mux.HandleFunc("DELETE /api/sites/{id}", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteDelete,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleDeleteSite()))
	mux.HandleFunc("GET /api/sites/{id}/stats", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetSiteStats()))
	mux.HandleFunc("GET /api/sites/{id}/hits", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetSiteHits()))
	mux.HandleFunc("GET /api/sites/{id}/hits/export", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleExportSiteHits()))
	mux.HandleFunc("GET /api/sites/{id}/ecommerce", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetSiteEcommerceSummary()))
	mux.HandleFunc("GET /api/sites/{id}/ecommerce/timeseries", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetSiteEcommerceTimeseries()))
	mux.HandleFunc("GET /api/sites/{id}/ecommerce/products", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetSiteEcommerceProducts()))
	mux.HandleFunc("GET /api/sites/{id}/ecommerce/sources", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetSiteEcommerceSources()))
	mux.HandleFunc("GET /api/favicon/{domain}", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetFavicon()))
	mux.HandleFunc("PUT /api/sites/{id}/retention", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteManageData,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleUpdateSiteRetention()))
	mux.HandleFunc("POST /api/sites/{id}/transfer-team", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteManageTeam,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleTransferSiteTeam()))
	mux.HandleFunc("GET /api/sites/{id}/exclusions", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteManageData,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleListSiteExclusions()))
	mux.HandleFunc("POST /api/sites/{id}/exclusions", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteManageData,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleCreateSiteExclusion()))
	mux.HandleFunc("DELETE /api/sites/{id}/exclusions/{ruleID}", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteManageData,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleDeleteSiteExclusion()))
}

var domainRegex = regexp.MustCompile(`^(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)

func canManageTenantRole(role string) bool {
	switch strings.TrimSpace(strings.ToLower(role)) {
	case database.TenantRoleOwner, database.TenantRoleAdmin:
		return true
	default:
		return false
	}
}

func (h *handler) handleGetSites() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		apiClientAuth, _ := r.Context().Value(shared.APIClientAuthKey).(*database.APIClientAuth)
		if userID == uuid.Nil && apiClientAuth == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		var (
			sites []api.Site
			err   error
		)
		switch {
		case userID != uuid.Nil:
			sites, err = h.ctx.Store.GetSites(r.Context(), userID)
		case apiClientAuth != nil && apiClientAuth.TenantID != uuid.Nil:
			sites, err = h.ctx.Store.ListSitesForTenant(r.Context(), apiClientAuth.TenantID)
		default:
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if err != nil {
			slog.Error("Failed to get sites", "error", err, "user_id", userID, "tenant_id", apiClientAuthTenantID(apiClientAuth))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if apiClientAuth != nil {
			filtered := make([]api.Site, 0, len(sites))
			for _, site := range sites {
				if _, allowed := apiClientAuth.SiteRoles[site.ID]; allowed {
					filtered = append(filtered, site)
				}
			}
			sites = filtered
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(sites); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}

func apiClientAuthTenantID(authz *database.APIClientAuth) uuid.UUID {
	if authz == nil {
		return uuid.Nil
	}
	return authz.TenantID
}

func (h *handler) handleCreateSite() http.HandlerFunc {
	type request struct {
		Domain string `json:"domain"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		domain := strings.ToLower(strings.TrimSpace(req.Domain))
		if domain == "" {
			http.Error(w, "Domain is required", http.StatusBadRequest)
			return
		}

		if strings.Contains(domain, "://") {
			http.Error(w, "Domain must not contain protocol (http:// or https://)", http.StatusBadRequest)
			return
		}

		if strings.HasPrefix(domain, "www.") {
			http.Error(w, "Domain must not start with 'www.' (we track subdomains automatically)", http.StatusBadRequest)
			return
		}

		if len(domain) > 253 || !domainRegex.MatchString(domain) {
			http.Error(w, "Invalid domain format (e.g. example.com)", http.StatusBadRequest)
			return
		}

		site, err := h.ctx.Store.CreateSite(r.Context(), userID, domain)
		if err != nil {
			slog.Error("Failed to create site", "error", err, "domain", domain)
			http.Error(w, "Failed to create site (domain might already exist)", http.StatusConflict)
			return
		}

		if h.ctx.Config.DataRetentionDays > 0 {
			if err := h.ctx.Store.UpdateSiteRetention(r.Context(), site.ID, userID, h.ctx.Config.DataRetentionDays); err != nil {
				slog.Warn("Failed to set default data retention policy", "site_id", site.ID, "error", err)
			}
		}

		if h.ctx.TenantStores != nil {
			if err := h.ctx.TenantStores.SyncSite(r.Context(), site.ID); err != nil {
				slog.Error("Failed to sync tenant site mirror after create", "error", err, "site_id", site.ID)
				http.Error(w, "Failed to create site", http.StatusInternalServerError)
				return
			}
		}

		slog.Info("Site created", "id", site.ID, "domain", domain, "user_id", userID)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(site); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}

func (h *handler) handleDeleteSite() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		siteIDStr := r.PathValue("id")
		siteID, err := uuid.Parse(siteIDStr)
		if err != nil {
			http.Error(w, "Invalid site_id", http.StatusBadRequest)
			return
		}

		if h.ctx.TenantStores != nil {
			if err := h.ctx.TenantStores.DeleteSite(r.Context(), siteID); err != nil {
				slog.Error("Failed to delete site", "error", err, "site_id", siteID)
				http.Error(w, "Failed to delete site", http.StatusInternalServerError)
				return
			}
		} else if err := h.ctx.Store.DeleteSite(r.Context(), siteID); err != nil {
			slog.Error("Failed to delete site", "error", err, "site_id", siteID)
			http.Error(w, "Failed to delete site", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}

func (h *handler) handleUpdateSiteRetention() http.HandlerFunc {
	type request struct {
		Days int `json:"days"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		siteIDStr := r.PathValue("id")
		siteID, err := uuid.Parse(siteIDStr)
		if err != nil {
			http.Error(w, "Invalid site_id", http.StatusBadRequest)
			return
		}

		site, err := h.ctx.Store.GetSite(r.Context(), siteID, userID)
		if err != nil || site == nil {
			http.Error(w, "Site not found", http.StatusNotFound)
			return
		}

		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if req.Days < 0 {
			http.Error(w, "Retention days must be non-negative", http.StatusBadRequest)
			return
		}

		if err := h.ctx.Store.UpdateSiteRetention(r.Context(), siteID, userID, req.Days); err != nil {
			slog.Error("Failed to update site retention", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if h.ctx.TenantStores != nil {
			if err := h.ctx.TenantStores.SyncSite(r.Context(), siteID); err != nil {
				slog.Error("Failed to sync tenant site mirror after retention update", "error", err, "site_id", siteID)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
		}

		w.WriteHeader(http.StatusOK)
	}
}

func (h *handler) handleTransferSiteTeam() http.HandlerFunc {
	type request struct {
		TeamID string `json:"team_id"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil || h.ctx.TenantStores == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		siteID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
		if err != nil {
			http.Error(w, "Invalid site_id", http.StatusBadRequest)
			return
		}

		var req request
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		destinationTeamID, err := uuid.Parse(strings.TrimSpace(req.TeamID))
		if err != nil {
			http.Error(w, "Invalid team_id", http.StatusBadRequest)
			return
		}

		destinationRole, err := h.ctx.Store.GetTenantRole(r.Context(), destinationTeamID, userID)
		if err != nil || !canManageTenantRole(destinationRole) {
			http.Error(w, "Access denied", http.StatusForbidden)
			return
		}

		sourceTeamID, err := h.ctx.Store.GetSiteTenantID(r.Context(), siteID)
		if err != nil {
			slog.Error("Failed to resolve source team for site transfer", "error", err, "site_id", siteID, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if err := h.ctx.TenantStores.TransferSite(r.Context(), siteID, destinationTeamID); err != nil {
			slog.Error("Failed to transfer site to team", "error", err, "site_id", siteID, "source_team_id", sourceTeamID, "destination_team_id", destinationTeamID, "user_id", userID)
			http.Error(w, "Failed to transfer site", http.StatusInternalServerError)
			return
		}

		site, _ := h.ctx.Store.GetSiteByID(r.Context(), siteID)
		siteLabel := siteID.String()
		if site != nil && site.Domain != "" {
			siteLabel = site.Domain
		}
		if err := h.ctx.Store.AppendTeamAuditEntry(r.Context(), sourceTeamID, userID, "site.transferred_out", fmt.Sprintf("Site %s moved to team %s", siteLabel, destinationTeamID), nil); err != nil {
			slog.Warn("Failed to append source team site transfer audit entry", "error", err, "site_id", siteID, "team_id", sourceTeamID)
		}
		if err := h.ctx.Store.AppendTeamAuditEntry(r.Context(), destinationTeamID, userID, "site.transferred_in", fmt.Sprintf("Site %s moved into this team", siteLabel), nil); err != nil {
			slog.Warn("Failed to append destination team site transfer audit entry", "error", err, "site_id", siteID, "team_id", destinationTeamID)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"status":              "ok",
			"site_id":             siteID,
			"source_team_id":      sourceTeamID,
			"destination_team_id": destinationTeamID,
		}); err != nil {
			slog.Error("Failed to encode site transfer response", "error", err, "site_id", siteID, "user_id", userID)
		}
	}
}
