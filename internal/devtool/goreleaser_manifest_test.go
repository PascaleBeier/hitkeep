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

func TestGoReleaserSnapshotVersionTemplate(t *testing.T) {
	manifest := readGoReleaserManifest(t)
	if !strings.Contains(manifest, "snapshot:\n  version_template: \"{{ .Env.HITKEEP_ARCHIVE_VERSION }}\"") {
		t.Fatal("GoReleaser snapshot version is not mapped from HITKEEP_ARCHIVE_VERSION")
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
		"goreleaser/v2@v2.18.0 release --clean --skip=publish --config .goreleaser.yaml",
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
		"HITKEEP_ARCHIVE_VERSION=\"${RELEASE_TAG_NAME#v}\" env -u GOROOT go run github.com/goreleaser/goreleaser/v2@v2.18.0 release --snapshot --clean --skip=publish --config .goreleaser.yaml",
		"HITKEEP_ARCHIVE_VERSION=\"${RELEASE_TAG_NAME#v}\"",
		"runner.temp",
		"runner.temp }}/public-assets",
		"build_metadata=\"$(go version -m \"$binary\")\"",
		"GOOS=linux",
		"GOARCH=${arch}",
		"amd64)\n                version_output=\"$(\"$binary\" --version)\"",
		"arm64)\n                if ! strings \"$binary\" | grep -Fxq \"$HITKEEP_VERSION\"; then",
		"binary version mismatch for %s: got %q, want %q",
	} {
		if !strings.Contains(workflow, want) {
			t.Errorf("branch archive workflow missing %q", want)
		}
	}
	if strings.Contains(workflow, "qemu-aarch64") {
		t.Error("release archive verification must not rely on QEMU runtime execution")
	}
}

func TestGoReleaserReleaseArchiveAssetsStayInWorkspace(t *testing.T) {
	path := filepath.Join("..", "..", ".github", "workflows", "pipeline.yml")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(contents)
	start := strings.Index(workflow, "  build-release-archives:\n")
	if start == -1 {
		t.Fatal("release archive job missing")
	}
	archiveJob := workflow[start:]
	if end := strings.Index(archiveJob, "\n  upload-release-binaries:"); end != -1 {
		archiveJob = archiveJob[:end]
	}
	for _, want := range []string{
		"path: ${{ runner.temp }}/public-assets",
		"PUBLIC_ASSETS_DIR: ${{ runner.temp }}/public-assets",
		"PUBLIC_ASSETS_ARCHIVE: public/.public-assets.tar.gz",
		"cp \"$archive\" \"$PUBLIC_ASSETS_ARCHIVE\"",
		"./hk ci restore-dashboard --archive \"$PUBLIC_ASSETS_ARCHIVE\"",
		"rm -f \"$PUBLIC_ASSETS_ARCHIVE\"",
	} {
		if !strings.Contains(archiveJob, want) {
			t.Errorf("release archive assets setup missing %q", want)
		}
	}
	if strings.Contains(archiveJob, "artifacts/public") {
		t.Error("release archive job must not stage public assets in artifacts/public")
	}
}

func TestGoReleaserReleaseConfigUsesProductionCommand(t *testing.T) {
	path := filepath.Join("..", "..", ".github", "workflows", "pipeline.yml")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(contents)
	for _, want := range []string{
		"./hk catalog configuration --output json",
		"env -u GOROOT go run ./cmd/hitkeep config init --output \"$release_inputs/hitkeep.example.yaml\"",
		"cmp \"$release_inputs/hitkeep.example.yaml\" hitkeep.example.yaml",
	} {
		if !strings.Contains(workflow, want) {
			t.Errorf("release configuration generation missing %q", want)
		}
	}
	if strings.Contains(workflow, "./hk config init") {
		t.Error("release configuration generation must use the production Cobra config init command")
	}
}

func TestGoReleaserSnapshotBuildCallsSetArchiveVersion(t *testing.T) {
	path := filepath.Join("..", "..", ".github", "workflows", "pipeline.yml")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(contents)
	const snapshotBuild = "goreleaser/v2@v2.18.0 build \\\n            --snapshot"
	const versionedSnapshotBuild = "HITKEEP_ARCHIVE_VERSION=\"${HITKEEP_VERSION#v}\" env -u GOROOT go run github.com/goreleaser/goreleaser/v2@v2.18.0 build"
	const cleanSnapshotBuild = "--snapshot \\\n            --clean \\\n            --single-target"
	if got := strings.Count(workflow, snapshotBuild); got != 2 {
		t.Errorf("snapshot GoReleaser build calls = %d, want 2", got)
	}
	if got := strings.Count(workflow, versionedSnapshotBuild); got != 2 {
		t.Errorf("snapshot GoReleaser build calls with an archive version = %d, want 2", got)
	}
	if got := strings.Count(workflow, cleanSnapshotBuild); got != 2 {
		t.Errorf("snapshot GoReleaser build calls that clean dist first = %d, want 2", got)
	}
}

func TestGoReleaserNativeBuildsVerifyRawBinaryVersions(t *testing.T) {
	path := filepath.Join("..", "..", ".github", "workflows", "pipeline.yml")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(contents)
	start := strings.Index(workflow, "  build-binaries:\n")
	if start == -1 {
		t.Fatal("native binary build job missing")
	}
	nativeJob := workflow[start:]
	if end := strings.Index(nativeJob, "\n  build-release-archives:"); end != -1 {
		nativeJob = nativeJob[:end]
	}
	verificationStart := strings.Index(nativeJob, "      - name: Verify native binary versions\n")
	if verificationStart == -1 {
		t.Fatal("native binary version verification step missing")
	}
	verificationStep := nativeJob[verificationStart:]
	if end := strings.Index(verificationStep, "\n      - uses: actions/upload-artifact"); end != -1 {
		verificationStep = verificationStep[:end]
	}
	for _, want := range []string{
		"HITKEEP_VERSION: ${{ inputs.version }}",
		"for binary in \"hitkeep-${{ matrix.artifact_suffix }}\" \"hitkeep-cloud-${{ matrix.artifact_suffix }}\"; do",
		"version_output=\"$(\"./$binary\" --version)\"",
		"test \"$version_output\" = \"$HITKEEP_VERSION\"",
	} {
		if !strings.Contains(verificationStep, want) {
			t.Errorf("native binary version verification missing %q", want)
		}
	}
	if strings.Contains(verificationStep, "qemu-") || strings.Contains(verificationStep, "strings \"$binary\"") {
		t.Error("native binary version verification must execute the matching runner binaries")
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
