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
	if destination.Scheme != "https" && !(allowDevelopmentTargets && destination.Scheme == "http") {
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

func blockedAddress(address netip.Addr) bool {
	return !address.IsValid() ||
		!address.IsGlobalUnicast() ||
		address.IsPrivate() ||
		address.IsLoopback() ||
		address.IsLinkLocalUnicast() ||
		address.IsLinkLocalMulticast() ||
		address.IsMulticast() ||
		address.IsUnspecified()
}
