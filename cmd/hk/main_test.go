package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"hitkeep/internal/devtool"
	json "hitkeep/internal/jsonapi"
)

func TestDetachedCLIActionOutlivesLauncher(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the hk command")
	}
	projectRoot := repositoryRoot(t)
	binary := filepath.Join(t.TempDir(), "hk")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", binary, "./cmd/hk")
	build.Dir = projectRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build hk: %v: %s", err, output)
	}

	workspace := filepath.Join(t.TempDir(), "workspace")
	if output, err := exec.CommandContext(t.Context(), "git", "init", "--quiet", workspace).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	dashboard := filepath.Join(workspace, "frontend", "dashboard")
	if err := os.MkdirAll(dashboard, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"go.mod":                               "module example.test/hk\n\ngo 1.27.0\n",
		"CONTRIBUTING.md":                      "# Contributing\n",
		"frontend/dashboard/package.json":      "{\"packageManager\":\"npm@12.0.2\"}\n",
		"frontend/dashboard/package-lock.json": "{}\n",
		"frontend/dashboard/.npmrc":            "legacy-peer-deps=true\n",
		"frontend/dashboard/.node-version":     "24.19.0\n",
	} {
		path := filepath.Join(workspace, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fakeBin := filepath.Join(t.TempDir(), "bin")
	if err := os.Mkdir(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, script := range map[string]string{
		"docker": "#!/bin/sh\necho 1.0.0\nexit 0\n",
	} {
		if err := os.WriteFile(filepath.Join(fakeBin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	stateDir := filepath.Join(t.TempDir(), "state")
	installFakeManagedToolchain(t, stateDir)
	t.Setenv("HK_STATE_DIR", stateDir)
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	command := exec.CommandContext(t.Context(), binary, "--workspace", workspace, "--output", "json", "setup", "--detach")
	command.Env = os.Environ()
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("start detached setup: %v: %s", err, output)
	}
	var envelope struct {
		Data devtool.RunStart `json:"data"`
	}
	if err := json.Unmarshal(output, &envelope); err != nil {
		t.Fatalf("decode detached setup: %v: %s", err, output)
	}
	if envelope.Data.RunID == "" {
		t.Fatalf("detached setup returned no run ID: %s", output)
	}

	app, err := devtool.NewApp(workspace)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		run, statusErr := app.GetRun(envelope.Data.RunID)
		if statusErr != nil {
			t.Fatal(statusErr)
		}
		if run.Status == "passed" {
			break
		}
		if run.Status == "failed" || run.Status == "cancelled" {
			t.Fatalf("detached setup ended as %s: %+v", run.Status, run)
		}
		if time.Now().After(deadline) {
			t.Fatalf("detached setup did not finish: %+v", run)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestMCPStdioWritesNoNonProtocolOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the hk command")
	}
	root := repositoryRoot(t)
	command := exec.Command("go", "run", "./cmd/hk", "mcp", "serve", "--workspace", root)
	command.Dir = root
	command.Stdin = bytes.NewReader(nil)
	command.Env = append(os.Environ(), "HK_STATE_DIR="+filepath.Join(t.TempDir(), "state"))
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("serve with closed stdin: %v: %s", err, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("non-protocol stdout: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}

func TestMCPStdioWithoutWorkspaceUsesConfiguredFallback(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the hk command")
	}
	projectRoot := repositoryRoot(t)
	binary := filepath.Join(t.TempDir(), "hk")
	build := exec.Command("go", "build", "-o", binary, "./cmd/hk")
	build.Dir = projectRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build hk: %v: %s", err, output)
	}
	workspace := filepath.Join(t.TempDir(), "workspace")
	if output, err := exec.Command("git", "init", "--quiet", workspace).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	if err := os.WriteFile(filepath.Join(workspace, "CONTRIBUTING.md"), []byte("# Contributing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module hitkeep\n\ngo 1.27.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	delegated := filepath.Join(t.TempDir(), "delegated")
	launcher := "#!/bin/sh\n: > " + delegated + "\nexec " + binary + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(workspace, "hk"), []byte(launcher), 0o700); err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(t.TempDir(), "state")
	t.Setenv("HK_STATE_DIR", stateDir)
	wantApp, err := devtool.NewApp(workspace)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.Command(binary, "mcp", "serve", "--fallback-workspace", workspace)
	command.Dir = projectRoot
	command.Env = append(os.Environ(), "HK_STATE_DIR="+stateDir)
	client := mcp.NewClient(&mcp.Implementation{Name: "hk-central-integration-test", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "hk_workspace_status", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	envelope := result.StructuredContent.(map[string]any)
	if result.IsError || envelope["workspace_id"] != wantApp.WorkspaceID() {
		t.Fatalf("central stdio server did not route by client root: %#v", envelope)
	}
	if _, err := os.Stat(delegated); err != nil {
		t.Fatalf("central server did not delegate to the workspace MCP: %v", err)
	}
}

func TestMCPStdioActionRunLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the hk command")
	}
	projectRoot := repositoryRoot(t)
	binary := filepath.Join(t.TempDir(), "hk")
	build := exec.Command("go", "build", "-o", binary, "./cmd/hk")
	build.Dir = projectRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build hk: %v: %s", err, output)
	}

	workspace := filepath.Join(t.TempDir(), "workspace")
	if output, err := exec.Command("git", "init", "--quiet", workspace).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	dashboard := filepath.Join(workspace, "frontend", "dashboard")
	if err := os.MkdirAll(dashboard, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"go.mod":                               "module example.test/hk\n\ngo 1.27.0\n",
		"CONTRIBUTING.md":                      "# Contributing\n",
		"frontend/dashboard/package.json":      "{\"packageManager\":\"npm@12.0.2\"}\n",
		"frontend/dashboard/package-lock.json": "{}\n",
		"frontend/dashboard/.npmrc":            "legacy-peer-deps=true\n",
		"frontend/dashboard/.node-version":     "24.19.0\n",
	} {
		path := filepath.Join(workspace, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fakeBin := filepath.Join(t.TempDir(), "bin")
	if err := os.Mkdir(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	slowFile := filepath.Join(t.TempDir(), "slow")
	goScript := "#!/bin/sh\nif [ \"${1:-}\" = version ]; then echo 'go version go1.27.0 test'; exit 0; fi\nif [ -f \"" + slowFile + "\" ]; then sleep 30; fi\nexit 0\n"
	dockerScript := "#!/bin/sh\ncase \"$*\" in *version*) echo 1.0.0; exit 0;; esac\nif [ -f \"" + slowFile + "\" ]; then sleep 30; fi\nexit 0\n"
	for name, script := range map[string]string{
		"docker": dockerScript, "go": goScript, "npm": "#!/bin/sh\nif [ \"${1:-}\" = --version ]; then echo '12.0.2'; exit 0; fi\nmkdir -p node_modules\nexit 0\n", "node": "#!/bin/sh\necho v24.19.0\n", "npx": "#!/bin/sh\nmkdir -p node_modules\nexit 0\n",
	} {
		if err := os.WriteFile(filepath.Join(fakeBin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	stateDir := filepath.Join(t.TempDir(), "state")
	installFakeManagedToolchain(t, stateDir)
	command := exec.Command(binary, "mcp", "serve", "--workspace", workspace)
	command.Env = append(os.Environ(), "PATH="+fakeBin+":"+os.Getenv("PATH"), "HK_STATE_DIR="+stateDir)
	client := mcp.NewClient(&mcp.Implementation{Name: "hk-integration-test", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	started, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "hk_setup_start", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	envelope := started.StructuredContent.(map[string]any)
	data := envelope["data"].(map[string]any)
	runID := data["run_id"].(string)
	for {
		statusResult, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: "hk_run_status", Arguments: map[string]any{"run_id": runID}})
		if callErr != nil {
			t.Fatal(callErr)
		}
		statusEnvelope := statusResult.StructuredContent.(map[string]any)
		run := statusEnvelope["data"].(map[string]any)
		status := run["status"].(string)
		if status == "passed" {
			break
		}
		if status == "failed" || status == "cancelled" {
			t.Fatalf("setup run ended as %s: %#v", status, run)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("%v: last run state %#v", ctx.Err(), run)
		case <-time.After(50 * time.Millisecond):
		}
	}

	if err := os.WriteFile(slowFile, []byte("slow\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	started, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "hk_setup_start", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	envelope = started.StructuredContent.(map[string]any)
	data = envelope["data"].(map[string]any)
	runID = data["run_id"].(string)
	if _, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "hk_run_cancel", Arguments: map[string]any{"run_id": runID}}); err != nil {
		t.Fatal(err)
	}
	for {
		statusResult, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: "hk_run_status", Arguments: map[string]any{"run_id": runID}})
		if callErr != nil {
			t.Fatal(callErr)
		}
		statusEnvelope := statusResult.StructuredContent.(map[string]any)
		run := statusEnvelope["data"].(map[string]any)
		status := run["status"].(string)
		if status == "cancelled" {
			break
		}
		if status == "failed" || status == "passed" {
			t.Fatalf("cancelled setup ended as %s: %#v", status, run)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("%v: last cancelled run state %#v", ctx.Err(), run)
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func installFakeManagedToolchain(t *testing.T, stateDir string) {
	t.Helper()
	platform := runtime.GOOS + "-" + runtime.GOARCH
	goBin := filepath.Join(stateDir, "shared", "toolchains", "go-1.27.0-"+platform, "bin")
	nodeRoot := filepath.Join(stateDir, "shared", "toolchains", "node-24.19.0-"+platform)
	nodeBin := filepath.Join(nodeRoot, "bin")
	npmCLI := filepath.Join(nodeRoot, "lib", "node_modules", "npm", "bin", "npm-cli.js")
	for _, directory := range []string{goBin, nodeBin, filepath.Dir(npmCLI)} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	managedCommands := map[string]string{
		filepath.Join(goBin, "go"):     "#!/bin/sh\nif [ \"${1:-}\" = version ]; then echo 'go version go1.27.0 test'; fi\nexit 0\n",
		filepath.Join(nodeBin, "node"): "#!/bin/sh\ncase \"${1:-}\" in *npm-cli.js) echo 12.0.2 ;; *) echo v24.19.0 ;; esac\nexit 0\n",
		filepath.Join(nodeBin, "npm"):  "#!/bin/sh\nif [ \"${1:-}\" = --version ]; then echo '12.0.2'; exit 0; fi\nmkdir -p node_modules\nexit 0\n",
		filepath.Join(nodeBin, "npx"):  "#!/bin/sh\nmkdir -p node_modules\nexit 0\n",
		npmCLI:                         "#!/bin/sh\nexit 0\n",
	}
	for path, script := range managedCommands {
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}
