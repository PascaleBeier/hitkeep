package devtool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	runtimeconfig "hitkeep/internal/config"
)

func TestRepositoryDevelopmentDocs(t *testing.T) {
	root := repositoryRoot(t)
	if err := ValidateDevelopmentDocs(root); err != nil {
		t.Fatal(err)
	}
}

func TestValidateReleaseMetadata(t *testing.T) {
	t.Run("current", func(t *testing.T) {
		root := releaseMetadataFixture(t)
		if err := validateReleaseMetadata(root); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("mismatched version", func(t *testing.T) {
		root := releaseMetadataFixture(t)
		writeFixtureFile(t, root, "server.json", `{"version":"2.11.0"}`)
		err := validateReleaseMetadata(root)
		if err == nil || !strings.Contains(err.Error(), `server.json $.version has version "2.11.0"; want "2.12.0"`) {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("invalid manifest version", func(t *testing.T) {
		root := releaseMetadataFixture(t)
		writeFixtureFile(t, root, ".release-please-manifest.json", `{ ".": "v2.12.0" }`)
		err := validateReleaseMetadata(root)
		if err == nil || !strings.Contains(err.Error(), `invalid root version "v2.12.0"`) {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("missing updater", func(t *testing.T) {
		root := releaseMetadataFixture(t)
		config := fixtureReleasePleaseConfig()
		config = strings.Replace(config, `,{"type":"generic","path":"charts/hitkeep/README.md"}`, "", 1)
		writeFixtureFile(t, root, "release-please-config.json", config)
		err := validateReleaseMetadata(root)
		if err == nil || !strings.Contains(err.Error(), "does not manage charts/hitkeep/README.md") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("downstream docs failure isolation", func(t *testing.T) {
		root := releaseMetadataFixture(t)
		writeFixtureFile(t, root, ".github/workflows/release.yml", "sync-docs-release:\nneeds.build-release.result == 'success'\nsync-hitkeep-release.yml\ngh run watch\n")
		err := validateReleaseMetadata(root)
		if err == nil || !strings.Contains(err.Error(), "must not surface downstream documentation workflow failures") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestValidateConfigurationDocument(t *testing.T) {
	known := map[string]runtimeconfig.ConfigurationSetting{
		"HITKEEP_LOG_LEVEL": {Environment: "HITKEEP_LOG_LEVEL", Default: "info"},
	}
	nonRuntime := map[string]bool{"HITKEEP_HOSTNAME": true}

	if err := validateConfigurationDocument("compose.yaml", "HITKEEP_LOG_LEVEL: ${HITKEEP_LOG_LEVEL:-info}\nHITKEEP_HOSTNAME: example.com", known, nonRuntime, true); err != nil {
		t.Fatal(err)
	}
	if err := validateConfigurationDocument("compose.yaml", "HITKEEP_LOG_LEVLE: debug", known, nonRuntime, true); err == nil || !strings.Contains(err.Error(), "unknown HitKeep configuration variable") {
		t.Fatalf("unexpected unknown-variable error: %v", err)
	}
	if err := validateConfigurationDocument("compose.yaml", "HITKEEP_LOG_LEVEL: ${HITKEEP_LOG_LEVEL:-debug}", known, nonRuntime, true); err == nil || !strings.Contains(err.Error(), `runtime default is "info"`) {
		t.Fatalf("unexpected stale-default error: %v", err)
	}
	if err := validateConfigurationDocument("compose.yaml", "HITKEEP_LOG_LEVEL: ${HITKEEP_LOG_LEVEL:-debug} # config-default-override: verbose example", known, nonRuntime, true); err != nil {
		t.Fatal(err)
	}
}

func releaseMetadataFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFixtureFile(t, root, ".release-please-manifest.json", `{ ".": "2.12.0" }`)
	writeFixtureFile(t, root, "server.json", `{"version":"2.12.0"}`)
	writeFixtureFile(t, root, "frontend/dashboard/package.json", `{"version":"2.12.0"}`)
	writeFixtureFile(t, root, "frontend/dashboard/package-lock.json", `{"version":"2.12.0","packages":{"":{"version":"2.12.0"}}}`)
	writeFixtureFile(t, root, "frontend/tracker/package.json", `{"version":"2.12.0"}`)
	writeFixtureFile(t, root, "frontend/dashboard/src/tracker/version.ts", "export const TRACKER_VERSION = '2.12.0'; // x-release-please-version\n")
	writeFixtureFile(t, root, "charts/hitkeep/Chart.yaml", "version: 2.12.0\nappVersion: 2.12.0\n")
	writeFixtureFile(t, root, "charts/hitkeep/README.md", "tag: 2.12.0 # x-release-please-version\n")
	writeFixtureFile(t, root, "release-please-config.json", fixtureReleasePleaseConfig())
	writeFixtureFile(t, root, ".github/workflows/pipeline.yml", "./hk catalog configuration --output json\nhitkeep-configuration.json\nrelease_tag: $tag\nrelease_version: $version\n")
	writeFixtureFile(t, root, ".github/workflows/release.yml", "sync-docs-release:\nneeds.build-release.result == 'success'\nsync-hitkeep-release.yml\n")
	return root
}

func fixtureReleasePleaseConfig() string {
	return `{"packages":{".":{"extra-files":[{"type":"json","path":"server.json","jsonpath":"$.version"},{"type":"json","path":"frontend/dashboard/package.json","jsonpath":"$.version"},{"type":"json","path":"frontend/dashboard/package-lock.json","jsonpath":"$.version"},{"type":"json","path":"frontend/dashboard/package-lock.json","jsonpath":"$['packages']['']['version']"},{"type":"json","path":"frontend/tracker/package.json","jsonpath":"$.version"},{"type":"generic","path":"frontend/dashboard/src/tracker/version.ts"},{"type":"yaml","path":"charts/hitkeep/Chart.yaml","jsonpath":"$.version"},{"type":"yaml","path":"charts/hitkeep/Chart.yaml","jsonpath":"$.appVersion"},{"type":"generic","path":"charts/hitkeep/README.md"}]}}}`
}

func writeFixtureFile(t *testing.T, root, name, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
