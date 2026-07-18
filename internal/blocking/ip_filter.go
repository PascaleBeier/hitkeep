package blocking

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/netip"
	"net/url"
	pathpkg "path"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
)

const defaultRefreshInterval = 30 * time.Second

const (
	BlockReasonInstanceCIDR      = "instance_cidr"
	BlockReasonInstanceCountry   = "instance_country"
	BlockReasonInstanceUserAgent = "instance_user_agent"
	BlockReasonInstancePath      = "instance_path"
	BlockReasonTeamCIDR          = "team_cidr"
	BlockReasonTeamCountry       = "team_country"
	BlockReasonTeamUserAgent     = "team_user_agent"
	BlockReasonTeamPath          = "team_path"
	BlockReasonSiteCIDR          = "site_cidr"
	BlockReasonSiteCountry       = "site_country"
	BlockReasonSiteUserAgent     = "site_user_agent"
	BlockReasonSitePath          = "site_path"
)

type compiledTrafficExclusions struct {
	cidrs      []netip.Prefix
	countries  map[string]struct{}
	userAgents []string
	paths      []string
}

type IPFilter struct {
	store *database.Store

	instanceRules compiledTrafficExclusions
	teamRules     map[uuid.UUID]compiledTrafficExclusions
	siteRules     map[uuid.UUID]compiledTrafficExclusions
	siteTeams     map[uuid.UUID]uuid.UUID

	mu sync.RWMutex
}

type TrafficExclusionContext struct {
	IP          string
	CountryCode string
	UserAgent   string
	Path        string
}

type BlockDecision struct {
	Blocked bool
	Reason  string
}

func NewIPFilter(store *database.Store) *IPFilter {
	return &IPFilter{
		store:         store,
		instanceRules: newCompiledTrafficExclusions(),
		teamRules:     make(map[uuid.UUID]compiledTrafficExclusions),
		siteRules:     make(map[uuid.UUID]compiledTrafficExclusions),
		siteTeams:     make(map[uuid.UUID]uuid.UUID),
	}
}

func newCompiledTrafficExclusions() compiledTrafficExclusions {
	return compiledTrafficExclusions{countries: make(map[string]struct{})}
}

// StartRefreshLoop updates in-memory rules every 30 seconds as a fallback for write-triggered refreshes.
func (f *IPFilter) StartRefreshLoop(ctx context.Context) {
	if f.store == nil {
		return
	}

	if err := f.Refresh(ctx); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("Failed initial traffic exclusion load", "error", err)
	}

	go func() {
		ticker := time.NewTicker(defaultRefreshInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := f.Refresh(ctx); err != nil && !errors.Is(err, context.Canceled) {
					slog.Error("Failed to refresh traffic exclusions", "error", err)
				}
			}
		}
	}()
}

func (f *IPFilter) Refresh(ctx context.Context) error {
	rules, err := f.store.ListAllTrafficExclusions(ctx)
	if err != nil {
		return err
	}
	mappings, err := f.store.ListSiteTeamMappings(ctx)
	if err != nil {
		return err
	}

	instanceRules := newCompiledTrafficExclusions()
	teamRules := make(map[uuid.UUID]compiledTrafficExclusions)
	siteRules := make(map[uuid.UUID]compiledTrafficExclusions)
	for _, rule := range rules {
		switch rule.Scope {
		case database.TrafficExclusionScopeInstance:
			appendCompiledRule(&instanceRules, rule)
		case database.TrafficExclusionScopeTeam:
			if rule.TeamID == nil {
				continue
			}
			compiled := teamRules[*rule.TeamID]
			if compiled.countries == nil {
				compiled = newCompiledTrafficExclusions()
			}
			appendCompiledRule(&compiled, rule)
			teamRules[*rule.TeamID] = compiled
		case database.TrafficExclusionScopeSite:
			if rule.SiteID == nil {
				continue
			}
			compiled := siteRules[*rule.SiteID]
			if compiled.countries == nil {
				compiled = newCompiledTrafficExclusions()
			}
			appendCompiledRule(&compiled, rule)
			siteRules[*rule.SiteID] = compiled
		}
	}

	siteTeams := make(map[uuid.UUID]uuid.UUID, len(mappings))
	for _, mapping := range mappings {
		siteTeams[mapping.SiteID] = mapping.TeamID
	}

	f.mu.Lock()
	f.instanceRules = instanceRules
	f.teamRules = teamRules
	f.siteRules = siteRules
	f.siteTeams = siteTeams
	f.mu.Unlock()
	return nil
}

