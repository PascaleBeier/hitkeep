package hitkeepcmd

import (
	"io"
	"reflect"
	"testing"
)

func TestRootCommandPreservesFirstArgumentRouting(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCall string
		wantArgs []string
	}{
		{name: "no arguments", args: []string{}, wantCall: "run"},
		{name: "server flag", args: []string{"--http-addr=:9090"}, wantCall: "run", wantArgs: []string{"--http-addr=:9090"}},
		{name: "unknown command", args: []string{"unknown"}, wantCall: "run", wantArgs: []string{"unknown"}},
		{name: "help remains server input", args: []string{"help"}, wantCall: "run", wantArgs: []string{"help"}},
		{name: "help flag remains server input", args: []string{"--help"}, wantCall: "run", wantArgs: []string{"--help"}},
		{name: "version flag remains server input", args: []string{"--version"}, wantCall: "run", wantArgs: []string{"--version"}},
		{name: "command after flag is server input", args: []string{"--http-addr=:9090", "update-spam-lists"}, wantCall: "run", wantArgs: []string{"--http-addr=:9090", "update-spam-lists"}},
		{name: "recover", args: []string{"recover", "disable-2fa", "-email", "user@example.com"}, wantCall: "recover", wantArgs: []string{"disable-2fa", "-email", "user@example.com"}},
		{name: "spam update", args: []string{"update-spam-lists", "-output", "spam.json"}, wantCall: "spam", wantArgs: []string{"-output", "spam.json"}},
		{name: "AI update", args: []string{"update-ai-agent-lists", "-output", "agents.json"}, wantCall: "ai", wantArgs: []string{"-output", "agents.json"}},
		{name: "import", args: []string{"import", "status", "--import-id", "123"}, wantCall: "import", wantArgs: []string{"status", "--import-id", "123"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var called string
			var gotArgs []string
			record := func(name string) func([]string) {
				return func(args []string) {
					called = name
					gotArgs = append([]string(nil), args...)
				}
			}
			root := newRootCommand(rootActions{
				run: func(args []string) {
					called = "run"
					gotArgs = append([]string(nil), args...)
				},
				recover:            record("recover"),
				updateSpamLists:    record("spam"),
				updateAIAgentLists: record("ai"),
				importData:         record("import"),
			})
			root.SetArgs(tt.args)
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			if called != tt.wantCall {
				t.Fatalf("called %q, want %q", called, tt.wantCall)
			}
			if !reflect.DeepEqual(gotArgs, tt.wantArgs) {
				t.Fatalf("args = %v, want %v", gotArgs, tt.wantArgs)
			}
		})
	}
}
