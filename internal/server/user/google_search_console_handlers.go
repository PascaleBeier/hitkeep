package user

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	authcore "hitkeep/internal/auth"
	"hitkeep/internal/database"
	"hitkeep/internal/searchconsole"
	"hitkeep/internal/server/shared"
	"hitkeep/internal/worker"
)

const (
	googleSearchConsoleStatusConnected          = "connected"
	googleSearchConsoleStatusDisconnected       = "disconnected"
	googleSearchConsoleStatusCredentialsMissing = "credentials_missing"
)

func (h *handler) handleGetGoogleSearchConsoleStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		teamID, role, ok := h.resolveGoogleSearchConsoleTeamAccess(w, r, userID)
		if !ok {
			return
		}

		status := h.buildGoogleSearchConsoleStatus(r, teamID, role)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(status); err != nil {
			slog.Error("Failed to encode Google Search Console status", "error", err, "team_id", teamID)
		}
	}
}

func (h *handler) handleConnectGoogleSearchConsole() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		teamID, ok := h.requireGoogleSearchConsoleManager(w, r, userID)
		if !ok {
			return
		}
		if !h.googleSearchConsoleCredentialsConfigured() {
			h.appendGoogleSearchConsoleAudit(r, teamID, userID, "google_search_console.connect_failed", "failure", "credentials_missing")
			writeTeamActionError(w, http.StatusPreconditionFailed, "credentials_missing", "Google Search Console credentials are not configured")
			return
		}

		returnPath, ok := decodeGoogleSearchConsoleConnectReturnPath(w, r)
		if !ok {
			return
		}
		state := h.ctx.AuthState.CreateGoogleSearchConsoleOAuthState(userID, teamID, returnPath, time.Now().UTC().Add(10*time.Minute))

		client := h.googleSearchConsoleClient()
		authURL, err := client.AuthCodeURL(state, h.googleSearchConsoleRedirectURL())
		if err != nil {
			slog.Error("Failed to build Google Search Console OAuth URL", "error", err, "team_id", teamID)
			http.Error(w, "Could not start Google Search Console connection", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(api.GoogleSearchConsoleConnectResponse{AuthURL: authURL}); err != nil {
			slog.Error("Failed to encode Google Search Console connect response", "error", err, "team_id", teamID)
		}
	}
}

func (h *handler) handleListGoogleSearchConsoleProperties() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		teamID, ok := h.requireGoogleSearchConsoleManager(w, r, userID)
		if !ok {
			return
		}
		conn, err := h.ctx.Store.GetGoogleSearchConsoleConnection(r.Context(), teamID)
		if err != nil {
			slog.Error("Failed to load Google Search Console connection for properties", "error", err, "team_id", teamID)
			http.Error(w, "Could not load Google Search Console connection", http.StatusInternalServerError)
			return
		}
		if conn == nil || !conn.Connected {
			writeTeamActionError(w, http.StatusPreconditionFailed, "not_connected", "Google Search Console is not connected")
			return
		}

		properties, err := h.googleSearchConsoleClient().ListProperties(r.Context(), googleSearchConsoleConnectionToken(conn))
		if err != nil {
			category := searchconsole.ClassifyError(err)
			slog.Warn("Failed to list Google Search Console properties", "category", category, "team_id", teamID)
			h.appendGoogleSearchConsoleAudit(r, teamID, userID, "google_search_console.properties_refresh_failed", "failure", string(category))
			writeTeamActionError(w, http.StatusBadGateway, string(category), "Could not list Google Search Console properties")
			return
		}

		resp, err := h.cacheGoogleSearchConsoleProperties(r, teamID, userID, properties)
		if err != nil {
			slog.Error("Failed to cache Google Search Console properties", "error", err, "team_id", teamID)
			http.Error(w, "Could not cache Google Search Console properties", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("Failed to encode Google Search Console properties", "error", err, "team_id", teamID)
		}
	}
}

