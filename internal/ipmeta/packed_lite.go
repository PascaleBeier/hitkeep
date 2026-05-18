package ipmeta

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/klauspost/compress/zstd"
)

var (
	cityLookupAsset = compressedLookupAsset{
		compressed: embeddedCityZSTDData,
		magic:      "HKCY",
	}

	asnLookupAsset = compressedLookupAsset{
		compressed: embeddedASNZSTDData,
		magic:      "HKAS",
	}
)

func lookupPackedCityMetadata(ip netip.Addr) Metadata {
	asset := cityLookupAsset.data()
	metaID, ok := lookupPackedMetadataID(
		ip,
		asset.ipv4Starts,
		asset.ipv4Meta,
		asset.ipv6Starts,
		asset.ipv6Meta,
	)
	if !ok {
		return Metadata{}
	}
	line := asset.metadataLine(metaID)
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

func lookupPackedASNMetadata(ip netip.Addr) *NetworkMetadata {
	asset := asnLookupAsset.data()
	metaID, ok := lookupPackedMetadataID(
		ip,
		asset.ipv4Starts,
		asset.ipv4Meta,
		asset.ipv6Starts,
		asset.ipv6Meta,
	)
	if !ok {
		return nil
	}
	line := asset.metadataLine(metaID)
	if line == "" {
		return nil
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
		return nil
	}
	return &NetworkMetadata{Provider: provider, ASN: asn, ASNOrg: provider}
}

func lookupPackedMetadataID(ip netip.Addr, ipv4Starts []byte, ipv4Meta []byte, ipv6Starts []byte, ipv6Meta []byte) (uint32, bool) {
	if ip.Is4() {
		if len(ipv4Starts) < 4 || len(ipv4Meta) < 4 {
			return 0, false
		}
		raw := ip.As4()
		value := binary.BigEndian.Uint32(raw[:])
		count := min(len(ipv4Starts)/4, len(ipv4Meta)/4)
		index := sort.Search(count, func(i int) bool {
			return binary.BigEndian.Uint32(ipv4Starts[i*4:i*4+4]) > value
		})
		if index == 0 {
			return 0, false
		}
		return binary.BigEndian.Uint32(ipv4Meta[(index-1)*4 : (index-1)*4+4]), true
	}

	if len(ipv6Starts) < 16 || len(ipv6Meta) < 4 {
		return 0, false
	}
	raw := ip.As16()
	high := binary.BigEndian.Uint64(raw[:8])
	low := binary.BigEndian.Uint64(raw[8:])
	count := min(len(ipv6Starts)/16, len(ipv6Meta)/4)
	index := sort.Search(count, func(i int) bool {
		offset := i * 16
		startHigh := binary.BigEndian.Uint64(ipv6Starts[offset : offset+8])
		startLow := binary.BigEndian.Uint64(ipv6Starts[offset+8 : offset+16])
		return startHigh > high || (startHigh == high && startLow > low)
	})
	if index == 0 {
		return 0, false
	}
	return binary.BigEndian.Uint32(ipv6Meta[(index-1)*4 : (index-1)*4+4]), true
}

func buildMetadataLineIndex(data string) []int {
	if data == "" {
		return nil
	}
	offsets := []int{0}
	for index, char := range data {
		if char == '\n' && index+1 < len(data) {
			offsets = append(offsets, index+1)
		}
	}
	return offsets
}

func metadataLine(data string, offsets []int, id uint32) string {
	if int(id) >= len(offsets) {
		return ""
	}
	start := offsets[id]
	end := len(data)
	if int(id)+1 < len(offsets) {
		end = offsets[id+1] - 1
	}
	return data[start:end]
}

func cleanPackedField(value string) string {
	value = strings.TrimSpace(value)
	if value == "-" {
		return ""
	}
	return value
}

type compressedAsset struct {
	compressed []byte
	once       sync.Once
	data       []byte
	err        error
}

func (a *compressedAsset) bytes() []byte {
	a.once.Do(func() {
		if len(a.compressed) == 0 {
			return
		}
		decoder, err := zstd.NewReader(
			nil,
			zstd.WithDecoderConcurrency(1),
			zstd.WithDecoderLowmem(true),
		)
		if err != nil {
			a.err = fmt.Errorf("create zstd decoder: %w", err)
			return
		}
		defer decoder.Close()
		data, err := decoder.DecodeAll(a.compressed, nil)
		if err != nil {
			a.err = fmt.Errorf("decode zstd asset: %w", err)
			return
		}
		a.data = data
	})
	return a.data
}

type compressedLookupAsset struct {
	compressed []byte
	magic      string
	once       sync.Once
	asset      packedLookupAsset
	err        error
}

type packedLookupAsset struct {
	ipv4Starts []byte
	ipv4Meta   []byte
	ipv6Starts []byte
	ipv6Meta   []byte
	metadata   string
	lineIndex  []int
}

func (a *compressedLookupAsset) data() packedLookupAsset {
	a.once.Do(func() {
		compressed := &compressedAsset{compressed: a.compressed}
		raw := compressed.bytes()
		if compressed.err != nil {
			a.err = fmt.Errorf("%s asset: %w", a.magic, compressed.err)
			return
		}
		asset, err := parsePackedLookupAsset(raw, a.magic)
		if err != nil {
			a.err = fmt.Errorf("%s asset: %w", a.magic, err)
			return
		}
		a.asset = asset
	})
	return a.asset
}

func parsePackedLookupAsset(raw []byte, magic string) (packedLookupAsset, error) {
	if len(raw) == 0 {
		return packedLookupAsset{}, nil
	}
	if len(raw) < 16 || string(raw[:4]) != magic {
		return packedLookupAsset{}, fmt.Errorf("invalid magic or header")
	}
	ipv4Count := int(binary.BigEndian.Uint32(raw[4:8]))
	ipv6Count := int(binary.BigEndian.Uint32(raw[8:12]))
	metaLen := int(binary.BigEndian.Uint32(raw[12:16]))
	offset := 16
	ipv4StartsLen := ipv4Count * 4
	ipv4MetaLen := ipv4Count * 4
	ipv6StartsLen := ipv6Count * 16
	ipv6MetaLen := ipv6Count * 4
	total := offset + ipv4StartsLen + ipv4MetaLen + ipv6StartsLen + ipv6MetaLen + metaLen
	if ipv4Count < 0 || ipv6Count < 0 || metaLen < 0 || total > len(raw) {
		return packedLookupAsset{}, fmt.Errorf("truncated asset")
	}
	asset := packedLookupAsset{}
	asset.ipv4Starts = raw[offset : offset+ipv4StartsLen]
	offset += ipv4StartsLen
	asset.ipv4Meta = raw[offset : offset+ipv4MetaLen]
	offset += ipv4MetaLen
	asset.ipv6Starts = raw[offset : offset+ipv6StartsLen]
	offset += ipv6StartsLen
	asset.ipv6Meta = raw[offset : offset+ipv6MetaLen]
	offset += ipv6MetaLen
	asset.metadata = string(raw[offset : offset+metaLen])
	asset.lineIndex = buildMetadataLineIndex(asset.metadata)
	return asset, nil
}

func (a packedLookupAsset) metadataLine(id uint32) string {
	return metadataLine(a.metadata, a.lineIndex, id)
}
