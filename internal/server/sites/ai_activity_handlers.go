package sites

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/server/filterparams"
	"hitkeep/internal/server/shared"
)

// handleGetSiteAIActivity serves the unified AI activity report: one merged view
// over tracked AI hits and server-log AI fetch records, where every count is
// tracked hits plus fetch records. Repeatable filter=type:value params narrow
// both sides where the dimension exists on both; see Store.GetAIActivity for the
// per-dimension mapping.
func (h *handler) handleGetSiteAIActivity() http.HandlerFunc {
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

		siteID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			http.Error(w, "Invalid site_id", http.StatusBadRequest)
			return
		}

		window, ok := parseAnalyticsWindow(w, r)
		if !ok {
			return
		}

		q := r.URL.Query()
		compareStart, compareEnd := filterparams.ParseComparisonRange(q.Get("compare_from"), q.Get("compare_to"))
		goalIDs, err := parseHitUUIDQueryParam(q, "goal_id")
		if err != nil {
			http.Error(w, "Invalid goal_id", http.StatusBadRequest)
			return
		}
		funnelIDs, err := parseHitUUIDQueryParam(q, "funnel_id")
		if err != nil {
			http.Error(w, "Invalid funnel_id", http.StatusBadRequest)
			return
		}

		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), siteID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to resolve analytics store", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		report, err := analyticsStore.GetAIActivity(r.Context(), api.AnalyticsParams{
			SiteID:       siteID,
			UserID:       userID,
			Start:        window.Start,
			End:          window.End,
			Filters:      window.Filters,
			GoalIDs:      goalIDs,
			FunnelIDs:    funnelIDs,
			CompareStart: compareStart,
			CompareEnd:   compareEnd,
		})
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to get AI activity report", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(report); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode response", "error", err)
		}
	}
}
