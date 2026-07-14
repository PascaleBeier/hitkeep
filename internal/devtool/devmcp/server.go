package devmcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"hitkeep/internal/devtool"
)

const maxRequestedLogLines = 200

type emptyInput struct{}
type qaPlanInput struct {
	Profile string `json:"profile,omitempty" jsonschema:"Profile to plan: changed, pr, or full"`
	BaseRef string `json:"base_ref,omitempty" jsonschema:"Git base ref used only for the changed profile"`
}
type runInput struct {
	RunID string `json:"run_id" jsonschema:"Validated hk run identifier"`
}
type logsInput struct {
	RunID  string `json:"run_id" jsonschema:"Validated hk run identifier"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum lines to return, from 1 through 200"`
	Cursor int    `json:"cursor,omitempty" jsonschema:"Previous next_cursor for incremental reads"`
	GateID string `json:"gate_id,omitempty" jsonschema:"Optional canonical gate identifier"`
}
type runListInput struct {
	Limit int `json:"limit,omitempty" jsonschema:"Maximum recent runs to return, from 1 through 100"`
}
type variantInput struct {
	Variant string `json:"variant,omitempty" jsonschema:"Build variant: self-hosted or cloud"`
	Runtime string `json:"runtime,omitempty" jsonschema:"Development runtime: native or container"`
	Seed    bool   `json:"seed,omitempty" jsonschema:"Seed isolated demo data before starting"`
}
type qaStartInput struct {
	Profile string   `json:"profile,omitempty" jsonschema:"QA profile: changed, pr, or full"`
	GateIDs []string `json:"gate_ids,omitempty" jsonschema:"Optional canonical gate identifiers"`
}
type buildInput struct {
	Variant string `json:"variant,omitempty" jsonschema:"Build variant: self-hosted or cloud"`
	Target  string `json:"target,omitempty" jsonschema:"Build target: binary or image"`
}

type envelopeOutput struct {
	devtool.Envelope
}

func NewServer(app *devtool.App, version string) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "hitkeep-developer", Version: version}, &mcp.ServerOptions{
		Instructions: "Local, worktree-confined HitKeep development operations. Mutable workflow facts come from hk catalogs; no arbitrary command execution is available.",
		Capabilities: &mcp.ServerCapabilities{},
	})
	registerTools(server, app)
	registerResources(server, app)
	return server
}

