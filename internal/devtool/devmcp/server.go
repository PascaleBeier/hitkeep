package devmcp

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"hitkeep/internal/devtool"
	json "hitkeep/internal/jsonapi"
)

const (
	maxRequestedLogLines = 200
	mcpListCacheTTL      = 24 * time.Hour
	centralMCPConnectTTL = 30 * time.Second
)

type workspaceInput struct {
	Workspace string `json:"workspace,omitempty" jsonschema:"Optional, defaults to configured fallback; accepts a catalogued workspace ID or absolute path."`
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
type devStartInput struct {
	workspaceInput
	Variant string `json:"variant,omitempty" jsonschema:"Build variant: self-hosted or cloud"`
	Seed    bool   `json:"seed,omitempty" jsonschema:"Seed isolated demo data before starting"`
}
type screenshotInput struct {
	workspaceInput
	Routes    []string `json:"routes,omitempty" jsonschema:"Local dashboard routes captured in one browser session"`
	Viewport  string   `json:"viewport,omitempty" jsonschema:"Viewport preset: desktop or mobile"`
	Theme     string   `json:"theme,omitempty" jsonschema:"Color scheme: light or dark"`
	Scale     int      `json:"scale,omitempty" jsonschema:"Device pixel ratio: 1 or 2"`
	WaitMS    int      `json:"wait_ms,omitempty" jsonschema:"Bounded visual settle time after route readiness"`
	FullPage  bool     `json:"full_page,omitempty" jsonschema:"Capture the complete document instead of the viewport"`
	Selector  string   `json:"selector,omitempty" jsonschema:"Optional visible CSS selector for a single-route component capture"`
	Anonymous bool     `json:"anonymous,omitempty" jsonschema:"Capture without seeded development authentication"`
}

type envelopeOutput struct {
	devtool.Envelope
}

type workspaceScoped interface {
	workspaceSelector() string
}

func (input workspaceInput) workspaceSelector() string { return strings.TrimSpace(input.Workspace) }

type appResolver interface {
	Resolve(context.Context, string) (*devtool.App, error)
}

type staticAppResolver struct{ app *devtool.App }

func (resolver staticAppResolver) Resolve(_ context.Context, selector string) (*devtool.App, error) {
	if selector == "" || selector == resolver.app.WorkspaceID() || filepath.Clean(selector) == filepath.Clean(resolver.app.Root()) {
		return resolver.app, nil
	}
	return nil, fmt.Errorf("workspace %q is outside the configured worktree", selector)
}

type workspaceMCPConnector func(context.Context, context.Context, *devtool.App, *mcp.ClientOptions) (*mcp.ClientSession, error)

type centralAppResolver struct {
	fallback string
	connect  workspaceMCPConnector // retained for compatibility tests; normal dispatch is in-process
	loadApp  func(string) (*devtool.App, error)
	registry *workspaceRegistry
}

func newCentralAppResolver(fallback string, connect workspaceMCPConnector) *centralAppResolver {
	resolver := &centralAppResolver{fallback: fallback, connect: connect}
	resolver.registry = newWorkspaceRegistry(fallback)
	return resolver
}

func (resolver *centralAppResolver) connector() workspaceMCPConnector {
	if resolver.connect != nil {
		return resolver.connect
	}
	return connectWorkspaceMCP
}

func (resolver *centralAppResolver) Resolve(ctx context.Context, selector string) (*devtool.App, error) {
	app, _, err := resolver.registry.resolve(ctx, selector, resolver.appForPath)
	return app, err
}

func (resolver *centralAppResolver) ResolveFresh(ctx context.Context, selector string) (*devtool.App, error) {
	return resolver.registry.resolveFresh(ctx, selector, resolver.appForPath)
}

type freshAppResolver struct {
	appResolver
}

func (resolver freshAppResolver) Resolve(ctx context.Context, selector string) (*devtool.App, error) {
	if central, ok := resolver.appResolver.(*centralAppResolver); ok {
		return central.ResolveFresh(ctx, selector)
	}
	return resolver.appResolver.Resolve(ctx, selector)
}

func (resolver *centralAppResolver) appForPath(path string) (*devtool.App, error) {
	loader := newHitKeepApp
	if resolver.loadApp != nil {
		loader = resolver.loadApp
	}
	return loader(path)
}

func samePath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(filepath.Clean(left))
	rightAbs, rightErr := filepath.Abs(filepath.Clean(right))
	return leftErr == nil && rightErr == nil && leftAbs == rightAbs
}

