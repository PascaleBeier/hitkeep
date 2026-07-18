package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"hitkeep/internal/devtool"
)

func TestJSONOutputUsesVersionedEnvelope(t *testing.T) {
	root := testRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	var stdout, stderr bytes.Buffer
	if err := Execute(context.Background(), "test", []string{"--workspace", root, "--output", "json", "catalog"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	var envelope devtool.Envelope
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	if envelope.SchemaVersion != devtool.SchemaVersion || envelope.Command != "catalog" || envelope.Status != "ok" {
		t.Fatalf("unexpected envelope: %+v", envelope)
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestJSONErrorsRemainMachineReadable(t *testing.T) {
	root := testRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	var stdout, stderr bytes.Buffer
	err := Execute(context.Background(), "test", []string{"--workspace", root, "--output", "json", "qa", "plan", "unknown"}, &stdout, &stderr)
	if err == nil || !IsReported(err) {
		t.Fatalf("expected reported command error, got %v", err)
	}
	var envelope devtool.Envelope
	if decodeErr := json.Unmarshal(stdout.Bytes(), &envelope); decodeErr != nil {
		t.Fatalf("decode output: %v\n%s", decodeErr, stdout.String())
	}
	if envelope.Status != "error" || envelope.Error == "" {
		t.Fatalf("unexpected error envelope: %+v", envelope)
	}
	if stderr.Len() != 0 {
		t.Fatalf("structured output leaked to stderr: %s", stderr.String())
	}
}

func TestHumanRendererUsesColorForQA(t *testing.T) {
	var stdout bytes.Buffer
	envelope := devtool.SuccessEnvelope("qa plan", "workspace", devtool.QAPlan{Profile: "changed", GateIDs: []string{"go-lint"}})
	if err := renderHuman(&stdout, envelope, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "\x1b[") {
		t.Fatalf("QA output was not colored: %q", stdout.String())
	}
}

func TestHumanRendererUsesPlainFallbackForAdministrativeOutput(t *testing.T) {
	envelope := devtool.SuccessEnvelope("catalog", "workspace", devtool.Catalog{
		Variants: []devtool.Variant{{ID: "self-hosted", Description: "Public self-hosted build", Publishable: true}},
	})
	var human, plain bytes.Buffer
	if err := renderHuman(&human, envelope, true); err != nil {
		t.Fatal(err)
	}
	if err := renderPlain(&plain, envelope); err != nil {
		t.Fatal(err)
	}
	if human.String() != plain.String() {
		t.Fatalf("administrative human output did not use the plain renderer: %q", human.String())
	}
}

func TestInvalidOutputIsRejectedBeforeCommandExecution(t *testing.T) {
	root := testRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	var stdout, stderr bytes.Buffer
	err := Execute(context.Background(), "test", []string{"--workspace", root, "--output", "yaml", "catalog"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), `unknown output format "yaml"`) {
		t.Fatalf("expected output validation error, got %v", err)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("invalid output produced command output: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestFollowRunLogStreamsEveryStoredLine(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "run.log")
	var log strings.Builder
	for index := range 500 {
		fmt.Fprintf(&log, "line-%03d\n", index)
	}
	if err := os.WriteFile(logPath, []byte(log.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	var lines []string
	err := followRunLog(context.Background(), "run-id", func(string) (devtool.Run, error) {
		return devtool.Run{ID: "run-id", Status: "passed", LogPath: logPath}, nil
	}, 0, func(line string) {
		lines = append(lines, line)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 500 || lines[0] != "line-000" || lines[len(lines)-1] != "line-499" {
		t.Fatalf("unexpected streamed lines: count=%d first=%q last=%q", len(lines), lines[0], lines[len(lines)-1])
	}
}

func TestFollowRunLogSkipsPreviouslyRenderedLines(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "run.log")
	if err := os.WriteFile(logPath, []byte("one\ntwo\nthree\nfour\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var lines []string
	err := followRunLog(context.Background(), "run-id", func(string) (devtool.Run, error) {
		return devtool.Run{ID: "run-id", Status: "passed", LogPath: logPath}, nil
	}, 2, func(line string) {
		lines = append(lines, line)
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(lines, ",") != "three,four" {
		t.Fatalf("previously rendered lines were repeated: %v", lines)
	}
}

func TestHumanDevOutputShowsStateAndURLs(t *testing.T) {
	status := devtool.DevStatus{
		State: devtool.DevStateReady, Variant: "self-hosted",
		Owner: devtool.DevOwnerDetached,
		URLs: devtool.URLs{
			Web: "http://127.0.0.1:4200", API: "http://127.0.0.1:8080", Mailpit: "http://127.0.0.1:8025",
		},
	}
	var output bytes.Buffer
	if err := renderHuman(&output, devtool.SuccessEnvelope("dev status", "workspace", status), false); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"HitKeep development", status.URLs.Web, status.URLs.API, status.URLs.Mailpit} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("development output missing %q: %q", expected, output.String())
		}
	}
}

func TestDevHelpPresentsTheSimpleHumanWorkflow(t *testing.T) {
	root := testRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	var stdout, stderr bytes.Buffer
	if err := Execute(context.Background(), "test", []string{"--workspace", root, "dev", "--help"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"logs", "reset", "restart", "stop"} {
		if !strings.Contains(stdout.String(), command) {
			t.Fatalf("dev help missing %q: %s", command, stdout.String())
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestCommandCatalogCoversHumanAndAgentConveniences(t *testing.T) {
	options := &options{output: "json", stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	catalog := buildCommandCatalog(newRoot(options))
	paths := map[string]bool{}
	for _, command := range catalog.Commands {
		paths[command.Path] = true
		if !command.StructuredOutput {
			t.Fatalf("command lacks structured output: %+v", command)
		}
	}
	for _, expected := range []string{
		"hk catalog commands", "hk catalog configuration", "hk dev restart", "hk dev logs", "hk fmt", "hk qa plan", "hk qa start", "hk run logs", "hk screenshot", "hk workspace handoff",
	} {
		if !paths[expected] {
			t.Errorf("command catalog is missing %s", expected)
		}
	}
	if !reflect.DeepEqual(catalog.OutputFormats, []string{"human", "plain", "json", "ndjson"}) {
		t.Fatalf("unexpected output formats: %v", catalog.OutputFormats)
	}
}

func TestDevJSONRequiresDetachedOwnership(t *testing.T) {
	root := testRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	for _, command := range [][]string{{"dev"}, {"dev", "reset"}} {
		var stdout, stderr bytes.Buffer
		args := append([]string{"--workspace", root, "--output", "json"}, command...)
		err := Execute(context.Background(), "test", args, &stdout, &stderr)
		if err == nil || !IsReported(err) {
			t.Fatalf("foreground JSON %v was accepted: %v", command, err)
		}
		var envelope devtool.Envelope
		if decodeErr := json.Unmarshal(stdout.Bytes(), &envelope); decodeErr != nil {
			t.Fatalf("decode output: %v\n%s", decodeErr, stdout.String())
		}
		if envelope.Status != "error" || !strings.Contains(envelope.Error, "--detach") {
			t.Fatalf("unexpected JSON ownership error: %+v", envelope)
		}
		if stderr.Len() != 0 {
			t.Fatalf("structured ownership error leaked to stderr: %s", stderr.String())
		}
	}
}

func TestDevLogsNDJSONStreamsEventsAndFinalBatch(t *testing.T) {
	root := testRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := devtool.NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := app.Workspace(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	generationID := "generation-cli-events"
	devDir := filepath.Join(workspace.StateDir, "dev")
	if err := os.MkdirAll(filepath.Join(devDir, "events"), 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	session, _ := json.Marshal(map[string]any{
		"state": "stopped", "generation_id": generationID, "variant": "self-hosted", "owner": "detached",
		"started_at": now, "stopped_at": now, "updated_at": now, "urls": workspace.URLs, "next_event_cursor": 1,
	})
	if err := os.WriteFile(filepath.Join(devDir, "session.json"), session, 0o600); err != nil {
		t.Fatal(err)
	}
	event, _ := json.Marshal(devtool.DevEvent{Cursor: 0, Timestamp: now, Type: "log", Component: "frontend", Level: "info", Message: "ready"})
	if err := os.WriteFile(filepath.Join(devDir, "events", generationID+".ndjson"), append(event, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := Execute(context.Background(), "test", []string{"--workspace", root, "--output", "ndjson", "dev", "logs", "--follow"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("NDJSON stream produced %d lines, want event and final batch:\n%s", len(lines), stdout.String())
	}
	var first, second devtool.Envelope
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatal(err)
	}
	if first.Command != "dev event" || second.Command != "dev logs" {
		t.Fatalf("unexpected NDJSON commands: %s, %s", first.Command, second.Command)
	}
	if stderr.Len() != 0 {
		t.Fatalf("structured logs leaked to stderr: %s", stderr.String())
	}
}

func TestInterruptExitCodeIs130(t *testing.T) {
	err := reportedError{cause: exitError{cause: context.Canceled, code: 130}}
	if ExitCode(err) != 130 || !IsReported(err) {
		t.Fatalf("interrupt error lost its conventional exit code: %v", err)
	}
}

func TestCentralMCPManifestUsesTheSelectedLocalLauncher(t *testing.T) {
	root := testRepository(t)
	if err := os.WriteFile(filepath.Join(root, "hk"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	var stdout, stderr bytes.Buffer
	if err := Execute(context.Background(), "test", []string{"--workspace", root, "--output", "json", "mcp", "manifest"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	var envelope devtool.Envelope
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode output: %v\n%s", err, stdout.String())
	}
	if envelope.Status != "ok" || envelope.WorkspaceID == "" {
		t.Fatalf("manifest lost its local launcher anchor: %+v", envelope)
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func testRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if output, err := exec.Command("git", "init", "--quiet", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	if err := os.WriteFile(filepath.Join(root, "CONTRIBUTING.md"), []byte("# Contributing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}
