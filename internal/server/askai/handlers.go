package askai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	hitai "hitkeep/internal/ai"
	"hitkeep/internal/analyticstools"
	"hitkeep/internal/api"
	authcore "hitkeep/internal/auth"
	"hitkeep/internal/database"
	"hitkeep/internal/server/shared"
	publicskills "hitkeep/skills"
)

const (
	maxAskAIQueryLength = 2000
	maxAskAIRangeDays   = 366
	askAIStreamAuditTTL = 5 * time.Second

	askAIAuditActionRequested     = "ask_ai.requested"
	askAIAuditActionResponded     = "ask_ai.responded"
	askAIAuditActionHistoryViewed = "ask_ai.history_viewed"
)

var errAskAIStreamWriteFailed = errors.New("ask ai stream write failed")

type handler struct {
	ctx *shared.Context
}

type askAIStreamingClient interface {
	StreamAskAI(context.Context, hitai.AskAIRequest, hitai.AskAIStreamSink) (hitai.AskAIResult, error)
}

func Register(mux *http.ServeMux, ctx *shared.Context) {
	h := &handler{ctx: ctx}
	mux.HandleFunc("POST /api/sites/{id}/ask-ai", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		AllowAPIKey: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleAsk()))
	mux.HandleFunc("POST /api/sites/{id}/ask-ai/events", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		AllowAPIKey: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleAskEvents()))
	mux.HandleFunc("GET /api/sites/{id}/ask-ai/history", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		AllowAPIKey: true,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleHistory()))
}

func (h *handler) handleAsk() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		prepared, ok := h.prepareAskAI(w, r)
		if !ok {
			return
		}
		result, err := h.ctx.AI.GenerateAskAI(r.Context(), prepared.AIRequest)
		if err != nil {
			if !h.auditAskAIResponse(w, r, prepared.AuditContext, askAIResponseAuditInput{
				RequestHash: prepared.RequestHash,
				Result:      &result,
				Outcome:     "failure",
				Status:      askAIErrorAuditStatus(err),
				HTTPStatus:  statusCodeForAskAIError(err),
				Err:         err,
			}) {
				return
			}
			h.handleAskAIError(w, err, prepared.SiteID)
			return
		}
		if !h.auditAskAIResponse(w, r, prepared.AuditContext, askAIResponseAuditInput{
			RequestHash: prepared.RequestHash,
			Result:      &result,
			Outcome:     "success",
			Status:      "success",
			HTTPStatus:  http.StatusOK,
		}) {
			return
		}
		writeJSON(w, http.StatusOK, apiAskAIResponse(result))
	}
}

func (h *handler) generateAskAIForEventStream(ctx context.Context, req hitai.AskAIRequest, stream askAIEventStream) (hitai.AskAIResult, error) {
	if client, ok := h.ctx.AI.(askAIStreamingClient); ok {
		return client.StreamAskAI(ctx, req, func(delta hitai.AskAIStreamDelta) error {
			switch delta.Type {
			case hitai.AskAIStreamDeltaAnswer:
				if delta.TextDelta == "" {
					return nil
				}
				if !stream.write("delta", api.AskAIStreamEvent{
					Type:          "delta",
					Status:        "streaming",
					DeltaMarkdown: delta.TextDelta,
				}) {
					return errAskAIStreamWriteFailed
				}
			case hitai.AskAIStreamDeltaProgress:
				messageKey := delta.MessageKey
				if messageKey == "" {
					messageKey = "askAi.progress.generating"
				}
				status := delta.Status
				if status == "" {
					status = "generating"
				}
				if !stream.write("progress", api.AskAIStreamEvent{
					Type:       "progress",
					Status:     status,
					MessageKey: messageKey,
					ToolCallID: delta.ToolCallID,
					ToolName:   delta.ToolName,
				}) {
					return errAskAIStreamWriteFailed
				}
			}
			return nil
		})
	}
	return h.ctx.AI.GenerateAskAI(ctx, req)
}

func (h *handler) handleAskEvents() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		prepared, ok := h.prepareAskAI(w, r)
		if !ok {
			return
		}
		stream, ok := newAskAIEventStream(w)
		if !ok {
			if !h.auditAskAIResponse(w, r, prepared.AuditContext, askAIResponseAuditInput{
				RequestHash: prepared.RequestHash,
				Outcome:     "failure",
				Status:      "streaming_unavailable",
				HTTPStatus:  http.StatusInternalServerError,
			}) {
				return
			}
			http.Error(w, "Ask AI streaming is not available", http.StatusInternalServerError)
			return
		}
		if !stream.write("progress", api.AskAIStreamEvent{
			Type:       "progress",
			Status:     "accepted",
			MessageKey: "askAi.progress.accepted",
		}) {
			h.auditAskAIStreamResponse(r, prepared.AuditContext, askAIResponseAuditInput{
				RequestHash: prepared.RequestHash,
				Outcome:     "failure",
				Status:      "stream_write_failed",
				HTTPStatus:  http.StatusOK,
			})
			return
		}
		if !stream.write("progress", api.AskAIStreamEvent{
			Type:       "progress",
			Status:     "generating",
			MessageKey: "askAi.progress.generating",
		}) {
			h.auditAskAIStreamResponse(r, prepared.AuditContext, askAIResponseAuditInput{
				RequestHash: prepared.RequestHash,
				Outcome:     "failure",
				Status:      "stream_write_failed",
				HTTPStatus:  http.StatusOK,
			})
			return
		}

		result, err := h.generateAskAIForEventStream(r.Context(), prepared.AIRequest, stream)
		if err != nil {
			status := askAIErrorAuditStatus(err)
			httpStatus := statusCodeForAskAIError(err)
			if errors.Is(err, errAskAIStreamWriteFailed) {
				status = "stream_write_failed"
				httpStatus = http.StatusOK
			}
			if !h.auditAskAIStreamResponse(r, prepared.AuditContext, askAIResponseAuditInput{
				RequestHash: prepared.RequestHash,
				Result:      &result,
				Outcome:     "failure",
				Status:      status,
				HTTPStatus:  httpStatus,
				Err:         err,
			}) {
				stream.write("error", api.AskAIStreamEvent{Type: "error", Status: "audit_failed", MessageKey: "askAi.errors.request"})
				return
			}
			if errors.Is(err, errAskAIStreamWriteFailed) {
				return
			}
			stream.write("error", api.AskAIStreamEvent{Type: "error", Status: status, MessageKey: askAIStreamErrorMessageKey(err), Error: http.StatusText(httpStatus)})
			return
		}
		response := apiAskAIResponse(result)
		if !h.auditAskAIStreamResponse(r, prepared.AuditContext, askAIResponseAuditInput{
			RequestHash: prepared.RequestHash,
			Result:      &result,
			Outcome:     "success",
			Status:      "success",
			HTTPStatus:  http.StatusOK,
		}) {
			stream.write("error", api.AskAIStreamEvent{Type: "error", Status: "audit_failed", MessageKey: "askAi.errors.request"})
			return
		}
		if !stream.write("final", api.AskAIStreamEvent{
			Type:       "final",
			Status:     "success",
			MessageKey: "askAi.progress.complete",
			Response:   &response,
		}) {
			h.auditAskAIStreamResponse(r, prepared.AuditContext, askAIResponseAuditInput{
				RequestHash: prepared.RequestHash,
				Outcome:     "failure",
				Status:      "stream_write_failed",
				HTTPStatus:  http.StatusOK,
			})
			return
		}
	}
}