func (resolver *centralAppResolver) DelegateTool(ctx context.Context, request *mcp.CallToolRequest, command string, input workspaceScoped) (*mcp.CallToolResult, envelopeOutput, error) {
	app, err := resolver.Resolve(ctx, input.workspaceSelector())
	if err != nil {
		return output(nil, command, nil, err)
	}
	arguments := map[string]any{}
	if request.Params != nil && len(request.Params.Arguments) > 0 {
		if err := json.Unmarshal(request.Params.Arguments, &arguments); err != nil {
			return output(app, command, nil, fmt.Errorf("decode delegated tool input: %w", err))
		}
	}
	delete(arguments, "workspace")
	callParams := &mcp.CallToolParams{Name: request.Params.Name, Arguments: arguments}
	if progressToken := request.Params.GetProgressToken(); progressToken != nil {
		if delegatedToken, ok := delegatedProgressToken(progressToken); ok {
			callParams.SetProgressToken(delegatedToken)
		}
	}
	result, err := resolver.callTool(ctx, app, request, callParams)
	if err != nil {
		return output(app, command, nil, fmt.Errorf("workspace MCP tool %s: %w", request.Params.Name, err))
	}
	envelope, err := delegatedEnvelope(result.StructuredContent, command, app.WorkspaceID(), result.IsError)
	if err != nil {
		return output(app, command, nil, err)
	}
	return result, envelopeOutput{Envelope: envelope}, nil
}

// delegatedProgressToken converts JSON-decoded numeric tokens into a form the
// Go MCP SDK accepts. The broker restores the original outer token when it
// forwards child progress notifications, so using its exact string form for
// the request-scoped child connection preserves the client-facing identity.
func delegatedProgressToken(token any) (any, bool) {
	switch token := token.(type) {
	case int, int32, int64, string:
		return token, true
	case float32:
		return strconv.FormatFloat(float64(token), 'g', -1, 32), true
	case float64:
		return strconv.FormatFloat(token, 'g', -1, 64), true
	case fmt.Stringer:
		raw := json.RawMessage(token.String())
		if !raw.IsValid() || raw.Kind() != '0' {
			return nil, false
		}
		return string(raw), true
	default:
		return nil, false
	}
}

func (resolver *centralAppResolver) DelegateResource(ctx context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	selector, uri, err := workspaceResourceURI(request.Params.URI)
	if err != nil {
		return nil, err
	}
	app, err := resolver.Resolve(ctx, selector)
	if err != nil {
		return nil, err
	}
	return resolver.callResource(ctx, app, uri)
}

func (resolver *centralAppResolver) callTool(ctx context.Context, app *devtool.App, request *mcp.CallToolRequest, params *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	connectContext, cancel := context.WithTimeout(ctx, centralMCPConnectTTL)
	defer cancel()
	session, err := resolver.connector()(connectContext, ctx, app, resolver.clientOptions(request))
	if err != nil {
		return nil, err
	}
	defer session.Close()
	return session.CallTool(ctx, params)
}

func (resolver *centralAppResolver) callResource(ctx context.Context, app *devtool.App, uri string) (*mcp.ReadResourceResult, error) {
	connectContext, cancel := context.WithTimeout(ctx, centralMCPConnectTTL)
	defer cancel()
	session, err := resolver.connector()(connectContext, ctx, app, nil)
	if err != nil {
		return nil, err
	}
	defer session.Close()
	return session.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
}

func (resolver *centralAppResolver) clientOptions(request *mcp.CallToolRequest) *mcp.ClientOptions {
	var outerToken any
	if request != nil && request.Params != nil {
		outerToken = request.Params.GetProgressToken()
	}
	return &mcp.ClientOptions{ProgressNotificationHandler: func(ctx context.Context, notification *mcp.ProgressNotificationClientRequest) {
		if request == nil || request.Session == nil || notification == nil || notification.Params == nil || outerToken == nil {
			return
		}
		params := *notification.Params
		params.ProgressToken = outerToken
		_ = request.Session.NotifyProgress(ctx, &params)
	}}
}

func (resolver *centralAppResolver) Close() {}

