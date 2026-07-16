package devmcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"hitkeep/internal/devtool"
)

const maxRequestedLogLines = 200

type workspaceInput struct {
	Workspace string `json:"workspace,omitempty" jsonschema:"Optional client-root name, workspace ID, or workspace path; required only when more than one HitKeep workspace root is active"`
}
type emptyInput struct{ workspaceInput }
type qaPlanInput struct {
	workspaceInput
	Profile string `json:"profile,omitempty" jsonschema:"Profile to plan: changed, pr, or full"`
	BaseRef string `json:"base_ref,omitempty" jsonschema:"Git base ref used only for the changed profile"`
}
type runInput struct {
	workspaceInput
	RunID string `json:"run_id" jsonschema:"Validated hk run identifier"`
}
type logsInput struct {
	workspaceInput
	RunID  string `json:"run_id" jsonschema:"Validated hk run identifier"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum lines to return, from 1 through 200"`
	Cursor int    `json:"cursor,omitempty" jsonschema:"Previous next_cursor for incremental reads"`
	GateID string `json:"gate_id,omitempty" jsonschema:"Optional canonical gate identifier"`
}
type runListInput struct {
	workspaceInput
	Limit int `json:"limit,omitempty" jsonschema:"Maximum recent runs to return, from 1 through 100"`
}
type variantInput struct {
	workspaceInput
	Variant string `json:"variant,omitempty" jsonschema:"Build variant: self-hosted or cloud"`
	Runtime string `json:"runtime,omitempty" jsonschema:"Development runtime: native or container"`
	Seed    bool   `json:"seed,omitempty" jsonschema:"Seed isolated demo data before starting"`
}
type qaStartInput struct {
	workspaceInput
	Profile string   `json:"profile,omitempty" jsonschema:"QA profile: changed, pr, or full"`
	GateIDs []string `json:"gate_ids,omitempty" jsonschema:"Optional canonical gate identifiers"`
}
type buildInput struct {
	workspaceInput
	Variant string `json:"variant,omitempty" jsonschema:"Build variant: self-hosted or cloud"`
	Target  string `json:"target,omitempty" jsonschema:"Build target: binary or image"`
}

type envelopeOutput struct {
	devtool.Envelope
}

type workspaceScoped interface {
	workspaceSelector() string
}

func (input workspaceInput) workspaceSelector() string { return strings.TrimSpace(input.Workspace) }

type appResolver interface {
	Resolve(context.Context, *mcp.ServerSession, string) (*devtool.App, error)
}

type toolDelegatingResolver interface {
	DelegateTool(context.Context, *mcp.CallToolRequest, string, workspaceScoped) (*mcp.CallToolResult, envelopeOutput, error)
}

type resourceDelegatingResolver interface {
	DelegateResource(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error)
}

type staticAppResolver struct{ app *devtool.App }

func (resolver staticAppResolver) Resolve(_ context.Context, _ *mcp.ServerSession, selector string) (*devtool.App, error) {
	if selector == "" || selector == resolver.app.WorkspaceID() || filepath.Clean(selector) == filepath.Clean(resolver.app.Root()) {
		return resolver.app, nil
	}
	return nil, fmt.Errorf("workspace %q is outside the configured worktree", selector)
}

type rootedApp struct {
	app  *devtool.App
	name string
}

type centralAppResolver struct{ fallback string }

func (resolver centralAppResolver) Resolve(ctx context.Context, session *mcp.ServerSession, selector string) (*devtool.App, error) {
	apps, rootsErr := appsFromClientRoots(ctx, session)
	if selector != "" {
		for _, candidate := range apps {
			if selector == candidate.name || selector == candidate.app.WorkspaceID() || filepath.Clean(selector) == filepath.Clean(candidate.app.Root()) {
				return candidate.app, nil
			}
		}
		if rootsErr != nil {
			return nil, fmt.Errorf("read MCP client roots: %w", rootsErr)
		}
		return nil, fmt.Errorf("workspace %q is not one of the client's HitKeep roots", selector)
	}
	if len(apps) == 1 {
		return apps[0].app, nil
	}
	if len(apps) > 1 {
		identifiers := make([]string, 0, len(apps))
		for _, candidate := range apps {
			identifiers = append(identifiers, candidate.app.WorkspaceID())
		}
		return nil, fmt.Errorf("multiple HitKeep workspaces are active; pass workspace as one of: %s", strings.Join(identifiers, ", "))
	}
	if resolver.fallback != "" {
		if app, err := newHitKeepApp(resolver.fallback); err == nil {
			return app, nil
		}
	}
	if rootsErr != nil {
		return nil, fmt.Errorf("read MCP client roots: %w", rootsErr)
	}
	return nil, errors.New("no HitKeep workspace root is active; open a HitKeep workspace or pass workspace explicitly")
}