func (h *handler) handleHistory() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		prepared, ok := h.prepareAskAIHistory(w, r)
		if !ok {
			return
		}
		limit, offset, err := parseAskAIHistoryPagination(r)
		if err != nil {
			if !h.auditAskAIHistory(w, r, prepared.AuditContext, askAIHistoryAuditInput{
				Outcome:    "failure",
				Status:     "invalid_request",
				HTTPStatus: http.StatusBadRequest,
			}) {
				return
			}
			http.Error(w, "Invalid Ask AI history request", http.StatusBadRequest)
			return
		}
		entries, total, err := h.ctx.Store.ListAskAIHistory(r.Context(), prepared.SiteID, limit, offset)
		if err != nil {
			slog.Error("Failed to list Ask AI history", "error", err, "site_id", prepared.SiteID)
			if !h.auditAskAIHistory(w, r, prepared.AuditContext, askAIHistoryAuditInput{
				Outcome:    "failure",
				Status:     "history_unavailable",
				HTTPStatus: http.StatusInternalServerError,
				Limit:      limit,
				Offset:     offset,
			}) {
				return
			}
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if !h.auditAskAIHistory(w, r, prepared.AuditContext, askAIHistoryAuditInput{
			Outcome:    "success",
			Status:     "success",
			HTTPStatus: http.StatusOK,
			Limit:      limit,
			Offset:     offset,
			Total:      total,
		}) {
			return
		}
		writeJSON(w, http.StatusOK, apiAskAIHistoryResponse(entries, total, limit, offset))
	}
}

type askAIPreparedRun struct {
	SiteID       uuid.UUID
	RequestHash  string
	AuditContext askAIAuditContext
	AIRequest    hitai.AskAIRequest
}

type askAIHistoryPrepared struct {
	SiteID       uuid.UUID
	AuditContext askAIAuditContext
}