func (h *handler) cacheGoogleSearchConsoleProperties(r *http.Request, teamID, userID uuid.UUID, properties []searchconsole.Property) (api.GoogleSearchConsolePropertiesResponse, error) {
	resp := api.GoogleSearchConsolePropertiesResponse{Properties: make([]api.GoogleSearchConsoleProperty, 0, len(properties))}
	inputs := make([]database.GoogleSearchConsolePropertyInput, 0, len(properties))
	seenAt := time.Now().UTC()
	for _, property := range properties {
		propertyURI := strings.TrimSpace(property.URI)
		if propertyURI == "" {
			continue
		}
		permissionLevel := strings.TrimSpace(property.PermissionLevel)
		inputs = append(inputs, database.GoogleSearchConsolePropertyInput{
			TeamID:          teamID,
			URI:             propertyURI,
			PermissionLevel: permissionLevel,
			SeenAt:          seenAt,
		})
		resp.Properties = append(resp.Properties, api.GoogleSearchConsoleProperty{
			URI:             propertyURI,
			PermissionLevel: permissionLevel,
		})
	}
	audit, err := h.googleSearchConsoleAuditParams(r, teamID, userID, "google_search_console.properties_refreshed", "success", "properties_refreshed")
	if err != nil {
		return api.GoogleSearchConsolePropertiesResponse{}, err
	}
	if err := h.ctx.Store.UpsertGoogleSearchConsolePropertiesWithAudit(r.Context(), inputs, audit); err != nil {
		return api.GoogleSearchConsolePropertiesResponse{}, err
	}
	return resp, nil
}

func (h *handler) handleGetGoogleSearchConsoleSiteMapping() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		siteID, teamID, ok := h.resolveGoogleSearchConsoleSiteScope(w, r, userID)
		if !ok {
			return
		}
		role, _ := h.ctx.Store.GetSiteRole(r.Context(), userID, siteID)
		resp, err := h.googleSearchConsoleSiteMappingResponse(r.Context(), siteID, teamID, googleSearchConsoleRoleCanManageSite(role))
		if err != nil {
			slog.Error("Failed to load Google Search Console site mapping", "error", err, "site_id", siteID)
			http.Error(w, "Could not load Google Search Console site mapping", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("Failed to encode Google Search Console site mapping", "error", err, "site_id", siteID)
		}
	}
}

func (h *handler) handleMapGoogleSearchConsoleSiteProperty() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		siteID, teamID, ok := h.resolveGoogleSearchConsoleSiteScope(w, r, userID)
		if !ok {
			return
		}
		if !h.requireGoogleSearchConsoleConnectedTeam(w, r, teamID) {
			return
		}

		var req api.GoogleSearchConsoleMapPropertyRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
			return
		}
		propertyURI := strings.TrimSpace(req.PropertyURI)
		if propertyURI == "" {
			http.Error(w, "property_uri is required", http.StatusBadRequest)
			return
		}
		property, err := h.ctx.Store.GetGoogleSearchConsoleProperty(r.Context(), teamID, propertyURI)
		if err != nil {
			slog.Error("Failed to validate Google Search Console property", "error", err, "team_id", teamID, "site_id", siteID)
			http.Error(w, "Could not validate Google Search Console property", http.StatusInternalServerError)
			return
		}
		if property == nil {
			http.Error(w, "Google Search Console property is not available for this team", http.StatusBadRequest)
			return
		}
		if !h.requireGoogleSearchConsolePropertyMatchesSite(w, r, teamID, siteID, propertyURI) {
			return
		}

		oldMapping, err := h.ctx.Store.GetGoogleSearchConsoleSiteMappingForTeam(r.Context(), siteID, teamID)
		if err != nil {
			slog.Error("Failed to load previous Google Search Console site mapping", "error", err, "site_id", siteID)
			http.Error(w, "Could not map Google Search Console property", http.StatusInternalServerError)
			return
		}
		oldPropertyURI := ""
		if oldMapping != nil {
			oldPropertyURI = oldMapping.PropertyURI
		}
		audit, err := h.googleSearchConsoleSiteAuditParams(r, teamID, siteID, userID, "google_search_console.property_mapped", "success", googleSearchConsoleMappingAuditDetails(oldPropertyURI, propertyURI))
		if err != nil {
			slog.Error("Failed to build Google Search Console map audit", "error", err, "team_id", teamID, "site_id", siteID)
			http.Error(w, "Could not map Google Search Console property", http.StatusInternalServerError)
			return
		}
		if err := h.ctx.Store.UpsertGoogleSearchConsoleSiteMappingWithAudit(r.Context(), database.GoogleSearchConsoleSiteMappingInput{
			SiteID:      siteID,
			TeamID:      teamID,
			PropertyURI: propertyURI,
			MappedBy:    userID,
			MappedAt:    time.Now().UTC(),
		}, audit); err != nil {
			slog.Error("Failed to map Google Search Console property", "error", err, "team_id", teamID, "site_id", siteID)
			http.Error(w, "Could not map Google Search Console property", http.StatusInternalServerError)
			return
		}

		resp, err := h.googleSearchConsoleSiteMappingResponse(r.Context(), siteID, teamID, true)
		if err != nil {
			slog.Error("Failed to load mapped Google Search Console site mapping", "error", err, "site_id", siteID)
			http.Error(w, "Could not load Google Search Console site mapping", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("Failed to encode Google Search Console site mapping", "error", err, "site_id", siteID)
		}
	}
}

