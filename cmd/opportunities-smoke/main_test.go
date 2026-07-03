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

func TestResolveSmokeAIBaseURLDefaultsMantleForOpenAICompatible(t *testing.T) {
	got := resolveSmokeAIBaseURL("openai-compatible", "eu-west-1", "")
	if got != "https://bedrock-mantle.eu-west-1.api.aws/v1" {
		t.Fatalf("expected regional Mantle base URL, got %q", got)
	}
}

func TestResolveSmokeAIBaseURLUsesDefaultRegionForOpenAICompatible(t *testing.T) {
	got := resolveSmokeAIBaseURL(" openai-compatible ", " ", "")
	if got != "https://bedrock-mantle.eu-central-1.api.aws/v1" {
		t.Fatalf("expected default-region Mantle base URL, got %q", got)
	}
}

func TestResolveSmokeAIBaseURLPreservesExplicitURLAndOtherProviders(t *testing.T) {
	if got := resolveSmokeAIBaseURL("openai-compatible", "eu-west-1", " https://gateway.example/v1 "); got != "https://gateway.example/v1" {
		t.Fatalf("expected explicit base URL to win, got %q", got)
	}
	if got := resolveSmokeAIBaseURL("bedrock", "eu-west-1", ""); got != "" {
		t.Fatalf("expected direct Bedrock to have no OpenAI-compatible base URL, got %q", got)
	}
}
