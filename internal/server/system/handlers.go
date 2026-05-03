package system

import (
	"encoding/json"
	"log/slog"
	"net/http"

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
		if h.ctx.Store != nil {
			if err := h.ctx.Store.DB().Ping(); err != nil {
				slog.Error("Healthcheck failed: database unreachable", "error", err)
				http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
				return
			}
		}

		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("ok")); err != nil {
			slog.Error("Failed to write healthcheck response", "error", err)
		}
	}
}

func (h *handler) handleReadyz() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.ctx.Cluster.IsLeader() {
			http.Error(w, "Service not ready", http.StatusServiceUnavailable)
			return
		}

		if h.ctx.Store == nil {
			http.Error(w, "Service not ready", http.StatusServiceUnavailable)
			return
		}

		if err := h.ctx.Store.DB().Ping(); err != nil {
			slog.Error("Readiness check failed: database unreachable", "error", err)
			http.Error(w, "Database unavailable", http.StatusServiceUnavailable)
			return
		}

		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("ok")); err != nil {
			slog.Error("Failed to write readiness response", "error", err)
		}
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
			slog.Error("Failed to get user count", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}
