package devtool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseValidationWorkflowContract(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "release-validation.yml"))
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(contents)
	for _, want := range []string{
		"workflow_dispatch:",
		"github.event.pull_request.head.repo.full_name == github.repository",
		"runs-on: ubuntu-22.04",
		"gcc-aarch64-linux-gnu g++-aarch64-linux-gnu",
		"goreleaser/goreleaser/v2@v2.18.0 release",
		"--snapshot --clean --skip=publish --config .goreleaser.yaml",
		`find dist -maxdepth 1 -type f -name "hitkeep_*_Linux_${arch}.tar.gz"`,
		"GOARCH=${arch}",
		"CGO_ENABLED=1",
		"sha256sum --check SHA256SUMS",
		"runs-on: ubuntu-24.04",
		"packages: read",
		"registry: ghcr.io",
		"password: ${{ github.token }}",
		"sigs.k8s.io/kind@v0.29.0",
		"./hk build image --variant self-hosted --output json",
		`--publish "127.0.0.1:${registry_port}:5000"`,
		"trap cleanup EXIT",
		"docker network connect kind",
		"previous_chart_digest",
		"HITKEEP_PREVIOUS_IMAGE=",
		"HITKEEP_PREVIOUS_CHART=",
		"HITKEEP_PREVIOUS_CHART_DIGEST=",
		"HITKEEP_CANDIDATE_CHART=",
		"HITKEEP_CANDIDATE_CHART_VERSION=",
		"HITKEEP_KIND_CLUSTER=",
		`./scripts/helm-smoke.sh "$candidate" self-hosted`,
	} {
		if !strings.Contains(workflow, want) {
			t.Errorf("release validation workflow missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"packages: write",
		"id-token: write",
		"attestations: write",
		"artifact-metadata: write",
		"actions/upload-artifact",
		"actions/attest",
		"DOCKERHUB_",
		"secrets.GHT",
		"helm push",
		"npm publish",
		"gh release",
		"repository_dispatch",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Errorf("release validation workflow must not contain %q", forbidden)
		}
	}
}