func (h *handler) prepareAskAI(w http.ResponseWriter, r *http.Request) (askAIPreparedRun, bool) {
	var prepared askAIPreparedRun
	if h.ctx.Store == nil {
		http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
		return prepared, false
	}
	siteID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		http.Error(w, "Invalid site ID", http.StatusBadRequest)
		return prepared, false
	}
	userID := shared.GetUserIDFromContext(r)
	if userID == uuid.Nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return prepared, false
	}
	if apiClientAuth := apiClientAuthFromRequest(r); shouldDenyAskAIAPIClientBeforeSiteLookup(apiClientAuth, siteID) {
		auditCtx := askAIAuditContext{SiteID: siteID, UserID: userID}
		if apiClientAuth.TenantID != uuid.Nil {
			auditCtx.TeamID = apiClientAuth.TenantID
		}
		if !h.auditAskAIRequestAndResponse(w, r, auditCtx, askAIRequestAuditInput{
			Outcome:    "denied",
			Status:     "dashboard_session_required",
			HTTPStatus: http.StatusForbidden,
		}, askAIResponseAuditInput{
			Outcome:    "denied",
			Status:     "dashboard_session_required",
			HTTPStatus: http.StatusForbidden,
		}) {
			return prepared, false
		}
		http.Error(w, "Ask AI requires a dashboard session", http.StatusForbidden)
		return prepared, false
	}
	site, err := h.ctx.Store.GetSiteByID(r.Context(), siteID)
	if err != nil {
		slog.Error("Failed to load Ask AI site", "error", err, "site_id", siteID)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return prepared, false
	}
	if site == nil {
		http.Error(w, "Site not found", http.StatusNotFound)
		return prepared, false
	}
	teamID, err := h.ctx.Store.GetSiteTenantID(r.Context(), siteID)
	if err != nil {
		slog.Error("Failed to resolve Ask AI team", "error", err, "site_id", siteID)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return prepared, false
	}
	auditCtx := askAIAuditContext{TeamID: teamID, SiteID: siteID, SiteDomain: site.Domain, UserID: userID}
	if apiClientAuthFromRequest(r) != nil {
		if !h.auditAskAIRequestAndResponse(w, r, auditCtx, askAIRequestAuditInput{
			Outcome:    "denied",
			Status:     "dashboard_session_required",
			HTTPStatus: http.StatusForbidden,
		}, askAIResponseAuditInput{
			Outcome:    "denied",
			Status:     "dashboard_session_required",
			HTTPStatus: http.StatusForbidden,
		}) {
			return prepared, false
		}
		http.Error(w, "Ask AI requires a dashboard session", http.StatusForbidden)
		return prepared, false
	}
	hasSiteView, err := h.hasAskAISiteView(r.Context(), userID, siteID)
	if err != nil {
		slog.Error("Failed to resolve Ask AI site permission", "error", err, "site_id", siteID, "user_id", userID)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return prepared, false
	}
	if !hasSiteView {
		if !h.auditAskAIRequestAndResponse(w, r, auditCtx, askAIRequestAuditInput{
			Outcome:    "denied",
			Status:     "access_denied",
			HTTPStatus: http.StatusForbidden,
		}, askAIResponseAuditInput{
			Outcome:    "denied",
			Status:     "access_denied",
			HTTPStatus: http.StatusForbidden,
		}) {
			return prepared, false
		}
		http.Error(w, "Access denied", http.StatusForbidden)
		return prepared, false
	}
	status := shared.AskAIStatus(r.Context(), h.ctx.Config, h.ctx.Store)
	if status == nil || !status.Available {
		code := statusCodeForAskAIStatus(status)
		statusLabel := "unavailable"
		if status != nil {
			statusLabel = status.Status
		}
		if !h.auditAskAIRequestAndResponse(w, r, auditCtx, askAIRequestAuditInput{
			Outcome:    "failure",
			Status:     statusLabel,
			HTTPStatus: code,
		}, askAIResponseAuditInput{
			Outcome:    "failure",
			Status:     statusLabel,
			HTTPStatus: code,
		}) {
			return prepared, false
		}
		writeJSON(w, code, status)
		return prepared, false
	}
	if h.ctx.AI == nil {
		if !h.auditAskAIRequestAndResponse(w, r, auditCtx, askAIRequestAuditInput{
			Outcome:    "failure",
			Status:     "not_configured",
			HTTPStatus: http.StatusConflict,
		}, askAIResponseAuditInput{
			Outcome:    "failure",
			Status:     "not_configured",
			HTTPStatus: http.StatusConflict,
		}) {
			return prepared, false
		}
		http.Error(w, "Ask AI is not configured", http.StatusConflict)
		return prepared, false
	}
	request, err := decodeAskAIRequest(w, r)
	if err != nil {
		if !h.auditAskAIRequestAndResponse(w, r, auditCtx, askAIRequestAuditInput{
			Outcome:    "failure",
			Status:     "invalid_request",
			HTTPStatus: http.StatusBadRequest,
		}, askAIResponseAuditInput{
			Outcome:    "failure",
			Status:     "invalid_request",
			HTTPStatus: http.StatusBadRequest,
		}) {
			return prepared, false
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return prepared, false
	}
	from, to, derivedRange, err := h.resolveAskAIRange(r.Context(), siteID, site.CreatedAt, request.From, request.To)
	if err != nil {
		if !h.auditAskAIRequestAndResponse(w, r, auditCtx, askAIRequestAuditInput{
			Request:    &request,
			Outcome:    "failure",
			Status:     "invalid_date_range",
			HTTPStatus: http.StatusBadRequest,
		}, askAIResponseAuditInput{
			Outcome:    "failure",
			Status:     "invalid_date_range",
			HTTPStatus: http.StatusBadRequest,
		}) {
			return prepared, false
		}
		http.Error(w, "Invalid date range", http.StatusBadRequest)
		return prepared, false
	}
	maxRangeDays := h.ctx.Config.MCPMaxRangeDays
	if maxRangeDays <= 0 {
		maxRangeDays = maxAskAIRangeDays
	}
	if to.Sub(from) > time.Duration(maxRangeDays)*24*time.Hour {
		if derivedRange {
			from = to.Add(-time.Duration(maxRangeDays) * 24 * time.Hour)
		} else {
			requestHash := askAIRequestAuditHash(request, from, to, nil)
			if !h.auditAskAIRequestAndResponse(w, r, auditCtx, askAIRequestAuditInput{
				Request:     &request,
				From:        from,
				To:          to,
				RequestHash: requestHash,
				Outcome:     "failure",
				Status:      "date_range_too_large",
				HTTPStatus:  http.StatusBadRequest,
			}, askAIResponseAuditInput{
				RequestHash: requestHash,
				Outcome:     "failure",
				Status:      "date_range_too_large",
				HTTPStatus:  http.StatusBadRequest,
			}) {
				return prepared, false
			}
			http.Error(w, "Date range is too large", http.StatusBadRequest)
			return prepared, false
		}
	}
	filters, err := askAIFilters(request.Filters)
	if err != nil {
		requestHash := askAIRequestAuditHash(request, from, to, nil)
		if !h.auditAskAIRequestAndResponse(w, r, auditCtx, askAIRequestAuditInput{
			Request:     &request,
			From:        from,
			To:          to,
			RequestHash: requestHash,
			Outcome:     "failure",
			Status:      "invalid_filter",
			HTTPStatus:  http.StatusBadRequest,
		}, askAIResponseAuditInput{
			RequestHash: requestHash,
			Outcome:     "failure",
			Status:      "invalid_filter",
			HTTPStatus:  http.StatusBadRequest,
		}) {
			return prepared, false
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return prepared, false
	}
	requestHash := askAIRequestAuditHash(request, from, to, filters)
	if !h.auditAskAIRequest(w, r, auditCtx, askAIRequestAuditInput{
		Request:     &request,
		From:        from,
		To:          to,
		Filters:     filters,
		RequestHash: requestHash,
		Outcome:     "success",
		Status:      "accepted",
		HTTPStatus:  http.StatusOK,
	}) {
		return prepared, false
	}
	analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), siteID)
	if err != nil {
		slog.Error("Failed to resolve Ask AI analytics store", "error", err, "site_id", siteID)
		if !h.auditAskAIResponse(w, r, auditCtx, askAIResponseAuditInput{
			RequestHash: requestHash,
			Outcome:     "failure",
			Status:      "analytics_store_unavailable",
			HTTPStatus:  http.StatusInternalServerError,
		}) {
			return prepared, false
		}
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return prepared, false
	}
	tools := analyticstools.NewBridge(analyticstools.Config{
		Analytics: analyticsStore, SiteID: siteID, UserID: userID, From: from, To: to, Filters: filters,
	}).Tools()
	return askAIPreparedRun{
		SiteID:       siteID,
		RequestHash:  requestHash,
		AuditContext: auditCtx,
		AIRequest: hitai.AskAIRequest{
			TeamID: teamID, SiteID: siteID, ActorID: userID, ActorType: "user", SiteDomain: site.Domain,
			Query: request.Query, From: from, To: to, Route: request.Route, Filters: toAIAskFilters(filters), History: toAIAskHistory(request.History),
			SkillText: publicskills.EmbeddedAnalyticsProcedurePack(), Tools: tools,
		},
	}, true
}

