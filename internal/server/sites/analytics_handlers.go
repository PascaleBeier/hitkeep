package sites

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
	"hitkeep/internal/server/shared"
)

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
			return
		}

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

		if compareFromStr := q.Get("compare_from"); compareFromStr != "" {
			if parsed, err := time.Parse(time.RFC3339, compareFromStr); err == nil {
				params.CompareStart = parsed
			}
		}
		if compareToStr := q.Get("compare_to"); compareToStr != "" {
			if parsed, err := time.Parse(time.RFC3339, compareToStr); err == nil {
				params.CompareEnd = parsed
			}
		}

		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), siteID)
		if err != nil {
			slog.Error("Failed to resolve analytics store", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		stats, err := analyticsStore.GetSiteStats(r.Context(), params)
		if err != nil {
			slog.Error("Failed to get site stats", "error", err, "site_id", siteID)
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, "Site not found", http.StatusNotFound)
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
			slog.Error("Failed to resolve analytics store", "error", err, "site_id", params.SiteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		payload, err := load(r.Context(), analyticsStore, params)
		if err != nil {
			slog.Error("Failed to get ecommerce "+label, "error", err, "site_id", params.SiteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
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
	case "path", "hostname", "referrer", "referrer_host", "device", "country", "browser", "language", "utm_campaign", "utm_content", "utm_medium", "utm_source", "utm_term":
		return nil
	default:
		return fmt.Errorf("invalid filter_type")
	}
}
