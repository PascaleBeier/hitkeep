package devtool

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var releaseValuePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,99}$`)
var buildTagPattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
var exactToolVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][A-Za-z0-9.-]+)?$`)

type ReleaseBuildRequest struct {
	Version string `json:"version"`
	GOOS    string `json:"goos"`
	GOARCH  string `json:"goarch"`
}

type ReleaseBuildResult struct {
	Artifacts []string `json:"artifacts"`
}

type GoBuildConfig struct {
	Variant      string   `json:"variant"`
	Tags         []string `json:"tags"`
	TagsCSV      string   `json:"tags_csv"`
	GOFLAGS      string   `json:"goflags"`
	GolangCIArgs string   `json:"golangci_args"`
}

type CIQAMatrix struct {
	Profile string   `json:"profile"`
	Prefix  string   `json:"prefix,omitempty"`
	Group   string   `json:"group,omitempty"`
	GateIDs []string `json:"gate_ids"`
}

type GoTestResult struct {
	Variant      string   `json:"variant"`
	Packages     []string `json:"packages"`
	PackageCount int      `json:"package_count"`
}

type ToolchainConfig struct {
	Go   string `json:"go"`
	Node string `json:"node"`
	NPM  string `json:"npm"`
}

func (a *App) ToolchainConfig() (ToolchainConfig, error) {
	config := ToolchainConfig{
		Go:   requiredVersion(a.workspace.Root, "go.mod", "go "),
		Node: requiredVersion(a.workspace.Root, filepath.Join("frontend", "dashboard", ".nvmrc"), ""),
		NPM:  requiredPackageManagerVersion(a.workspace.Root),
	}
	if config.Go == "" || config.Node == "" || config.NPM == "" {
		return ToolchainConfig{}, errors.New("repository toolchain version files are incomplete")
	}
	return config, nil
}

func (a *App) CIQAMatrix(profile, prefix, group string) (CIQAMatrix, error) {
	if !slices.Contains([]string{"pr", "full"}, profile) {
		return CIQAMatrix{}, fmt.Errorf("CI QA profile must be pr or full")
	}
	matrix := CIQAMatrix{Profile: profile, Prefix: prefix, Group: group}
	for _, gateID := range profileGateIDs(profile) {
		gate, _ := GateByID(gateID)
		if prefix != "" && !strings.HasPrefix(gateID, prefix) {
			continue
		}
		if group != "" && gate.CIGroup != group {
			continue
		}
		matrix.GateIDs = append(matrix.GateIDs, gateID)
	}
	if len(matrix.GateIDs) == 0 {
		return CIQAMatrix{}, fmt.Errorf("no %s gates match prefix %q and group %q", profile, prefix, group)
	}
	return matrix, nil
}

func (a *App) GoBuildConfig(variantID string, extraTags []string) (GoBuildConfig, error) {
	variant, err := VariantByID(variantID)
	if err != nil {
		return GoBuildConfig{}, err
	}
	for _, tag := range extraTags {
		if !buildTagPattern.MatchString(tag) {
			return GoBuildConfig{}, fmt.Errorf("invalid Go build tag %q", tag)
		}
	}
	tags := compactSorted(append(variant.BuildTags, extraTags...))
	csv := strings.Join(tags, ",")
	return GoBuildConfig{Variant: variant.ID, Tags: tags, TagsCSV: csv, GOFLAGS: "-tags=" + csv, GolangCIArgs: "--build-tags=" + csv}, nil
}

func (a *App) RunRaceShard(ctx context.Context, shard string, writer io.Writer) (RaceShardResult, error) {
	if !slices.Contains([]string{"heavy", "rest"}, shard) {
		return RaceShardResult{}, fmt.Errorf("unknown race shard %q", shard)
	}
	common, _ := VariantByID("self-hosted")
	packages, err := a.listGoPackages(ctx, common)
	if err != nil {
		return RaceShardResult{}, err
	}
	packages = partitionRacePackages(packages, shard)
	result := RaceShardResult{Shard: shard, Packages: packages, PackageCount: len(packages)}
	if len(packages) == 0 {
		return result, fmt.Errorf("race shard %q selected no packages", shard)
	}
	args := raceTestArgs(packages)
	if err := a.runCommand(ctx, writer, commandSpec{Args: args, Env: []string{"GOFLAGS=-tags=" + strings.Join(common.BuildTags, ",")}, Display: fmt.Sprintf("go test -race [%s shard: %d packages]", shard, len(packages))}); err != nil {
		return result, err
	}
	return result, nil
}

func raceTestArgs(packages []string) []string {
	// Keep the package timeout below the enclosing 30-minute gate timeout so a
	// genuine hang still emits a Go stack dump while normal slower race runs are
	// not constrained by Go's 10-minute default.
	return append([]string{"go", "test", "-race", "-timeout", "20m"}, packages...)
}

