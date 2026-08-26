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
}
