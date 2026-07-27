package user

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/controlstore"
	"hitkeep/internal/database"
	"hitkeep/internal/server/shared"
)

var customTrackingHostnameRegex = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,}$`)

func (h *handler) customTrackingDomainPlanBlocked(r *http.Request, teamID uuid.UUID) bool {
	return !h.ctx.Limits().AllowsCustomTrackingDomains(r.Context(), shared.GetUserIDFromContext(r), teamID)
}

type trackingDomainVerifier struct {
	lookupTXT    func(context.Context, string) ([]string, error)
	lookupCNAME  func(context.Context, string) (string, error)
	lookupIPAddr func(context.Context, string) ([]netip.Addr, error)
	httpClient   *http.Client
}

var defaultTrackingDomainVerifier = trackingDomainVerifier{
	lookupTXT: func(ctx context.Context, name string) ([]string, error) {
		return net.DefaultResolver.LookupTXT(ctx, name)
	},
	lookupCNAME: func(ctx context.Context, host string) (string, error) {
		return net.DefaultResolver.LookupCNAME(ctx, host)
	},
	lookupIPAddr: func(ctx context.Context, host string) ([]netip.Addr, error) {
		addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		out := make([]netip.Addr, 0, len(addrs))
		for _, addr := range addrs {
			if parsed, ok := netip.AddrFromSlice(addr.IP); ok {
				out = append(out, parsed.Unmap())
			}
		}
		return out, nil
	},
	httpClient: &http.Client{Timeout: 8 * time.Second},
}

func (h *handler) handleListCustomTrackingDomains() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		teamID, ok := parseTeamIDPath(w, r)
		if !ok {
			return
		}
		domains, err := h.ctx.Store.ListCustomTrackingDomains(r.Context(), teamID)
		if err != nil {
			slog.Error("Failed to list custom tracking domains", "error", err, "team_id", teamID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		domains = decorateCustomTrackingDomains(domains, h.ctx.Config.CustomTrackingDNSTargetValue())
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(domains); err != nil {
			slog.Error("Failed to encode custom tracking domains response", "error", err, "team_id", teamID)
		}
	}
}

func (h *handler) handleCreateCustomTrackingDomain() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		teamID, ok := parseTeamIDPath(w, r)
		if !ok {
			return
		}
		if h.customTrackingDomainPlanBlocked(r, teamID) {
			http.Error(w, "Custom tracking domains require the Pro plan or higher", http.StatusForbidden)
			return
		}
		userID := shared.GetUserIDFromContext(r)

		var req api.CreateCustomTrackingDomainRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		hostname := database.NormalizeCustomTrackingHostname(req.Hostname)
		if !isValidCustomTrackingHostname(hostname) {
			http.Error(w, "Invalid custom tracking hostname", http.StatusBadRequest)
			return
		}
		if h.conflictsWithPublicHost(hostname) {
			http.Error(w, "Custom tracking domain must not match the HitKeep public host", http.StatusConflict)
			return
		}
		if site, err := h.ctx.Store.FindSiteByDomain(r.Context(), hostname); err != nil {
			slog.Error("Failed to check site domain conflict", "error", err, "hostname", hostname)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		} else if site != nil {
			http.Error(w, "Custom tracking domain conflicts with an existing site domain", http.StatusConflict)
			return
		}
		if existing, err := h.ctx.Store.FindCustomTrackingDomainByHostname(r.Context(), hostname); err != nil {
			slog.Error("Failed to check custom tracking domain conflict", "error", err, "hostname", hostname)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		} else if existing != nil {
			http.Error(w, "Custom tracking domain already exists", http.StatusConflict)
			return
		}

		domain, err := h.ctx.Store.CreateCustomTrackingDomain(r.Context(), database.CustomTrackingDomainInput{
			TeamID:  teamID,
			Host:    hostname,
			TLSMode: h.ctx.Config.CustomTrackingTLSMode,
		})
		if err != nil {
			slog.Error("Failed to create custom tracking domain", "error", err, "team_id", teamID, "hostname", hostname)
			http.Error(w, "Failed to create custom tracking domain", http.StatusConflict)
			return
		}
		h.appendCustomTrackingDomainAudit(r, teamID, userID, *domain, "tracking_domain.created", "Custom tracking domain "+domain.Hostname+" created")
		*domain = decorateCustomTrackingDomain(*domain, h.ctx.Config.CustomTrackingDNSTargetValue())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(domain); err != nil {
			slog.Error("Failed to encode custom tracking domain response", "error", err, "team_id", teamID)
		}
	}
}

func (h *handler) handleVerifyCustomTrackingDomain() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		teamID, domainID, ok := parseTeamDomainIDs(w, r)
		if !ok {
			return
		}
		if h.customTrackingDomainPlanBlocked(r, teamID) {
			http.Error(w, "Custom tracking domains require the Pro plan or higher", http.StatusForbidden)
			return
		}
		userID := shared.GetUserIDFromContext(r)
		domain, err := h.ctx.Store.GetCustomTrackingDomainForTeam(r.Context(), teamID, domainID)
		if err != nil {
			slog.Error("Failed to load custom tracking domain", "error", err, "team_id", teamID, "domain_id", domainID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if domain == nil {
			http.Error(w, "Custom tracking domain not found", http.StatusNotFound)
			return
		}

		verified, err := h.verifyCustomTrackingDomain(r.Context(), *domain)
		if err != nil {
			slog.Error("Failed to verify custom tracking domain", "error", err, "team_id", teamID, "domain_id", domainID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		h.appendCustomTrackingDomainAudit(r, teamID, userID, *verified, "tracking_domain.verified", "Custom tracking domain "+verified.Hostname+" verification checked")
		*verified = decorateCustomTrackingDomain(*verified, h.ctx.Config.CustomTrackingDNSTargetValue())
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(verified); err != nil {
			slog.Error("Failed to encode verified custom tracking domain response", "error", err, "team_id", teamID, "domain_id", domainID)
		}
	}
}

func (h *handler) handleUpdateCustomTrackingDomain() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		teamID, domainID, ok := parseTeamDomainIDs(w, r)
		if !ok {
			return
		}
		userID := shared.GetUserIDFromContext(r)
		var req api.UpdateCustomTrackingDomainRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		if req.Enabled == nil {
			http.Error(w, "enabled is required", http.StatusBadRequest)
			return
		}
		domain, err := h.ctx.Store.UpdateCustomTrackingDomainEnabled(r.Context(), teamID, domainID, *req.Enabled)
		if errors.Is(err, controlstore.ErrCustomTrackingDomainNotFound) {
			http.Error(w, "Custom tracking domain not found", http.StatusNotFound)
			return
		}
		if err != nil {
			slog.Error("Failed to update custom tracking domain", "error", err, "team_id", teamID, "domain_id", domainID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		action := "tracking_domain.enabled"
		details := "Custom tracking domain " + domain.Hostname + " enabled"
		if !*req.Enabled {
			action = "tracking_domain.disabled"
			details = "Custom tracking domain " + domain.Hostname + " disabled"
		}
		h.appendCustomTrackingDomainAudit(r, teamID, userID, *domain, action, details)
		*domain = decorateCustomTrackingDomain(*domain, h.ctx.Config.CustomTrackingDNSTargetValue())
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(domain); err != nil {
			slog.Error("Failed to encode updated custom tracking domain response", "error", err, "team_id", teamID, "domain_id", domainID)
		}
	}
}

func (h *handler) handleDeleteCustomTrackingDomain() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		teamID, domainID, ok := parseTeamDomainIDs(w, r)
		if !ok {
			return
		}
		userID := shared.GetUserIDFromContext(r)
		domain, err := h.ctx.Store.GetCustomTrackingDomainForTeam(r.Context(), teamID, domainID)
		if err != nil {
			slog.Error("Failed to load custom tracking domain before delete", "error", err, "team_id", teamID, "domain_id", domainID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if domain == nil {
			http.Error(w, "Custom tracking domain not found", http.StatusNotFound)
			return
		}
		if err := h.ctx.Store.DeleteCustomTrackingDomain(r.Context(), teamID, domainID); errors.Is(err, controlstore.ErrCustomTrackingDomainNotFound) {
			http.Error(w, "Custom tracking domain not found", http.StatusNotFound)
			return
		} else if err != nil {
			slog.Error("Failed to delete custom tracking domain", "error", err, "team_id", teamID, "domain_id", domainID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		h.appendCustomTrackingDomainAudit(r, teamID, userID, *domain, "tracking_domain.deleted", "Custom tracking domain "+domain.Hostname+" deleted")
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *handler) verifyCustomTrackingDomain(ctx context.Context, domain api.CustomTrackingDomain) (*api.CustomTrackingDomain, error) {
	now := time.Now().UTC()
	verifier := defaultTrackingDomainVerifier
	expectedTXT := domain.DNSTXTValue
	txtOK, txtErr := verifier.hasExpectedTXT(ctx, domain.DNSTXTName, expectedTXT)
	if !txtOK {
		return h.ctx.Store.UpdateCustomTrackingDomainVerification(ctx, domain.ID, database.CustomTrackingDomainVerificationResult{
			VerificationStatus: string(api.CustomTrackingDomainStatusFailed),
			TargetStatus:       string(api.CustomTrackingDomainStatusPending),
			TLSStatus:          string(api.CustomTrackingDomainStatusPending),
			LastError:          "TXT record " + domain.DNSTXTName + " does not contain " + expectedTXT + dnsErrorSuffix(txtErr),
			LastCheckedAt:      now,
		})
	}

	targetOK, targetErr := verifier.matchesDNSTarget(ctx, domain.Hostname, h.ctx.Config.CustomTrackingDNSTargetValue())
	if !targetOK {
		return h.ctx.Store.UpdateCustomTrackingDomainVerification(ctx, domain.ID, database.CustomTrackingDomainVerificationResult{
			VerificationStatus: string(api.CustomTrackingDomainStatusFailed),
			TargetStatus:       string(api.CustomTrackingDomainStatusFailed),
			TLSStatus:          string(api.CustomTrackingDomainStatusPending),
			LastError:          "DNS target does not match this HitKeep deployment" + dnsErrorSuffix(targetErr),
			LastCheckedAt:      now,
		})
	}

	verifiedAt := now
	intermediate, err := h.ctx.Store.UpdateCustomTrackingDomainVerification(ctx, domain.ID, database.CustomTrackingDomainVerificationResult{
		VerificationStatus: string(api.CustomTrackingDomainStatusVerified),
		TargetStatus:       string(api.CustomTrackingDomainStatusVerified),
		TLSStatus:          string(api.CustomTrackingDomainStatusPending),
		VerifiedAt:         &verifiedAt,
		LastCheckedAt:      now,
	})
	if err != nil {
		return nil, err
	}
	if intermediate != nil {
		domain = *intermediate
	}

	tlsStatus := string(api.CustomTrackingDomainStatusVerified)
	lastError := ""
	if err := verifier.probeTrackerAsset(ctx, domain.Hostname); err != nil {
		tlsStatus = string(api.CustomTrackingDomainStatusFailed)
		lastError = err.Error()
	}
	return h.ctx.Store.UpdateCustomTrackingDomainVerification(ctx, domain.ID, database.CustomTrackingDomainVerificationResult{
		VerificationStatus: string(api.CustomTrackingDomainStatusVerified),
		TargetStatus:       string(api.CustomTrackingDomainStatusVerified),
		TLSStatus:          tlsStatus,
		LastError:          lastError,
		VerifiedAt:         &verifiedAt,
		LastCheckedAt:      time.Now().UTC(),
	})
}

func (v trackingDomainVerifier) hasExpectedTXT(ctx context.Context, name, expected string) (bool, error) {
	records, err := v.lookupTXT(ctx, name)
	if err != nil {
		return false, err
	}
	for _, record := range records {
		if strings.TrimSpace(record) == expected {
			return true, nil
		}
	}
	return false, nil
}

func (v trackingDomainVerifier) matchesDNSTarget(ctx context.Context, hostname, target string) (bool, error) {
	target = normalizeTrackingDNSTarget(target)
	if target == "" {
		return false, fmt.Errorf("no custom tracking DNS target is configured")
	}

	var firstErr error
	if cname, err := v.lookupCNAME(ctx, hostname); err == nil && normalizeTrackingDNSTarget(cname) == target {
		return true, nil
	} else if err != nil {
		firstErr = err
	}

	hostIPs, err := v.lookupIPAddr(ctx, hostname)
	if err != nil {
		if firstErr == nil {
			firstErr = err
		}
	} else {
		if targetIP, err := netip.ParseAddr(target); err == nil {
			if containsIP(hostIPs, targetIP.Unmap()) {
				return true, nil
			}
		} else {
			targetIPs, targetErr := v.lookupIPAddr(ctx, target)
			if targetErr == nil && intersectsIP(hostIPs, targetIPs) {
				return true, nil
			} else if targetErr != nil && firstErr == nil {
				firstErr = targetErr
			}
		}
	}

	return false, firstErr
}

func (v trackingDomainVerifier) probeTrackerAsset(ctx context.Context, hostname string) error {
	client := v.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+hostname+"/hk.js", nil)
	if err != nil {
		return fmt.Errorf("build TLS probe request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTPS tracker probe failed: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTPS tracker probe returned %d", resp.StatusCode)
	}
	return nil
}

func (h *handler) conflictsWithPublicHost(hostname string) bool {
	publicHost := ""
	if h != nil && h.ctx != nil && h.ctx.Config != nil {
		if parsed, err := url.Parse(strings.TrimSpace(h.ctx.Config.PublicURL)); err == nil {
			publicHost = database.NormalizeCustomTrackingHostname(parsed.Hostname())
		}
	}
	return publicHost != "" && hostname == publicHost
}

func parseTeamIDPath(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	teamID, err := uuid.Parse(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		http.Error(w, "Invalid team ID", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return teamID, true
}

func parseTeamDomainIDs(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	teamID, ok := parseTeamIDPath(w, r)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	domainID, err := uuid.Parse(strings.TrimSpace(r.PathValue("domainId")))
	if err != nil {
		http.Error(w, "Invalid custom tracking domain ID", http.StatusBadRequest)
		return uuid.Nil, uuid.Nil, false
	}
	return teamID, domainID, true
}

func isValidCustomTrackingHostname(hostname string) bool {
	if len(hostname) > 253 || strings.Contains(hostname, "://") {
		return false
	}
	return customTrackingHostnameRegex.MatchString(hostname)
}

func decorateCustomTrackingDomains(domains []api.CustomTrackingDomain, target string) []api.CustomTrackingDomain {
	out := make([]api.CustomTrackingDomain, 0, len(domains))
	for _, domain := range domains {
		out = append(out, decorateCustomTrackingDomain(domain, target))
	}
	return out
}

func decorateCustomTrackingDomain(domain api.CustomTrackingDomain, target string) api.CustomTrackingDomain {
	domain.DNSTarget = target
	domain.Active = database.CustomTrackingDomainIsActive(domain)
	return domain
}

func normalizeTrackingDNSTarget(value string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), ".")
}

func containsIP(addrs []netip.Addr, target netip.Addr) bool {
	for _, addr := range addrs {
		if addr.Unmap() == target.Unmap() {
			return true
		}
	}
	return false
}

func intersectsIP(left, right []netip.Addr) bool {
	for _, addr := range left {
		if containsIP(right, addr) {
			return true
		}
	}
	return false
}

func dnsErrorSuffix(err error) string {
	if err == nil {
		return ""
	}
	return ": " + err.Error()
}

func (h *handler) appendCustomTrackingDomainAudit(r *http.Request, teamID, actorID uuid.UUID, domain api.CustomTrackingDomain, action, details string) {
	h.ctx.AppendAuditEvent(r.Context(), r, shared.AuditEvent{
		ActorID:     actorID,
		TeamID:      teamID,
		Action:      action,
		TargetType:  "tracking_domain",
		TargetID:    domain.ID.String(),
		TargetLabel: domain.Hostname,
		Outcome:     "success",
		Details:     details,
	})
}