func workspaceResourceURI(raw string) (selector, localURI string, err error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "hitkeep-dev" {
		return "", "", errors.New("unknown developer resource")
	}
	if parsed.Host != "workspaces" {
		return "", raw, nil
	}
	// Split the escaped path so an absolute selector such as %2Ftmp%2Fhitkeep
	// remains one workspace segment instead of being mistaken for nested URI
	// components. The URI templates reserve the first segment for the selector.
	parts := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	if len(parts) < 2 || strings.TrimSpace(parts[0]) == "" {
		return "", "", errors.New("workspace resource URI must include a workspace selector")
	}
	selector, err = url.PathUnescape(parts[0])
	if err != nil || strings.TrimSpace(selector) == "" {
		return "", "", errors.New("workspace resource URI has an invalid workspace selector")
	}
	localURI = "hitkeep-dev://" + strings.Join(parts[1:], "/")
	if parsed.RawQuery != "" {
		localURI += "?" + parsed.RawQuery
	}
	return selector, localURI, nil
}

func connectWorkspaceMCP(connectContext, lifetimeContext context.Context, app *devtool.App, options *mcp.ClientOptions) (*mcp.ClientSession, error) {
	launcher := filepath.Join(app.Root(), "hk")
	info, err := os.Lstat(launcher)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace hk launcher: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return nil, fmt.Errorf("workspace hk launcher must be a regular executable file: %s", launcher)
	}
	command := exec.CommandContext(lifetimeContext, launcher, "--workspace", app.Root(), "mcp", "serve") //nolint:gosec // launcher is confined to the selected HitKeep root
	client := mcp.NewClient(&mcp.Implementation{Name: "hitkeep-developer-broker", Version: "1"}, options)
	session, err := client.Connect(connectContext, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		return nil, fmt.Errorf("connect workspace MCP: %w", err)
	}
	return session, nil
}

func delegatedEnvelope(value any, command, workspaceID string, isError bool) (devtool.Envelope, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return devtool.Envelope{}, fmt.Errorf("encode workspace MCP result: %w", err)
	}
	var envelope devtool.Envelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return devtool.Envelope{}, fmt.Errorf("decode workspace MCP result: %w", err)
	}
	if envelope.SchemaVersion != devtool.SchemaVersion {
		return devtool.Envelope{}, fmt.Errorf("workspace MCP returned schema %q, want %q", envelope.SchemaVersion, devtool.SchemaVersion)
	}
	if envelope.Command != command {
		return devtool.Envelope{}, fmt.Errorf("workspace MCP returned command %q, want %q", envelope.Command, command)
	}
	if envelope.WorkspaceID != workspaceID {
		return devtool.Envelope{}, fmt.Errorf("workspace MCP returned workspace %q, want %q", envelope.WorkspaceID, workspaceID)
	}
	wantStatus := "ok"
	if isError {
		wantStatus = "error"
	}
	if envelope.Status != wantStatus || isError && envelope.Error == "" {
		return devtool.Envelope{}, errors.New("workspace MCP returned an inconsistent structured envelope")
	}
	return envelope, nil
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

func NewServer(app *devtool.App, version string) *mcp.Server {
	return newServer(staticAppResolver{app: app}, version)
}

// NewCentralServer returns one stateless broker that delegates each request to
// the configured fallback worktree or an explicitly selected catalogued worktree.
func NewCentralServer(fallbackWorkspace, version string) *mcp.Server {
	return newServer(newCentralAppResolver(fallbackWorkspace, nil), version)
}

func newServer(resolver appResolver, version string) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:        "hitkeep-developer",
		Title:       "HitKeep Developer MCP",
		Description: "Compact, stateless, worktree-confined HitKeep development broker.",
		Version:     version,
	}, &mcp.ServerOptions{
		Instructions: "Resolve each request in-process against the configured fallback or explicit catalogued workspace. Durable development sessions and finite runs remain filesystem-backed. Starts are asynchronous; poll bounded status views for progress. No arbitrary commands or repository mutation are available.",
		Capabilities: &mcp.ServerCapabilities{
			Tools: &mcp.ToolCapabilities{ListChanged: false},
		},
	})
	server.AddReceivingMiddleware(devCacheMiddleware())
	registerTools(server, resolver)
	return server
}

func devCacheMiddleware() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			result, err := next(ctx, method, req)
			if err != nil || result == nil {
				return result, err
			}
			setCache := func(cache *mcp.Cacheable, ttl time.Duration) {
				cache.TTLMs = int(ttl / time.Millisecond)
				cache.CacheScope = "public"
			}
			switch typed := result.(type) {
			case *mcp.DiscoverResult:
				setCache(&typed.Cacheable, mcpListCacheTTL)
			case *mcp.ListToolsResult:
				setCache(&typed.Cacheable, mcpListCacheTTL)
			case *mcp.ListResourcesResult:
				setCache(&typed.Cacheable, mcpListCacheTTL)
			case *mcp.ListResourceTemplatesResult:
				setCache(&typed.Cacheable, mcpListCacheTTL)
			case *mcp.ReadResourceResult:
				setCache(&typed.Cacheable, 0)
			}
			return result, nil
		}
	}
}

