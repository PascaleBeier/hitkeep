package hitkeepcmd

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportCommandUsesRootConfigurationAndStreams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/sites/site/imports" {
			t.Errorf("request path = %q", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"imports":[]}`)
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "hitkeep.yaml")
	if err := os.WriteFile(path, []byte("api-url: "+server.URL+"\napi-token: file-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	root := NewRootCommand(nil)
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	if err := ExecuteRoot(t.Context(), root, []string{"--config", path, "import", "list", "--site", "site"}); err != nil {
		t.Fatalf("ExecuteRoot() error = %v", err)
	}
	if got := stdout.String(); got != "" {
		t.Errorf("stdout = %q, want empty", got)
	}
	if got := stderr.String(); got != "" {
		t.Errorf("stderr = %q, want empty", got)
	}
}

func TestImportCommandUsesTypedConfigurationAndFlagOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hitkeep.yaml")
	if err := os.WriteFile(path, []byte("public-url: http://public-file.example\napi-url: http://api-file.example\napi-token: file-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HITKEEP_API_URL", "http://api-env.example")
	t.Setenv("HITKEEP_API_TOKEN", "env-token")
	command, err := newImportExecutor(t.Context(), nil, io.Discard, io.Discard, path, nil)
	if err != nil {
		t.Fatal(err)
	}
	options, err := command.resolveOptions(importCLIOptions{siteID: "site", apiURL: "http://flag.example", token: "flag-token"})
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
			t.Setenv("HITKEEP_API_TOKEN", "")
			var stdout, stderr bytes.Buffer
			command := newImportCommandRoute(nil)
			command.SetOut(&stdout)
			command.SetErr(&stderr)
			command.SetArgs(tt.args)
			err := command.ExecuteContext(t.Context())
			if tt.want == 0 {
				if err != nil {
					t.Fatalf("ExecuteContext() error = %v", err)
				}
				if got := stdout.String(); !strings.Contains(got, "Usage:\n  import list [flags]\n") {
					t.Errorf("help stdout = %q, want Cobra usage", got)
				}
				if got := stderr.String(); got != "" {
					t.Errorf("help stderr = %q, want empty", got)
				}
				return
			}
			if exitErr, ok := errors.AsType[*ExitError](err); !ok || exitErr.Code != tt.want {
				t.Fatalf("ExecuteContext() error = %v, want ExitError code %d", err, tt.want)
			}
			if got := stdout.String(); got != "" {
				t.Errorf("stdout = %q, want empty", got)
			}
			if got := stderr.String(); got != "--token or HITKEEP_API_TOKEN is required\n" {
				t.Errorf("validation stderr = %q", got)
			}
		})
	}
}
