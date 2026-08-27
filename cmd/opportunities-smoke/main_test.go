package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hitkeep/internal/opportunities/smokegate"
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

func TestSmokeCommandUsesDefaultsAndDoesNotPrintAPIKey(t *testing.T) {
	const apiKey = "super-secret-api-key"
	t.Setenv("HITKEEP_AI_API_KEY", " "+apiKey+" ")

	var got smokeConfig
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	exitCode := executeSmoke(t.Context(), []string{"--db", "restored.db", "--out", filepath.Join(t.TempDir(), "report.md")}, stdout, stderr, func(_ context.Context, conf smokeConfig) (smokegate.Report, error) {
		got = conf
		return smokegate.Report{}, nil
	})

	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}
	if got.Provider != "openai-compatible" || got.Model != "openai.gpt-oss-120b" || got.Region != "eu-central-1" || got.DataPath != "data" {
		t.Fatalf("unexpected defaults: %+v", got)
	}
	if got.APIKey != apiKey {
		t.Fatal("API key was not passed through typed configuration")
	}
	if strings.Contains(stdout.String()+stderr.String(), apiKey) {
		t.Fatalf("API key leaked to command output: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestSmokeCommandRetainsSingleDashLongFlagsAndRedactsRunnerErrors(t *testing.T) {
	const apiKey = "super-secret-api-key"
	t.Setenv("HITKEEP_AI_API_KEY", apiKey)
	stderr := new(bytes.Buffer)
	code := executeSmoke(t.Context(), []string{"-db", "restored.db", "-out", filepath.Join(t.TempDir(), "report.md")}, new(bytes.Buffer), stderr, func(_ context.Context, _ smokeConfig) (smokegate.Report, error) {
		return smokegate.Report{}, errors.New("upstream rejected " + apiKey)
	})
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if got := stderr.String(); strings.Contains(got, apiKey) || !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("runner error was not redacted: %q", got)
	}
}

func TestSmokeCommandFlagPrecedenceOverEnvironment(t *testing.T) {
	t.Setenv("HITKEEP_AI_PROVIDER", "from-env")
	t.Setenv("HITKEEP_AI_MODEL", "env-model")
	t.Setenv("HITKEEP_AI_REGION", "eu-west-1")
	t.Setenv("HITKEEP_AI_BASE_URL", "https://env.example/v1")
	t.Setenv("HITKEEP_DATA_PATH", "env-data")

	var got smokeConfig
	exitCode := executeSmoke(t.Context(), []string{
		"--db", "restored.db",
		"--out", filepath.Join(t.TempDir(), "report.md"),
		"--provider", "from-flag",
		"--model", "flag-model",
		"--region", "us-east-1",
		"--base-url", "https://flag.example/v1",
		"--data-path", "flag-data",
	}, new(bytes.Buffer), new(bytes.Buffer), func(_ context.Context, conf smokeConfig) (smokegate.Report, error) {
		got = conf
		return smokegate.Report{}, nil
	})

	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}
	if got.Provider != "from-flag" || got.Model != "flag-model" || got.Region != "us-east-1" || got.BaseURL != "https://flag.example/v1" || got.DataPath != "flag-data" {
		t.Fatalf("flag values did not win: %+v", got)
	}
}

func TestSmokeCommandEnvironmentValues(t *testing.T) {
	t.Setenv("HITKEEP_AI_PROVIDER", "from-env")
	t.Setenv("HITKEEP_AI_MODEL", "env-model")
	t.Setenv("HITKEEP_AI_REGION", "eu-west-1")
	t.Setenv("HITKEEP_AI_BASE_URL", "https://env.example/v1")
	t.Setenv("HITKEEP_DATA_PATH", "env-data")

	var got smokeConfig
	exitCode := executeSmoke(t.Context(), []string{"--db", "restored.db", "--out", filepath.Join(t.TempDir(), "report.md")}, new(bytes.Buffer), new(bytes.Buffer), func(_ context.Context, conf smokeConfig) (smokegate.Report, error) {
		got = conf
		return smokegate.Report{}, nil
	})

	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}
	if got.Provider != "from-env" || got.Model != "env-model" || got.Region != "eu-west-1" || got.BaseURL != "https://env.example/v1" || got.DataPath != "env-data" {
		t.Fatalf("environment values missing: %+v", got)
	}
}

func TestSmokeCommandHelpAndValidation(t *testing.T) {
	stdout := new(bytes.Buffer)
	if code := executeSmoke(t.Context(), []string{"--help"}, stdout, new(bytes.Buffer), runSmoke); code != 0 {
		t.Fatalf("help exit code = %d, want 0", code)
	}
	for _, want := range []string{"--db string", "--provider string", "--window-days int"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("help output missing %q: %s", want, stdout.String())
		}
	}

	stderr := new(bytes.Buffer)
	if code := executeSmoke(t.Context(), nil, new(bytes.Buffer), stderr, runSmoke); code != 1 {
		t.Fatalf("missing db exit code = %d, want 1", code)
	}
	if got := stderr.String(); !strings.Contains(got, "-db is required") || strings.Contains(got, "Usage:") {
		t.Fatalf("validation stderr = %q", got)
	}
}

func TestSmokeCommandPassesExecutionContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var got context.Context

	code := executeSmoke(ctx, []string{"--db", "restored.db", "--out", filepath.Join(t.TempDir(), "report.md")}, new(bytes.Buffer), new(bytes.Buffer), func(ctx context.Context, _ smokeConfig) (smokegate.Report, error) {
		got = ctx
		return smokegate.Report{}, nil
	})
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if got == nil || !errors.Is(got.Err(), context.Canceled) {
		t.Fatalf("runner context = %v, want canceled", got)
	}
}