func appendCompiledRule(compiled *compiledTrafficExclusions, rule api.IPExclusion) {
	switch rule.Type {
	case "cidr":
		_, prefix, err := NormalizeCIDR(rule.CIDR)
		if err == nil {
			compiled.cidrs = append(compiled.cidrs, prefix)
		}
	case "country":
		if countryCode := normalizeCountryCode(rule.CountryCode); countryCode != "" {
			compiled.countries[countryCode] = struct{}{}
		}
	case "user_agent":
		if userAgent := strings.ToLower(strings.TrimSpace(rule.UserAgent)); userAgent != "" {
			compiled.userAgents = append(compiled.userAgents, userAgent)
		}
	case "path":
		if exclusionPath, ok := NormalizeExclusionPath(rule.Path); ok {
			compiled.paths = append(compiled.paths, exclusionPath)
		}
	}
}

func (f *IPFilter) IsBlocked(siteID uuid.UUID, ipStr string) bool {
	return f.Evaluate(siteID, ipStr, "").Blocked
}

// Evaluate preserves the network-only API used before contextual exclusions were added.
func (f *IPFilter) Evaluate(siteID uuid.UUID, ipStr, countryCode string) BlockDecision {
	return f.EvaluateTraffic(siteID, TrafficExclusionContext{IP: ipStr, CountryCode: countryCode})
}

func (f *IPFilter) EvaluateTraffic(siteID uuid.UUID, traffic TrafficExclusionContext) BlockDecision {
	ip := parseIP(traffic.IP)
	countryCode := normalizeCountryCode(traffic.CountryCode)
	userAgent := strings.ToLower(strings.TrimSpace(traffic.UserAgent))
	exclusionPath, hasPath := NormalizeExclusionPath(traffic.Path)

	f.mu.RLock()
	defer f.mu.RUnlock()

	if decision := evaluateCompiledRules(f.instanceRules, "instance", ip, countryCode, userAgent, exclusionPath, hasPath); decision.Blocked {
		return decision
	}
	if teamID, ok := f.siteTeams[siteID]; ok {
		if rules, exists := f.teamRules[teamID]; exists {
			if decision := evaluateCompiledRules(rules, "team", ip, countryCode, userAgent, exclusionPath, hasPath); decision.Blocked {
				return decision
			}
		}
	}
	if rules, ok := f.siteRules[siteID]; ok {
		if decision := evaluateCompiledRules(rules, "site", ip, countryCode, userAgent, exclusionPath, hasPath); decision.Blocked {
			return decision
		}
	}
	return BlockDecision{}
}

func evaluateCompiledRules(
	rules compiledTrafficExclusions,
	scope string,
	ip netip.Addr,
	countryCode, userAgent, exclusionPath string,
	hasPath bool,
) BlockDecision {
	if ip.IsValid() {
		for _, network := range rules.cidrs {
			if network.Contains(ip) {
				return BlockDecision{Blocked: true, Reason: scope + "_cidr"}
			}
		}
	}
	if countryCode != "" {
		if _, ok := rules.countries[countryCode]; ok {
			return BlockDecision{Blocked: true, Reason: scope + "_country"}
		}
	}
	if userAgent != "" {
		for _, rule := range rules.userAgents {
			if strings.Contains(userAgent, rule) {
				return BlockDecision{Blocked: true, Reason: scope + "_user_agent"}
			}
		}
	}
	if hasPath {
		for _, rule := range rules.paths {
			if pathMatches(rule, exclusionPath) {
				return BlockDecision{Blocked: true, Reason: scope + "_path"}
			}
		}
	}
	return BlockDecision{}
}

func NormalizeExclusionPath(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", false
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", false
	}
	cleaned := parsed.EscapedPath()
	if cleaned == "" && (parsed.Host != "" || strings.HasPrefix(trimmed, "/")) {
		cleaned = "/"
	}
	if cleaned == "" {
		cleaned = parsed.Path
	}
	if cleaned == "" {
		return "", false
	}
	if !strings.HasPrefix(cleaned, "/") {
		cleaned = "/" + cleaned
	}
	cleaned = pathpkg.Clean(cleaned)
	if cleaned == "." || cleaned == "" {
		return "", false
	}
	return cleaned, true
}

func pathMatches(rule, candidate string) bool {
	if rule == "/" {
		return true
	}
	return candidate == rule || strings.HasPrefix(candidate, rule+"/")
}

func parseIP(value string) netip.Addr {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return netip.Addr{}
	}
	if host, _, err := net.SplitHostPort(trimmed); err == nil {
		trimmed = host
	}
	addr, err := netip.ParseAddr(trimmed)
	if err != nil {
		return netip.Addr{}
	}
	return addr.Unmap()
}

func normalizeCountryCode(value string) string {
	code := strings.ToUpper(strings.TrimSpace(value))
	if len(code) != 2 {
		return ""
	}
	return code
}
