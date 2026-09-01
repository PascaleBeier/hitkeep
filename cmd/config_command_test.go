package hitkeepcmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"

	runtimeconfig "hitkeep/config"
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
		{name: "uppercase key", path: filepath.Join(directory, "uppercase.yaml"), contents: "DATA-PATH: /tmp/hitkeep\n", want: "unknown configuration key"},
		{name: "mixed-case key", path: filepath.Join(directory, "mixed-case.yaml"), contents: "Data-Path: /tmp/hitkeep\n", want: "unknown configuration key"},
		{name: "nested key", path: filepath.Join(directory, "nested.yaml"), contents: "data:\n  path: /tmp/hitkeep\n", want: "unknown configuration key"},
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

func TestConfigFallbackUsesPublicRootRuntime(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	configPath := filepath.Join(t.TempDir(), "missing.yaml")
	args := []string{"config", "foo"}
	want := runContext(context.Background(), logger, args, configPath)
	if want == nil {
		t.Fatal("legacy runtime accepted missing explicit configuration")
	}

	root := NewRootCommand(logger)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	got := ExecuteRoot(context.Background(), root, []string{"--config", configPath, "config", "foo"})
	if got == nil {
		t.Fatal("public root accepted missing explicit configuration")
	}
	if got.Error() != want.Error() {
		t.Fatalf("public root error = %q, want legacy error %q", got, want)
	}
	if !strings.Contains(got.Error(), configPath) {
		t.Fatalf("public root error = %q, want selected config path %q", got, configPath)
	}
}

func TestConfigCommandPreservesLegacyFallback(t *testing.T) {
	for _, configArgs := range [][]string{{"--config", "leading.yaml"}, {"--config=leading.yaml"}} {
		for _, args := range [][]string{{}, {"foo"}, {"--legacy-flag"}} {
			t.Run(strings.Join(append(append([]string(nil), configArgs...), args...), "_"), func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				var got []string
				var gotConfig string
				actions := rootActions{
					runContext: func(gotCtx context.Context, args []string, configFile string) error {
						if gotCtx.Err() != context.Canceled {
							t.Fatalf("fallback context error = %v, want %v", gotCtx.Err(), context.Canceled)
						}
						got = append([]string(nil), args...)
						gotConfig = configFile
						return nil
					},
				}
				root := newRootCommand(actions)
				root.AddCommand(newConfigCommand(
					afero.NewMemMapFs(),
					slog.New(slog.NewTextHandler(io.Discard, nil)),
					func(command *cobra.Command, args []string) error {
						return actions.runWithContext(command.Context(), args, rootConfigFile(command.Context()))
					},
				))
				root.SetOut(io.Discard)
				root.SetErr(io.Discard)
				commandArgs := append(append(append([]string(nil), configArgs...), "config"), args...)
				if err := ExecuteRoot(ctx, root, commandArgs); err != nil {
					t.Fatalf("execute config fallback: %v", err)
				}
				want := append([]string{"config"}, args...)
				if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
					t.Fatalf("fallback args = %q, want %q", got, want)
				}
				if gotConfig != "leading.yaml" {
					t.Fatalf("fallback config = %q, want %q", gotConfig, "leading.yaml")
				}
			})
		}
	}
}

type replacingWriteFS struct {
	afero.Fs
	replacement []byte
}

func (fs *replacingWriteFS) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	file, err := fs.Fs.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	return &replacingWriteFile{File: file, fs: fs.Fs, path: name, replacement: fs.replacement}, nil
}

type replacingWriteFile struct {
	afero.File
	fs          afero.Fs
	path        string
	replacement []byte
}

func (file *replacingWriteFile) Write([]byte) (int, error) {
	_ = file.File.Close()
	_ = file.fs.Remove(file.path)
	if err := afero.WriteFile(file.fs, file.path, file.replacement, 0o600); err != nil {
		return 0, err
	}
	return 0, errors.New("injected write failure")
}

func TestConfigInitWriteFailurePreservesReplacementPath(t *testing.T) {
	base := afero.NewMemMapFs()
	const outputPath = "hitkeep.yaml"
	replacement := []byte("replacement-owned\n")
	command := newConfigInitCommand(&replacingWriteFS{Fs: base, replacement: replacement})
	command.SetOut(io.Discard)
	command.SetErr(io.Discard)
	command.SetArgs([]string{"--output", outputPath})
	if _, err := command.ExecuteC(); err == nil {
		t.Fatal("config init unexpectedly succeeded after an injected write failure")
	}
	contents, err := afero.ReadFile(base, outputPath)
	if err != nil {
		t.Fatalf("read replacement path: %v", err)
	}
	if !bytes.Equal(contents, replacement) {
		t.Fatalf("replacement contents = %q, want %q", contents, replacement)
	}
}
