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
	out, err := exec.CommandContext(t.Context(), "go", "list", "-deps", "hitkeep/cmd/hitkeep").CombinedOutput()
	if err != nil {
		t.Fatalf("go list production deps: %v\n%s", err, out)
	}

	deps := map[string]bool{}
	for dep := range strings.FieldsSeq(string(out)) {
		deps[dep] = true
	}
	for _, unwanted := range []string{
		"hitkeep/cmd/ipmeta-generate",
		"hitkeep/internal/ipmeta/ipmetagen",
		"github.com/DataDog/zstd",
		"github.com/ip2location/ip2location-go/v9",
		"lukechampine.com/uint128",
	} {
		if deps[unwanted] {
			t.Fatalf("production command must not depend on generator package %s", unwanted)
		}
	}
}

func TestSpamFeedPackageDoesNotDependOnDatabase(t *testing.T) {
	out, err := exec.CommandContext(t.Context(), "go", "list", "-deps", "hitkeep/internal/blocking/spamfeed").CombinedOutput()
	if err != nil {
		t.Fatalf("go list spam feed deps: %v\n%s", err, out)
	}
	for dep := range strings.FieldsSeq(string(out)) {
		if dep == "hitkeep/internal/database" || strings.Contains(dep, "duckdb") {
			t.Fatalf("spam feed package must not depend on %s", dep)
		}
	}
}

func TestMaintenanceDependencyBoundaries(t *testing.T) {
	for _, tc := range []struct {
		path      string
		forbidden []string
	}{
		{path: "hitkeep/cmd/data-refresh", forbidden: []string{"hitkeep/cmd", "hitkeep/internal/server", "hitkeep/internal/database", "github.com/duckdb/duckdb-go/v2"}},
		{path: "hitkeep/internal/duckdbextensions/update", forbidden: []string{"hitkeep/internal/ipmeta/ipmetagen", "github.com/DataDog/zstd", "github.com/ip2location/ip2location-go/v9"}},
	} {
		out, err := exec.CommandContext(t.Context(), "go", "list", "-deps", tc.path).CombinedOutput()
		if err != nil {
			t.Fatalf("go list %s: %v\n%s", tc.path, err, out)
		}
		for dep := range strings.FieldsSeq(string(out)) {
			for _, unwanted := range tc.forbidden {
				if dep == unwanted || (unwanted != "hitkeep/cmd" && strings.HasPrefix(dep, unwanted+"/")) {
					t.Fatalf("%s must not depend on %s", tc.path, dep)
				}
			}
		}
	}
}

func TestProductionBinaryDoesNotContainGeneratorTokenPlumbing(t *testing.T) {
	binaryPath := filepath.Join(t.TempDir(), "hitkeep")
	out, err := exec.CommandContext(t.Context(), "go", "build", "-o", binaryPath, "hitkeep/cmd/hitkeep").CombinedOutput()
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

func TestDataRefreshWorkflowContracts(t *testing.T) {
	workflow, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "data-refresh.yml"))
	if err != nil {
		t.Fatalf("read data refresh workflow: %v", err)
	}
	contents := string(workflow)
	for _, required := range []string{
		"workflow_dispatch:",
		"persist-credentials: false",
		"fail-fast: false",
		"cancel-in-progress: false",
		"target: ai-agents",
		"target: spam",
		"target: ipmeta",
		"target: duckdb",
		"run: go run ./cmd/data-refresh ai-agents",
		"run: go run ./cmd/data-refresh spam",
		"run: go run ./cmd/data-refresh ipmeta",
		"IP2LOCATION_DOWNLOAD_TOKEN: ${{ secrets.IP2LOCATION_DOWNLOAD_TOKEN }}",
		"run: go run ./cmd/data-refresh duckdb",
		"run: go run ./internal/duckdbextensions/update -check",
		"GH_TOKEN: ${{ secrets.GHT }}",
		"git status --porcelain=v1 -z --untracked-files=all",
		"--force-with-lease=refs/heads/$BRANCH:",
		"--body-file",
	} {
		if !strings.Contains(contents, required) {
			t.Fatalf("data refresh workflow must contain %q", required)
		}
	}
	for _, forbidden := range []string{
		"-ip2location-token",
		"git add internal/ipmeta",
		"git push --force origin",
	} {
		if strings.Contains(contents, forbidden) {
			t.Fatalf("data refresh workflow must not contain %q", forbidden)
		}
	}
	if strings.Count(contents, "secrets.GHT") != 1 {
		t.Fatal("PR credential must be scoped to one publication step")
	}
}