func (resolver centralAppResolver) DelegateTool(ctx context.Context, request *mcp.CallToolRequest, command string, input workspaceScoped) (*mcp.CallToolResult, envelopeOutput, error) {
	app, err := resolver.Resolve(ctx, request.Session, input.workspaceSelector())
	if err != nil {
		return output(nil, command, nil, err)
	}
	session, err := connectWorkspaceMCP(ctx, app)
	if err != nil {
		return output(app, command, nil, err)
	}
	defer session.Close()

	raw, err := json.Marshal(input)
	if err != nil {
		return output(app, command, nil, fmt.Errorf("encode delegated tool input: %w", err))
	}
	arguments := map[string]any{}
	if err := json.Unmarshal(raw, &arguments); err != nil {
		return output(app, command, nil, fmt.Errorf("decode delegated tool input: %w", err))
	}
	delete(arguments, "workspace")
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: request.Params.Name, Arguments: arguments})
	if err != nil {
		return output(app, command, nil, fmt.Errorf("workspace MCP tool %s: %w", request.Params.Name, err))
	}
	envelope, err := delegatedEnvelope(result.StructuredContent)
	if err != nil {
		return output(app, command, nil, err)
	}
	return result, envelopeOutput{Envelope: envelope}, nil
}

func (resolver centralAppResolver) DelegateResource(ctx context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	app, err := resolver.Resolve(ctx, request.Session, "")
	if err != nil {
		return nil, err
	}
	session, err := connectWorkspaceMCP(ctx, app)
	if err != nil {
		return nil, err
	}
	defer session.Close()
	return session.ReadResource(ctx, &mcp.ReadResourceParams{URI: request.Params.URI})
}

func connectWorkspaceMCP(ctx context.Context, app *devtool.App) (*mcp.ClientSession, error) {
	launcher := filepath.Join(app.Root(), "hk")
	info, err := os.Lstat(launcher)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace hk launcher: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return nil, fmt.Errorf("workspace hk launcher must be a regular executable file: %s", launcher)
	}
	command := exec.CommandContext(ctx, launcher, "--workspace", app.Root(), "mcp", "serve") //nolint:gosec // launcher is confined to the selected HitKeep root
	client := mcp.NewClient(&mcp.Implementation{Name: "hitkeep-developer-broker", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		return nil, fmt.Errorf("connect workspace MCP: %w", err)
	}
	return session, nil
}

func delegatedEnvelope(value any) (devtool.Envelope, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return devtool.Envelope{}, fmt.Errorf("encode workspace MCP result: %w", err)
	}
	var envelope devtool.Envelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return devtool.Envelope{}, fmt.Errorf("decode workspace MCP result: %w", err)
	}
	if envelope.SchemaVersion == "" || envelope.Command == "" || envelope.Status == "" {
		return devtool.Envelope{}, errors.New("workspace MCP returned an invalid structured envelope")
	}
	return envelope, nil
}

func appsFromClientRoots(ctx context.Context, session *mcp.ServerSession) ([]rootedApp, error) {
	if session == nil {
		return nil, errors.New("MCP session is unavailable")
	}
	result, err := session.ListRoots(ctx, nil)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	apps := make([]rootedApp, 0, len(result.Roots))
	for _, root := range result.Roots {
		path, pathErr := clientRootPath(root.URI)
		if pathErr != nil {
			continue
		}
		app, appErr := newHitKeepApp(path)
		if appErr != nil || seen[app.WorkspaceID()] {
			continue
		}
		seen[app.WorkspaceID()] = true
		apps = append(apps, rootedApp{app: app, name: strings.TrimSpace(root.Name)})
	}
	return apps, nil
}

