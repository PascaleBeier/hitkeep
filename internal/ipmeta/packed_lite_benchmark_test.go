package ipmeta

import (
	"net/netip"
	"testing"
)

var benchmarkMetadata Metadata
var benchmarkCountryCode string

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

func BenchmarkLookupCountryPackedSteadyIPv4(b *testing.B) {
	resetPackedLookupAssetsForTest()
	ip := netip.MustParseAddr("80.187.73.186")
	benchmarkCountryCode = LookupCountry(ip)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkCountryCode = LookupCountry(ip)
	}
}

func BenchmarkLookupWithCountryPackedSteadyIPv4(b *testing.B) {
	resetPackedLookupAssetsForTest()
	ip := netip.MustParseAddr("80.187.73.186")
	benchmarkMetadata = LookupWithCountry(ip, "DE")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkMetadata = LookupWithCountry(ip, "DE")
	}
}

func BenchmarkLookupCountryThenPackedSteadyIPv4(b *testing.B) {
	resetPackedLookupAssetsForTest()
	ip := netip.MustParseAddr("80.187.73.186")
	benchmarkCountryCode = LookupCountry(ip)
	benchmarkMetadata = Lookup(ip)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkCountryCode = LookupCountry(ip)
		benchmarkMetadata = Lookup(ip)
	}
}

func BenchmarkLookupCountryThenWithCountryPackedSteadyIPv4(b *testing.B) {
	resetPackedLookupAssetsForTest()
	ip := netip.MustParseAddr("80.187.73.186")
	benchmarkCountryCode = LookupCountry(ip)
	benchmarkMetadata = LookupWithCountry(ip, benchmarkCountryCode)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkCountryCode = LookupCountry(ip)
		benchmarkMetadata = LookupWithCountry(ip, benchmarkCountryCode)
	}
}

func BenchmarkLookupTwicePackedSteadyIPv4(b *testing.B) {
	resetPackedLookupAssetsForTest()
	ip := netip.MustParseAddr("80.187.73.186")
	benchmarkCountryCode = Lookup(ip).CountryCode
	benchmarkMetadata = Lookup(ip)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkCountryCode = Lookup(ip).CountryCode
		benchmarkMetadata = Lookup(ip)
	}
}

func BenchmarkLookupCountryPackedColdIPv4(b *testing.B) {
	ip := netip.MustParseAddr("80.187.73.186")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resetPackedLookupAssetsForTest()
		benchmarkCountryCode = LookupCountry(ip)
	}
}

func BenchmarkLookupWithCountryPackedColdIPv4(b *testing.B) {
	ip := netip.MustParseAddr("80.187.73.186")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resetPackedLookupAssetsForTest()
		benchmarkMetadata = LookupWithCountry(ip, "DE")
	}
}

func BenchmarkLookupCountryThenPackedColdIPv4(b *testing.B) {
	ip := netip.MustParseAddr("80.187.73.186")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resetPackedLookupAssetsForTest()
		benchmarkCountryCode = LookupCountry(ip)
		benchmarkMetadata = Lookup(ip)
	}
}

func BenchmarkLookupCountryThenWithCountryPackedColdIPv4(b *testing.B) {
	ip := netip.MustParseAddr("80.187.73.186")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resetPackedLookupAssetsForTest()
		benchmarkCountryCode = LookupCountry(ip)
		benchmarkMetadata = LookupWithCountry(ip, benchmarkCountryCode)
	}
}

func BenchmarkLookupTwicePackedColdIPv4(b *testing.B) {
	ip := netip.MustParseAddr("80.187.73.186")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resetPackedLookupAssetsForTest()
		benchmarkCountryCode = Lookup(ip).CountryCode
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
	countryLookupAsset = newFramedAsset(embeddedCountryZSTDData, "HKC2", framedCountryAsset, framedCountryCacheBytes)
	cityLookupAsset = newFramedAsset(embeddedCityZSTDData, "HKY2", framedLookupAsset, framedCityCacheBytes)
	asnLookupAsset = newFramedAsset(embeddedASNZSTDData, "HKA2", framedLookupAsset, framedASNCacheBytes)
	framedDecoderState.Lock()
	if framedDecoderState.decoder != nil {
		framedDecoderState.decoder.Close()
		framedDecoderState.decoder = nil
	}
	framedDecoderState.Unlock()
}
