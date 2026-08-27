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

func clearSmokeEnvironment(t *testing.T) {
	t.Helper()
	for _, binding := range smokeCatalogBindings {
		t.Setenv(smokeCatalogSetting(binding.field).Environment, "")
	}
}

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
	clearSmokeEnvironment(t)
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
		t.Fatalf("unexpected defaults: provider=%q model=%q region=%q data-path=%q", got.Provider, got.Model, got.Region, got.DataPath)
	}
	if got.APIKey != apiKey {
		t.Fatal("API key was not passed through typed configuration")
	}
	if strings.Contains(stdout.String()+stderr.String(), apiKey) {
		t.Fatalf("API key leaked to command output: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestSmokeCommandRetainsSingleDashLongFlagsAndRedactsRunnerErrors(t *testing.T) {
	clearSmokeEnvironment(t)
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
	clearSmokeEnvironment(t)
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
		t.Fatalf("flag values did not win: provider=%q model=%q region=%q base-url=%q data-path=%q", got.Provider, got.Model, got.Region, got.BaseURL, got.DataPath)
	}
}

func TestSmokeCommandEnvironmentValues(t *testing.T) {
	clearSmokeEnvironment(t)
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
		t.Fatalf("environment values missing: provider=%q model=%q region=%q base-url=%q data-path=%q", got.Provider, got.Model, got.Region, got.BaseURL, got.DataPath)
	}
}

func TestSmokeCommandHelpAndValidation(t *testing.T) {
	clearSmokeEnvironment(t)
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	if code := executeSmoke(t.Context(), []string{"--help"}, stdout, stderr, runSmoke); code != 0 {
		t.Fatalf("help exit code = %d, want 0", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("help stdout = %q, want empty", stdout.String())
	}
	for _, want := range []string{"--db string", "--provider string", "--window-days int"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("help output missing %q: %s", want, stderr.String())
		}
	}

	stderr.Reset()
	if code := executeSmoke(t.Context(), nil, new(bytes.Buffer), stderr, runSmoke); code != 1 {
		t.Fatalf("missing db exit code = %d, want 1", code)
	}
	if got := stderr.String(); !strings.Contains(got, "-db is required") || strings.Contains(got, "Usage:") {
		t.Fatalf("validation stderr = %q", got)
	}
}

func TestSmokeCommandLegacyParseBoundary(t *testing.T) {
	clearSmokeEnvironment(t)
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantUsage  bool
	}{
		{name: "short help", args: []string{"-h"}, wantCode: 0, wantUsage: true},
		{name: "long help", args: []string{"--help"}, wantCode: 0, wantUsage: true},
		{name: "legacy help", args: []string{"-help"}, wantCode: 0, wantUsage: true},
		{name: "false short help", args: []string{"-h=false"}, wantCode: 0, wantUsage: true},
		{name: "false long help", args: []string{"--help=false"}, wantCode: 0, wantUsage: true},
		{name: "unknown flag", args: []string{"-unknown"}, wantCode: 2, wantUsage: true},
		{name: "invalid integer", args: []string{"-window-days=not-a-number"}, wantCode: 2, wantUsage: true},
		{name: "invalid boolean", args: []string{"-ai=not-a-bool"}, wantCode: 2, wantUsage: true},
		{name: "runtime failure", args: []string{"-db", "restored.db", "-out", filepath.Join(t.TempDir(), "report.md")}, wantCode: 1},
		{name: "release not ready", args: []string{"-db", "restored.db", "-out", filepath.Join(t.TempDir(), "not-ready.md")}, wantCode: 2, wantStdout: "not-ready.md\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout := new(bytes.Buffer)
			stderr := new(bytes.Buffer)
			code := executeSmoke(t.Context(), tt.args, stdout, stderr, func(_ context.Context, _ smokeConfig) (smokegate.Report, error) {
				if tt.name == "runtime failure" {
					return smokegate.Report{}, errors.New("runtime failure")
				}
				return smokegate.Report{}, nil
			})
			if code != tt.wantCode {
				t.Fatalf("exit code = %d, want %d", code, tt.wantCode)
			}
			if tt.wantUsage != strings.Contains(stderr.String(), "Usage") {
				t.Fatalf("stderr usage = %q, want usage=%t", stderr.String(), tt.wantUsage)
			}
			if tt.wantUsage && stdout.Len() != 0 {
				t.Fatalf("syntax stdout = %q, want empty", stdout.String())
			}
			if tt.name == "runtime failure" && (strings.Contains(stderr.String(), "Usage") || stderr.String() != "runtime failure\n") {
				t.Fatalf("runtime stderr = %q", stderr.String())
			}
			if tt.wantStdout != "" && !strings.HasSuffix(stdout.String(), tt.wantStdout) {
				t.Fatalf("stdout = %q, want suffix %q", stdout.String(), tt.wantStdout)
			}
		})
	}
}

func TestSmokeCommandUsesCanonicalCatalogAIEnvironmentMetadata(t *testing.T) {
	clearSmokeEnvironment(t)
	for _, binding := range smokeCatalogBindings {
		setting := smokeCatalogSetting(binding.field)
		if setting.Environment == "" {
			t.Fatalf("catalog setting %s has no environment variable", binding.field)
		}
	}
	apiKey := smokeCatalogSetting("AIAPIKey")
	if apiKey.Default != "" || apiKey.Sensitive != "redact" {
		t.Fatalf("AI API key catalog metadata = %+v", apiKey)
	}

	t.Setenv(smokeCatalogSetting("AIProvider").Environment, "catalog-provider")
	var got smokeConfig
	code := executeSmoke(t.Context(), []string{"-db", "restored.db", "-out", filepath.Join(t.TempDir(), "report.md")}, new(bytes.Buffer), new(bytes.Buffer), func(_ context.Context, conf smokeConfig) (smokegate.Report, error) {
		got = conf
		return smokegate.Report{}, nil
	})
	if code != 2 || got.Provider != "catalog-provider" {
		t.Fatalf("catalog environment binding result = code %d, provider=%q", code, got.Provider)
	}
}

func TestSmokeCommandUsesCanonicalCatalogDefaults(t *testing.T) {
	clearSmokeEnvironment(t)
	cmd := newSmokeCommand(runSmoke)
	for _, field := range []string{"AIBaseURL", "DataPath"} {
		setting := smokeCatalogSetting(field)
		flag := map[string]string{"AIBaseURL": "base-url", "DataPath": "data-path"}[field]
		value, err := cmd.Flags().GetString(flag)
		if err != nil {
			t.Fatalf("read %s default: %v", flag, err)
		}
		if value != setting.Default {
			t.Fatalf("%s default = %q, catalog = %q", flag, value, setting.Default)
		}
	}
}

func TestSmokeCommandPassesExecutionContext(t *testing.T) {
	clearSmokeEnvironment(t)
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
