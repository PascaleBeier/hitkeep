package ipmeta

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"net/netip"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/klauspost/compress/zstd"
)

const (
	framedAssetHeaderSize    = 32
	framedBlockDirectorySize = 32
	maxFramedBlockRawSize    = 2 << 20
	maxFramedMetadataRawSize = 16 << 20
	framedAssetFormatVersion = 2
	framedCountryCacheBytes  = 6 << 20
	framedCityCacheBytes     = 12 << 20
	framedASNCacheBytes      = 6 << 20
)

type framedAssetKind uint8

const (
	framedCountryAsset framedAssetKind = iota + 1
	framedLookupAsset
)

type framedAsset struct {
	compressed []byte
	magic      string
	kind       framedAssetKind
	budget     int64

	once      sync.Once
	parseErr  error
	ipv4      []*framedBlockDescriptor
	ipv6      []*framedBlockDescriptor
	allBlocks []*framedBlockDescriptor

	metadataCount      uint32
	metadataRawLen     uint32
	metadataCRC        uint32
	metadataCompressed []byte
	metadataOnce       sync.Once
	metadata           string
	metadataOffsets    []uint32

	cacheMu    sync.Mutex
	cacheBytes int64
	clock      int

	errMu sync.Mutex
	errs  []error
}

type framedBlockDescriptor struct {
	firstRaw   [16]byte
	first4     uint32
	firstHigh  uint64
	firstLow   uint64
	count      uint32
	rawLen     uint32
	crc        uint32
	compressed []byte

	decoded    atomic.Pointer[framedRangeBlock]
	referenced atomic.Bool
	failed     atomic.Bool
}

type framedRangeBlock struct {
	data []byte
}

func newFramedAsset(compressed []byte, magic string, kind framedAssetKind, budget int64) *framedAsset {
	return &framedAsset{
		compressed: compressed,
		magic:      magic,
		kind:       kind,
		budget:     budget,
	}
}

func (a *framedAsset) lookupCountry(ip netip.Addr) string {
	if a.kind != framedCountryAsset {
		return ""
	}
	payload, ok := a.lookupPayload(ip)
	if !ok || len(payload) < 2 {
		return ""
	}
	return countryCodeFromBytes(payload[0], payload[1])
}

func (a *framedAsset) lookupMetadataID(ip netip.Addr) (uint32, bool) {
	if a.kind != framedLookupAsset {
		return 0, false
	}
	payload, ok := a.lookupPayload(ip)
	if !ok || len(payload) < 4 {
		return 0, false
	}
	return binary.BigEndian.Uint32(payload[:4]), true
}

func (a *framedAsset) lookupPayload(ip netip.Addr) ([]byte, bool) {
	a.ensureParsed()
	if a.parseErr != nil {
		return nil, false
	}

	descriptors := a.ipv6
	var index int
	if ip.Is4() {
		descriptors = a.ipv4
		raw := ip.As4()
		value := binary.BigEndian.Uint32(raw[:])
		index = sort.Search(len(descriptors), func(i int) bool {
			return descriptors[i].first4 > value
		}) - 1
	} else {
		raw := ip.As16()
		high := binary.BigEndian.Uint64(raw[:8])
		low := binary.BigEndian.Uint64(raw[8:])
		index = sort.Search(len(descriptors), func(i int) bool {
			return descriptors[i].firstHigh > high ||
				(descriptors[i].firstHigh == high && descriptors[i].firstLow > low)
		}) - 1
	}
	if index < 0 {
		return nil, false
	}

	descriptor := descriptors[index]
	block := a.loadRangeBlock(descriptor, ip.Is4())
	if block == nil {
		return nil, false
	}
	count := int(descriptor.count)
	if ip.Is4() {
		raw := ip.As4()
		value := binary.BigEndian.Uint32(raw[:])
		recordIndex := sort.Search(count, func(i int) bool {
			offset := i * 4
			return binary.BigEndian.Uint32(block.data[offset:offset+4]) > value
		}) - 1
		if recordIndex < 0 {
			return nil, false
		}
		offset := count*4 + recordIndex*a.payloadSize()
		return block.data[offset : offset+a.payloadSize()], true
	}

	raw := ip.As16()
	high := binary.BigEndian.Uint64(raw[:8])
	low := binary.BigEndian.Uint64(raw[8:])
	recordIndex := sort.Search(count, func(i int) bool {
		offset := i * 16
		startHigh := binary.BigEndian.Uint64(block.data[offset : offset+8])
		startLow := binary.BigEndian.Uint64(block.data[offset+8 : offset+16])
		return startHigh > high || (startHigh == high && startLow > low)
	}) - 1
	if recordIndex < 0 {
		return nil, false
	}
	offset := count*16 + recordIndex*a.payloadSize()
	return block.data[offset : offset+a.payloadSize()], true
}

