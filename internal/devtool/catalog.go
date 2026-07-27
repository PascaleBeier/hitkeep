package devtool

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
)

var variants = []Variant{
	{
		ID:                  "self-hosted",
		Description:         "Public self-hosted HitKeep build",
		BuildTags:           []string{"hashicorpmetrics", "timetzdata"},
		LocalImage:          "ghcr.io/pascalebeier/hitkeep:snapshot",
		Publishable:         true,
		ProductionImageOnly: false,
	},
	{
		ID:          "cloud",
		Description: "Managed-cloud parity build for local use",
		BuildTags:   []string{"hashicorpmetrics", "timetzdata", "s3", "billing", "tenancy"},
		Environment: map[string]string{
			"HITKEEP_CLOUD_HOSTED":               "true",
			"HITKEEP_CLOUD_SIGNUP_ENABLED":       "true",
			"HITKEEP_CLOUD_JURISDICTION":         "EU",
			"HITKEEP_CLOUD_REGION":               "eu-central-1",
			"HITKEEP_CLOUD_UPGRADE_URL":          "http://localhost:4200/admin/team",
			"HITKEEP_CLOUD_SUPPORT_URL":          "https://hitkeep.com/support/help/",
			"HITKEEP_CLOUD_CHECKOUT_SUCCESS_URL": "http://localhost:4200/admin/team?checkout=success",
			"HITKEEP_CLOUD_CHECKOUT_CANCEL_URL":  "http://localhost:4200/admin/team?checkout=cancelled",
		},
		LocalImage:          "hitkeep:cloud-local",
		Publishable:         false,
		ProductionImageOnly: true,
	},
}

