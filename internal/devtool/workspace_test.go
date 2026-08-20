package devtool

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	json "hitkeep/internal/jsonapi"
)

func TestRunCommandPreservesContextCancellation(t *testing.T) {
	root := initTestRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = app.runCommand(ctx, io.Discard, commandSpec{Args: []string{"go", "version"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("run command error = %v, want context cancellation", err)
	}
}

func TestQACancellationPreservesCancelledGateState(t *testing.T) {
	root := initTestRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	run := Run{ID: "20260714T000000-cancel", WorkspaceID: app.workspace.ID, Request: RunRequest{Kind: "qa", Profile: "pr", GateIDs: []string{"go-vet"}}, Status: "running", LogPath: filepath.Join(app.workspace.StateDir, "runs", "cancel.log"), StartedAt: time.Now().UTC()}
	if err := app.writeRun(run); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	results, err := app.executeQA(ctx, run.ID, run.Request, io.Discard)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("executeQA error = %v, want context cancellation", err)
	}
	if len(results) != 1 || results[0].Status != "cancelled" || results[0].Error != "" {
		t.Fatalf("cancelled gate result = %+v", results)
	}
}

func TestQAGateTimeoutStartsAfterSchedulerWait(t *testing.T) {
	root := initTestRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	t.Setenv("HK_QA_SLOTS", "1")
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	releaseBlockingSlot, err := app.acquireQASlots(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}

	type acquiredGate struct {
		ctx        context.Context
		finish     func()
		acquiredAt time.Time
		err        error
	}
	result := make(chan acquiredGate, 1)
	started := time.Now()
	go func() {
		gateContext, finish, acquireErr := app.acquireQAGateContext(context.Background(), Gate{Weight: 1, Timeout: "200ms"})
		result <- acquiredGate{ctx: gateContext, finish: finish, acquiredAt: time.Now(), err: acquireErr}
	}()
	time.Sleep(250 * time.Millisecond)
	releaseBlockingSlot()

	acquired := <-result
	if acquired.err != nil {
		t.Fatal(acquired.err)
	}
	defer acquired.finish()
	if elapsed := time.Since(started); elapsed < 200*time.Millisecond {
		t.Fatalf("scheduler wait = %s, want it to exceed the gate timeout", elapsed)
	}
	deadline, ok := acquired.ctx.Deadline()
	if !ok {
		t.Fatal("gate context has no execution deadline")
	}
	if remainingAtAcquisition := deadline.Sub(acquired.acquiredAt); remainingAtAcquisition < 150*time.Millisecond {
		t.Fatalf("gate timeout remaining after scheduler wait = %s, want nearly the full execution timeout", remainingAtAcquisition)
	}
}

func TestGetRunReconcilesExitedWorker(t *testing.T) {
	root := initTestRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	run := Run{
		ID:          "20260714T000000-exited",
		WorkspaceID: app.workspace.ID,
		Request:     RunRequest{Kind: "setup"},
		Status:      "running",
		PID:         1 << 30,
		LogPath:     filepath.Join(app.workspace.StateDir, "runs", "exited.log"),
		StartedAt:   time.Now().Add(-time.Minute),
	}
	if err := app.writeRun(run); err != nil {
		t.Fatal(err)
	}
	got, err := app.GetRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "failed" || got.PID != 0 || got.FinishedAt == nil || got.ExitCode == nil {
		t.Fatalf("exited worker was not reconciled: %+v", got)
	}
}

func TestValidateWorkspacePathRejectsEscapes(t *testing.T) {
	root := initTestRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	workspace, err := ResolveWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}

	inside := filepath.Join(root, "inside")
	if err := os.Mkdir(inside, 0o700); err != nil {
		t.Fatal(err)
	}
	if got, err := ValidateWorkspacePath(workspace, inside); err != nil || filepath.Base(got) != "inside" {
		t.Fatalf("inside path: got %q, err %v", got, err)
	}

	outside := t.TempDir()
	if _, err := ValidateWorkspacePath(workspace, outside); err == nil {
		t.Fatal("outside path was accepted")
	}
	symlink := filepath.Join(root, "escape")
	if err := os.Symlink(outside, symlink); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateWorkspacePath(workspace, symlink); err == nil {
		t.Fatal("symlink escape was accepted")
	}
}

