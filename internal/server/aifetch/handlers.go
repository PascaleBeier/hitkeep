package aifetch

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/aianalytics"
	"hitkeep/internal/api"
	authcore "hitkeep/internal/auth"
	"hitkeep/internal/exportfmt"
	"hitkeep/internal/server/shared"
)

type handler struct {
	ctx *shared.Context
}

type ingestPayload struct {
	Path        string `json:"path"`
	Hostname    string `json:"hostname,omitempty"`
	StatusCode  int    `json:"status_code"`
	ContentType string `json:"content_type,omitempty"`
	ResponseMs  int    `json:"response_ms,omitempty"`
	BytesServed int64  `json:"bytes_served,omitempty"`
	UserAgent   string `json:"user_agent"`
}

func Register(mux *http.ServeMux, ctx *shared.Context) {
	h := &handler{ctx: ctx}
	mux.HandleFunc("POST /api/sites/{id}/ingest/ai-fetch", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteManageData,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleCreateAIFetch()))
	mux.HandleFunc("GET /api/sites/{id}/ai-fetch/overview", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetOverview()))
	mux.HandleFunc("GET /api/sites/{id}/ai-fetch/timeseries", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetTimeseries()))
	mux.HandleFunc("GET /api/sites/{id}/ai-fetch/correlation", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetCorrelation()))
	mux.HandleFunc("GET /api/sites/{id}/ai-fetch/export", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleExportAIFetch()))
}

func parseSiteAndRange(w http.ResponseWriter, r *http.Request) (api.AIFetchQueryParams, bool) {
	siteID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid site_id", http.StatusBadRequest)
		return api.AIFetchQueryParams{}, false
	}

	now := time.Now().UTC()
	params := api.AIFetchQueryParams{
		SiteID: siteID,
		Start:  now.AddDate(0, 0, -30),
		End:    now,
	}
	query := r.URL.Query()
	if from := query.Get("from"); from != "" {
		parsed, err := time.Parse(time.RFC3339, from)
		if err != nil {
			http.Error(w, "Invalid from date, expected RFC3339", http.StatusBadRequest)
			return api.AIFetchQueryParams{}, false
		}
		params.Start = parsed
	}
	if to := query.Get("to"); to != "" {
		parsed, err := time.Parse(time.RFC3339, to)
		if err != nil {
			http.Error(w, "Invalid to date, expected RFC3339", http.StatusBadRequest)
			return api.AIFetchQueryParams{}, false
		}
		params.End = parsed
	}
	params.AssistantName = strings.TrimSpace(query.Get("assistant_name"))
	params.AssistantFamily = strings.TrimSpace(query.Get("assistant_family"))
	params.ResourceType = strings.TrimSpace(query.Get("resource_type"))
	return params, true
}

func parseCorrelationParams(w http.ResponseWriter, r *http.Request) (api.AIFetchCorrelationParams, bool) {
	base, ok := parseSiteAndRange(w, r)
	if !ok {
		return api.AIFetchCorrelationParams{}, false
	}

	params := api.AIFetchCorrelationParams{
		SiteID:          base.SiteID,
		Start:           base.Start,
		End:             base.End,
		AssistantName:   base.AssistantName,
		AssistantFamily: base.AssistantFamily,
		ResourceType:    base.ResourceType,
		WindowDays:      30,
	}

	if raw := strings.TrimSpace(r.URL.Query().Get("window_days")); raw != "" {
		windowDays, err := strconv.Atoi(raw)
		if err != nil || windowDays < 1 || windowDays > 90 {
			http.Error(w, "Invalid window_days, expected integer between 1 and 90", http.StatusBadRequest)
			return api.AIFetchCorrelationParams{}, false
		}
		params.WindowDays = windowDays
	}

	return params, true
}

func (h *handler) handleCreateAIFetch() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		siteID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			h.recordRejection()
			http.Error(w, "Invalid site_id", http.StatusBadRequest)
			return
		}
		site, err := h.ctx.Store.GetSiteByID(r.Context(), siteID)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if site == nil {
			h.recordRejection()
			http.Error(w, "Site not found", http.StatusNotFound)
			return
		}

		userIP := shared.GetRealIP(r, h.ctx.Config.GetTrustedProxyNetworks())
		if h.ctx.IPFilter != nil && h.ctx.IPFilter.IsBlocked(site.ID, userIP) {
			h.recordRejection()
			w.WriteHeader(http.StatusAccepted)
			return
		}
		if h.ctx.SpamFilter != nil {
			decision := h.ctx.SpamFilter.Evaluate(site.Domain, userIP, nil)
			if decision.Blocked {
				slog.Info("Dropped spam ai fetch", "site_id", site.ID, "reason", decision.Reason)
				h.recordSpamDrop()
				w.WriteHeader(http.StatusAccepted)
				return
			}
		}

		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		var payload ingestPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			h.recordRejection()
			http.Error(w, "Bad request body", http.StatusBadRequest)
			return
		}
		if payload.StatusCode < 100 || payload.StatusCode > 599 {
			h.recordRejection()
			http.Error(w, "status_code must be between 100 and 599", http.StatusBadRequest)
			return
		}
		identity := aianalytics.ClassifyBot(payload.UserAgent)
		if identity == nil {
			h.recordRejection()
			http.Error(w, "user_agent must match a known AI bot", http.StatusBadRequest)
			return
		}

		path, hostname, ok := normalizeFetchTarget(payload.Path, payload.Hostname)
		if !ok {
			h.recordRejection()
			http.Error(w, "path is required", http.StatusBadRequest)
			return
		}

		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), siteID)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		var contentType *string
		if trimmed := strings.TrimSpace(payload.ContentType); trimmed != "" {
			contentType = &trimmed
		}
		var responseMs *int
		if payload.ResponseMs > 0 {
			responseMs = &payload.ResponseMs
		}
		var bytesServed *int64
		if payload.BytesServed > 0 {
			bytesServed = &payload.BytesServed
		}
		userAgent := strings.TrimSpace(payload.UserAgent)

		record := &api.AIFetch{
			SiteID:          siteID,
			Timestamp:       time.Now().UTC(),
			AssistantName:   identity.Name,
			AssistantFamily: identity.Family,
			Path:            path,
			Hostname:        hostname,
			StatusCode:      payload.StatusCode,
			ContentType:     contentType,
			ResourceType:    aianalytics.ClassifyResourceType(payload.ContentType),
			ResponseMs:      responseMs,
			BytesServed:     bytesServed,
			UserAgent:       &userAgent,
		}

		if err := analyticsStore.CreateAIFetch(r.Context(), record); err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusAccepted)
	}
}