var gates = []Gate{
	{ID: "go-format", Description: "Check repository Go formatting without modifying files", CIGroup: "go-checks", Command: []string{"./hk", "fmt", "check"}, Profiles: []string{"pr", "full"}, Paths: []string{"*.go"}, Weight: 1, Timeout: "5m"},
	{ID: "go-fix", Description: "Check pinned Go source migrations without modifying files", CIGroup: "go-checks", Command: []string{"./hk", "fix", "check"}, Profiles: []string{"pr", "full"}, Paths: []string{"*.go", "go.mod", "go.sum"}, Weight: 2, Timeout: "10m"},
	{ID: "go-lint", Description: "Lint Go code with the pinned CI configuration", CIGroup: "golangci", Command: []string{"golangci-lint", "run"}, Profiles: []string{"pr", "full"}, Paths: []string{"*.go", "go.mod", "go.sum", ".golangci.yml"}, Weight: 2, Timeout: "10m"},
	{ID: "go-vet", Description: "Run go vet with self-hosted tags", CIGroup: "go-checks", Command: []string{"go", "vet", "./..."}, Profiles: []string{"pr", "full"}, Paths: []string{"*.go", "go.mod", "go.sum"}, Weight: 2, Timeout: "10m"},
	{ID: "go-staticcheck", Description: "Run pinned Staticcheck", CIGroup: "go-checks", Command: []string{"go", "run", "honnef.co/go/tools/cmd/staticcheck@v" + ToolVersion("staticcheck"), "./..."}, Profiles: []string{"pr", "full"}, Paths: []string{"*.go", "go.mod", "go.sum"}, Weight: 2, Timeout: "15m"},
	{ID: "go-race-heavy", Description: "Run race tests for the canonical heavy Go package shard", CIGroup: "go-race", Command: []string{"./hk", "ci", "race", "--shard", "heavy"}, Profiles: []string{"pr", "full"}, Paths: []string{"*.go", "*.sql", "go.mod", "go.sum"}, Weight: 4, Timeout: "30m"},
	{ID: "go-race-rest", Description: "Run race tests for the complementary Go package shard", CIGroup: "go-race", Command: []string{"./hk", "ci", "race", "--shard", "rest"}, Profiles: []string{"pr", "full"}, Paths: []string{"*.go", "*.sql", "go.mod", "go.sum"}, Weight: 4, Timeout: "30m"},
	{ID: "default-tenant-migration-acceptance", Description: "Run exhaustive default-tenant split, SQLite conversion, publication, and attached-data-plane acceptance tests", Command: []string{"env", "HITKEEP_DEFAULT_TENANT_MIGRATION_ACCEPTANCE=1", "go", "test", "./internal/controlstore", "./internal/database", "./internal/migrationacceptance", "-run", "Test(DefaultTenantSplitFaultBoundariesResume|SplitFingerprintSurvivesLargeParquetRoundTrip|TenantStoreManagerUsesOneAttachedDataPlane|LegacyTenantAnalyticsDrainMovesMissingRowsAndIsIdempotent|PublishLegacyConversionRetainsSourceAndPublishesSQLite|PublishLegacyConversionCompletesAfterSourceRename|InspectPublicationRejectsChecksumConflict|InspectPublicationRejectsPublishedSQLiteWithChangedEvidence|InspectPublicationRebuildsMissingOrMalformedWorkFromRetainedEvidence|ImportLegacyCopiesAndVerifiesClosedRegistry)$"}, Profiles: []string{"full"}, Paths: []string{"internal/controlstore/", "internal/database/", "internal/migrationacceptance/", "internal/devtool/", ".github/workflows/"}, Weight: 4, Timeout: "30m"},
	{ID: "mcp-audit", Description: "Audit the production MCP protocol surface", CIGroup: "production-mcp", Command: []string{"go", "test", "./internal/mcpserver", "-run", "TestMCP.*Audit"}, Profiles: []string{"pr", "full"}, Paths: []string{"internal/mcpserver/", "internal/analyticstools/", "skills/", "server.json"}, Weight: 2, Timeout: "10m"},
	{ID: "mcp-schema", Description: "Validate MCP registry metadata", CIGroup: "production-mcp", Command: []string{"tests/scripts/mcp-audit.sh", "--schema-only"}, Profiles: []string{"pr", "full"}, Paths: []string{"internal/mcpserver/", "tests/scripts/mcp-audit.sh", "server.json"}, Weight: 1, Timeout: "5m"},
	{ID: "developer-mcp", Description: "Validate the developer CLI/MCP contract and production binary boundary", CIGroup: "go-checks", Command: []string{"go", "test", "./cmd/hk", "./internal/devtool/devmcp"}, Profiles: []string{"pr", "full"}, Paths: []string{"cmd/hk/", "cmd/hitkeep/", "internal/devtool/", "hk", "go.mod", "go.sum"}, Weight: 1, Timeout: "5m"},
	{ID: "developer-docs", Description: "Check CLI, release and configuration metadata, canonical skill packs, analytics procedures, and documentation drift", CIGroup: "go-checks", Command: []string{"go", "test", "./internal/devtool", "./skills"}, Profiles: []string{"pr", "full"}, Paths: []string{"AGENTS.md", "CONTRIBUTING.md", "README.md", "Dockerfile", "compose.yaml", "compose.cluster.yaml", "compose.dev.yaml", "examples/", ".agents/", ".codex/", ".github/actions/", ".github/workflows/", ".release-please-manifest.json", "release-please-config.json", "server.json", "skills/", "internal/config/", "internal/devtool/", "frontend/dashboard/.npmrc", "frontend/dashboard/.nvmrc", "frontend/dashboard/package.json", "frontend/dashboard/package-lock.json", "frontend/dashboard/src/tracker/version.ts", "frontend/tracker/package.json", "charts/hitkeep/"}, Weight: 1, Timeout: "5m"},
	{ID: "frontend-format", Description: "Check dashboard formatting", CIGroup: "frontend-static", Command: []string{"npm", "run", "fmt:check"}, WorkingDir: "frontend/dashboard", Profiles: []string{"pr", "full"}, Paths: []string{"frontend/dashboard/"}, Weight: 1, Timeout: "5m"},
	{ID: "frontend-audit", Description: "Audit dashboard npm dependencies for high-severity vulnerabilities", CIGroup: "frontend-static", Command: []string{"npm", "audit", "--audit-level=high", "--no-fund"}, WorkingDir: "frontend/dashboard", Profiles: []string{"pr", "full"}, Paths: []string{"frontend/dashboard/.npmrc", "frontend/dashboard/package.json", "frontend/dashboard/package-lock.json"}, Weight: 1, Timeout: "5m"},
	{ID: "frontend-lint", Description: "Lint the Angular dashboard", CIGroup: "frontend-static", Command: []string{"npm", "run", "lint"}, WorkingDir: "frontend/dashboard", Profiles: []string{"pr", "full"}, Paths: []string{"frontend/dashboard/"}, Weight: 2, Timeout: "10m"},
	{ID: "frontend-i18n", Description: "Validate all dashboard locales", CIGroup: "frontend-static", Command: []string{"npm", "run", "i18n:check"}, WorkingDir: "frontend/dashboard", Profiles: []string{"pr", "full"}, Paths: []string{"frontend/dashboard/public/i18n/", "frontend/dashboard/src/"}, Weight: 1, Timeout: "5m"},
	{ID: "tracker-package", Description: "Build and verify the @hitkeep/tracker npm package", CIGroup: "frontend-static", Command: []string{"npm", "run", "verify:tracker-package"}, WorkingDir: "frontend/dashboard", Profiles: []string{"pr", "full"}, Paths: []string{"frontend/dashboard/src/tracker/", "frontend/dashboard/scripts/", "frontend/dashboard/tsconfig.tracker-package.json", "frontend/tracker/"}, Weight: 2, Timeout: "10m"},
	{ID: "frontend-unit", Description: "Run dashboard unit tests", CIGroup: "frontend-unit", Command: []string{"npm", "run", "test:ci"}, WorkingDir: "frontend/dashboard", Profiles: []string{"pr", "full"}, Paths: []string{"frontend/dashboard/"}, Weight: 3, Timeout: "20m"},
	{ID: "frontend-e2e", Description: "Run seeded Playwright end-to-end tests", CIGroup: "frontend-e2e", Command: []string{"npm", "run", "e2e"}, WorkingDir: "frontend/dashboard", Profiles: []string{"pr", "full"}, Paths: []string{"frontend/dashboard/", "cmd/", "internal/", "tests/e2e/"}, Weight: 4, Timeout: "30m"},
	{ID: "govulncheck", Description: "Scan Go dependencies for reachable vulnerabilities", CIGroup: "govulncheck", Command: []string{"go", "run", "golang.org/x/vuln/cmd/govulncheck@v" + ToolVersion("govulncheck"), "./..."}, Profiles: []string{"pr", "full"}, Paths: []string{"*.go", "go.mod", "go.sum"}, Weight: 2, Timeout: "15m"},
	{ID: "zizmor", Description: "Audit GitHub Actions workflows", CIGroup: "zizmor", Command: []string{"zizmor", ".github"}, Profiles: []string{"pr", "full"}, Paths: []string{".github/actions/", ".github/workflows/"}, Weight: 1, Timeout: "10m"},
	{ID: "cloud-build", Description: "Build the cloud-tagged binary", Command: []string{"./hk", "ci", "verify-build", "--variant", "cloud"}, Profiles: []string{"full"}, Paths: []string{"*.go", "go.mod", "go.sum", "Dockerfile"}, Weight: 3, Timeout: "20m"},
	{ID: "cloud-test", Description: "Run cloud-tagged tests for repository packages outside the developer platform", Command: []string{"./hk", "ci", "cloud-test"}, Profiles: []string{"full"}, Paths: []string{"*.go", "*.sql", "go.mod", "go.sum"}, Weight: 4, Timeout: "30m"},
	{ID: "self-hosted-image", Description: "Build and smoke-test the self-hosted production image", Command: []string{"docker", "buildx", "build", ".", "--target", "local-image", "--load"}, Profiles: []string{"full"}, Paths: []string{"Dockerfile", ".dockerignore", "cmd/", "internal/", "frontend/"}, Weight: 4, Timeout: "35m"},
	{ID: "cloud-image", Description: "Build and smoke-test the local-only cloud production image", Command: []string{"docker", "buildx", "build", ".", "--target", "local-image", "--load"}, Profiles: []string{"full"}, Paths: []string{"Dockerfile", ".dockerignore", "cmd/", "internal/", "frontend/"}, Weight: 4, Timeout: "35m"},
}