func (h *handler) prepareAskAIHistory(w http.ResponseWriter, r *http.Request) (askAIHistoryPrepared, bool) {
	var prepared askAIHistoryPrepared
	if h.ctx.Store == nil {
		http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
		return prepared, false
	}
	siteID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		http.Error(w, "Invalid site ID", http.StatusBadRequest)
		return prepared, false
	}
	userID := shared.GetUserIDFromContext(r)
	if userID == uuid.Nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return prepared, false
	}
	if apiClientAuth := apiClientAuthFromRequest(r); shouldDenyAskAIAPIClientBeforeSiteLookup(apiClientAuth, siteID) {
		auditCtx := askAIAuditContext{SiteID: siteID, UserID: userID}
		if apiClientAuth.TenantID != uuid.Nil {
			auditCtx.TeamID = apiClientAuth.TenantID
		}
		if !h.auditAskAIHistory(w, r, auditCtx, askAIHistoryAuditInput{
			Outcome:    "denied",
			Status:     "dashboard_session_required",
			HTTPStatus: http.StatusForbidden,
		}) {
			return prepared, false
		}
		http.Error(w, "Ask AI requires a dashboard session", http.StatusForbidden)
		return prepared, false
	}
	site, err := h.ctx.Store.GetSiteByID(r.Context(), siteID)
	if err != nil {
		slog.Error("Failed to load Ask AI history site", "error", err, "site_id", siteID)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return prepared, false
	}
	if site == nil {
		http.Error(w, "Site not found", http.StatusNotFound)
		return prepared, false
	}
	teamID, err := h.ctx.Store.GetSiteTenantID(r.Context(), siteID)
	if err != nil {
		slog.Error("Failed to resolve Ask AI history team", "error", err, "site_id", siteID)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return prepared, false
	}
	auditCtx := askAIAuditContext{TeamID: teamID, SiteID: siteID, SiteDomain: site.Domain, UserID: userID}
	if apiClientAuthFromRequest(r) != nil {
		if !h.auditAskAIHistory(w, r, auditCtx, askAIHistoryAuditInput{
			Outcome:    "denied",
			Status:     "dashboard_session_required",
			HTTPStatus: http.StatusForbidden,
		}) {
			return prepared, false
		}
		http.Error(w, "Ask AI requires a dashboard session", http.StatusForbidden)
		return prepared, false
	}
	hasSiteView, err := h.hasAskAISiteView(r.Context(), userID, siteID)
	if err != nil {
		slog.Error("Failed to resolve Ask AI history site permission", "error", err, "site_id", siteID, "user_id", userID)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return prepared, false
	}
	if !hasSiteView {
		if !h.auditAskAIHistory(w, r, auditCtx, askAIHistoryAuditInput{
			Outcome:    "denied",
			Status:     "access_denied",
			HTTPStatus: http.StatusForbidden,
		}) {
			return prepared, false
		}
		http.Error(w, "Access denied", http.StatusForbidden)
		return prepared, false
	}
	status := shared.AskAIStatus(r.Context(), h.ctx.Config, h.ctx.Store)
	if status == nil || !status.Enabled {
		code := statusCodeForAskAIStatus(status)
		statusLabel := "unavailable"
		if status != nil {
			statusLabel = status.Status
		}
		if !h.auditAskAIHistory(w, r, auditCtx, askAIHistoryAuditInput{
			Outcome:    "failure",
			Status:     statusLabel,
			HTTPStatus: code,
		}) {
			return prepared, false
		}
		writeJSON(w, code, status)
		return prepared, false
	}
	return askAIHistoryPrepared{SiteID: siteID, AuditContext: auditCtx}, true
}

type askAIEventStream struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

func newAskAIEventStream(w http.ResponseWriter) (askAIEventStream, bool) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return askAIEventStream{}, false
	}
	header := w.Header()
	header.Set("Content-Type", "text/event-stream; charset=utf-8")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("X-Accel-Buffering", "no")
	return askAIEventStream{w: w, flusher: flusher}, true
}

func (s askAIEventStream) write(event string, payload api.AskAIStreamEvent) bool {
	raw, err := json.Marshal(payload)
	if err != nil {
		slog.Error("Failed to encode Ask AI stream event", "error", err, "event", event)
		return false
	}
	if _, err := fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, raw); err != nil {
		slog.Debug("Ask AI stream write failed", "error", err, "event", event)
		return false
	}
	s.flusher.Flush()
	return true
}

func askAIStreamErrorMessageKey(err error) string {
	switch {
	case errors.Is(err, hitai.ErrBudgetExhausted):
		return "askAi.errors.budget"
	case errors.Is(err, hitai.ErrDisabled), errors.Is(err, hitai.ErrNotConfigured):
		return "askAi.errors.notConfigured"
	default:
		return "askAi.errors.request"
	}
}

func decodeAskAIRequest(w http.ResponseWriter, r *http.Request) (api.AskAIRequest, error) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var req api.AskAIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return api.AskAIRequest{}, errors.New("invalid request body")
	}
	req.Query = strings.TrimSpace(req.Query)
	if req.Query == "" {
		return api.AskAIRequest{}, errors.New("query is required")
	}
	if len(req.Query) > maxAskAIQueryLength {
		return api.AskAIRequest{}, errors.New("query is too long")
	}
	req.Route = strings.TrimSpace(req.Route)
	req.History = trimAPIAskAIHistory(req.History, 8)
	return req, nil
}

func (h *handler) resolveAskAIRange(ctx context.Context, siteID uuid.UUID, siteCreatedAt time.Time, fromRaw, toRaw string) (time.Time, time.Time, bool, error) {
	fromRaw = strings.TrimSpace(fromRaw)
	toRaw = strings.TrimSpace(toRaw)
	if fromRaw != "" && toRaw != "" {
		from, to, err := parseExplicitAskAIRange(fromRaw, toRaw)
		return from, to, false, err
	}

	boundsFrom, boundsTo, err := h.ctx.Store.GetSiteAnalyticsBounds(ctx, siteID)
	if err != nil {
		return time.Time{}, time.Time{}, false, err
	}

	var from, to time.Time
	switch {
	case fromRaw != "":
		from, err = parseAskAITime(fromRaw, false)
		if err != nil {
			return time.Time{}, time.Time{}, false, err
		}
		to = boundsTo
		if to.IsZero() {
			to = time.Now().UTC()
		}
	case toRaw != "":
		to, err = parseAskAITime(toRaw, true)
		if err != nil {
			return time.Time{}, time.Time{}, false, err
		}
		from = boundsFrom
		if from.IsZero() {
			from = siteCreatedAt
		}
	case !boundsFrom.IsZero() && !boundsTo.IsZero():
		from, to = boundsFrom, boundsTo
	default:
		from = siteCreatedAt
		to = time.Now().UTC()
	}

	from, to, err = normalizeAskAIRange(from, to, fromRaw == "" && toRaw == "")
	return from, to, true, err
}

func parseExplicitAskAIRange(fromRaw, toRaw string) (time.Time, time.Time, error) {
	from, err := parseAskAITime(fromRaw, false)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	to, err := parseAskAITime(toRaw, true)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return normalizeAskAIRange(from, to, false)
}

