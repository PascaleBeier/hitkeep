package share

import (
	"net/http"

	"hitkeep/server/shared"
)

func (h *handler) handleGetShareRealtime() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Realtime == nil {
			http.Error(w, "Service not available", http.StatusServiceUnavailable)
			return
		}

		site, ok := h.loadShareSite(w, r)
		if !ok {
			return
		}
		if !h.ensureSiteMatch(w, r, site) {
			return
		}

		shared.ServeRealtimeStream(w, r, h.ctx.Realtime, site.ID)
	}
}
