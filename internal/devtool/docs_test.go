package devtool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

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

	t.Run("migration workflow is not reusable", func(t *testing.T) {
		root := releaseMetadataFixture(t)
		writeFixtureFile(t, root, ".github/workflows/default-tenant-migration-acceptance.yml", "on:\n  workflow_dispatch:\njobs: {}\n")
		err := validateReleaseMetadata(root)
		if err == nil || !strings.Contains(err.Error(), "must support workflow_call") {
			t.Fatalf("unexpected error: %v", err)
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

	t.Run("root package draft override", func(t *testing.T) {
		root := releaseMetadataFixture(t)
		config := strings.Replace(fixtureReleasePleaseConfig(), `".":{`, `".":{"draft":false,`, 1)
		writeFixtureFile(t, root, "release-please-config.json", config)
		err := validateReleaseMetadata(root)
		if err == nil || !strings.Contains(err.Error(), "effective packages['.'].draft must be true") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("missing GoReleaser release build", func(t *testing.T) {
		root := releaseMetadataFixture(t)
		writeFixtureFile(t, root, ".github/workflows/pipeline.yml", "./hk catalog configuration --output json\n./hk catalog configuration-manifest\nhitkeep-configuration.json\nhitkeep.example.yaml\nhitkeep-configuration-manifest.json\nrelease_tag: $tag\nrelease_version: $version\n")
		err := validateReleaseMetadata(root)
		if err == nil || !strings.Contains(err.Error(), `.github/workflows/pipeline.yml is missing release metadata contract "github.com/goreleaser/goreleaser/v2@v2.18.0"`) {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("manual release build is rejected", func(t *testing.T) {
		root := releaseMetadataFixture(t)
		writeFixtureFile(t, root, ".github/workflows/pipeline.yml", "github.com/goreleaser/goreleaser/v2@v2.18.0\n--snapshot\n--clean\n--single-target\n--id self-hosted\n--id cloud\n./hk catalog configuration --output json\n./hk catalog configuration-manifest\nhitkeep-configuration.json\nhitkeep.example.yaml\nhitkeep-configuration-manifest.json\nrelease_tag: $tag\nrelease_version: $version\n./hk ci build-binaries\n")
		err := validateReleaseMetadata(root)
		if err == nil || !strings.Contains(err.Error(), ".github/workflows/pipeline.yml must not run ./hk ci build-binaries") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("missing example configuration release asset", func(t *testing.T) {
		root := releaseMetadataFixture(t)
		writeFixtureFile(t, root, ".github/workflows/pipeline.yml", "github.com/goreleaser/goreleaser/v2@v2.18.0\n--snapshot\n--clean\n--single-target\n--id self-hosted\n--id cloud\n./hk catalog configuration --output json\n./hk catalog configuration-manifest\nhitkeep-configuration.json\nhitkeep-configuration-manifest.json\nrelease_tag: $tag\nrelease_version: $version\n")
		err := validateReleaseMetadata(root)
		if err == nil || !strings.Contains(err.Error(), `.github/workflows/pipeline.yml is missing release metadata contract "hitkeep.example.yaml"`) {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("downstream docs failure isolation", func(t *testing.T) {
		root := releaseMetadataFixture(t)
		workflow, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "release.yml"))
		if err != nil {
			t.Fatal(err)
		}
		writeFixtureFile(t, root, ".github/workflows/release.yml", string(workflow)+"\n# gh run watch\n")
		err = validateReleaseMetadata(root)
		if err == nil || !strings.Contains(err.Error(), "must not surface downstream documentation workflow failures") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestValidateGovulncheckWorkflowContract(t *testing.T) {
	valid := []byte("plan_id=\"$(./hk qa plan pr --output json | jq -r '.data.plan_id')\"\n./hk qa pr --plan-id \"$plan_id\" --gate \"$gates\"\n")
	if err := validateGovulncheckWorkflowContract(valid); err != nil {
		t.Fatalf("validateGovulncheckWorkflowContract() error = %v", err)
	}
	for _, workflow := range [][]byte{
		[]byte("./hk qa pr --gate \"$gates\"\n"),
		[]byte("plan_id=\"$(./hk qa plan pr --output json | jq -r '.data.plan_id')\"\n./hk qa pr --gate \"$gates\"\n"),
		[]byte("./hk qa pr --plan-id \"$plan_id\" --gate \"$gates\"\n"),
	} {
		if err := validateGovulncheckWorkflowContract(workflow); err == nil {
			t.Fatalf("validateGovulncheckWorkflowContract() accepted %q", workflow)
		}
	}
}

func TestWorkflowNeedsRejectsUnsupportedYAMLNodes(t *testing.T) {
	for _, kind := range []yaml.Kind{yaml.DocumentNode, yaml.MappingNode, yaml.AliasNode} {
		var needs workflowNeeds
		if err := needs.UnmarshalYAML(&yaml.Node{Kind: kind}); err == nil {
			t.Errorf("UnmarshalYAML(%v) accepted unsupported node", kind)
		}
	}
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
	case "config.example.yaml":
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
	writeFixtureFile(t, root, ".goreleaser.yaml", "files:\n  - hitkeep-configuration.json\n  - hitkeep.example.yaml\n  - hitkeep-configuration-manifest.json\n")
	writeFixtureFile(t, root, ".github/workflows/pipeline.yml", "github.com/goreleaser/goreleaser/v2@v2.18.0\n--snapshot\n--clean\n--single-target\n--id self-hosted\n--id cloud\n./hk catalog configuration --output json\n./hk catalog configuration-manifest\nhitkeep-configuration.json\nhitkeep.example.yaml\nhitkeep-configuration-manifest.json\nrelease_tag: $tag\nrelease_version: $version\n")
	writeFixtureFile(t, root, ".github/workflows/release.yml", `# sync-hitkeep-release.yml
jobs:
  release-please: {}
  build-release:
    needs: release-please
  migration-interruption:
    needs:
      - release-please
      - build-release
    if: ${{ needs.release-please.outputs.release_created == 'true' && needs.build-release.result == 'success' }}
    uses: ./.github/workflows/default-tenant-migration-acceptance.yml
  upgrade-from-v2-12:
    needs: build-release
    strategy:
      matrix:
        include:
          - surface: docker
          - surface: compose
          - surface: helm
    steps:
      - name: Resolve immutable upgrade fixture
        id: fixture
        env:
          CANDIDATE_DIGEST: ${{ needs.build-release.outputs.image_digest }}
        run: |
          manifest="tests/fixtures/release-fixtures.json"
          previous_version="2.12.0"
      - name: Smoke Docker upgrade from supported floor
        env:
          CANDIDATE_IMAGE: ${{ steps.fixture.outputs.candidate }}
          HITKEEP_PREVIOUS_IMAGE: ${{ steps.fixture.outputs.previous }}
        run: ./scripts/docker-smoke.sh "$CANDIDATE_IMAGE" self-hosted --recreate
      - name: Smoke Compose upgrade from supported floor
        env:
          CANDIDATE_IMAGE: ${{ steps.fixture.outputs.candidate }}
          HITKEEP_PREVIOUS_IMAGE: ${{ steps.fixture.outputs.previous }}
        run: ./scripts/compose-smoke.sh "$CANDIDATE_IMAGE" self-hosted
      - name: Smoke Helm upgrade from supported floor
        env:
          CANDIDATE_IMAGE: ${{ steps.fixture.outputs.candidate }}
          HITKEEP_PREVIOUS_IMAGE: ${{ steps.fixture.outputs.previous }}
        run: ./scripts/helm-smoke.sh "$CANDIDATE_IMAGE" self-hosted
  publish-helm:
    needs: build-release
  verify-tracker-package:
    needs: build-release
    steps:
      - name: Pack verified tracker artifact
        run: npm pack --json
      - name: Upload verified tracker artifact
  finalize-release:
    needs:
      - release-please
      - build-release
      - migration-interruption
      - upgrade-from-v2-12
      - publish-helm
      - verify-tracker-package
    steps:
      - name: Download verified tracker artifact
      - name: Publish immutable tracker candidate
        run: |
          integrity="$(openssl dgst -sha512 -binary \"$tarball\")"
          npm publish "$tarball"
          npm view @hitkeep/tracker dist.integrity
      - name: Promote tracker latest dist-tag
      - name: Promote immutable image to mutable tags
      - name: Publish draft GitHub release
  sync-docs-release:
    needs: finalize-release
    steps:
      - name: Dispatch hitkeep-docs release synchronization
        run: |
          source_workflow_sha256="$(gh api -H 'Accept: application/vnd.github.raw+json' \"repos/$GITHUB_REPOSITORY/contents/.github/workflows/release.yml?ref=$GITHUB_SHA\" | sha256sum | awk '{print $1}')"
          gh workflow run sync-hitkeep-release.yml \\
            -f source_run_id="${GITHUB_RUN_ID}.${GITHUB_RUN_ATTEMPT}" \\
            -f source_head_sha="${GITHUB_SHA}" \\
            -f source_workflow_sha256="${source_workflow_sha256}"
  deploy-cloud:
    needs: finalize-release
`)
	writeFixtureFile(t, root, ".github/workflows/default-tenant-migration-acceptance.yml", `on:
  workflow_call:
jobs: {}
`)
	return root
}

func fixtureReleasePleaseConfig() string {
	return `{"draft":true,"packages":{".":{"extra-files":[{"type":"json","path":"server.json","jsonpath":"$.version"},{"type":"json","path":"frontend/dashboard/package.json","jsonpath":"$.version"},{"type":"json","path":"frontend/dashboard/package-lock.json","jsonpath":"$['packages']['']['version']"},{"type":"json","path":"frontend/tracker/package.json","jsonpath":"$.version"},{"type":"generic","path":"frontend/dashboard/src/tracker/version.ts"},{"type":"yaml","path":"charts/hitkeep/Chart.yaml","jsonpath":"$.version"},{"type":"yaml","path":"charts/hitkeep/Chart.yaml","jsonpath":"$.appVersion"},{"type":"generic","path":"charts/hitkeep/README.md"}]}}}`
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
