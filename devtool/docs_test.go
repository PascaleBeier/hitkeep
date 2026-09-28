package devtool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	runtimeconfig "hitkeep/config"
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

	t.Run("published candidate is rejected", func(t *testing.T) {
		root := releaseMetadataFixture(t)
		config := strings.Replace(fixtureReleasePleaseConfig(), `".":{"draft":true`, `".":{"draft":false`, 1)
		writeFixtureFile(t, root, "release-please-config.json", config)
		err := validateReleaseMetadata(root)
		if err == nil || !strings.Contains(err.Error(), "effective packages['.'].draft must be true") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("missing forced tag creation", func(t *testing.T) {
		root := releaseMetadataFixture(t)
		config := strings.Replace(fixtureReleasePleaseConfig(), `"force-tag-creation":true`, `"force-tag-creation":false`, 1)
		writeFixtureFile(t, root, "release-please-config.json", config)
		err := validateReleaseMetadata(root)
		if err == nil || !strings.Contains(err.Error(), "force-tag-creation must be true") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

}

func TestValidateContainerDataPath(t *testing.T) {
	if err := validateContainerDataPath("Dockerfile", "ENV HITKEEP_DATA_PATH=\"/var/lib/hitkeep/data\"\nVOLUME [\"/var/lib/hitkeep\"]"); err != nil {
		t.Fatalf("validateContainerDataPath() error = %v, want nil", err)
	}
	if err := validateContainerDataPath("Dockerfile", "VOLUME [\"/var/lib/hitkeep\"]"); err == nil {
		t.Fatal("validateContainerDataPath() succeeded without a HITKEEP_DATA_PATH image default")
	}
	if err := validateContainerDataPath("Dockerfile", "ENV HITKEEP_DATA_PATH=\"/var/lib/hitkeep/data\""); err == nil {
		t.Fatal("validateContainerDataPath() succeeded without a persistent volume")
	}
}

func TestValidateConfigurationPublicationRejectsMissingAndDefaultDrift(t *testing.T) {
	requirements := runtimeconfig.PublicationRequirements()
	for _, requirement := range requirements {
		for _, surface := range requirement.Surfaces {
			for _, path := range requirement.Paths[surface] {
				t.Run(path, func(t *testing.T) {
					if actual := configurationPublicationSurface(path); actual != surface {
						t.Fatalf("configurationPublicationSurface(%q) = %q, want %q", path, actual, surface)
					}
					contents, drift := publicationTestContents(path)
					if err := validateConfigurationPublication(path, contents, requirements); err != nil {
						t.Fatalf("validateConfigurationPublication() error = %v", err)
					}
					if err := validateConfigurationPublication(path, "", requirements); err == nil {
						t.Fatal("validateConfigurationPublication() accepted an omitted required setting")
					}
					if err := validateConfigurationPublication(path, drift, requirements); err == nil {
						t.Fatal("validateConfigurationPublication() accepted a default drift")
					}
				})
			}
		}
	}
}

func TestValidateRequiredConfigurationPublicationsRejectsDeletedPath(t *testing.T) {
	requirements := runtimeconfig.PublicationRequirements()
	contents := map[string]string{}
	for _, requirement := range requirements {
		for _, surface := range requirement.Surfaces {
			for _, path := range requirement.Paths[surface] {
				contents[path], _ = publicationTestContents(path)
			}
		}
	}
	load := func(path string) string { return contents[path] }
	if err := validateRequiredConfigurationPublications(requirements, load); err != nil {
		t.Fatalf("validateRequiredConfigurationPublications() error = %v", err)
	}
	for path, content := range contents {
		delete(contents, path)
		if err := validateRequiredConfigurationPublications(requirements, load); err == nil {
			t.Errorf("validateRequiredConfigurationPublications() accepted deleted path %s", path)
		}
		contents[path] = content
	}
}

func publicationTestContents(path string) (string, string) {
	switch path {
	case "Dockerfile":
		return "ENV HITKEEP_DATA_PATH=\"/var/lib/hitkeep/data\"", "ENV HITKEEP_DATA_PATH=\"/tmp/hitkeep\""
	case "charts/hitkeep/templates/statefulset.yaml":
		return "- name: HITKEEP_DATA_PATH\n  value: {{ .Values.persistence.mountPath | quote }}" + helmPublicationValuesSeparator + "cache:\n  mountPath: /var/lib/hitkeep/data\npersistence:\n  mountPath: /var/lib/hitkeep/data", "- name: HITKEEP_DATA_PATH\n  value: {{ .Values.persistence.mountPath | quote }}" + helmPublicationValuesSeparator + "cache:\n  mountPath: /var/lib/hitkeep/data\npersistence:\n  mountPath: /tmp/hitkeep"
	case runtimeconfig.ConfigurationExampleFilename:
		return "data-path: data", "data-path: /tmp/hitkeep"
	default:
		return "HITKEEP_DATA_PATH: /var/lib/hitkeep/data", "HITKEEP_DATA_PATH: /tmp/hitkeep"
	}
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
	writeFixtureFile(t, root, "frontend/dashboard/package-lock.json", `{"lockfileVersion":4,"packages":{"":{"version":"2.12.0"}}}`)
	writeFixtureFile(t, root, "frontend/tracker/package.json", `{"version":"2.12.0"}`)
	writeFixtureFile(t, root, "frontend/dashboard/src/tracker/version.ts", "export const TRACKER_VERSION = '2.12.0'; // x-release-please-version\n")
	writeFixtureFile(t, root, "charts/hitkeep/Chart.yaml", "version: 2.12.0\nappVersion: 2.12.0\n")
	writeFixtureFile(t, root, "charts/hitkeep/README.md", "tag: 2.12.0 # x-release-please-version\n")
	writeFixtureFile(t, root, "release-please-config.json", fixtureReleasePleaseConfig())
	return root
}

func fixtureReleasePleaseConfig() string {
	return `{"draft":true,"prerelease":false,"force-tag-creation":true,"packages":{".":{"draft":true,"prerelease":false,"extra-files":[{"type":"json","path":"server.json","jsonpath":"$.version"},{"type":"json","path":"frontend/dashboard/package.json","jsonpath":"$.version"},{"type":"json","path":"frontend/dashboard/package-lock.json","jsonpath":"$['packages']['']['version']"},{"type":"json","path":"frontend/tracker/package.json","jsonpath":"$.version"},{"type":"generic","path":"frontend/dashboard/src/tracker/version.ts"},{"type":"yaml","path":"charts/hitkeep/Chart.yaml","jsonpath":"$.version"},{"type":"yaml","path":"charts/hitkeep/Chart.yaml","jsonpath":"$.appVersion"},{"type":"generic","path":"charts/hitkeep/README.md"}]}}}`
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
