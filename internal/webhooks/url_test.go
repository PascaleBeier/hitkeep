package webhooks

import (
	"context"
	"net/netip"
	"testing"
)

type staticResolver map[string][]netip.Addr

func (r staticResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	return r[host], nil
}

func TestValidateDestinationRequiresHTTPSAndPublicAddresses(t *testing.T) {
	t.Parallel()

	resolver := staticResolver{
		"hooks.example.com": {netip.MustParseAddr("93.184.216.34")},
		"internal.example":  {netip.MustParseAddr("10.0.0.5")},
		"mixed.example":     {netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr("127.0.0.1")},
	}

	if _, err := ValidateDestination(context.Background(), "https://hooks.example.com/hitkeep", false, resolver); err != nil {
		t.Fatalf("expected public HTTPS destination: %v", err)
	}

	for _, rawURL := range []string{
		"http://hooks.example.com/hitkeep",
		"https://127.0.0.1/hitkeep",
		"https://169.254.169.254/latest/meta-data",
		"https://internal.example/hitkeep",
		"https://mixed.example/hitkeep",
		"https://user:pass@hooks.example.com/hitkeep",
	} {
		if _, err := ValidateDestination(context.Background(), rawURL, false, resolver); err == nil {
			t.Errorf("expected %q to be rejected", rawURL)
		}
	}
}

func TestValidateDestinationAllowsExplicitDevelopmentTargets(t *testing.T) {
	t.Parallel()

	resolver := staticResolver{"localhost": {netip.MustParseAddr("127.0.0.1")}}
	got, err := ValidateDestination(context.Background(), "http://localhost:9000/hook", true, resolver)
	if err != nil {
		t.Fatalf("expected development destination: %v", err)
	}
	if got.String() != "http://localhost:9000/hook" {
		t.Fatalf("unexpected normalized URL %q", got)
	}
}

func TestAddressAllowedRejectsSpecialPurposeNetworks(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"0.0.0.0", "100.64.0.1", "127.0.0.1", "169.254.169.254", "192.0.2.1",
		"198.18.0.1", "198.51.100.1", "203.0.113.1", "224.0.0.1", "240.0.0.1",
		"::", "::1", "::ffff:100.64.0.1", "64:ff9b:1::1", "100::1", "2001:2::1",
		"2001:db8::1", "2001:20::1", "2002::1", "fc00::1", "fe80::1", "ff00::1",
	} {
		address := netip.MustParseAddr(raw)
		if AddressAllowed(address) {
			t.Errorf("expected special-purpose address %s to be rejected", address)
		}
	}

	for _, raw := range []string{"1.1.1.1", "8.8.8.8", "93.184.216.34", "2606:4700:4700::1111"} {
		address := netip.MustParseAddr(raw)
		if !AddressAllowed(address) {
			t.Errorf("expected public address %s to be allowed", address)
		}
	}
}

var _ Resolver = staticResolver{}
