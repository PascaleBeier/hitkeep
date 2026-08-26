package hitkeepcmd

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	runtimeconfig "hitkeep/internal/config"
)

func executeProductionCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()
	command := NewRootCommand(slog.New(slog.NewTextHandler(io.Discard, nil)))
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs(args)
	_, err := command.ExecuteC()
	return output.String(), err
}

func TestConfigInitWritesCanonicalExampleWithoutOverwriting(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "hitkeep.yaml")

	if _, err := executeProductionCommand(t, "config", "init", "--output", outputPath); err != nil {
		t.Fatalf("config init: %v", err)
	}
	contents, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read generated configuration: %v", err)
	}
	if !bytes.Equal(contents, runtimeconfig.RenderExampleYAML()) {
		t.Fatal("config init output differs from the canonical generated example")
	}

	const existing = "operator-owned\n"
	if err := os.WriteFile(outputPath, []byte(existing), 0o600); err != nil {
		t.Fatalf("replace generated configuration: %v", err)
	}
	if _, err := executeProductionCommand(t, "config", "init", "--output", outputPath); err == nil {
		t.Fatal("config init unexpectedly overwrote an existing file")
	}
	preserved, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read preserved configuration: %v", err)
	}
	if string(preserved) != existing {
		t.Fatalf("existing configuration changed: %q", preserved)
	}
}

func TestConfigValidateUsesStrictExplicitFileLoader(t *testing.T) {
	directory := t.TempDir()
	validPath := filepath.Join(directory, "valid.yaml")
	if err := os.WriteFile(validPath, runtimeconfig.RenderExampleYAML(), 0o600); err != nil {
		t.Fatalf("write valid configuration: %v", err)
	}
	if _, err := executeProductionCommand(t, "config", "validate", "--config", validPath); err != nil {
		t.Fatalf("validate canonical configuration: %v", err)
	}

	secretPath := filepath.Join(directory, "secret.yaml")
	const secret = "must-not-appear-in-output"
	if err := os.WriteFile(secretPath, []byte("jwt-secret: "+secret+"\n"), 0o600); err != nil {
		t.Fatalf("write sensitive configuration: %v", err)
	}
	output, err := executeProductionCommand(t, "config", "validate", "--config", secretPath)
	if err != nil {
		t.Fatalf("validate sensitive configuration: %v", err)
	}
	if strings.Contains(output, secret) {
		t.Fatal("config validate exposed a sensitive value")
	}

	tests := []struct {
		name     string
		path     string
		contents string
		want     string
	}{
		{name: "missing", path: filepath.Join(directory, "missing.yaml"), want: "read configuration file"},
		{name: "malformed", path: filepath.Join(directory, "malformed.yaml"), contents: "server: [", want: "invalid YAML"},
		{name: "unknown key", path: filepath.Join(directory, "unknown.yaml"), contents: "not_a_hitkeep_setting: true\n", want: "unknown configuration key"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.contents != "" {
				if err := os.WriteFile(test.path, []byte(test.contents), 0o600); err != nil {
					t.Fatalf("write configuration: %v", err)
				}
			}
			_, err := executeProductionCommand(t, "config", "validate", "--config", test.path)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("config validate error = %v, want substring %q", err, test.want)
			}
		})
	}
}
