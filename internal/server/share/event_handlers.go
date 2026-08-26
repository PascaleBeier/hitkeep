package share

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
	"hitkeep/internal/server/filterparams"
	"hitkeep/internal/server/shared"
	json "hitkeep/jsonapi"
)

func (h *handler) handleGetShareEventNames() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		site, ok := h.loadShareSite(w, r)
		if !ok {
			return
		}
		if !h.ensureSiteMatch(w, r, site) {
			return
		}

		start, end := parseTimeseriesRange(r.URL.Query())

		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), site.ID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to resolve analytics store", "error", err, "site_id", site.ID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		names, err := analyticsStore.GetEventNames(r.Context(), api.EventNamesParams{
			SiteID: site.ID,
			Start:  start,
			End:    end,
		})
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to get share event names", "error", err, "site_id", site.ID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, names); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode response", "error", err)
		}
	}
}

func (h *handler) handleGetShareEventPropertyKeys() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		site, ok := h.loadShareSite(w, r)
		if !ok {
			return
		}
		if !h.ensureSiteMatch(w, r, site) {
			return
		}

		q := r.URL.Query()
		eventName := q.Get("event_name")
		if eventName == "" {
			http.Error(w, "event_name is required", http.StatusBadRequest)
			return
		}

		start, end := parseTimeseriesRange(q)

		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), site.ID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to resolve analytics store", "error", err, "site_id", site.ID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		keys, err := analyticsStore.GetEventPropertyKeys(r.Context(), api.EventNamesParams{
			SiteID: site.ID,
			Start:  start,
			End:    end,
		}, eventName)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to get share event property keys", "error", err, "site_id", site.ID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, keys); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode response", "error", err)
		}
	}
}

func (h *handler) handleGetShareEventPropertyBreakdown() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		site, ok := h.loadShareSite(w, r)
		if !ok {
			return
		}
		if !h.ensureSiteMatch(w, r, site) {
			return
		}

		q := r.URL.Query()
		eventName := q.Get("event_name")
		propertyKey := q.Get("property_key")
		if eventName == "" || propertyKey == "" {
			http.Error(w, "event_name and property_key are required", http.StatusBadRequest)
			return
		}

		start, end := parseTimeseriesRange(q)

		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), site.ID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to resolve analytics store", "error", err, "site_id", site.ID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		breakdown, err := analyticsStore.GetEventPropertyBreakdown(r.Context(), api.EventBreakdownParams{
			SiteID:      site.ID,
			Start:       start,
			End:         end,
			EventName:   eventName,
			PropertyKey: propertyKey,
		})
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to get share event property breakdown", "error", err, "site_id", site.ID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, breakdown); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode response", "error", err)
		}
	}
}

type shareEventQueryParams struct {
	SiteID         uuid.UUID
	Start          time.Time
	End            time.Time
	EventName      string
	PropertyKey    string
	PropertyValue  string
	Filters        []api.Filter
	DimensionKey   string
	DimensionValue string
}

