package share

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
)

func (h *handler) parseShareWebVitalsParams(w http.ResponseWriter, r *http.Request, site *api.Site, requireMetric bool, defaultLimit int) (api.WebVitalsParams, bool) {
	q := r.URL.Query()
	start, end, ok := parseShareWebVitalsRange(w, q.Get("from"), q.Get("to"))
	if !ok {
		return api.WebVitalsParams{}, false
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
		SiteID: site.ID,
		Start:  start,
		End:    end,
		Metric: metric,
		Path:   strings.TrimSpace(q.Get("path")),
		Rating: rating,
		Limit:  limit,
	}, true
}

func parseShareWebVitalsRange(w http.ResponseWriter, fromStr, toStr string) (time.Time, time.Time, bool) {
	now := time.Now().UTC()
	end := now.AddDate(0, 0, 1)
	start := end.AddDate(0, 0, -30)

	if fromStr != "" {
		parsed, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			http.Error(w, "Invalid from", http.StatusBadRequest)
			return time.Time{}, time.Time{}, false
		}
		start = parsed
	}
	if toStr != "" {
		parsed, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			http.Error(w, "Invalid to", http.StatusBadRequest)
			return time.Time{}, time.Time{}, false
		}
		end = parsed
	}

	return start, end, true
}

func (h *handler) handleGetShareWebVitalsSummary() http.HandlerFunc {
	return h.handleGetShareWebVitals(false, 0, func(ctx context.Context, store *database.Store, params api.WebVitalsParams) (any, error) {
		return store.GetWebVitalsSummary(ctx, params)
	}, "summary")
}

func (h *handler) handleGetShareWebVitalsTimeseries() http.HandlerFunc {
	return h.handleGetShareWebVitals(true, 0, func(ctx context.Context, store *database.Store, params api.WebVitalsParams) (any, error) {
		return store.GetWebVitalsTimeseries(ctx, params)
	}, "timeseries")
}

func (h *handler) handleGetShareWebVitalsPages() http.HandlerFunc {
	return h.handleGetShareWebVitals(true, 25, func(ctx context.Context, store *database.Store, params api.WebVitalsParams) (any, error) {
		return store.GetWebVitalsPages(ctx, params)
	}, "pages")
}

func (h *handler) handleGetShareWebVitalsBreakdown() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		site, ok := h.loadShareSite(w, r)
		if !ok {
			return
		}
		if !h.ensureSiteMatch(w, r, site) {
			return
		}

		params, ok := h.parseShareWebVitalsParams(w, r, site, true, 25)
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
			slog.Error("Failed to get share web vitals breakdown", "error", err, "site_id", params.SiteID, "dimension", dimension)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}

func (h *handler) handleGetShareWebVitals(
	requireMetric bool,
	defaultLimit int,
	load func(context.Context, *database.Store, api.WebVitalsParams) (any, error),
	label string,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		site, ok := h.loadShareSite(w, r)
		if !ok {
			return
		}
		if !h.ensureSiteMatch(w, r, site) {
			return
		}

		params, ok := h.parseShareWebVitalsParams(w, r, site, requireMetric, defaultLimit)
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
			slog.Error("Failed to get share web vitals "+label, "error", err, "site_id", params.SiteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}
