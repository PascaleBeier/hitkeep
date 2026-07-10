package goals

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	authcore "hitkeep/internal/auth"
	"hitkeep/internal/database"
	"hitkeep/internal/realtime"
	"hitkeep/internal/server/shared"
	"hitkeep/internal/webhooks"
)

type handler struct {
	ctx *shared.Context
}

func Register(mux *http.ServeMux, ctx *shared.Context) {
	h := &handler{ctx: ctx}
	mux.HandleFunc("GET /api/sites/{id}/goals", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetGoals()))
	mux.HandleFunc("GET /api/sites/{id}/goals/timeseries", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetGoalTimeseries()))
	mux.HandleFunc("POST /api/sites/{id}/goals", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteManageGoals,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleCreateGoal()))
	mux.HandleFunc("PUT /api/sites/{id}/goals/{goalID}", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteManageGoals,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleUpdateGoal()))
	mux.HandleFunc("DELETE /api/sites/{id}/goals/{goalID}", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteManageGoals,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleDeleteGoal()))

	mux.HandleFunc("GET /api/sites/{id}/funnels", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetFunnels()))
	mux.HandleFunc("GET /api/sites/{id}/funnels/timeseries", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetFunnelTimeseries()))
	mux.HandleFunc("POST /api/sites/{id}/funnels", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteManageGoals,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleCreateFunnel()))
	mux.HandleFunc("DELETE /api/sites/{id}/funnels/{funnelID}", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteManageGoals,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleDeleteFunnel()))
	mux.HandleFunc("GET /api/sites/{id}/funnels/{funnelID}/stats", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetFunnelStats()))
}

// parseSiteID extracts and validates the site UUID from the URL path.
// Authorization is already handled by the RequirePermission middleware.
func parseSiteID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	siteID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid site_id", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return siteID, true
}