func (a *framedAsset) metadataLine(id uint32) string {
	a.loadMetadata()
	if int(id) >= len(a.metadataOffsets) {
		return ""
	}
	start := int(a.metadataOffsets[id])
	end := len(a.metadata)
	if int(id)+1 < len(a.metadataOffsets) {
		end = int(a.metadataOffsets[id+1]) - 1
	}
	return a.metadata[start:end]
}

func (a *framedAsset) loadMetadata() {
	a.ensureParsed()
	if a.parseErr != nil || a.metadataCount == 0 {
		return
	}
	a.metadataOnce.Do(func() {
		raw, err := decodeFramedData(a.metadataCompressed, int(a.metadataRawLen))
		if err != nil {
			a.recordError(fmt.Errorf("%s metadata: %w", a.magic, err))
			return
		}
		if crc32.ChecksumIEEE(raw) != a.metadataCRC {
			a.recordError(fmt.Errorf("%s metadata: checksum mismatch", a.magic))
			return
		}
		offsets := make([]uint32, 0, a.metadataCount)
		offsets = append(offsets, 0)
		for index, value := range raw {
			if value == '\n' && index+1 < len(raw) {
				//nolint:gosec // metadata blocks are capped well below math.MaxUint32.
				offsets = append(offsets, uint32(index+1))
			}
		}
		if len(offsets) != int(a.metadataCount) {
			a.recordError(fmt.Errorf("%s metadata: got %d records, want %d", a.magic, len(offsets), a.metadataCount))
			return
		}
		a.metadata = string(raw)
		a.metadataOffsets = offsets
	})
}

func (a *framedAsset) loadRangeBlock(descriptor *framedBlockDescriptor, ipv4 bool) *framedRangeBlock {
	if block := descriptor.decoded.Load(); block != nil {
		descriptor.referenced.Store(true)
		return block
	}
	if descriptor.failed.Load() {
		return nil
	}

	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()
	if block := descriptor.decoded.Load(); block != nil {
		descriptor.referenced.Store(true)
		return block
	}
	if descriptor.failed.Load() {
		return nil
	}
	raw, err := decodeFramedData(descriptor.compressed, int(descriptor.rawLen))
	if err != nil {
		descriptor.failed.Store(true)
		a.recordError(fmt.Errorf("%s range block: %w", a.magic, err))
		return nil
	}
	if crc32.ChecksumIEEE(raw) != descriptor.crc {
		descriptor.failed.Store(true)
		a.recordError(fmt.Errorf("%s range block: checksum mismatch", a.magic))
		return nil
	}
	if err := a.validateRangeBlock(raw, descriptor, ipv4); err != nil {
		descriptor.failed.Store(true)
		a.recordError(fmt.Errorf("%s range block: %w", a.magic, err))
		return nil
	}

	block := &framedRangeBlock{data: raw}
	descriptor.referenced.Store(true)
	descriptor.decoded.Store(block)
	a.cacheBytes += int64(len(raw))
	a.evictRangeBlocks(descriptor)
	return block
}

func (a *framedAsset) evictRangeBlocks(keep *framedBlockDescriptor) {
	if a.budget <= 0 || a.cacheBytes <= a.budget || len(a.allBlocks) == 0 {
		return
	}
	maxScans := len(a.allBlocks) * 4
	for scans := 0; a.cacheBytes > a.budget && scans < maxScans; scans++ {
		if a.clock >= len(a.allBlocks) {
			a.clock = 0
		}
		descriptor := a.allBlocks[a.clock]
		a.clock++
		if descriptor == keep {
			continue
		}
		block := descriptor.decoded.Load()
		if block == nil {
			continue
		}
		if descriptor.referenced.Swap(false) {
			continue
		}
		if descriptor.decoded.CompareAndSwap(block, nil) {
			a.cacheBytes -= int64(len(block.data))
		}
	}
}

