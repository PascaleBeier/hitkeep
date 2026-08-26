package hitkeepcmd

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

const versionCommandSubprocessEnv = "HITKEEP_VERSION_COMMAND_SUBPROCESS"

func TestVersionCommand(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantRun    []string
		wantConfig string
	}{
		{name: "version", args: []string{"--version"}},
		{name: "config prefix", args: []string{"--config", "ignored.yaml", "--version"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			var gotArgs []string
			var gotConfig string
			root := newRootCommand(rootActions{
				run: func(args []string, configFile string) error {
					gotArgs = append([]string(nil), args...)
					gotConfig = configFile
					return nil
				},
			})
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			root.SetArgs(test.args)

			if err := root.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if got, want := strings.Join(gotArgs, "\n"), strings.Join(test.wantRun, "\n"); got != want {
				t.Errorf("run args = %q, want %q", gotArgs, test.wantRun)
			}
			if gotConfig != test.wantConfig {
				t.Errorf("run config = %q, want %q", gotConfig, test.wantConfig)
			}
			if len(test.wantRun) == 0 {
				if got, want := stdout.String(), Version+"\n"; got != want {
					t.Errorf("stdout = %q, want %q", got, want)
				}
				if got := stderr.String(); got != "" {
					t.Errorf("stderr = %q, want empty", got)
				}
			}
		})
	}
}

func TestVersionCommandSubprocess(t *testing.T) {
	if os.Getenv(versionCommandSubprocessEnv) == "1" {
		root := newRootCommand(rootActions{
			run: func([]string, string) error {
				os.Exit(42)
				return nil
			},
		})
		root.SetOut(os.Stdout)
		root.SetErr(os.Stderr)
		root.SetArgs(strings.Split(os.Getenv(versionCommandSubprocessEnv+"_ARGS"), "\n"))
		if err := root.Execute(); err != nil {
			_, _ = os.Stderr.WriteString(err.Error() + "\n")
			os.Exit(1)
		}
		os.Exit(0)
	}

	for _, args := range [][]string{
		{"--version"},
		{"--config", "ignored.yaml", "--version"},
	} {
		command := exec.Command(os.Args[0], "-test.run=^TestVersionCommandSubprocess$")
		command.Env = append(os.Environ(),
			versionCommandSubprocessEnv+"=1",
			versionCommandSubprocessEnv+"_ARGS="+strings.Join(args, "\n"),
		)
		var stdout, stderr bytes.Buffer
		command.Stdout = &stdout
		command.Stderr = &stderr

		if err := command.Run(); err != nil {
			t.Fatalf("version subprocess %q failed: %v; stderr = %q", args, err, stderr.String())
		}
		if got, want := stdout.String(), Version+"\n"; got != want {
			t.Errorf("version subprocess %q stdout = %q, want %q", args, got, want)
		}
		if got := stderr.String(); got != "" {
			t.Errorf("version subprocess %q stderr = %q, want empty", args, got)
		}
	}
}
