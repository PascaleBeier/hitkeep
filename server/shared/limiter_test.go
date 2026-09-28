package shared

import (
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestIsTrustedProxyDefaultsToFalse(t *testing.T) {
	if IsTrustedProxy(netip.MustParseAddr("127.0.0.1"), nil) {
		t.Fatalf("expected proxy to be untrusted when trusted proxy list is empty")
	}
}

func TestGetRealIPIgnoresForwardedHeadersWithoutTrustedProxies(t *testing.T) {
	req := httptest.NewRequest("GET", "http://localhost", nil)
	req.RemoteAddr = "198.51.100.10:12345"
	req.Header.Set("CF-Connecting-IP", "203.0.113.42")
	req.Header.Set("X-Real-IP", "203.0.113.43")
	req.Header.Set("X-Forwarded-For", "203.0.113.44")

	ip := GetRealIP(req, nil)
	if ip != "198.51.100.10" {
		t.Fatalf("expected direct remote ip, got %q", ip)
	}
}

func TestGetRealIPUsesForwardedHeadersForTrustedProxy(t *testing.T) {
	proxyNet := netip.MustParsePrefix("10.0.0.0/8")

	req := httptest.NewRequest("GET", "http://localhost", nil)
	req.RemoteAddr = "10.0.0.5:44321"
	req.Header.Set("X-Forwarded-For", "203.0.113.10, 10.0.0.5")

	ip := GetRealIP(req, []netip.Prefix{proxyNet})
	if ip != "203.0.113.10" {
		t.Fatalf("expected client ip from X-Forwarded-For, got %q", ip)
	}
}

func TestGetRealIPUsesAllForwardedHeaderOccurrences(t *testing.T) {
	proxyNet := netip.MustParsePrefix("10.0.0.0/8")

	req := httptest.NewRequest("GET", "http://localhost", nil)
	req.RemoteAddr = "10.0.0.5:44321"
	req.Header.Add("X-Forwarded-For", "198.51.100.77")
	req.Header.Add("X-Forwarded-For", "203.0.113.10, 10.0.0.5")

	ip := GetRealIP(req, []netip.Prefix{proxyNet})
	if ip != "203.0.113.10" {
		t.Fatalf("expected client ip from all X-Forwarded-For values, got %q", ip)
	}
}
