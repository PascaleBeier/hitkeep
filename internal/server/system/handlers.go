package system

import (
	"context"
	"encoding/json"
	"net/http"

	"hitkeep/internal/database"
	"hitkeep/internal/server/shared"
)

type handler struct {
	ctx *shared.Context
}

func Register(mux *http.ServeMux, ctx *shared.Context) {
	h := &handler{ctx: ctx}
	mux.HandleFunc("GET /healthz", h.handleHealthz())
	mux.HandleFunc("GET /readyz", h.handleReadyz())
	mux.HandleFunc("GET /api/status", h.handleGetStatus())
	mux.HandleFunc("GET /api/docs/versions", h.handleGetAPIDocVersions())
	mux.HandleFunc("GET /api/docs/v1/openapi.json", h.handleGetAPIDocV1())
}

func (h *handler) handleHealthz() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("ok")); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to write healthcheck response", "error", err)
		}
	}
}

func (h *handler) handleReadyz() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Cluster != nil && !h.ctx.Cluster.IsLeader() {
			writeNotReady(r.Context(), w, "not_leader")
			return
		}

		if h.ctx.Store == nil {
			writeNotReady(r.Context(), w, "database_unavailable")
			return
		}

		status := h.ctx.Store.DatabaseStatus()
		if status.State == database.DatabaseStateHealthy && h.ctx.TenantStores != nil {
			if tenantStatus, unavailable := h.ctx.TenantStores.UnavailableDatabaseStatus(); unavailable {
				status = tenantStatus
			}
		}
		if status.State != database.DatabaseStateHealthy {
			writeNotReady(r.Context(), w, databaseReadinessReason(status.State))
			return
		}

		if err := h.ctx.Store.DB().Ping(); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Readiness check failed: database unreachable", "error", err)
			writeNotReady(r.Context(), w, "database_unavailable")
			return
		}

		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("ok")); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to write readiness response", "error", err)
		}
	}
}

func databaseReadinessReason(state string) string {
	switch state {
	case database.DatabaseStateRecovering:
		return "database_recovering"
	case database.DatabaseStateNeedsAttention:
		return "database_needs_attention"
	default:
		return "database_unavailable"
	}
}

func writeNotReady(ctx context.Context, w http.ResponseWriter, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", "5")
	w.WriteHeader(http.StatusServiceUnavailable)
	if err := json.NewEncoder(w).Encode(map[string]any{
		"status":              "not_ready",
		"reason":              reason,
		"retry_after_seconds": 5,
	}); err != nil {
		shared.LoggerFromContext(ctx).Error("Failed to encode readiness response", "error", err)
	}
}

func (h *handler) handleGetStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		response, err := h.ctx.SystemStatusResponse(r.Context())
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to get user count", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode response", "error", err)
		}
	}
}