func newHitKeepApp(path string) (*devtool.App, error) {
	app, err := devtool.NewApp(path)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(app.Root(), "go.mod"))
	if err != nil {
		return nil, errors.New("workspace has no HitKeep module")
	}
	for line := range strings.Lines(string(raw)) {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) == 2 && fields[0] == "module" && fields[1] == "hitkeep" {
			return app, nil
		}
		if fields[0] == "module" {
			break
		}
	}
	return nil, errors.New("workspace is not the HitKeep module")
}

func clientRootPath(rawURI string) (string, error) {
	parsed, err := url.Parse(rawURI)
	if err != nil || parsed.Scheme != "file" || parsed.Path == "" || parsed.Host != "" && parsed.Host != "localhost" {
		return "", errors.New("MCP root is not a local file URI")
	}
	return filepath.FromSlash(parsed.Path), nil
}

func NewServer(app *devtool.App, version string) *mcp.Server {
	return newServer(staticAppResolver{app: app}, version)
}

// NewCentralServer returns one root-aware MCP server that delegates every
// operation to the worktree selected by the connected client's MCP roots.
func NewCentralServer(fallbackWorkspace, version string) *mcp.Server {
	return newServer(centralAppResolver{fallback: fallbackWorkspace}, version)
}

func newServer(resolver appResolver, version string) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "hitkeep-developer", Version: version}, &mcp.ServerOptions{
		Instructions: "Local, root-routed and worktree-confined HitKeep development operations. Pass workspace only when the client exposes multiple HitKeep roots. Mutable workflow facts come from hk catalogs; no arbitrary command execution is available.",
		Capabilities: &mcp.ServerCapabilities{},
	})
	registerTools(server, resolver)
	registerResources(server, resolver)
	return server
}