func (h *handler) handleUnmapGoogleSearchConsoleSiteProperty() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		siteID, teamID, ok := h.resolveGoogleSearchConsoleSiteScope(w, r, userID)
		if !ok {
			return
		}
		if !h.requireGoogleSearchConsoleConnectedTeam(w, r, teamID) {
			return
		}
		oldMapping, err := h.ctx.Store.GetGoogleSearchConsoleSiteMappingForTeam(r.Context(), siteID, teamID)
		if err != nil {
			slog.Error("Failed to load Google Search Console site mapping before unmap", "error", err, "site_id", siteID)
			http.Error(w, "Could not unmap Google Search Console property", http.StatusInternalServerError)
			return
		}
		oldPropertyURI := ""
		if oldMapping != nil {
			oldPropertyURI = oldMapping.PropertyURI
		}
		audit, err := h.googleSearchConsoleSiteAuditParams(r, teamID, siteID, userID, "google_search_console.property_unmapped", "success", googleSearchConsoleMappingAuditDetails(oldPropertyURI, ""))
		if err != nil {
			slog.Error("Failed to build Google Search Console unmap audit", "error", err, "team_id", teamID, "site_id", siteID)
			http.Error(w, "Could not unmap Google Search Console property", http.StatusInternalServerError)
			return
		}
		if err := h.ctx.Store.DeleteGoogleSearchConsoleSiteMappingWithAudit(r.Context(), siteID, audit); err != nil {
			slog.Error("Failed to unmap Google Search Console property", "error", err, "site_id", siteID)
			http.Error(w, "Could not unmap Google Search Console property", http.StatusInternalServerError)
			return
		}

		resp, err := h.googleSearchConsoleSiteMappingResponse(r.Context(), siteID, teamID, true)
		if err != nil {
			slog.Error("Failed to load unmapped Google Search Console site mapping", "error", err, "site_id", siteID)
			http.Error(w, "Could not load Google Search Console site mapping", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("Failed to encode Google Search Console site mapping", "error", err, "site_id", siteID)
		}
	}
}

func (h *handler) handleRequestGoogleSearchConsoleSiteSync() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		siteID, teamID, ok := h.resolveGoogleSearchConsoleSiteScope(w, r, userID)
		if !ok {
			return
		}
		if !h.requireGoogleSearchConsoleConnectedTeam(w, r, teamID) {
			return
		}
		mapping, err := h.ctx.Store.GetGoogleSearchConsoleSiteMappingForTeam(r.Context(), siteID, teamID)
		if err != nil {
			slog.Error("Failed to load Google Search Console mapping for sync", "error", err, "site_id", siteID)
			http.Error(w, "Could not request Google Search Console sync", http.StatusInternalServerError)
			return
		}
		if mapping == nil {
			writeTeamActionError(w, http.StatusPreconditionFailed, "not_mapped", "Google Search Console property is not mapped")
			return
		}

		now := time.Now().UTC()
		previousState, err := h.ctx.Store.GetGoogleSearchConsoleSyncState(r.Context(), siteID)
		if err != nil {
			slog.Error("Failed to load previous Google Search Console sync state", "error", err, "site_id", siteID)
			http.Error(w, "Could not request Google Search Console sync", http.StatusInternalServerError)
			return
		}
		input := database.GoogleSearchConsoleSyncStateInput{
			SiteID:        siteID,
			TeamID:        teamID,
			State:         "pending",
			LastAttemptAt: &now,
			Manual:        true,
		}
		if previousState != nil {
			input.ImportedStartDate = previousState.ImportedStartDate
			input.ImportedEndDate = previousState.ImportedEndDate
			input.LastSuccessAt = previousState.LastSuccessAt
		}
		audit, err := h.googleSearchConsoleSiteAuditParams(r, teamID, siteID, userID, "google_search_console.sync_requested", "success", "sync_requested")
		if err != nil {
			slog.Error("Failed to build Google Search Console sync audit", "error", err, "team_id", teamID, "site_id", siteID)
			http.Error(w, "Could not request Google Search Console sync", http.StatusInternalServerError)
			return
		}
		if err := h.ctx.Store.UpsertGoogleSearchConsoleSyncStateWithAudit(r.Context(), input, audit); err != nil {
			slog.Error("Failed to mark Google Search Console sync pending", "error", err, "site_id", siteID)
			http.Error(w, "Could not request Google Search Console sync", http.StatusInternalServerError)
			return
		}
		if err := h.runGoogleSearchConsoleManualSync(r, siteID, teamID); err != nil {
			slog.Warn("Immediate Google Search Console sync failed",
				"site_id", siteID,
				"team_id", teamID,
				"category", searchconsole.ClassifyError(err),
			)
		}

		resp, err := h.googleSearchConsoleSiteMappingResponse(r.Context(), siteID, teamID, true)
		if err != nil {
			slog.Error("Failed to load Google Search Console sync response", "error", err, "site_id", siteID)
			http.Error(w, "Could not load Google Search Console site mapping", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("Failed to encode Google Search Console sync response", "error", err, "site_id", siteID)
		}
	}
}

