package devtool

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAgentOptimizedCommandUsesSupportedStructuredFlags(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{name: "nested hk", in: []string{"./hk", "docs", "check"}, want: []string{"./hk", "docs", "check", "--output", "json"}},
		{name: "go test", in: []string{"go", "test", "./..."}, want: []string{"go", "test", "-json", "./..."}},
		{name: "go vet", in: []string{"go", "vet", "./..."}, want: []string{"go", "vet", "-json", "./..."}},
		{name: "staticcheck", in: []string{"go", "run", "honnef.co/go/tools/cmd/staticcheck@v1", "./..."}, want: []string{"go", "run", "honnef.co/go/tools/cmd/staticcheck@v1", "-f=json", "./..."}},
		{name: "govulncheck", in: []string{"go", "run", "golang.org/x/vuln/cmd/govulncheck@v1", "./..."}, want: []string{"go", "run", "golang.org/x/vuln/cmd/govulncheck@v1", "-format=json", "./..."}},
		{name: "golangci", in: []string{"golangci-lint", "run"}, want: []string{"golangci-lint", "run", "--output.text.path=", "--output.json.path=stdout", "--show-stats=false", "--color=never"}},
		{name: "golangci via go run", in: []string{"go", "run", "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2", "run"}, want: []string{"go", "run", "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2", "run", "--output.text.path=", "--output.json.path=stdout", "--show-stats=false", "--color=never"}},
		{name: "zizmor", in: []string{"zizmor", ".github"}, want: []string{"zizmor", ".github", "--format=json", "--no-progress", "--color=never"}},
		{name: "buildx", in: []string{"docker", "buildx", "build", "."}, want: []string{"docker", "buildx", "build", ".", "--progress=rawjson"}},
		{name: "compose", in: []string{"docker", "compose", "up", "-d"}, want: []string{"docker", "compose", "--progress=json", "up", "-d"}},
		{name: "unsupported unchanged", in: []string{"npm", "run", "test:ci"}, want: []string{"npm", "run", "test:ci"}},
		{name: "existing output", in: []string{"./hk", "catalog", "--output=json"}, want: []string{"./hk", "catalog", "--output=json"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := agentOptimizedCommand(test.in); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("agent command = %q, want %q", got, test.want)
			}
		})
	}
}

func TestCatalogAdvertisesAgentCommandWhenItDiffers(t *testing.T) {
	catalog := CatalogSnapshot()
	var found bool
	for _, gate := range catalog.Gates {
		if gate.ID == "mcp-audit" {
			found = true
			if !reflect.DeepEqual(gate.AgentCommand, agentOptimizedCommand(gate.Command)) {
				t.Fatalf("agent command = %q for %q", gate.AgentCommand, gate.Command)
			}
		}
	}
	if !found {
		t.Fatal("go test gate not found")
	}
}

func TestRunCommandAppliesAgentArgumentsAndEnvironment(t *testing.T) {
	workspace := t.TempDir()
	if output, err := exec.CommandContext(t.Context(), "git", "init", "--quiet", workspace).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	fakeGo := filepath.Join(t.TempDir(), "go")
	script := "#!/bin/sh\nprintf 'args=%s\\n' \"$*\"\nprintf 'agent=%s no_color=%s term=%s\\n' \"$HK_AGENT_OUTPUT\" \"$NO_COLOR\" \"$TERM\"\n"
	if err := os.WriteFile(fakeGo, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	app, err := NewApp(workspace)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err = app.runCommand(WithAgentOutput(context.Background()), &output, commandSpec{Args: []string{fakeGo, "test", "./..."}})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"test -json ./...", "agent=json no_color=1 term=dumb"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("agent command output missing %q: %s", expected, output.String())
		}
	}
}