func registerTools(server *mcp.Server, resolver appResolver) {
	readOnly := annotations(true, true, false, false)
	action := annotations(false, false, false, false)
	destructiveAction := annotations(false, true, false, true)

	mcp.AddTool(server, tool("hk_workspace_status", "Inspect the selected Git worktree, allocated ports, URLs, and change count.", readOnly), routedHandler(resolver, "workspace status", func(ctx context.Context, app *devtool.App, _ emptyInput) (any, error) {
		return app.Workspace(ctx)
	}))
	mcp.AddTool(server, tool("hk_workspace_list", "List HitKeep worktrees with their isolated workspace identifiers, ports, and state.", readOnly), routedHandler(resolver, "workspace list", func(ctx context.Context, app *devtool.App, _ emptyInput) (any, error) {
		return app.Workspaces(ctx)
	}))
	mcp.AddTool(server, tool("hk_workspace_handoff", "Return compact, secret-free handoff context for the selected worktree.", readOnly), routedHandler(resolver, "workspace handoff", func(ctx context.Context, app *devtool.App, _ emptyInput) (any, error) {
		return app.Handoff(ctx)
	}))
	mcp.AddTool(server, tool("hk_doctor", "Check the local toolchain and container runtime prerequisites.", readOnly), routedHandler(resolver, "doctor", func(ctx context.Context, app *devtool.App, _ emptyInput) (any, error) {
		return app.Doctor(ctx), nil
	}))
	mcp.AddTool(server, tool("hk_qa_plan", "Select canonical QA gates without running them.", readOnly), routedHandler(resolver, "qa plan", func(ctx context.Context, app *devtool.App, input qaPlanInput) (any, error) {
		if input.Profile == "" {
			input.Profile = "changed"
		}
		return app.QAPlan(ctx, input.Profile, input.BaseRef)
	}))
	mcp.AddTool(server, tool("hk_run_status", "Inspect one asynchronous hk run.", readOnly), routedHandler(resolver, "run status", func(_ context.Context, app *devtool.App, input runInput) (any, error) {
		return app.GetRun(input.RunID)
	}))
	mcp.AddTool(server, tool("hk_run_list", "List bounded recent runs for the selected worktree.", readOnly), routedHandler(resolver, "run list", func(_ context.Context, app *devtool.App, input runListInput) (any, error) {
		if input.Limit == 0 {
			input.Limit = 20
		}
		if input.Limit < 1 || input.Limit > 100 {
			return nil, errors.New("limit must be between 1 and 100")
		}
		return app.RecentRuns(input.Limit)
	}))
	mcp.AddTool(server, tool("hk_logs_tail", "Read a bounded, redacted tail of one hk run log.", readOnly), routedHandler(resolver, "logs tail", func(_ context.Context, app *devtool.App, input logsInput) (any, error) {
		if input.Limit < 0 || input.Limit > maxRequestedLogLines || input.Cursor < 0 {
			return nil, errors.New("limit must be between 1 and 200")
		}
		if input.GateID != "" {
			return app.TailGateLogAfter(input.RunID, input.GateID, input.Limit, input.Cursor)
		}
		return app.TailLogAfter(input.RunID, input.Limit, input.Cursor)
	}))

	mcp.AddTool(server, tool("hk_setup_start", "Start reproducible dependency setup and return a run ID immediately.", action), startHandler(resolver, "setup start", func(emptyInput) devtool.RunRequest {
		return devtool.RunRequest{Kind: "setup"}
	}))
	mcp.AddTool(server, tool("hk_dev_start", "Start the isolated development stack and return a run ID immediately.", action), startHandler(resolver, "dev start", func(input variantInput) devtool.RunRequest {
		return devtool.RunRequest{Kind: "dev-start", Variant: input.Variant, Runtime: input.Runtime, Seed: input.Seed}
	}))
	mcp.AddTool(server, tool("hk_dev_stop", "Stop only the selected worktree's development stack.", destructiveAction), startHandler(resolver, "dev stop", func(input variantInput) devtool.RunRequest {
		return devtool.RunRequest{Kind: "dev-stop", Variant: input.Variant, Runtime: input.Runtime}
	}))
	mcp.AddTool(server, tool("hk_qa_start", "Start canonical QA gates asynchronously and return a run ID.", action), startHandler(resolver, "qa start", func(input qaStartInput) devtool.RunRequest {
		return devtool.RunRequest{Kind: "qa", Profile: input.Profile, GateIDs: input.GateIDs}
	}))
	mcp.AddTool(server, tool("hk_build_start", "Start a deterministic binary or local image build and return a run ID.", action), startHandler(resolver, "build start", func(input buildInput) devtool.RunRequest {
		return devtool.RunRequest{Kind: "build", Variant: input.Variant, Target: input.Target}
	}))
	mcp.AddTool(server, tool("hk_smoke_start", "Start a local production-image smoke test and return a run ID.", action), startHandler(resolver, "smoke start", func(input variantInput) devtool.RunRequest {
		return devtool.RunRequest{Kind: "smoke", Variant: input.Variant}
	}))
	mcp.AddTool(server, tool("hk_run_cancel", "Request cancellation of one active hk run.", destructiveAction), routedHandler(resolver, "run cancel", func(_ context.Context, app *devtool.App, input runInput) (any, error) {
		return app.CancelRun(input.RunID)
	}))
}

func routedHandler[In workspaceScoped](resolver appResolver, command string, handler func(context.Context, *devtool.App, In) (any, error)) func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, envelopeOutput, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, envelopeOutput, error) {
		if delegate, ok := resolver.(toolDelegatingResolver); ok {
			return delegate.DelegateTool(ctx, request, command, input)
		}
		app, err := resolver.Resolve(ctx, request.Session, input.workspaceSelector())
		if err != nil {
			return output(nil, command, nil, err)
		}
		value, err := handler(ctx, app, input)
		return output(app, command, value, err)
	}
}

func startHandler[In workspaceScoped](resolver appResolver, command string, request func(In) devtool.RunRequest) func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, envelopeOutput, error) {
	return routedHandler(resolver, command, func(ctx context.Context, app *devtool.App, input In) (any, error) {
		return app.StartRun(ctx, request(input))
	})
}

