package devtool

import (
	"path/filepath"
	"strings"
	"testing"

	runtimeconfig "hitkeep/config"
)

func TestVerifySelfHostedReleaseArchiveRequiresConfigurationManifest(t *testing.T) {
	catalog := []byte("{\"schema_version\":\"hitkeep.config/v2\"}\n")
	example := []byte("data-path: /var/lib/hitkeep/data\n")
	manifest := runtimeconfig.RenderConfigurationReleaseManifest(catalog, example)
	archive := filepath.Join(t.TempDir(), "hitkeep_2.99.0_Linux_amd64.tar.gz")
	members := []archiveMember{
		{name: "hitkeep-linux-amd64", mode: 0o755},
		{name: "LICENSE", mode: 0o644},
		{name: "README.md", mode: 0o644},
		{name: runtimeconfig.ConfigurationCatalogFilename, mode: 0o644, data: catalog},
		{name: runtimeconfig.ConfigurationExampleFilename, mode: 0o644, data: example},
		{name: runtimeconfig.ConfigurationReleaseManifestFilename, mode: 0o644, data: manifest},
	}
	writeReleaseArchive(t, archive, members)
	if err := verifySelfHostedReleaseArchive(archive, "2.99.0", "amd64", catalog, example, manifest); err != nil {
		t.Fatalf("verifySelfHostedReleaseArchive() error = %v", err)
	}

	members = members[:len(members)-1]
	writeReleaseArchive(t, archive, members)
	err := verifySelfHostedReleaseArchive(archive, "2.99.0", "amd64", catalog, example, manifest)
	if err == nil || !strings.Contains(err.Error(), "missing required members") {
		t.Fatalf("verifySelfHostedReleaseArchive() error = %v, want missing manifest error", err)
	}
}

func TestVerifySelfHostedReleaseArchiveRejectsStaleConfigurationManifest(t *testing.T) {
	catalog := []byte("catalog\n")
	example := []byte("example\n")
	staleManifest := runtimeconfig.RenderConfigurationReleaseManifest([]byte("old catalog\n"), example)
	archive := filepath.Join(t.TempDir(), "hitkeep_2.99.0_Linux_amd64.tar.gz")
	writeReleaseArchive(t, archive, []archiveMember{
		{name: "hitkeep-linux-amd64", mode: 0o755},
		{name: "LICENSE", mode: 0o644},
		{name: "README.md", mode: 0o644},
		{name: runtimeconfig.ConfigurationCatalogFilename, mode: 0o644, data: catalog},
		{name: runtimeconfig.ConfigurationExampleFilename, mode: 0o644, data: example},
		{name: runtimeconfig.ConfigurationReleaseManifestFilename, mode: 0o644, data: staleManifest},
	})

	err := verifySelfHostedReleaseArchive(archive, "2.99.0", "amd64", catalog, example, staleManifest)
	if err == nil || !strings.Contains(err.Error(), runtimeconfig.ConfigurationCatalogFilename) {
		t.Fatalf("verifySelfHostedReleaseArchive() error = %v, want stale catalog manifest error", err)
	}
}