func (a *App) RunCloudTests(ctx context.Context, writer io.Writer) (GoTestResult, error) {
	cloud, _ := VariantByID("cloud")
	packages, err := a.listGoPackages(ctx, cloud)
	if err != nil {
		return GoTestResult{Variant: cloud.ID}, err
	}
	packages = cloudTestPackages(packages)
	result := GoTestResult{Variant: cloud.ID, Packages: packages, PackageCount: len(packages)}
	if len(packages) == 0 {
		return result, errors.New("cloud test selected no repository packages")
	}
	args := append([]string{"go", "test"}, packages...)
	environment := []string{"GOFLAGS=-tags=" + strings.Join(cloud.BuildTags, ",")}
	if err := a.runCommand(ctx, writer, commandSpec{Args: args, Env: environment, Display: fmt.Sprintf("go test [cloud variant: %d repository packages]", len(packages))}); err != nil {
		return result, err
	}
	return result, nil
}

func (a *App) listGoPackages(ctx context.Context, variant Variant) ([]string, error) {
	list := exec.CommandContext(ctx, "go", "list", "./...") //nolint:gosec // fixed command and arguments
	list.Dir = a.workspace.Root
	list.Env = a.commandEnvironment([]string{"GOFLAGS=-tags=" + strings.Join(variant.BuildTags, ",")})
	output, err := list.Output()
	if err != nil {
		return nil, fmt.Errorf("list %s Go packages: %w", variant.ID, err)
	}
	packages := make([]string, 0)
	for packageName := range strings.FieldsSeq(string(output)) {
		if strings.Contains(packageName, "/node_modules/") {
			continue
		}
		packages = append(packages, packageName)
	}
	return compactSorted(packages), nil
}

func cloudTestPackages(packages []string) []string {
	selected := make([]string, 0, len(packages))
	for _, packageName := range packages {
		if strings.Contains(packageName, "/node_modules/") || packageName == "hitkeep/cmd/hk" || packageName == developerPackagePrefix || strings.HasPrefix(packageName, developerPackagePrefix+"/") {
			continue
		}
		selected = append(selected, packageName)
	}
	return selected
}

func partitionRacePackages(packages []string, shard string) []string {
	heavyPrefixes := []string{"hitkeep/internal/database", "hitkeep/internal/mcpserver", "hitkeep/internal/mailer"}
	var selected []string
	for _, packageName := range packages {
		heavy := false
		for _, prefix := range heavyPrefixes {
			if packageName == prefix || strings.HasPrefix(packageName, prefix+"/") {
				heavy = true
				break
			}
		}
		if shard == "heavy" && heavy || shard == "rest" && !heavy {
			selected = append(selected, packageName)
		}
	}
	slices.Sort(selected)
	return selected
}

func (a *App) BuildReleaseBinaries(ctx context.Context, request ReleaseBuildRequest, writer io.Writer) (ReleaseBuildResult, error) {
	if !releaseValuePattern.MatchString(request.Version) {
		return ReleaseBuildResult{}, fmt.Errorf("invalid release version %q", request.Version)
	}
	if request.GOOS != "linux" {
		return ReleaseBuildResult{}, fmt.Errorf("unsupported release GOOS %q", request.GOOS)
	}
	if !slices.Contains([]string{"amd64", "arm64"}, request.GOARCH) {
		return ReleaseBuildResult{}, fmt.Errorf("unsupported release GOARCH %q", request.GOARCH)
	}
	if err := a.ValidateProductionBoundary(ctx); err != nil {
		return ReleaseBuildResult{}, err
	}
	common, _ := VariantByID("self-hosted")
	cloud, _ := VariantByID("cloud")
	artifacts := []string{
		filepath.Join(a.workspace.Root, "hitkeep-linux-"+request.GOARCH),
		filepath.Join(a.workspace.Root, "hitkeep-cloud-linux-"+request.GOARCH),
	}
	environment := []string{"CGO_ENABLED=1", "GOOS=" + request.GOOS, "GOARCH=" + request.GOARCH}
	for index, variant := range []Variant{common, cloud} {
		args := []string{
			"go", "build",
			"-tags", strings.Join(variant.BuildTags, " "),
			"-ldflags", "-w -s -X hitkeep/cmd.Version=" + request.Version,
			"-o", artifacts[index],
			"./cmd/hitkeep/main.go",
		}
		if err := a.runCommand(ctx, writer, commandSpec{Args: args, Env: environment}); err != nil {
			return ReleaseBuildResult{}, err
		}
	}
	return ReleaseBuildResult{Artifacts: artifacts}, nil
}
