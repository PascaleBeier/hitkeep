package hitkeepcmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
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
	if legacyExit, commandExit := legacyErr.(*exec.ExitError).ExitCode(), commandErr.(*exec.ExitError).ExitCode(); legacyExit != commandExit {
		t.Errorf("healthcheck exit codes = %d, %d", legacyExit, commandExit)
	}
	if legacyStdout.String() != commandStdout.String() {
		t.Errorf("healthcheck stdout differs: legacy %q, command %q", legacyStdout.String(), commandStdout.String())
	}
	if legacyStderr.String() != commandStderr.String() {
		t.Errorf("healthcheck stderr differs: legacy %q, command %q", legacyStderr.String(), commandStderr.String())
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
			if !reflect.DeepEqual(gotArgs, tt.wantArgs) {
				t.Errorf("run args = %#v, want %#v", gotArgs, tt.wantArgs)
			}
			if gotConfig != tt.wantConfig {
				t.Errorf("run config = %q, want %q", gotConfig, tt.wantConfig)
			}
		})
	}
}