func normalizeAskAIRange(from, to time.Time, allowInstant bool) (time.Time, time.Time, error) {
	if from.IsZero() || to.IsZero() {
		return time.Time{}, time.Time{}, errors.New("invalid date range")
	}
	from = from.UTC()
	to = to.UTC()
	if from.Equal(to) && allowInstant {
		to = to.Add(time.Nanosecond)
	}
	if !from.Before(to) {
		return time.Time{}, time.Time{}, errors.New("invalid date range")
	}
	return from, to, nil
}

func parseAskAITime(raw string, endOfDateOnly bool) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed.UTC(), nil
	}
	parsed, err := time.Parse(time.DateOnly, raw)
	if err != nil {
		return time.Time{}, err
	}
	if endOfDateOnly {
		parsed = parsed.AddDate(0, 0, 1)
	}
	return parsed.UTC(), nil
}

var askAIFilterTypes = map[string]bool{
	"path": true, "hostname": true, "referrer": true, "referrer_host": true, "device": true, "country": true,
	"city": true, "provider": true, "asn": true, "browser": true, "language": true, "utm_campaign": true,
	"utm_content": true, "utm_medium": true, "utm_source": true, "utm_term": true, "qr_code_id": true,
}

func askAIFilters(filters []api.AskAIFilter) ([]api.Filter, error) {
	out := make([]api.Filter, 0, len(filters))
	for _, filter := range filters {
		filterType := strings.ToLower(strings.TrimSpace(filter.Type))
		filterValue := strings.TrimSpace(filter.Value)
		if filterType == "" || filterValue == "" {
			return nil, errors.New("filter type and value are required together")
		}
		if !askAIFilterTypes[filterType] {
			return nil, errors.New("invalid filter type")
		}
		out = append(out, api.Filter{Type: filterType, Value: filterValue})
	}
	return out, nil
}

func trimAPIAskAIHistory(history []api.AskAIMessage, limit int) []api.AskAIMessage {
	if len(history) > limit {
		history = history[len(history)-limit:]
	}
	out := make([]api.AskAIMessage, 0, len(history))
	for _, message := range history {
		role := strings.TrimSpace(message.Role)
		if role != "user" && role != "assistant" {
			continue
		}
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		if len(content) > 1200 {
			content = content[:1200]
		}
		out = append(out, api.AskAIMessage{Role: role, Content: content})
	}
	return out
}

func toAIAskFilters(filters []api.Filter) []hitai.AskAIFilter {
	out := make([]hitai.AskAIFilter, 0, len(filters))
	for _, filter := range filters {
		out = append(out, hitai.AskAIFilter{Type: filter.Type, Value: filter.Value})
	}
	return out
}

func toAIAskHistory(history []api.AskAIMessage) []hitai.AskAIMessage {
	out := make([]hitai.AskAIMessage, 0, len(history))
	for _, message := range history {
		out = append(out, hitai.AskAIMessage{Role: message.Role, Content: message.Content})
	}
	return out
}

func apiAskAIResponse(result hitai.AskAIResult) api.AskAIResponse {
	output := result.Output
	return api.AskAIResponse{
		RunID:          result.RunID.String(),
		AnswerMarkdown: output.AnswerMarkdown,
		Citations:      apiAskAICitations(output.Citations),
		Charts:         apiAskAICharts(output.Charts),
		Actions:        apiAskAIActions(output.Actions),
	}
}

func apiAskAICitations(citations []hitai.AskAICitation) []api.AskAICitation {
	out := make([]api.AskAICitation, 0, len(citations))
	for _, citation := range citations {
		out = append(out, api.AskAICitation{Label: citation.Label, ToolCallID: citation.ToolCallID})
	}
	return out
}

func apiAskAICharts(charts []hitai.AskAIChart) []api.AskAIChart {
	out := make([]api.AskAIChart, 0, len(charts))
	for _, chart := range charts {
		out = append(out, api.AskAIChart{
			Type: chart.Type, Title: chart.Title, XKey: chart.XKey, Series: apiAskAIChartSeries(chart.Series), Rows: chart.Rows,
		})
	}
	return out
}

func apiAskAIChartSeries(series []hitai.AskAIChartSeries) []api.AskAIChartSeries {
	out := make([]api.AskAIChartSeries, 0, len(series))
	for _, item := range series {
		out = append(out, api.AskAIChartSeries{Key: item.Key, Label: item.Label})
	}
	return out
}

func apiAskAIActions(actions []hitai.AskAIAction) []api.AskAIAction {
	out := make([]api.AskAIAction, 0, len(actions))
	for _, action := range actions {
		out = append(out, api.AskAIAction{Type: action.Type, Label: action.Label, Target: action.Target, Format: action.Format})
	}
	return out
}

func apiAskAIHistoryResponse(entries []database.AskAIHistoryEntry, total, limit, offset int) api.AskAIHistoryResponse {
	out := make([]api.AskAIHistoryEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, api.AskAIHistoryEntry{
			RunID:               entry.RunID.String(),
			CreatedAt:           entry.CreatedAt,
			Status:              entry.Status,
			ErrorCategory:       entry.ErrorCategory,
			Provider:            entry.Provider,
			Model:               entry.Model,
			TemplateVersion:     entry.TemplateVersion,
			InputHash:           entry.InputHash,
			OutputHash:          entry.OutputHash,
			AnswerChars:         entry.AnswerChars,
			CitationCount:       entry.CitationCount,
			ChartCount:          entry.ChartCount,
			ActionCount:         entry.ActionCount,
			ChartTypes:          nonNilStringSlice(entry.ChartTypes),
			ActionTypes:         nonNilStringSlice(entry.ActionTypes),
			InputTokens:         entry.InputTokens,
			OutputTokens:        entry.OutputTokens,
			TotalTokens:         entry.TotalTokens,
			ToolCallCount:       entry.ToolCallCount,
			LifecycleEventCount: entry.LifecycleEventCount,
			ToolNames:           nonNilStringSlice(entry.ToolNames),
		})
	}
	return api.AskAIHistoryResponse{
		Entries: out,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
		HasMore: offset+len(out) < total,
	}
}

