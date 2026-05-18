package shared

import (
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestCountryCodeFromRequestTrustsCDNCountryForTrustedPeer(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.10:443"
	req.Header.Set("Cf-Ipcountry", "de")

	got := CountryCodeFromRequestWithResolver(req, []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}, func(netip.Addr) string {
		t.Fatal("IP metadata resolver should not be called when trusted CDN country is present")
		return ""
	})
	if got != "DE" {
		t.Fatalf("expected trusted CDN country DE, got %q", got)
	}
}

func TestCountryCodeFromRequestIgnoresCountryHeaderFromUntrustedPeer(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "198.51.100.10:443"
	req.Header.Set("Cf-Ipcountry", "US")

	got := CountryCodeFromRequestWithResolver(req, []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}, func(netip.Addr) string {
		return ""
	})
	if got != "" {
		t.Fatalf("expected untrusted country header to be ignored, got %q", got)
	}
}

func TestCountryCodeFromRequestFallsBackToIPMetadataForResolvedClient(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.10:443"
	req.Header.Set("X-Forwarded-For", "8.8.8.8")

	got := CountryCodeFromRequestWithResolver(req, []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}, func(ip netip.Addr) string {
		if ip.String() != "8.8.8.8" {
			t.Fatalf("expected resolver to receive client IP, got %s", ip)
		}
		return "us"
	})
	if got != "US" {
		t.Fatalf("expected IP metadata country US, got %q", got)
	}
}

func TestCountryCodeFromRequestSkipsPrivateClientIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.5:443"

	got := CountryCodeFromRequestWithResolver(req, nil, func(netip.Addr) string {
		t.Fatal("IP metadata resolver should not be called for private client IPs")
		return "US"
	})
	if got != "" {
		t.Fatalf("expected private IP to have no country, got %q", got)
	}
}

func TestNormalizeCountryCodeRequiresKnownCountry(t *testing.T) {
	tests := map[string]string{
		"de":  "DE",
		"US":  "US",
		"zz":  "",
		"eu":  "",
		"deu": "",
		"1A":  "",
	}

	for input, want := range tests {
		if got := NormalizeCountryCode(input); got != want {
			t.Fatalf("NormalizeCountryCode(%q) = %q, want %q", input, got, want)
		}
	}
}
