package devtool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDevelopmentToolVersionsAreCanonical(t *testing.T) {
	want := map[string]string{
		"golangci-lint": "2.13.0",
		"govulncheck":   "1.6.0",
		"staticcheck":   "0.8.0-rc.1",
		"zizmor":        "1.29.0",
	}
	for name, version := range want {
		if got := ToolVersion(name); got != version {
			t.Fatalf("ToolVersion(%q) = %q, want %q", name, got, version)
		}
	}

	staticcheck, err := GateByID("go-staticcheck")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(staticcheck.Command, " "), "@v"+want["staticcheck"]) {
		t.Fatalf("staticcheck gate does not use canonical version: %v", staticcheck.Command)
	}
	golangci, err := GateByID("go-lint")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(golangci.Command, " "), "@v"+want["golangci-lint"]) {
		t.Fatalf("golangci-lint gate does not use canonical version: %v", golangci.Command)
	}
	govulncheck, err := GateByID("govulncheck")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(govulncheck.Command, " "), "@v"+want["govulncheck"]) {
		t.Fatalf("govulncheck gate does not use canonical version: %v", govulncheck.Command)
	}
}

func TestWorkflowsReadCanonicalDevelopmentToolVersions(t *testing.T) {
	root := repositoryRoot(t)
	for _, workflow := range []string{"ci.yml", "zizmor.yml"} {
		raw, err := os.ReadFile(filepath.Join(root, ".github", "workflows", workflow))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), "internal/devtool/tool-versions.json") {
			t.Fatalf("%s does not read the canonical development tool versions", workflow)
		}
	}
}
