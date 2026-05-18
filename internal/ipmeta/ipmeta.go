// Package ipmeta resolves coarse IP geolocation and network metadata from
// embedded IP2Location LITE-derived data.
//
// HitKeep uses the IP2Location LITE database for IP geolocation:
// https://www.ip2location.com.
package ipmeta

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"sort"
	"sync"
)

const attribution = "HitKeep uses the IP2Location LITE database for IP geolocation. https://www.ip2location.com"

var (
	countryPackedOnce sync.Once
	countryPacked     packedCountryAsset
	errCountryPacked  error
)

// Metadata is coarse aggregate-only IP metadata for analytics dimensions.
type Metadata struct {
	CountryCode string
	Region      string
	City        string
	Provider    string
	ASN         int
	ASNOrg      string
}

// IsZero reports whether no metadata was resolved.
func (m Metadata) IsZero() bool {
	return m.CountryCode == "" &&
		m.Region == "" &&
		m.City == "" &&
		m.Provider == "" &&
		m.ASN == 0 &&
		m.ASNOrg == ""
}

// Attribution returns the public attribution required by the IP2Location LITE
// data license.
func Attribution() string {
	return attribution
}

// AssetLoadErrors reports embedded metadata asset errors observed by the lazy
// lookup loaders. Empty generated assets are ignored so tests can inject
// fixture ranges without writing packed assets.
func AssetLoadErrors() []error {
	packedCountryData()
	cityLookupAsset.data()
	asnLookupAsset.data()

	var errs []error
	if errCountryPacked != nil {
		errs = append(errs, errCountryPacked)
	}
	if cityLookupAsset.err != nil {
		errs = append(errs, cityLookupAsset.err)
	}
	if asnLookupAsset.err != nil {
		errs = append(errs, asnLookupAsset.err)
	}
	return errs
}

// Lookup resolves coarse metadata for a public IP address.
func Lookup(ip netip.Addr) Metadata {
	if !ip.IsValid() || isPrivateMetadataIP(ip) {
		return Metadata{}
	}
	meta := lookupCountryMetadata(ip)
	if city := lookupCityMetadata(ip); !city.IsZero() {
		if meta.CountryCode == "" {
			meta.CountryCode = city.CountryCode
		}
		meta.Region = city.Region
		meta.City = city.City
	}
	if network := lookupNetworkMetadata(ip); network != nil {
		meta.Provider = network.Provider
		meta.ASN = network.ASN
		meta.ASNOrg = network.ASNOrg
	}
	return meta
}

func isPrivateMetadataIP(ip netip.Addr) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

type geoRange struct {
	first    netip.Addr
	last     netip.Addr
	metadata Metadata
}

type networkRange struct {
	first    netip.Addr
	last     netip.Addr
	metadata NetworkMetadata
}

// NetworkMetadata is coarse aggregate-only network metadata.
type NetworkMetadata struct {
	Provider string
	ASN      int
	ASNOrg   string
}

func (r geoRange) contains(ip netip.Addr) bool {
	if ip.BitLen() != r.first.BitLen() || ip.BitLen() != r.last.BitLen() {
		return false
	}
	return ip.Compare(r.first) >= 0 && ip.Compare(r.last) <= 0
}

func (r networkRange) contains(ip netip.Addr) bool {
	if ip.BitLen() != r.first.BitLen() || ip.BitLen() != r.last.BitLen() {
		return false
	}
	return ip.Compare(r.first) >= 0 && ip.Compare(r.last) <= 0
}

func lookupCountryMetadata(ip netip.Addr) Metadata {
	if countryCode := lookupPackedCountryCode(ip); countryCode != "" {
		return Metadata{CountryCode: countryCode}
	}
	return lookupGeoMetadata(ip, embeddedCountryRanges)
}