func registerTools(server *mcp.Server, app *devtool.App) {
	readOnly := annotations(true, true, false, false)
	action := annotations(false, false, false, false)
	destructiveAction := annotations(false, true, false, true)

	mcp.AddTool(server, tool("hk_workspace_status", "Inspect the selected Git worktree, allocated ports, URLs, and change count.", readOnly), func(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, envelopeOutput, error) {
		value, err := app.Workspace(ctx)
		return output(app, "workspace status", value, err)
	})
	mcp.AddTool(server, tool("hk_workspace_list", "List HitKeep worktrees with their isolated workspace identifiers, ports, and state.", readOnly), func(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, envelopeOutput, error) {
		value, err := app.Workspaces(ctx)
		return output(app, "workspace list", value, err)
	})
	mcp.AddTool(server, tool("hk_workspace_handoff", "Return compact, secret-free handoff context for the selected worktree.", readOnly), func(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, envelopeOutput, error) {
		value, err := app.Handoff(ctx)
		return output(app, "workspace handoff", value, err)
	})
	mcp.AddTool(server, tool("hk_doctor", "Check the local toolchain and container runtime prerequisites.", readOnly), func(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, envelopeOutput, error) {
		return output(app, "doctor", app.Doctor(ctx), nil)
	})
	mcp.AddTool(server, tool("hk_qa_plan", "Select canonical QA gates without running them.", readOnly), func(ctx context.Context, _ *mcp.CallToolRequest, input qaPlanInput) (*mcp.CallToolResult, envelopeOutput, error) {
		if input.Profile == "" {
			input.Profile = "changed"
		}
		value, err := app.QAPlan(ctx, input.Profile, input.BaseRef)
		return output(app, "qa plan", value, err)
	})
	mcp.AddTool(server, tool("hk_run_status", "Inspect one asynchronous hk run.", readOnly), func(_ context.Context, _ *mcp.CallToolRequest, input runInput) (*mcp.CallToolResult, envelopeOutput, error) {
		value, err := app.GetRun(input.RunID)
		return output(app, "run status", value, err)
	})
	mcp.AddTool(server, tool("hk_run_list", "List bounded recent runs for the selected worktree.", readOnly), func(_ context.Context, _ *mcp.CallToolRequest, input runListInput) (*mcp.CallToolResult, envelopeOutput, error) {
		if input.Limit == 0 {
			input.Limit = 20
		}
		if input.Limit < 1 || input.Limit > 100 {
			return output(app, "run list", nil, errors.New("limit must be between 1 and 100"))
		}
		value, err := app.RecentRuns(input.Limit)
		return output(app, "run list", value, err)
	})
	mcp.AddTool(server, tool("hk_logs_tail", "Read a bounded, redacted tail of one hk run log.", readOnly), func(_ context.Context, _ *mcp.CallToolRequest, input logsInput) (*mcp.CallToolResult, envelopeOutput, error) {
		if input.Limit < 0 || input.Limit > maxRequestedLogLines || input.Cursor < 0 {
			return output(app, "logs tail", nil, errors.New("limit must be between 1 and 200"))
		}
		var value devtool.LogTail
		var err error
		if input.GateID != "" {
			value, err = app.TailGateLogAfter(input.RunID, input.GateID, input.Limit, input.Cursor)
		} else {
			value, err = app.TailLogAfter(input.RunID, input.Limit, input.Cursor)
		}
		return output(app, "logs tail", value, err)
	})

	mcp.AddTool(server, tool("hk_setup_start", "Start reproducible dependency setup and return a run ID immediately.", action), startHandler(app, "setup start", func(emptyInput) devtool.RunRequest {
		return devtool.RunRequest{Kind: "setup"}
	}))
	mcp.AddTool(server, tool("hk_dev_start", "Start the isolated development stack and return a run ID immediately.", action), startHandler(app, "dev start", func(input variantInput) devtool.RunRequest {
		return devtool.RunRequest{Kind: "dev-start", Variant: input.Variant, Runtime: input.Runtime, Seed: input.Seed}
	}))
	mcp.AddTool(server, tool("hk_dev_stop", "Stop only the selected worktree's development stack.", destructiveAction), startHandler(app, "dev stop", func(input variantInput) devtool.RunRequest {
		return devtool.RunRequest{Kind: "dev-stop", Variant: input.Variant, Runtime: input.Runtime}
	}))
	mcp.AddTool(server, tool("hk_qa_start", "Start canonical QA gates asynchronously and return a run ID.", action), startHandler(app, "qa start", func(input qaStartInput) devtool.RunRequest {
		return devtool.RunRequest{Kind: "qa", Profile: input.Profile, GateIDs: input.GateIDs}
	}))
	mcp.AddTool(server, tool("hk_build_start", "Start a deterministic binary or local image build and return a run ID.", action), startHandler(app, "build start", func(input buildInput) devtool.RunRequest {
		return devtool.RunRequest{Kind: "build", Variant: input.Variant, Target: input.Target}
	}))
	mcp.AddTool(server, tool("hk_smoke_start", "Start a local production-image smoke test and return a run ID.", action), startHandler(app, "smoke start", func(input variantInput) devtool.RunRequest {
		return devtool.RunRequest{Kind: "smoke", Variant: input.Variant}
	}))
	mcp.AddTool(server, tool("hk_run_cancel", "Request cancellation of one active hk run.", destructiveAction), func(_ context.Context, _ *mcp.CallToolRequest, input runInput) (*mcp.CallToolResult, envelopeOutput, error) {
		value, err := app.CancelRun(input.RunID)
		return output(app, "run cancel", value, err)
	})
}

func startHandler[In any](app *devtool.App, command string, request func(In) devtool.RunRequest) func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, envelopeOutput, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input In) (*mcp.CallToolResult, envelopeOutput, error) {
		value, err := app.StartRun(ctx, request(input))
		return output(app, command, value, err)
	}
}

func output(app *devtool.App, command string, data any, err error) (*mcp.CallToolResult, envelopeOutput, error) {
	var envelope devtool.Envelope
	if err != nil {
		envelope = devtool.ErrorEnvelope(command, workspaceID(app), err)
	} else {
		envelope = devtool.SuccessEnvelope(command, workspaceID(app), data)
	}
	return &mcp.CallToolResult{IsError: err != nil}, envelopeOutput{Envelope: envelope}, nil
}

func tool(name, description string, annotation *mcp.ToolAnnotations) *mcp.Tool {
	return &mcp.Tool{Name: name, Description: description, Annotations: annotation, InputSchema: inputSchema(name)}
}

func inputSchema(name string) *jsonschema.Schema {
	properties := map[string]*jsonschema.Schema{}
	required := []string{}
	switch name {
	case "hk_qa_plan":
		properties["profile"] = enum("changed", "pr", "full")
		properties["base_ref"] = &jsonschema.Schema{Type: "string", MaxLength: new(200)}
	case "hk_run_status", "hk_run_cancel":
		properties["run_id"] = runIDSchema()
		required = []string{"run_id"}
	case "hk_logs_tail":
		properties["run_id"] = runIDSchema()
		properties["limit"] = &jsonschema.Schema{Type: "integer", Minimum: new(1.0), Maximum: new(float64(maxRequestedLogLines))}
		properties["cursor"] = &jsonschema.Schema{Type: "integer", Minimum: new(0.0)}
		properties["gate_id"] = gateEnum()
		required = []string{"run_id"}
	case "hk_run_list":
		properties["limit"] = &jsonschema.Schema{Type: "integer", Minimum: new(1.0), Maximum: new(100.0)}
	case "hk_dev_start", "hk_dev_stop", "hk_smoke_start":
		properties["variant"] = enum("self-hosted", "cloud")
		if name == "hk_dev_start" || name == "hk_dev_stop" {
			properties["runtime"] = enum("native", "container")
		}
		if name == "hk_dev_start" {
			properties["seed"] = &jsonschema.Schema{Type: "boolean"}
		}
	case "hk_qa_start":
		properties["profile"] = enum("changed", "pr", "full")
		properties["gate_ids"] = &jsonschema.Schema{Type: "array", Items: gateEnum(), UniqueItems: true, MaxItems: new(len(devtool.CatalogSnapshot().Gates))}
	case "hk_build_start":
		properties["variant"] = enum("self-hosted", "cloud")
		properties["target"] = enum("binary", "image")
	}
	return &jsonschema.Schema{Type: "object", Properties: properties, Required: required, AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}}}
}

