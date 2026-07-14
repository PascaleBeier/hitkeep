package user

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/appurl"
	"hitkeep/internal/database"
	"hitkeep/internal/server/shared"
	"hitkeep/internal/sso"
)

const (
	defaultSSOEmailClaim       = "email"
	defaultSSODisplayNameClaim = "name"
	teamSSODiscoveryTimeout    = 10 * time.Second
	maxSSODomains              = 50
	maxSSOIssuerURLLength      = 2048
	maxSSOClientIDLength       = 512
)

var ssoClaimPathPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,127}$`)

type teamSSORequest struct {
	ProviderType     string   `json:"provider_type"`
	IssuerURL        string   `json:"issuer_url"`
	ClientID         string   `json:"client_id"`
	ClientSecret     string   `json:"client_secret"`
	AllowedDomains   []string `json:"allowed_domains"`
	EmailClaim       string   `json:"email_claim"`
	DisplayNameClaim string   `json:"display_name_claim"`
	AutoProvision    bool     `json:"auto_provision"`
	Enabled          bool     `json:"enabled"`
}

func (h *handler) handleGetTeamSSO() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		teamID, actorID, ok := h.resolveTeamSSOAccess(w, r)
		if !ok {
			return
		}
		config, err := h.ctx.Store.GetTeamSSOConfig(r.Context(), teamID)
		if err != nil {
			slog.Error("Failed to load team SSO configuration", "error", err, "team_id", teamID, "user_id", actorID)
			http.Error(w, "Could not load SSO configuration", http.StatusInternalServerError)
			return
		}
		writeTeamSSOConfig(w, h.teamSSOResponse(config))
	}
}

func (h *handler) handleUpsertTeamSSO() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		teamID, actorID, ok := h.resolveTeamSSOAccess(w, r)
		if !ok {
			return
		}
		var req teamSSORequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			writeTeamActionError(w, http.StatusBadRequest, "invalid_request", "Enter a valid SSO configuration")
			return
		}

		normalized, existing, err := h.normalizeTeamSSORequest(r.Context(), teamID, req)
		if err != nil {
			h.writeTeamSSOValidationError(w, err)
			return
		}
		discoveryCtx, cancel := context.WithTimeout(r.Context(), teamSSODiscoveryTimeout)
		defer cancel()
		if _, err := h.ssoClient().Discover(discoveryCtx, normalized.IssuerURL); err != nil {
			// Discovery errors can contain upstream response bodies. Keep provider
			// details out of logs and return a stable operator-facing error instead.
			slog.Warn("Team SSO provider discovery failed", "team_id", teamID)
			writeTeamActionError(w, http.StatusBadGateway, "discovery_failed", "HitKeep could not validate the OIDC issuer. Check the issuer URL and try again.")
			return
		}

		secretCiphertext := ""
		if existing != nil {
			secretCiphertext = existing.ClientSecretEncrypted
		}
		if strings.TrimSpace(req.ClientSecret) != "" {
			box, err := sso.NewSecretBox(h.ctx.Config.JWTSecret)
			if err != nil {
				slog.Error("Failed to initialize SSO secret encryption", "error", err, "team_id", teamID)
				http.Error(w, "Could not save SSO configuration", http.StatusInternalServerError)
				return
			}
			secretCiphertext, err = box.Seal(req.ClientSecret)
			if err != nil {
				slog.Error("Failed to encrypt SSO client secret", "error", err, "team_id", teamID)
				http.Error(w, "Could not save SSO configuration", http.StatusInternalServerError)
				return
			}
		}
		if secretCiphertext == "" {
			writeTeamActionError(w, http.StatusBadRequest, "client_secret_required", "Enter the OIDC client secret")
			return
		}

		config := database.TeamSSOConfig{
			TeamID:                teamID,
			ProviderType:          normalized.ProviderType,
			IssuerURL:             normalized.IssuerURL,
			ClientID:              normalized.ClientID,
			ClientSecretEncrypted: secretCiphertext,
			AllowedDomains:        normalized.AllowedDomains,
			EmailClaim:            normalized.EmailClaim,
			DisplayNameClaim:      normalized.DisplayNameClaim,
			AutoProvision:         normalized.AutoProvision,
			Enabled:               normalized.Enabled,
		}
		if err := h.ctx.Store.UpsertTeamSSOConfig(r.Context(), config); err != nil {
			if errors.Is(err, database.ErrTeamSSODomainConflict) {
				writeTeamActionError(w, http.StatusConflict, "domain_conflict", "One of these email domains is already assigned to another team's SSO provider")
				return
			}
			slog.Error("Failed to save team SSO configuration", "error", err, "team_id", teamID)
			http.Error(w, "Could not save SSO configuration", http.StatusInternalServerError)
			return
		}
		stored, err := h.ctx.Store.GetTeamSSOConfig(r.Context(), teamID)
		if err != nil {
			slog.Error("Failed to reload team SSO configuration", "error", err, "team_id", teamID)
			http.Error(w, "Could not load SSO configuration", http.StatusInternalServerError)
			return
		}
		h.appendTeamSSOAudit(r, teamID, actorID, "sso.configuration_updated", "success", fmt.Sprintf("SSO configuration updated (enabled=%t, domains=%d)", config.Enabled, len(config.AllowedDomains)))
		writeTeamSSOConfig(w, h.teamSSOResponse(stored))
	}
}

func (h *handler) handleTestTeamSSO() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		teamID, actorID, ok := h.resolveTeamSSOAccess(w, r)
		if !ok {
			return
		}
		config, err := h.ctx.Store.GetTeamSSOConfig(r.Context(), teamID)
		if err != nil || config == nil {
			writeTeamActionError(w, http.StatusNotFound, "not_configured", "Configure SSO before testing the connection")
			return
		}
		box, err := sso.NewSecretBox(h.ctx.Config.JWTSecret)
		if err != nil {
			http.Error(w, "Could not test SSO configuration", http.StatusInternalServerError)
			return
		}
		if _, err := box.Open(config.ClientSecretEncrypted); err != nil {
			slog.Error("Stored SSO client secret could not be decrypted", "error", err, "team_id", teamID)
			http.Error(w, "Could not test SSO configuration", http.StatusInternalServerError)
			return
		}
		discoveryCtx, cancel := context.WithTimeout(r.Context(), teamSSODiscoveryTimeout)
		defer cancel()
		if _, err := h.ssoClient().Discover(discoveryCtx, config.IssuerURL); err != nil {
			h.appendTeamSSOAudit(r, teamID, actorID, "sso.connection_tested", "failure", "SSO discovery test failed")
			writeTeamActionError(w, http.StatusBadGateway, "discovery_failed", "HitKeep could not reach the configured OIDC issuer")
			return
		}
		h.appendTeamSSOAudit(r, teamID, actorID, "sso.connection_tested", "success", "SSO discovery test succeeded")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

func (h *handler) handleDeleteTeamSSO() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		teamID, actorID, ok := h.resolveTeamSSOAccess(w, r)
		if !ok {
			return
		}
		if err := h.ctx.Store.DeleteTeamSSOConfig(r.Context(), teamID); err != nil {
			if errors.Is(err, database.ErrTeamSSONotFound) {
				writeTeamActionError(w, http.StatusNotFound, "not_configured", "SSO is not configured for this team")
				return
			}
			slog.Error("Failed to delete team SSO configuration", "error", err, "team_id", teamID)
			http.Error(w, "Could not delete SSO configuration", http.StatusInternalServerError)
			return
		}
		h.appendTeamSSOAudit(r, teamID, actorID, "sso.configuration_deleted", "success", "SSO configuration deleted")
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *handler) resolveTeamSSOAccess(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	actorID := shared.GetUserIDFromContext(r)
	teamID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
	if actorID == uuid.Nil || err != nil {
		http.Error(w, "Invalid team scope", http.StatusBadRequest)
		return uuid.Nil, uuid.Nil, false
	}
	limits := h.ctx.Limits()
	if !limits.AllowsSSO(r.Context(), actorID, teamID) {
		writeTeamActionError(w, http.StatusForbidden, "sso_not_available", "SSO is not available for this team")
		return uuid.Nil, uuid.Nil, false
	}
	return teamID, actorID, true
}

func (h *handler) normalizeTeamSSORequest(ctx context.Context, teamID uuid.UUID, req teamSSORequest) (teamSSORequest, *database.TeamSSOConfig, error) {
	req.ProviderType = strings.ToLower(strings.TrimSpace(req.ProviderType))
	if req.ProviderType == "" {
		req.ProviderType = "oidc"
	}
	if req.ProviderType != "oidc" {
		return req, nil, errors.New("only OIDC providers are supported")
	}
	if len(strings.TrimSpace(req.IssuerURL)) > maxSSOIssuerURLLength {
		return req, nil, errors.New("OIDC issuer URL is too long")
	}
	issuerURL, err := sso.NormalizeIssuerURL(req.IssuerURL)
	if err != nil {
		return req, nil, err
	}
	req.IssuerURL = issuerURL
	req.ClientID = strings.TrimSpace(req.ClientID)
	if req.ClientID == "" {
		return req, nil, errors.New("OIDC client ID is required")
	}
	if len(req.ClientID) > maxSSOClientIDLength {
		return req, nil, errors.New("OIDC client ID is too long")
	}
	req.EmailClaim = normalizeSSOClaimPath(req.EmailClaim, defaultSSOEmailClaim)
	req.DisplayNameClaim = normalizeSSOClaimPath(req.DisplayNameClaim, defaultSSODisplayNameClaim)
	if !validSSOClaimPath(req.EmailClaim) || !validSSOClaimPath(req.DisplayNameClaim) {
		return req, nil, errors.New("claim mappings must use valid claim paths")
	}
	domains, err := normalizeSSODomains(req.AllowedDomains)
	if err != nil {
		return req, nil, err
	}
	req.AllowedDomains = domains
	existing, err := h.ctx.Store.GetTeamSSOConfig(ctx, teamID)
	if err != nil {
		return req, nil, err
	}
	return req, existing, nil
}

func normalizeSSOClaimPath(raw, fallback string) string {
	if value := strings.TrimSpace(raw); value != "" {
		return value
	}
	return fallback
}

func validSSOClaimPath(value string) bool {
	return ssoClaimPathPattern.MatchString(value) && !strings.Contains(value, "..")
}

func normalizeSSODomains(input []string) ([]string, error) {
	if len(input) > maxSSODomains {
		return nil, fmt.Errorf("no more than %d email domains are allowed", maxSSODomains)
	}
	domains := make([]string, 0, len(input))
	seen := make(map[string]struct{}, len(input))
	for _, raw := range input {
		domain := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(strings.TrimPrefix(raw, "@"))), ".")
		if !validSSODomain(domain) {
			return nil, fmt.Errorf("invalid SSO email domain")
		}
		if _, ok := seen[domain]; ok {
			continue
		}
		seen[domain] = struct{}{}
		domains = append(domains, domain)
	}
	if len(domains) == 0 {
		return nil, errors.New("at least one email domain is required")
	}
	slices.Sort(domains)
	return domains, nil
}

func validSSODomain(domain string) bool {
	if domain == "" || len(domain) > 253 || strings.ContainsAny(domain, "/:@[]") {
		return false
	}
	for _, label := range strings.Split(domain, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				return false
			}
		}
	}
	return true
}

func (h *handler) writeTeamSSOValidationError(w http.ResponseWriter, err error) {
	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = "Enter a valid SSO configuration"
	}
	writeTeamActionError(w, http.StatusBadRequest, "invalid_configuration", message)
}

func (h *handler) teamSSOResponse(config *database.TeamSSOConfig) api.TeamSSOConfig {
	resp := api.TeamSSOConfig{
		ProviderType:     "oidc",
		EmailClaim:       defaultSSOEmailClaim,
		DisplayNameClaim: defaultSSODisplayNameClaim,
		AllowedDomains:   []string{},
		CallbackURL:      appurl.Path(h.ctx.Config.PublicURL, "/api/auth/sso/callback"),
	}
	if config == nil {
		return resp
	}
	resp.ProviderType = config.ProviderType
	resp.IssuerURL = config.IssuerURL
	resp.ClientID = config.ClientID
	resp.ClientSecretConfigured = strings.TrimSpace(config.ClientSecretEncrypted) != ""
	resp.AllowedDomains = config.AllowedDomains
	resp.EmailClaim = config.EmailClaim
	resp.DisplayNameClaim = config.DisplayNameClaim
	resp.AutoProvision = config.AutoProvision
	resp.Enabled = config.Enabled
	resp.UpdatedAt = config.UpdatedAt
	return resp
}

func writeTeamSSOConfig(w http.ResponseWriter, config api.TeamSSOConfig) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(config); err != nil {
		slog.Error("Failed to encode team SSO response", "error", err)
	}
}

func (h *handler) appendTeamSSOAudit(r *http.Request, teamID, actorID uuid.UUID, action, outcome, details string) {
	targetLabel := teamID.String()
	if team, err := h.ctx.Store.GetTenant(r.Context(), teamID); err == nil && team != nil {
		targetLabel = team.Name
	}
	h.ctx.AppendAuditEvent(r.Context(), r, shared.AuditEvent{
		ActorID:     actorID,
		TeamID:      teamID,
		Action:      action,
		TargetType:  "sso_configuration",
		TargetID:    teamID.String(),
		TargetLabel: targetLabel,
		Outcome:     outcome,
		Details:     details,
	})
}

func (h *handler) ssoClient() *sso.Client {
	if h.ctx.SSO != nil {
		return h.ctx.SSO
	}
	return sso.NewRuntimeClient(h.ctx.Config != nil && h.ctx.Config.CloudHosted)
}