func TestCatalogGuardsCloudPublication(t *testing.T) {
	cloud, err := VariantByID("cloud")
	if err != nil {
		t.Fatal(err)
	}
	if cloud.Publishable || !cloud.ProductionImageOnly {
		t.Fatalf("cloud publication boundary changed: %+v", cloud)
	}
	if _, err := GateByID("arbitrary-shell"); err == nil {
		t.Fatal("unknown QA gate was accepted")
	}
	if err := ValidateRunRequest(RunRequest{Kind: "qa", Profile: "pr", GateIDs: []string{"arbitrary-shell"}}); err == nil {
		t.Fatal("unknown QA gate was accepted in a run")
	}
}

func TestGitWorktreesReceiveIsolatedWorkspaceState(t *testing.T) {
	root := initTestRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	for _, args := range [][]string{{"-C", root, "config", "user.email", "test@example.com"}, {"-C", root, "config", "user.name", "Test"}} {
		if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git config: %v: %s", err, output)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"-C", root, "add", "README.md"}, {"-C", root, "commit", "--quiet", "-m", "test"}} {
		if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git setup: %v: %s", err, output)
		}
	}
	second := filepath.Join(t.TempDir(), "second")
	if output, err := exec.Command("git", "-C", root, "worktree", "add", "--quiet", "--detach", second, "HEAD").CombinedOutput(); err != nil {
		t.Fatalf("add worktree: %v: %s", err, output)
	}
	workspaces, err := ListWorkspaces(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(workspaces) != 2 {
		t.Fatalf("workspaces: got %d, want 2", len(workspaces))
	}
	if err := os.WriteFile(filepath.Join(root, "CONTRIBUTING.md"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspaces, err = ListWorkspaces(root)
	if err != nil {
		t.Fatal(err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	var mainWorkspace Workspace
	for _, workspace := range workspaces {
		if workspace.Root == resolvedRoot {
			mainWorkspace = workspace
			break
		}
	}
	if mainWorkspace.DirtyCount != 1 || len(mainWorkspace.ChangedPaths) != 1 || mainWorkspace.ChangedPaths[0] != "CONTRIBUTING.md" {
		t.Fatalf("workspace list lost working-tree state: %+v", mainWorkspace)
	}
	if workspaces[0].ID == workspaces[1].ID || workspaces[0].ComposeProject == workspaces[1].ComposeProject || workspaces[0].StateDir == workspaces[1].StateDir {
		t.Fatalf("worktree state collided: %+v %+v", workspaces[0], workspaces[1])
	}
	if workspaces[0].Ports != (Ports{}) || workspaces[1].Ports != (Ports{}) {
		t.Fatalf("read-only discovery allocated ports: %+v", workspaces)
	}
	stateEntries, err := os.ReadDir(filepath.Join(os.Getenv("HK_STATE_DIR"), "workspaces"))
	if err == nil && len(stateEntries) != 0 {
		t.Fatalf("read-only discovery created workspace state: %+v", stateEntries)
	}
}

func TestWorkingTreeChangedPathsPreservesPorcelainPrefixes(t *testing.T) {
	root := initTestRepository(t)
	if err := os.WriteFile(filepath.Join(root, "CONTRIBUTING.md"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	paths, err := workingTreeChangedPaths(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != "CONTRIBUTING.md" {
		t.Fatalf("changed paths = %v, want CONTRIBUTING.md", paths)
	}
}

func TestRedactError(t *testing.T) {
	got := redactError("TOKEN=abc PASSWORD=hunter2 ordinary=value")
	if got != "TOKEN=[redacted] PASSWORD=[redacted] ordinary=value" {
		t.Fatalf("redaction mismatch: %q", got)
	}
}

func TestTailLogIsBoundedAndRedacted(t *testing.T) {
	root := initTestRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	runID := "20260714T000000-test"
	logPath := filepath.Join(app.workspace.StateDir, "runs", runID+".log")
	run := Run{ID: runID, WorkspaceID: app.workspace.ID, Request: RunRequest{Kind: "setup"}, Status: "passed", LogPath: logPath}
	if err := app.writeRun(run); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	encoder := jsontext.NewEncoder(file)
	for index := range 250 {
		value := map[string]any{"line": index}
		if index == 249 {
			value["message"] = "TOKEN=secret"
		}
		if err := json.MarshalEncode(encoder, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	tail, err := app.TailLog(runID, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(tail.Lines) != maxLogLines || !tail.Truncated {
		t.Fatalf("tail bounds: %+v", tail)
	}
	if got := tail.Lines[len(tail.Lines)-1]; got == "" || redactError(got) != got || got == `{"line":249,"message":"TOKEN=secret"}` {
		t.Fatalf("secret was not redacted: %q", got)
	}
}

func initTestRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	command := exec.Command("git", "init", "--quiet", root)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	return root
}
