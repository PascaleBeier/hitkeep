package devtool

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

type archiveMember struct {
	name string
	mode int64
	data []byte
}

func TestVerifySelfHostedReleaseArchive(t *testing.T) {
	example := []byte("http_addr: ':8080'\n")
	archive := filepath.Join(t.TempDir(), "hitkeep_2.12.0_Linux_amd64.tar.gz")
	writeReleaseArchive(t, archive, []archiveMember{
		{name: "hitkeep-linux-amd64", mode: 0o755, data: []byte("binary")},
		{name: "LICENSE", mode: 0o644, data: []byte("license")},
		{name: "README.md", mode: 0o644, data: []byte("readme")},
		{name: "hitkeep.example.yaml", mode: 0o644, data: example},
	})

	if err := verifySelfHostedReleaseArchive(archive, "2.12.0", "amd64", example); err != nil {
		t.Fatal(err)
	}
}

func TestVerifySelfHostedReleaseArchiveRejectsCloudMember(t *testing.T) {
	example := []byte("http_addr: ':8080'\n")
	archive := filepath.Join(t.TempDir(), "hitkeep_2.12.0_Linux_amd64.tar.gz")
	writeReleaseArchive(t, archive, []archiveMember{
		{name: "hitkeep-linux-amd64", mode: 0o755, data: []byte("binary")},
		{name: "LICENSE", mode: 0o644, data: []byte("license")},
		{name: "README.md", mode: 0o644, data: []byte("readme")},
		{name: "hitkeep.example.yaml", mode: 0o644, data: example},
		{name: "hitkeep-cloud-linux-amd64", mode: 0o755, data: []byte("cloud")},
	})

	if err := verifySelfHostedReleaseArchive(archive, "2.12.0", "amd64", example); err == nil {
		t.Fatal("expected cloud member rejection")
	}
}

func TestVerifySelfHostedReleaseArchiveRejectsAlteredExample(t *testing.T) {
	example := []byte("http_addr: ':8080'\n")
	archive := filepath.Join(t.TempDir(), "hitkeep_2.12.0_Linux_arm64.tar.gz")
	writeReleaseArchive(t, archive, []archiveMember{
		{name: "hitkeep-linux-arm64", mode: 0o755, data: []byte("binary")},
		{name: "LICENSE", mode: 0o644, data: []byte("license")},
		{name: "README.md", mode: 0o644, data: []byte("readme")},
		{name: "hitkeep.example.yaml", mode: 0o644, data: []byte("changed")},
	})

	if err := verifySelfHostedReleaseArchive(archive, "2.12.0", "arm64", example); err == nil {
		t.Fatal("expected example mismatch rejection")
	}
}

func TestReleaseArchiveMetadata(t *testing.T) {
	version, arch, err := releaseArchiveMetadata("hitkeep_2.12.0_Linux_arm64.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	if version != "2.12.0" || arch != "arm64" {
		t.Fatalf("metadata = %q, %q", version, arch)
	}
}

func writeReleaseArchive(t *testing.T, path string, members []archiveMember) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, member := range members {
		if err := tarWriter.WriteHeader(&tar.Header{Name: member.name, Mode: member.mode, Size: int64(len(member.data)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(member.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
