package shared

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	lru "github.com/hashicorp/golang-lru/v2/expirable"
	"golang.org/x/time/rate"
)

const (
	rateLimiterCacheSize = 10000
	rateLimiterEntryTTL  = 3 * time.Minute
)

// IPRateLimiter manages rate limiters for individual IPs.
type IPRateLimiter struct {
	ips   *lru.LRU[string, *rate.Limiter]
	rate  rate.Limit
	burst int
}

// NewIPRateLimiter creates a limiter backed by a bounded expiring LRU.
// r: requests per second
// b: burst size (allow short spikes of this many requests)
func NewIPRateLimiter(r rate.Limit, b int) *IPRateLimiter {
	return &IPRateLimiter{
		ips:   lru.NewLRU[string, *rate.Limiter](rateLimiterCacheSize, nil, rateLimiterEntryTTL),
		rate:  r,
		burst: b,
	}
}

// GetLimiter returns the rate limiter for the provided IP.
func (i *IPRateLimiter) GetLimiter(ip string) *rate.Limiter {
	if limiter, ok := i.ips.Get(ip); ok && limiter != nil {
		return limiter
	}

	limiter := rate.NewLimiter(i.rate, i.burst)
	i.ips.Add(ip, limiter)
	return limiter
}

func (i *IPRateLimiter) Len() int {
	if i == nil || i.ips == nil {
		return 0
	}
	return i.ips.Len()
}

func (i *IPRateLimiter) Stop() {
	// expirable.LRU manages its own bounded lifecycle; nothing to stop here.
}

// GetRealIP extracts the real client IP using trusted proxy configuration.
func GetRealIP(r *http.Request, trustedProxies []netip.Prefix) string {
	directIP := RemoteIPFromAddr(r.RemoteAddr)
	parsedDirectIP, ok := ParseAddr(directIP)
	if !ok {
		if directIP != "" {
			return directIP
		}
		return r.RemoteAddr
	}

	if !IsTrustedProxy(parsedDirectIP, trustedProxies) {
		return parsedDirectIP.String()
	}

	if ip, ok := firstValidHeaderIP(r.Header, "CF-Connecting-IP"); ok {
		return ip.String()
	}
	if ip, ok := firstValidHeaderIP(r.Header, "Fastly-Client-IP"); ok {
		return ip.String()
	}
	if ip, ok := firstValidHeaderIP(r.Header, "CloudFront-Viewer-Address"); ok {
		return ip.String()
	}

	if ip, ok := firstValidHeaderIP(r.Header, "X-Real-IP"); ok {
		return ip.String()
	}

	if len(trustedProxies) > 0 {
		parts := allHeaderTokens(r.Header.Values("X-Forwarded-For"))
		if trustsAllProxiesForAddr(parsedDirectIP, trustedProxies) {
			if ip, ok := firstValidIP(parts); ok {
				return ip.String()
			}
		}
		for i := len(parts) - 1; i >= 0; i-- {
			parsedIP, ok := ParseAddr(parts[i])
			if !ok {
				continue
			}
			if !isAddrInNetworks(parsedIP, trustedProxies) {
				return parsedIP.String()
			}
		}
	}

	return parsedDirectIP.String()
}

// RemoteIPFromAddr extracts the IP portion of a host:port address.
func RemoteIPFromAddr(addr string) string {
	trimmed := strings.TrimSpace(addr)
	if trimmed == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(trimmed)
	if err != nil {
		if parsed, ok := ParseAddr(trimmed); ok {
			return parsed.String()
		}
		return trimmed
	}
	if parsed, ok := ParseAddr(host); ok {
		return parsed.String()
	}
	return host
}

// ParseAddr parses an IP address value and canonicalizes it with IPv4 unmapping.
func ParseAddr(value string) (netip.Addr, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return netip.Addr{}, false
	}

	if host, _, err := net.SplitHostPort(trimmed); err == nil {
		trimmed = host
	}

	addr, err := netip.ParseAddr(trimmed)
	if err != nil {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}

// IsTrustedProxy reports if an IP belongs to any of the provided networks.
func IsTrustedProxy(ip netip.Addr, trustedProxies []netip.Prefix) bool {
	if len(trustedProxies) == 0 {
		return false
	}
	return isAddrInNetworks(ip, trustedProxies)
}

func firstValidHeaderIP(header http.Header, name string) (netip.Addr, bool) {
	values := header.Values(name)
	for i := len(values) - 1; i >= 0; i-- {
		for _, token := range reverseTokens(allHeaderTokens([]string{values[i]})) {
			if parsed, ok := ParseAddr(token); ok {
				return parsed, true
			}
		}
	}
	return netip.Addr{}, false
}

func firstValidIP(tokens []string) (netip.Addr, bool) {
	for _, token := range tokens {
		if parsed, ok := ParseAddr(token); ok {
			return parsed, true
		}
	}
	return netip.Addr{}, false
}

func allHeaderTokens(values []string) []string {
	tokens := make([]string, 0, len(values))
	for _, value := range values {
		for token := range strings.SplitSeq(value, ",") {
			token = strings.TrimSpace(token)
			if token != "" {
				tokens = append(tokens, token)
			}
		}
	}
	return tokens
}

func reverseTokens(tokens []string) []string {
	for left, right := 0, len(tokens)-1; left < right; left, right = left+1, right-1 {
		tokens[left], tokens[right] = tokens[right], tokens[left]
	}
	return tokens
}

func trustsAllProxiesForAddr(ip netip.Addr, networks []netip.Prefix) bool {
	for _, network := range networks {
		if network.Bits() != 0 {
			continue
		}
		addr := network.Addr().Unmap()
		if ip.Is4() && addr.Is4() {
			return true
		}
		if ip.Is6() && addr.Is6() {
			return true
		}
	}
	return false
}

// isAddrInNetworks checks if an IP belongs to any of the provided networks.
func isAddrInNetworks(ip netip.Addr, networks []netip.Prefix) bool {
	for _, network := range networks {
		if network.Contains(ip.Unmap()) {
			return true
		}
	}
	return false
}
