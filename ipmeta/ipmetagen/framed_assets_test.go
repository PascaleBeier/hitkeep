package ipmetagen

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func TestBuildFramedCountryAssetSplitsAddressFamiliesIntoIndependentFrames(t *testing.T) {
	asset, err := buildFramedCountryAsset(
		[]byte{1, 0, 0, 0, 8, 8, 8, 0, 80, 0, 0, 0},
		"AUUSDE",
		[]byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		"DE",
		12,
	)
	if err != nil {
		t.Fatalf("build framed country asset: %v", err)
	}
	if got := string(asset[:4]); got != "HKC2" {
		t.Fatalf("expected HKC2 magic, got %q", got)
	}
	if version := binary.BigEndian.Uint16(asset[4:6]); version != 2 {
		t.Fatalf("expected version 2, got %d", version)
	}
	if ipv4Blocks := binary.BigEndian.Uint32(asset[24:28]); ipv4Blocks != 2 {
		t.Fatalf("expected 2 IPv4 blocks, got %d", ipv4Blocks)
	}
	if ipv6Blocks := binary.BigEndian.Uint32(asset[28:32]); ipv6Blocks != 1 {
		t.Fatalf("expected 1 IPv6 block, got %d", ipv6Blocks)
	}

	frames := decodeGeneratedFrames(t, asset)
	if len(frames) != 3 {
		t.Fatalf("expected 3 range frames, got %d", len(frames))
	}
	if got := frames[0]; string(got) != string([]byte{1, 0, 0, 0, 8, 8, 8, 0, 'A', 'U', 'U', 'S'}) {
		t.Fatalf("unexpected first IPv4 frame %v", got)
	}
	if got := frames[1]; string(got) != string([]byte{80, 0, 0, 0, 'D', 'E'}) {
		t.Fatalf("unexpected second IPv4 frame %v", got)
	}
}

func TestBuildFramedLookupAssetKeepsMetadataInSeparateFrame(t *testing.T) {
	metadata := []byte("US\tCalifornia\tMountain View\nDE\tBerlin\tBerlin")
	asset, err := buildFramedLookupAsset(
		"HKY2",
		[]byte{1, 0, 0, 0, 8, 8, 8, 0},
		[]byte{0, 0, 0, 1, 0, 0, 0, 0},
		nil,
		nil,
		metadata,
		2,
		8,
	)
	if err != nil {
		t.Fatalf("build framed lookup asset: %v", err)
	}
	if got := string(asset[:4]); got != "HKY2" {
		t.Fatalf("expected HKY2 magic, got %q", got)
	}
	if metadataCount := binary.BigEndian.Uint32(asset[8:12]); metadataCount != 2 {
		t.Fatalf("expected 2 metadata records, got %d", metadataCount)
	}
	if rawLen := binary.BigEndian.Uint32(asset[12:16]); rawLen != uint32(len(metadata)) {
		t.Fatalf("expected metadata raw length %d, got %d", len(metadata), rawLen)
	}

	decoder, err := zstd.NewReader(nil)
	if err != nil {
		t.Fatalf("create decoder: %v", err)
	}
	defer decoder.Close()
	directoryEnd := 32 + 2*32
	metadataCompressedLen := int(binary.BigEndian.Uint32(asset[16:20]))
	decoded, err := decoder.DecodeAll(asset[directoryEnd:directoryEnd+metadataCompressedLen], nil)
	if err != nil {
		t.Fatalf("decode metadata frame: %v", err)
	}
	if string(decoded) != string(metadata) {
		t.Fatalf("unexpected metadata frame %q", decoded)
	}

	repeated, err := buildFramedLookupAsset(
		"HKY2",
		[]byte{1, 0, 0, 0, 8, 8, 8, 0},
		[]byte{0, 0, 0, 1, 0, 0, 0, 0},
		nil,
		nil,
		metadata,
		2,
		8,
	)
	if err != nil {
		t.Fatalf("repeat framed lookup build: %v", err)
	}
	if !bytes.Equal(asset, repeated) {
		t.Fatal("framed asset generation is not deterministic")
	}
}

func TestBuildFramedAssetsRejectInvalidLookupIndexesAndOrdering(t *testing.T) {
	metadata := []byte("US\tCalifornia\tMountain View")
	if _, err := buildFramedLookupAsset(
		"HKY2",
		[]byte{8, 8, 8, 0},
		[]byte{0, 0, 0, 1},
		nil,
		nil,
		metadata,
		1,
		8,
	); err == nil {
		t.Fatal("expected out-of-range metadata ID to fail")
	}
	if _, err := buildFramedCountryAsset(
		[]byte{8, 8, 8, 0, 1, 0, 0, 0},
		"USAU",
		nil,
		"",
		12,
	); err == nil {
		t.Fatal("expected unsorted country starts to fail")
	}
}

func TestBuildFramedLookupAssetRejectsMetadataAboveRuntimeLimit(t *testing.T) {
	metadata := bytes.Repeat([]byte{'A'}, (16<<20)+1)
	if _, err := buildFramedLookupAsset(
		"HKY2",
		[]byte{8, 8, 8, 0},
		[]byte{0, 0, 0, 0},
		nil,
		nil,
		metadata,
		1,
		framedRangeBlockTarget,
	); err == nil {
		t.Fatal("expected metadata above the runtime limit to fail")
	}
}

func TestBuildFramedAssetRejectsRangeFrameAboveRuntimeLimit(t *testing.T) {
	if _, err := buildFramedAsset("HKC2", nil, 0, []generatedFrame{{
		recordCount: 1,
		rawLen:      (2 << 20) + 1,
		compressed:  []byte{1},
	}}, nil); err == nil {
		t.Fatal("expected range frame above the runtime limit to fail")
	}
}

func decodeGeneratedFrames(t *testing.T, asset []byte) [][]byte {
	t.Helper()
	ipv4Blocks := int(binary.BigEndian.Uint32(asset[24:28]))
	ipv6Blocks := int(binary.BigEndian.Uint32(asset[28:32]))
	blockCount := ipv4Blocks + ipv6Blocks
	directoryOffset := 32
	frameOffset := directoryOffset + blockCount*32 + int(binary.BigEndian.Uint32(asset[16:20]))
	decoder, err := zstd.NewReader(nil)
	if err != nil {
		t.Fatalf("create decoder: %v", err)
	}
	defer decoder.Close()

	frames := make([][]byte, 0, blockCount)
	for index := range blockCount {
		entry := asset[directoryOffset+index*32 : directoryOffset+(index+1)*32]
		compressedLen := int(binary.BigEndian.Uint32(entry[24:28]))
		raw, err := decoder.DecodeAll(asset[frameOffset:frameOffset+compressedLen], nil)
		if err != nil {
			t.Fatalf("decode frame %d: %v", index, err)
		}
		frames = append(frames, raw)
		frameOffset += compressedLen
	}
	return frames
}