func (h *handler) requireGoogleSearchConsoleManager(w http.ResponseWriter, r *http.Request, userID uuid.UUID) (uuid.UUID, bool) {
	teamID, role, ok := h.resolveGoogleSearchConsoleTeamAccess(w, r, userID)
	if !ok {
		return uuid.Nil, false
	}
	if !canManageTeam(role) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return uuid.Nil, false
	}
	return teamID, true
}

func (h *handler) runGoogleSearchConsoleManualSync(r *http.Request, siteID, teamID uuid.UUID) error {
	if h == nil || h.ctx == nil || h.ctx.TenantStores == nil {
		return fmt.Errorf("search console sync worker is not configured")
	}
	syncWorker := worker.NewSearchConsoleSyncWorker(h.ctx.TenantStores, h.googleSearchConsoleClient())
	return syncWorker.ImportSite(r.Context(), siteID)
}

func decodeGoogleSearchConsoleConnectReturnPath(w http.ResponseWriter, r *http.Request) (string, bool) {
	type request struct {
		ReturnPath string `json:"return_path"`
	}
	var req request
	if r.Body == nil {
		return sanitizeGoogleSearchConsoleReturnPath(req.ReturnPath), true
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return "", false
	}
	return sanitizeGoogleSearchConsoleReturnPath(req.ReturnPath), true
}

func (h *handler) handleGoogleSearchConsoleCallback() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		state, code, ok := h.validateGoogleSearchConsoleCallback(w, r, userID)
		if !ok {
			return
		}
		role, err := h.ctx.Store.GetTenantRole(r.Context(), state.TeamID, userID)
		if err != nil || !canManageTeam(role) {
			h.appendGoogleSearchConsoleAudit(r, state.TeamID, userID, "google_search_console.connect_failed", "failure", "permission_lost")
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		if !h.completeGoogleSearchConsoleConnection(w, r, state.TeamID, userID, code) {
			return
		}

		http.Redirect(w, r, sanitizeGoogleSearchConsoleReturnPath(state.ReturnPath), http.StatusFound)
	}
}

func (h *handler) validateGoogleSearchConsoleCallback(w http.ResponseWriter, r *http.Request, userID uuid.UUID) (shared.GoogleSearchConsoleOAuthState, string, bool) {
	rawState := r.URL.Query().Get("state")
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	state, ok := h.ctx.AuthState.ConsumeGoogleSearchConsoleOAuthState(rawState)
	if !ok || state.UserID != userID || state.TeamID == uuid.Nil {
		http.Error(w, "Invalid OAuth state", http.StatusBadRequest)
		return shared.GoogleSearchConsoleOAuthState{}, "", false
	}
	if code == "" {
		h.appendGoogleSearchConsoleAudit(r, state.TeamID, userID, "google_search_console.connect_failed", "failure", "missing_code")
		http.Error(w, "Missing OAuth code", http.StatusBadRequest)
		return shared.GoogleSearchConsoleOAuthState{}, "", false
	}
	return state, code, true
}

