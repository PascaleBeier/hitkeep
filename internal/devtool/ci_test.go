package devtool

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestToolchainConfigUsesCanonicalVersionFiles(t *testing.T) {
	root := initTestRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	dashboard := filepath.Join(root, "frontend", "dashboard")
	if err := os.MkdirAll(dashboard, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"go.mod":                          "module example.test\n\ngo 1.26.5\n",
		"frontend/dashboard/.nvmrc":       "24.18.0\n",
		"frontend/dashboard/package.json": `{"packageManager":"npm@12.0.2"}`,
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	config, err := app.ToolchainConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.Go != "1.26.5" || config.Node != "24.18.0" || config.NPM != "12.0.2" {
		t.Fatalf("unexpected toolchain: %+v", config)
	}
}

func TestFrontendAuditGateIsCanonicalStaticCheck(t *testing.T) {
	gate, err := GateByID("frontend-audit")
	if err != nil {
		t.Fatal(err)
	}
	if gate.CIGroup != "frontend-static" || !slices.Equal(gate.Command, []string{"npm", "audit", "--audit-level=high", "--no-fund"}) {
		t.Fatalf("unexpected frontend audit gate: %+v", gate)
	}
}

func TestReleaseBuildRejectsUnboundedInputs(t *testing.T) {
	root := initTestRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []ReleaseBuildRequest{
		{Version: "v1.0.0; touch nope", GOOS: "linux", GOARCH: "amd64"},
		{Version: "v1.0.0", GOOS: "darwin", GOARCH: "arm64"},
		{Version: "v1.0.0", GOOS: "linux", GOARCH: "386"},
	} {
		if _, err := app.BuildReleaseBinaries(context.Background(), request, io.Discard); err == nil {
			t.Fatalf("unsafe release request was accepted: %+v", request)
		}
	}
}

func TestRaceShardsAreDisjointAndComplete(t *testing.T) {
	packages := []string{
		"hitkeep/cmd/hitkeep",
		"hitkeep/internal/database",
		"hitkeep/internal/database/migrations",
		"hitkeep/internal/mailer",
		"hitkeep/internal/mcpserver",
		"hitkeep/internal/server",
	}
	heavy := partitionRacePackages(packages, "heavy")
	rest := partitionRacePackages(packages, "rest")
	for _, packageName := range heavy {
		if slices.Contains(rest, packageName) {
			t.Fatalf("package appears in both race shards: %s", packageName)
		}
	}
	combined := append(slices.Clone(heavy), rest...)
	slices.Sort(combined)
	slices.Sort(packages)
	if !slices.Equal(combined, packages) {
		t.Fatalf("race shards do not cover package catalog\nwant %v\n got %v", packages, combined)
	}
}

func TestCloudTestsExcludeDeveloperAndFrontendDependencyPackages(t *testing.T) {
	packages := []string{
		"hitkeep/cmd/hitkeep",
		"hitkeep/cmd/hk",
		"hitkeep/internal/database",
		"hitkeep/internal/devtool",
		"hitkeep/internal/devtool/cli",
		"hitkeep/frontend/dashboard/node_modules/flatted/golang/pkg/flatted",
		"hitkeep/skills",
	}
	want := []string{"hitkeep/cmd/hitkeep", "hitkeep/internal/database", "hitkeep/skills"}
	if got := cloudTestPackages(packages); !slices.Equal(got, want) {
		t.Fatalf("cloud packages = %v, want %v", got, want)
	}
}

func TestCIQAMatrixUsesCanonicalStableGateIDs(t *testing.T) {
	root := initTestRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	matrix, err := app.CIQAMatrix("pr", "", "go-race")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"go-race-heavy", "go-race-rest"}
	if !slices.Equal(matrix.GateIDs, want) {
		t.Fatalf("race matrix = %v, want %v", matrix.GateIDs, want)
	}
	if matrix.Group != "go-race" {
		t.Fatalf("matrix group = %q, want go-race", matrix.Group)
	}
}

func TestRaceTestArgsUseGateBoundedPackageTimeout(t *testing.T) {
	got := raceTestArgs([]string{"hitkeep/internal/database"})
	want := []string{"go", "test", "-race", "-timeout", "20m", "hitkeep/internal/database"}
	if !slices.Equal(got, want) {
		t.Fatalf("race arguments = %v, want %v", got, want)
	}
}

func TestDefaultTenantMigrationAcceptanceIsFullProfileOnly(t *testing.T) {
	gate, err := GateByID("default-tenant-migration-acceptance")
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(gate.Profiles, "pr") || !slices.Contains(gate.Profiles, "full") {
		t.Fatalf("acceptance profiles = %v, want full only", gate.Profiles)
	}
	if !slices.Contains(gate.Command, "HITKEEP_DEFAULT_TENANT_MIGRATION_ACCEPTANCE=1") {
		t.Fatalf("acceptance command does not enable its opt-in tests: %v", gate.Command)
	}
}
