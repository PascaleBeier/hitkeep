package qrcodes

import (
	"net/http/httptest"
	"net/netip"
	"testing"

	"hitkeep/internal/config"
	"hitkeep/internal/ipmeta"
	"hitkeep/internal/server/shared"
)

func TestQROpenRequestContextLooksUpMetadataOnce(t *testing.T) {
	lookups := 0
	h := &handler{
		ctx: &shared.Context{Config: &config.Config{}},
		lookupIPMetadata: func(ip netip.Addr) ipmeta.Metadata {
			lookups++
			if got := ip.String(); got != "8.8.8.8" {
				t.Fatalf("expected lookup for 8.8.8.8, got %s", got)
			}
			return ipmeta.Metadata{
				CountryCode: "US",
				Region:      "California",
				City:        "Mountain View",
				Provider:    "Google LLC",
				ASN:         15169,
				ASNOrg:      "Google LLC",
			}
		},
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "8.8.8.8:443"

	userIP, countryCode, metadata := h.qrOpenRequestContext(req)

	if lookups != 1 {
		t.Fatalf("expected one metadata lookup, got %d", lookups)
	}
	if userIP != "8.8.8.8" || countryCode != "US" {
		t.Fatalf("unexpected request context: ip=%q country=%q", userIP, countryCode)
	}
	if metadata.City != "Mountain View" || metadata.ASN != 15169 {
		t.Fatalf("unexpected metadata: %+v", metadata)
	}
}
