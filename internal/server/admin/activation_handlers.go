package admin

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"hitkeep/internal/database"
)

func (h *handler) handleGetActivation() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		limit := parseActivationInt(query.Get("limit"), 50)
		offset := parseActivationInt(query.Get("offset"), 0)
		lastSeenFrom, err := parseActivationTime(query.Get("last_seen_from"))
		if err != nil {
			http.Error(w, "Invalid last_seen_from", http.StatusBadRequest)
			return
		}
		lastSeenTo, err := parseActivationTime(query.Get("last_seen_to"))
		if err != nil {
			http.Error(w, "Invalid last_seen_to", http.StatusBadRequest)
			return
		}

		activationQuery := database.ActivationQuery{
			Status:       strings.TrimSpace(query.Get("status")),
			Team:         strings.TrimSpace(query.Get("team")),
			Domain:       strings.TrimSpace(query.Get("domain")),
			LastSeenFrom: lastSeenFrom,
			LastSeenTo:   lastSeenTo,
			Limit:        limit,
			Offset:       offset,
			Now:          time.Now().UTC(),
		}
		if h.ctx.TenantStores == nil {
			http.Error(w, "Tenant data plane unavailable", http.StatusServiceUnavailable)
			return
		}
		resp, err := h.ctx.TenantStores.ListSystemActivation(r.Context(), activationQuery)
		if err != nil {
			slog.Error("Failed to load activation view", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		for i := range resp.Rows {
			resp.Rows[i].CloudRegion = h.ctx.Config.CloudRegion
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

func parseActivationInt(raw string, fallback int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func parseActivationTime(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	parsed = parsed.UTC()
	return &parsed, nil
}
