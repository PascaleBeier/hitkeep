package sites

import (
	"net/http"
	"time"

	"hitkeep/server/shared"
)

func (h *handler) handleGetSiteTrackingStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		siteID, ok := shared.PathUUID(w, r, "id", "Invalid site_id")
		if !ok {
			return
		}

		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), siteID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to resolve tracking status analytics store", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		status, err := analyticsStore.GetSiteTrackingStatus(r.Context(), siteID, time.Now().UTC())
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to load site tracking status", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if status == nil {
			http.Error(w, "Site not found", http.StatusNotFound)
			return
		}

		shared.WriteJSON(r.Context(), w, http.StatusOK, status)
	}
}

func (h *handler) handleGetSiteSetupState() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		siteID, ok := shared.PathUUID(w, r, "id", "Invalid site_id")
		if !ok {
			return
		}

		// The flags read hits, events, web_vitals and ai_fetches, which all live
		// in the tenant database, so this needs the analytics store rather than
		// the control-plane store the tracking status above is happy with.
		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), siteID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to resolve analytics store", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		state, err := analyticsStore.GetSiteSetupState(r.Context(), siteID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to load site setup state", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		shared.WriteJSON(r.Context(), w, http.StatusOK, state)
	}
}
