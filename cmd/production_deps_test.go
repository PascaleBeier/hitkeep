package hitkeepcmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionCommandDoesNotDependOnGenerators(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "hitkeep/cmd/hitkeep").CombinedOutput()
	if err != nil {
		t.Fatalf("go list production deps: %v\n%s", err, out)
	}

	deps := map[string]bool{}
	for _, dep := range strings.Fields(string(out)) {
		deps[dep] = true
	}
	for _, unwanted := range []string{
		"hitkeep/cmd/ipmeta-generate",
		"hitkeep/internal/ipmeta/ipmetagen",
		"github.com/ip2location/ip2location-go/v9",
		"lukechampine.com/uint128",
	} {
		if deps[unwanted] {
			t.Fatalf("production command must not depend on generator package %s", unwanted)
		}
	}
}

func TestProductionBinaryDoesNotContainGeneratorTokenPlumbing(t *testing.T) {
	binaryPath := filepath.Join(t.TempDir(), "hitkeep")
	out, err := exec.Command("go", "build", "-o", binaryPath, "hitkeep/cmd/hitkeep").CombinedOutput()
	if err != nil {
		t.Fatalf("build production command: %v\n%s", err, out)
	}
	binary, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("read production binary: %v", err)
	}

	for _, unwanted := range []string{
		"cmd/ipmeta-generate",
		"ipmetagen",
		"ip2location-go",
		"IP2LOCATION_DOWNLOAD_TOKEN",
		"DB3LITEBINIPV6",
		"DBASNLITEBINIPV6",
	} {
		if bytes.Contains(binary, []byte(unwanted)) {
			t.Fatalf("production binary must not contain generator/token string %q", unwanted)
		}
	}
}

func TestIPMetadataRefreshWorkflowUsesGitHubSecretToken(t *testing.T) {
	workflow, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "ipmeta-refresh.yml"))
	if err != nil {
		t.Fatalf("read IP metadata refresh workflow: %v", err)
	}
	contents := string(workflow)
	for _, required := range []string{
		`cron: "0 19 1,15 * *"`,
		"IP2LOCATION_DOWNLOAD_TOKEN: ${{ secrets.IP2LOCATION_DOWNLOAD_TOKEN }}",
		"run: go run ./cmd/ipmeta-generate",
	} {
		if !strings.Contains(contents, required) {
			t.Fatalf("IP metadata refresh workflow must contain %q", required)
		}
	}
	if strings.Contains(contents, "-ip2location-token") {
		t.Fatal("IP metadata refresh workflow must not pass the download token on the command line")
	}
}
