package sites

import (
	"net/http"

	"hitkeep/server/shared"
)

func (h *handler) handleGetSiteRealtime() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Realtime == nil {
			http.Error(w, "Service not available", http.StatusServiceUnavailable)
			return
		}

		siteID, ok := shared.PathUUID(w, r, "id", "Invalid site_id")
		if !ok {
			return
		}

		shared.ServeRealtimeStream(w, r, h.ctx.Realtime, siteID)
	}
}
