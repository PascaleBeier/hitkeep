// File: internal/server/geoip.go
package ingest

import (
	"iter"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"

	"github.com/phuslu/iploc"

	"hitkeep/internal/server/shared"
)

// CountryCodeExtractor provides methods to extract country codes from various sources.
// It uses a fallback strategy optimized for web analytics accuracy.
type CountryCodeExtractor struct {
	ipResolver       func(netip.Addr) string
	trustedProxyNets []netip.Prefix
}

// NewCountryCodeExtractor creates a new extractor with the default IP resolver.
func NewCountryCodeExtractor(trustedProxyNets []netip.Prefix) *CountryCodeExtractor {
	return &CountryCodeExtractor{
		ipResolver: func(ip netip.Addr) string {
			return iploc.Country(net.IP(ip.AsSlice()))
		},
		trustedProxyNets: trustedProxyNets,
	}
}

// ExtractFromRequest extracts country code from request using multiple strategies.
// Returns an empty string if no valid country code can be determined.
func (e *CountryCodeExtractor) ExtractFromRequest(r *http.Request, language *string) string {
	for code := range e.strategies(r, language) {
		if e.isValidCountryCode(code) {
			return strings.ToUpper(code)
		}
	}
	return ""
}

// strategies returns an iterator that yields country codes from various sources
func (e *CountryCodeExtractor) strategies(r *http.Request, language *string) iter.Seq[string] {
	return func(yield func(string) bool) {
		if e.shouldTrustProxyHeaders(r) {
			if code := e.fromCDNHeaders(r); code != "" {
				if !yield(code) {
					return
				}
			}

			if code := e.fromProxyHeaders(r); code != "" {
				if !yield(code) {
					return
				}
			}
		}

		if code := e.fromGeoIP(r); code != "" {
			if !yield(code) {
				return
			}
		}
	}
}

// fromCDNHeaders extracts country code from CDN-provided headers.
func (e *CountryCodeExtractor) fromCDNHeaders(r *http.Request) string {
	// Cloudflare (most common)
	if code := r.Header.Get("Cf-Ipcountry"); code != "" {
		return code
	}

	// Fastly
	if code := r.Header.Get("Fastly-Client-Ip-Country"); code != "" {
		return code
	}

	// AWS CloudFront
	if code := r.Header.Get("Cloudfront-Viewer-Country"); code != "" {
		return code
	}

	// Akamai
	if code := r.Header.Get("X-Akamai-Edgescape"); code != "" {
		// country_code=US,region_code=CA
		if parts := strings.Split(code, ","); len(parts) > 0 {
			if cc := strings.TrimPrefix(parts[0], "country_code="); cc != "" {
				return cc
			}
		}
	}

	return ""
}

// fromProxyHeaders extracts country code from reverse proxy headers.
func (e *CountryCodeExtractor) fromProxyHeaders(r *http.Request) string {
	if code := r.Header.Get("X-Country-Code"); code != "" {
		return code
	}

	if code := r.Header.Get("X-Geoip-Country-Code"); code != "" {
		return code
	}

	if code := r.Header.Get("X-Geo-Country"); code != "" {
		return code
	}

	return ""
}

// fromGeoIP extracts country code using local GeoIP database lookup.
func (e *CountryCodeExtractor) fromGeoIP(r *http.Request) string {
	userIP := shared.GetRealIP(r, e.trustedProxyNets)

	parsedIP, ok := shared.ParseAddr(userIP)
	if !ok {
		return ""
	}

	// Skip private/local IPs - they won't resolve to countries
	if e.isPrivateIP(parsedIP) {
		return ""
	}

	return e.ipResolver(parsedIP)
}

func (e *CountryCodeExtractor) shouldTrustProxyHeaders(r *http.Request) bool {
	directIP := shared.RemoteIPFromAddr(r.RemoteAddr)
	if directIP == "" {
		return false
	}

	parsedDirectIP, ok := shared.ParseAddr(directIP)
	if !ok {
		return false
	}

	return shared.IsTrustedProxy(parsedDirectIP, e.trustedProxyNets)
}

// isPrivateIP checks if an IP is in private/local address space.
func (e *CountryCodeExtractor) isPrivateIP(ip netip.Addr) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

// isValidCountryCode checks if a country code is valid.
// Rejects empty strings, "XX" (unknown), and "T1" (Tor/anonymous proxy).
func (e *CountryCodeExtractor) isValidCountryCode(code string) bool {
	if code == "" || len(code) != 2 {
		return false
	}

	// Common placeholder/invalid codes
	invalid := []string{"XX", "T1", "A1", "A2", "O1"}
	return !slices.Contains(invalid, code)
}
