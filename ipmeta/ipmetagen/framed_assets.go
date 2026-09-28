package ipmetagen

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"

	datadogzstd "github.com/DataDog/zstd"
)

const (
	framedAssetVersion       = 2
	framedAssetHeaderSize    = 32
	framedDirectoryEntrySize = 32
	framedRangeBlockTarget   = 1 << 20
	framedCompressionLevel   = 19
	framedMaxBlockRawSize    = 2 << 20
	framedMaxMetadataRawSize = 16 << 20
)

type generatedFrame struct {
	first       [16]byte
	recordCount uint32
	rawLen      uint32
	compressed  []byte
	crc         uint32
}

func buildFramedCountryAsset(
	ipv4Starts []byte,
	ipv4Codes string,
	ipv6Starts []byte,
	ipv6Codes string,
	target int,
) ([]byte, error) {
	if len(ipv4Starts)%4 != 0 || len(ipv4Codes) != (len(ipv4Starts)/4)*2 {
		return nil, fmt.Errorf("country IPv4 starts and codes do not align")
	}
	if len(ipv6Starts)%16 != 0 || len(ipv6Codes) != (len(ipv6Starts)/16)*2 {
		return nil, fmt.Errorf("country IPv6 starts and codes do not align")
	}
	ipv4Frames, err := buildGeneratedFrames(ipv4Starts, []byte(ipv4Codes), 4, 2, target, true)
	if err != nil {
		return nil, fmt.Errorf("build IPv4 frames: %w", err)
	}
	ipv6Frames, err := buildGeneratedFrames(ipv6Starts, []byte(ipv6Codes), 16, 2, target, false)
	if err != nil {
		return nil, fmt.Errorf("build IPv6 frames: %w", err)
	}
	return buildFramedAsset("HKC2", nil, 0, ipv4Frames, ipv6Frames)
}

func buildFramedLookupAsset(
	magic string,
	ipv4Starts []byte,
	ipv4Meta []byte,
	ipv6Starts []byte,
	ipv6Meta []byte,
	metadata []byte,
	metadataCount uint32,
	target int,
) ([]byte, error) {
	if len(ipv4Starts)%4 != 0 || len(ipv4Meta) != (len(ipv4Starts)/4)*4 {
		return nil, fmt.Errorf("lookup IPv4 starts and metadata do not align")
	}
	if len(ipv6Starts)%16 != 0 || len(ipv6Meta) != (len(ipv6Starts)/16)*4 {
		return nil, fmt.Errorf("lookup IPv6 starts and metadata do not align")
	}
	if metadataCount == 0 || len(metadata) == 0 {
		return nil, fmt.Errorf("lookup metadata is empty")
	}
	if got := countFramedMetadataRecords(metadata); got != metadataCount {
		return nil, fmt.Errorf("lookup metadata has %d records, want %d", got, metadataCount)
	}
	if err := validateFramedMetadataIDs(ipv4Meta, metadataCount, "IPv4"); err != nil {
		return nil, err
	}
	if err := validateFramedMetadataIDs(ipv6Meta, metadataCount, "IPv6"); err != nil {
		return nil, err
	}
	ipv4Frames, err := buildGeneratedFrames(ipv4Starts, ipv4Meta, 4, 4, target, true)
	if err != nil {
		return nil, fmt.Errorf("build IPv4 frames: %w", err)
	}
	ipv6Frames, err := buildGeneratedFrames(ipv6Starts, ipv6Meta, 16, 4, target, false)
	if err != nil {
		return nil, fmt.Errorf("build IPv6 frames: %w", err)
	}
	return buildFramedAsset(magic, metadata, metadataCount, ipv4Frames, ipv6Frames)
}

func buildFramedAsset(
	magic string,
	metadata []byte,
	metadataCount uint32,
	ipv4Frames []generatedFrame,
	ipv6Frames []generatedFrame,
) ([]byte, error) {
	if len(magic) != 4 {
		return nil, fmt.Errorf("asset magic %q must be 4 bytes", magic)
	}
	if len(metadata) > framedMaxMetadataRawSize {
		return nil, fmt.Errorf("metadata exceeds runtime limit of %d bytes", framedMaxMetadataRawSize)
	}
	for _, frames := range [][]generatedFrame{ipv4Frames, ipv6Frames} {
		for _, frame := range frames {
			if frame.rawLen > framedMaxBlockRawSize {
				return nil, fmt.Errorf("range frame exceeds runtime limit of %d bytes", framedMaxBlockRawSize)
			}
		}
	}
	var metadataCompressed []byte
	var err error
	if len(metadata) > 0 {
		metadataCompressed, err = compressFramedData(metadata)
		if err != nil {
			return nil, fmt.Errorf("compress metadata: %w", err)
		}
	}
	metadataRawLen, err := checkedUint32(len(metadata), "metadata bytes")
	if err != nil {
		return nil, err
	}
	metadataCompressedLen, err := checkedUint32(len(metadataCompressed), "compressed metadata bytes")
	if err != nil {
		return nil, err
	}
	ipv4FrameCount, err := checkedUint32(len(ipv4Frames), "IPv4 frames")
	if err != nil {
		return nil, err
	}
	ipv6FrameCount, err := checkedUint32(len(ipv6Frames), "IPv6 frames")
	if err != nil {
		return nil, err
	}

	var out bytes.Buffer
	out.WriteString(magic)
	writeUint16(&out, framedAssetVersion)
	writeUint16(&out, 0)
	writeUint32(&out, metadataCount)
	writeUint32(&out, metadataRawLen)
	writeUint32(&out, metadataCompressedLen)
	writeUint32(&out, crc32.ChecksumIEEE(metadata))
	writeUint32(&out, ipv4FrameCount)
	writeUint32(&out, ipv6FrameCount)
	for _, frame := range append(append([]generatedFrame(nil), ipv4Frames...), ipv6Frames...) {
		compressedLen, err := checkedUint32(len(frame.compressed), "compressed range frame bytes")
		if err != nil {
			return nil, err
		}
		out.Write(frame.first[:])
		writeUint32(&out, frame.recordCount)
		writeUint32(&out, frame.rawLen)
		writeUint32(&out, compressedLen)
		writeUint32(&out, frame.crc)
	}
	out.Write(metadataCompressed)
	for _, frame := range ipv4Frames {
		out.Write(frame.compressed)
	}
	for _, frame := range ipv6Frames {
		out.Write(frame.compressed)
	}
	return out.Bytes(), nil
}

