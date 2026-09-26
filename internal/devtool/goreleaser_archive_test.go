package devtool

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	runtimeconfig "hitkeep/config"
)

type archiveMember struct {
	name string
	mode int64
	data []byte
}

func TestVerifySelfHostedReleaseArchive(t *testing.T) {
	catalog, example, manifest := releaseConfigurationInputs()
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			native := []byte("already-built-native-binary-" + arch)
			archive := filepath.Join(t.TempDir(), "hitkeep_2.12.0_Linux_"+arch+".tar.gz")
			if err := os.WriteFile(filepath.Join(filepath.Dir(archive), "hitkeep-linux-"+arch), native, 0o755); err != nil {
				t.Fatal(err)
			}
			members := releaseArchiveMembers(arch, catalog, example, manifest)
			members[0].data = native
			writeReleaseArchive(t, archive, members)

			if err := verifySelfHostedReleaseArchive(archive, "2.12.0", arch, catalog, example, manifest); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestVerifySelfHostedReleaseArchiveRejectsAlteredNativeBinary(t *testing.T) {
	catalog, example, manifest := releaseConfigurationInputs()
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			archive := filepath.Join(t.TempDir(), "hitkeep_2.12.0_Linux_"+arch+".tar.gz")
			if err := os.WriteFile(filepath.Join(filepath.Dir(archive), "hitkeep-linux-"+arch), []byte("already-built-native-binary"), 0o755); err != nil {
				t.Fatal(err)
			}
			members := releaseArchiveMembers(arch, catalog, example, manifest)
			members[0].data = []byte("altered-native-binary")
			writeReleaseArchive(t, archive, members)

			err := verifySelfHostedReleaseArchive(archive, "2.12.0", arch, catalog, example, manifest)
			want := "release archive hitkeep-linux-" + arch + " differs from the generated release input"
			if err == nil || err.Error() != want {
				t.Fatalf("verifySelfHostedReleaseArchive() error = %v, want %q", err, want)
			}
		})
	}
}

func TestVerifySelfHostedReleaseArchiveRejectsCloudMember(t *testing.T) {
	catalog, example, manifest := releaseConfigurationInputs()
	archive := filepath.Join(t.TempDir(), "hitkeep_2.12.0_Linux_amd64.tar.gz")
	members := append(releaseArchiveMembers("amd64", catalog, example, manifest), archiveMember{name: "hitkeep-cloud-linux-amd64", mode: 0o755, data: []byte("cloud")})
	if err := os.WriteFile(filepath.Join(filepath.Dir(archive), "hitkeep-linux-amd64"), members[0].data, 0o755); err != nil {
		t.Fatal(err)
	}
	writeReleaseArchive(t, archive, members)

	err := verifySelfHostedReleaseArchive(archive, "2.12.0", "amd64", catalog, example, manifest)
	if err == nil || err.Error() != `unexpected release archive member "hitkeep-cloud-linux-amd64"` {
		t.Fatalf("verifySelfHostedReleaseArchive() error = %v, want cloud member rejection", err)
	}
}

