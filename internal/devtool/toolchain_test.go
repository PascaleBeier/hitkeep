package devtool

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNodeArchiveChecksumUsesPublishedChecksumList(t *testing.T) {
	filename := "node-v24.1.0-darwin-arm64.tar.gz"
	checksum := strings.Repeat("a", sha256.Size*2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(checksum + "  " + filename + "\n"))
	}))
	defer server.Close()

	detected, err := nodeArchiveChecksum(context.Background(), server.URL, filename)
	if err != nil {
		t.Fatal(err)
	}
	if detected != checksum {
		t.Fatalf("checksum = %q, want %q", detected, checksum)
	}
}

func TestInstallVerifiedToolchainArchiveExtractsTopLevelDirectory(t *testing.T) {
	archive := toolchainArchiveFixture(t, []tar.Header{
		{Name: "toolchain/bin", Typeflag: tar.TypeDir, Mode: 0o755},
		{Name: "toolchain/bin/tool", Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len("managed\n"))},
	}, []string{"", "managed\n"})
	raw, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write(raw)
	}))
	defer server.Close()

	target := filepath.Join(t.TempDir(), "toolchain")
	if err := installVerifiedToolchainArchive(context.Background(), server.URL, hex.EncodeToString(digest[:]), target); err != nil {
		t.Fatal(err)
	}
	installed, err := os.ReadFile(filepath.Join(target, "bin", "tool"))
	if err != nil {
		t.Fatal(err)
	}
	if string(installed) != "managed\n" {
		t.Fatalf("installed content = %q", installed)
	}
}

func TestExtractToolchainArchiveRejectsTraversal(t *testing.T) {
	archive := toolchainArchiveFixture(t, []tar.Header{
		{Name: "../escape", Typeflag: tar.TypeReg, Mode: 0o600, Size: 1},
	}, []string{"x"})
	err := extractToolchainTarGzip(archive, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "escapes destination") {
		t.Fatalf("expected traversal rejection, got %v", err)
	}
}

func toolchainArchiveFixture(t *testing.T, headers []tar.Header, contents []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "toolchain.tar.gz")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	compressed := gzip.NewWriter(file)
	archive := tar.NewWriter(compressed)
	for index := range headers {
		header := headers[index]
		if err := archive.WriteHeader(&header); err != nil {
			t.Fatal(err)
		}
		if contents[index] != "" {
			if _, err := archive.Write([]byte(contents[index])); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}
