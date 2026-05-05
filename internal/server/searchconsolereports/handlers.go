package searchconsolereports

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
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

func Register(mux *http.ServeMux, ctx *shared.Context) {
	h := &handler{ctx: ctx}
	mux.HandleFunc("GET /api/sites/{id}/search-console/overview", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetOverview()))
	mux.HandleFunc("GET /api/sites/{id}/search-console/series", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetSeries()))
	mux.HandleFunc("GET /api/sites/{id}/search-console/queries", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetDimension("query")))
	mux.HandleFunc("GET /api/sites/{id}/search-console/pages", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetDimension("page")))
	mux.HandleFunc("GET /api/sites/{id}/search-console/breakdowns", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetBreakdown()))
}

func (h *handler) handleGetOverview() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		overview, params, ok, err := loadReport(h, w, r, func(store *database.Store, params api.SearchConsoleReportParams) (api.SearchConsoleOverview, error) {
			return store.GetSearchConsoleOverview(r.Context(), params)
		})
		if !ok {
			return
		}
		if err != nil {
			slog.Error("Failed to get Search Console overview", "error", err, "site_id", params.SiteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(overview); err != nil {
			slog.Error("Failed to encode Search Console overview", "error", err, "site_id", params.SiteID)
		}
	}
}

func (h *handler) handleGetSeries() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		series, params, ok, err := loadReport(h, w, r, func(store *database.Store, params api.SearchConsoleReportParams) (api.SearchConsoleSeriesResponse, error) {
			return store.GetSearchConsoleSeries(r.Context(), params)
		})
		if !ok {
			return
		}
		if err != nil {
			slog.Error("Failed to get Search Console series", "error", err, "site_id", params.SiteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(series); err != nil {
			slog.Error("Failed to encode Search Console series", "error", err, "site_id", params.SiteID)
		}
	}
}

func (h *handler) handleGetDimension(dimension string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, params, ok, err := loadReport(h, w, r, func(store *database.Store, params api.SearchConsoleReportParams) (api.SearchConsoleDimensionResponse, error) {
			return store.GetSearchConsoleDimension(r.Context(), params, dimension)
		})
		if !ok {
			return
		}
		if err != nil {
			slog.Error("Failed to get Search Console dimension rows", "error", err, "site_id", params.SiteID, "dimension", dimension)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(rows); err != nil {
			slog.Error("Failed to encode Search Console dimension rows", "error", err, "site_id", params.SiteID, "dimension", dimension)
		}
	}
}

func loadReport[T any](h *handler, w http.ResponseWriter, r *http.Request, load func(*database.Store, api.SearchConsoleReportParams) (T, error)) (T, api.SearchConsoleReportParams, bool, error) {
	params, ok := h.parseReportParams(w, r)
	if !ok {
		var zero T
		return zero, api.SearchConsoleReportParams{}, false, nil
	}
	analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), params.SiteID)
	if err != nil {
		slog.Error("Failed to resolve Search Console analytics store", "error", err, "site_id", params.SiteID)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		var zero T
		return zero, params, false, nil
	}
	result, err := load(analyticsStore, params)
	return result, params, true, err
}

func (h *handler) handleGetBreakdown() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dimension := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("dimension")))
		if dimension != "country" && dimension != "device" {
			http.Error(w, "Invalid dimension", http.StatusBadRequest)
			return
		}
		h.handleGetDimension(dimension).ServeHTTP(w, r)
	}
}

func (h *handler) parseReportParams(w http.ResponseWriter, r *http.Request) (api.SearchConsoleReportParams, bool) {
	if h.ctx.Store == nil {
		http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
		return api.SearchConsoleReportParams{}, false
	}
	siteID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid site_id", http.StatusBadRequest)
		return api.SearchConsoleReportParams{}, false
	}
	mapping, err := h.searchConsoleMappingForReport(r, siteID)
	if err != nil {
		slog.Error("Failed to resolve Search Console mapping for report", "error", err, "site_id", siteID)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return api.SearchConsoleReportParams{}, false
	}
	if mapping == nil {
		http.Error(w, "Search Console property is not mapped", http.StatusPreconditionFailed)
		return api.SearchConsoleReportParams{}, false
	}

	now := time.Now().UTC()
	params := api.SearchConsoleReportParams{
		SiteID:      siteID,
		PropertyURI: mapping.PropertyURI,
		Start:       now.AddDate(0, 0, -30),
		End:         now,
		Page:        strings.TrimSpace(r.URL.Query().Get("page")),
		Path:        strings.TrimSpace(r.URL.Query().Get("path")),
		Country:     strings.TrimSpace(r.URL.Query().Get("country")),
		Device:      strings.TrimSpace(r.URL.Query().Get("device")),
		Limit:       10,
	}
	if from := strings.TrimSpace(r.URL.Query().Get("from")); from != "" {
		parsed, err := time.Parse(time.RFC3339, from)
		if err != nil {
			http.Error(w, "Invalid from date, expected RFC3339", http.StatusBadRequest)
			return api.SearchConsoleReportParams{}, false
		}
		params.Start = parsed
	}
	if to := strings.TrimSpace(r.URL.Query().Get("to")); to != "" {
		parsed, err := time.Parse(time.RFC3339, to)
		if err != nil {
			http.Error(w, "Invalid to date, expected RFC3339", http.StatusBadRequest)
			return api.SearchConsoleReportParams{}, false
		}
		params.End = parsed
	}
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil || limit < 1 || limit > 100 {
			http.Error(w, "Invalid limit, expected integer between 1 and 100", http.StatusBadRequest)
			return api.SearchConsoleReportParams{}, false
		}
		params.Limit = limit
	}
	return params, true
}

func (h *handler) searchConsoleMappingForReport(r *http.Request, siteID uuid.UUID) (*database.GoogleSearchConsoleSiteMapping, error) {
	teamID, err := h.ctx.Store.GetSiteTenantID(r.Context(), siteID)
	if err != nil {
		return nil, err
	}
	return h.ctx.Store.GetGoogleSearchConsoleSiteMappingForTeam(r.Context(), siteID, teamID)
}
