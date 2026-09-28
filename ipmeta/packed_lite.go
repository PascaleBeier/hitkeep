package ipmeta

import (
	"net/netip"
	"strconv"
	"strings"
)

var (
	cityLookupAsset = newFramedAsset(embeddedCityZSTDData, "HKY2", framedLookupAsset, framedCityCacheBytes)
	asnLookupAsset  = newFramedAsset(embeddedASNZSTDData, "HKA2", framedLookupAsset, framedASNCacheBytes)
)

func lookupPackedCityMetadata(ip netip.Addr) Metadata {
	metaID, ok := cityLookupAsset.lookupMetadataID(ip)
	if !ok {
		return Metadata{}
	}
	line := cityLookupAsset.metadataLine(metaID)
	if line == "" {
		return Metadata{}
	}
	countryCode, rest, _ := strings.Cut(line, "\t")
	region, city, _ := strings.Cut(rest, "\t")
	return Metadata{
		CountryCode: cleanPackedField(countryCode),
		Region:      cleanPackedField(region),
		City:        cleanPackedField(city),
	}
}

func lookupPackedASNMetadata(ip netip.Addr) (NetworkMetadata, bool) {
	metaID, ok := asnLookupAsset.lookupMetadataID(ip)
	if !ok {
		return NetworkMetadata{}, false
	}
	line := asnLookupAsset.metadataLine(metaID)
	if line == "" {
		return NetworkMetadata{}, false
	}
	asnRaw, provider, _ := strings.Cut(line, "\t")
	provider = cleanPackedField(provider)
	asn := 0
	if asnRaw = cleanPackedField(asnRaw); asnRaw != "" {
		if parsed, err := strconv.Atoi(asnRaw); err == nil {
			asn = parsed
		}
	}
	if provider == "" && asn == 0 {
		return NetworkMetadata{}, false
	}
	return NetworkMetadata{Provider: provider, ASN: asn, ASNOrg: provider}, true
}

func cleanPackedField(value string) string {
	value = strings.TrimSpace(value)
	if value == "-" {
		return ""
	}
	return value
}
