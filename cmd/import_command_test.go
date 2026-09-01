package hitkeepcmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportCommandUsesCobraContextAndStreams(t *testing.T) {
	type contextKey struct{}
	ctx := context.WithValue(t.Context(), contextKey{}, "value")
	var stdin, stdout, stderr bytes.Buffer
	stdin.WriteString("yes\n")
	called := false
	root := newRootCommand(rootActions{
		importData: func(got context.Context, args []string, in io.Reader, out, errOut io.Writer, configFile string) error {
			if configFile != "/tmp/hitkeep.yaml" {
				t.Fatalf("configFile = %q, want /tmp/hitkeep.yaml", configFile)
			}
			called = true
			if got.Value(contextKey{}) != "value" {
				t.Fatal("import command did not receive the Cobra context")
			}
			if gotArgs := "list --site site"; len(args) != 3 || args[0]+" "+args[1]+" "+args[2] != gotArgs {
				t.Fatalf("args = %q, want %q", args, gotArgs)
			}
			if in != &stdin || out != &stdout || errOut != &stderr {
				t.Fatal("import command did not receive command streams")
			}
			return nil
		},
	})
	root.SetIn(&stdin)
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	if err := ExecuteRoot(ctx, root, []string{"--config", "/tmp/hitkeep.yaml", "import", "list", "--site", "site"}); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	if !called {
		t.Fatal("import command was not called")
	}
}

func TestImportCommandUsesTypedConfigurationAndFlagOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hitkeep.yaml")
	if err := os.WriteFile(path, []byte("public-url: http://public-file.example\napi-url: http://api-file.example\napi-token: file-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HITKEEP_API_URL", "http://api-env.example")
	t.Setenv("HITKEEP_API_TOKEN", "env-token")
	command, err := newImportCommand(t.Context(), nil, io.Discard, io.Discard, path, nil)
	if err != nil {
		t.Fatal(err)
	}
	options, err := command.parseOptions([]string{"--site", "site", "--url", "http://flag.example", "--token", "flag-token"})
	if err != nil {
		t.Fatal(err)
	}
	if options.apiURL != "http://flag.example" || options.token != "flag-token" {
		t.Fatalf("flag overrides = (%q, %q), want flag values", options.apiURL, options.token)
	}
}

func TestImportCommandPreservesHelpAndValidationExitSemantics(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int
	}{
		{name: "help", args: []string{"list", "--help"}, want: 0},
		{name: "missing token", args: []string{"list", "--site", "site"}, want: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			command := importCommand{ctx: t.Context(), out: &stdout, errOut: &stderr, apiURL: defaultImportAPIURL}
			err := command.run(tt.args)
			if tt.want == 0 {
				if err != nil {
					t.Fatalf("run() error = %v", err)
				}
			} else if exitErr, ok := errors.AsType[*ExitError](err); !ok || exitErr.Code != tt.want {
				t.Fatalf("run() error = %v, want ExitError code %d", err, tt.want)
			}
			if got := stdout.String(); got != "" {
				t.Errorf("stdout = %q, want empty", got)
			}
			if tt.want == 0 && !strings.HasPrefix(stderr.String(), "Usage of import:") {
				t.Errorf("help stderr = %q, want stdlib usage", stderr.String())
			}
			if tt.want == 2 && stderr.String() != "--token or HITKEEP_API_TOKEN is required\n" {
				t.Errorf("validation stderr = %q", stderr.String())
			}
		})
	}
}

func TestImportCommandReturnsActionErrors(t *testing.T) {
	want := errors.New("import failed")
	root := newRootCommand(rootActions{
		importData: func(context.Context, []string, io.Reader, io.Writer, io.Writer, string) error { return want },
	})
	root.SetArgs([]string{"import", "list"})
	if err := root.ExecuteContext(t.Context()); !errors.Is(err, want) {
		t.Fatalf("ExecuteContext() error = %v, want %v", err, want)
	}
}
