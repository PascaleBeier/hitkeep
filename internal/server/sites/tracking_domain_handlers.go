package sites

import (
	"net/http"
	"strings"

	"github.com/google/uuid"

	"hitkeep/appurl"
	"hitkeep/internal/api"
	"hitkeep/internal/database"
	"hitkeep/internal/server/shared"
	json "hitkeep/jsonapi"
)

func (h *handler) handleGetSiteTrackingDomainOptions() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		options, ok := h.siteTrackingDomainOptions(w, r)
		if !ok {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.MarshalWrite(w, options); err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to encode site tracking domain options", "error", err, "site_id", options.SiteID)
		}
	}
}

func (h *handler) siteTrackingDomainOptions(w http.ResponseWriter, r *http.Request) (api.SiteTrackingDomainOptions, bool) {
	siteID, ok := parseSiteIDPath(w, r)
	if !ok {
		return api.SiteTrackingDomainOptions{}, false
	}
	teamID, ok := h.siteTeamID(w, r, siteID)
	if !ok {
		return api.SiteTrackingDomainOptions{}, false
	}
	options, ok := h.buildSiteTrackingDomainOptions(r, siteID, teamID)
	if !ok {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return api.SiteTrackingDomainOptions{}, false
	}
	return options, true
}

func (h *handler) siteTeamID(w http.ResponseWriter, r *http.Request, siteID uuid.UUID) (uuid.UUID, bool) {
	if h.ctx.Store == nil {
		http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
		return uuid.Nil, false
	}
	teamID, err := h.ctx.Store.GetSiteTenantID(r.Context(), siteID)
	if err != nil {
		shared.LoggerFromContext(r.Context()).Error("Failed to resolve site team for tracking domain options", "error", err, "site_id", siteID)
		http.Error(w, "Site not found", http.StatusNotFound)
		return uuid.Nil, false
	}
	return teamID, true
}

func (h *handler) buildSiteTrackingDomainOptions(r *http.Request, siteID, teamID uuid.UUID) (api.SiteTrackingDomainOptions, bool) {
	domains, err := h.ctx.Store.ListCustomTrackingDomains(r.Context(), teamID)
	if err != nil {
		shared.LoggerFromContext(r.Context()).Error("Failed to list site tracking domain options", "error", err, "site_id", siteID, "team_id", teamID)
		return api.SiteTrackingDomainOptions{}, false
	}
	target := ""
	defaultURL := "/hk.js"
	if h.ctx.Config != nil {
		target = h.ctx.Config.CustomTrackingDNSTargetValue()
		defaultURL = appurl.Path(h.ctx.Config.PublicURL, "/hk.js")
	}
	for i := range domains {
		domains[i].DNSTarget = target
		domains[i].Active = database.CustomTrackingDomainIsActive(domains[i])
	}
	return api.SiteTrackingDomainOptions{
		SiteID:     siteID,
		TeamID:     teamID,
		DefaultURL: defaultURL,
		Domains:    domains,
	}, true
}

func parseSiteIDPath(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	siteID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		http.Error(w, "Invalid site ID", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return siteID, true
}