func (h *handler) recordSpamDrop() {
	if h.ctx.SystemCounters != nil {
		h.ctx.SystemCounters.Spam.Add(1)
	}
}

func (h *handler) recordRejection() {
	if h.ctx.SystemCounters != nil {
		h.ctx.SystemCounters.Rejections.Add(1)
	}
}

func (h *handler) handleGetOverview() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}
		params, ok := parseSiteAndRange(w, r)
		if !ok {
			return
		}

		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), params.SiteID)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		result, err := analyticsStore.GetAIFetchOverview(r.Context(), params)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	}
}

func (h *handler) handleGetTimeseries() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}
		params, ok := parseSiteAndRange(w, r)
		if !ok {
			return
		}

		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), params.SiteID)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		result, err := analyticsStore.GetAIFetchTimeseries(r.Context(), params)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	}
}

func (h *handler) handleGetCorrelation() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}
		params, ok := parseCorrelationParams(w, r)
		if !ok {
			return
		}

		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), params.SiteID)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		result, err := analyticsStore.GetAIFetchCorrelation(r.Context(), params)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	}
}

func (h *handler) handleExportAIFetch() http.HandlerFunc {
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

		params, ok := parseSiteAndRange(w, r)
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
			filename := fmt.Sprintf("ai-fetches_%s_%d.csv", params.SiteID, time.Now().Unix())
			w.Header().Set("Content-Type", exportfmt.ContentType(exportfmt.FormatCSV))
			w.Header().Set("Content-Disposition", "attachment; filename="+filename)

			if err := analyticsStore.ExportAIFetchCSV(r.Context(), params, w); err != nil {
				slog.Error("Failed to export ai fetches", "error", err, "site_id", params.SiteID, "user_id", userID)
			}
			return
		}

		tmpFile, err := analyticsStore.ExportAIFetchFile(r.Context(), params, format)
		if err != nil {
			slog.Error("Failed to export ai fetches", "error", err, "site_id", params.SiteID, "user_id", userID)
			http.Error(w, "Failed to export ai fetches", http.StatusInternalServerError)
			return
		}
		downloadName := fmt.Sprintf("ai-fetches_%s_%d.%s", params.SiteID, time.Now().Unix(), format)
		shared.ServeTempExportFile(w, r, tmpFile, downloadName, exportfmt.ContentType(format), "hitkeep_aifetch_")

		go func() {
			cleanupAIFetchExportFile(tmpFile)
		}()
	}
}

func cleanupAIFetchExportFile(filename string) {
	if filename == "" {
		return
	}

	cleaned := filepath.Clean(filename)
	base := filepath.Base(cleaned)
	if !strings.HasPrefix(base, "hitkeep_aifetch_") {
		return
	}

	tempDir := filepath.Clean(os.TempDir())
	rel, err := filepath.Rel(tempDir, cleaned)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return
	}

	//nolint:gosec // cleaned path is constrained to an app-owned temp file under os.TempDir.
	_ = os.Remove(cleaned)
}

func normalizeFetchTarget(rawPath, rawHostname string) (string, *string, bool) {
	trimmedPath := strings.TrimSpace(rawPath)
	trimmedHost := strings.TrimSpace(rawHostname)

	var parsedURL *url.URL
	if trimmedPath != "" {
		if parsed, err := url.Parse(trimmedPath); err == nil && parsed.Host != "" {
			parsedURL = parsed
			trimmedPath = parsed.EscapedPath()
			if trimmedPath == "" {
				trimmedPath = "/"
			}
			if parsed.RawQuery != "" {
				trimmedPath += "?" + parsed.RawQuery
			}
			if trimmedHost == "" {
				trimmedHost = strings.ToLower(parsed.Hostname())
			}
		}
	}

	if trimmedPath == "" {
		return "", nil, false
	}
	if parsedURL == nil && !strings.HasPrefix(trimmedPath, "/") {
		trimmedPath = "/" + trimmedPath
	}

	if trimmedHost == "" {
		return trimmedPath, nil, true
	}

	host := strings.ToLower(trimmedHost)
	return trimmedPath, &host, true
}
