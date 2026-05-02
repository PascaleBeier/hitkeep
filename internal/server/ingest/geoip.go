package ingest

import (
	"net/http"
	"net/netip"

	"hitkeep/internal/server/shared"
)

// CountryCodeExtractor keeps the ingest-facing API while sharing trusted-proxy
// country resolution with audit logging.
type CountryCodeExtractor struct {
	ipResolver       shared.CountryCodeResolver
	trustedProxyNets []netip.Prefix
}

func NewCountryCodeExtractor(trustedProxyNets []netip.Prefix) *CountryCodeExtractor {
	return &CountryCodeExtractor{
		ipResolver:       shared.DefaultCountryCodeResolver,
		trustedProxyNets: trustedProxyNets,
	}
}

func (e *CountryCodeExtractor) ExtractFromRequest(r *http.Request, _ *string) string {
	return shared.CountryCodeFromRequestWithResolver(r, e.trustedProxyNets, e.ipResolver)
}
