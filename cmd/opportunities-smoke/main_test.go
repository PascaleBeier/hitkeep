package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareWorkingDBCopiesSourceWithoutMutatingIt(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.db")
	if err := os.WriteFile(source, []byte("restored-backup"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	working, cleanup, err := prepareWorkingDB(source)
	if err != nil {
		t.Fatalf("prepare working db: %v", err)
	}
	t.Cleanup(cleanup)
	if working == source {
		t.Fatal("expected working db to be a copy, not the source path")
	}
	if err := os.WriteFile(working, []byte("mutated"), 0o600); err != nil {
		t.Fatalf("mutate working db: %v", err)
	}
	sourceContents, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	if string(sourceContents) != "restored-backup" {
		t.Fatalf("source db was mutated: %q", sourceContents)
	}
}