func nonNilStringSlice(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func parseAskAIHistoryPagination(r *http.Request) (int, int, error) {
	limit, err := parseAskAIHistoryIntQuery(r, "limit", 20)
	if err != nil {
		return 0, 0, err
	}
	offset, err := parseAskAIHistoryIntQuery(r, "offset", 0)
	if err != nil {
		return 0, 0, err
	}
	if limit <= 0 || limit > 100 || offset < 0 {
		return 0, 0, errors.New("invalid ask ai history pagination")
	}
	return limit, offset, nil
}

func parseAskAIHistoryIntQuery(r *http.Request, key string, fallback int) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	return value, nil
}

func statusCodeForAskAIStatus(status *api.AskAIStatus) int {
	if status != nil && status.Status == "budget_exhausted" {
		return http.StatusTooManyRequests
	}
	return http.StatusConflict
}

func statusCodeForAskAIError(err error) int {
	switch {
	case errors.Is(err, hitai.ErrBudgetExhausted):
		return http.StatusTooManyRequests
	case errors.Is(err, hitai.ErrDisabled), errors.Is(err, hitai.ErrNotConfigured):
		return http.StatusConflict
	default:
		return http.StatusBadGateway
	}
}

func askAIErrorAuditStatus(err error) string {
	switch {
	case errors.Is(err, hitai.ErrDisabled):
		return "disabled"
	case errors.Is(err, hitai.ErrNotConfigured):
		return "not_configured"
	case errors.Is(err, hitai.ErrBudgetExhausted):
		return "budget_exhausted"
	case errors.Is(err, hitai.ErrInvalidOutput):
		return "invalid_output"
	default:
		return hitai.ClassifyError(err)
	}
}

func (h *handler) handleAskAIError(w http.ResponseWriter, err error, siteID uuid.UUID) {
	switch {
	case errors.Is(err, hitai.ErrDisabled), errors.Is(err, hitai.ErrNotConfigured):
		http.Error(w, "Ask AI is not configured", http.StatusConflict)
	case errors.Is(err, hitai.ErrBudgetExhausted):
		http.Error(w, "Ask AI budget exhausted", http.StatusTooManyRequests)
	case errors.Is(err, hitai.ErrInvalidOutput):
		http.Error(w, "Ask AI returned an invalid response", http.StatusBadGateway)
	default:
		slog.Error("Ask AI request failed", "error", err, "site_id", siteID)
		http.Error(w, "Ask AI request failed", http.StatusBadGateway)
	}
}

func (h *handler) hasAskAISiteView(ctx context.Context, userID, siteID uuid.UUID) (bool, error) {
	instanceRole, err := h.ctx.Store.GetInstanceRole(ctx, userID)
	if err != nil {
		return false, err
	}
	if instanceRole.HasPermission(authcore.PermSiteView) {
		return true, nil
	}

	siteRole, err := h.ctx.Store.GetSiteRole(ctx, userID, siteID)
	if err != nil {
		if isNoAskAISiteAccessError(err) {
			return false, nil
		}
		return false, err
	}
	return siteRole.HasPermission(authcore.PermSiteView), nil
}

func isNoAskAISiteAccessError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no access to site")
}

func apiClientAuthFromRequest(r *http.Request) *database.APIClientAuth {
	if r == nil {
		return nil
	}
	return apiClientAuthFromContext(r.Context())
}

func apiClientAuthFromContext(ctx context.Context) *database.APIClientAuth {
	apiClientAuth, _ := ctx.Value(shared.APIClientAuthKey).(*database.APIClientAuth)
	return apiClientAuth
}

func shouldDenyAskAIAPIClientBeforeSiteLookup(apiClientAuth *database.APIClientAuth, siteID uuid.UUID) bool {
	if apiClientAuth == nil {
		return false
	}
	if apiClientAuth.TenantID != uuid.Nil {
		return true
	}
	_, hasExplicitSiteScope := apiClientAuth.SiteRoles[siteID]
	return !hasExplicitSiteScope
}

type askAIAuditContext struct {
	TeamID     uuid.UUID
	SiteID     uuid.UUID
	SiteDomain string
	UserID     uuid.UUID
}

type askAIRequestAuditInput struct {
	Request     *api.AskAIRequest
	From        time.Time
	To          time.Time
	Filters     []api.Filter
	RequestHash string
	Outcome     string
	Status      string
	HTTPStatus  int
}

type askAIResponseAuditInput struct {
	RequestHash string
	Result      *hitai.AskAIResult
	Outcome     string
	Status      string
	HTTPStatus  int
	Err         error
}

type askAIHistoryAuditInput struct {
	Outcome    string
	Status     string
	HTTPStatus int
	Limit      int
	Offset     int
	Total      int
}

func (h *handler) auditAskAIRequest(w http.ResponseWriter, r *http.Request, auditCtx askAIAuditContext, input askAIRequestAuditInput) bool {
	event := shared.AuditEvent{
		ActorID:      auditCtx.UserID,
		TeamID:       auditCtx.TeamID,
		Action:       askAIAuditActionRequested,
		TargetType:   "site",
		TargetID:     auditCtx.SiteID.String(),
		TargetLabel:  auditCtx.SiteDomain,
		Outcome:      auditOutcome(input.Outcome, "failure"),
		Details:      askAIRequestAuditDetails(input),
		MetadataJSON: mustAuditJSON(askAIRequestAuditMetadata(input)),
	}
	if err := h.ctx.AppendAuditEventChecked(r.Context(), r, event); err != nil {
		slog.Error("Failed to append Ask AI request audit", "error", err, "site_id", auditCtx.SiteID, "status", input.Status)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return false
	}
	return true
}

func (h *handler) auditAskAIResponse(w http.ResponseWriter, r *http.Request, auditCtx askAIAuditContext, input askAIResponseAuditInput) bool {
	if err := h.appendAskAIResponseAudit(r.Context(), r, auditCtx, input); err != nil {
		slog.Error("Failed to append Ask AI response audit", "error", err, "site_id", auditCtx.SiteID, "status", input.Status)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return false
	}
	return true
}

func (h *handler) auditAskAIHistory(w http.ResponseWriter, r *http.Request, auditCtx askAIAuditContext, input askAIHistoryAuditInput) bool {
	event := shared.AuditEvent{
		ActorID:      auditCtx.UserID,
		TeamID:       auditCtx.TeamID,
		Action:       askAIAuditActionHistoryViewed,
		TargetType:   "site",
		TargetID:     auditCtx.SiteID.String(),
		TargetLabel:  auditCtx.SiteDomain,
		Outcome:      auditOutcome(input.Outcome, "failure"),
		Details:      askAIHistoryAuditDetails(input),
		MetadataJSON: mustAuditJSON(askAIHistoryAuditMetadata(input)),
	}
	if err := h.ctx.AppendAuditEventChecked(r.Context(), r, event); err != nil {
		slog.Error("Failed to append Ask AI history audit", "error", err, "site_id", auditCtx.SiteID, "status", input.Status)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return false
	}
	return true
}

