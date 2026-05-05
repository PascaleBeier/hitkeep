package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	authcore "hitkeep/internal/auth"
	"hitkeep/internal/database"
	"hitkeep/internal/exportfmt"
	"hitkeep/internal/server/filterparams"
	"hitkeep/internal/server/shared"
)

type handler struct {
	ctx *shared.Context
}

func Register(mux *http.ServeMux, ctx *shared.Context) {
	h := &handler{ctx: ctx}
	mux.HandleFunc("GET /api/sites/{id}/events/names", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetEventNames()))
	mux.HandleFunc("GET /api/sites/{id}/events/properties", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetEventPropertyKeys()))
	mux.HandleFunc("GET /api/sites/{id}/events/breakdown", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetEventPropertyBreakdown()))
	mux.HandleFunc("GET /api/sites/{id}/events/timeseries", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetEventTimeseries()))
	mux.HandleFunc("GET /api/sites/{id}/events/audience", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetEventAudience()))
	mux.HandleFunc("GET /api/sites/{id}/ai-chatbots/export", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleExportAIChatbots()))
}

func parseSiteAndRange(w http.ResponseWriter, r *http.Request) (uuid.UUID, time.Time, time.Time, bool) {
	siteIDStr := r.PathValue("id")
	siteID, err := uuid.Parse(siteIDStr)
	if err != nil {
		http.Error(w, "Invalid site_id", http.StatusBadRequest)
		return uuid.Nil, time.Time{}, time.Time{}, false
	}

	q := r.URL.Query()
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
	return siteID, start, end, true
}

