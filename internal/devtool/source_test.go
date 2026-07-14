package devtool

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFormatGoChecksAndWritesWorkspaceSources(t *testing.T) {
	workspace := t.TempDir()
	if output, err := exec.Command("git", "init", "--quiet", workspace).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	unformatted := []byte("package example\n\nfunc Value( )int{return 1}\n")
	path := filepath.Join(workspace, "value.go")
	if err := os.WriteFile(path, unformatted, 0o644); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(t.TempDir(), "outside.go")
	if err := os.WriteFile(external, []byte("not valid Go"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(workspace, "outside.go")); err != nil {
		t.Fatal(err)
	}

	app, err := NewApp(workspace)
	if err != nil {
		t.Fatal(err)
	}
	checked, err := app.FormatGo(false)
	if err == nil {
		t.Fatal("format check unexpectedly passed")
	}
	var dataError interface{ ErrorData() any }
	if !errors.As(err, &dataError) {
		t.Fatalf("format check error has no structured result: %T", err)
	}
	if checked.Current || checked.ChangedFileCount != 1 || !reflect.DeepEqual(checked.ChangedFiles, []string{"value.go"}) {
		t.Fatalf("unexpected format check: %+v", checked)
	}
	if raw, readErr := os.ReadFile(path); readErr != nil || !reflect.DeepEqual(raw, unformatted) {
		t.Fatalf("check modified source: %v: %q", readErr, raw)
	}

	written, err := app.FormatGo(true)
	if err != nil {
		t.Fatal(err)
	}
	if !written.Current || written.ChangedFileCount != 1 {
		t.Fatalf("unexpected write result: %+v", written)
	}
	current, err := app.FormatGo(false)
	if err != nil || !current.Current || current.ChangedFileCount != 0 {
		t.Fatalf("formatted workspace is not current: %+v, %v", current, err)
	}
	if raw, readErr := os.ReadFile(external); readErr != nil || string(raw) != "not valid Go" {
		t.Fatalf("format followed workspace symlink: %v: %q", readErr, raw)
	}
}

func TestGoFixChangedFilesAreBoundedToWorkspacePaths(t *testing.T) {
	workspace := filepath.Join(string(filepath.Separator), "work", "hitkeep")
	output := []byte("diff value.go.orig value.go\n--- value.go.orig\n+++ value.go (new)\n" +
		"diff internal/other.go.orig internal/other.go\n--- internal/other.go.orig\n+++ internal/other.go\n" +
		"--- outside.go.orig\n+++ ../outside.go\n")
	want := []string{"internal/other.go", "value.go"}
	if got := goFixChangedFiles(output, workspace); !reflect.DeepEqual(got, want) {
		t.Fatalf("changed paths = %v, want %v", got, want)
	}
}

func TestErrorEnvelopePreservesStructuredSourceDrift(t *testing.T) {
	result := SourceChangeResult{Tool: "gofmt", Mode: "check", ChangedFiles: []string{"main.go"}, ChangedFileCount: 1}
	envelope := ErrorEnvelope("fmt check", "workspace", sourceChangesPendingError{result: result})
	if !reflect.DeepEqual(envelope.Data, result) {
		t.Fatalf("structured error data = %#v, want %#v", envelope.Data, result)
	}
}