func (a *framedAsset) validateRangeBlock(raw []byte, descriptor *framedBlockDescriptor, ipv4 bool) error {
	stride := a.recordStride(ipv4)
	if len(raw) != int(descriptor.count)*stride {
		return fmt.Errorf("record length %d does not match count %d and stride %d", len(raw), descriptor.count, stride)
	}
	if len(raw) == 0 {
		return fmt.Errorf("empty range block")
	}
	addressSize := 16
	if ipv4 {
		addressSize = 4
	}
	if !equalFramedAddress(raw[:addressSize], descriptor.firstRaw, ipv4) {
		return fmt.Errorf("first address does not match directory")
	}
	for index := 1; index < int(descriptor.count); index++ {
		previous := raw[(index-1)*addressSize : index*addressSize]
		current := raw[index*addressSize : (index+1)*addressSize]
		if compareFramedAddress(previous, current) >= 0 {
			return fmt.Errorf("addresses are not strictly sorted")
		}
	}
	if a.kind == framedLookupAsset {
		payloadOffset := int(descriptor.count) * addressSize
		for index := range int(descriptor.count) {
			offset := payloadOffset + index*a.payloadSize()
			id := binary.BigEndian.Uint32(raw[offset : offset+a.payloadSize()])
			if id >= a.metadataCount {
				return fmt.Errorf("metadata ID %d exceeds record count %d", id, a.metadataCount)
			}
		}
	}
	return nil
}

func (a *framedAsset) ensureParsed() {
	a.once.Do(func() {
		a.parseErr = a.parse()
		if a.parseErr != nil {
			a.recordError(a.parseErr)
		}
	})
}

