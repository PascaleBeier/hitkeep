package share

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	authcore "hitkeep/internal/auth"
	"hitkeep/internal/exportfmt"
	"hitkeep/internal/server/shared"
)

type handler struct {
	ctx *shared.Context
}

func Register(mux *http.ServeMux, ctx *shared.Context) {
	h := &handler{ctx: ctx}

	mux.HandleFunc("GET /api/sites/{id}/share", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteManageTeam,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleListShareLinks()))

	mux.HandleFunc("POST /api/sites/{id}/share", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteManageTeam,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleCreateShareLink()))

	mux.HandleFunc("DELETE /api/sites/{id}/share/{shareID}", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteManageTeam,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleDeleteShareLink()))

	mux.HandleFunc("GET /api/share/{token}/site", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetShareSite()))

	mux.HandleFunc("GET /api/share/{token}/sites/{id}/stats", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetShareSiteStats()))

	mux.HandleFunc("GET /api/share/{token}/sites/{id}/hits", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetShareHits()))

	mux.HandleFunc("GET /api/share/{token}/sites/{id}/hits/export", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.ApiLimiter,
	}, h.handleExportShareHits()))

	mux.HandleFunc("GET /api/share/{token}/sites/{id}/goals", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetShareGoals()))

	mux.HandleFunc("GET /api/share/{token}/sites/{id}/goals/timeseries", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetShareGoalTimeseries()))

	mux.HandleFunc("GET /api/share/{token}/sites/{id}/funnels", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetShareFunnels()))

	mux.HandleFunc("GET /api/share/{token}/sites/{id}/funnels/timeseries", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetShareFunnelTimeseries()))

	mux.HandleFunc("GET /api/share/{token}/sites/{id}/funnels/{funnelID}/stats", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetShareFunnelStats()))

	mux.HandleFunc("GET /api/share/{token}/sites/{id}/events/names", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetShareEventNames()))

	mux.HandleFunc("GET /api/share/{token}/sites/{id}/events/properties", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetShareEventPropertyKeys()))

	mux.HandleFunc("GET /api/share/{token}/sites/{id}/events/breakdown", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetShareEventPropertyBreakdown()))

	mux.HandleFunc("GET /api/share/{token}/sites/{id}/events/timeseries", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetShareEventTimeseries()))

	mux.HandleFunc("GET /api/share/{token}/sites/{id}/events/audience", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetShareEventAudience()))

	mux.HandleFunc("GET /api/share/{token}/sites/{id}/ecommerce", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetShareEcommerceSummary()))

	mux.HandleFunc("GET /api/share/{token}/sites/{id}/ecommerce/timeseries", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetShareEcommerceTimeseries()))

	mux.HandleFunc("GET /api/share/{token}/sites/{id}/ecommerce/products", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetShareEcommerceProducts()))

	mux.HandleFunc("GET /api/share/{token}/sites/{id}/ecommerce/sources", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetShareEcommerceSources()))
}

