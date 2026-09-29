package askai

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
	"hitkeep/server/shared"
)

func (h *handler) handleStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}
		siteID, ok := shared.PathUUID(w, r, "id", "Invalid site ID")
		if !ok {
			return
		}
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if apiClientAuthFromRequest(r) != nil {
			http.Error(w, "Ask AI requires a dashboard session", http.StatusForbidden)
			return
		}
		site, err := h.ctx.Store.GetSiteByID(r.Context(), siteID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to load Ask AI status site", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if site == nil {
			http.Error(w, "Site not found", http.StatusNotFound)
			return
		}
		allowed, err := h.hasAskAISiteView(r.Context(), userID, siteID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to resolve Ask AI status permission", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if !allowed {
			http.Error(w, "Access denied", http.StatusForbidden)
			return
		}
		teamID, err := h.ctx.Store.GetSiteTenantID(r.Context(), siteID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to resolve Ask AI status team", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		status, err := h.askAIStatusForTeam(r.Context(), teamID, userID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to load Ask AI daily usage", "error", err, "team_id", teamID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		shared.WriteJSON(r.Context(), w, http.StatusOK, status)
	}
}

type teamQuotaSlot struct {
	ch   chan struct{}
	refs int
}

// lockTeamQuota serializes completed-answer accounting for one team on the
// leader. Waiting requests honor cancellation; unused slots are removed.
func (h *handler) lockTeamQuota(ctx context.Context, teamID uuid.UUID) (func(), error) {
	h.quotaMu.Lock()
	if h.quotaSlots == nil {
		h.quotaSlots = make(map[uuid.UUID]*teamQuotaSlot)
	}
	slot := h.quotaSlots[teamID]
	if slot == nil {
		slot = &teamQuotaSlot{ch: make(chan struct{}, 1)}
		h.quotaSlots[teamID] = slot
	}
	slot.refs++
	h.quotaMu.Unlock()

	releaseRef := func() {
		h.quotaMu.Lock()
		slot.refs--
		if slot.refs == 0 {
			delete(h.quotaSlots, teamID)
		}
		h.quotaMu.Unlock()
	}
	select {
	case slot.ch <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-slot.ch
			releaseRef()
			return nil, err
		}
		return func() {
			<-slot.ch
			releaseRef()
		}, nil
	case <-ctx.Done():
		releaseRef()
		return nil, ctx.Err()
	}
}

func (h *handler) askAIStatusForTeam(ctx context.Context, teamID, userID uuid.UUID) (*api.AskAIStatus, error) {
	status := shared.AskAIStatus(ctx, h.ctx.Config, h.ctx.Store)
	if h.ctx.Store == nil {
		return status, nil
	}
	limit := h.ctx.Limits().TeamAskAIDailyLimit(ctx, userID, teamID)
	if limit <= 0 {
		return status, nil
	}
	start := time.Now().UTC().Truncate(24 * time.Hour)
	reset := start.Add(24 * time.Hour)
	used, err := h.ctx.Store.CountSuccessfulAskAIResponses(ctx, teamID, start, reset)
	if err != nil {
		return nil, err
	}
	remaining := max(0, limit-used)
	status.DailyLimit = &limit
	status.DailyUsed = &used
	status.DailyRemaining = &remaining
	status.DailyResetAt = &reset
	if remaining == 0 && status.Available {
		status.Status = "daily_limit_exhausted"
		status.Available = false
	}
	return status, nil
}