func (h *handler) auditAskAIStreamResponse(r *http.Request, auditCtx askAIAuditContext, input askAIResponseAuditInput) bool {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), askAIStreamAuditTTL)
	defer cancel()
	if err := h.appendAskAIResponseAudit(ctx, r, auditCtx, input); err != nil {
		slog.Error("Failed to append Ask AI stream response audit", "error", err, "site_id", auditCtx.SiteID, "status", input.Status)
		return false
	}
	return true
}

func (h *handler) appendAskAIResponseAudit(ctx context.Context, r *http.Request, auditCtx askAIAuditContext, input askAIResponseAuditInput) error {
	event := shared.AuditEvent{
		ActorID:      auditCtx.UserID,
		TeamID:       auditCtx.TeamID,
		Action:       askAIAuditActionResponded,
		TargetType:   "site",
		TargetID:     auditCtx.SiteID.String(),
		TargetLabel:  auditCtx.SiteDomain,
		Outcome:      auditOutcome(input.Outcome, "failure"),
		Details:      askAIResponseAuditDetails(input),
		MetadataJSON: mustAuditJSON(askAIResponseAuditMetadata(input)),
	}
	return h.ctx.AppendAuditEventChecked(ctx, r, event)
}

func (h *handler) auditAskAIRequestAndResponse(w http.ResponseWriter, r *http.Request, auditCtx askAIAuditContext, request askAIRequestAuditInput, response askAIResponseAuditInput) bool {
	if !h.auditAskAIRequest(w, r, auditCtx, request) {
		return false
	}
	if response.RequestHash == "" {
		response.RequestHash = request.RequestHash
	}
	if response.Outcome == "" {
		response.Outcome = request.Outcome
	}
	if response.Status == "" {
		response.Status = request.Status
	}
	if response.HTTPStatus == 0 {
		response.HTTPStatus = request.HTTPStatus
	}
	return h.auditAskAIResponse(w, r, auditCtx, response)
}

func auditOutcome(outcome, fallback string) string {
	outcome = strings.TrimSpace(outcome)
	if outcome == "" {
		return fallback
	}
	return outcome
}

func askAIRequestAuditHash(req api.AskAIRequest, from, to time.Time, filters []api.Filter) string {
	return hitai.HashAny(map[string]any{
		"query":   strings.TrimSpace(req.Query),
		"from":    from.UTC().Format(time.RFC3339),
		"to":      to.UTC().Format(time.RFC3339),
		"route":   strings.TrimSpace(req.Route),
		"filters": filters,
		"history": trimAPIAskAIHistory(req.History, 8),
	})
}

func askAIRequestAuditDetails(input askAIRequestAuditInput) string {
	parts := []string{
		"status=" + safeAuditValue(input.Status),
		fmt.Sprintf("http_status=%d", input.HTTPStatus),
	}
	if input.RequestHash != "" {
		parts = append(parts, "request_sha256="+input.RequestHash)
	}
	if input.Request != nil {
		query := strings.TrimSpace(input.Request.Query)
		route := strings.TrimSpace(input.Request.Route)
		parts = append(parts,
			"query_sha256="+hitai.HashString(query),
			fmt.Sprintf("query_chars=%d", len(query)),
			"route_sha256="+hitai.HashString(route),
			fmt.Sprintf("route_chars=%d", len(route)),
			fmt.Sprintf("history=%d", len(input.Request.History)),
		)
	}
	if !input.From.IsZero() {
		parts = append(parts, "from="+input.From.UTC().Format(time.DateOnly))
	}
	if !input.To.IsZero() {
		parts = append(parts, "to="+input.To.UTC().Format(time.DateOnly))
	}
	if len(input.Filters) > 0 {
		parts = append(parts, fmt.Sprintf("filters=%d", len(input.Filters)), "filter_types="+strings.Join(askAIFilterTypesForAudit(input.Filters), ","))
	}
	return strings.Join(parts, " ")
}

func askAIResponseAuditDetails(input askAIResponseAuditInput) string {
	parts := []string{
		"status=" + safeAuditValue(input.Status),
		fmt.Sprintf("http_status=%d", input.HTTPStatus),
	}
	if input.RequestHash != "" {
		parts = append(parts, "request_sha256="+input.RequestHash)
	}
	if input.Err != nil {
		if input.Result != nil && input.Result.RunID != uuid.Nil {
			parts = append(parts, "run_id="+input.Result.RunID.String())
		}
		parts = append(parts, "error_category="+safeAuditValue(askAIErrorAuditStatus(input.Err)))
		return strings.Join(parts, " ")
	}
	if input.Result != nil {
		output := input.Result.Output
		parts = append(parts,
			"run_id="+input.Result.RunID.String(),
			"output_sha256="+hitai.HashAny(output),
			"answer_sha256="+hitai.HashString(output.AnswerMarkdown),
			fmt.Sprintf("answer_chars=%d", len(strings.TrimSpace(output.AnswerMarkdown))),
			fmt.Sprintf("citations=%d", len(output.Citations)),
			fmt.Sprintf("charts=%d", len(output.Charts)),
			fmt.Sprintf("actions=%d", len(output.Actions)),
		)
		if actionTypes := askAIActionTypesForAudit(output.Actions); actionTypes != "" {
			parts = append(parts, "action_types="+actionTypes)
		}
		if chartTypes := askAIChartTypesForAudit(output.Charts); chartTypes != "" {
			parts = append(parts, "chart_types="+chartTypes)
		}
	}
	return strings.Join(parts, " ")
}

func askAIHistoryAuditDetails(input askAIHistoryAuditInput) string {
	parts := []string{
		"status=" + safeAuditValue(input.Status),
		fmt.Sprintf("http_status=%d", input.HTTPStatus),
	}
	if input.Limit > 0 {
		parts = append(parts, fmt.Sprintf("limit=%d", input.Limit))
	}
	if input.Offset > 0 {
		parts = append(parts, fmt.Sprintf("offset=%d", input.Offset))
	}
	if input.Total > 0 {
		parts = append(parts, fmt.Sprintf("total=%d", input.Total))
	}
	return strings.Join(parts, " ")
}

