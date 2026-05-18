package sites

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
)

func (h *handler) parseWebVitalsParams(w http.ResponseWriter, r *http.Request, requireMetric bool, defaultLimit int) (api.WebVitalsParams, bool) {
	if h.ctx.Store == nil {
		http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
		return api.WebVitalsParams{}, false
	}

	siteID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid site_id", http.StatusBadRequest)
		return api.WebVitalsParams{}, false
	}

	now := time.Now().UTC()
	end := now.AddDate(0, 0, 1)
	start := end.AddDate(0, 0, -30)
	q := r.URL.Query()
	if fromStr := q.Get("from"); fromStr != "" {
		parsed, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			http.Error(w, "Invalid from", http.StatusBadRequest)
			return api.WebVitalsParams{}, false
		}
		start = parsed
	}
	if toStr := q.Get("to"); toStr != "" {
		parsed, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			http.Error(w, "Invalid to", http.StatusBadRequest)
			return api.WebVitalsParams{}, false
		}
		end = parsed
	}

	metric := api.WebVitalMetric(strings.TrimSpace(q.Get("metric")))
	if metric != "" {
		if _, err := database.WebVitalRatingForValue(metric, 0); err != nil {
			http.Error(w, "Invalid metric", http.StatusBadRequest)
			return api.WebVitalsParams{}, false
		}
	} else if requireMetric {
		http.Error(w, "metric is required", http.StatusBadRequest)
		return api.WebVitalsParams{}, false
	}

	rating := api.WebVitalRating(strings.TrimSpace(q.Get("rating")))
	if rating != "" {
		switch rating {
		case api.WebVitalRatingGood, api.WebVitalRatingNeedsImprovement, api.WebVitalRatingPoor:
		default:
			http.Error(w, "Invalid rating", http.StatusBadRequest)
			return api.WebVitalsParams{}, false
		}
	}

	limit := defaultLimit
	if rawLimit := q.Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil {
			http.Error(w, "Invalid limit", http.StatusBadRequest)
			return api.WebVitalsParams{}, false
		}
		limit = parsed
	}

	return api.WebVitalsParams{
		SiteID: siteID,
		Start:  start,
		End:    end,
		Metric: metric,
		Path:   strings.TrimSpace(q.Get("path")),
		Rating: rating,
		Limit:  limit,
	}, true
}

func (h *handler) handleGetSiteWebVitalsSummary() http.HandlerFunc {
	return h.handleGetSiteWebVitals(false, 0, func(ctx context.Context, store *database.Store, params api.WebVitalsParams) (any, error) {
		return store.GetWebVitalsSummary(ctx, params)
	}, "summary")
}

func (h *handler) handleGetSiteWebVitalsTimeseries() http.HandlerFunc {
	return h.handleGetSiteWebVitals(true, 0, func(ctx context.Context, store *database.Store, params api.WebVitalsParams) (any, error) {
		return store.GetWebVitalsTimeseries(ctx, params)
	}, "timeseries")
}

func (h *handler) handleGetSiteWebVitalsPages() http.HandlerFunc {
	return h.handleGetSiteWebVitals(true, 25, func(ctx context.Context, store *database.Store, params api.WebVitalsParams) (any, error) {
		return store.GetWebVitalsPages(ctx, params)
	}, "pages")
}

func (h *handler) handleGetSiteWebVitalsBreakdown() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		params, ok := h.parseWebVitalsParams(w, r, true, 25)
		if !ok {
			return
		}
		dimension := api.WebVitalDimension(strings.TrimSpace(r.URL.Query().Get("dimension")))
		switch dimension {
		case api.WebVitalDimensionCountry, api.WebVitalDimensionLanguage, api.WebVitalDimensionBrowser, api.WebVitalDimensionDevice, api.WebVitalDimensionCity, api.WebVitalDimensionProvider, api.WebVitalDimensionASN:
		default:
			http.Error(w, "Invalid dimension", http.StatusBadRequest)
			return
		}

		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), params.SiteID)
		if err != nil {
			slog.Error("Failed to resolve analytics store", "error", err, "site_id", params.SiteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		payload, err := analyticsStore.GetWebVitalsBreakdown(r.Context(), params, dimension)
		if err != nil {
			slog.Error("Failed to get web vitals breakdown", "error", err, "site_id", params.SiteID, "dimension", dimension)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}

func (h *handler) handleGetSiteWebVitals(
	requireMetric bool,
	defaultLimit int,
	load func(context.Context, *database.Store, api.WebVitalsParams) (any, error),
	label string,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		params, ok := h.parseWebVitalsParams(w, r, requireMetric, defaultLimit)
		if !ok {
			return
		}

		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), params.SiteID)
		if err != nil {
			slog.Error("Failed to resolve analytics store", "error", err, "site_id", params.SiteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		payload, err := load(r.Context(), analyticsStore, params)
		if err != nil {
			slog.Error("Failed to get web vitals "+label, "error", err, "site_id", params.SiteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}
