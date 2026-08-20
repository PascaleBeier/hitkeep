package sites

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
	json "hitkeep/internal/jsonapi"
	"hitkeep/internal/server/filterparams"
	"hitkeep/internal/server/shared"
)

func (h *handler) handleGetSitesOverviewStats() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		apiClientAuth, _ := r.Context().Value(shared.APIClientAuthKey).(*database.APIClientAuth)
		if userID == uuid.Nil && apiClientAuth == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if userID == uuid.Nil && apiClientAuth != nil && apiClientAuth.TenantID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		sites, err := h.listAccessibleSites(r.Context(), userID, apiClientAuth)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to get overview sites", "error", err, "user_id", userID, "tenant_id", apiClientAuthTenantID(apiClientAuth))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		start, end := parseOverviewStatsRange(r.URL.Query())
		response := api.SitesOverviewStatsResponse{
			Sites: make([]api.SiteOverviewStats, 0, len(sites)),
		}

		for _, site := range sites {
			analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), site.ID)
			if err != nil {
				shared.LoggerFromContext(r.Context()).Error("Failed to resolve overview analytics store", "error", err, "site_id", site.ID)
				response.Sites = append(response.Sites, overviewStatsError(site.ID))
				continue
			}

			stats, err := analyticsStore.GetSiteOverviewStats(r.Context(), api.AnalyticsParams{
				SiteID: site.ID,
				UserID: userID,
				Start:  start,
				End:    end,
			})
			if err != nil {
				shared.LoggerFromContext(r.Context()).Error("Failed to get overview site stats", "error", err, "site_id", site.ID)
				response.Sites = append(response.Sites, overviewStatsError(site.ID))
				continue
			}
			response.Sites = append(response.Sites, *stats)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, response); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode response", "error", err)
		}
	}
}

func (h *handler) listAccessibleSites(ctx context.Context, userID uuid.UUID, apiClientAuth *database.APIClientAuth) ([]api.Site, error) {
	var (
		sites []api.Site
		err   error
	)
	switch {
	case userID != uuid.Nil:
		sites, err = h.ctx.Store.GetSites(ctx, userID)
	case apiClientAuth != nil && apiClientAuth.TenantID != uuid.Nil:
		sites, err = h.ctx.Store.ListSitesForTenant(ctx, apiClientAuth.TenantID)
	default:
		return nil, nil
	}
	if err != nil {
		return nil, err
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

	return sites, nil
}

func parseOverviewStatsRange(q url.Values) (time.Time, time.Time) {
	now := time.Now().UTC()
	end := now.AddDate(0, 0, 1)
	start := end.AddDate(0, 0, -30)

	if fromStr := q.Get("from"); fromStr != "" {
		if parsed, err := time.Parse(time.RFC3339, fromStr); err == nil {
			start = parsed
		}
	}
	if toStr := q.Get("to"); toStr != "" {
		if parsed, err := time.Parse(time.RFC3339, toStr); err == nil {
			end = parsed
		}
	}
	return start, end
}

func overviewStatsError(siteID uuid.UUID) api.SiteOverviewStats {
	return api.SiteOverviewStats{
		SiteID:    siteID,
		Status:    api.SiteOverviewStatsError,
		ChartData: []api.ChartDataPoint{},
		Error:     "stats_unavailable",
	}
}

func (h *handler) handleGetSiteStats() http.HandlerFunc {
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

		siteIDStr := r.PathValue("id")
		siteID, err := uuid.Parse(siteIDStr)
		if err != nil {
			http.Error(w, "Invalid site_id", http.StatusBadRequest)
			return
		}

		window, ok := parseAnalyticsWindow(w, r)
		if !ok {
			return
		}
		start, end, filters := window.Start, window.End, window.Filters

		q := r.URL.Query()
		var goalIDs []uuid.UUID
		for _, rawID := range q["goal_id"] {
			id, err := uuid.Parse(rawID)
			if err != nil {
				http.Error(w, "Invalid goal_id", http.StatusBadRequest)
				return
			}
			goalIDs = append(goalIDs, id)
		}

		var funnelIDs []uuid.UUID
		for _, rawID := range q["funnel_id"] {
			id, err := uuid.Parse(rawID)
			if err != nil {
				http.Error(w, "Invalid funnel_id", http.StatusBadRequest)
				return
			}
			funnelIDs = append(funnelIDs, id)
		}

		params := api.AnalyticsParams{
			SiteID:    siteID,
			UserID:    userID,
			Start:     start,
			End:       end,
			Filters:   filters,
			GoalIDs:   goalIDs,
			FunnelIDs: funnelIDs,
		}

		params.CompareStart, params.CompareEnd = filterparams.ParseComparisonRange(q.Get("compare_from"), q.Get("compare_to"))

		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), siteID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to resolve analytics store", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		stats, err := analyticsStore.GetSiteStats(r.Context(), params)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to get site stats", "error", err, "site_id", siteID)
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, "Site not found", http.StatusNotFound)
			} else {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, stats); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode response", "error", err)
		}
	}
}