func askAIRequestAuditMetadata(input askAIRequestAuditInput) map[string]any {
	metadata := map[string]any{
		"status":      safeAuditValue(input.Status),
		"http_status": input.HTTPStatus,
	}
	if input.RequestHash != "" {
		metadata["request_sha256"] = input.RequestHash
	}
	if input.Request != nil {
		query := strings.TrimSpace(input.Request.Query)
		route := strings.TrimSpace(input.Request.Route)
		metadata["query_sha256"] = hitai.HashString(query)
		metadata["query_chars"] = len(query)
		metadata["route_sha256"] = hitai.HashString(route)
		metadata["route_chars"] = len(route)
		metadata["history_count"] = len(input.Request.History)
	}
	if !input.From.IsZero() {
		metadata["from"] = input.From.UTC().Format(time.RFC3339)
	}
	if !input.To.IsZero() {
		metadata["to"] = input.To.UTC().Format(time.RFC3339)
	}
	if len(input.Filters) > 0 {
		filters := make([]map[string]any, 0, len(input.Filters))
		for _, filter := range input.Filters {
			value := strings.TrimSpace(filter.Value)
			filters = append(filters, map[string]any{
				"type":         strings.TrimSpace(filter.Type),
				"value_sha256": hitai.HashString(value),
				"value_chars":  len(value),
			})
		}
		metadata["filters"] = filters
	}
	return metadata
}

func askAIResponseAuditMetadata(input askAIResponseAuditInput) map[string]any {
	metadata := map[string]any{
		"status":      safeAuditValue(input.Status),
		"http_status": input.HTTPStatus,
	}
	if input.RequestHash != "" {
		metadata["request_sha256"] = input.RequestHash
	}
	if input.Err != nil {
		if input.Result != nil && input.Result.RunID != uuid.Nil {
			metadata["run_id"] = input.Result.RunID.String()
		}
		metadata["error_category"] = askAIErrorAuditStatus(input.Err)
		return metadata
	}
	if input.Result == nil {
		return metadata
	}
	output := input.Result.Output
	metadata["run_id"] = input.Result.RunID.String()
	metadata["output_sha256"] = hitai.HashAny(output)
	metadata["answer_sha256"] = hitai.HashString(output.AnswerMarkdown)
	metadata["answer_chars"] = len(strings.TrimSpace(output.AnswerMarkdown))
	metadata["citations"] = askAICitationsForAudit(output.Citations)
	metadata["charts"] = askAIChartsForAudit(output.Charts)
	metadata["actions"] = askAIActionsForAudit(output.Actions)
	return metadata
}

func askAIHistoryAuditMetadata(input askAIHistoryAuditInput) map[string]any {
	metadata := map[string]any{
		"status":      safeAuditValue(input.Status),
		"http_status": input.HTTPStatus,
	}
	if input.Limit > 0 {
		metadata["limit"] = input.Limit
	}
	if input.Offset > 0 {
		metadata["offset"] = input.Offset
	}
	if input.Total > 0 {
		metadata["total"] = input.Total
	}
	return metadata
}

func askAIFilterTypesForAudit(filters []api.Filter) []string {
	types := make([]string, 0, len(filters))
	for _, filter := range filters {
		if filterType := strings.TrimSpace(filter.Type); filterType != "" {
			types = append(types, filterType)
		}
	}
	sort.Strings(types)
	return types
}

func askAIActionTypesForAudit(actions []hitai.AskAIAction) string {
	types := make([]string, 0, len(actions))
	for _, action := range actions {
		if actionType := strings.TrimSpace(action.Type); actionType != "" {
			types = append(types, actionType)
		}
	}
	sort.Strings(types)
	return strings.Join(types, ",")
}

func askAIChartTypesForAudit(charts []hitai.AskAIChart) string {
	types := make([]string, 0, len(charts))
	for _, chart := range charts {
		if chartType := strings.TrimSpace(chart.Type); chartType != "" {
			types = append(types, chartType)
		}
	}
	sort.Strings(types)
	return strings.Join(types, ",")
}

func askAICitationsForAudit(citations []hitai.AskAICitation) []map[string]any {
	out := make([]map[string]any, 0, len(citations))
	for _, citation := range citations {
		label := strings.TrimSpace(citation.Label)
		out = append(out, map[string]any{
			"tool_call_id": strings.TrimSpace(citation.ToolCallID),
			"label_sha256": hitai.HashString(label),
			"label_chars":  len(label),
		})
	}
	return out
}

func askAIChartsForAudit(charts []hitai.AskAIChart) []map[string]any {
	out := make([]map[string]any, 0, len(charts))
	for _, chart := range charts {
		title := strings.TrimSpace(chart.Title)
		out = append(out, map[string]any{
			"type":          strings.TrimSpace(chart.Type),
			"title_sha256":  hitai.HashString(title),
			"title_chars":   len(title),
			"row_count":     len(chart.Rows),
			"series_count":  len(chart.Series),
			"series_labels": askAIChartSeriesForAudit(chart.Series),
		})
	}
	return out
}

func askAIChartSeriesForAudit(series []hitai.AskAIChartSeries) []map[string]any {
	out := make([]map[string]any, 0, len(series))
	for _, item := range series {
		key := strings.TrimSpace(item.Key)
		label := strings.TrimSpace(item.Label)
		out = append(out, map[string]any{
			"key_sha256":   hitai.HashString(key),
			"key_chars":    len(key),
			"label_sha256": hitai.HashString(label),
			"label_chars":  len(label),
		})
	}
	return out
}

func askAIActionsForAudit(actions []hitai.AskAIAction) []map[string]any {
	out := make([]map[string]any, 0, len(actions))
	for _, action := range actions {
		target := strings.TrimSpace(action.Target)
		label := strings.TrimSpace(action.Label)
		out = append(out, map[string]any{
			"type":          strings.TrimSpace(action.Type),
			"format":        strings.TrimSpace(action.Format),
			"target_sha256": hitai.HashString(target),
			"target_chars":  len(target),
			"label_sha256":  hitai.HashString(label),
			"label_chars":   len(label),
		})
	}
	return out
}

func mustAuditJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func safeAuditValue(value string) string {
	return strings.NewReplacer(" ", "_", "\n", "_", "\r", "_", "\t", "_").Replace(strings.TrimSpace(value))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("Failed to encode Ask AI response", "error", err)
	}
}
