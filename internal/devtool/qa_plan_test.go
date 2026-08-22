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
