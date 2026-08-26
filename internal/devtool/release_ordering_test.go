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
  publish-helm:
    needs: build-release
  verify-tracker-package:
    needs: build-release
  link-release-blog:
    needs: release-please
  finalize-release:
    needs:
      - release-please
      - build-release
      - publish-helm
      - verify-tracker-package
    steps:
      - name: Publish immutable tracker candidate
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

	earlyPublish := strings.Replace(workflow, "      - name: Promote immutable image to mutable tags\n      - name: Publish draft GitHub release", "      - name: Publish draft GitHub release\n      - name: Promote immutable image to mutable tags", 1)
	if err := validateReleaseWorkflowGraph([]byte(earlyPublish)); err == nil {
		t.Fatal("validateReleaseWorkflowGraph() accepted a draft publication before mutable tag promotion")
	}

	latestBeforeCandidate := strings.Replace(workflow, "      - name: Publish immutable tracker candidate\n      - name: Promote tracker latest dist-tag", "      - name: Promote tracker latest dist-tag\n      - name: Publish immutable tracker candidate", 1)
	if err := validateReleaseWorkflowGraph([]byte(latestBeforeCandidate)); err == nil {
		t.Fatal("validateReleaseWorkflowGraph() accepted a latest dist-tag promotion before retry-safe candidate publication")
	}

	patFinalizer := strings.Replace(workflow, "      - name: Publish draft GitHub release", "      - name: Publish draft GitHub release\n        env:\n          GH_TOKEN: ${{ secrets.GHT }}", 1)
	if err := validateReleaseWorkflowGraph([]byte(patFinalizer)); err == nil {
		t.Fatal("validateReleaseWorkflowGraph() accepted secrets.GHT in the finalizer")
	}
}
