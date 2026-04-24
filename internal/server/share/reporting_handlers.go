package share

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
)

func (h *handler) handleGetShareGoals() http.HandlerFunc {
	return h.handleGetShareDefinitions(
		func(ctx context.Context, store *database.Store, siteID uuid.UUID) (any, error) {
			return store.GetGoals(ctx, siteID)
		},
		"Failed to get share goals",
	)
}

func (h *handler) handleGetShareGoalTimeseries() http.HandlerFunc {
	return h.handleTimeseries("goal_id", "Invalid goal_id", "Failed to get share goal timeseries",
		func(ctx context.Context, store *database.Store, params api.AnalyticsParams, ids []uuid.UUID) (any, error) {
			return store.GetGoalTimeseries(ctx, params, ids)
		})
}

func (h *handler) handleGetShareFunnels() http.HandlerFunc {
	return h.handleGetShareDefinitions(
		func(ctx context.Context, store *database.Store, siteID uuid.UUID) (any, error) {
			return store.GetFunnels(ctx, siteID)
		},
		"Failed to get share funnels",
	)
}

func (h *handler) handleGetShareDefinitions(
	fetch func(context.Context, *database.Store, uuid.UUID) (any, error),
	logMessage string,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		site, ok := h.loadShareSite(w, r)
		if !ok {
			return
		}
		if !h.ensureSiteMatch(w, r, site) {
			return
		}

		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), site.ID)
		if err != nil {
			slog.Error("Failed to resolve analytics store", "error", err, "site_id", site.ID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		definitions, err := fetch(r.Context(), analyticsStore, site.ID)
		if err != nil {
			slog.Error(logMessage, "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(definitions); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}

func (h *handler) handleGetShareFunnelTimeseries() http.HandlerFunc {
	return h.handleTimeseries("funnel_id", "Invalid funnel_id", "Failed to get share funnel timeseries",
		func(ctx context.Context, store *database.Store, params api.AnalyticsParams, ids []uuid.UUID) (any, error) {
			return store.GetFunnelTimeseries(ctx, params, ids)
		})
}

func (h *handler) handleGetShareFunnelStats() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		site, ok := h.loadShareSite(w, r)
		if !ok {
			return
		}
		if !h.ensureSiteMatch(w, r, site) {
			return
		}

		funnelIDStr := r.PathValue("funnelID")
		funnelID, err := uuid.Parse(funnelIDStr)
		if err != nil {
			http.Error(w, "Invalid funnel_id", http.StatusBadRequest)
			return
		}

		start, end := parseTimeseriesRange(r.URL.Query())

		params := api.AnalyticsParams{
			SiteID: site.ID,
			UserID: site.UserID,
			Start:  start,
			End:    end,
		}

		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), site.ID)
		if err != nil {
			slog.Error("Failed to resolve analytics store", "error", err, "site_id", site.ID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		stats, err := analyticsStore.GetFunnelStats(r.Context(), funnelID, params)
		if err != nil {
			slog.Error("Failed to get share funnel stats", "error", err)
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, "Funnel not found", http.StatusNotFound)
			} else {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(stats); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}

func (h *handler) loadShareSite(w http.ResponseWriter, r *http.Request) (*api.Site, bool) {
	if h.ctx.Store == nil {
		http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
		return nil, false
	}

	token := strings.TrimSpace(r.PathValue("token"))
	if token == "" {
		http.Error(w, "Invalid token", http.StatusBadRequest)
		return nil, false
	}

	site, err := h.ctx.Store.GetShareSiteByToken(r.Context(), token)
	if err != nil {
		slog.Error("Failed to load share site", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return nil, false
	}
	if site == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return nil, false
	}

	return site, true
}

func (h *handler) ensureSiteMatch(w http.ResponseWriter, r *http.Request, site *api.Site) bool {
	siteIDStr := r.PathValue("id")
	if siteIDStr == "" {
		return true
	}

	siteID, err := uuid.Parse(siteIDStr)
	if err != nil {
		http.Error(w, "Invalid site_id", http.StatusBadRequest)
		return false
	}
	if siteID != site.ID {
		http.Error(w, "Not found", http.StatusNotFound)
		return false
	}

	return true
}

func (h *handler) handleTimeseries(
	idParam string,
	invalidIDMessage string,
	logMessage string,
	fetch func(context.Context, *database.Store, api.AnalyticsParams, []uuid.UUID) (any, error),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		site, ok := h.loadShareSite(w, r)
		if !ok {
			return
		}
		if !h.ensureSiteMatch(w, r, site) {
			return
		}

		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), site.ID)
		if err != nil {
			slog.Error("Failed to resolve analytics store", "error", err, "site_id", site.ID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		start, end := parseTimeseriesRange(r.URL.Query())

		ids, err := parseUUIDQueryParam(r.URL.Query(), idParam)
		if err != nil {
			http.Error(w, invalidIDMessage, http.StatusBadRequest)
			return
		}

		params := api.AnalyticsParams{
			SiteID: site.ID,
			UserID: site.UserID,
			Start:  start,
			End:    end,
		}

		series, err := fetch(r.Context(), analyticsStore, params, ids)
		if err != nil {
			slog.Error(logMessage, "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(series); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}

func parseStatsRange(q url.Values) (time.Time, time.Time) {
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

func parseTimeseriesRange(q url.Values) (time.Time, time.Time) {
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

func parseUUIDQueryParam(q url.Values, key string) ([]uuid.UUID, error) {
	values := q[key]
	if len(values) == 0 {
		return nil, nil
	}

	ids := make([]uuid.UUID, 0, len(values))
	for _, rawID := range values {
		id, err := uuid.Parse(rawID)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func parsePathUUID(w http.ResponseWriter, r *http.Request, key string, invalidMessage string) (uuid.UUID, bool) {
	value := strings.TrimSpace(r.PathValue(key))
	id, err := uuid.Parse(value)
	if err != nil {
		http.Error(w, invalidMessage, http.StatusBadRequest)
		return uuid.Nil, false
	}
	return id, true
}

func parseFilters(q url.Values) ([]api.Filter, error) {
	var filters []api.Filter

	for _, raw := range q["filter"] {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		parts := strings.SplitN(raw, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid filter format")
		}
		filterType := strings.ToLower(strings.TrimSpace(parts[0]))
		filterValue := strings.TrimSpace(parts[1])
		if err := validateFilter(filterType, filterValue); err != nil {
			return nil, err
		}
		filters = append(filters, api.Filter{Type: filterType, Value: filterValue})
	}

	filterType := strings.ToLower(strings.TrimSpace(q.Get("filter_type")))
	filterValue := strings.TrimSpace(q.Get("filter_value"))
	if filterType != "" || filterValue != "" {
		if err := validateFilter(filterType, filterValue); err != nil {
			return nil, err
		}
		filters = append(filters, api.Filter{Type: filterType, Value: filterValue})
	}

	return filters, nil
}

func validateFilter(filterType, filterValue string) error {
	if filterType == "" || filterValue == "" {
		return fmt.Errorf("filter_type and filter_value are required together")
	}

	switch filterType {
	case "path", "referrer", "device", "country", "browser", "language", "utm_campaign", "utm_content", "utm_medium", "utm_source", "utm_term":
		return nil
	default:
		return fmt.Errorf("invalid filter_type")
	}
}