func (h *handler) handleListShareLinks() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		siteID, ok := parsePathUUID(w, r, "id", "Invalid site_id")
		if !ok {
			return
		}

		links, err := h.ctx.Store.ListShareLinks(r.Context(), siteID)
		if err != nil {
			//nolint:gosec // IDs are parsed as UUIDs before logging; structured logging is intentional.
			slog.Error("Failed to list share links", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(links); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}

func (h *handler) handleCreateShareLink() http.HandlerFunc {
	type response struct {
		ID        uuid.UUID `json:"id"`
		URL       string    `json:"url"`
		Token     string    `json:"token"`
		TokenHint string    `json:"token_hint"`
		CreatedAt time.Time `json:"created_at"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		siteID, ok := parsePathUUID(w, r, "id", "Invalid site_id")
		if !ok {
			return
		}

		link, token, err := h.ctx.Store.CreateShareLink(r.Context(), siteID, userID)
		if err != nil {
			//nolint:gosec // IDs are sourced from auth context/path UUID parsing; structured logging is intentional.
			slog.Error("Failed to create share link", "error", err, "site_id", siteID, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		publicURL := strings.TrimRight(h.ctx.Config.PublicURL, "/")
		resp := response{
			ID:        link.ID,
			URL:       publicURL + "/share/" + token,
			Token:     token,
			TokenHint: link.TokenHint,
			CreatedAt: link.CreatedAt,
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}

func (h *handler) handleDeleteShareLink() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		siteID, ok := parsePathUUID(w, r, "id", "Invalid site_id")
		if !ok {
			return
		}

		shareID, ok := parsePathUUID(w, r, "shareID", "Invalid share_id")
		if !ok {
			return
		}

		revoked, err := h.ctx.Store.RevokeShareLink(r.Context(), siteID, shareID)
		if err != nil {
			//nolint:gosec // IDs are parsed as UUIDs before logging; structured logging is intentional.
			slog.Error("Failed to delete share link", "error", err, "site_id", siteID, "share_id", shareID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if !revoked {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *handler) handleGetShareSite() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		site, ok := h.loadShareSite(w, r)
		if !ok {
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(site); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}

func (h *handler) handleGetShareSiteStats() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		site, ok := h.loadShareSite(w, r)
		if !ok {
			return
		}
		if !h.ensureSiteMatch(w, r, site) {
			return
		}

		q := r.URL.Query()
		start, end := parseStatsRange(q)

		filters, err := parseFilters(q)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		goalIDs, err := parseUUIDQueryParam(q, "goal_id")
		if err != nil {
			http.Error(w, "Invalid goal_id", http.StatusBadRequest)
			return
		}

		funnelIDs, err := parseUUIDQueryParam(q, "funnel_id")
		if err != nil {
			http.Error(w, "Invalid funnel_id", http.StatusBadRequest)
			return
		}

		params := api.AnalyticsParams{
			SiteID:    site.ID,
			UserID:    site.UserID,
			Start:     start,
			End:       end,
			Filters:   filters,
			GoalIDs:   goalIDs,
			FunnelIDs: funnelIDs,
		}

		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), site.ID)
		if err != nil {
			slog.Error("Failed to resolve analytics store", "error", err, "site_id", site.ID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		stats, err := analyticsStore.GetSiteStats(r.Context(), params)
		if err != nil {
			//nolint:gosec // site_id comes from a validated share-site association and is logged for diagnostics.
			slog.Error("Failed to get share stats", "error", err, "site_id", site.ID)
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, "Not found", http.StatusNotFound)
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

func (h *handler) handleGetShareHits() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		site, ok := h.loadShareSite(w, r)
		if !ok {
			return
		}
		if !h.ensureSiteMatch(w, r, site) {
			return
		}

		q := r.URL.Query()

		now := time.Now().UTC()
		start := now.Add(-24 * time.Hour)
		end := now

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

		limit := 10
		offset := 0
		if l := q.Get("limit"); l != "" {
			if val, err := strconv.Atoi(l); err == nil {
				limit = val
			}
		}
		if o := q.Get("offset"); o != "" {
			if val, err := strconv.Atoi(o); err == nil {
				offset = val
			}
		}
		if limit > 100 {
			limit = 100
		}
		if limit < 1 {
			limit = 10
		}

		params := api.HitQueryParams{
			SiteID:    site.ID,
			UserID:    site.UserID,
			Start:     start,
			End:       end,
			Query:     q.Get("q"),
			SortField: q.Get("sort"),
			SortOrder: q.Get("order"),
			Limit:     limit,
			Offset:    offset,
			Filters:   filters,
		}

		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), site.ID)
		if err != nil {
			slog.Error("Failed to resolve analytics store", "error", err, "site_id", site.ID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		result, err := analyticsStore.GetHits(r.Context(), params)
		if err != nil {
			//nolint:gosec // site_id comes from a validated share-site association and is logged for diagnostics.
			slog.Error("Failed to get share hits", "error", err, "site_id", site.ID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(result); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}

func (h *handler) handleExportShareHits() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		site, ok := h.loadShareSite(w, r)
		if !ok {
			return
		}
		if !h.ensureSiteMatch(w, r, site) {
			return
		}

		q := r.URL.Query()

		now := time.Now().UTC()
		start := now.Add(-24 * time.Hour)
		end := now

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

		format := exportfmt.Normalize(q.Get("format"), exportfmt.FormatCSV)

		params := api.HitQueryParams{
			SiteID:  site.ID,
			UserID:  site.UserID,
			Start:   start,
			End:     end,
			Query:   q.Get("q"),
			Filters: filters,
		}

		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), site.ID)
		if err != nil {
			slog.Error("Failed to resolve analytics store", "error", err, "site_id", site.ID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if format == exportfmt.FormatCSV {
			filename := fmt.Sprintf("hits_%s_%d.csv", site.ID, time.Now().Unix())
			w.Header().Set("Content-Type", exportfmt.ContentType(exportfmt.FormatCSV))
			w.Header().Set("Content-Disposition", "attachment; filename="+filename)

			if err := analyticsStore.ExportHitsCSV(r.Context(), params, w); err != nil {
				//nolint:gosec // site_id comes from a validated share-site association and is logged for diagnostics.
				slog.Error("Failed to export share hits", "error", err, "site_id", site.ID)
			}
			return
		}

		filename, err := analyticsStore.ExportHitsFile(r.Context(), params, format)
		if err != nil {
			//nolint:gosec // site_id comes from a validated share-site association and is logged for diagnostics.
			slog.Error("Failed to export share hits", "error", err, "site_id", site.ID)
			http.Error(w, "Failed to export hits", http.StatusInternalServerError)
			return
		}
		downloadName := fmt.Sprintf("hits_%s_%d.%s", site.ID, time.Now().Unix(), format)
		w.Header().Set("Content-Disposition", "attachment; filename="+downloadName)
		w.Header().Set("Content-Type", exportfmt.ContentType(format))
		http.ServeFile(w, r, filename)

		go func() {
			cleanupShareHitsExportFile(filename)
		}()
	}
}

func cleanupShareHitsExportFile(filename string) {
	if filename == "" {
		return
	}

	cleaned := filepath.Clean(filename)
	base := filepath.Base(cleaned)
	if !strings.HasPrefix(base, "hitkeep_hits_") {
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
