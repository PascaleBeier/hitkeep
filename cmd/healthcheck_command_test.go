package hitkeepcmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"testing"
)

func TestHealthcheckCommandSubprocessParity(t *testing.T) {
	if os.Getenv("HITKEEP_HEALTHCHECK_SUBPROCESS") == "1" {
		args := os.Args
		for i, arg := range args {
			if arg == "--" {
				args = args[i+1:]
				break
			}
		}
		root := newRootCommand(rootActions{
			run: func([]string, string) error {
				if os.Getenv("HITKEEP_HEALTHCHECK_SUBPROCESS_SUCCESS") == "1" {
					return nil
				}
				return errors.New("healthcheck failed")
			},
		})
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	var legacyStdout, legacyStderr bytes.Buffer
	legacy := exec.Command(os.Args[0], "-test.run=^TestHealthcheckCommandSubprocessParity$", "--", "--healthcheck")
	legacy.Env = append(os.Environ(), "HITKEEP_HEALTHCHECK_SUBPROCESS=1")
	legacy.Stdout = &legacyStdout
	legacy.Stderr = &legacyStderr
	legacyErr := legacy.Run()

	var commandStdout, commandStderr bytes.Buffer
	command := exec.Command(os.Args[0], "-test.run=^TestHealthcheckCommandSubprocessParity$", "--", "healthcheck")
	command.Env = append(os.Environ(), "HITKEEP_HEALTHCHECK_SUBPROCESS=1")
	command.Stdout = &commandStdout
	command.Stderr = &commandStderr
	commandErr := command.Run()

	if legacyErr == nil || commandErr == nil {
		t.Fatalf("healthcheck exit errors = %v, %v; both commands must fail", legacyErr, commandErr)
	}
	if got := legacyErr.(*exec.ExitError).ExitCode(); got != 1 {
		t.Errorf("legacy healthcheck exit code = %d, want 1", got)
	}
	if got := commandErr.(*exec.ExitError).ExitCode(); got != 1 {
		t.Errorf("healthcheck command exit code = %d, want 1", got)
	}
	if got := legacyStdout.String(); got != "" {
		t.Errorf("legacy healthcheck stdout = %q, want empty", got)
	}
	if got := commandStdout.String(); got != "" {
		t.Errorf("healthcheck command stdout = %q, want empty", got)
	}
	if got := legacyStderr.String(); got != "healthcheck failed\n" {
		t.Errorf("legacy healthcheck stderr = %q, want %q", got, "healthcheck failed\n")
	}
	if got := commandStderr.String(); got != "healthcheck failed\n" {
		t.Errorf("healthcheck command stderr = %q, want %q", got, "healthcheck failed\n")
	}
}

func TestHealthcheckCommandSubprocessSuccessParity(t *testing.T) {
	var legacyStdout, legacyStderr bytes.Buffer
	legacy := exec.Command(os.Args[0], "-test.run=^TestHealthcheckCommandSubprocessParity$", "--", "--healthcheck")
	legacy.Env = append(os.Environ(), "HITKEEP_HEALTHCHECK_SUBPROCESS=1", "HITKEEP_HEALTHCHECK_SUBPROCESS_SUCCESS=1")
	legacy.Stdout = &legacyStdout
	legacy.Stderr = &legacyStderr
	if err := legacy.Run(); err != nil {
		t.Fatalf("legacy healthcheck error = %v", err)
	}

	var commandStdout, commandStderr bytes.Buffer
	command := exec.Command(os.Args[0], "-test.run=^TestHealthcheckCommandSubprocessParity$", "--", "healthcheck")
	command.Env = append(os.Environ(), "HITKEEP_HEALTHCHECK_SUBPROCESS=1", "HITKEEP_HEALTHCHECK_SUBPROCESS_SUCCESS=1")
	command.Stdout = &commandStdout
	command.Stderr = &commandStderr
	if err := command.Run(); err != nil {
		t.Fatalf("healthcheck command error = %v", err)
	}
	if got := legacyStdout.String(); got != "" {
		t.Errorf("legacy healthcheck stdout = %q, want empty", got)
	}
	if got := commandStdout.String(); got != "" {
		t.Errorf("healthcheck command stdout = %q, want empty", got)
	}
	if got := legacyStderr.String(); got != "" {
		t.Errorf("legacy healthcheck stderr = %q, want empty", got)
	}
	if got := commandStderr.String(); got != "" {
		t.Errorf("healthcheck command stderr = %q, want empty", got)
	}
}

func TestRootCommandCompatibilityContract(t *testing.T) {
	tests := []struct {
		name       string
		input      []string
		wantArgs   []string
		wantConfig string
		wantErr    string
	}{
		{name: "no subcommand starts the server", input: []string{}, wantArgs: []string{}},
		{name: "legacy healthcheck flag", input: []string{"--healthcheck"}, wantArgs: []string{"--healthcheck"}},
		{name: "bare help remains server input", input: []string{"help"}, wantArgs: []string{"help"}},
		{name: "bare help flag remains server input", input: []string{"--help"}, wantArgs: []string{"--help"}},
		{name: "unknown command remains server input", input: []string{"unknown-command"}, wantArgs: []string{"unknown-command"}},
		{name: "missing config path", input: []string{"--config"}, wantErr: "--config requires a path"},
		{name: "non-leading config remains server input", input: []string{"--listen-address=:8090", "--config", "testdata/hitkeep.yaml"}, wantArgs: []string{"--listen-address=:8090", "--config", "testdata/hitkeep.yaml"}},
		{name: "interspersed server flags remain server input", input: []string{"--listen-address=:8090", "--log-level=debug"}, wantArgs: []string{"--listen-address=:8090", "--log-level=debug"}},
		{name: "root config selects healthcheck configuration", input: []string{"--config", "testdata/hitkeep.yaml", "healthcheck"}, wantArgs: []string{"--healthcheck"}, wantConfig: "testdata/hitkeep.yaml"},
		{name: "healthcheck config selects configuration", input: []string{"healthcheck", "--config", "testdata/hitkeep.yaml"}, wantArgs: []string{"--healthcheck"}, wantConfig: "testdata/hitkeep.yaml"},
		{name: "healthcheck help remains legacy input", input: []string{"healthcheck", "--help"}, wantArgs: []string{"--help", "--healthcheck"}},
		{name: "help healthcheck remains legacy input", input: []string{"help", "healthcheck"}, wantArgs: []string{"help", "healthcheck"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			var gotArgs []string
			var gotConfig string
			root := newRootCommand(rootActions{
				run: func(args []string, configFile string) error {
					called = true
					gotArgs = args
					gotConfig = configFile
					return nil
				},
			})
			root.SetArgs(tt.input)

			err := root.Execute()
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("Execute() error = %v, want %q", err, tt.wantErr)
				}
				if called {
					t.Fatal("run was called for a bootstrap error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if !called {
				t.Fatal("run was not called")
			}
			if !slices.Equal(gotArgs, tt.wantArgs) {
				t.Errorf("run args = %#v, want %#v", gotArgs, tt.wantArgs)
			}
			if gotConfig != tt.wantConfig {
				t.Errorf("run config = %q, want %q", gotConfig, tt.wantConfig)
			}
		})
	}
}

func TestRootCommandNoSubcommandUsesLegacyRuntime(t *testing.T) {
	called := false
	root := newRootCommand(rootActions{
		run: func(args []string, configFile string) error {
			called = true
			if len(args) != 0 {
				t.Errorf("root args = %q, want empty", args)
			}
			if configFile != "" {
				t.Errorf("root config file = %q, want empty", configFile)
			}
			return nil
		},
	})
	root.SetArgs([]string{})

	if err := root.ExecuteContext(t.Context()); err != nil {
		t.Fatalf("ExecuteContext: %v", err)
	}
	if !called {
		t.Fatal("legacy runtime was not called without a subcommand")
	}
}

func TestHealthcheckCommandHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	called := false
	root := newRootCommand(rootActions{
		run: func([]string, string) error {
			called = true
			return nil
		},
	})
	root.SetArgs([]string{"healthcheck"})

	err := root.ExecuteContext(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ExecuteContext() error = %v, want context cancellation", err)
	}
	if called {
		t.Fatal("run was called after command context was canceled")
	}
}

