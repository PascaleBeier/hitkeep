package share

import (
	"encoding/json"
	"net/http"

	"hitkeep/internal/api"
	"hitkeep/internal/server/filterparams"
	"hitkeep/internal/server/shared"
)

// handleGetShareAIActivity serves the unified AI activity report for a shared
// dashboard. The dashboard's share interceptor rewrites every GET /api/sites/*
// call onto the share prefix, so this route has to exist or the AI activity page
// 404s in share mode.
//
// The shared report includes the fetch side: the query runs under the token
// grant for the shared site, and crawler volume is the same aggregate the owner
// sees rather than a separate privilege.
func (h *handler) handleGetShareAIActivity() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		site, ok := h.loadShareSite(w, r)
		if !ok {
			return
		}
		if !h.ensureSiteMatch(w, r, site) {
			return
		}

		q := r.URL.Query()
		start, end, ok := filterparams.ParseAnalyticsRange(w, q.Get("from"), q.Get("to"))
		if !ok {
			return
		}

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

		compareStart, compareEnd := filterparams.ParseComparisonRange(q.Get("compare_from"), q.Get("compare_to"))

		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), site.ID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to resolve analytics store", "error", err, "site_id", site.ID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		report, err := analyticsStore.GetAIActivity(r.Context(), api.AnalyticsParams{
			SiteID:       site.ID,
			UserID:       site.UserID,
			Start:        start,
			End:          end,
			Filters:      filters,
			GoalIDs:      goalIDs,
			FunnelIDs:    funnelIDs,
			CompareStart: compareStart,
			CompareEnd:   compareEnd,
		})
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to get share AI activity report", "error", err, "site_id", site.ID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(report); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode response", "error", err)
		}
	}
}