func (h *handler) handleGetEventNames() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		siteID, start, end, ok := parseSiteAndRange(w, r)
		if !ok {
			return
		}

		params := api.EventNamesParams{
			SiteID: siteID,
			Start:  start,
			End:    end,
		}

		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), siteID)
		if err != nil {
			slog.Error("Failed to resolve analytics store", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		names, err := analyticsStore.GetEventNames(r.Context(), params)
		if err != nil {
			slog.Error("Failed to get event names", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(names); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}

func (h *handler) handleGetEventPropertyKeys() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		siteID, start, end, ok := parseSiteAndRange(w, r)
		if !ok {
			return
		}

		eventName := r.URL.Query().Get("event_name")
		if eventName == "" {
			http.Error(w, "event_name is required", http.StatusBadRequest)
			return
		}

		params := api.EventNamesParams{
			SiteID: siteID,
			Start:  start,
			End:    end,
		}

		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), siteID)
		if err != nil {
			slog.Error("Failed to resolve analytics store", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		keys, err := analyticsStore.GetEventPropertyKeys(r.Context(), params, eventName)
		if err != nil {
			slog.Error("Failed to get event property keys", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(keys); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}

func (h *handler) handleGetEventPropertyBreakdown() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		siteID, start, end, ok := parseSiteAndRange(w, r)
		if !ok {
			return
		}

		q := r.URL.Query()
		eventName := q.Get("event_name")
		propertyKey := q.Get("property_key")
		if eventName == "" || propertyKey == "" {
			http.Error(w, "event_name and property_key are required", http.StatusBadRequest)
			return
		}

		params := api.EventBreakdownParams{
			SiteID:      siteID,
			Start:       start,
			End:         end,
			EventName:   eventName,
			PropertyKey: propertyKey,
		}

		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), siteID)
		if err != nil {
			slog.Error("Failed to resolve analytics store", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		breakdown, err := analyticsStore.GetEventPropertyBreakdown(r.Context(), params)
		if err != nil {
			slog.Error("Failed to get event property breakdown", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(breakdown); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}

type eventQueryParams struct {
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

var chatbotExportScopeKeys = map[string]struct{}{
	"provider": {},
	"bot_id":   {},
	"surface":  {},
	"model":    {},
}

func parseChatbotExportParams(w http.ResponseWriter, r *http.Request) (api.ChatbotExportParams, bool) {
	siteID, start, end, ok := parseSiteAndRange(w, r)
	if !ok {
		return api.ChatbotExportParams{}, false
	}

	scopeKey := strings.TrimSpace(r.URL.Query().Get("scope_key"))
	scopeValue := strings.TrimSpace(r.URL.Query().Get("scope_value"))
	if scopeKey == "" && scopeValue != "" {
		http.Error(w, "scope_key is required when scope_value is provided", http.StatusBadRequest)
		return api.ChatbotExportParams{}, false
	}
	if scopeKey != "" {
		if _, allowed := chatbotExportScopeKeys[scopeKey]; !allowed {
			http.Error(w, "Invalid scope_key", http.StatusBadRequest)
			return api.ChatbotExportParams{}, false
		}
		if scopeValue == "" {
			http.Error(w, "scope_value is required when scope_key is provided", http.StatusBadRequest)
			return api.ChatbotExportParams{}, false
		}
	}

	return api.ChatbotExportParams{
		SiteID:     siteID,
		Start:      start,
		End:        end,
		ScopeKey:   scopeKey,
		ScopeValue: scopeValue,
	}, true
}

func (h *handler) parseEventQueryParams(w http.ResponseWriter, r *http.Request) (eventQueryParams, bool) {
	if h.ctx.Store == nil {
		http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
		return eventQueryParams{}, false
	}

	siteID, start, end, ok := parseSiteAndRange(w, r)
	if !ok {
		return eventQueryParams{}, false
	}

	q := r.URL.Query()
	eventName := q.Get("event_name")
	if eventName == "" {
		http.Error(w, "event_name is required", http.StatusBadRequest)
		return eventQueryParams{}, false
	}
	filters, err := filterparams.ParseHitFilters(q, filterparams.LegacyPair{
		TypeParam:          "dimension_key",
		ValueParam:         "dimension_value",
		MissingMessage:     "filter type and value are required together",
		InvalidTypeMessage: "invalid filter type",
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return eventQueryParams{}, false
	}

	return eventQueryParams{
		SiteID:        siteID,
		Start:         start,
		End:           end,
		EventName:     eventName,
		PropertyKey:   q.Get("property_key"),
		PropertyValue: q.Get("property_value"),
		Filters:       filters,
	}, true
}

func (h *handler) handleGetEventAudience() http.HandlerFunc {
	return h.eventQueryHandler("event audience", func(ctx context.Context, store *database.Store, p eventQueryParams) (any, error) {
		return store.GetEventAudience(ctx, api.EventAudienceParams(p))
	})
}

func (h *handler) handleExportAIChatbots() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		params, ok := parseChatbotExportParams(w, r)
		if !ok {
			return
		}

		format := exportfmt.Normalize(r.URL.Query().Get("format"), exportfmt.FormatCSV)

		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), params.SiteID)
		if err != nil {
			slog.Error("Failed to resolve analytics store", "error", err, "site_id", params.SiteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if format == exportfmt.FormatCSV {
			filename := fmt.Sprintf("ai-chatbots_%s_%d.csv", params.SiteID, time.Now().Unix())
			w.Header().Set("Content-Type", exportfmt.ContentType(exportfmt.FormatCSV))
			w.Header().Set("Content-Disposition", "attachment; filename="+filename)

			if err := analyticsStore.ExportChatbotEventsCSV(r.Context(), params, w); err != nil {
				slog.Error("Failed to export chatbot events", "error", err, "site_id", params.SiteID, "user_id", userID)
			}
			return
		}

		filename, err := analyticsStore.ExportChatbotEventsFile(r.Context(), params, format)
		if err != nil {
			slog.Error("Failed to export chatbot events", "error", err, "site_id", params.SiteID, "user_id", userID)
			http.Error(w, "Failed to export chatbot events", http.StatusInternalServerError)
			return
		}

		downloadName := fmt.Sprintf("ai-chatbots_%s_%d.%s", params.SiteID, time.Now().Unix(), format)
		shared.ServeTempExportFile(w, r, filename, downloadName, exportfmt.ContentType(format), "hitkeep_ai_chatbots_")

		go cleanupAIChatbotExportFile(filename)
	}
}

func (h *handler) handleGetEventTimeseries() http.HandlerFunc {
	return h.eventQueryHandler("event timeseries", func(ctx context.Context, store *database.Store, p eventQueryParams) (any, error) {
		return store.GetEventTimeseries(ctx, api.EventTimeseriesParams(p))
	})
}

func (h *handler) eventQueryHandler(label string, query func(context.Context, *database.Store, eventQueryParams) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := h.parseEventQueryParams(w, r)
		if !ok {
			return
		}

		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), p.SiteID)
		if err != nil {
			slog.Error("Failed to resolve analytics store", "error", err, "site_id", p.SiteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		result, err := query(r.Context(), analyticsStore, p)
		if err != nil {
			slog.Error("Failed to get "+label, "error", err, "site_id", p.SiteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(result); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}

func cleanupAIChatbotExportFile(filename string) {
	if filename == "" {
		return
	}

	cleaned := filepath.Clean(filename)
	base := filepath.Base(cleaned)
	if !strings.HasPrefix(base, "hitkeep_ai_chatbots_") {
		return
	}

	tempDir := filepath.Clean(os.TempDir())
	rel, err := filepath.Rel(tempDir, cleaned)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return
	}

	//nolint:gosec // cleaned path is constrained to an app-owned temp export under os.TempDir.
	_ = os.Remove(cleaned)
}
