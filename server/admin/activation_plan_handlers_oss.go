//go:build !billing

package admin

import (
	"net/http"

	"hitkeep/server/shared"
)

func (h *handler) handleSetActivationTeamPlan() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		shared.WriteJSON(r.Context(), w, http.StatusNotImplemented, map[string]string{
			"status": "error", "message": "Cloud billing is not available on this build",
		})
	}
}