func (h *handler) completeGoogleSearchConsoleConnection(w http.ResponseWriter, r *http.Request, teamID, userID uuid.UUID, code string) bool {
	token, err := h.googleSearchConsoleClient().ExchangeCode(r.Context(), code, h.googleSearchConsoleRedirectURL())
	if err != nil {
		h.appendGoogleSearchConsoleAudit(r, teamID, userID, "google_search_console.connect_failed", "failure", "exchange_failed")
		slog.Warn("Google Search Console OAuth exchange failed", "team_id", teamID)
		http.Error(w, "Could not connect Google Search Console", http.StatusBadGateway)
		return false
	}
	if strings.TrimSpace(token.RefreshToken) == "" {
		h.appendGoogleSearchConsoleAudit(r, teamID, userID, "google_search_console.connect_failed", "failure", "refresh_token_missing")
		slog.Warn("Google Search Console OAuth exchange did not return a refresh token", "team_id", teamID)
		http.Error(w, "Could not connect Google Search Console", http.StatusBadGateway)
		return false
	}

	audit, err := h.googleSearchConsoleAuditParams(r, teamID, userID, "google_search_console.connected", "success", "connected")
	if err != nil {
		slog.Error("Failed to build Google Search Console connection audit", "error", err, "team_id", teamID)
		http.Error(w, "Could not store Google Search Console connection", http.StatusInternalServerError)
		return false
	}
	if err := h.ctx.Store.UpsertGoogleSearchConsoleConnectionWithAudit(r.Context(), databaseGoogleSearchConsoleConnectionInput(teamID, userID, token, time.Now().UTC()), audit); err != nil {
		slog.Error("Failed to store Google Search Console connection", "error", err, "team_id", teamID)
		http.Error(w, "Could not store Google Search Console connection", http.StatusInternalServerError)
		return false
	}
	return true
}

func (h *handler) handleDisconnectGoogleSearchConsole() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		teamID, ok := h.requireGoogleSearchConsoleManager(w, r, userID)
		if !ok {
			return
		}

		audit, err := h.googleSearchConsoleAuditParams(r, teamID, userID, "google_search_console.disconnected", "success", "disconnected")
		if err != nil {
			slog.Error("Failed to build Google Search Console disconnect audit", "error", err, "team_id", teamID)
			http.Error(w, "Could not disconnect Google Search Console", http.StatusInternalServerError)
			return
		}
		if err := h.ctx.Store.DisconnectGoogleSearchConsoleConnectionWithAudit(r.Context(), teamID, time.Now().UTC(), audit); err != nil {
			slog.Error("Failed to disconnect Google Search Console", "error", err, "team_id", teamID)
			http.Error(w, "Could not disconnect Google Search Console", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
			slog.Error("Failed to encode Google Search Console disconnect response", "error", err, "team_id", teamID)
		}
	}
}

func (h *handler) resolveGoogleSearchConsoleTeamAccess(w http.ResponseWriter, r *http.Request, userID uuid.UUID) (uuid.UUID, string, bool) {
	teamID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid team_id", http.StatusBadRequest)
		return uuid.Nil, "", false
	}

	role, err := h.ctx.Store.GetTenantRole(r.Context(), teamID, userID)
	if err != nil {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return uuid.Nil, "", false
	}

	return teamID, role, true
}

func (h *handler) resolveGoogleSearchConsoleSiteScope(w http.ResponseWriter, r *http.Request, userID uuid.UUID) (uuid.UUID, uuid.UUID, bool) {
	if h.ctx == nil || h.ctx.Store == nil {
		http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
		return uuid.Nil, uuid.Nil, false
	}
	siteID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		http.Error(w, "Invalid site_id", http.StatusBadRequest)
		return uuid.Nil, uuid.Nil, false
	}
	teamID, err := h.ctx.Store.GetSiteTenantID(r.Context(), siteID)
	if err != nil {
		http.Error(w, "Invalid site_id", http.StatusBadRequest)
		return uuid.Nil, uuid.Nil, false
	}
	activeTeamID, err := h.ctx.Store.GetActiveTenantID(r.Context(), userID)
	if err != nil || activeTeamID != teamID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return uuid.Nil, uuid.Nil, false
	}
	return siteID, teamID, true
}

func (h *handler) requireGoogleSearchConsoleConnectedTeam(w http.ResponseWriter, r *http.Request, teamID uuid.UUID) bool {
	conn, err := h.ctx.Store.GetGoogleSearchConsoleConnection(r.Context(), teamID)
	if err != nil {
		slog.Error("Failed to load Google Search Console connection", "error", err, "team_id", teamID)
		http.Error(w, "Could not load Google Search Console connection", http.StatusInternalServerError)
		return false
	}
	if conn == nil || !conn.Connected {
		writeTeamActionError(w, http.StatusPreconditionFailed, "not_connected", "Google Search Console is not connected")
		return false
	}
	return true
}