func (h *handler) parseEcommerceParams(w http.ResponseWriter, r *http.Request, defaultLimit int) (api.EcommerceParams, bool) {
	if h.ctx.Store == nil {
		http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
		return api.EcommerceParams{}, false
	}

	siteIDStr := r.PathValue("id")
	siteID, err := uuid.Parse(siteIDStr)
	if err != nil {
		http.Error(w, "Invalid site_id", http.StatusBadRequest)
		return api.EcommerceParams{}, false
	}

	now := time.Now().UTC()
	end := now.AddDate(0, 0, 1)
	start := end.AddDate(0, 0, -30)
	q := r.URL.Query()

	if fromStr := q.Get("from"); fromStr != "" {
		if parsed, err := time.Parse(time.RFC3339, fromStr); err == nil {
			start = parsed
		}
	}
	if toStr := q.Get("to"); toStr != "" {
		if parsed, err := time.Parse(time.RFC3339, toStr); err == nil {
			end = parsed
		}
	}

	filters, err := parseFilters(q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return api.EcommerceParams{}, false
	}

	limit := defaultLimit
	if rawLimit := q.Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil {
			http.Error(w, "Invalid limit", http.StatusBadRequest)
			return api.EcommerceParams{}, false
		}
		limit = parsed
	}

	return api.EcommerceParams{
		SiteID:   siteID,
		Start:    start,
		End:      end,
		Filters:  filters,
		ItemID:   strings.TrimSpace(q.Get("item_id")),
		ItemName: strings.TrimSpace(q.Get("item_name")),
		Limit:    limit,
	}, true
}

func (h *handler) handleGetSiteEcommerceSummary() http.HandlerFunc {
	return h.handleGetSiteEcommerce(func(ctx context.Context, store *database.Store, params api.EcommerceParams) (any, error) {
		return store.GetEcommerceSummary(ctx, params)
	}, "summary")
}

func (h *handler) handleGetSiteEcommerceTimeseries() http.HandlerFunc {
	return h.handleGetSiteEcommerce(func(ctx context.Context, store *database.Store, params api.EcommerceParams) (any, error) {
		return store.GetEcommerceTimeSeries(ctx, params)
	}, "timeseries")
}

func (h *handler) handleGetSiteEcommerceProducts() http.HandlerFunc {
	return h.handleGetSiteEcommerce(func(ctx context.Context, store *database.Store, params api.EcommerceParams) (any, error) {
		return store.GetEcommerceTopProducts(ctx, params)
	}, "products")
}

func (h *handler) handleGetSiteEcommerceSources() http.HandlerFunc {
	return h.handleGetSiteEcommerce(func(ctx context.Context, store *database.Store, params api.EcommerceParams) (any, error) {
		return store.GetEcommerceSources(ctx, params)
	}, "sources")
}

func (h *handler) handleGetSiteEcommerce(load func(context.Context, *database.Store, api.EcommerceParams) (any, error), label string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		params, ok := h.parseEcommerceParams(w, r, 10)
		if !ok {
			return
		}

		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), params.SiteID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to resolve analytics store", "error", err, "site_id", params.SiteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		payload, err := load(r.Context(), analyticsStore, params)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to get ecommerce "+label, "error", err, "site_id", params.SiteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, payload); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode response", "error", err)
		}
	}
}

// analyticsWindow holds the from/to/filter triple every hit-backed analytics
// endpoint accepts.
type analyticsWindow struct {
	Start   time.Time
	End     time.Time
	Filters []api.Filter
}

// parseAnalyticsWindow resolves the shared from/to/filter query parameters and
// writes the error response itself when one is malformed. Site stats and the AI
// traffic series share it so their defaults and validation cannot drift apart.
func parseAnalyticsWindow(w http.ResponseWriter, r *http.Request) (analyticsWindow, bool) {
	q := r.URL.Query()
	start, end, ok := filterparams.ParseAnalyticsRange(w, q.Get("from"), q.Get("to"))
	if !ok {
		return analyticsWindow{}, false
	}

	filters, err := parseFilters(q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return analyticsWindow{}, false
	}

	return analyticsWindow{Start: start, End: end, Filters: filters}, true
}

func parseFilters(q url.Values) ([]api.Filter, error) {
	return filterparams.ParseHitFilters(q, filterparams.LegacyPair{
		TypeParam:          "filter_type",
		ValueParam:         "filter_value",
		MissingMessage:     "filter_type and filter_value are required together",
		InvalidTypeMessage: "invalid filter_type",
	})
}
