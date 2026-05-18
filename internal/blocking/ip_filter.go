package blocking

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/database"
)

const defaultRefreshInterval = 30 * time.Second

type IPFilter struct {
	store *database.Store

	// Map site ID to blocked networks.
	siteRules map[uuid.UUID][]netip.Prefix
	// Global blocked networks.
	globalRules []netip.Prefix
	// Map site ID to blocked country codes.
	siteCountries map[uuid.UUID]map[string]struct{}
	// Global blocked country codes.
	globalCountries map[string]struct{}

	mu sync.RWMutex
}

const (
	BlockReasonInstanceCIDR    = "instance_cidr"
	BlockReasonSiteCIDR        = "site_cidr"
	BlockReasonInstanceCountry = "instance_country"
	BlockReasonSiteCountry     = "site_country"
)

type BlockDecision struct {
	Blocked bool
	Reason  string
}

func NewIPFilter(store *database.Store) *IPFilter {
	return &IPFilter{
		store:           store,
		siteRules:       make(map[uuid.UUID][]netip.Prefix),
		siteCountries:   make(map[uuid.UUID]map[string]struct{}),
		globalCountries: make(map[string]struct{}),
	}
}

// StartRefreshLoop updates in-memory rules every 30 seconds.
func (f *IPFilter) StartRefreshLoop(ctx context.Context) {
	if f.store == nil {
		return
	}

	if err := f.Refresh(ctx); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("Failed initial IP exclusion load", "error", err)
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
					slog.Error("Failed to refresh IP exclusions", "error", err)
				}
			}
		}
	}()
}

func (f *IPFilter) Refresh(ctx context.Context) error {
	instanceCIDRs, err := f.store.ListInstanceExclusionCIDRs(ctx)
	if err != nil {
		return err
	}

	siteCIDRs, err := f.store.ListSiteExclusionCIDRs(ctx)
	if err != nil {
		return err
	}
	instanceCountries, err := f.store.ListInstanceExclusionCountries(ctx)
	if err != nil {
		return err
	}
	siteCountries, err := f.store.ListSiteExclusionCountries(ctx)
	if err != nil {
		return err
	}

	newGlobals := make([]netip.Prefix, 0, len(instanceCIDRs))
	for _, cidr := range instanceCIDRs {
		_, ipNet, parseErr := NormalizeCIDR(cidr)
		if parseErr != nil {
			slog.Warn("Skipping invalid instance exclusion CIDR", "cidr", cidr, "error", parseErr)
			continue
		}
		newGlobals = append(newGlobals, ipNet)
	}

	newSites := make(map[uuid.UUID][]netip.Prefix)
	for _, rule := range siteCIDRs {
		_, ipNet, parseErr := NormalizeCIDR(rule.CIDR)
		if parseErr != nil {
			slog.Warn("Skipping invalid site exclusion CIDR", "site_id", rule.SiteID, "cidr", rule.CIDR, "error", parseErr)
			continue
		}
		newSites[rule.SiteID] = append(newSites[rule.SiteID], ipNet)
	}

	newGlobalCountries := make(map[string]struct{}, len(instanceCountries))
	for _, countryCode := range instanceCountries {
		if countryCode = normalizeCountryCode(countryCode); countryCode != "" {
			newGlobalCountries[countryCode] = struct{}{}
		}
	}

	newSiteCountries := make(map[uuid.UUID]map[string]struct{})
	for _, rule := range siteCountries {
		countryCode := normalizeCountryCode(rule.CountryCode)
		if countryCode == "" {
			continue
		}
		if _, ok := newSiteCountries[rule.SiteID]; !ok {
			newSiteCountries[rule.SiteID] = make(map[string]struct{})
		}
		newSiteCountries[rule.SiteID][countryCode] = struct{}{}
	}

	f.mu.Lock()
	f.globalRules = newGlobals
	f.siteRules = newSites
	f.globalCountries = newGlobalCountries
	f.siteCountries = newSiteCountries
	f.mu.Unlock()

	return nil
}

func (f *IPFilter) IsBlocked(siteID uuid.UUID, ipStr string) bool {
	return f.Evaluate(siteID, ipStr, "").Blocked
}

func (f *IPFilter) Evaluate(siteID uuid.UUID, ipStr string, countryCode string) BlockDecision {
	ip := parseIP(ipStr)

	f.mu.RLock()
	defer f.mu.RUnlock()

	if ip.IsValid() {
		for _, blockedNetwork := range f.globalRules {
			if blockedNetwork.Contains(ip) {
				return BlockDecision{Blocked: true, Reason: BlockReasonInstanceCIDR}
			}
		}

		if siteNetworks, ok := f.siteRules[siteID]; ok {
			for _, blockedNetwork := range siteNetworks {
				if blockedNetwork.Contains(ip) {
					return BlockDecision{Blocked: true, Reason: BlockReasonSiteCIDR}
				}
			}
		}
	}

	countryCode = normalizeCountryCode(countryCode)
	if countryCode != "" {
		if _, ok := f.globalCountries[countryCode]; ok {
			return BlockDecision{Blocked: true, Reason: BlockReasonInstanceCountry}
		}
		if siteCountryRules, ok := f.siteCountries[siteID]; ok {
			if _, ok := siteCountryRules[countryCode]; ok {
				return BlockDecision{Blocked: true, Reason: BlockReasonSiteCountry}
			}
		}
	}

	return BlockDecision{}
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
