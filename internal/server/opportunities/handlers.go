package opportunities

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	authcore "hitkeep/internal/auth"
	"hitkeep/internal/database"
	opportunitysvc "hitkeep/internal/opportunities"
	"hitkeep/internal/server/shared"
)

type handler struct {
	ctx *shared.Context
}

const maxGenerateRangeDays = 366

func Register(mux *http.ServeMux, ctx *shared.Context) {
	h := &handler{ctx: ctx}
	mux.HandleFunc("GET /api/sites/{id}/opportunities", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleList()))
	mux.HandleFunc("GET /api/sites/{id}/opportunities/digest-preview", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleDigestPreview()))
	mux.HandleFunc("POST /api/sites/{id}/opportunities/generate", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteManageData,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGenerate()))
	mux.HandleFunc("PATCH /api/sites/{id}/opportunities/{opportunityID}", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteManageData,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleUpdateStatus()))
}

func (h *handler) handleList() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, ok := parseUUIDPath(w, r, "id")
		if !ok {
			return
		}
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}
		opps, err := h.ctx.Store.ListOpportunities(r.Context(), siteID)
		if err != nil {
			slog.Error("Failed to list opportunities", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		opps = opportunitysvc.RankOpportunities(opps)
		writeJSON(w, http.StatusOK, api.OpportunityListResponse{Opportunities: opps})
	}
}

func (h *handler) handleDigestPreview() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, ok := parseUUIDPath(w, r, "id")
		if !ok {
			return
		}
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}
		frequency, ok := parseDigestPreviewFrequency(w, r)
		if !ok {
			return
		}
		preview, err := opportunitysvc.SelectDigestPreviewForSite(r.Context(), h.ctx.Store, opportunitysvc.DigestPreviewForSiteInput{
			SiteID:    siteID,
			Frequency: frequency,
		})
		if err != nil {
			slog.Error("Failed to build opportunity digest preview", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, apiOpportunityDigestPreview(preview))
	}
}