func TestHealthcheckCommandPassesCommandContext(t *testing.T) {
	ctx := context.WithValue(t.Context(), "healthcheck-context", "present")
	called := false
	root := newRootCommand(rootActions{
		runContext: func(got context.Context, args []string, configFile string) error {
			called = true
			if got.Value("healthcheck-context") != "present" {
				t.Errorf("healthcheck context value = %v, want present", got.Value("healthcheck-context"))
			}
			if configFile != "" {
				t.Errorf("healthcheck config file = %q, want empty", configFile)
			}
			if len(args) != 1 || args[0] != "--healthcheck" {
				t.Errorf("healthcheck args = %q, want [--healthcheck]", args)
			}
			return nil
		},
	})
	root.SetArgs([]string{"healthcheck"})
	if err := root.ExecuteContext(ctx); err != nil {
		t.Fatalf("ExecuteContext: %v", err)
	}
	if !called {
		t.Fatal("healthcheck context-aware runtime was not called")
	}
}

func TestRootCommandCompletionShape(t *testing.T) {
	root := newRootCommand(rootActions{})
	var healthcheck, completion bool
	for _, command := range root.Commands() {
		switch command.Name() {
		case "healthcheck":
			healthcheck = true
		case "completion":
			completion = true
		}
	}
	if !healthcheck {
		t.Fatal("healthcheck is not an explicit Cobra subcommand")
	}
	if completion {
		t.Fatal("default Cobra completion command changed legacy command routing")
	}
}

func TestHealthcheckCommandPreservesRootBootstrapGrammar(t *testing.T) {
	tests := []struct {
		name       string
		input      []string
		wantArgs   []string
		wantConfig string
	}{
		{
			name:     "subcommand passes interspersed server flags to legacy handler",
			input:    []string{"healthcheck", "--listen-address=:8090"},
			wantArgs: []string{"--listen-address=:8090", "--healthcheck"},
		},
		{
			name:       "leading root config remains the bootstrap grammar",
			input:      []string{"--config", "testdata/hitkeep.yaml", "healthcheck", "--listen-address=:8090"},
			wantArgs:   []string{"--listen-address=:8090", "--healthcheck"},
			wantConfig: "testdata/hitkeep.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotArgs []string
			var gotConfig string
			root := newRootCommand(rootActions{
				run: func(args []string, configFile string) error {
					gotArgs = args
					gotConfig = configFile
					return nil
				},
			})
			root.SetArgs(tt.input)

			if err := root.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if !slices.Equal(gotArgs, tt.wantArgs) {
				t.Errorf("run args = %#v, want %#v", gotArgs, tt.wantArgs)
			}
			if gotConfig != tt.wantConfig {
				t.Errorf("run config = %q, want %q", gotConfig, tt.wantConfig)
			}
		})
	}
}