func enum(values ...string) *jsonschema.Schema {
	items := make([]any, len(values))
	for index, value := range values {
		items[index] = value
	}
	return &jsonschema.Schema{Type: "string", Enum: items}
}

func gateEnum() *jsonschema.Schema {
	gates := devtool.CatalogSnapshot().Gates
	values := make([]string, len(gates))
	for index, gate := range gates {
		values[index] = gate.ID
	}
	return enum(values...)
}

func runIDSchema() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "string", Pattern: `^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`, MaxLength: new(100)}
}

func annotations(readOnly, idempotent, openWorld, destructive bool) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{ReadOnlyHint: readOnly, IdempotentHint: idempotent, OpenWorldHint: &openWorld, DestructiveHint: &destructive}
}

func workspaceID(app *devtool.App) string {
	return app.WorkspaceID()
}

func registerResources(server *mcp.Server, app *devtool.App) {
	server.AddResource(resource("Current workspace", "hitkeep-dev://workspace/current", "Current secret-free worktree state."), resourceHandler(app))
	server.AddResource(resource("Build variants", "hitkeep-dev://catalog/variants", "Canonical self-hosted and cloud variant catalog."), resourceHandler(app))
	server.AddResource(resource("QA catalog", "hitkeep-dev://catalog/qa", "Canonical QA profiles and gate catalog."), resourceHandler(app))
	server.AddResource(resource("Contributing guide", "hitkeep-dev://docs/contributing", "Repository contributor onboarding guidance."), resourceHandler(app))
	server.AddResourceTemplate(&mcp.ResourceTemplate{Name: "Run summary", Description: "One asynchronous hk run summary.", MIMEType: "application/json", URITemplate: "hitkeep-dev://runs/{run_id}/summary"}, resourceHandler(app))
	server.AddResourceTemplate(&mcp.ResourceTemplate{Name: "Run logs", Description: "Bounded and redacted run log tail.", MIMEType: "application/json", URITemplate: "hitkeep-dev://runs/{run_id}/logs/{gate}"}, resourceHandler(app))
}

func resource(name, uri, description string) *mcp.Resource {
	return &mcp.Resource{Name: name, URI: uri, Description: description, MIMEType: "application/json"}
}

func resourceHandler(app *devtool.App) mcp.ResourceHandler {
	return func(ctx context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		uri := request.Params.URI
		var value any
		var err error
		switch uri {
		case "hitkeep-dev://workspace/current":
			value, err = app.Workspace(ctx)
		case "hitkeep-dev://catalog/variants":
			value = map[string]any{"schema_version": devtool.SchemaVersion, "variants": app.Catalog().Variants}
		case "hitkeep-dev://catalog/qa":
			catalog := app.Catalog()
			value = map[string]any{"schema_version": devtool.SchemaVersion, "profiles": catalog.Profiles, "gates": catalog.Gates}
		case "hitkeep-dev://docs/contributing":
			value, err = contributingResource(app.Root())
		default:
			value, err = runResource(app, uri)
		}
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: uri, MIMEType: "application/json", Text: string(raw)}}}, nil
	}
}

func contributingResource(root string) (map[string]any, error) {
	raw, err := os.ReadFile(filepath.Join(root, "CONTRIBUTING.md"))
	if err != nil {
		return nil, err
	}
	const maxBytes = 64 * 1024
	truncated := len(raw) > maxBytes
	if truncated {
		raw = raw[:maxBytes]
	}
	return map[string]any{"schema_version": devtool.SchemaVersion, "markdown": string(raw), "truncated": truncated}, nil
}

func runResource(app *devtool.App, rawURI string) (any, error) {
	parsed, err := url.Parse(rawURI)
	if err != nil || parsed.Scheme != "hitkeep-dev" || parsed.Host != "runs" {
		return nil, errors.New("unknown developer resource")
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) == 2 && parts[1] == "summary" {
		return app.GetRun(parts[0])
	}
	if len(parts) == 3 && parts[1] == "logs" {
		if parts[2] != "all" {
			return app.TailGateLog(parts[0], parts[2], 80)
		}
		return app.TailLog(parts[0], 80)
	}
	return nil, fmt.Errorf("unknown run resource %q", rawURI)
}

func RunStdio(ctx context.Context, app *devtool.App, version string) error {
	return NewServer(app, version).Run(ctx, &mcp.StdioTransport{})
}

func ParseLimit(value string) (int, error) {
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 1 || limit > maxRequestedLogLines {
		return 0, errors.New("limit must be between 1 and 200")
	}
	return limit, nil
}
