package devtool

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestCompletePlanForBackendChangeContainsNoFrontendGates(t *testing.T) {
	root := initTestRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	path := filepath.Join(root, "internal", "server", "planner_acceptance.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package server\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := app.QAPlan(context.Background(), "complete", "origin/main")
	if err != nil {
		t.Fatal(err)
	}
	if plan.PlanID == "" || plan.SourceSnapshot == "" || plan.DecisionRequired {
		t.Fatalf("incomplete source-bound plan: %+v", plan)
	}
	for _, gateID := range plan.GateIDs {
		if strings.HasPrefix(gateID, "frontend-") || gateID == "tracker-package" {
			t.Fatalf("backend-only complete plan selected frontend gate %q: %+v", gateID, plan)
		}
	}
	if !slices.Contains(plan.GateIDs, "go-vet") {
		t.Fatalf("backend-only complete plan omitted Go checks: %+v", plan)
	}
}

func TestUnknownPathRequiresDecision(t *testing.T) {
	root := initTestRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	if err := os.WriteFile(filepath.Join(root, "unknown.payload"), []byte("unknown\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := app.QAPlan(context.Background(), "complete", "origin/main")
	if err != nil {
		t.Fatal(err)
	}
	if !plan.DecisionRequired || len(plan.GateIDs) != 0 || plan.DecisionReason != "unclassified_path" {
		t.Fatalf("unknown path did not fail closed: %+v", plan)
	}
}

func TestExampleConfigurationSelectsBackendAndDocumentationAreas(t *testing.T) {
	areas, known := classifyChangedPath("hitkeep.example.yaml")
	if !known {
		t.Fatal("hitkeep.example.yaml is not classified")
	}
	if len(areas) != 2 || areas[0] != changeBackend || areas[1] != changeDocumentation {
		t.Fatalf("areas = %v, want backend and documentation", areas)
	}
}

func TestFrontendManifestsSelectDependencyAndDashboardAreas(t *testing.T) {
	want := []string{changeDependencies, changeDashboard}
	for _, path := range []string{"frontend/dashboard/package.json", "frontend/dashboard/package-lock.json"} {
		got, known := classifyChangedPath(path)
		if !known {
			t.Fatalf("classifyChangedPath(%q) reported an unknown path", path)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("classifyChangedPath(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestPrepareQARequestPreservesValidatedGateSubset(t *testing.T) {
	root := initTestRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := app.QAPlan(context.Background(), "pr", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.GateIDs) < 2 {
		t.Fatalf("PR plan needs multiple gates for subset coverage: %+v", plan)
	}

	selected := []string{plan.GateIDs[0]}
	request, err := app.prepareQARequest(context.Background(), RunRequest{
		Kind: "qa", Profile: "pr", PlanID: plan.PlanID, GateIDs: selected,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(request.GateIDs, selected) {
		t.Fatalf("gate subset expanded from %v to %v", selected, request.GateIDs)
	}

	_, err = app.prepareQARequest(context.Background(), RunRequest{
		Kind: "qa", Profile: "pr", PlanID: plan.PlanID, GateIDs: []string{"not-in-plan"},
	})
	if err == nil || !strings.Contains(err.Error(), "not selected by plan") {
		t.Fatalf("out-of-plan gate was not rejected: %v", err)
	}
}

func TestFullProfileBypassesEvidenceReuse(t *testing.T) {
	app := &App{workspace: Workspace{StateDir: t.TempDir()}}
	gate := Gate{ID: "go-vet", ContractVersion: "1", ReuseTTL: "24h", Volatility: "deterministic"}
	request := RunRequest{Kind: "qa", Profile: "full", PlanID: "plan"}
	if err := app.storeGateEvidence(gate, request, "run", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, reused := app.reusableGateEvidence(gate, request); reused {
		t.Fatal("full profile unexpectedly reused evidence")
	}
}
