package sites

import (
	"net/http"

	"github.com/google/uuid"

	"hitkeep/server/shared"
)

func (h *handler) handleGetSiteRealtime() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Realtime == nil {
			http.Error(w, "Service not available", http.StatusServiceUnavailable)
			return
		}

		siteID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			http.Error(w, "Invalid site_id", http.StatusBadRequest)
			return
		}

		shared.ServeRealtimeStream(w, r, h.ctx.Realtime, siteID)
	}
}
