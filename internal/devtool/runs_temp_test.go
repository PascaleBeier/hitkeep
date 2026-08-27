package devtool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestStartRunIsolatesWorkerTemporaryAndGoCaches(t *testing.T) {
	root := initTestRepository(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	t.Setenv("HK_STATE_DIR", stateRoot)
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "global-tmp"))
	t.Setenv("GOTMPDIR", filepath.Join(t.TempDir(), "global-go-tmp"))
	t.Setenv("GOCACHE", filepath.Join(t.TempDir(), "global-go-cache"))
	original := map[string]string{"TMPDIR": os.Getenv("TMPDIR"), "GOTMPDIR": os.Getenv("GOTMPDIR"), "GOCACHE": os.Getenv("GOCACHE")}

	worker := filepath.Join(t.TempDir(), "hk-worker")
	if err := os.WriteFile(worker, []byte("#!/bin/sh\nenv > \"$HK_RUN_TEMP_ROOT/worker-env\"\nwhile :; do sleep 1; done\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	app.executable = worker
	start, err := app.StartRun(context.Background(), RunRequest{Kind: "setup"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		run, getErr := app.GetRun(start.RunID)
		if getErr == nil && run.PID > 0 {
			_ = syscall.Kill(-run.PID, syscall.SIGKILL)
		}
		_ = app.removeRunTempRoot(start.RunID)
	})

	tempRoot := app.runTempRoot(start.RunID)
	workerEnvironment := waitForWorkerEnvironment(t, filepath.Join(tempRoot, "worker-env"))
	for key, want := range map[string]string{
		"HK_RUN_TEMP_ROOT": tempRoot,
		"TMPDIR":           filepath.Join(tempRoot, "tmp"),
		"GOTMPDIR":         filepath.Join(tempRoot, "go-tmp"),
		"GOCACHE":          filepath.Join(tempRoot, "go-cache"),
	} {
		if got := workerEnvironmentValue(workerEnvironment, key); got != want {
			t.Fatalf("worker %s = %q, want %q", key, got, want)
		}
	}
	for key, want := range original {
		if got := os.Getenv(key); got != want {
			t.Fatalf("global %s changed to %q, want %q", key, got, want)
		}
	}
}

func TestCommandEnvironmentPreservesValidatedRunIsolation(t *testing.T) {
	root := initTestRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	const runID = "20260827T010000-abcd1234"
	if _, err := app.prepareRunTempEnvironment(runID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.removeRunTempRoot(runID) })
	t.Setenv("HK_RUN_TEMP_ROOT", app.runTempRoot(runID))

	environment := environmentValues(app.commandEnvironment(nil))
	if got, want := environment["TMPDIR"], filepath.Join(app.runTempRoot(runID), "tmp"); got != want {
		t.Fatalf("TMPDIR = %q, want %q", got, want)
	}
	if got, want := environment["GOTMPDIR"], filepath.Join(app.runTempRoot(runID), "go-tmp"); got != want {
		t.Fatalf("GOTMPDIR = %q, want %q", got, want)
	}
	if got, want := environment["GOCACHE"], filepath.Join(app.runTempRoot(runID), "go-cache"); got != want {
		t.Fatalf("GOCACHE = %q, want %q", got, want)
	}
}

func TestExecuteRunTerminalCleanupForPassFailAndCancellation(t *testing.T) {
	root := initTestRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"passed", "failed", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			runID := "20260827T010000-" + status[:4] + "1234"
			if _, err := app.prepareRunTempEnvironment(runID); err != nil {
				t.Fatal(err)
			}
			app.cleanupRunTempRoot(runID)
			if _, err := os.Stat(app.runTempRoot(runID)); !os.IsNotExist(err) {
				t.Fatalf("terminal %s root still exists: %v", status, err)
			}
		})
	}
}

func TestPruneTerminalRunTempRootsLeavesActiveAndUnknownRoots(t *testing.T) {
	root := initTestRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	terminalID := "20260827T010000-deadbeef"
	activeID := "20260827T010001-cafe1234"
	unknownID := "20260827T010002-feed1234"
	for _, runID := range []string{terminalID, activeID, unknownID} {
		if _, err := app.prepareRunTempEnvironment(runID); err != nil {
			t.Fatal(err)
		}
	}
	started := time.Now().UTC().Add(-3 * time.Second)
	if err := app.writeRun(Run{ID: terminalID, WorkspaceID: app.workspace.ID, Request: RunRequest{Kind: "setup"}, Status: "running", PID: 1 << 30, StartedAt: started}); err != nil {
		t.Fatal(err)
	}
	if err := app.writeRun(Run{ID: activeID, WorkspaceID: app.workspace.ID, Request: RunRequest{Kind: "setup"}, Status: "running", PID: os.Getpid()}); err != nil {
		t.Fatal(err)
	}
	if err := app.pruneTerminalRunTempRoots(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(app.runTempRoot(terminalID)); !os.IsNotExist(err) {
		t.Fatalf("crashed worker root remains: %v", err)
	}
	for _, runID := range []string{activeID, unknownID} {
		if _, err := os.Stat(app.runTempRoot(runID)); err != nil {
			t.Fatalf("%s root was removed: %v", runID, err)
		}
	}
}

func waitForWorkerEnvironment(t *testing.T, path string) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		content, err := os.ReadFile(path)
		if err == nil && workerEnvironmentValue(string(content), "GOCACHE") != "" {
			return string(content)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
	return ""
}

func workerEnvironmentValue(environment, key string) string {
	return environmentValues(strings.Split(strings.TrimSpace(environment), "\n"))[key]
}

func environmentValues(environment []string) map[string]string {
	values := make(map[string]string, len(environment))
	for _, entry := range environment {
		key, value, found := strings.Cut(entry, "=")
		if found {
			values[key] = value
		}
	}
	return values
}