func registerTools(server *mcp.Server, resolver appResolver) {
	freshResolver := freshAppResolver{appResolver: resolver}
	readOnly := annotations(true, true, false, false)
	action := annotations(false, false, false, false)
	idempotentAction := annotations(false, true, false, false)
	destructiveAction := annotations(false, true, false, true)

	mcp.AddTool(server, tool("hk_context", "Read one compact workspace, catalog, configuration, handoff, or runtime view.", readOnly), routedHandler(resolver, "context", func(ctx context.Context, app *devtool.App, input contextInput) (any, error) {
		switch input.View {
		case "", "current":
			return app.Workspace(ctx)
		case "workspaces":
			return app.Workspaces(ctx)
		case "catalog", "configuration":
			return app.Catalog(), nil
		case "handoff":
			return app.Handoff(ctx)
		case "runtime":
			if central, ok := resolver.(*centralAppResolver); ok {
				return central.registry.health(), nil
			}
			return registryHealth{WorkspaceCount: 1}, nil
		default:
			return nil, fmt.Errorf("unknown context view %q", input.View)
		}
	}))
	mcp.AddTool(server, tool("hk_doctor", "Run workspace and managed-toolchain diagnostics.", readOnly), routedHandler(resolver, "doctor", func(ctx context.Context, app *devtool.App, _ emptyInput) (any, error) {
		return app.Doctor(ctx), nil
	}))
	mcp.AddTool(server, tool("hk_qa_plan", "Persist a source-bound change-aware QA plan; complete is the completion default.", readOnly), routedHandler(freshResolver, "qa plan", func(ctx context.Context, app *devtool.App, input qaPlanInput) (any, error) {
		if input.Profile == "" {
			input.Profile = "complete"
		}
		return app.QAPlan(ctx, input.Profile, input.BaseRef)
	}))
	mcp.AddTool(server, tool("hk_dev_start", "Accept or reuse an asynchronous development generation.", idempotentAction), routedRequestHandler(freshResolver, "dev start", func(ctx context.Context, _ *mcp.CallToolRequest, app *devtool.App, input devStartInput) (any, error) {
		return app.StartDevDetachedObserved(devtool.WithAgentOutput(ctx), devtool.DevRequest{Variant: input.Variant, Seed: input.Seed}, nil)
	}))
	mcp.AddTool(server, tool("hk_dev_status", "Read generation state and optional bounded cursor-based events.", readOnly), routedHandler(resolver, "dev status", func(ctx context.Context, app *devtool.App, input devStatusInput) (any, error) {
		if input.Cursor < 0 || input.Limit < 0 || input.Limit > maxRequestedLogLines {
			return nil, errors.New("cursor must be non-negative and limit must be between 1 and 200")
		}
		status, err := app.DevStatus(ctx)
		if err != nil || input.Cursor == 0 && input.Limit == 0 {
			return status, err
		}
		if input.Limit == 0 {
			input.Limit = 50
		}
		events, eventErr := app.DevLogs(input.Cursor, input.Limit)
		return struct {
			Status devtool.DevStatus   `json:"status"`
			Events devtool.DevLogBatch `json:"events"`
		}{Status: status, Events: events}, eventErr
	}))
	mcp.AddTool(server, tool("hk_dev_stop", "Stop one exact observed development generation.", destructiveAction), routedHandler(resolver, "dev stop", func(ctx context.Context, app *devtool.App, input devStopInput) (any, error) {
		status, err := app.DevStatus(ctx)
		if err != nil {
			return nil, err
		}
		if input.GenerationID == "" || input.GenerationID != status.GenerationID {
			return nil, errors.New("generation_id must match the currently observed generation")
		}
		return app.StopDev(ctx)
	}))
	mcp.AddTool(server, tool("hk_run_start", "Start setup, QA, build, or smoke work asynchronously.", action), startHandler(freshResolver, "run start", func(input runStartInput) devtool.RunRequest {
		return devtool.RunRequest{Kind: input.Kind, Profile: input.Profile, PlanID: input.PlanID, GateIDs: input.GateIDs, Variant: input.Variant, Target: input.Target}
	}))
	mcp.AddTool(server, tool("hk_run_status", "Get one run and optionally retrieve bounded gate logs by cursor.", readOnly), routedHandler(resolver, "run status", func(_ context.Context, app *devtool.App, input runStatusInput) (any, error) {
		if input.Cursor < 0 || input.Limit < 0 || input.Limit > maxRequestedLogLines {
			return nil, errors.New("cursor must be non-negative and limit must be between 1 and 200")
		}
		run, err := app.GetRun(input.RunID)
		if err != nil || input.GateID == "" && input.Cursor == 0 && input.Limit == 0 {
			return run, err
		}
		if input.Limit == 0 {
			input.Limit = 50
		}
		var logs any
		if input.GateID != "" {
			logs, err = app.TailGateLogAfter(input.RunID, input.GateID, input.Limit, input.Cursor)
		} else {
			logs, err = app.TailLogAfter(input.RunID, input.Limit, input.Cursor)
		}
		return map[string]any{"run": run, "logs": logs}, err
	}))
	mcp.AddTool(server, tool("hk_run_cancel", "Cancel one exact observed finite run.", destructiveAction), routedHandler(resolver, "run cancel", func(_ context.Context, app *devtool.App, input runInput) (any, error) {
		return app.CancelRun(input.RunID)
	}))
	mcp.AddTool(server, tool("hk_screenshot", "Capture and register bounded local screenshot artifacts.", action), screenshotHandler(resolver))
}

