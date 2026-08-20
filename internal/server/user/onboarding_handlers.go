package user

import (
	"net/http"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	json "hitkeep/internal/jsonapi"
	"hitkeep/internal/server/shared"
)

func (h *handler) handleGetUserOnboarding() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		var onboarding *api.UserOnboarding
		var err error
		if h.ctx.TenantStores != nil {
			onboarding, err = h.ctx.TenantStores.GetUserOnboarding(r.Context(), userID)
		} else {
			onboarding, err = h.ctx.Store.GetUserOnboarding(r.Context(), userID)
		}
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to load user onboarding", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, onboarding); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode user onboarding response", "error", err)
		}
	}
}

func (h *handler) handleDismissUserOnboarding() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if err := h.ctx.Store.DismissUserOnboarding(r.Context(), userID); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to dismiss user onboarding", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
