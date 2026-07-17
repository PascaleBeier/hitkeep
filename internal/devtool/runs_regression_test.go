package devtool

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
)

func TestStartRunDeduplicatesAtomicallyAcrossAppInstances(t *testing.T) {
	root := initTestRepository(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	t.Setenv("HK_STATE_DIR", stateRoot)
	executable := filepath.Join(t.TempDir(), "hk-worker")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nwhile :; do sleep 1; done\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	const callers = 8
	apps := make([]*App, callers)
	for index := range apps {
		app, err := NewApp(root)
		if err != nil {
			t.Fatal(err)
		}
		app.executable = executable
		apps[index] = app
	}
	starts := make([]RunStart, callers)
	errorsByCaller := make([]error, callers)
	ready := make(chan struct{})
	var wait sync.WaitGroup
	for index, app := range apps {
		wait.Go(func() {
			<-ready
			starts[index], errorsByCaller[index] = app.StartRun(context.Background(), RunRequest{Kind: "setup"})
		})
	}
	close(ready)
	wait.Wait()
	for index, err := range errorsByCaller {
		if err != nil {
			t.Fatalf("caller %d: %v", index, err)
		}
		if starts[index].RunID != starts[0].RunID {
			t.Fatalf("caller %d created run %s, want %s", index, starts[index].RunID, starts[0].RunID)
		}
	}
	run, err := apps[0].GetRun(starts[0].RunID)
	if err != nil {
		t.Fatal(err)
	}
	if run.PID <= 0 {
		t.Fatalf("deduplicated run has no worker: %+v", run)
	}
	if err := syscall.Kill(-run.PID, syscall.SIGKILL); err != nil {
		t.Fatalf("stop test worker: %v", err)
	}
}

func TestContainerDevResetRemovesOnlyWorkspaceVolume(t *testing.T) {
	root := initTestRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	home := t.TempDir()
	t.Setenv("HOME", home)
	fakeBin := t.TempDir()
	script := `#!/bin/sh
if [ "${1:-}" = volume ] && [ "${2:-}" = ls ]; then
  echo hitkeep-workspace-data
  exit 0
fi
printf '%s\n' "$@" > "$HOME/docker-args"
`
	if err := os.WriteFile(filepath.Join(fakeBin, "docker"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	runFile := filepath.Join(app.workspace.StateDir, "runs", "keep.log")
	if err := os.MkdirAll(filepath.Dir(runFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runFile, []byte("log"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := app.resetDevData(context.Background(), io.Discard); err != nil {
		t.Fatal(err)
	}
	arguments, err := os.ReadFile(filepath.Join(home, "docker-args"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(arguments), "volume\nrm\nhitkeep-workspace-data") {
		t.Fatalf("workspace volume was not removed: %s", arguments)
	}
	if _, err := os.Stat(runFile); err != nil {
		t.Fatalf("reset removed non-data workspace state: %v", err)
	}
}

func TestVerifyVariantBuildUsesTemporaryWorkspaceStateOutput(t *testing.T) {
	root := initTestRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	record := filepath.Join(home, "go-args")
	fakeBin := t.TempDir()
	script := `#!/bin/sh
printf '%s\n' "$@" > "$HOME/go-args"
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then
    shift
    mkdir -p "$(dirname "$1")"
    : > "$1"
  fi
  shift
done
`
	if err := os.WriteFile(filepath.Join(fakeBin, "go"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := app.VerifyVariantBuild(context.Background(), "cloud", io.Discard); err != nil {
		t.Fatal(err)
	}
	arguments, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(arguments), app.workspace.StateDir) || !strings.Contains(string(arguments), "hitkeep-cloud") {
		t.Fatalf("build output was not confined to workspace state: %s", arguments)
	}
	if _, err := os.Stat(filepath.Join(root, "main")); !os.IsNotExist(err) {
		t.Fatalf("variant verification created a repository-root binary: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(app.workspace.StateDir, "build-check-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary build outputs were not removed: %v", matches)
	}
}
