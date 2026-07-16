package devtool

import (
	"context"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

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

func TestDeveloperReleaseTargetsCoverMacAndLinuxOnArmAndAMD(t *testing.T) {
	want := []string{
		"hk-darwin-amd64",
		"hk-darwin-arm64",
		"hk-linux-amd64",
		"hk-linux-arm64",
	}
	var got []string
	for _, goos := range []string{"darwin", "linux"} {
		for _, goarch := range []string{"amd64", "arm64"} {
			artifact, err := developerReleaseArtifact("/release", goos, goarch)
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, filepath.Base(artifact))
		}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("developer release targets = %v, want %v", got, want)
	}
	for _, target := range [][2]string{{"windows", "amd64"}, {"linux", "386"}, {"freebsd", "arm64"}} {
		if _, err := developerReleaseArtifact("/release", target[0], target[1]); err == nil || !strings.Contains(err.Error(), "unsupported developer release target") {
			t.Fatalf("unsupported developer target was accepted: %s/%s (%v)", target[0], target[1], err)
		}
	}
}

func TestDeveloperReleaseBuildRejectsUnsafeVersionBeforeBuilding(t *testing.T) {
	root := initTestRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.BuildDeveloperBinary(context.Background(), ReleaseBuildRequest{Version: "v1; unsafe", GOOS: "linux", GOARCH: "amd64"}, io.Discard); err == nil {
		t.Fatal("unsafe developer release version was accepted")
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