func routedHandler[In workspaceScoped](resolver appResolver, command string, handler func(context.Context, *devtool.App, In) (any, error)) func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, envelopeOutput, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input In) (*mcp.CallToolResult, envelopeOutput, error) {
		app, err := resolver.Resolve(ctx, input.workspaceSelector())
		if err != nil {
			return output(nil, command, nil, err)
		}
		value, err := handler(ctx, app, input)
		return output(app, command, value, err)
	}
}

func routedRequestHandler[In workspaceScoped](resolver appResolver, command string, handler func(context.Context, *mcp.CallToolRequest, *devtool.App, In) (any, error)) func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, envelopeOutput, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, envelopeOutput, error) {
		app, err := resolver.Resolve(ctx, input.workspaceSelector())
		if err != nil {
			return output(nil, command, nil, err)
		}
		value, err := handler(ctx, request, app, input)
		return output(app, command, value, err)
	}
}

func screenshotHandler(resolver appResolver) func(context.Context, *mcp.CallToolRequest, screenshotInput) (*mcp.CallToolResult, envelopeOutput, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, input screenshotInput) (*mcp.CallToolResult, envelopeOutput, error) {
		app, err := resolver.Resolve(ctx, input.workspaceSelector())
		if err != nil {
			return output(nil, "screenshot", nil, err)
		}
		result, captureErr := app.CaptureScreenshots(ctx, devtool.ScreenshotRequest{
			Routes: input.Routes, Viewport: input.Viewport, Theme: input.Theme, Scale: input.Scale,
			WaitMS: input.WaitMS, FullPage: input.FullPage, Selector: input.Selector, Anonymous: input.Anonymous,
		})
		toolResult, envelope, _ := output(app, "screenshot", result, captureErr)
		if captureErr != nil {
			return toolResult, envelope, nil
		}
		appendScreenshotResourceLinks(toolResult, result)
		return toolResult, envelope, nil
	}
}

func appendScreenshotResourceLinks(toolResult *mcp.CallToolResult, result devtool.ScreenshotResult) {
	for _, artifact := range result.Artifacts {
		size := artifact.Bytes
		toolResult.Content = append(toolResult.Content, &mcp.ResourceLink{
			URI:         localFileURI(artifact.Path),
			Name:        filepath.Base(artifact.Path),
			Title:       "HitKeep " + artifact.Route,
			Description: fmt.Sprintf("%dx%d local visual-QA screenshot", artifact.Width, artifact.Height),
			MIMEType:    artifact.MIMEType,
			Size:        &size,
		})
	}
}