func (h *handler) parseShareEventQueryParams(w http.ResponseWriter, r *http.Request) (*api.Site, shareEventQueryParams, bool) {
	site, ok := h.loadShareSite(w, r)
	if !ok {
		return nil, shareEventQueryParams{}, false
	}
	if !h.ensureSiteMatch(w, r, site) {
		return nil, shareEventQueryParams{}, false
	}

	q := r.URL.Query()
	eventName := q.Get("event_name")
	if eventName == "" {
		http.Error(w, "event_name is required", http.StatusBadRequest)
		return nil, shareEventQueryParams{}, false
	}
	filters, err := filterparams.ParseHitFilters(q, filterparams.LegacyPair{
		TypeParam:          "dimension_key",
		ValueParam:         "dimension_value",
		MissingMessage:     "filter type and value are required together",
		InvalidTypeMessage: "invalid filter type",
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil, shareEventQueryParams{}, false
	}

	start, end := parseTimeseriesRange(q)

	return site, shareEventQueryParams{
		SiteID:        site.ID,
		Start:         start,
		End:           end,
		EventName:     eventName,
		PropertyKey:   q.Get("property_key"),
		PropertyValue: q.Get("property_value"),
		Filters:       filters,
	}, true
}

func (h *handler) handleGetShareEventTimeseries() http.HandlerFunc {
	return h.shareEventQueryHandler("event timeseries",
		func(ctx context.Context, store *database.Store, p shareEventQueryParams) (any, error) {
			return store.GetEventTimeseries(ctx, api.EventTimeseriesParams{
				SiteID:         p.SiteID,
				Start:          p.Start,
				End:            p.End,
				EventName:      p.EventName,
				PropertyKey:    p.PropertyKey,
				PropertyValue:  p.PropertyValue,
				Filters:        p.Filters,
				DimensionKey:   p.DimensionKey,
				DimensionValue: p.DimensionValue,
			})
		})
}

func (h *handler) handleGetShareEventAudience() http.HandlerFunc {
	return h.shareEventQueryHandler("event audience",
		func(ctx context.Context, store *database.Store, p shareEventQueryParams) (any, error) {
			return store.GetEventAudience(ctx, api.EventAudienceParams{
				SiteID:         p.SiteID,
				Start:          p.Start,
				End:            p.End,
				EventName:      p.EventName,
				PropertyKey:    p.PropertyKey,
				PropertyValue:  p.PropertyValue,
				Filters:        p.Filters,
				DimensionKey:   p.DimensionKey,
				DimensionValue: p.DimensionValue,
			})
		})
}

func (h *handler) shareEventQueryHandler(
	label string,
	query func(context.Context, *database.Store, shareEventQueryParams) (any, error),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		site, p, ok := h.parseShareEventQueryParams(w, r)
		if !ok {
			return
		}

		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), site.ID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to resolve analytics store", "error", err, "site_id", site.ID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		result, err := query(r.Context(), analyticsStore, p)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to get share "+label, "error", err, "site_id", site.ID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, result); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode response", "error", err)
		}
	}
}

func (h *handler) parseShareEcommerceParams(w http.ResponseWriter, r *http.Request, site *api.Site, defaultLimit int) (api.EcommerceParams, bool) {
	q := r.URL.Query()
	start, end := parseTimeseriesRange(q)

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
		SiteID:   site.ID,
		Start:    start,
		End:      end,
		Filters:  filters,
		ItemID:   strings.TrimSpace(q.Get("item_id")),
		ItemName: strings.TrimSpace(q.Get("item_name")),
		Limit:    limit,
	}, true
}

func (h *handler) handleGetShareEcommerceSummary() http.HandlerFunc {
	return h.handleGetShareEcommerce(func(ctx context.Context, store *database.Store, params api.EcommerceParams) (any, error) {
		return store.GetEcommerceSummary(ctx, params)
	}, "summary")
}

func (h *handler) handleGetShareEcommerceTimeseries() http.HandlerFunc {
	return h.handleGetShareEcommerce(func(ctx context.Context, store *database.Store, params api.EcommerceParams) (any, error) {
		return store.GetEcommerceTimeSeries(ctx, params)
	}, "timeseries")
}

func (h *handler) handleGetShareEcommerceProducts() http.HandlerFunc {
	return h.handleGetShareEcommerce(func(ctx context.Context, store *database.Store, params api.EcommerceParams) (any, error) {
		return store.GetEcommerceTopProducts(ctx, params)
	}, "products")
}

func (h *handler) handleGetShareEcommerceSources() http.HandlerFunc {
	return h.handleGetShareEcommerce(func(ctx context.Context, store *database.Store, params api.EcommerceParams) (any, error) {
		return store.GetEcommerceSources(ctx, params)
	}, "sources")
}

func (h *handler) handleGetShareEcommerce(
	load func(context.Context, *database.Store, api.EcommerceParams) (any, error),
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

		params, ok := h.parseShareEcommerceParams(w, r, site, 10)
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
			shared.LoggerFromContext(r.Context()).Error("Failed to get share ecommerce "+label, "error", err, "site_id", params.SiteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, payload); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode response", "error", err)
		}
	}
}
