package devtool

import (
	"strings"
	"testing"
)

func TestValidateReleaseWorkflowGraph(t *testing.T) {
	workflow := `jobs:
  release-please: {}
  build-release:
    needs: release-please
  upgrade-from-v2-12:
    needs: build-release
    steps:
      - name: Smoke upgrade from supported floor
        env:
          CANDIDATE_DIGEST: ${{ needs.build-release.outputs.image_digest }}
        run: |
          manifest="tests/fixtures/release-fixtures.json"
          previous_version="2.12.0"
          candidate="${{ needs.build-release.outputs.image_digest }}"
          ./scripts/docker-smoke.sh "$candidate" self-hosted --recreate
  upgrade-compose-from-v2-12:
    needs: build-release
    steps:
      - name: Smoke Compose upgrade from supported floor
        env:
          CANDIDATE_DIGEST: ${{ needs.build-release.outputs.image_digest }}
        run: |
          manifest="tests/fixtures/release-fixtures.json"
          previous_version="2.12.0"
          candidate="${{ needs.build-release.outputs.image_digest }}"
          ./scripts/compose-smoke.sh "$candidate" self-hosted
  upgrade-helm-from-v2-12:
    needs: build-release
    steps:
      - name: Smoke Helm upgrade from supported floor
        env:
          CANDIDATE_DIGEST: ${{ needs.build-release.outputs.image_digest }}
        run: |
          manifest="tests/fixtures/release-fixtures.json"
          previous_version="2.12.0"
          candidate="${{ needs.build-release.outputs.image_digest }}"
          ./scripts/helm-smoke.sh "$candidate" self-hosted
  publish-helm:
    needs: build-release
  verify-tracker-package:
    needs: build-release
    steps:
      - name: Pack verified tracker artifact
        run: npm pack --json
      - name: Upload verified tracker artifact
  link-release-blog:
    needs: release-please
  finalize-release:
    needs:
      - release-please
      - build-release
      - upgrade-from-v2-12
      - upgrade-compose-from-v2-12
      - upgrade-helm-from-v2-12
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
  deploy-cloud:
    needs: finalize-release
`
	if err := validateReleaseWorkflowGraph([]byte(workflow)); err != nil {
		t.Fatalf("validateReleaseWorkflowGraph() error = %v", err)
	}

	missingHelm := strings.Replace(workflow, "      - publish-helm\n", "", 1)
	if err := validateReleaseWorkflowGraph([]byte(missingHelm)); err == nil {
		t.Fatal("validateReleaseWorkflowGraph() accepted a finalizer that can run before Helm publication")
	}

	missingUpgrade := strings.Replace(workflow, "      - upgrade-from-v2-12\n", "", 1)
	if err := validateReleaseWorkflowGraph([]byte(missingUpgrade)); err == nil {
		t.Fatal("validateReleaseWorkflowGraph() accepted a finalizer that can run before the upgrade smoke")
	}

	missingComposeUpgrade := strings.Replace(workflow, "      - upgrade-compose-from-v2-12\n", "", 1)
	if err := validateReleaseWorkflowGraph([]byte(missingComposeUpgrade)); err == nil {
		t.Fatal("validateReleaseWorkflowGraph() accepted a finalizer that can run before the Compose upgrade smoke")
	}

	missingComposeUpgradeSmoke := strings.Replace(workflow, "./scripts/compose-smoke.sh", "echo skipped", 1)
	if err := validateReleaseWorkflowGraph([]byte(missingComposeUpgradeSmoke)); err == nil {
		t.Fatal("validateReleaseWorkflowGraph() accepted a Compose upgrade job that does not invoke the smoke fixture")
	}

	missingHelmUpgrade := strings.Replace(workflow, "      - upgrade-helm-from-v2-12\n", "", 1)
	if err := validateReleaseWorkflowGraph([]byte(missingHelmUpgrade)); err == nil {
		t.Fatal("validateReleaseWorkflowGraph() accepted a finalizer that can run before the Helm upgrade smoke")
	}

	missingHelmUpgradeSmoke := strings.Replace(workflow, "./scripts/helm-smoke.sh", "echo skipped", 1)
	if err := validateReleaseWorkflowGraph([]byte(missingHelmUpgradeSmoke)); err == nil {
		t.Fatal("validateReleaseWorkflowGraph() accepted a Helm upgrade job that does not invoke the smoke fixture")
	}

	missingUpgradeSmoke := strings.Replace(workflow, "./scripts/docker-smoke.sh", "echo skipped", 1)
	if err := validateReleaseWorkflowGraph([]byte(missingUpgradeSmoke)); err == nil {
		t.Fatal("validateReleaseWorkflowGraph() accepted an upgrade job that does not invoke the smoke fixture")
	}

	missingTrackerArtifact := strings.Replace(workflow, "      - name: Download verified tracker artifact\n", "", 1)
	if err := validateReleaseWorkflowGraph([]byte(missingTrackerArtifact)); err == nil {
		t.Fatal("validateReleaseWorkflowGraph() accepted a finalizer without the verified tracker artifact")
	}

	earlyPublish := strings.Replace(workflow, "      - name: Promote immutable image to mutable tags\n      - name: Publish draft GitHub release", "      - name: Publish draft GitHub release\n      - name: Promote immutable image to mutable tags", 1)
	if err := validateReleaseWorkflowGraph([]byte(earlyPublish)); err == nil {
		t.Fatal("validateReleaseWorkflowGraph() accepted a draft publication before mutable tag promotion")
	}

	latestBeforeCandidate := strings.Replace(workflow, "      - name: Publish immutable tracker candidate", "      - name: candidate-placeholder", 1)
	latestBeforeCandidate = strings.Replace(latestBeforeCandidate, "      - name: Promote tracker latest dist-tag", "      - name: Publish immutable tracker candidate", 1)
	latestBeforeCandidate = strings.Replace(latestBeforeCandidate, "      - name: candidate-placeholder", "      - name: Promote tracker latest dist-tag", 1)
	if err := validateReleaseWorkflowGraph([]byte(latestBeforeCandidate)); err == nil {
		t.Fatal("validateReleaseWorkflowGraph() accepted a latest dist-tag promotion before retry-safe candidate publication")
	}

	patFinalizer := strings.Replace(workflow, "      - name: Publish draft GitHub release", "      - name: Publish draft GitHub release\n        env:\n          GH_TOKEN: ${{ secrets.GHT }}", 1)
	if err := validateReleaseWorkflowGraph([]byte(patFinalizer)); err == nil {
		t.Fatal("validateReleaseWorkflowGraph() accepted secrets.GHT in the finalizer")
	}
}