func (h *handler) handleListDefinitions(
	fetch func(context.Context, *database.Store, uuid.UUID) (any, error),
	logMessage string,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, ok := parseSiteID(w, r)
		if !ok {
			return
		}

		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), siteID)
		if err != nil {
			slog.Error("Failed to resolve analytics store", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		definitions, err := fetch(r.Context(), analyticsStore, siteID)
		if err != nil {
			slog.Error(logMessage, "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(definitions); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}

func (h *handler) handleDeleteDefinition(
	pathParam string,
	invalidIDMessage string,
	deleteDefinition func(context.Context, *database.Store, uuid.UUID, uuid.UUID) error,
	deleteLogMessage string,
	deleteLegacyLogMessage string,
	realtimeKind string,
	onDeleted func(context.Context, uuid.UUID, uuid.UUID),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, ok := parseSiteID(w, r)
		if !ok {
			return
		}

		definitionID, err := uuid.Parse(r.PathValue(pathParam))
		if err != nil {
			http.Error(w, invalidIDMessage, http.StatusBadRequest)
			return
		}

		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), siteID)
		if err != nil {
			slog.Error("Failed to resolve analytics store", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if err := deleteDefinition(r.Context(), analyticsStore, definitionID, siteID); err != nil {
			slog.Error(deleteLogMessage, "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if analyticsStore != h.ctx.Store {
			if err := deleteDefinition(r.Context(), h.ctx.Store, definitionID, siteID); err != nil && !database.IsNotFoundError(err) {
				slog.Error(deleteLegacyLogMessage, "error", err, "site_id", siteID, "definition_id", definitionID)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
		}

		h.publishDefinitionChange(siteID, realtimeKind)
		if onDeleted != nil {
			onDeleted(r.Context(), siteID, definitionID)
		}
		w.WriteHeader(http.StatusOK)
	}
}

// Goals

func (h *handler) handleGetGoals() http.HandlerFunc {
	return h.handleListDefinitions(
		func(ctx context.Context, store *database.Store, siteID uuid.UUID) (any, error) {
			return store.GetGoals(ctx, siteID)
		},
		"Failed to get goals",
	)
}

func (h *handler) handleGetGoalTimeseries() http.HandlerFunc {
	return h.handleTimeseries("goal_id", "Invalid goal_id", "Failed to get goal timeseries",
		func(ctx context.Context, store *database.Store, params api.AnalyticsParams, ids []uuid.UUID) (any, error) {
			return store.GetGoalTimeseries(ctx, params, ids)
		})
}

func (h *handler) handleCreateGoal() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, ok := parseSiteID(w, r)
		if !ok {
			return
		}

		var req api.Goal
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if req.Name == "" || req.Value == "" || (req.Type != "event" && req.Type != "path") {
			http.Error(w, "Invalid goal data", http.StatusBadRequest)
			return
		}

		req.SiteID = siteID
		req.CreatedAt = time.Now()

		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), siteID)
		if err != nil {
			slog.Error("Failed to resolve analytics store", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if err := analyticsStore.CreateGoal(r.Context(), &req); err != nil {
			slog.Error("Failed to create goal", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if analyticsStore != h.ctx.Store {
			if err := h.ctx.Store.UpsertGoal(r.Context(), &req); err != nil {
				slog.Error("Failed to write legacy shared goal", "error", err, "site_id", siteID, "goal_id", req.ID)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
		}
		h.publishDefinitionChange(siteID, realtime.KindGoals)
		h.ctx.EmitWebhookEvent(r.Context(), webhooks.Event{
			Type:   webhooks.EventGoalCreated,
			SiteID: &siteID,
			Data: map[string]any{
				"site_id": siteID.String(),
				"goal_id": req.ID.String(),
				"name":    req.Name,
				"type":    req.Type,
			},
		})

		w.WriteHeader(http.StatusCreated)
	}
}

func (h *handler) handleDeleteGoal() http.HandlerFunc {
	return h.handleDeleteDefinition(
		"goalID",
		"Invalid goal_id",
		func(ctx context.Context, store *database.Store, definitionID uuid.UUID, siteID uuid.UUID) error {
			return store.DeleteGoal(ctx, definitionID, siteID)
		},
		"Failed to delete goal",
		"Failed to delete legacy shared goal",
		realtime.KindGoals,
		func(ctx context.Context, siteID, goalID uuid.UUID) {
			h.ctx.EmitWebhookEvent(ctx, webhooks.Event{
				Type:   webhooks.EventGoalDeleted,
				SiteID: &siteID,
				Data:   map[string]any{"site_id": siteID.String(), "goal_id": goalID.String()},
			})
		},
	)
}

func (h *handler) handleUpdateGoal() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, ok := parseSiteID(w, r)
		if !ok {
			return
		}
		goalID, err := uuid.Parse(r.PathValue("goalID"))
		if err != nil {
			http.Error(w, "Invalid goal_id", http.StatusBadRequest)
			return
		}
		var input api.Goal
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		if input.Name == "" || input.Value == "" || (input.Type != "event" && input.Type != "path") {
			http.Error(w, "Invalid goal data", http.StatusBadRequest)
			return
		}
		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), siteID)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		goals, err := analyticsStore.GetGoals(r.Context(), siteID)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		var existing *api.Goal
		for i := range goals {
			if goals[i].ID == goalID {
				existing = &goals[i]
				break
			}
		}
		if existing == nil {
			http.Error(w, "Goal not found", http.StatusNotFound)
			return
		}
		input.ID = goalID
		input.SiteID = siteID
		input.CreatedAt = existing.CreatedAt
		if err := analyticsStore.UpsertGoal(r.Context(), &input); err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if analyticsStore != h.ctx.Store {
			if err := h.ctx.Store.UpsertGoal(r.Context(), &input); err != nil {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
		}
		h.publishDefinitionChange(siteID, realtime.KindGoals)
		h.ctx.EmitWebhookEvent(r.Context(), webhooks.Event{
			Type:   webhooks.EventGoalUpdated,
			SiteID: &siteID,
			Data: map[string]any{
				"site_id": siteID.String(), "goal_id": goalID.String(), "name": input.Name, "type": input.Type,
			},
		})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(input)
	}
}

// Funnels

func (h *handler) handleGetFunnels() http.HandlerFunc {
	return h.handleListDefinitions(
		func(ctx context.Context, store *database.Store, siteID uuid.UUID) (any, error) {
			return store.GetFunnels(ctx, siteID)
		},
		"Failed to get funnels",
	)
}

func (h *handler) handleGetFunnelTimeseries() http.HandlerFunc {
	return h.handleTimeseries("funnel_id", "Invalid funnel_id", "Failed to get funnel timeseries",
		func(ctx context.Context, store *database.Store, params api.AnalyticsParams, ids []uuid.UUID) (any, error) {
			return store.GetFunnelTimeseries(ctx, params, ids)
		})
}

func (h *handler) handleTimeseries(
	idParam string,
	invalidIDMessage string,
	logMessage string,
	fetch func(context.Context, *database.Store, api.AnalyticsParams, []uuid.UUID) (any, error),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, ok := parseSiteID(w, r)
		if !ok {
			return
		}

		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), siteID)
		if err != nil {
			slog.Error("Failed to resolve analytics store", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		start, end := parseTimeseriesRange(r.URL.Query())

		ids, err := parseUUIDQueryParam(r.URL.Query(), idParam)
		if err != nil {
			http.Error(w, invalidIDMessage, http.StatusBadRequest)
			return
		}

		params := api.AnalyticsParams{
			SiteID: siteID,
			Start:  start,
			End:    end,
		}

		series, err := fetch(r.Context(), analyticsStore, params, ids)
		if err != nil {
			slog.Error(logMessage, "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(series); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}

func parseTimeseriesRange(q url.Values) (time.Time, time.Time) {
	now := time.Now().UTC()
	end := now.AddDate(0, 0, 1)
	start := end.AddDate(0, 0, -30)

	if fromStr := q.Get("from"); fromStr != "" {
		if parsed, err := time.Parse(time.RFC3339, fromStr); err == nil {
			start = parsed
		}
	}
	if toStr := q.Get("to"); toStr != "" {
		if parsed, err := time.Parse(time.RFC3339, toStr); err == nil {
			end = parsed
		}
	}

	return start, end
}

func parseUUIDQueryParam(q url.Values, key string) ([]uuid.UUID, error) {
	values := q[key]
	if len(values) == 0 {
		return nil, nil
	}

	ids := make([]uuid.UUID, 0, len(values))
	for _, rawID := range values {
		id, err := uuid.Parse(rawID)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (h *handler) handleCreateFunnel() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, ok := parseSiteID(w, r)
		if !ok {
			return
		}

		var req api.Funnel
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if req.Name == "" || len(req.Steps) < 2 {
			http.Error(w, "Invalid funnel data (need name and at least 2 steps)", http.StatusBadRequest)
			return
		}

		req.SiteID = siteID
		req.CreatedAt = time.Now()

		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), siteID)
		if err != nil {
			slog.Error("Failed to resolve analytics store", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if err := analyticsStore.CreateFunnel(r.Context(), &req); err != nil {
			slog.Error("Failed to create funnel", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if analyticsStore != h.ctx.Store {
			if err := h.ctx.Store.UpsertFunnel(r.Context(), &req); err != nil {
				slog.Error("Failed to write legacy shared funnel", "error", err, "site_id", siteID, "funnel_id", req.ID)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
		}
		h.publishDefinitionChange(siteID, realtime.KindFunnels)

		w.WriteHeader(http.StatusCreated)
	}
}

func (h *handler) handleDeleteFunnel() http.HandlerFunc {
	return h.handleDeleteDefinition(
		"funnelID",
		"Invalid funnel_id",
		func(ctx context.Context, store *database.Store, definitionID uuid.UUID, siteID uuid.UUID) error {
			return store.DeleteFunnel(ctx, definitionID, siteID)
		},
		"Failed to delete funnel",
		"Failed to delete legacy shared funnel",
		realtime.KindFunnels,
		nil,
	)
}

func (h *handler) publishDefinitionChange(siteID uuid.UUID, kind string) {
	if h.ctx == nil || h.ctx.Realtime == nil || kind == "" {
		return
	}
	now := time.Now().UTC()
	h.ctx.Realtime.Publish(realtime.Event{
		SiteID:      siteID,
		Kinds:       []string{kind},
		ChangedAt:   now,
		BucketStart: now.Truncate(time.Minute),
		Counts:      map[string]int{kind: 1},
	})
}

func (h *handler) handleGetFunnelStats() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, ok := parseSiteID(w, r)
		if !ok {
			return
		}

		userID := shared.GetUserIDFromContext(r)

		funnelIDStr := r.PathValue("funnelID")
		funnelID, err := uuid.Parse(funnelIDStr)
		if err != nil {
			http.Error(w, "Invalid funnel_id", http.StatusBadRequest)
			return
		}

		// Parse time range
		now := time.Now().UTC()
		end := now.AddDate(0, 0, 1)
		start := end.AddDate(0, 0, -30)

		q := r.URL.Query()
		if fromStr := q.Get("from"); fromStr != "" {
			if parsed, err := time.Parse(time.RFC3339, fromStr); err == nil {
				start = parsed
			}
		}
		if toStr := q.Get("to"); toStr != "" {
			if parsed, err := time.Parse(time.RFC3339, toStr); err == nil {
				end = parsed
			}
		}

		params := api.AnalyticsParams{
			SiteID: siteID,
			UserID: userID,
			Start:  start,
			End:    end,
		}

		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), siteID)
		if err != nil {
			slog.Error("Failed to resolve analytics store", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		stats, err := analyticsStore.GetFunnelStats(r.Context(), funnelID, params)
		if err != nil {
			slog.Error("Failed to get funnel stats", "error", err)
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, "Funnel not found", http.StatusNotFound)
			} else {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(stats); err != nil {
			slog.Error("Failed to encode response", "error", err)
		}
	}
}
