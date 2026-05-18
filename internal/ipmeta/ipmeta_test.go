package ipmeta

import (
	"bytes"
	"encoding/binary"
	"net/netip"
	"strings"
	"sync"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func TestLookupReturnsEmbeddedMetadataForPublicIP(t *testing.T) {
	meta := Lookup(netip.MustParseAddr("8.8.8.8"))

	if meta.CountryCode != "US" {
		t.Fatalf("expected country US, got %q", meta.CountryCode)
	}
	if meta.Region != "California" {
		t.Fatalf("expected region California, got %q", meta.Region)
	}
	if meta.City != "Mountain View" {
		t.Fatalf("expected city Mountain View, got %q", meta.City)
	}
	if meta.Provider != "Google LLC" {
		t.Fatalf("expected provider Google LLC, got %q", meta.Provider)
	}
	if meta.ASN != 15169 {
		t.Fatalf("expected ASN 15169, got %d", meta.ASN)
	}
	if meta.ASNOrg != "Google LLC" {
		t.Fatalf("expected ASN org Google LLC, got %q", meta.ASNOrg)
	}
}

func TestLookupKeepsDB1CountryWhenCityOverlayDiffers(t *testing.T) {
	originalCountryZSTDData := embeddedCountryZSTDData
	originalCountryRanges := embeddedCountryRanges
	originalCityRanges := embeddedCityRanges
	originalNetworkRanges := embeddedNetworkRanges
	t.Cleanup(func() {
		embeddedCountryZSTDData = originalCountryZSTDData
		countryPackedOnce = sync.Once{}
		countryPacked = packedCountryAsset{}
		errCountryPacked = nil
		embeddedCountryRanges = originalCountryRanges
		embeddedCityRanges = originalCityRanges
		embeddedNetworkRanges = originalNetworkRanges
	})

	embeddedCountryZSTDData = testCountryAsset(t, []byte{9, 9, 9, 0}, "DE", nil, "")
	countryPackedOnce = sync.Once{}
	countryPacked = packedCountryAsset{}
	errCountryPacked = nil
	embeddedCountryRanges = []geoRange{{
		first:    netip.MustParseAddr("9.9.9.0"),
		last:     netip.MustParseAddr("9.9.9.255"),
		metadata: Metadata{CountryCode: "FR"},
	}}
	embeddedCityRanges = []geoRange{{
		first:    netip.MustParseAddr("9.9.9.0"),
		last:     netip.MustParseAddr("9.9.9.255"),
		metadata: Metadata{CountryCode: "US", Region: "California", City: "Berkeley"},
	}}
	embeddedNetworkRanges = nil

	meta := Lookup(netip.MustParseAddr("9.9.9.9"))
	if meta.CountryCode != "DE" {
		t.Fatalf("expected DB1 country to remain authoritative, got %q", meta.CountryCode)
	}
	if meta.Region != "California" || meta.City != "Berkeley" {
		t.Fatalf("expected DB3 city overlay, got %+v", meta)
	}
}

func TestLookupPackedIPv6CountryMetadata(t *testing.T) {
	originalCountryZSTDData := embeddedCountryZSTDData
	originalCountryRanges := embeddedCountryRanges
	t.Cleanup(func() {
		embeddedCountryZSTDData = originalCountryZSTDData
		countryPackedOnce = sync.Once{}
		countryPacked = packedCountryAsset{}
		errCountryPacked = nil
		embeddedCountryRanges = originalCountryRanges
	})

	embeddedCountryZSTDData = testCountryAsset(t, nil, "", []byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, "NL")
	countryPackedOnce = sync.Once{}
	countryPacked = packedCountryAsset{}
	errCountryPacked = nil
	embeddedCountryRanges = nil

	meta := Lookup(netip.MustParseAddr("2001:db8::1"))
	if meta.CountryCode != "NL" {
		t.Fatalf("expected packed IPv6 country NL, got %q", meta.CountryCode)
	}
}

func testCountryAsset(t *testing.T, ipv4Starts []byte, ipv4Codes string, ipv6Starts []byte, ipv6Codes string) []byte {
	t.Helper()
	var raw bytes.Buffer
	raw.WriteString("HKCO")
	writeTestUint32(&raw, uint32(len(ipv4Starts)/4))
	writeTestUint32(&raw, uint32(len(ipv6Starts)/16))
	raw.Write(ipv4Starts)
	raw.WriteString(ipv4Codes)
	raw.Write(ipv6Starts)
	raw.WriteString(ipv6Codes)
	writer, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatalf("create country asset writer: %v", err)
	}
	compressed := writer.EncodeAll(raw.Bytes(), nil)
	if err := writer.Close(); err != nil {
		t.Fatalf("close country asset: %v", err)
	}
	return compressed
}

func writeTestUint32(w *bytes.Buffer, value uint32) {
	var raw [4]byte
	binary.BigEndian.PutUint32(raw[:], value)
	w.Write(raw[:])
}

func TestLookupSkipsPrivateAndInvalidMetadataIPs(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.0.0.10", "192.168.1.10", "169.254.10.20", "::1"} {
		t.Run(raw, func(t *testing.T) {
			if meta := Lookup(netip.MustParseAddr(raw)); !meta.IsZero() {
				t.Fatalf("expected empty metadata for %s, got %+v", raw, meta)
			}
		})
	}
}

func TestAttributionNamesIP2LocationLITE(t *testing.T) {
	attribution := Attribution()

	for _, required := range []string{"HitKeep", "IP2Location LITE", "IP geolocation", "https://www.ip2location.com"} {
		if !strings.Contains(attribution, required) {
			t.Fatalf("expected attribution to contain %q, got %q", required, attribution)
		}
	}
}

func TestAssetLoadErrorsEmptyForGeneratedAssets(t *testing.T) {
	if errs := AssetLoadErrors(); len(errs) != 0 {
		t.Fatalf("expected generated assets to load cleanly, got %v", errs)
	}
}

func TestCompressedAssetsExposeDecodeErrors(t *testing.T) {
	asset := &compressedAsset{compressed: []byte("not zstd")}

	if got := asset.bytes(); len(got) != 0 {
		t.Fatalf("expected invalid asset to decode empty, got %d bytes", len(got))
	}
	if asset.err == nil || !strings.Contains(asset.err.Error(), "decode zstd asset") {
		t.Fatalf("expected decode error, got %v", asset.err)
	}
}

func TestPackedLookupAssetsExposeParseErrors(t *testing.T) {
	asset := &compressedLookupAsset{
		compressed: testCompressedBytes(t, []byte("not a lookup asset")),
		magic:      "HKCY",
	}

	if got := asset.data(); len(got.metadata) != 0 {
		t.Fatalf("expected invalid lookup asset to parse empty, got %#v", got)
	}
	if asset.err == nil || !strings.Contains(asset.err.Error(), "invalid magic") {
		t.Fatalf("expected parse error, got %v", asset.err)
	}
}

func testCompressedBytes(t *testing.T, raw []byte) []byte {
	t.Helper()
	writer, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatalf("create zstd writer: %v", err)
	}
	compressed := writer.EncodeAll(raw, nil)
	if err := writer.Close(); err != nil {
		t.Fatalf("close zstd writer: %v", err)
	}
	return compressed
}
