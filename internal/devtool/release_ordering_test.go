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
  upgrade-from-supported-floor:
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
          repository="${GITHUB_REPOSITORY,,}"
          candidate="ghcr.io/${repository}@${CANDIDATE_DIGEST}"
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
      - name: Build and verify package
        run: |
          plan_id="$(./hk qa plan pr --output json | jq -r '.data.plan_id')"
          ./hk qa pr --plan-id "$plan_id" --gate tracker-package
      - name: Pack verified tracker artifact
        run: |
          metadata="$(npm pack --json)"
          tarball="$(jq -r 'to_entries | first | .value.filename' <<< "$metadata")"
      - name: Upload verified tracker artifact
  link-release-blog:
    needs: release-please
  docs-attestation:
    needs:
      - release-please
      - build-release
      - upgrade-from-supported-floor
      - publish-helm
      - verify-tracker-package
    steps:
      - name: Dispatch and verify exact documentation attestation
        env:
          DOCS_REPOSITORY: PascaleBeier/hitkeep-docs
          DOCS_WORKFLOW_SHA256: 394bedb5cf9b30c79a9eff03ada1bc28b50f6d9fba495a7e2218f1a36e074fc8
        run: |
          gh workflow run sync-hitkeep-release.yml --ref main \\
            -f prepublication=true \\
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
      - upgrade-from-supported-floor
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
      - name: Promote GitHub release
        run: gh release edit "$TAG" --draft=false --latest
  sync-docs-release:
    needs:
      - finalize-release
      - docs-attestation
    steps:
      - name: Dispatch hitkeep-docs release synchronization
        env:
          SOURCE_WORKFLOW_SHA256: ${{ needs.docs-attestation.outputs.source_workflow_sha256 }}
          SOURCE_CATALOG_SHA256: ${{ needs.docs-attestation.outputs.source_catalog_sha256 }}
          SOURCE_EXAMPLE_SHA256: ${{ needs.docs-attestation.outputs.source_example_sha256 }}
          SOURCE_MANIFEST_SHA256: ${{ needs.docs-attestation.outputs.source_manifest_sha256 }}
        run: |
          gh workflow run sync-hitkeep-release.yml \\
            -f prepublication=false \\
            -f source_run_id="${GITHUB_RUN_ID}.${GITHUB_RUN_ATTEMPT}" \\
            -f source_head_sha="${GITHUB_SHA}" \\
            -f source_workflow_sha256="${SOURCE_WORKFLOW_SHA256}" \\
            -f source_catalog_sha256="${SOURCE_CATALOG_SHA256}" \\
            -f source_example_sha256="${SOURCE_EXAMPLE_SHA256}" \\
            -f source_manifest_sha256="${SOURCE_MANIFEST_SHA256}"
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

	missingUpgrade := strings.Replace(workflow, "      - upgrade-from-supported-floor\n", "", 1)
	if err := validateReleaseWorkflowGraph([]byte(missingUpgrade)); err == nil {
		t.Fatal("validateReleaseWorkflowGraph() accepted a finalizer that can run before the upgrade smoke")
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

	earlyPublish := strings.Replace(workflow, "      - name: Promote immutable image to mutable tags\n      - name: Promote GitHub release", "      - name: Promote GitHub release\n      - name: Promote immutable image to mutable tags", 1)
	if err := validateReleaseWorkflowGraph([]byte(earlyPublish)); err == nil {
		t.Fatal("validateReleaseWorkflowGraph() accepted a draft publication before mutable tag promotion")
	}

	missingDraftPublication := strings.Replace(workflow, "--draft=false", "--prerelease=false", 1)
	if err := validateReleaseWorkflowGraph([]byte(missingDraftPublication)); err == nil {
		t.Fatal("validateReleaseWorkflowGraph() accepted a finalizer that does not publish the draft")
	}

	unnormalizedRepository := strings.Replace(workflow, `repository="${GITHUB_REPOSITORY,,}"`, `repository="$GITHUB_REPOSITORY"`, 1)
	if err := validateReleaseWorkflowGraph([]byte(unnormalizedRepository)); err == nil {
		t.Fatal("validateReleaseWorkflowGraph() accepted a candidate image repository without lowercase normalization")
	}

	missingTrackerPlan := strings.Replace(workflow, `--plan-id "$plan_id"`, "", 1)
	if err := validateReleaseWorkflowGraph([]byte(missingTrackerPlan)); err == nil {
		t.Fatal("validateReleaseWorkflowGraph() accepted tracker QA without its persisted plan ID")
	}

	arrayOnlyPackMetadata := strings.Replace(workflow, "to_entries | first | .value.filename", ".[0].filename", 1)
	if err := validateReleaseWorkflowGraph([]byte(arrayOnlyPackMetadata)); err == nil {
		t.Fatal("validateReleaseWorkflowGraph() accepted the obsolete array-only npm pack metadata parser")
	}

	latestBeforeCandidate := strings.Replace(workflow, "      - name: Publish immutable tracker candidate", "      - name: candidate-placeholder", 1)
	latestBeforeCandidate = strings.Replace(latestBeforeCandidate, "      - name: Promote tracker latest dist-tag", "      - name: Publish immutable tracker candidate", 1)
	latestBeforeCandidate = strings.Replace(latestBeforeCandidate, "      - name: candidate-placeholder", "      - name: Promote tracker latest dist-tag", 1)
	if err := validateReleaseWorkflowGraph([]byte(latestBeforeCandidate)); err == nil {
		t.Fatal("validateReleaseWorkflowGraph() accepted a latest dist-tag promotion before retry-safe candidate publication")
	}

	patFinalizer := strings.Replace(workflow, "      - name: Promote GitHub release", "      - name: Promote GitHub release\n        env:\n          GH_TOKEN: ${{ secrets.GHT }}", 1)
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
		{"wrong workflow hash", "DOCS_WORKFLOW_SHA256: 394bedb5cf9b30c79a9eff03ada1bc28b50f6d9fba495a7e2218f1a36e074fc8", "DOCS_WORKFLOW_SHA256: invalid"},
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
