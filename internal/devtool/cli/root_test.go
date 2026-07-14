package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

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