func (a *framedAsset) parse() error {
	if len(a.compressed) < framedAssetHeaderSize {
		return fmt.Errorf("%s asset: truncated header", a.magic)
	}
	if string(a.compressed[:4]) != a.magic {
		return fmt.Errorf("%s asset: invalid magic", a.magic)
	}
	if version := binary.BigEndian.Uint16(a.compressed[4:6]); version != framedAssetFormatVersion {
		return fmt.Errorf("%s asset: unsupported version %d", a.magic, version)
	}
	a.metadataCount = binary.BigEndian.Uint32(a.compressed[8:12])
	a.metadataRawLen = binary.BigEndian.Uint32(a.compressed[12:16])
	metadataCompressedLen := binary.BigEndian.Uint32(a.compressed[16:20])
	a.metadataCRC = binary.BigEndian.Uint32(a.compressed[20:24])
	ipv4Count := binary.BigEndian.Uint32(a.compressed[24:28])
	ipv6Count := binary.BigEndian.Uint32(a.compressed[28:32])
	if a.metadataRawLen > maxFramedMetadataRawSize {
		return fmt.Errorf("%s asset: metadata exceeds limit", a.magic)
	}
	totalBlocks := uint64(ipv4Count) + uint64(ipv6Count)
	directoryLen := totalBlocks * framedBlockDirectorySize
	payloadOffset := uint64(framedAssetHeaderSize) + directoryLen
	if payloadOffset > uint64(len(a.compressed)) {
		return fmt.Errorf("%s asset: truncated directory", a.magic)
	}
	payloadEnd := payloadOffset + uint64(metadataCompressedLen)
	if payloadEnd > uint64(len(a.compressed)) {
		return fmt.Errorf("%s asset: truncated metadata frame", a.magic)
	}
	a.metadataCompressed = a.compressed[payloadOffset:payloadEnd]

	directoryOffset := framedAssetHeaderSize
	frameOffset := payloadEnd
	for index := range totalBlocks {
		entry := a.compressed[directoryOffset : directoryOffset+framedBlockDirectorySize]
		directoryOffset += framedBlockDirectorySize
		descriptor := &framedBlockDescriptor{}
		copy(descriptor.firstRaw[:], entry[:16])
		descriptor.first4 = binary.BigEndian.Uint32(entry[12:16])
		descriptor.firstHigh = binary.BigEndian.Uint64(entry[:8])
		descriptor.firstLow = binary.BigEndian.Uint64(entry[8:16])
		descriptor.count = binary.BigEndian.Uint32(entry[16:20])
		descriptor.rawLen = binary.BigEndian.Uint32(entry[20:24])
		compressedLen := binary.BigEndian.Uint32(entry[24:28])
		descriptor.crc = binary.BigEndian.Uint32(entry[28:32])
		ipv4 := index < uint64(ipv4Count)
		//nolint:gosec // recordStride is one of the fixed positive values 6, 8, 18, or 20.
		expectedRawLen := uint64(descriptor.count) * uint64(a.recordStride(ipv4))
		if descriptor.count == 0 || descriptor.rawLen == 0 || uint64(descriptor.rawLen) != expectedRawLen {
			return fmt.Errorf("%s asset: invalid block record length", a.magic)
		}
		if descriptor.rawLen > maxFramedBlockRawSize {
			return fmt.Errorf("%s asset: range block exceeds limit", a.magic)
		}
		nextFrameOffset := frameOffset + uint64(compressedLen)
		if compressedLen == 0 || nextFrameOffset > uint64(len(a.compressed)) {
			return fmt.Errorf("%s asset: truncated range frame", a.magic)
		}
		descriptor.compressed = a.compressed[frameOffset:nextFrameOffset]
		frameOffset = nextFrameOffset
		if ipv4 {
			a.ipv4 = append(a.ipv4, descriptor)
		} else {
			a.ipv6 = append(a.ipv6, descriptor)
		}
		a.allBlocks = append(a.allBlocks, descriptor)
	}
	if frameOffset != uint64(len(a.compressed)) {
		return fmt.Errorf("%s asset: trailing data", a.magic)
	}
	if err := validateFramedDirectory(a.ipv4, true); err != nil {
		return fmt.Errorf("%s asset: IPv4 directory: %w", a.magic, err)
	}
	if err := validateFramedDirectory(a.ipv6, false); err != nil {
		return fmt.Errorf("%s asset: IPv6 directory: %w", a.magic, err)
	}
	if a.kind == framedCountryAsset &&
		(a.metadataCount != 0 || a.metadataRawLen != 0 || metadataCompressedLen != 0 || a.metadataCRC != 0) {
		return fmt.Errorf("%s asset: country metadata must be empty", a.magic)
	}
	if a.kind == framedLookupAsset && a.metadataCount == 0 {
		return fmt.Errorf("%s asset: lookup metadata is empty", a.magic)
	}
	return nil
}

func validateFramedDirectory(descriptors []*framedBlockDescriptor, ipv4 bool) error {
	for index := 1; index < len(descriptors); index++ {
		previous := descriptors[index-1]
		current := descriptors[index]
		if ipv4 {
			if previous.first4 >= current.first4 {
				return fmt.Errorf("block starts are not strictly sorted")
			}
			continue
		}
		if previous.firstHigh > current.firstHigh ||
			(previous.firstHigh == current.firstHigh && previous.firstLow >= current.firstLow) {
			return fmt.Errorf("block starts are not strictly sorted")
		}
	}
	return nil
}

func (a *framedAsset) recordStride(ipv4 bool) int {
	addressSize := 16
	if ipv4 {
		addressSize = 4
	}
	return addressSize + a.payloadSize()
}

func (a *framedAsset) payloadSize() int {
	if a.kind == framedCountryAsset {
		return 2
	}
	return 4
}

func (a *framedAsset) errors() []error {
	a.ensureParsed()
	a.errMu.Lock()
	defer a.errMu.Unlock()
	return append([]error(nil), a.errs...)
}

func (a *framedAsset) validateAll() []error {
	a.ensureParsed()
	if a.parseErr == nil {
		a.loadMetadata()
		a.validateRangeFamily(a.ipv4, true)
		a.validateRangeFamily(a.ipv6, false)
	}
	return a.errors()
}

