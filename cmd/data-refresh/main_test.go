package main

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDispatchRejectsUnknownTargetAndFlags(t *testing.T) {
	for _, args := range [][]string{{}, {"unknown"}, {"ai-agents", "-bogus"}, {"spam", "-bogus"}, {"duckdb", "-bogus"}, {"ai-agents", "-output", "x", "extra"}} {
		var out bytes.Buffer
		if err := run(context.Background(), args, &out); err == nil {
			t.Fatalf("args %q accepted", args)
		}
		if out.Len() != 0 {
			t.Fatalf("args %q wrote output: %q", args, out.String())
		}
	}
}

func TestIPBadFlagFailsBeforeGeneration(t *testing.T) {
	var out bytes.Buffer
	if err := run(context.Background(), []string{"ipmeta", "-bogus"}, &out); err == nil {
		t.Fatal("IP flag accepted")
	}
}

func TestGeneratorFailureExitsNonzero(t *testing.T) {
	output := filepath.Join(t.TempDir(), "ipmeta.go")
	missing := filepath.Join(t.TempDir(), "missing.csv")
	command := exec.CommandContext(t.Context(), "go", "run", "./cmd/data-refresh", "ipmeta", "-db1", missing, "-out", output)
	command.Dir = filepath.Join("..", "..")
	if err := command.Run(); err == nil {
		t.Fatal("failed generator exited successfully")
	}
}

func TestDuckDBCheckAcceptsOnlyCheckFlag(t *testing.T) {
	var out bytes.Buffer
	err := run(context.Background(), []string{"duckdb", "-output", "x"}, &out)
	if err == nil || !strings.Contains(err.Error(), "flag") {
		t.Fatalf("unexpected error: %v", err)
	}
}
