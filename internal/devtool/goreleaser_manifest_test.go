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
		"./hk catalog configuration-manifest",
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

func TestFilesystemLayoutManifestPinsBuildOwnershipSurfaces(t *testing.T) {
	root := filepath.Join("..", "..")
	contents, err := os.ReadFile(filepath.Join(root, "docs", "config-refactor", "filesystem-layout-manifest.md"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := string(contents)
	for _, row := range []string{
		"| `internal/devtool/catalog.go::variants` | Canonical developer build owner | Defines variant IDs, build tags, developer-container environment, local image names, and publishability metadata. Its cloud values are developer/build defaults, not runtime settings. |",
		"| `internal/devtool/app.go::App.ComposeEnvironment` | Workspace projection | Projects the selected variant plus workspace-scoped paths and ports into Compose variables; it does not define supported variants or production configuration precedence. |",
		"| `internal/devtool/runs.go::App.executeBuild` | Build orchestrator | Resolves a catalog variant, enforces the production/developer dependency boundary, and invokes the selected binary or image build without redefining tags or defaults. |",
		"| `Dockerfile` | Image-build consumer | Consumes explicit build arguments and produces the selected application image; it is not a configuration catalog or runtime parser. |",
		"| `.goreleaser.yaml` | Release-build projection | Maps the canonical self-hosted and cloud build identities to release tags, CGO targets, archive contents, and names; it must remain aligned with the developer catalog. |",
		"| `.github/workflows/pipeline.yml` | Delivery consumer | Supplies explicit version/ref inputs, restores verified assets, and invokes the canonical build projections. Publication and attestation policy lives here, but variant semantics do not. |",
	} {
		if count := strings.Count(manifest, row); count != 1 {
			t.Errorf("build ownership row appears %d times, want exactly once: %s", count, row)
		}
	}

	for path, anchors := range map[string][]string{
		"internal/devtool/catalog.go":    {"var variants = []Variant{"},
		"internal/devtool/app.go":        {"func (a *App) ComposeEnvironment(variant Variant) []string"},
		"internal/devtool/runs.go":       {"func (a *App) executeBuild(ctx context.Context, request RunRequest, writer io.Writer) error"},
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

// TestFilesystemLayoutManifestPinsReviewedFamilyRecords is a prose-presence sentinel, not semantic evidence validation.
func TestFilesystemLayoutManifestPinsReviewedFamilyRecords(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join("..", "..", "docs", "config-refactor", "filesystem-layout-manifest.md"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := string(contents)
	records := map[string][]string{
		"### `internal/devtool`":                                {"decomposition required / stay internal", "native PID/lock/process/toolchain/cancellation state"},
		"### `internal/importables`":                            {"`os.Open(source.Path)`", "`zip.OpenReader(source.Path)`", "untrusted staging/source-path containment boundary", "stay internal / blocked"},
		"### `internal/cluster`":                                {"the two direct importers", "memberlist", "in-memory", "stay internal / blocked"},
		"### `internal/mcpserver`":                              {"`internal/mcpserver` → `internal/mcpserver`; no move is approved", "the exact direct importer is `internal/server/server.go`", "91 affected symbols lower bound", "No Go import cycle is evidenced", "six production Go files and two test files", "owns no filesystem, subprocess, or package-local persistence boundary", "uses a 10-second timeout", "caps responses at 2 MiB", "46 `Test*` functions", "`user_id` and `owner_email`", "general non-default-tenant routing", "No compatibility shim or forwarding package is justified", "decomposition required / stay internal / blocked"},
		"### `internal/ingest`":                                 {"no direct filesystem operations found", "stay internal / blocked"},
		"### `internal/ipmeta` and `internal/ipmeta/ipmetagen`": {"embedded `io/fs`", "ordinary generated-file candidates", "stay internal / blocked"},
		"### `internal/mailables`":                              {"`billing` build tag", "stay internal / blocked"},
		"### `internal/mailer` and `internal/mailer/drivers`":   {"SMTP is a network boundary", "stay internal / blocked"},
		"### `internal/testutil`":                               {"Subrecord — `passkeys.go`", "no compatibility shim", "Subrecord — `testdb/testdb.go`", "Rollback: retain the existing native fixture path; no shim.", "stay internal / blocked"},
		"### `internal/socialauth`":                             {"`crypto/subtle`", "`Client.Complete` gives provider completion a 10s timeout", "custom Microsoft JWKS and GitHub JSON reads", "not general OIDC discovery or OAuth token exchange", "no compatibility shim", "stay internal / blocked"},
		"### `internal/sso`":                                    {"the ten direct importers", "validates DNS and dial targets as public", "self-hosted mode permits private identity providers", "both discovery and `Complete` token exchange/verification are bounded by `providerTimeout`", "AES-GCM", "HMAC-SHA256 domain separation", "timeout fix unless a separately approved compatibility/security decision", "stay internal / blocked"},
		"### `internal/aianalytics`":                            {"Runtime subrecord", "Updater subrecord", "successful body `Close` delegates to the underlying `resp.Body`", "Rollback for this documentation decision is removal of the record", "decomposition required / stay internal / blocked"},
		"### `internal/auth`":                                   {"`internal/auth` → `internal/auth` (no move)", "Combined package imports are stdlib `fmt`, `net/http`, `slices`, `sort`, `strings`, and `time`", "It imports no HitKeep package, so it introduces no internal dependency cycle.", "jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()})", "RenderTypeScriptCapabilities", "TestGeneratedTypeScriptCapabilitiesAreCurrent", "Retain the HS256 restriction", "separate approved compatibility decision", "stay internal / blocked"},
	}
	for heading, fragments := range records {
		if count := strings.Count(manifest, heading); count != 1 {
			t.Errorf("reviewed family heading %s appears %d times, want exactly once", heading, count)
			continue
		}
		section := manifest[strings.Index(manifest, heading)+len(heading):]
		if next := strings.Index(section, "\n### "); next >= 0 {
			section = section[:next]
		}
		for _, fragment := range fragments {
			if !strings.Contains(section, fragment) {
				t.Errorf("reviewed family %s is missing %q", heading, fragment)
			}
		}
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
