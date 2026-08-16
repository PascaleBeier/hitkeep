package permissions

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"hitkeep/internal/server/access"
	"hitkeep/internal/server/shared"
)

type handler struct {
	ctx *shared.Context
}

func Register(mux *http.ServeMux, ctx *shared.Context) {
	h := &handler{ctx: ctx}
	mux.HandleFunc("GET /api/user/permissions", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetUserPermissions()))
}

func (h *handler) handleGetUserPermissions() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		builder, ok := h.accessBuilder(w)
		if !ok {
			return
		}

		resp, err := builder.ForUser(r.Context(), userID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to build permission context", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		writeJSON(r.Context(), w, resp)
	}
}

func (h *handler) accessBuilder(w http.ResponseWriter) (access.Builder, bool) {
	if h.ctx.Store == nil {
		http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
		return access.Builder{}, false
	}
	return access.Builder{Store: h.ctx.Store, Limits: h.ctx.Limits()}, true
}

func writeJSON(ctx context.Context, w http.ResponseWriter, resp any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		shared.LoggerFromContext(ctx).Error("Failed to encode response", "error", err)
	}
}