func TestVerifySelfHostedReleaseArchiveRejectsAlteredExample(t *testing.T) {
	catalog, example, manifest := releaseConfigurationInputs()
	archive := filepath.Join(t.TempDir(), "hitkeep_2.12.0_Linux_arm64.tar.gz")
	members := releaseArchiveMembers("arm64", catalog, example, manifest)
	if err := os.WriteFile(filepath.Join(filepath.Dir(archive), "hitkeep-linux-arm64"), members[0].data, 0o755); err != nil {
		t.Fatal(err)
	}
	members[4].data = []byte("changed")
	writeReleaseArchive(t, archive, members)

	err := verifySelfHostedReleaseArchive(archive, "2.12.0", "arm64", catalog, example, manifest)
	if err == nil || err.Error() != "release archive hitkeep.example.yaml differs from the generated release input" {
		t.Fatalf("verifySelfHostedReleaseArchive() error = %v, want example mismatch rejection", err)
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

func TestProductionArchiveRecipeIsDeterministic(t *testing.T) {
	if output, err := exec.Command("tar", "--version").CombinedOutput(); err != nil || !bytes.Contains(output, []byte("GNU tar")) {
		t.Skip("GNU tar with the production PAX controls is unavailable")
	}
	staging := filepath.Join(t.TempDir(), "staging")
	if err := os.Mkdir(staging, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, member := range []archiveMember{
		{name: "hitkeep-linux-amd64", mode: 0o755, data: []byte("native")},
		{name: "LICENSE", mode: 0o644, data: []byte("license")},
		{name: "README.md", mode: 0o644, data: []byte("readme")},
		{name: "internal/duckdbextensions/LICENSE.aws", mode: 0o644, data: []byte("aws license")},
		{name: "internal/duckdbextensions/LICENSE.excel", mode: 0o644, data: []byte("excel license")},
		{name: "internal/duckdbextensions/LICENSE.httpfs", mode: 0o644, data: []byte("httpfs license")},
		{name: "hitkeep-configuration.json", mode: 0o644, data: []byte("catalog\n")},
		{name: "hitkeep.example.yaml", mode: 0o644, data: []byte("example\n")},
		{name: "hitkeep-configuration-manifest.json", mode: 0o644, data: []byte("manifest\n")},
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(staging, member.name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(staging, member.name), member.data, os.FileMode(member.mode)); err != nil {
			t.Fatal(err)
		}
	}
	first := filepath.Join(t.TempDir(), "first.tar.gz")
	second := filepath.Join(t.TempDir(), "second.tar.gz")
	for _, destination := range []string{first, second} {
		command := exec.Command("bash", "-c", `set -euo pipefail
cd "$1"
tar --format=posix --owner=0 --group=0 --numeric-owner --mtime="@$3" --pax-option=delete=atime,delete=ctime --no-recursion -cf - hitkeep-linux-amd64 LICENSE README.md internal/duckdbextensions/LICENSE.* hitkeep-configuration.json hitkeep.example.yaml hitkeep-configuration-manifest.json | gzip -n > "$2"`, "bash", staging, destination, "1700000000")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("package archive: %v: %s", err, output)
		}
	}
	firstBytes, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := os.ReadFile(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstBytes, secondBytes) {
		t.Fatal("production archive recipe produced different bytes for equivalent inputs")
	}
	reader, err := gzip.NewReader(bytes.NewReader(firstBytes))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if !reader.ModTime.IsZero() || reader.Name != "" {
		t.Fatalf("gzip header = modtime %v, name %q", reader.ModTime, reader.Name)
	}
	tarReader := tar.NewReader(reader)
	for _, name := range []string{"hitkeep-linux-amd64", "LICENSE", "README.md", "internal/duckdbextensions/LICENSE.aws", "internal/duckdbextensions/LICENSE.excel", "internal/duckdbextensions/LICENSE.httpfs", "hitkeep-configuration.json", "hitkeep.example.yaml", "hitkeep-configuration-manifest.json"} {
		header, err := tarReader.Next()
		if err != nil {
			t.Fatal(err)
		}
		if header.Name != name || header.ModTime.Unix() != 1700000000 || header.Uid != 0 || header.Gid != 0 {
			t.Fatalf("tar header = %+v, want deterministic %q header", header, name)
		}
	}
}

func releaseConfigurationInputs() ([]byte, []byte, []byte) {
	catalog := []byte("{\"schema_version\":\"hitkeep.config/v2\"}\n")
	example := []byte("http_addr: ':8080'\n")
	return catalog, example, runtimeconfig.RenderConfigurationReleaseManifest(catalog, example)
}

func releaseArchiveMembers(arch string, catalog, example, manifest []byte) []archiveMember {
	return []archiveMember{
		{name: "hitkeep-linux-" + arch, mode: 0o755, data: []byte("binary")},
		{name: "LICENSE", mode: 0o644, data: []byte("license")},
		{name: "README.md", mode: 0o644, data: []byte("readme")},
		{name: runtimeconfig.ConfigurationCatalogFilename, mode: 0o644, data: catalog},
		{name: runtimeconfig.ConfigurationExampleFilename, mode: 0o644, data: example},
		{name: "internal/duckdbextensions/LICENSE.aws", mode: 0o644, data: []byte("license")},
		{name: "internal/duckdbextensions/LICENSE.excel", mode: 0o644, data: []byte("license")},
		{name: "internal/duckdbextensions/LICENSE.httpfs", mode: 0o644, data: []byte("license")},
		{name: runtimeconfig.ConfigurationReleaseManifestFilename, mode: 0o644, data: manifest},
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
