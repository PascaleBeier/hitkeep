package ipmeta

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"net/netip"
	"os"
	"sync"
	"testing"
	"time"
)

func loadedRangeBlocks(asset *framedAsset) int {
	asset.ensureParsed()
	count := 0
	for _, descriptor := range asset.allBlocks {
		if descriptor.decoded.Load() != nil {
			count++
		}
	}
	return count
}

func loadedRangeBytes(asset *framedAsset) int64 {
	asset.cacheMu.Lock()
	defer asset.cacheMu.Unlock()
	return asset.cacheBytes
}

func TestFramedCountryAssetLooksUpAcrossBlocks(t *testing.T) {
	data := testFramedAsset(t, "HKC2", 0, nil, []testFrame{
		{first: netip.MustParseAddr("1.0.0.0"), records: testCountryRecords(
			"1.0.0.0", "AU",
			"8.8.8.0", "US",
		)},
		{first: netip.MustParseAddr("9.0.0.0"), records: testCountryRecords(
			"9.0.0.0", "US",
			"80.0.0.0", "DE",
		)},
	}, nil)
	asset := newFramedAsset(data, "HKC2", framedCountryAsset, 1<<20)

	tests := map[string]string{
		"1.0.0.1":    "AU",
		"8.8.8.8":    "US",
		"9.9.9.9":    "US",
		"80.187.1.1": "DE",
	}
	for rawIP, want := range tests {
		if got := asset.lookupCountry(netip.MustParseAddr(rawIP)); got != want {
			t.Fatalf("lookupCountry(%s) = %q, want %q", rawIP, got, want)
		}
	}
	if errs := asset.errors(); len(errs) != 0 {
		t.Fatalf("unexpected asset errors: %v", errs)
	}
}

func TestFramedLookupAssetLoadsOnlySelectedRangeBlock(t *testing.T) {
	metadata := []byte("US\tCalifornia\tMountain View\nDE\tBerlin\tBerlin")
	data := testFramedAsset(t, "HKY2", 2, metadata, []testFrame{
		{first: netip.MustParseAddr("1.0.0.0"), records: testLookupRecords(
			"1.0.0.0", uint32(1),
		)},
		{first: netip.MustParseAddr("8.8.8.0"), records: testLookupRecords(
			"8.8.8.0", uint32(0),
		)},
	}, nil)
	asset := newFramedAsset(data, "HKY2", framedLookupAsset, 1<<20)

	metaID, ok := asset.lookupMetadataID(netip.MustParseAddr("8.8.8.8"))
	if !ok || metaID != 0 {
		t.Fatalf("expected metadata ID 0, got %d ok=%v", metaID, ok)
	}
	if got := asset.metadataLine(metaID); got != "US\tCalifornia\tMountain View" {
		t.Fatalf("unexpected metadata line %q", got)
	}
	if loaded := loadedRangeBlocks(asset); loaded != 1 {
		t.Fatalf("expected one loaded range block, got %d", loaded)
	}
}

func TestFramedAssetEvictsRangeBlocksToBudget(t *testing.T) {
	data := testFramedAsset(t, "HKC2", 0, nil, []testFrame{
		{first: netip.MustParseAddr("1.0.0.0"), records: testCountryRecords("1.0.0.0", "AU")},
		{first: netip.MustParseAddr("8.8.8.0"), records: testCountryRecords("8.8.8.0", "US")},
		{first: netip.MustParseAddr("80.0.0.0"), records: testCountryRecords("80.0.0.0", "DE")},
	}, nil)
	asset := newFramedAsset(data, "HKC2", framedCountryAsset, 6)

	for _, rawIP := range []string{"1.0.0.1", "8.8.8.8", "80.187.1.1"} {
		if got := asset.lookupCountry(netip.MustParseAddr(rawIP)); got == "" {
			t.Fatalf("expected metadata for %s", rawIP)
		}
	}
	if got := loadedRangeBytes(asset); got > 6 {
		t.Fatalf("loaded range bytes %d exceed budget 6", got)
	}
}

func TestFramedAssetCoordinatesConcurrentBlockLoads(t *testing.T) {
	data := testFramedAsset(t, "HKC2", 0, nil, []testFrame{{
		first: netip.MustParseAddr("8.8.8.0"),
		records: testCountryRecords(
			"8.8.8.0", "US",
			"9.0.0.0", "DE",
		),
	}}, nil)
	asset := newFramedAsset(data, "HKC2", framedCountryAsset, 1<<20)

	var wait sync.WaitGroup
	for range 32 {
		wait.Go(func() {
			if got := asset.lookupCountry(netip.MustParseAddr("8.8.8.8")); got != "US" {
				t.Errorf("expected country US, got %q", got)
			}
		})
	}
	wait.Wait()
	if loaded := loadedRangeBlocks(asset); loaded != 1 {
		t.Fatalf("expected one decoded block, got %d", loaded)
	}
}

