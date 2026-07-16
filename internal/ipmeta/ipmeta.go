// Package ipmeta resolves coarse IP geolocation and network metadata from
// embedded IP2Location LITE-derived data.
//
// HitKeep uses the IP2Location LITE database for IP geolocation:
// https://www.ip2location.com.
package ipmeta

import (
	"net/netip"
	"sort"
)

const attribution = "HitKeep uses the IP2Location LITE database for IP geolocation. https://www.ip2location.com"

var countryLookupAsset = newFramedAsset(embeddedCountryZSTDData, "HKC2", framedCountryAsset, framedCountryCacheBytes)

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

// AssetLoadErrors fully validates the embedded metadata containers and reports
// any header, frame, checksum, or range-ordering errors.
func AssetLoadErrors() []error {
	countryErrors := countryLookupAsset.validateAll()
	cityErrors := cityLookupAsset.validateAll()
	asnErrors := asnLookupAsset.validateAll()
	errs := make([]error, 0, len(countryErrors)+len(cityErrors)+len(asnErrors))
	errs = append(errs, countryErrors...)
	errs = append(errs, cityErrors...)
	errs = append(errs, asnErrors...)
	return errs
}

// Lookup resolves coarse metadata for a public IP address.
func Lookup(ip netip.Addr) Metadata {
	return lookup(ip, "", false)
}

// LookupWithCountry resolves city and network metadata while reusing an
// already-resolved country code.
func LookupWithCountry(ip netip.Addr, countryCode string) Metadata {
	return lookup(ip, countryCode, true)
}

func lookup(ip netip.Addr, countryCode string, countryResolved bool) Metadata {
	if !ip.IsValid() || isPrivateMetadataIP(ip) {
		return Metadata{}
	}
	meta := Metadata{CountryCode: countryCode}
	if !countryResolved {
		meta = lookupCountryMetadata(ip)
	}
	if city := lookupCityMetadata(ip); !city.IsZero() {
		if meta.CountryCode == "" {
			meta.CountryCode = city.CountryCode
		}
		meta.Region = city.Region
		meta.City = city.City
	}
	if network, ok := lookupNetworkMetadata(ip); ok {
		meta.Provider = network.Provider
		meta.ASN = network.ASN
		meta.ASNOrg = network.ASNOrg
	}
	return meta
}

// LookupCountry resolves only the country code for a public IP address without
// loading city or network metadata.
func LookupCountry(ip netip.Addr) string {
	if !ip.IsValid() || isPrivateMetadataIP(ip) {
		return ""
	}
	return lookupCountryMetadata(ip).CountryCode
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
	if countryCode := countryLookupAsset.lookupCountry(ip); countryCode != "" {
		return Metadata{CountryCode: countryCode}
	}
	return lookupGeoMetadata(ip, embeddedCountryRanges)
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

func lookupNetworkMetadata(ip netip.Addr) (NetworkMetadata, bool) {
	if metadata, ok := lookupPackedASNMetadata(ip); ok {
		return metadata, true
	}
	index := sort.Search(len(embeddedNetworkRanges), func(i int) bool {
		return embeddedNetworkRanges[i].first.Compare(ip) > 0
	})
	if index == 0 {
		return NetworkMetadata{}, false
	}
	entry := embeddedNetworkRanges[index-1]
	if !entry.contains(ip) {
		return NetworkMetadata{}, false
	}
	return entry.metadata, true
}
