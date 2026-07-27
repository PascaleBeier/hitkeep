package controlstore

import (
	"maps"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	lru "github.com/hashicorp/golang-lru/v2/expirable"
	"golang.org/x/sync/singleflight"

	"hitkeep/internal/api"
	"hitkeep/internal/auth"
)

const (
	passwordResetCacheSize = 4096
	passwordResetTTL       = time.Hour

	apiClientAuthCacheSize = 4096
	apiClientAuthTTL       = 5 * time.Minute

	instanceRoleCacheSize = 8192
	instanceRoleTTL       = 30 * time.Second

	siteRoleCacheSize = 16384
	siteRoleTTL       = 30 * time.Second

	siteTenantCacheSize = 16384
	siteTenantTTL       = 30 * time.Second

	// Sized above the site count so junk-domain negative entries do not
	// evict live sites.
	siteDomainCacheSize = 16384
	siteDomainTTL       = 30 * time.Second

	customTrackingDomainCacheSize = 4096
	customTrackingDomainTTL       = 30 * time.Second

	cloudConversionMilestoneCacheSize = 32768
	cloudConversionMilestoneTTL       = 24 * time.Hour
)

type passwordResetEntry struct {
	Email     string
	ExpiresAt time.Time
}

type PendingSignupEntry struct {
	Email           string
	HashedPassword  string
	GivenName       string
	LastName        string
	TeamName        string
	Jurisdiction    string
	Locale          string
	PlanCode        string
	BillingInterval string
	AcceptedTosAt   time.Time
	ExpiresAt       time.Time
}

type apiClientAuthCacheEntry struct {
	Auth      APIClientAuth
	ExpiresAt time.Time
}

type runtimeCache struct {
	passwordResetMu         sync.Mutex
	passwordResetsByToken   *lru.LRU[string, passwordResetEntry]
	passwordResetTokenIndex *lru.LRU[string, string]

	apiClientAuthMu        sync.Mutex
	apiClientAuthByToken   *lru.LRU[string, apiClientAuthCacheEntry]
	apiClientTokenByClient *lru.LRU[uuid.UUID, string]

	instanceRoles *lru.LRU[uuid.UUID, auth.InstanceRole]

	siteRolesMu sync.Mutex
	siteRoles   *lru.LRU[uuid.UUID, map[uuid.UUID]auth.SiteRole]

	siteTenantIDs *lru.LRU[uuid.UUID, uuid.UUID]
	siteTenantSF  singleflight.Group

	// A nil value is a cached negative lookup.
	sitesByDomain *lru.LRU[string, *api.Site]
	siteDomainSF  singleflight.Group

	// A nil value is a cached negative lookup.
	customTrackingByHost *lru.LRU[string, *api.CustomTrackingDomain]
	customTrackingSF     singleflight.Group

	cloudConversionMilestones *lru.LRU[string, bool]
}

func newRuntimeCache() *runtimeCache {
	return &runtimeCache{
		passwordResetsByToken:     lru.NewLRU[string, passwordResetEntry](passwordResetCacheSize, nil, passwordResetTTL),
		passwordResetTokenIndex:   lru.NewLRU[string, string](passwordResetCacheSize, nil, passwordResetTTL),
		apiClientAuthByToken:      lru.NewLRU[string, apiClientAuthCacheEntry](apiClientAuthCacheSize, nil, apiClientAuthTTL),
		apiClientTokenByClient:    lru.NewLRU[uuid.UUID, string](apiClientAuthCacheSize, nil, apiClientAuthTTL),
		instanceRoles:             lru.NewLRU[uuid.UUID, auth.InstanceRole](instanceRoleCacheSize, nil, instanceRoleTTL),
		siteRoles:                 lru.NewLRU[uuid.UUID, map[uuid.UUID]auth.SiteRole](siteRoleCacheSize, nil, siteRoleTTL),
		siteTenantIDs:             lru.NewLRU[uuid.UUID, uuid.UUID](siteTenantCacheSize, nil, siteTenantTTL),
		sitesByDomain:             lru.NewLRU[string, *api.Site](siteDomainCacheSize, nil, siteDomainTTL),
		customTrackingByHost:      lru.NewLRU[string, *api.CustomTrackingDomain](customTrackingDomainCacheSize, nil, customTrackingDomainTTL),
		cloudConversionMilestones: lru.NewLRU[string, bool](cloudConversionMilestoneCacheSize, nil, cloudConversionMilestoneTTL),
	}
}

