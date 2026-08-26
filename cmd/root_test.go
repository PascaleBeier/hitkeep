package hitkeepcmd

import (
	"context"
	"io"
	"reflect"
	"testing"
)

func TestRootCommandPreservesFirstArgumentRouting(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCall   string
		wantArgs   []string
		wantConfig string
		wantErr    bool
	}{
		{name: "no arguments", args: []string{}, wantCall: "run"},
		{name: "server flag", args: []string{"--http-addr=:9090"}, wantCall: "run", wantArgs: []string{"--http-addr=:9090"}},
		{name: "unknown command", args: []string{"unknown"}, wantCall: "run", wantArgs: []string{"unknown"}},
		{name: "help remains server input", args: []string{"help"}, wantCall: "run", wantArgs: []string{"help"}},
		{name: "help flag remains server input", args: []string{"--help"}, wantCall: "run", wantArgs: []string{"--help"}},
		{name: "version exits before starting server", args: []string{"--version"}},
		{name: "version with argument remains server input", args: []string{"--version", "serve"}, wantCall: "run", wantArgs: []string{"--version", "serve"}},
		{name: "interspersed version remains server input", args: []string{"serve", "--version"}, wantCall: "run", wantArgs: []string{"serve", "--version"}},
		{name: "command after flag is server input", args: []string{"--http-addr=:9090", "update-spam-lists"}, wantCall: "run", wantArgs: []string{"--http-addr=:9090", "update-spam-lists"}},
		{name: "explicit config", args: []string{"--config", "/etc/hitkeep.yaml", "--healthcheck"}, wantCall: "run", wantArgs: []string{"--healthcheck"}, wantConfig: "/etc/hitkeep.yaml"},
		{name: "explicit config equals", args: []string{"--config=/etc/hitkeep.yaml", "recover"}, wantCall: "run", wantArgs: []string{"recover"}, wantConfig: "/etc/hitkeep.yaml"},
		{name: "config after another flag remains server input", args: []string{"--healthcheck", "--config", "/etc/hitkeep.yaml"}, wantCall: "run", wantArgs: []string{"--healthcheck", "--config", "/etc/hitkeep.yaml"}},
		{name: "missing config path", args: []string{"--config"}, wantErr: true},
		{name: "empty config path", args: []string{"--config="}, wantErr: true},
		{name: "recover", args: []string{"recover", "disable-2fa", "-email", "user@example.com"}, wantCall: "recover", wantArgs: []string{"disable-2fa", "-email", "user@example.com"}},
		{name: "spam update", args: []string{"update-spam-lists", "-output", "spam.json"}, wantCall: "spam", wantArgs: []string{"-output", "spam.json"}},
		{name: "AI update", args: []string{"update-ai-agent-lists", "-output", "agents.json"}, wantCall: "ai", wantArgs: []string{"-output", "agents.json"}},
		{name: "import", args: []string{"import", "status", "--import-id", "123"}, wantCall: "import", wantArgs: []string{"status", "--import-id", "123"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var called string
			var gotArgs []string
			var gotConfig string
			record := func(name string) func([]string) {
				return func(args []string) {
					called = name
					gotArgs = append([]string(nil), args...)
				}
			}
			recordRecover := func(_ context.Context, args []string, _ io.Reader, _, _ io.Writer) error {
				called = "recover"
				gotArgs = append([]string(nil), args...)
				return nil
			}
			root := newRootCommand(rootActions{
				run: func(args []string, configFile string) error {
					called = "run"
					gotArgs = append([]string(nil), args...)
					gotConfig = configFile
					return nil
				},
				recover:            recordRecover,
				updateSpamLists:    record("spam"),
				updateAIAgentLists: record("ai"),
				importData:         record("import"),
			})
			root.SetArgs(tt.args)
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			err := root.Execute()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if called != tt.wantCall {
				t.Fatalf("called %q, want %q", called, tt.wantCall)
			}
			if !reflect.DeepEqual(gotArgs, tt.wantArgs) {
				t.Fatalf("args = %v, want %v", gotArgs, tt.wantArgs)
			}
			if gotConfig != tt.wantConfig {
				t.Fatalf("config = %q, want %q", gotConfig, tt.wantConfig)
			}
		})
	}
}