func TestFramedAssetCoordinatesConcurrentBlockFailures(t *testing.T) {
	data := testFramedAsset(t, "HKC2", 0, nil, []testFrame{{
		first:   netip.MustParseAddr("8.8.8.0"),
		records: testCountryRecords("8.8.8.0", "US"),
	}}, nil)
	data[len(data)-1] ^= 0xff
	asset := newFramedAsset(data, "HKC2", framedCountryAsset, 1<<20)
	asset.ensureParsed()
	asset.cacheMu.Lock()

	var wait sync.WaitGroup
	ready := make(chan struct{}, 32)
	for range 32 {
		wait.Go(func() {
			ready <- struct{}{}
			_ = asset.lookupCountry(netip.MustParseAddr("8.8.8.8"))
		})
	}
	for range 32 {
		<-ready
	}
	time.Sleep(20 * time.Millisecond)
	asset.cacheMu.Unlock()
	wait.Wait()

	if errs := asset.errors(); len(errs) != 1 {
		t.Fatalf("expected one corrupt block error, got %d: %v", len(errs), errs)
	}
}

func TestFramedAssetRejectsCorruptBlock(t *testing.T) {
	data := testFramedAsset(t, "HKC2", 0, nil, []testFrame{{
		first:   netip.MustParseAddr("8.8.8.0"),
		records: testCountryRecords("8.8.8.0", "US"),
	}}, nil)
	data[len(data)-1] ^= 0xff
	asset := newFramedAsset(data, "HKC2", framedCountryAsset, 1<<20)

	if got := asset.lookupCountry(netip.MustParseAddr("8.8.8.8")); got != "" {
		t.Fatalf("expected corrupt block lookup to be empty, got %q", got)
	}
	if errs := asset.errors(); len(errs) == 0 {
		t.Fatal("expected corrupt block error")
	}
}

func TestFramedLookupAssetRejectsMetadataIDOutsideTable(t *testing.T) {
	data := testFramedAsset(t, "HKY2", 1, []byte("US\tCalifornia\tMountain View"), []testFrame{{
		first:   netip.MustParseAddr("8.8.8.0"),
		records: testLookupRecords("8.8.8.0", uint32(1)),
	}}, nil)
	asset := newFramedAsset(data, "HKY2", framedLookupAsset, 1<<20)

	if errs := asset.validateAll(); len(errs) == 0 {
		t.Fatal("expected out-of-range metadata ID to fail validation")
	}
}

func TestFramedCountryAssetRejectsMetadataFrame(t *testing.T) {
	data := testFramedAsset(t, "HKC2", 0, []byte("hidden metadata"), []testFrame{{
		first:   netip.MustParseAddr("8.8.8.0"),
		records: testCountryRecords("8.8.8.0", "US"),
	}}, nil)
	asset := newFramedAsset(data, "HKC2", framedCountryAsset, 1<<20)

	if errs := asset.validateAll(); len(errs) == 0 {
		t.Fatal("expected country metadata frame to fail validation")
	}
}

func TestFramedAssetRejectsCrossBlockOrderingOverlap(t *testing.T) {
	data := testFramedAsset(t, "HKC2", 0, nil, []testFrame{
		{
			first: netip.MustParseAddr("1.0.0.0"),
			records: testCountryRecords(
				"1.0.0.0", "AU",
				"100.0.0.0", "US",
			),
		},
		{
			first:   netip.MustParseAddr("50.0.0.0"),
			records: testCountryRecords("50.0.0.0", "DE"),
		},
	}, nil)
	asset := newFramedAsset(data, "HKC2", framedCountryAsset, 1<<20)

	if errs := asset.validateAll(); len(errs) == 0 {
		t.Fatal("expected cross-block ordering overlap to fail validation")
	}
}