func localFileURI(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

func startHandler[In workspaceScoped](resolver appResolver, command string, request func(In) devtool.RunRequest) func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, envelopeOutput, error) {
	return routedHandler(resolver, command, func(ctx context.Context, app *devtool.App, input In) (any, error) {
		return app.StartRun(devtool.WithAgentOutput(ctx), request(input))
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
	return &mcp.Tool{Name: name, Description: description, Annotations: annotation, InputSchema: inputSchema(name), OutputSchema: envelopeOutputSchema()}
}

// envelopeOutputSchema returns the inferred envelope schema with the
// `any`-typed data field forced into object form. jsonschema-go marshals the
// empty schema it infers for `any` as the boolean form `true`, which some MCP
// clients (Claude Code) reject when validating outputSchema properties. A
// description-only schema stays permissive but serializes as an object.
var envelopeOutputSchema = sync.OnceValue(func() *jsonschema.Schema {
	schema, err := jsonschema.For[envelopeOutput](nil)
	if err != nil {
		panic(fmt.Errorf("devmcp: infer envelope output schema: %w", err))
	}
	schema.Properties["data"] = &jsonschema.Schema{Description: "Command-specific payload."}
	return schema
})

func inputSchema(name string) *jsonschema.Schema {
	properties := map[string]*jsonschema.Schema{
		"workspace": {Type: "string", MaxLength: new(4096), Description: "Optional configured workspace ID or absolute catalogued path."},
	}
	required := []string{}
	switch name {
	case "hk_context":
		properties["view"] = enum("current", "workspaces", "catalog", "configuration", "handoff", "runtime")
	case "hk_qa_plan":
		properties["profile"] = enum("changed", "complete", "pr", "full")
		properties["base_ref"] = &jsonschema.Schema{Type: "string", MaxLength: new(200)}
	case "hk_run_status":
		properties["run_id"] = runIDSchema()
		properties["limit"] = &jsonschema.Schema{Type: "integer", Minimum: new(1.0), Maximum: new(float64(maxRequestedLogLines))}
		properties["cursor"] = &jsonschema.Schema{Type: "integer", Minimum: new(0.0)}
		properties["gate_id"] = gateEnum()
		required = []string{"run_id"}
	case "hk_run_cancel":
		properties["run_id"] = runIDSchema()
		required = []string{"run_id"}
	case "hk_run_start":
		properties["kind"] = enum("setup", "qa", "build", "smoke")
		properties["plan_id"] = &jsonschema.Schema{Type: "string", MaxLength: new(100)}
		properties["profile"] = enum("changed", "complete", "pr", "full")
		properties["gate_ids"] = &jsonschema.Schema{Type: "array", Items: gateEnum(), UniqueItems: true, MaxItems: new(len(devtool.CatalogSnapshot().Gates))}
		properties["variant"] = enum("self-hosted", "cloud")
		properties["target"] = enum("binary", "image")
		required = []string{"kind"}
	case "hk_dev_start":
		properties["variant"] = enum("self-hosted", "cloud")
		properties["seed"] = &jsonschema.Schema{Type: "boolean"}
	case "hk_dev_status":
		properties["cursor"] = &jsonschema.Schema{Type: "integer", Minimum: new(0.0)}
		properties["limit"] = &jsonschema.Schema{Type: "integer", Minimum: new(1.0), Maximum: new(float64(maxRequestedLogLines))}
	case "hk_dev_stop":
		properties["generation_id"] = &jsonschema.Schema{Type: "string", MinLength: new(1), MaxLength: new(100)}
		required = []string{"generation_id"}
	case "hk_screenshot":
		properties["routes"] = &jsonschema.Schema{
			Type: "array", MinItems: new(1), MaxItems: new(devtool.MaxScreenshotRoutes), UniqueItems: true,
			Items: &jsonschema.Schema{Type: "string", MinLength: new(1), MaxLength: new(2048), Pattern: `^/(?:[^/]|$)`},
		}
		properties["viewport"] = enum("desktop", "mobile")
		properties["theme"] = enum("light", "dark")
		properties["scale"] = &jsonschema.Schema{Type: "integer", Enum: []any{1, 2}}
		properties["wait_ms"] = &jsonschema.Schema{Type: "integer", Minimum: new(0.0), Maximum: new(5000.0)}
		properties["full_page"] = &jsonschema.Schema{Type: "boolean"}
		properties["selector"] = &jsonschema.Schema{Type: "string", MaxLength: new(500)}
		properties["anonymous"] = &jsonschema.Schema{Type: "boolean"}
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

func RunStdio(ctx context.Context, app *devtool.App, version string) error {
	return NewServer(app, version).Run(ctx, &mcp.StdioTransport{})
}

func RunCentralStdio(ctx context.Context, fallbackWorkspace, version string) error {
	resolver := newCentralAppResolver(fallbackWorkspace, nil)
	defer resolver.Close()
	return newServer(resolver, version).Run(ctx, &mcp.StdioTransport{})
}

func ParseLimit(value string) (int, error) {
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 1 || limit > maxRequestedLogLines {
		return 0, errors.New("limit must be between 1 and 200")
	}
	return limit, nil
}