func normalizeEmailCacheKey(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func cloneAPIClientAuth(input APIClientAuth) *APIClientAuth {
	cloned := input
	if len(input.SiteRoles) == 0 {
		cloned.SiteRoles = map[uuid.UUID]auth.SiteRole{}
		return &cloned
	}

	cloned.SiteRoles = make(map[uuid.UUID]auth.SiteRole, len(input.SiteRoles))
	maps.Copy(cloned.SiteRoles, input.SiteRoles)
	return &cloned
}

func cloneSiteRoleMap(input map[uuid.UUID]auth.SiteRole) map[uuid.UUID]auth.SiteRole {
	if len(input) == 0 {
		return map[uuid.UUID]auth.SiteRole{}
	}

	cloned := make(map[uuid.UUID]auth.SiteRole, len(input))
	maps.Copy(cloned, input)
	return cloned
}

func (s *Store) storePasswordResetToken(email, token string, expiresAt time.Time) {
	if s == nil || s.runtime == nil {
		return
	}

	emailKey := normalizeEmailCacheKey(email)
	token = strings.TrimSpace(token)
	if emailKey == "" || token == "" {
		return
	}

	s.runtime.passwordResetMu.Lock()
	defer s.runtime.passwordResetMu.Unlock()

	if previousToken, ok := s.runtime.passwordResetTokenIndex.Peek(emailKey); ok && strings.TrimSpace(previousToken) != "" {
		s.runtime.passwordResetsByToken.Remove(previousToken)
	}

	entry := passwordResetEntry{
		Email:     strings.TrimSpace(email),
		ExpiresAt: expiresAt.UTC(),
	}
	s.runtime.passwordResetsByToken.Add(token, entry)
	s.runtime.passwordResetTokenIndex.Add(emailKey, token)
}

func (s *Store) lookupPasswordResetToken(token string, consume bool) (passwordResetEntry, bool, error) {
	if s == nil || s.runtime == nil {
		return passwordResetEntry{}, false, ErrPasswordResetInvalid
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return passwordResetEntry{}, false, ErrPasswordResetInvalid
	}

	s.runtime.passwordResetMu.Lock()
	defer s.runtime.passwordResetMu.Unlock()

	entry, ok := s.runtime.passwordResetsByToken.Get(token)
	if !ok || strings.TrimSpace(entry.Email) == "" {
		return passwordResetEntry{}, false, ErrPasswordResetInvalid
	}
	if !entry.ExpiresAt.IsZero() && time.Now().UTC().After(entry.ExpiresAt.UTC()) {
		s.runtime.passwordResetsByToken.Remove(token)
		s.runtime.passwordResetTokenIndex.Remove(normalizeEmailCacheKey(entry.Email))
		return passwordResetEntry{}, false, ErrPasswordResetExpired
	}
	if consume {
		s.runtime.passwordResetsByToken.Remove(token)
		s.runtime.passwordResetTokenIndex.Remove(normalizeEmailCacheKey(entry.Email))
	}
	return entry, true, nil
}

func (s *Store) deletePasswordResetTokenForEmail(email string) {
	if s == nil || s.runtime == nil {
		return
	}

	emailKey := normalizeEmailCacheKey(email)
	if emailKey == "" {
		return
	}
	s.runtime.passwordResetMu.Lock()
	defer s.runtime.passwordResetMu.Unlock()

	if token, ok := s.runtime.passwordResetTokenIndex.Peek(emailKey); ok {
		s.runtime.passwordResetsByToken.Remove(strings.TrimSpace(token))
	}
	s.runtime.passwordResetTokenIndex.Remove(emailKey)
}

func (s *Store) cacheAPIClientAuth(tokenHash string, authz APIClientAuth, expiresAt time.Time) {
	if s == nil || s.runtime == nil || strings.TrimSpace(tokenHash) == "" {
		return
	}

	s.runtime.apiClientAuthMu.Lock()
	defer s.runtime.apiClientAuthMu.Unlock()

	s.runtime.apiClientAuthByToken.Add(strings.TrimSpace(tokenHash), apiClientAuthCacheEntry{
		Auth:      *cloneAPIClientAuth(authz),
		ExpiresAt: expiresAt.UTC(),
	})
	s.runtime.apiClientTokenByClient.Add(authz.ClientID, strings.TrimSpace(tokenHash))
}

func (s *Store) getCachedAPIClientAuth(tokenHash string) (*APIClientAuth, bool) {
	if s == nil || s.runtime == nil || strings.TrimSpace(tokenHash) == "" {
		return nil, false
	}

	s.runtime.apiClientAuthMu.Lock()
	defer s.runtime.apiClientAuthMu.Unlock()

	entry, ok := s.runtime.apiClientAuthByToken.Get(strings.TrimSpace(tokenHash))
	if !ok {
		return nil, false
	}
	if !entry.ExpiresAt.IsZero() && time.Now().UTC().After(entry.ExpiresAt.UTC()) {
		s.runtime.apiClientAuthByToken.Remove(strings.TrimSpace(tokenHash))
		s.runtime.apiClientTokenByClient.Remove(entry.Auth.ClientID)
		return nil, false
	}
	return cloneAPIClientAuth(entry.Auth), true
}

func (s *Store) invalidateAPIClientAuthCache(clientID uuid.UUID) {
	if s == nil || s.runtime == nil || clientID == uuid.Nil {
		return
	}

	s.runtime.apiClientAuthMu.Lock()
	defer s.runtime.apiClientAuthMu.Unlock()

	if tokenHash, ok := s.runtime.apiClientTokenByClient.Peek(clientID); ok && strings.TrimSpace(tokenHash) != "" {
		s.runtime.apiClientAuthByToken.Remove(tokenHash)
	}
	s.runtime.apiClientTokenByClient.Remove(clientID)
}

func (s *Store) getCachedInstanceRole(userID uuid.UUID) (auth.InstanceRole, bool) {
	if s == nil || s.runtime == nil || userID == uuid.Nil {
		return "", false
	}
	return s.runtime.instanceRoles.Get(userID)
}

func (s *Store) cacheInstanceRole(userID uuid.UUID, role auth.InstanceRole) {
	if s == nil || s.runtime == nil || userID == uuid.Nil {
		return
	}
	s.runtime.instanceRoles.Add(userID, role)
}

func (s *Store) invalidateInstanceRole(userID uuid.UUID) {
	if s == nil || s.runtime == nil || userID == uuid.Nil {
		return
	}
	s.runtime.instanceRoles.Remove(userID)
}

func (s *Store) getCachedSiteRole(userID, siteID uuid.UUID) (auth.SiteRole, bool) {
	if s == nil || s.runtime == nil || userID == uuid.Nil || siteID == uuid.Nil {
		return "", false
	}

	s.runtime.siteRolesMu.Lock()
	defer s.runtime.siteRolesMu.Unlock()

	rolesBySite, ok := s.runtime.siteRoles.Get(userID)
	if !ok {
		return "", false
	}
	role, ok := rolesBySite[siteID]
	return role, ok
}

func (s *Store) cacheSiteRole(userID, siteID uuid.UUID, role auth.SiteRole) {
	if s == nil || s.runtime == nil || userID == uuid.Nil || siteID == uuid.Nil {
		return
	}

	s.runtime.siteRolesMu.Lock()
	defer s.runtime.siteRolesMu.Unlock()

	rolesBySite, _ := s.runtime.siteRoles.Peek(userID)
	cloned := cloneSiteRoleMap(rolesBySite)
	cloned[siteID] = role
	s.runtime.siteRoles.Add(userID, cloned)
}

func (s *Store) invalidateSiteRole(userID, siteID uuid.UUID) {
	if s == nil || s.runtime == nil || userID == uuid.Nil || siteID == uuid.Nil {
		return
	}

	s.runtime.siteRolesMu.Lock()
	defer s.runtime.siteRolesMu.Unlock()

	rolesBySite, ok := s.runtime.siteRoles.Peek(userID)
	if !ok {
		return
	}

	cloned := cloneSiteRoleMap(rolesBySite)
	delete(cloned, siteID)
	if len(cloned) == 0 {
		s.runtime.siteRoles.Remove(userID)
		return
	}
	s.runtime.siteRoles.Add(userID, cloned)
}

func (s *Store) invalidateAllSiteRolesForUser(userID uuid.UUID) {
	if s == nil || s.runtime == nil || userID == uuid.Nil {
		return
	}

	s.runtime.siteRolesMu.Lock()
	defer s.runtime.siteRolesMu.Unlock()

	s.runtime.siteRoles.Remove(userID)
}

func (s *Store) getCachedSiteTenantID(siteID uuid.UUID) (uuid.UUID, bool) {
	if s == nil || s.runtime == nil || siteID == uuid.Nil {
		return uuid.Nil, false
	}
	return s.runtime.siteTenantIDs.Get(siteID)
}

func (s *Store) cacheSiteTenantID(siteID, tenantID uuid.UUID) {
	if s == nil || s.runtime == nil || siteID == uuid.Nil || tenantID == uuid.Nil {
		return
	}
	s.runtime.siteTenantIDs.Add(siteID, tenantID)
}

func (s *Store) invalidateSiteTenantID(siteID uuid.UUID) {
	if s == nil || s.runtime == nil || siteID == uuid.Nil {
		return
	}
	s.runtime.siteTenantIDs.Remove(siteID)
}

func cloneSite(input *api.Site) *api.Site {
	if input == nil {
		return nil
	}
	cloned := *input
	return &cloned
}

func cloneCustomTrackingDomain(input *api.CustomTrackingDomain) *api.CustomTrackingDomain {
	if input == nil {
		return nil
	}
	cloned := *input
	cloned.VerifiedAt = cloneTimePtr(input.VerifiedAt)
	cloned.LastCheckedAt = cloneTimePtr(input.LastCheckedAt)
	cloned.LastTLSAskAt = cloneTimePtr(input.LastTLSAskAt)
	return &cloned
}

func cloneTimePtr(input *time.Time) *time.Time {
	if input == nil {
		return nil
	}
	cloned := *input
	return &cloned
}

func (s *Store) getCachedSiteByDomain(domain string) (*api.Site, bool) {
	if s == nil || s.runtime == nil || domain == "" {
		return nil, false
	}
	site, ok := s.runtime.sitesByDomain.Get(domain)
	if !ok {
		return nil, false
	}
	return cloneSite(site), true
}

func (s *Store) cacheSiteByDomain(domain string, site *api.Site) {
	if s == nil || s.runtime == nil || domain == "" {
		return
	}
	s.runtime.sitesByDomain.Add(domain, cloneSite(site))
}

func (s *Store) invalidateSiteDomain(domain string) {
	if s == nil || s.runtime == nil || domain == "" {
		return
	}
	s.runtime.sitesByDomain.Remove(domain)
}

func (s *Store) getCachedCustomTrackingDomain(hostname string) (*api.CustomTrackingDomain, bool) {
	if s == nil || s.runtime == nil || hostname == "" {
		return nil, false
	}
	domain, ok := s.runtime.customTrackingByHost.Get(hostname)
	if !ok {
		return nil, false
	}
	return cloneCustomTrackingDomain(domain), true
}

func (s *Store) cacheCustomTrackingDomain(hostname string, domain *api.CustomTrackingDomain) {
	if s == nil || s.runtime == nil || hostname == "" {
		return
	}
	s.runtime.customTrackingByHost.Add(hostname, cloneCustomTrackingDomain(domain))
}

func (s *Store) invalidateCustomTrackingDomainHost(hostname string) {
	if s == nil || s.runtime == nil || hostname == "" {
		return
	}
	s.runtime.customTrackingByHost.Remove(hostname)
}

func (s *Store) CacheInstanceRoleSize() int {
	if s == nil || s.runtime == nil || s.runtime.instanceRoles == nil {
		return 0
	}
	return s.runtime.instanceRoles.Len()
}

func (s *Store) CacheAPIClientAuthSize() int {
	if s == nil || s.runtime == nil || s.runtime.apiClientAuthByToken == nil {
		return 0
	}
	s.runtime.apiClientAuthMu.Lock()
	defer s.runtime.apiClientAuthMu.Unlock()
	return s.runtime.apiClientAuthByToken.Len()
}