func (h *handler) handleGenerate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, ok := parseUUIDPath(w, r, "id")
		if !ok {
			return
		}
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}
		from, to, err := parseRange(r)
		if err != nil {
			http.Error(w, "Invalid date range", http.StatusBadRequest)
			return
		}
		site, err := h.ctx.Store.GetSiteByID(r.Context(), siteID)
		if err != nil {
			slog.Error("Failed to load opportunity site", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if site == nil {
			http.Error(w, "Site not found", http.StatusNotFound)
			return
		}
		teamID, err := h.ctx.Store.GetSiteTenantID(r.Context(), siteID)
		if err != nil {
			slog.Error("Failed to resolve opportunity team", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), siteID)
		if err != nil {
			slog.Error("Failed to resolve opportunity analytics store", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		actor := opportunityActorFromRequest(r)
		audit, err := h.ctx.BuildAuditEntryParams(r.Context(), r, actor.auditEvent(shared.AuditEvent{
			TeamID:      teamID,
			Action:      "opportunities.generated",
			TargetType:  "site",
			TargetID:    siteID.String(),
			TargetLabel: site.Domain,
			Outcome:     "success",
			Details:     "generated opportunities",
		}))
		if err != nil {
			slog.Error("Failed to build opportunity generate audit", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		permission := opportunityPermissionFromRequest(r)
		svc := opportunitysvc.Service{Shared: h.ctx.Store, AI: h.ctx.AI}
		opps, runID, aiStatus, err := svc.Generate(r.Context(), opportunitysvc.GenerateInput{
			TeamID:                teamID,
			Site:                  *site,
			Store:                 analyticsStore,
			Audit:                 &audit,
			From:                  from,
			To:                    to,
			ActorID:               actor.runActorID,
			ActorType:             actor.actorType,
			APIClientAuth:         actor.apiClient,
			EffectiveUserID:       permission.UserID,
			EffectiveInstanceRole: permission.InstanceRole,
			EffectiveSiteRole:     permission.SiteRole,
		})
		if err != nil {
			slog.Error("Failed to generate opportunities", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, api.OpportunityGenerateResponse{Opportunities: opps, AIRunID: runID, AIStatus: aiStatus})
	}
}

func (h *handler) handleUpdateStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, ok := parseUUIDPath(w, r, "id")
		if !ok {
			return
		}
		opportunityID, ok := parseUUIDPath(w, r, "opportunityID")
		if !ok {
			return
		}
		var req api.OpportunityStatusUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		status := strings.TrimSpace(req.Status)
		existing, err := h.ctx.Store.GetOpportunity(r.Context(), siteID, opportunityID)
		if err != nil {
			slog.Error("Failed to load opportunity for status audit", "error", err, "site_id", siteID, "opportunity_id", opportunityID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if existing == nil {
			http.Error(w, "Opportunity not found", http.StatusNotFound)
			return
		}
		actor := opportunityActorFromRequest(r)
		audit, err := h.ctx.BuildAuditEntryParams(r.Context(), r, actor.auditEvent(shared.AuditEvent{
			TeamID:      existing.TeamID,
			Action:      "opportunities.status_updated",
			TargetType:  "opportunity",
			TargetID:    existing.ID.String(),
			TargetLabel: existing.TypeKey,
			Outcome:     "success",
			Details:     "status=" + status,
		}))
		if err != nil {
			slog.Error("Failed to build opportunity status audit", "error", err, "site_id", siteID, "opportunity_id", opportunityID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		opportunity, err := h.ctx.Store.UpdateOpportunityStatusWithAudit(r.Context(), siteID, opportunityID, status, audit)
		if err != nil {
			if strings.Contains(err.Error(), "unsupported opportunity status") {
				http.Error(w, "Unsupported opportunity status", http.StatusBadRequest)
				return
			}
			slog.Error("Failed to update opportunity status", "error", err, "site_id", siteID, "opportunity_id", opportunityID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if opportunity == nil {
			http.Error(w, "Opportunity not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, opportunity)
	}
}

func parseDigestPreviewFrequency(w http.ResponseWriter, r *http.Request) (api.ReportFrequency, bool) {
	frequency := api.ReportFrequency(strings.TrimSpace(r.URL.Query().Get("frequency")))
	if frequency == "" {
		frequency = api.ReportFrequencyWeekly
	}
	if frequency != api.ReportFrequencyDaily && frequency != api.ReportFrequencyWeekly {
		http.Error(w, "Unsupported opportunity digest frequency", http.StatusBadRequest)
		return "", false
	}
	return frequency, true
}

func apiOpportunityDigestPreview(preview opportunitysvc.DigestPreview) api.OpportunityDigestPreviewResponse {
	items := make([]api.OpportunityDigestPreviewItem, 0, len(preview.Items))
	for _, item := range preview.Items {
		items = append(items, api.OpportunityDigestPreviewItem{
			ID:               item.ID,
			SiteID:           item.SiteID,
			Kind:             item.Kind,
			TypeKey:          item.TypeKey,
			Category:         item.Category,
			TitleKey:         item.TitleKey,
			ActionKey:        item.ActionKey,
			DigestKey:        item.DigestKey,
			CopyParams:       item.CopyParams,
			ImpactValue:      item.ImpactValue,
			ImpactLabelKey:   item.ImpactLabelKey,
			Confidence:       item.Confidence,
			Score:            item.Score,
			ScoreBreakdown:   item.ScoreBreakdown,
			Status:           item.Status,
			RouteLabelKey:    item.RouteLabelKey,
			RouteParams:      item.RouteParams,
			RouteIcon:        item.RouteIcon,
			Evidence:         item.Evidence,
			CitedEvidenceIDs: item.CitedEvidenceIDs,
		})
	}
	return api.OpportunityDigestPreviewResponse{
		Frequency:  preview.Frequency,
		ShouldSend: preview.ShouldSend,
		Reason:     preview.Reason,
		Items:      items,
	}
}

type opportunityActor struct {
	runActorID uuid.UUID
	actorType  string
	apiClient  *database.APIClientAuth
}

func opportunityActorFromRequest(r *http.Request) opportunityActor {
	if apiClient, _ := r.Context().Value(shared.APIClientAuthKey).(*database.APIClientAuth); apiClient != nil {
		return opportunityActor{
			runActorID: apiClient.ClientID,
			actorType:  "api_client",
			apiClient:  apiClient,
		}
	}
	return opportunityActor{
		runActorID: shared.GetUserIDFromContext(r),
		actorType:  "user",
	}
}

func opportunityPermissionFromRequest(r *http.Request) shared.PermissionContext {
	permission, _ := r.Context().Value(shared.PermissionKey).(shared.PermissionContext)
	return permission
}

func (a opportunityActor) auditEvent(event shared.AuditEvent) shared.AuditEvent {
	if a.apiClient == nil {
		return event
	}
	event.ActorID = a.apiClient.UserID
	event.ActorRole = "api_client"
	event.Details = appendAuditDetail(event.Details, "actor_type=api_client")
	event.Details = appendAuditDetail(event.Details, "api_client_id="+a.apiClient.ClientID.String())
	event.MetadataJSON = apiClientAuditMetadata(a.apiClient)
	return event
}

func appendAuditDetail(details, addition string) string {
	details = strings.TrimSpace(details)
	addition = strings.TrimSpace(addition)
	if details == "" {
		return addition
	}
	if addition == "" || strings.Contains(details, addition) {
		return details
	}
	return details + "; " + addition
}

func apiClientAuditMetadata(apiClient *database.APIClientAuth) string {
	if apiClient == nil {
		return ""
	}
	ownerType := "personal"
	if apiClient.TenantID != uuid.Nil {
		ownerType = "team"
	}
	raw, err := json.Marshal(map[string]string{
		"actor_type":    "api_client",
		"api_client_id": apiClient.ClientID.String(),
		"owner_type":    ownerType,
		"user_id":       apiClient.UserID.String(),
		"team_id":       apiClient.TenantID.String(),
	})
	if err != nil {
		return ""
	}
	return string(raw)
}

func parseUUIDPath(w http.ResponseWriter, r *http.Request, key string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(key))
	if err != nil {
		http.Error(w, "Invalid "+key, http.StatusBadRequest)
		return uuid.Nil, false
	}
	return id, true
}

func parseRange(r *http.Request) (time.Time, time.Time, error) {
	now := time.Now().UTC()
	to, err := parseOptionalTime(r.URL.Query().Get("to"), now)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	from, err := parseOptionalTime(r.URL.Query().Get("from"), to.AddDate(0, 0, -30))
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if !to.After(from) {
		return time.Time{}, time.Time{}, fmt.Errorf("to must be after from")
	}
	if to.Sub(from) > maxGenerateRangeDays*24*time.Hour {
		return time.Time{}, time.Time{}, fmt.Errorf("range exceeds %d days", maxGenerateRangeDays)
	}
	return from.UTC(), to.UTC(), nil
}

func parseOptionalTime(raw string, fallback time.Time) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}
	if ts, err := time.Parse(time.RFC3339, raw); err == nil {
		return ts, nil
	}
	if ts, err := time.Parse("2006-01-02", raw); err == nil {
		return ts, nil
	}
	return time.Time{}, fmt.Errorf("invalid time %q", raw)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("Failed to encode opportunities response", "error", err)
	}
}