func TestGeneratedFramedAssetsStayWithinDecodedCacheBudget(t *testing.T) {
	resetPackedLookupAssetsForTest()
	assets := []*framedAsset{countryLookupAsset, cityLookupAsset, asnLookupAsset}
	for _, asset := range assets {
		asset.ensureParsed()
		asset.loadMetadata()
		for _, descriptor := range asset.ipv4 {
			if block := asset.loadRangeBlock(descriptor, true); block == nil {
				t.Fatalf("%s IPv4 block failed to load: %v", asset.magic, asset.errors())
			}
		}
		for _, descriptor := range asset.ipv6 {
			if block := asset.loadRangeBlock(descriptor, false); block == nil {
				t.Fatalf("%s IPv6 block failed to load: %v", asset.magic, asset.errors())
			}
		}
	}

	rangeBytes := loadedRangeBytes(countryLookupAsset) +
		loadedRangeBytes(cityLookupAsset) +
		loadedRangeBytes(asnLookupAsset)
	metadataBytes := int64(len(cityLookupAsset.metadata)+len(asnLookupAsset.metadata)) +
		int64(4*(len(cityLookupAsset.metadataOffsets)+len(asnLookupAsset.metadataOffsets)))
	total := rangeBytes + metadataBytes
	t.Logf("decoded cache bytes: ranges=%d metadata=%d total=%d", rangeBytes, metadataBytes, total)
	if total > 32<<20 {
		t.Fatalf("decoded cache uses %d bytes, exceeds 32 MiB", total)
	}
}

func TestGeneratedFramedAssetsStayWithinEmbeddedSizeBudget(t *testing.T) {
	var total int64
	for _, path := range []string{
		"data_country.hk.zst",
		"data_city.hk.zst",
		"data_asn.hk.zst",
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		total += info.Size()
	}
	t.Logf("embedded IP metadata bytes: %d", total)
	if total > 18_000_000 {
		t.Fatalf("embedded IP metadata uses %d bytes, exceeds 18,000,000", total)
	}
}

type testFrame struct {
	first   netip.Addr
	records []byte
}

func testFramedAsset(t *testing.T, magic string, metadataCount uint32, metadata []byte, ipv4, ipv6 []testFrame) []byte {
	t.Helper()
	var metadataFrame []byte
	if metadata != nil {
		metadataFrame = testCompressedBytes(t, metadata)
	}
	frames := append(append([]testFrame(nil), ipv4...), ipv6...)
	compressedFrames := make([][]byte, len(frames))

	var directory bytes.Buffer
	for index, frame := range frames {
		compressedFrames[index] = testCompressedBytes(t, frame.records)
		first := frame.first.As16()
		directory.Write(first[:])
		stride := 20
		if frame.first.Is4() {
			stride = 8
		}
		if magic == "HKC2" {
			stride -= 2
		}
		writeTestUint32(&directory, uint32(len(frame.records)/stride))
		writeTestUint32(&directory, uint32(len(frame.records)))
		writeTestUint32(&directory, uint32(len(compressedFrames[index])))
		writeTestUint32(&directory, crc32.ChecksumIEEE(frame.records))
	}

	var out bytes.Buffer
	out.WriteString(magic)
	_ = binary.Write(&out, binary.BigEndian, uint16(2))
	_ = binary.Write(&out, binary.BigEndian, uint16(0))
	writeTestUint32(&out, metadataCount)
	writeTestUint32(&out, uint32(len(metadata)))
	writeTestUint32(&out, uint32(len(metadataFrame)))
	writeTestUint32(&out, crc32.ChecksumIEEE(metadata))
	writeTestUint32(&out, uint32(len(ipv4)))
	writeTestUint32(&out, uint32(len(ipv6)))
	out.Write(directory.Bytes())
	out.Write(metadataFrame)
	for _, frame := range compressedFrames {
		out.Write(frame)
	}
	return out.Bytes()
}

func testCountryRecords(values ...any) []byte {
	var starts bytes.Buffer
	var codes bytes.Buffer
	for index := 0; index < len(values); index += 2 {
		ip := netip.MustParseAddr(values[index].(string)).As4()
		starts.Write(ip[:])
		codes.WriteString(values[index+1].(string))
	}
	var out bytes.Buffer
	out.Write(starts.Bytes())
	out.Write(codes.Bytes())
	return out.Bytes()
}

func testLookupRecords(values ...any) []byte {
	var starts bytes.Buffer
	var metadata bytes.Buffer
	for index := 0; index < len(values); index += 2 {
		ip := netip.MustParseAddr(values[index].(string)).As4()
		starts.Write(ip[:])
		writeTestUint32(&metadata, values[index+1].(uint32))
	}
	var out bytes.Buffer
	out.Write(starts.Bytes())
	out.Write(metadata.Bytes())
	return out.Bytes()
}
