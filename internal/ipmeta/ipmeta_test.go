package ipmeta

import (
	"bytes"
	"encoding/binary"
	"net/netip"
	"strings"
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
		embeddedCountryRanges = originalCountryRanges
		embeddedCityRanges = originalCityRanges
		embeddedNetworkRanges = originalNetworkRanges
		resetPackedLookupAssetsForTest()
	})

	embeddedCountryZSTDData = testCountryAsset(t, []byte{9, 9, 9, 0}, "DE", nil, "")
	resetPackedLookupAssetsForTest()
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
		embeddedCountryRanges = originalCountryRanges
		resetPackedLookupAssetsForTest()
	})

	embeddedCountryZSTDData = testCountryAsset(t, nil, "", []byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, "NL")
	resetPackedLookupAssetsForTest()
	embeddedCountryRanges = nil

	meta := Lookup(netip.MustParseAddr("2001:db8::1"))
	if meta.CountryCode != "NL" {
		t.Fatalf("expected packed IPv6 country NL, got %q", meta.CountryCode)
	}
}

func testCountryAsset(t *testing.T, ipv4Starts []byte, ipv4Codes string, ipv6Starts []byte, ipv6Codes string) []byte {
	t.Helper()
	var ipv4Frames []testFrame
	if len(ipv4Starts) > 0 {
		ipv4Frames = append(ipv4Frames, testFrame{
			first:   netip.AddrFrom4([4]byte(ipv4Starts[:4])),
			records: testCountryFrameRecords(ipv4Starts, ipv4Codes, 4),
		})
	}
	var ipv6Frames []testFrame
	if len(ipv6Starts) > 0 {
		ipv6Frames = append(ipv6Frames, testFrame{
			first:   netip.AddrFrom16([16]byte(ipv6Starts[:16])),
			records: testCountryFrameRecords(ipv6Starts, ipv6Codes, 16),
		})
	}
	return testFramedAsset(t, "HKC2", 0, nil, ipv4Frames, ipv6Frames)
}

func testCountryFrameRecords(starts []byte, codes string, addressSize int) []byte {
	var records bytes.Buffer
	for index := 0; index < len(starts)/addressSize; index++ {
		records.Write(starts[index*addressSize : (index+1)*addressSize])
		records.WriteString(codes[index*2 : (index+1)*2])
	}
	return records.Bytes()
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

func TestLookupCountryDoesNotLoadCityOrASNAssets(t *testing.T) {
	originalCityData := embeddedCityZSTDData
	originalASNData := embeddedASNZSTDData
	t.Cleanup(func() {
		embeddedCityZSTDData = originalCityData
		embeddedASNZSTDData = originalASNData
		resetPackedLookupAssetsForTest()
	})

	embeddedCityZSTDData = []byte("invalid city asset")
	embeddedASNZSTDData = []byte("invalid ASN asset")
	resetPackedLookupAssetsForTest()

	if got := LookupCountry(netip.MustParseAddr("8.8.8.8")); got != "US" {
		t.Fatalf("expected country US, got %q", got)
	}
	if cityLookupAsset.parseErr != nil {
		t.Fatalf("country-only lookup loaded city asset: %v", cityLookupAsset.parseErr)
	}
	if asnLookupAsset.parseErr != nil {
		t.Fatalf("country-only lookup loaded ASN asset: %v", asnLookupAsset.parseErr)
	}
}

func TestLookupWithCountryDoesNotLoadCountryAsset(t *testing.T) {
	originalCountryData := embeddedCountryZSTDData
	t.Cleanup(func() {
		embeddedCountryZSTDData = originalCountryData
		resetPackedLookupAssetsForTest()
	})

	embeddedCountryZSTDData = []byte("invalid country asset")
	resetPackedLookupAssetsForTest()

	meta := LookupWithCountry(netip.MustParseAddr("8.8.8.8"), "US")
	if meta.CountryCode != "US" || meta.City != "Mountain View" || meta.ASN != 15169 {
		t.Fatalf("unexpected metadata: %+v", meta)
	}
	if countryLookupAsset.parseErr != nil {
		t.Fatalf("lookup with resolved country loaded country asset: %v", countryLookupAsset.parseErr)
	}
}

func TestLookupSteadyStateDoesNotAllocate(t *testing.T) {
	resetPackedLookupAssetsForTest()
	ip := netip.MustParseAddr("8.8.8.8")
	_ = Lookup(ip)

	var metadata Metadata
	allocations := testing.AllocsPerRun(1000, func() {
		metadata = Lookup(ip)
	})
	if metadata.IsZero() {
		t.Fatal("expected public IP metadata")
	}
	if allocations != 0 {
		t.Fatalf("expected zero steady-state allocations, got %.2f", allocations)
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

func TestFramedAssetsExposeParseErrors(t *testing.T) {
	asset := newFramedAsset([]byte("not a lookup asset"), "HKY2", framedLookupAsset, 1<<20)
	if _, ok := asset.lookupMetadataID(netip.MustParseAddr("8.8.8.8")); ok {
		t.Fatal("expected invalid lookup asset to return no metadata")
	}
	errs := asset.errors()
	if len(errs) == 0 || !strings.Contains(errs[0].Error(), "truncated header") {
		t.Fatalf("expected parse error, got %v", errs)
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
