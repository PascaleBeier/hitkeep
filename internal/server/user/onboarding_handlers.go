package user

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"hitkeep/internal/server/shared"
)

func (h *handler) handleGetUserOnboarding() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if h.ctx.TenantStores == nil {
			http.Error(w, "Tenant data plane unavailable", http.StatusServiceUnavailable)
			return
		}
		onboarding, err := h.ctx.TenantStores.GetUserOnboarding(r.Context(), userID)
		if err != nil {
			slog.Error("Failed to load user onboarding", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(onboarding); err != nil {
			slog.Error("Failed to encode user onboarding response", "error", err)
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
			slog.Error("Failed to dismiss user onboarding", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
