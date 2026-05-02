package shared

import (
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"

	"github.com/phuslu/iploc"
)

type CountryCodeResolver func(netip.Addr) string

func DefaultCountryCodeResolver(ip netip.Addr) string {
	return iploc.Country(net.IP(ip.AsSlice()))
}

func CountryCodeFromRequest(r *http.Request, trustedProxies []netip.Prefix) string {
	return CountryCodeFromRequestWithResolver(r, trustedProxies, DefaultCountryCodeResolver)
}

func CountryCodeFromRequestWithResolver(r *http.Request, trustedProxies []netip.Prefix, resolver CountryCodeResolver) string {
	if resolver == nil {
		resolver = DefaultCountryCodeResolver
	}

	if shouldTrustCountryHeaders(r, trustedProxies) {
		if code := validAuditCountryCode(countryCodeFromCDNHeaders(r)); code != "" {
			return code
		}
		if code := validAuditCountryCode(countryCodeFromProxyHeaders(r)); code != "" {
			return code
		}
	}

	userIP := GetRealIP(r, trustedProxies)
	parsedIP, ok := ParseAddr(userIP)
	if !ok || isPrivateCountryIP(parsedIP) {
		return ""
	}

	return validAuditCountryCode(resolver(parsedIP))
}

func shouldTrustCountryHeaders(r *http.Request, trustedProxies []netip.Prefix) bool {
	directIP := RemoteIPFromAddr(r.RemoteAddr)
	if directIP == "" {
		return false
	}
	parsedDirectIP, ok := ParseAddr(directIP)
	if !ok {
		return false
	}
	return IsTrustedProxy(parsedDirectIP, trustedProxies)
}

func countryCodeFromCDNHeaders(r *http.Request) string {
	if code := r.Header.Get("Cf-Ipcountry"); code != "" {
		return code
	}
	if code := r.Header.Get("Fastly-Client-Ip-Country"); code != "" {
		return code
	}
	if code := r.Header.Get("Cloudfront-Viewer-Country"); code != "" {
		return code
	}
	if code := r.Header.Get("X-Akamai-Edgescape"); code != "" {
		for _, part := range strings.Split(code, ",") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "country_code=") {
				return strings.TrimPrefix(part, "country_code=")
			}
		}
	}
	return ""
}

func countryCodeFromProxyHeaders(r *http.Request) string {
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

func isPrivateCountryIP(ip netip.Addr) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

func validAuditCountryCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if len(code) != 2 {
		return ""
	}

	invalid := []string{"XX", "T1", "A1", "A2", "O1"}
	if slices.Contains(invalid, code) {
		return ""
	}
	return code
}
