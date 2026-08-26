package devtool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	qaPlannerVersion = "2"
	qaCatalogVersion = "2"
)

const (
	changeBackend       = "backend"
	changeDatabase      = "database"
	changeAPI           = "api-contract"
	changeDashboard     = "dashboard"
	changeTracker       = "tracker"
	changeProductionMCP = "production-mcp"
	changeDeveloper     = "developer-platform"
	changeDocumentation = "documentation"
	changeDelivery      = "delivery"
	changeDependencies  = "dependencies"
)

func applyGateMetadata(gate *Gate) {
	gate.ContractVersion = "1"
	gate.Volatility = "deterministic"
	gate.ReuseTTL = "24h"
	gate.Depth = "complete"
	switch gate.ID {
	case "go-format", "go-fix", "go-lint", "go-vet", "go-staticcheck", "govulncheck":
		gate.ChangeAreas = []string{changeBackend, changeDatabase, changeAPI, changeProductionMCP, changeDeveloper, changeDependencies}
	case "go-race-database":
		gate.ChangeAreas = []string{changeDatabase}
	case "go-race-server":
		gate.ChangeAreas = []string{changeBackend, changeAPI, changeProductionMCP}
	case "go-race-rest":
		gate.ChangeAreas = []string{changeBackend, changeDeveloper}
	case "default-tenant-migration-acceptance":
		gate.ChangeAreas = []string{changeDatabase}
		gate.Volatility = "integration"
		gate.ReuseTTL = "2h"
	case "mcp-audit", "mcp-schema":
		gate.ChangeAreas = []string{changeProductionMCP}
	case "developer-mcp":
		gate.ChangeAreas = []string{changeDeveloper}
	case "developer-docs":
		gate.ChangeAreas = []string{changeDeveloper, changeDocumentation, changeDelivery, changeDependencies}
	case "frontend-format", "frontend-audit", "frontend-lint", "frontend-i18n", "frontend-unit":
		gate.ChangeAreas = []string{changeDashboard}
	case "tracker-package":
		gate.ChangeAreas = []string{changeTracker}
	case "frontend-e2e":
		gate.ChangeAreas = []string{changeDashboard, changeAPI}
		gate.Volatility = "integration"
		gate.ReuseTTL = "2h"
	case "zizmor":
		gate.ChangeAreas = []string{changeDelivery}
	case "cloud-build", "cloud-test", "self-hosted-image", "cloud-image":
		gate.ChangeAreas = []string{changeDelivery}
		gate.Volatility = "release"
		gate.ReuseTTL = ""
	}
	if slices.Contains([]string{"go-format", "go-fix", "go-lint", "go-vet", "go-staticcheck", "developer-mcp", "developer-docs", "frontend-format", "frontend-audit", "frontend-lint", "frontend-i18n", "frontend-unit", "tracker-package", "mcp-audit", "mcp-schema"}, gate.ID) {
		gate.Depth = "changed"
	}
}

func classifyChangedPath(path string) ([]string, bool) {
	path = filepath.ToSlash(strings.TrimSpace(path))
	switch {
	case path == "go.mod" || path == "go.sum":
		return []string{changeDependencies}, true
	case path == "hitkeep.example.yaml":
		return []string{changeBackend, changeDocumentation}, true
	case path == "frontend/dashboard/package.json" || path == "frontend/dashboard/package-lock.json":
		return []string{changeDependencies, changeDashboard}, true
	case strings.HasPrefix(path, "internal/database/"):
		return []string{changeDatabase}, true
	case strings.HasPrefix(path, "internal/mcpserver/") || strings.HasPrefix(path, "internal/analyticstools/") || strings.HasPrefix(path, "skills/") || path == "server.json":
		return []string{changeProductionMCP}, true
	case strings.HasPrefix(path, "internal/devtool/") || strings.HasPrefix(path, "cmd/hk/") || path == "hk" || strings.HasPrefix(path, ".agents/"):
		return []string{changeDeveloper}, true
	case strings.HasPrefix(path, "internal/api/") || strings.Contains(path, "openapi"):
		return []string{changeAPI}, true
	case strings.HasPrefix(path, "frontend/dashboard/src/tracker/") || strings.HasPrefix(path, "frontend/tracker/"):
		return []string{changeTracker}, true
	case strings.HasPrefix(path, "frontend/dashboard/"):
		return []string{changeDashboard}, true
	case strings.HasPrefix(path, "docs/") || path == "README.md" || path == "CONTRIBUTING.md" || path == "AGENTS.md":
		return []string{changeDocumentation}, true
	case strings.HasPrefix(path, ".github/") || strings.HasPrefix(path, "charts/") || path == "Dockerfile" || path == "scripts/docker-smoke.sh" || strings.HasPrefix(path, "compose"):
		return []string{changeDelivery}, true
	case strings.HasSuffix(path, ".go") || strings.HasSuffix(path, ".sql") || strings.HasPrefix(path, "cmd/") || strings.HasPrefix(path, "internal/"):
		return []string{changeBackend}, true
	default:
		return nil, false
	}
}

