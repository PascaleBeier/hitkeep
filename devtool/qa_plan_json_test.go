package devtool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"hitkeep/jsonapi"
)

func TestQAPlanJSONV2PreservesLegacyBytesPlanIDAndResume(t *testing.T) {
	root := initTestRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}

	plan, err := app.QAPlan(t.Context(), "pr", "")
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	current, err := jsonapi.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(current, legacy) {
		t.Fatalf("JSON v2 changed QA-plan bytes:\nlegacy: %s\ncurrent: %s", legacy, current)
	}
	if got := qaPlanID(plan); got != plan.PlanID {
		t.Fatalf("qaPlanID(plan) = %q, want %q", got, plan.PlanID)
	}

	var decoded QAPlan
	if err := jsonapi.Unmarshal(legacy, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, plan) {
		t.Fatalf("decoded plan = %+v, want %+v", decoded, plan)
	}

	planPath := filepath.Join(app.workspace.StateDir, "qa-plans", plan.PlanID+".json")
	if err := os.WriteFile(planPath, legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	selected := []string{plan.GateIDs[0]}
	request, err := app.prepareQARequest(t.Context(), RunRequest{
		Kind: "qa", Profile: "pr", PlanID: plan.PlanID, GateIDs: selected,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(request.GateIDs, selected) {
		t.Fatalf("resumed gate IDs = %v, want %v", request.GateIDs, selected)
	}
}

func TestPrepareQARequestRecomputesTruncatedChangedPaths(t *testing.T) {
	root := initTestRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	paths := make([]string, maxStructuredPaths+1)
	for i := range paths {
		paths[i] = filepath.Join("internal", "server", fmt.Sprintf("qa-plan-%02d.go", i))
		path := filepath.Join(root, paths[i])
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("package server\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := app.QAPlan(t.Context(), "complete", "origin/main")
	if err != nil {
		t.Fatal(err)
	}
	if !plan.ChangedPathsTruncated || plan.ChangedPathCount != len(paths) || len(plan.ChangedPaths) != maxStructuredPaths {
		t.Fatalf("plan did not truncate changed paths as expected: %+v", plan)
	}
	outside := ""
	for _, path := range paths {
		if !slices.Contains(plan.ChangedPaths, path) {
			outside = path
			break
		}
	}
	if outside == "" {
		t.Fatal("no changed path was omitted from persisted plan")
	}

	request := RunRequest{Kind: "qa", Profile: "complete", PlanID: plan.PlanID}
	if _, err := app.prepareQARequest(t.Context(), request); err != nil {
		t.Fatalf("fresh truncated plan did not resume: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, outside), []byte("package server\n\n// changed after planning\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := app.prepareQARequest(t.Context(), request); err == nil || !strings.Contains(err.Error(), "qa_plan_stale") {
		t.Fatalf("change outside persisted paths did not stale plan: %v", err)
	}
}

func TestQAPlanIDIgnoresSerializationOnlyFields(t *testing.T) {
	plan := QAPlan{
		Profile:          "changed",
		BaseRef:          "origin/main",
		SourceSnapshot:   "legacy-snapshot",
		PlannerVersion:   qaPlannerVersion,
		CatalogVersion:   qaCatalogVersion,
		GateIDs:          []string{"go-format", "go-vet"},
		ChangedPathCount: 0,
	}
	const legacyPlanID = "62b54ec93a86c87dd4a875dd"
	if got := qaPlanID(plan); got != legacyPlanID {
		t.Fatalf("qaPlanID(plan) = %q, want legacy ID %q", got, legacyPlanID)
	}
	plan.ChangedPathCount = 26
	plan.ChangedPathsTruncated = true
	if got := qaPlanID(plan); got != legacyPlanID {
		t.Fatalf("qaPlanID(plan with JSON-only fields) = %q, want legacy ID %q", got, legacyPlanID)
	}
}