func output(app *devtool.App, command string, data any, err error) (*mcp.CallToolResult, envelopeOutput, error) {
	workspaceID := ""
	if app != nil {
		workspaceID = app.WorkspaceID()
	}
	var envelope devtool.Envelope
	if err != nil {
		envelope = devtool.ErrorEnvelope(command, workspaceID, err)
	} else {
		envelope = devtool.SuccessEnvelope(command, workspaceID, data)
	}
	return &mcp.CallToolResult{IsError: err != nil}, envelopeOutput{Envelope: envelope}, nil
}

func tool(name, description string, annotation *mcp.ToolAnnotations) *mcp.Tool {
	return &mcp.Tool{Name: name, Description: description, Annotations: annotation, InputSchema: inputSchema(name)}
}

func inputSchema(name string) *jsonschema.Schema {
	properties := map[string]*jsonschema.Schema{
		"workspace": {Type: "string", MaxLength: new(4096), Description: "Optional client-root name, workspace ID, or workspace path; required only for ambiguous multi-root sessions"},
	}
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

func registerResources(server *mcp.Server, resolver appResolver) {
	server.AddResource(resource("Current workspace", "hitkeep-dev://workspace/current", "Current secret-free worktree state."), resourceHandler(resolver))
	server.AddResource(resource("Build variants", "hitkeep-dev://catalog/variants", "Canonical self-hosted and cloud variant catalog."), resourceHandler(resolver))
	server.AddResource(resource("QA catalog", "hitkeep-dev://catalog/qa", "Canonical QA profiles and gate catalog."), resourceHandler(resolver))
	server.AddResource(resource("Contributing guide", "hitkeep-dev://docs/contributing", "Repository contributor onboarding guidance."), resourceHandler(resolver))
	server.AddResourceTemplate(&mcp.ResourceTemplate{Name: "Run summary", Description: "One asynchronous hk run summary.", MIMEType: "application/json", URITemplate: "hitkeep-dev://runs/{run_id}/summary"}, resourceHandler(resolver))
	server.AddResourceTemplate(&mcp.ResourceTemplate{Name: "Run logs", Description: "Bounded and redacted run log tail.", MIMEType: "application/json", URITemplate: "hitkeep-dev://runs/{run_id}/logs/{gate}"}, resourceHandler(resolver))
}

func resource(name, uri, description string) *mcp.Resource {
	return &mcp.Resource{Name: name, URI: uri, Description: description, MIMEType: "application/json"}
}

func resourceHandler(resolver appResolver) mcp.ResourceHandler {
	return func(ctx context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		if delegate, ok := resolver.(resourceDelegatingResolver); ok {
			return delegate.DelegateResource(ctx, request)
		}
		app, err := resolver.Resolve(ctx, request.Session, "")
		if err != nil {
			return nil, err
		}
		uri := request.Params.URI
		var value any
		var readErr error
		switch uri {
		case "hitkeep-dev://workspace/current":
			value, readErr = app.Workspace(ctx)
		case "hitkeep-dev://catalog/variants":
			value = map[string]any{"schema_version": devtool.SchemaVersion, "variants": app.Catalog().Variants}
		case "hitkeep-dev://catalog/qa":
			catalog := app.Catalog()
			value = map[string]any{"schema_version": devtool.SchemaVersion, "profiles": catalog.Profiles, "gates": catalog.Gates}
		case "hitkeep-dev://docs/contributing":
			value, readErr = contributingResource(app.Root())
		default:
			value, readErr = runResource(app, uri)
		}
		if readErr != nil {
			return nil, readErr
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

func RunCentralStdio(ctx context.Context, fallbackWorkspace, version string) error {
	return NewCentralServer(fallbackWorkspace, version).Run(ctx, &mcp.StdioTransport{})
}

func ParseLimit(value string) (int, error) {
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 1 || limit > maxRequestedLogLines {
		return 0, errors.New("limit must be between 1 and 200")
	}
	return limit, nil
}