func (h *handler) requireGoogleSearchConsolePropertyMatchesSite(w http.ResponseWriter, r *http.Request, teamID, siteID uuid.UUID, propertyURI string) bool {
	site, err := h.ctx.Store.GetSiteByID(r.Context(), siteID)
	if err != nil {
		slog.Error("Failed to load site for Google Search Console property validation", "error", err, "team_id", teamID, "site_id", siteID)
		http.Error(w, "Could not validate Google Search Console property", http.StatusInternalServerError)
		return false
	}
	if site == nil || !googleSearchConsolePropertyMatchesSite(site.Domain, propertyURI) {
		http.Error(w, "Google Search Console property does not match this site", http.StatusBadRequest)
		return false
	}
	return true
}

func (h *handler) googleSearchConsoleSiteMappingResponse(ctx context.Context, siteID, teamID uuid.UUID, canManage bool) (api.GoogleSearchConsoleSiteMappingResponse, error) {
	resp := api.GoogleSearchConsoleSiteMappingResponse{
		SiteID:    siteID,
		TeamID:    teamID,
		CanManage: canManage,
	}
	mapping, err := h.ctx.Store.GetGoogleSearchConsoleSiteMappingForTeam(ctx, siteID, teamID)
	if err != nil {
		return resp, err
	}
	if mapping == nil {
		return resp, nil
	}
	resp.Mapped = true
	resp.PropertyURI = mapping.PropertyURI
	mappedAt := mapping.MappedAt
	resp.MappedAt = &mappedAt
	property, err := h.ctx.Store.GetGoogleSearchConsoleProperty(ctx, teamID, mapping.PropertyURI)
	if err != nil {
		return resp, err
	}
	if property != nil {
		resp.PropertyPermissionLevel = property.PermissionLevel
	}
	syncState, err := h.ctx.Store.GetGoogleSearchConsoleSyncState(ctx, siteID)
	if err != nil {
		return resp, err
	}
	if syncState != nil && syncState.TeamID == teamID {
		resp.SyncStatus = googleSearchConsoleSyncStatusResponse(syncState)
	}
	return resp, nil
}

func (h *handler) buildGoogleSearchConsoleStatus(r *http.Request, teamID uuid.UUID, role string) api.GoogleSearchConsoleStatus {
	credentialStatus := "configured"
	configured := h.googleSearchConsoleCredentialsConfigured()
	status := googleSearchConsoleStatusDisconnected
	if !configured {
		credentialStatus = "missing"
		status = googleSearchConsoleStatusCredentialsMissing
	}

	if h.ctx != nil && h.ctx.Store != nil {
		conn, err := h.ctx.Store.GetGoogleSearchConsoleConnection(r.Context(), teamID)
		if err == nil && conn != nil && conn.Connected {
			connectedAt := conn.ConnectedAt
			return api.GoogleSearchConsoleStatus{
				Status:                 googleSearchConsoleStatusConnected,
				Configured:             configured,
				Connected:              true,
				CredentialStatus:       credentialStatus,
				ConnectedAccountLabel:  conn.GoogleAccountEmail,
				LastConnectedAt:        &connectedAt,
				NeedsAdminAction:       false,
				CanManage:              canManageTeam(role),
				ManagedCredentialsMode: h.googleSearchConsoleCredentialsMode(),
			}
		}
	}

	statusResp := api.GoogleSearchConsoleStatus{
		Status:                 status,
		Configured:             configured,
		Connected:              false,
		CredentialStatus:       credentialStatus,
		NeedsAdminAction:       !configured,
		CanManage:              canManageTeam(role),
		ManagedCredentialsMode: h.googleSearchConsoleCredentialsMode(),
	}
	if h.ctx != nil && h.ctx.Store != nil {
		conn, err := h.ctx.Store.GetGoogleSearchConsoleConnection(r.Context(), teamID)
		if err == nil && conn != nil && conn.DisconnectedAt != nil {
			statusResp.LastDisconnectedAt = conn.DisconnectedAt
		}
	}
	return statusResp
}

