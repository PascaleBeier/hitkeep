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
		"- hitkeep-configuration.json",
		"- hitkeep.example.yaml",
		"- hitkeep-configuration-manifest.json",
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
	path := filepath.Join("..", ".github", "workflows", "pipeline.yml")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(contents)
	for _, want := range []string{
		"build-release-archives:",
		"- build-binaries",
		"pattern: binaries-linux-*",
		"name: release-inputs-${{ inputs.version }}",
		"--format=posix --owner=0 --group=0 --numeric-owner --mtime=\"@${timestamp}\"",
		"--pax-option=delete=atime,delete=ctime",
		"| gzip -n > \"$archive\"",
		"test \"$(sha256sum \"hitkeep-linux-${arch}\" | awk '{print $1}')\" = \\",
		"sha256sum hitkeep_*.tar.gz | LC_ALL=C sort > goreleaser-SHA256SUMS",
		"./hk ci release-checksums",
		"goreleaser-SHA256SUMS",
		"hitkeep-configuration-manifest.json",
		"release-archives-${{ inputs.version }}",
		"hitkeep_${release_version}_Linux_amd64.tar.gz",
		"hitkeep_${release_version}_Linux_arm64.tar.gz",
	} {
		if !strings.Contains(workflow, want) {
			t.Errorf("release workflow missing %q", want)
		}
	}
}

func TestPipelineRetargetsExistingPrereleaseTagBeforeReleaseEdit(t *testing.T) {
	path := filepath.Join("..", ".github", "workflows", "pipeline.yml")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const existingRelease = "if gh release view \"${RELEASE_TAG_NAME}\" >/dev/null 2>&1; then"
	branchStart := strings.Index(string(contents), existingRelease)
	if branchStart < 0 {
		t.Fatal("pipeline prerelease update branch is missing")
	}
	branch := string(contents)[branchStart:]
	retarget := strings.Index(branch, "gh api --method PATCH \"repos/${GITHUB_REPOSITORY}/git/refs/tags/${RELEASE_TAG_NAME}\" -f sha=\"${RELEASE_TARGET_SHA}\" -F force=true")
	edit := strings.Index(branch, "gh release edit \"${RELEASE_TAG_NAME}\"")
	if retarget < 0 || edit < 0 || retarget > edit {
		t.Fatal("existing prerelease tag must be retargeted through the GitHub API before release edit")
	}
}

func TestGoReleaserBranchArchiveWorkflowContract(t *testing.T) {
	path := filepath.Join("..", ".github", "workflows", "pipeline.yml")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(contents)
	start := strings.Index(workflow, "  build-release-archives:\n")
	end := strings.Index(workflow[start:], "\n  upload-release-binaries:")
	if start < 0 || end < 0 {
		t.Fatal("release archive job missing")
	}
	archiveJob := workflow[start : start+end]
	for _, want := range []string{
		"if: ${{ !cancelled() }}",
		"fetch-depth: 0",
		"release_source_tag",
		"release_source_sha",
		"ref: ${{ inputs.release_source_tag || inputs.checkout_ref || github.sha }}",
		"git rev-parse --verify \"refs/tags/${RELEASE_SOURCE_TAG}^{commit}\"",
		"test \"$tag_commit\" = \"$RELEASE_SOURCE_SHA\"",
		"pattern: binaries-linux-*",
		"name: release-inputs-${{ inputs.version }}",
		"tar --format=posix",
		"gzip -n",
	} {
		if !strings.Contains(archiveJob, want) {
			t.Errorf("branch archive workflow missing %q", want)
		}
	}
	for _, forbidden := range []string{"actions/setup-go", "go run", "goreleaser/v2@", "apt-get", "gcc-aarch64", "qemu-aarch64"} {
		if strings.Contains(archiveJob, forbidden) {
			t.Errorf("release archive job must not compile: found %q", forbidden)
		}
	}
}

