package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"hitkeep/internal/database"
	"hitkeep/internal/server/shared"
)

func (s *Server) databaseAvailabilityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s == nil || s.store == nil || databaseIndependentRoute(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if status := s.databaseAvailabilityStatus(); status.State != database.DatabaseStateHealthy {
			writeDatabaseUnavailable(r.Context(), w, status.State)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) databaseAvailabilityStatus() database.DatabaseStatus {
	if s == nil || s.store == nil {
		return database.DatabaseStatus{State: database.DatabaseStateFailed}
	}
	status := s.store.DatabaseStatus()
	if status.State != database.DatabaseStateHealthy || s.ctx == nil || s.ctx.TenantStores == nil {
		return status
	}
	if tenantStatus, unavailable := s.ctx.TenantStores.UnavailableDatabaseStatus(); unavailable {
		return tenantStatus
	}
	return status
}

func databaseIndependentRoute(path string) bool {
	switch path {
	case "/healthz", "/readyz":
		return true
	}
	if strings.HasPrefix(path, "/api/docs/") || path == "/api/docs/versions" {
		return true
	}
	return !strings.HasPrefix(path, "/api/") &&
		path != "/ingest" &&
		path != "/ingest/event" &&
		path != "/ingest/web-vitals"
}

func writeDatabaseUnavailable(ctx context.Context, w http.ResponseWriter, state string) {
	code, message := databaseUnavailableResponse(state)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", "5")
	w.WriteHeader(http.StatusServiceUnavailable)
	if err := json.NewEncoder(w).Encode(map[string]any{
		"status":              "error",
		"code":                code,
		"message":             message,
		"retry_after_seconds": 5,
	}); err != nil {
		shared.LoggerFromContext(ctx).Error("Failed to encode database recovery response", "error", err)
	}
}

func databaseUnavailableResponse(state string) (string, string) {
	switch state {
	case database.DatabaseStateRecovering:
		return "database_recovering", "Database recovery is in progress"
	case database.DatabaseStateNeedsAttention:
		return "database_needs_attention", "Database recovery requires operator attention"
	default:
		return "database_unavailable", "Database is unavailable"
	}
}
