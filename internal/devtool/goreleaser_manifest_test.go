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

func readGoReleaserManifest(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", ".goreleaser.yaml")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(contents)
}
