package devtool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoReleaserTaggedSelfHostedArchiveManifest(t *testing.T) {
	manifest := readGoReleaserManifest(t)
	archive := manifest[strings.Index(manifest, "archives:\n"):strings.Index(manifest, "\nchecksum:")]

	for _, want := range []string{
		"id: self-hosted",
		"ids:\n      - self-hosted",
		"formats:\n      - tar.gz",
		`name_template: "hitkeep_{{ .Version }}_Linux_{{ .Arch }}"`,
		"- LICENSE",
		"- README.md",
		"- hitkeep.example.yaml",
	} {
		if !strings.Contains(archive, want) {
			t.Errorf("self-hosted archive manifest missing %q", want)
		}
	}
	if strings.Contains(archive, "cloud") {
		t.Error("self-hosted archive must not include cloud artifacts")
	}
}

func TestGoReleaserCrossCompilerTemplates(t *testing.T) {
	manifest := readGoReleaserManifest(t)
	for _, want := range []string{
		`CC={{ if eq .Arch "arm64" }}aarch64-linux-gnu-gcc{{ else }}gcc{{ end }}`,
		`CXX={{ if eq .Arch "arm64" }}aarch64-linux-gnu-g++{{ else }}g++{{ end }}`,
	} {
		if strings.Count(manifest, want) != 2 {
			t.Errorf("expected both Linux builds to define %q", want)
		}
	}
}

func TestGoReleaserTaggedArchiveChecksums(t *testing.T) {
	manifest := readGoReleaserManifest(t)
	checksum := manifest[strings.Index(manifest, "checksum:\n"):]
	for _, want := range []string{
		"name_template: SHA256SUMS",
		"algorithm: sha256",
		"ids:\n    - self-hosted",
	} {
		if !strings.Contains(checksum, want) {
			t.Errorf("checksum manifest missing %q", want)
		}
	}
}

func TestGoReleaserReleaseWorkflowContract(t *testing.T) {
	path := filepath.Join("..", "..", ".github", "workflows", "pipeline.yml")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(contents)
	for _, want := range []string{
		"build-release-archives:",
		"runs-on: ubuntu-22.04",
		"gcc-aarch64-linux-gnu g++-aarch64-linux-gnu",
		"release_args=(release --clean --skip=publish --config .goreleaser.yaml)",
		"--clean",
		"--skip=publish",
		"sha256sum --check goreleaser-SHA256SUMS",
		"./hk ci release-checksums",
		"goreleaser-SHA256SUMS",
		"hitkeep-cloud-linux-amd64",
		"hitkeep-linux-amd64",
		"release-archives-${{ inputs.version }}",
		"hitkeep_${release_version}_Linux_amd64.tar.gz",
		"hitkeep_${release_version}_Linux_arm64.tar.gz",
	} {
		if !strings.Contains(workflow, want) {
			t.Errorf("release workflow missing %q", want)
		}
	}
}

func TestGoReleaserBranchArchiveWorkflowContract(t *testing.T) {
	path := filepath.Join("..", "..", ".github", "workflows", "pipeline.yml")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(contents)
	for _, want := range []string{
		"if: ${{ !cancelled() }}",
		"fetch-depth: 0",
		"release_source_tag",
		"release_source_sha",
		"ref: ${{ inputs.release_source_tag || inputs.checkout_ref || github.sha }}",
		"git rev-parse --verify \"refs/tags/${RELEASE_SOURCE_TAG}^{commit}\"",
		"test \"$tag_commit\" = \"$RELEASE_SOURCE_SHA\"",
		"--snapshot",
		"runner.temp",
		"runner.temp }}/public-assets",
		"go version -m \"$binary\"",
		"\"$binary\" --version",
	} {
		if !strings.Contains(workflow, want) {
			t.Errorf("branch archive workflow missing %q", want)
		}
	}
}

func TestGoReleaserReleaseCallSitePinsImmutableSource(t *testing.T) {
	path := filepath.Join("..", "..", ".github", "workflows", "release.yml")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(contents)
	for _, want := range []string{
		"checkout_ref: ${{ github.sha }}",
		"release_source_tag: ${{ needs.release-please.outputs.tag_name }}",
		"release_source_sha: ${{ github.sha }}",
	} {
		if !strings.Contains(workflow, want) {
			t.Errorf("release call site missing %q", want)
		}
	}
}

func readGoReleaserManifest(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", ".goreleaser.yaml")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(contents)
}
