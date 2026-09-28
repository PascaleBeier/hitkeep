package blocking

import (
	"fmt"
	"net/netip"
	"strings"
)

// NormalizeCIDR accepts a plain IP or CIDR string and returns canonical CIDR form.
func NormalizeCIDR(input string) (string, netip.Prefix, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", netip.Prefix{}, fmt.Errorf("cidr value is required")
	}

	if strings.Contains(trimmed, "/") {
		ipNet, err := netip.ParsePrefix(trimmed)
		if err != nil {
			return "", netip.Prefix{}, fmt.Errorf("invalid cidr %q", trimmed)
		}
		ipNet = ipNet.Masked()
		return ipNet.String(), ipNet, nil
	}

	ip, err := netip.ParseAddr(trimmed)
	if err != nil {
		return "", netip.Prefix{}, fmt.Errorf("invalid ip %q", trimmed)
	}

	ip = ip.Unmap()
	bits := 128
	if ip.Is4() {
		bits = 32
	}
	ipNet := netip.PrefixFrom(ip, bits)
	return ipNet.String(), ipNet, nil
}