func (a *framedAsset) validateRangeFamily(descriptors []*framedBlockDescriptor, ipv4 bool) {
	family := "IPv6"
	addressSize := 16
	if ipv4 {
		family = "IPv4"
		addressSize = 4
	}
	for index, descriptor := range descriptors {
		block := a.loadRangeBlock(descriptor, ipv4)
		if descriptor.failed.Load() {
			a.recordError(fmt.Errorf("%s %s block %d failed validation", a.magic, family, index))
			continue
		}
		if block == nil || index+1 >= len(descriptors) {
			continue
		}
		lastOffset := (int(descriptor.count) - 1) * addressSize
		last := block.data[lastOffset : lastOffset+addressSize]
		nextFirst := descriptors[index+1].firstRaw[:]
		if ipv4 {
			nextFirst = descriptors[index+1].firstRaw[12:]
		}
		if compareFramedAddress(last, nextFirst) >= 0 {
			a.recordError(fmt.Errorf("%s %s block %d overlaps block %d", a.magic, family, index, index+1))
		}
	}
}

func (a *framedAsset) recordError(err error) {
	a.errMu.Lock()
	defer a.errMu.Unlock()
	a.errs = append(a.errs, err)
}

func decodeFramedData(compressed []byte, rawLen int) ([]byte, error) {
	framedDecoderState.Lock()
	defer framedDecoderState.Unlock()
	if framedDecoderState.decoder == nil {
		var err error
		framedDecoderState.decoder, err = zstd.NewReader(
			nil,
			zstd.WithDecoderConcurrency(1),
			zstd.WithDecoderLowmem(true),
			zstd.WithDecoderMaxMemory(maxFramedMetadataRawSize),
		)
		if err != nil {
			return nil, fmt.Errorf("create zstd decoder: %w", err)
		}
	}
	raw, err := framedDecoderState.decoder.DecodeAll(compressed, make([]byte, 0, rawLen))
	if err != nil {
		return nil, fmt.Errorf("decode zstd frame: %w", err)
	}
	if len(raw) != rawLen {
		return nil, fmt.Errorf("decoded length %d, want %d", len(raw), rawLen)
	}
	return raw, nil
}

var framedDecoderState = struct {
	sync.Mutex
	decoder *zstd.Decoder
}{}

func equalFramedAddress(raw []byte, first [16]byte, ipv4 bool) bool {
	if ipv4 {
		return binary.BigEndian.Uint32(raw) == binary.BigEndian.Uint32(first[12:])
	}
	return binary.BigEndian.Uint64(raw[:8]) == binary.BigEndian.Uint64(first[:8]) &&
		binary.BigEndian.Uint64(raw[8:]) == binary.BigEndian.Uint64(first[8:])
}

func compareFramedAddress(left, right []byte) int {
	if len(left) == 4 {
		leftValue := binary.BigEndian.Uint32(left)
		rightValue := binary.BigEndian.Uint32(right)
		if leftValue < rightValue {
			return -1
		}
		if leftValue > rightValue {
			return 1
		}
		return 0
	}
	leftHigh := binary.BigEndian.Uint64(left[:8])
	rightHigh := binary.BigEndian.Uint64(right[:8])
	if leftHigh < rightHigh {
		return -1
	}
	if leftHigh > rightHigh {
		return 1
	}
	if len(left) == 16 {
		leftLow := binary.BigEndian.Uint64(left[8:])
		rightLow := binary.BigEndian.Uint64(right[8:])
		if leftLow < rightLow {
			return -1
		}
		if leftLow > rightLow {
			return 1
		}
	}
	return 0
}

var countryCodeTable = func() [26 * 26]string {
	var table [26 * 26]string
	for first := byte('A'); first <= 'Z'; first++ {
		for second := byte('A'); second <= 'Z'; second++ {
			table[int(first-'A')*26+int(second-'A')] = string([]byte{first, second})
		}
	}
	return table
}()

func countryCodeFromBytes(first, second byte) string {
	if first < 'A' || first > 'Z' || second < 'A' || second > 'Z' {
		return ""
	}
	return countryCodeTable[int(first-'A')*26+int(second-'A')]
}
