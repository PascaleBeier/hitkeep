package ipmeta

import (
	"net/netip"
	"sync"
	"testing"
)

var benchmarkMetadata Metadata

func BenchmarkLookupPackedSteadyIPv4(b *testing.B) {
	resetPackedLookupAssetsForTest()
	ip := netip.MustParseAddr("80.187.73.186")
	benchmarkMetadata = Lookup(ip)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkMetadata = Lookup(ip)
	}
}

func BenchmarkLookupPackedSteadyIPv6(b *testing.B) {
	resetPackedLookupAssetsForTest()
	ip := netip.MustParseAddr("2a01:599:216:6f76:ac5f:34c6:5e1f:5419")
	benchmarkMetadata = Lookup(ip)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkMetadata = Lookup(ip)
	}
}

func BenchmarkLookupPackedColdIPv4(b *testing.B) {
	ip := netip.MustParseAddr("80.187.73.186")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resetPackedLookupAssetsForTest()
		benchmarkMetadata = Lookup(ip)
	}
}

func BenchmarkLookupPackedColdIPv6(b *testing.B) {
	ip := netip.MustParseAddr("2a01:599:216:6f76:ac5f:34c6:5e1f:5419")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resetPackedLookupAssetsForTest()
		benchmarkMetadata = Lookup(ip)
	}
}

func resetPackedLookupAssetsForTest() {
	countryPackedOnce = sync.Once{}
	countryPacked = packedCountryAsset{}
	cityLookupAsset.once = sync.Once{}
	cityLookupAsset.asset = packedLookupAsset{}
	asnLookupAsset.once = sync.Once{}
	asnLookupAsset.asset = packedLookupAsset{}
}