func lookupPackedCountryCode(ip netip.Addr) string {
	asset := packedCountryData()
	if ip.Is4() && len(asset.ipv4Starts) >= 4 {
		raw := ip.As4()
		value := binary.BigEndian.Uint32(raw[:])
		count := len(asset.ipv4Starts) / 4
		index := sort.Search(count, func(i int) bool {
			return binary.BigEndian.Uint32(asset.ipv4Starts[i*4:i*4+4]) > value
		})
		return packedCountryCodeAt(asset.ipv4Codes, index-1)
	}
	if !ip.Is4() && len(asset.ipv6Starts) >= 16 {
		raw := ip.As16()
		high := binary.BigEndian.Uint64(raw[:8])
		low := binary.BigEndian.Uint64(raw[8:])
		count := len(asset.ipv6Starts) / 16
		index := sort.Search(count, func(i int) bool {
			offset := i * 16
			startHigh := binary.BigEndian.Uint64(asset.ipv6Starts[offset : offset+8])
			startLow := binary.BigEndian.Uint64(asset.ipv6Starts[offset+8 : offset+16])
			return startHigh > high || (startHigh == high && startLow > low)
		})
		return packedCountryCodeAt(asset.ipv6Codes, index-1)
	}
	return ""
}

type packedCountryAsset struct {
	ipv4Starts []byte
	ipv4Codes  string
	ipv6Starts []byte
	ipv6Codes  string
}

func packedCountryData() packedCountryAsset {
	countryPackedOnce.Do(func() {
		compressed := &compressedAsset{compressed: embeddedCountryZSTDData}
		raw := compressed.bytes()
		if compressed.err != nil {
			errCountryPacked = fmt.Errorf("country asset: %w", compressed.err)
			return
		}
		asset, err := parsePackedCountryData(raw)
		if err != nil {
			errCountryPacked = fmt.Errorf("country asset: %w", err)
			return
		}
		countryPacked = asset
	})
	return countryPacked
}

func parsePackedCountryData(raw []byte) (packedCountryAsset, error) {
	if len(raw) == 0 {
		return packedCountryAsset{}, nil
	}
	if len(raw) < 12 || string(raw[:4]) != "HKCO" {
		return packedCountryAsset{}, fmt.Errorf("invalid magic or header")
	}
	ipv4Count := int(binary.BigEndian.Uint32(raw[4:8]))
	ipv6Count := int(binary.BigEndian.Uint32(raw[8:12]))
	offset := 12
	ipv4StartsLen := ipv4Count * 4
	ipv4CodesLen := ipv4Count * 2
	ipv6StartsLen := ipv6Count * 16
	ipv6CodesLen := ipv6Count * 2
	total := offset + ipv4StartsLen + ipv4CodesLen + ipv6StartsLen + ipv6CodesLen
	if ipv4Count < 0 || ipv6Count < 0 || total > len(raw) {
		return packedCountryAsset{}, fmt.Errorf("truncated asset")
	}
	asset := packedCountryAsset{}
	asset.ipv4Starts = raw[offset : offset+ipv4StartsLen]
	offset += ipv4StartsLen
	asset.ipv4Codes = string(raw[offset : offset+ipv4CodesLen])
	offset += ipv4CodesLen
	asset.ipv6Starts = raw[offset : offset+ipv6StartsLen]
	offset += ipv6StartsLen
	asset.ipv6Codes = string(raw[offset : offset+ipv6CodesLen])
	return asset, nil
}

func packedCountryCodeAt(codes string, index int) string {
	if index < 0 {
		return ""
	}
	offset := index * 2
	if offset+2 > len(codes) {
		return ""
	}
	return codes[offset : offset+2]
}

func lookupCityMetadata(ip netip.Addr) Metadata {
	if meta := lookupPackedCityMetadata(ip); !meta.IsZero() {
		return meta
	}
	return lookupGeoMetadata(ip, embeddedCityRanges)
}

func lookupGeoMetadata(ip netip.Addr, ranges []geoRange) Metadata {
	index := sort.Search(len(ranges), func(i int) bool {
		return ranges[i].first.Compare(ip) > 0
	})
	if index == 0 {
		return Metadata{}
	}
	entry := ranges[index-1]
	if entry.contains(ip) {
		return entry.metadata
	}
	return Metadata{}
}

func lookupNetworkMetadata(ip netip.Addr) *NetworkMetadata {
	if metadata := lookupPackedASNMetadata(ip); metadata != nil {
		return metadata
	}
	index := sort.Search(len(embeddedNetworkRanges), func(i int) bool {
		return embeddedNetworkRanges[i].first.Compare(ip) > 0
	})
	if index == 0 {
		return nil
	}
	entry := embeddedNetworkRanges[index-1]
	if !entry.contains(ip) {
		return nil
	}
	return &entry.metadata
}