func (h *handler) googleSearchConsoleCredentialsConfigured() bool {
	if h == nil || h.ctx == nil || h.ctx.Config == nil {
		return false
	}
	return strings.TrimSpace(h.ctx.Config.GoogleSearchConsoleClientID) != "" &&
		strings.TrimSpace(h.ctx.Config.GoogleSearchConsoleClientSecret) != ""
}

func (h *handler) googleSearchConsoleCredentialsMode() string {
	if h != nil && h.ctx != nil && h.ctx.Config != nil && h.ctx.Config.CloudHosted {
		return "managed"
	}
	return "self_hosted"
}

func (h *handler) googleSearchConsoleRedirectURL() string {
	if h == nil || h.ctx == nil || h.ctx.Config == nil {
		return ""
	}
	if configured := strings.TrimSpace(h.ctx.Config.GoogleSearchConsoleRedirectURL); configured != "" {
		return configured
	}
	return strings.TrimRight(strings.TrimSpace(h.ctx.Config.PublicURL), "/") + "/api/integrations/google-search-console/oauth/callback"
}

func (h *handler) googleSearchConsoleClient() searchconsole.Client {
	if h != nil && h.ctx != nil && h.ctx.SearchConsole != nil {
		return h.ctx.SearchConsole
	}
	return newSearchConsoleClientFromHandler(h)
}

func newSearchConsoleClientFromHandler(h *handler) searchconsole.Client {
	if h == nil || h.ctx == nil || h.ctx.Config == nil {
		return searchconsole.NewGoogleClient(searchconsole.OAuthConfig{})
	}
	return searchconsole.NewGoogleClient(searchconsole.OAuthConfig{
		ClientID:     h.ctx.Config.GoogleSearchConsoleClientID,
		ClientSecret: h.ctx.Config.GoogleSearchConsoleClientSecret,
	})
}

func (h *handler) appendGoogleSearchConsoleAudit(r *http.Request, teamID, actorID uuid.UUID, action, outcome, detail string) {
	params, err := h.googleSearchConsoleAuditParams(r, teamID, actorID, action, outcome, detail)
	if err != nil {
		slog.Error("Failed to build Google Search Console audit", "error", err, "action", action, "team_id", teamID)
		return
	}
	if err := h.ctx.Store.AppendAuditEntry(r.Context(), params); err != nil {
		slog.Error("Failed to append Google Search Console audit", "error", err, "action", action, "team_id", teamID)
	}
}

func (h *handler) googleSearchConsoleAuditParams(r *http.Request, teamID, actorID uuid.UUID, action, outcome, detail string) (database.AuditEntryParams, error) {
	targetLabel := ""
	if h != nil && h.ctx != nil && h.ctx.Store != nil {
		if team, err := h.ctx.Store.GetTenant(r.Context(), teamID); err == nil && team != nil {
			targetLabel = team.Name
		}
	}
	return h.ctx.BuildAuditEntryParams(r.Context(), r, shared.AuditEvent{
		ActorID:     actorID,
		TeamID:      teamID,
		Action:      action,
		TargetType:  googleSearchConsoleConnectionTargetType(),
		TargetID:    googleSearchConsoleConnectionTargetID(teamID),
		TargetLabel: targetLabel,
		Outcome:     googleSearchConsoleAuditOutcome(outcome),
		Details:     googleSearchConsoleAuditDetails(detail),
	})
}

func (h *handler) googleSearchConsoleSiteAuditParams(r *http.Request, teamID, siteID, actorID uuid.UUID, action, outcome, details string) (database.AuditEntryParams, error) {
	targetLabel := ""
	if h != nil && h.ctx != nil && h.ctx.Store != nil {
		if site, err := h.ctx.Store.GetSiteByID(r.Context(), siteID); err == nil && site != nil {
			targetLabel = site.Domain
		}
	}
	return h.ctx.BuildAuditEntryParams(r.Context(), r, shared.AuditEvent{
		ActorID:     actorID,
		TeamID:      teamID,
		Action:      action,
		TargetType:  "site",
		TargetID:    siteID.String(),
		TargetLabel: targetLabel,
		Outcome:     googleSearchConsoleAuditOutcome(outcome),
		Details:     details,
	})
}

func sanitizeGoogleSearchConsoleReturnPath(returnPath string) string {
	returnPath = strings.TrimSpace(returnPath)
	if returnPath == "" {
		return "/integration/google-search-console"
	}
	parsed, err := url.Parse(returnPath)
	if err != nil || parsed.IsAbs() || !strings.HasPrefix(returnPath, "/") || strings.HasPrefix(returnPath, "//") {
		return "/integration/google-search-console"
	}
	return returnPath
}