func CatalogSnapshot() Catalog {
	catalog := Catalog{SchemaVersion: SchemaVersion, Variants: cloneVariants(), Gates: cloneGates(), Profiles: []string{"changed", "pr", "full"}}
	for index := range catalog.Gates {
		agentCommand := agentOptimizedCommand(catalog.Gates[index].Command)
		if !slices.Equal(agentCommand, catalog.Gates[index].Command) {
			catalog.Gates[index].AgentCommand = agentCommand
		}
	}
	return catalog
}

func VariantByID(id string) (Variant, error) {
	for _, variant := range variants {
		if variant.ID == id {
			return cloneVariant(variant), nil
		}
	}
	return Variant{}, fmt.Errorf("unknown variant %q", id)
}

func GateByID(id string) (Gate, error) {
	for _, gate := range gates {
		if gate.ID == id {
			return cloneGate(gate), nil
		}
	}
	return Gate{}, fmt.Errorf("unknown gate %q", id)
}

func ValidateRunRequest(request RunRequest) error {
	switch request.Kind {
	case "setup":
		return nil
	case "build", "smoke":
		if request.Variant == "" {
			request.Variant = "self-hosted"
		}
		if _, err := VariantByID(request.Variant); err != nil {
			return err
		}
		if request.Kind == "build" && request.Target != "binary" && request.Target != "image" {
			return errors.New("build target must be binary or image")
		}
		return nil
	case "qa":
		if !slices.Contains([]string{"changed", "pr", "full"}, request.Profile) {
			return errors.New("qa profile must be changed, pr, or full")
		}
		for _, id := range request.GateIDs {
			if _, err := GateByID(id); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported run kind %q", request.Kind)
	}
}

func finiteRunKind(kind string) bool {
	switch kind {
	case "setup", "qa", "build", "smoke":
		return true
	default:
		return false
	}
}

func profileGateIDs(profile string) []string {
	var ids []string
	for _, gate := range gates {
		if slices.Contains(gate.Profiles, profile) {
			ids = append(ids, gate.ID)
		}
	}
	return ids
}

func gateMatchesPath(gate Gate, path string) bool {
	for _, pattern := range gate.Paths {
		if strings.HasSuffix(pattern, "/") && strings.HasPrefix(path, pattern) {
			return true
		}
		if strings.HasPrefix(pattern, "*.") && strings.HasSuffix(path, strings.TrimPrefix(pattern, "*")) {
			return true
		}
		if path == pattern {
			return true
		}
	}
	return false
}

func cloneVariants() []Variant {
	out := make([]Variant, 0, len(variants))
	for _, variant := range variants {
		out = append(out, cloneVariant(variant))
	}
	return out
}

func cloneVariant(variant Variant) Variant {
	variant.BuildTags = slices.Clone(variant.BuildTags)
	variant.Environment = cloneMap(variant.Environment)
	return variant
}

func cloneGates() []Gate {
	out := make([]Gate, 0, len(gates))
	for _, gate := range gates {
		out = append(out, cloneGate(gate))
	}
	return out
}

func cloneGate(gate Gate) Gate {
	gate.Command = slices.Clone(gate.Command)
	gate.AgentCommand = slices.Clone(gate.AgentCommand)
	gate.Profiles = slices.Clone(gate.Profiles)
	gate.Paths = slices.Clone(gate.Paths)
	return gate
}

func cloneMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	maps.Copy(out, in)
	return out
}