func buildGeneratedFrames(starts, metadata []byte, addressSize, metadataSize, target int, ipv4 bool) ([]generatedFrame, error) {
	if addressSize <= 0 || metadataSize <= 0 || len(starts)%addressSize != 0 {
		return nil, fmt.Errorf("start data does not align to address size %d", addressSize)
	}
	recordCount := len(starts) / addressSize
	if len(metadata) != recordCount*metadataSize {
		return nil, fmt.Errorf("metadata does not align to %d records", recordCount)
	}
	if target <= 0 {
		return nil, fmt.Errorf("range block target must be positive")
	}
	if recordCount == 0 {
		return nil, nil
	}
	for index := 1; index < recordCount; index++ {
		previous := starts[(index-1)*addressSize : index*addressSize]
		current := starts[index*addressSize : (index+1)*addressSize]
		if bytes.Compare(previous, current) >= 0 {
			return nil, fmt.Errorf("range starts are not strictly sorted")
		}
	}
	stride := addressSize + metadataSize
	recordsPerFrame := max(1, target/stride)
	frames := make([]generatedFrame, 0, (recordCount+recordsPerFrame-1)/recordsPerFrame)
	for firstRecord := 0; firstRecord < recordCount; firstRecord += recordsPerFrame {
		endRecord := min(recordCount, firstRecord+recordsPerFrame)
		var raw bytes.Buffer
		raw.Grow((endRecord - firstRecord) * stride)
		raw.Write(starts[firstRecord*addressSize : endRecord*addressSize])
		raw.Write(metadata[firstRecord*metadataSize : endRecord*metadataSize])
		rawBytes := raw.Bytes()
		if len(rawBytes) > framedMaxBlockRawSize {
			return nil, fmt.Errorf("range frame exceeds runtime limit of %d bytes", framedMaxBlockRawSize)
		}
		compressed, err := compressFramedData(rawBytes)
		if err != nil {
			return nil, err
		}
		frameRecordCount, err := checkedUint32(endRecord-firstRecord, "range frame records")
		if err != nil {
			return nil, err
		}
		frameRawLen, err := checkedUint32(len(rawBytes), "range frame bytes")
		if err != nil {
			return nil, err
		}
		frame := generatedFrame{
			recordCount: frameRecordCount,
			rawLen:      frameRawLen,
			compressed:  compressed,
			crc:         crc32.ChecksumIEEE(rawBytes),
			first:       framedAddress(rawBytes[:addressSize], ipv4),
		}
		frames = append(frames, frame)
	}
	return frames, nil
}

func framedAddress(raw []byte, ipv4 bool) [16]byte {
	var address [16]byte
	if ipv4 {
		address[10] = 0xff
		address[11] = 0xff
		copy(address[12:], raw)
		return address
	}
	copy(address[:], raw)
	return address
}

func compressFramedData(raw []byte) ([]byte, error) {
	compressed, err := datadogzstd.CompressLevel(nil, raw, framedCompressionLevel)
	if err != nil {
		return nil, err
	}
	return compressed, nil
}

func countFramedMetadataRecords(metadata []byte) uint32 {
	if len(metadata) == 0 {
		return 0
	}
	count := uint32(1)
	for _, value := range metadata {
		if value == '\n' {
			count++
		}
	}
	return count
}

func validateFramedMetadataIDs(metadata []byte, count uint32, family string) error {
	for offset := 0; offset < len(metadata); offset += 4 {
		if id := binary.BigEndian.Uint32(metadata[offset : offset+4]); id >= count {
			return fmt.Errorf("%s metadata ID %d exceeds record count %d", family, id, count)
		}
	}
	return nil
}

func writeUint16(w *bytes.Buffer, value uint16) {
	var raw [2]byte
	binary.BigEndian.PutUint16(raw[:], value)
	w.Write(raw[:])
}
