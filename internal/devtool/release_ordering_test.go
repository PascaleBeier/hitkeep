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
  link-release-blog:
    needs: release-please
  docs-attestation:
    needs:
      - release-please
      - build-release
      - migration-interruption
      - upgrade-from-v2-12
      - publish-helm
      - verify-tracker-package
    steps:
      - name: Dispatch and verify exact documentation attestation
        env:
          DOCS_REPOSITORY: PascaleBeier/hitkeep-docs
          DOCS_WORKFLOW_SHA256: 222ee0a991741b076d9571d2315e7859daf20b3176742420b748ab2c5c53f291
        run: |
          gh workflow run sync-hitkeep-release.yml --ref main \\
            -f source_run_id="$source_run_id" \\
            -f source_head_sha="$GITHUB_SHA" \\
            -f source_workflow_sha256="$source_workflow_sha256" \\
            -f source_catalog_sha256="$catalog_sha256" \\
            -f source_example_sha256="$example_sha256" \\
            -f source_manifest_sha256="$manifest_sha256"
          gh run list --event workflow_dispatch
          gh run watch "$docs_run_id"
          gh api "repos/$DOCS_REPOSITORY/actions/runs/$docs_run_id"
          gh api "repos/$DOCS_REPOSITORY/check-suites/$check_suite_id"
          gh run download "$docs_run_id" --name hitkeep-docs-release-attestation
          jq '.id == $run_id and .head_sha == $docs_head_sha and .app.id == 15368 and .conclusion == "success" and .source.tag == $tag and .source.run_id == $source_run_id'
  finalize-release:
    needs:
      - release-please
      - build-release
      - migration-interruption
      - upgrade-from-v2-12
      - publish-helm
      - verify-tracker-package
      - docs-attestation
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

	missingMigrationInterruption := strings.Replace(workflow, "      - migration-interruption\n", "", 1)
	if err := validateReleaseWorkflowGraph([]byte(missingMigrationInterruption)); err == nil {
		t.Fatal("validateReleaseWorkflowGraph() accepted a finalizer that can run before migration interruption acceptance")
	}

	missingMigrationGate := strings.Replace(workflow, "  migration-interruption:\n", "  migration-interruption-removed:\n", 1)
	if err := validateReleaseWorkflowGraph([]byte(missingMigrationGate)); err == nil {
		t.Fatal("validateReleaseWorkflowGraph() accepted a release without migration interruption acceptance")
	}

	missingComposeSurface := strings.Replace(workflow, "          - surface: compose\n", "", 1)
	if err := validateReleaseWorkflowGraph([]byte(missingComposeSurface)); err == nil {
		t.Fatal("validateReleaseWorkflowGraph() accepted an upgrade matrix without the Compose surface")
	}

	missingComposeUpgradeSmoke := strings.Replace(workflow, "./scripts/compose-smoke.sh", "echo skipped", 1)
	if err := validateReleaseWorkflowGraph([]byte(missingComposeUpgradeSmoke)); err == nil {
		t.Fatal("validateReleaseWorkflowGraph() accepted a Compose upgrade job that does not invoke the smoke fixture")
	}

	missingHelmSurface := strings.Replace(workflow, "          - surface: helm\n", "", 1)
	if err := validateReleaseWorkflowGraph([]byte(missingHelmSurface)); err == nil {
		t.Fatal("validateReleaseWorkflowGraph() accepted an upgrade matrix without the Helm surface")
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

	for _, test := range []struct {
		name string
		old  string
		new  string
	}{
		{"missing gate", "  docs-attestation:\n", "  docs-attestation-missing:\n"},
		{"wrong repository", "DOCS_REPOSITORY: PascaleBeier/hitkeep-docs", "DOCS_REPOSITORY: other/docs"},
		{"wrong workflow", "sync-hitkeep-release.yml", "other.yml"},
		{"wrong ref", "--ref main", "--ref release"},
		{"wrong event", "--event workflow_dispatch", "--event push"},
		{"stale trusted producer run", ".source.run_id == $source_run_id", ".source.run_id == \"old-run\""},
		{"different tag", ".source.tag == $tag", ".source.tag == \"v9.9.9\""},
		{"wrong head", ".head_sha == $docs_head_sha", ".head_sha == \"bad\""},
		{"wrong run", ".id == $run_id", ".id == 0"},
		{"wrong conclusion", ".conclusion == \"success\"", ".conclusion == \"failure\""},
		{"wrong app", ".app.id == 15368", ".app.id == 1"},
		{"wrong workflow hash", "DOCS_WORKFLOW_SHA256: 222ee0a991741b076d9571d2315e7859daf20b3176742420b748ab2c5c53f291", "DOCS_WORKFLOW_SHA256: invalid"},
		{"missing artifact", "hitkeep-docs-release-attestation", "missing-attestation"},
	} {
		t.Run(test.name, func(t *testing.T) {
			invalid := strings.Replace(workflow, test.old, test.new, 1)
			if err := validateReleaseWorkflowGraph([]byte(invalid)); err == nil {
				t.Fatalf("validateReleaseWorkflowGraph() accepted %s", test.name)
			}
		})
	}
}