func databaseGoogleSearchConsoleConnectionInput(teamID, userID uuid.UUID, token searchconsole.Token, connectedAt time.Time) database.GoogleSearchConsoleConnectionInput {
	return database.GoogleSearchConsoleConnectionInput{
		TeamID:             teamID,
		ConnectedByUserID:  userID,
		GoogleAccountEmail: token.GoogleAccountEmail,
		GoogleAccountID:    token.GoogleAccountID,
		AccessToken:        token.AccessToken,
		RefreshToken:       token.RefreshToken,
		TokenType:          token.TokenType,
		Scope:              token.Scope,
		TokenExpiry:        token.Expiry,
		ConnectedAt:        connectedAt,
	}
}

func googleSearchConsoleConnectionToken(conn *database.GoogleSearchConsoleConnection) searchconsole.Token {
	if conn == nil {
		return searchconsole.Token{}
	}
	return searchconsole.Token{
		AccessToken:  conn.AccessToken,
		RefreshToken: conn.RefreshToken,
		TokenType:    conn.TokenType,
		Scope:        conn.Scope,
		Expiry:       conn.TokenExpiry,
	}
}

func googleSearchConsolePropertyMatchesSite(siteDomain, propertyURI string) bool {
	siteHost := googleSearchConsoleNormalizeHost(siteDomain)
	if siteHost == "" {
		return false
	}
	property := strings.ToLower(strings.TrimSpace(propertyURI))
	if strings.HasPrefix(property, "sc-domain:") {
		propertyDomain := googleSearchConsoleNormalizeHost(strings.TrimPrefix(property, "sc-domain:"))
		return propertyDomain != "" && (siteHost == propertyDomain || strings.HasSuffix(siteHost, "."+propertyDomain))
	}
	parsed, err := url.Parse(property)
	if err != nil {
		return false
	}
	propertyHost := googleSearchConsoleNormalizeHost(parsed.Hostname())
	if propertyHost == "" {
		return false
	}
	return googleSearchConsoleHostWithoutWWW(siteHost) == googleSearchConsoleHostWithoutWWW(propertyHost)
}

func googleSearchConsoleNormalizeHost(value string) string {
	host := strings.ToLower(strings.TrimSpace(value))
	if strings.Contains(host, "://") {
		parsed, err := url.Parse(host)
		if err != nil {
			return ""
		}
		host = parsed.Hostname()
	}
	return strings.TrimSuffix(host, ".")
}

func googleSearchConsoleHostWithoutWWW(host string) string {
	return strings.TrimPrefix(host, "www.")
}

func googleSearchConsoleConnectionTargetType() string {
	return "google_search_console_connection"
}

func googleSearchConsoleConnectionTargetID(teamID uuid.UUID) string {
	if teamID == uuid.Nil {
		return ""
	}
	return teamID.String()
}

func googleSearchConsoleAuditDetails(outcome string) string {
	outcome = strings.TrimSpace(outcome)
	if outcome == "" {
		return "outcome=unknown"
	}
	return "outcome=" + outcome
}

func googleSearchConsoleAuditOutcome(outcome string) string {
	outcome = strings.TrimSpace(outcome)
	if outcome == "" {
		return "success"
	}
	return outcome
}

func googleSearchConsoleRoleCanManageSite(role authcore.SiteRole) bool {
	return role == authcore.SiteOwner || role == authcore.SiteAdmin
}

func googleSearchConsoleSyncStatusResponse(state *database.GoogleSearchConsoleSyncState) *api.GoogleSearchConsoleSyncStatus {
	if state == nil {
		return nil
	}
	return &api.GoogleSearchConsoleSyncStatus{
		State:             state.State,
		ImportedStartDate: api.NewDateOnlyPtr(state.ImportedStartDate),
		ImportedEndDate:   api.NewDateOnlyPtr(state.ImportedEndDate),
		LastSuccessAt:     state.LastSuccessAt,
		LastAttemptAt:     state.LastAttemptAt,
		LastErrorCategory: state.LastErrorCategory,
		NextRetryAt:       state.NextRetryAt,
		Manual:            state.Manual,
	}
}

func googleSearchConsoleMappingAuditDetails(oldPropertyURI, newPropertyURI string) string {
	return "old_property_uri=" + strings.TrimSpace(oldPropertyURI) + ";new_property_uri=" + strings.TrimSpace(newPropertyURI)
}