func TestFilesystemLayoutManifestPinsBuildOwnershipSurfaces(t *testing.T) {
	root := ".."
	contents, err := os.ReadFile(filepath.Join(root, "docs", "config-refactor", "filesystem-layout-manifest.md"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := string(contents)
	for _, row := range []string{
		"| `devtool/catalog.go::variants` | Canonical developer build owner | Defines variant IDs, build tags, developer-container environment, local image names, and publishability metadata. Its cloud values are developer/build defaults, not runtime settings. |",
		"| `devtool/app.go::App.ComposeEnvironment` | Workspace projection | Projects the selected variant plus workspace-scoped paths and ports into Compose variables; it does not define supported variants or production configuration precedence. |",
		"| `devtool/runs.go::App.executeBuild` | Build orchestrator | Resolves a catalog variant, enforces the production/developer dependency boundary, and invokes the selected binary or image build without redefining tags or defaults. |",
		"| `Dockerfile` | Image-build consumer | Consumes explicit build arguments and produces the selected application image; it is not a configuration catalog or runtime parser. |",
		"| `.goreleaser.yaml` | Release-build projection | Maps the canonical self-hosted and cloud build identities to release tags, CGO targets, archive contents, and names; it must remain aligned with the developer catalog. |",
		"| `.github/workflows/pipeline.yml` | Delivery consumer | Supplies explicit version/ref inputs, restores verified assets, and invokes the canonical build projections. Publication and attestation policy lives here, but variant semantics do not. |",
	} {
		if count := strings.Count(manifest, row); count != 1 {
			t.Errorf("build ownership row appears %d times, want exactly once: %s", count, row)
		}
	}

	for path, anchors := range map[string][]string{
		"devtool/catalog.go":             {"var variants = []Variant{"},
		"devtool/app.go":                 {"func (a *App) ComposeEnvironment(variant Variant) []string"},
		"devtool/runs.go":                {"func (a *App) executeBuild(ctx context.Context, request RunRequest, writer io.Writer) error"},
		"Dockerfile":                     nil,
		".goreleaser.yaml":               {"id: self-hosted", "id: cloud", "CGO_ENABLED=1"},
		".github/workflows/pipeline.yml": {"build-release-archives:", "build-and-push-image:"},
	} {
		source, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Errorf("read build ownership surface %s: %v", path, err)
			continue
		}
		for _, anchor := range anchors {
			if !strings.Contains(string(source), anchor) {
				t.Errorf("build ownership surface %s is missing %q", path, anchor)
			}
		}
	}
}

func TestGoReleaserReleaseArchiveAssetsStayInWorkspace(t *testing.T) {
	path := filepath.Join("..", ".github", "workflows", "pipeline.yml")
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
		"pattern: binaries-linux-*",
		"name: release-inputs-${{ inputs.version }}",
		"path: .",
		"cp \"hitkeep-linux-${arch}\" LICENSE README.md hitkeep-configuration.json hitkeep.example.yaml hitkeep-configuration-manifest.json \"$staging/\"",
	} {
		if !strings.Contains(archiveJob, want) {
			t.Errorf("release archive artifact setup missing %q", want)
		}
	}
	for _, forbidden := range []string{"public-assets", "restore-dashboard", "prepare-public-image"} {
		if strings.Contains(archiveJob, forbidden) {
			t.Errorf("release archive job must package downloaded release artifacts only: found %q", forbidden)
		}
	}
}

func TestGoReleaserReleaseConfigUsesProductionCommand(t *testing.T) {
	path := filepath.Join("..", ".github", "workflows", "pipeline.yml")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(contents)
	for _, want := range []string{
		"./hk catalog configuration --output json",
		"env -u GOROOT go run ./cmd/hitkeep config init --output \"$release_inputs/hitkeep.example.yaml\"",
		"name: release-inputs-${{ inputs.version }}",
		"path: ${{ runner.temp }}/hitkeep-release-inputs",
	} {
		if !strings.Contains(workflow, want) {
			t.Errorf("release configuration generation missing %q", want)
		}
	}
	if strings.Count(workflow, "./hk catalog configuration --output json") != 1 || strings.Count(workflow, "go run ./cmd/hitkeep config init") != 1 {
		t.Error("release configuration inputs must be generated once and transported unchanged")
	}
	if strings.Contains(workflow, "./hk config init") {
		t.Error("release configuration generation must use the production Cobra config init command")
	}
}

func TestGoReleaserSnapshotBuildCallsSetArchiveVersion(t *testing.T) {
	path := filepath.Join("..", ".github", "workflows", "pipeline.yml")
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
	path := filepath.Join("..", ".github", "workflows", "pipeline.yml")
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
	path := filepath.Join("..", ".github", "workflows", "release.yml")
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
	path := filepath.Join("..", ".goreleaser.yaml")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(contents)
}