func (a *App) buildQAPlan(ctx context.Context, profile, baseRef string) (QAPlan, error) {
	if !slices.Contains([]string{"changed", "complete", "pr", "full"}, profile) {
		return QAPlan{}, fmt.Errorf("unknown QA profile %q", profile)
	}
	if baseRef == "" {
		baseRef = "origin/main"
	}
	workspace, err := a.Workspace(ctx)
	if err != nil {
		return QAPlan{}, err
	}
	plan := QAPlan{Profile: profile, BaseRef: baseRef, PlannerVersion: qaPlannerVersion, CatalogVersion: qaCatalogVersion}
	if profile == "pr" || profile == "full" {
		plan.GateIDs = profileGateIDs(profile)
		plan.SourceSnapshot, err = qaSourceSnapshot(a.workspace.Root, workspace.Head, nil)
	} else {
		changed, changeErr := changedPaths(a.workspace.Root, baseRef)
		if changeErr != nil {
			changed, changeErr = workingTreeChangedPaths(a.workspace.Root)
		}
		if changeErr != nil {
			return QAPlan{}, changeErr
		}
		plan.ChangedPathCount = len(changed)
		plan.ChangedPaths, plan.ChangedPathsTruncated = boundedStrings(changed, maxStructuredPaths)
		areas := map[string]bool{}
		for _, path := range changed {
			pathAreas, known := classifyChangedPath(path)
			if !known {
				plan.DecisionRequired = true
				plan.DecisionReason = "unclassified_path"
				continue
			}
			for _, area := range pathAreas {
				areas[area] = true
			}
		}
		if !plan.DecisionRequired {
			for _, gate := range CatalogSnapshot().Gates {
				if profile == "changed" && gate.Depth != "changed" {
					plan.SkippedGateIDs = append(plan.SkippedGateIDs, gate.ID)
					continue
				}
				if profile == "complete" && !slices.Contains(gate.Profiles, "pr") {
					plan.SkippedGateIDs = append(plan.SkippedGateIDs, gate.ID)
					continue
				}
				selected := false
				for _, area := range gate.ChangeAreas {
					selected = selected || areas[area]
				}
				if selected {
					plan.GateIDs = append(plan.GateIDs, gate.ID)
				} else {
					plan.SkippedGateIDs = append(plan.SkippedGateIDs, gate.ID)
				}
			}
		}
		plan.SourceSnapshot, err = qaSourceSnapshot(a.workspace.Root, workspace.Head, changed)
	}
	if err != nil {
		return QAPlan{}, err
	}
	plan.PlanID = qaPlanID(plan)
	planPath := filepath.Join(a.workspace.StateDir, "qa-plans", plan.PlanID+".json")
	if err := os.MkdirAll(filepath.Dir(planPath), 0o755); err != nil {
		return QAPlan{}, fmt.Errorf("create QA plan directory: %w", err)
	}
	if err := writeJSONAtomic(planPath, plan); err != nil {
		return QAPlan{}, fmt.Errorf("persist QA plan: %w", err)
	}
	return plan, nil
}

func qaSourceSnapshot(root, head string, paths []string) (string, error) {
	hash := sha256.New()
	fmt.Fprintf(hash, "head=%s\n", head)
	paths = slices.Clone(paths)
	slices.Sort(paths)
	for _, path := range paths {
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(path)))
		if os.IsNotExist(err) {
			fmt.Fprintf(hash, "%s:deleted\n", path)
			continue
		}
		if err != nil {
			return "", err
		}
		fmt.Fprintf(hash, "%s:%d:%d:%d\n", path, info.Size(), info.ModTime().UnixNano(), info.Mode())
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (a *App) prepareQARequest(ctx context.Context, request RunRequest) (RunRequest, error) {
	if request.PlanID == "" {
		return RunRequest{}, fmt.Errorf("qa plan_id is required")
	}
	path := filepath.Join(a.workspace.StateDir, "qa-plans", request.PlanID+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return RunRequest{}, fmt.Errorf("load QA plan: %w", err)
	}
	var plan QAPlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return RunRequest{}, fmt.Errorf("decode QA plan: %w", err)
	}
	if plan.PlanID != request.PlanID || plan.PlannerVersion != qaPlannerVersion || plan.CatalogVersion != qaCatalogVersion {
		return RunRequest{}, fmt.Errorf("qa_plan_stale: plan or catalog version changed")
	}
	if plan.DecisionRequired {
		return RunRequest{}, fmt.Errorf("decision_required: %s", plan.DecisionReason)
	}
	workspace, err := a.Workspace(ctx)
	if err != nil {
		return RunRequest{}, err
	}
	current, err := qaSourceSnapshot(a.workspace.Root, workspace.Head, plan.ChangedPaths)
	if err != nil {
		return RunRequest{}, err
	}
	if current != plan.SourceSnapshot {
		return RunRequest{}, fmt.Errorf("qa_plan_stale: source changed after planning")
	}
	request.Profile = plan.Profile
	if len(request.GateIDs) == 0 {
		request.GateIDs = slices.Clone(plan.GateIDs)
		return request, nil
	}
	for _, gateID := range request.GateIDs {
		if !slices.Contains(plan.GateIDs, gateID) {
			return RunRequest{}, fmt.Errorf("qa gate %q is not selected by plan %s", gateID, plan.PlanID)
		}
	}
	return request, nil
}

func qaPlanID(plan QAPlan) string {
	hash := sha256.New()
	fmt.Fprintf(hash, "%s\n%s\n%s\n%s\n%s\n", plan.Profile, plan.BaseRef, plan.SourceSnapshot, plan.PlannerVersion, plan.CatalogVersion)
	for _, gate := range plan.GateIDs {
		fmt.Fprintln(hash, gate)
	}
	return hex.EncodeToString(hash.Sum(nil))[:24]
}
