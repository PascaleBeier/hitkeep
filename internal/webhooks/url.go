package webhooks

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
)

type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

func ValidateDestination(ctx context.Context, rawURL string, allowDevelopmentTargets bool, resolver Resolver) (*url.URL, error) {
	destination, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || destination.Hostname() == "" {
		return nil, fmt.Errorf("invalid webhook destination URL")
	}
	if destination.Scheme != "https" && (!allowDevelopmentTargets || destination.Scheme != "http") {
		return nil, fmt.Errorf("webhook destination must use HTTPS")
	}
	if destination.User != nil {
		return nil, fmt.Errorf("webhook destination must not contain credentials")
	}
	if destination.Fragment != "" {
		return nil, fmt.Errorf("webhook destination must not contain a fragment")
	}

	if allowDevelopmentTargets {
		return destination, nil
	}
	if resolver == nil {
		resolver = net.DefaultResolver
	}

	addresses, err := resolver.LookupNetIP(ctx, "ip", destination.Hostname())
	if err != nil {
		return nil, fmt.Errorf("resolve webhook destination: %w", err)
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("webhook destination did not resolve to an IP address")
	}
	for _, address := range addresses {
		if !AddressAllowed(address) {
			return nil, fmt.Errorf("webhook destination resolves to a private or reserved address")
		}
	}

	return destination, nil
}

func AddressAllowed(address netip.Addr) bool {
	return !blockedAddress(address.Unmap())
}

var nonPublicAddressPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/96"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/32"),
	netip.MustParsePrefix("2001:2::/48"),
	netip.MustParsePrefix("2001:10::/28"),
	netip.MustParsePrefix("2001:20::/28"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("5f00::/16"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

func blockedAddress(address netip.Addr) bool {
	if !address.IsValid() || !address.IsGlobalUnicast() {
		return true
	}
	for _, prefix := range nonPublicAddressPrefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}
