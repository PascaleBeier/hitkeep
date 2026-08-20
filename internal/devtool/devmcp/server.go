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
	mcpListCacheTTL      = 5 * time.Minute
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
}
type devInput struct{ workspaceInput }
type devStartInput struct {
	workspaceInput
	Variant string `json:"variant,omitempty" jsonschema:"Build variant: self-hosted or cloud"`
	Seed    bool   `json:"seed,omitempty" jsonschema:"Seed isolated demo data before starting"`
}
type devLogsInput struct {
	workspaceInput
	Cursor int64 `json:"cursor,omitempty" jsonschema:"Next event cursor for incremental reads"`
	Limit  int   `json:"limit,omitempty" jsonschema:"Maximum events to return, from 1 through 200"`
	Follow bool  `json:"follow,omitempty" jsonschema:"Continue streaming events until cancellation or session termination"`
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

type toolDelegatingResolver interface {
	DelegateTool(context.Context, *mcp.CallToolRequest, string, workspaceScoped) (*mcp.CallToolResult, envelopeOutput, error)
}

type resourceDelegatingResolver interface {
	DelegateResource(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error)
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
	connect  workspaceMCPConnector
	loadApp  func(string) (*devtool.App, error)
}

func newCentralAppResolver(fallback string, connect workspaceMCPConnector) *centralAppResolver {
	return &centralAppResolver{fallback: fallback, connect: connect}
}

func (resolver *centralAppResolver) connector() workspaceMCPConnector {
	if resolver.connect != nil {
		return resolver.connect
	}
	return connectWorkspaceMCP
}

func (resolver *centralAppResolver) Resolve(ctx context.Context, selector string) (*devtool.App, error) {
	if resolver.fallback == "" {
		return nil, errors.New("no configured fallback HitKeep workspace is available")
	}
	fallback, err := resolver.appForPath(resolver.fallback)
	if err != nil {
		return nil, fmt.Errorf("resolve fallback workspace: %w", err)
	}
	if selector == "" {
		return fallback, nil
	}

	base, err := fallback.Workspace(ctx)
	if err != nil {
		return nil, fmt.Errorf("inspect fallback workspace: %w", err)
	}
	workspaces, err := fallback.Workspaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("list HitKeep workspaces: %w", err)
	}
	workspaces = append(workspaces, base)
	seen := map[string]bool{}
	for _, workspace := range workspaces {
		if seen[workspace.ID] || workspace.ID == "" || workspace.GitCommonDir != base.GitCommonDir {
			continue
		}
		seen[workspace.ID] = true
		app, appErr := resolver.appForPath(workspace.Root)
		if appErr != nil {
			continue
		}
		if selector == app.WorkspaceID() || samePath(selector, app.Root()) {
			return app, nil
		}
	}
	return nil, fmt.Errorf("workspace %q is not one of the configured fallback clone's HitKeep workspaces", selector)
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
		Description: "Stateless, worktree-confined HitKeep development operations.",
		Version:     version,
	}, &mcp.ServerOptions{
		Instructions: "Local, stateless and worktree-confined HitKeep development operations. Each request resolves its configured fallback or explicit workspace selector, while development sessions and finite runs remain explicit filesystem-backed application state. Progress notifications are available during active requests; logs are returned through bounded tools and resources. No arbitrary commands are available.",
		Capabilities: &mcp.ServerCapabilities{
			Tools:     &mcp.ToolCapabilities{ListChanged: false},
			Resources: &mcp.ResourceCapabilities{ListChanged: false, Subscribe: false},
		},
	})
	server.AddReceivingMiddleware(devCacheMiddleware())
	registerTools(server, resolver)
	registerResources(server, resolver)
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
				cache.CacheScope = "private"
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
	readOnly := annotations(true, true, false, false)
	action := annotations(false, false, false, false)
	idempotentAction := annotations(false, true, false, false)
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
	mcp.AddTool(server, tool("hk_doctor", "Check the managed toolchain and container runtime prerequisites.", readOnly), routedHandler(resolver, "doctor", func(ctx context.Context, app *devtool.App, _ emptyInput) (any, error) {
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
	mcp.AddTool(server, tool("hk_dev_start", "Start or reuse the selected workspace's development session and stream progress until ready.", idempotentAction), routedRequestHandler(resolver, "dev start", func(ctx context.Context, request *mcp.CallToolRequest, app *devtool.App, input devStartInput) (any, error) {
		return app.StartDevDetachedObserved(devtool.WithAgentOutput(ctx), devtool.DevRequest{Variant: input.Variant, Seed: input.Seed}, devEventNotifier(ctx, request))
	}))
	mcp.AddTool(server, tool("hk_dev_status", "Inspect the selected workspace's development session.", readOnly), routedHandler(resolver, "dev status", func(ctx context.Context, app *devtool.App, _ devInput) (any, error) {
		return app.DevStatus(ctx)
	}))
	mcp.AddTool(server, tool("hk_dev_logs", "Read or follow cursor-addressed development events without changing the session.", readOnly), routedRequestHandler(resolver, "dev logs", func(ctx context.Context, request *mcp.CallToolRequest, app *devtool.App, input devLogsInput) (any, error) {
		if input.Cursor < 0 || input.Limit < 0 || input.Limit > maxRequestedLogLines {
			return nil, errors.New("cursor must be non-negative and limit must be between 1 and 200")
		}
		notify := devEventNotifier(ctx, request)
		if input.Follow {
			return app.FollowDevEvents(ctx, input.Cursor, input.Limit, notify)
		}
		batch, err := app.DevLogs(input.Cursor, input.Limit)
		if err != nil {
			return batch, err
		}
		for _, event := range batch.Events {
			notify(event)
		}
		return batch, nil
	}))
	mcp.AddTool(server, tool("hk_dev_stop", "Stop only the selected workspace's development session and stream shutdown progress.", destructiveAction), routedRequestHandler(resolver, "dev stop", func(ctx context.Context, request *mcp.CallToolRequest, app *devtool.App, _ devInput) (any, error) {
		before, _ := app.DevStatus(ctx)
		cursor := before.NextEventCursor
		type stopResult struct {
			status devtool.DevStatus
			err    error
		}
		done := make(chan stopResult, 1)
		stopContext := context.WithoutCancel(ctx)
		go func() {
			status, err := app.StopDev(stopContext)
			done <- stopResult{status: status, err: err}
		}()
		notify := devEventNotifier(ctx, request)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case result := <-done:
				batch, _ := app.DevLogs(cursor, maxRequestedLogLines)
				for _, event := range batch.Events {
					notify(event)
				}
				return result.status, result.err
			case <-ctx.Done():
				return app.DevStatus(context.WithoutCancel(ctx))
			case <-ticker.C:
				batch, batchErr := app.DevLogs(cursor, maxRequestedLogLines)
				if batchErr != nil {
					continue
				}
				for _, event := range batch.Events {
					notify(event)
				}
				cursor = batch.NextCursor
			}
		}
	}))
	mcp.AddTool(server, tool("hk_screenshot", "Capture up to eight local dashboard routes in one browser session and return managed PNG resource links.", action), screenshotHandler(resolver))
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
		if delegate, ok := resolver.(toolDelegatingResolver); ok {
			return delegate.DelegateTool(ctx, request, command, input)
		}
		app, err := resolver.Resolve(ctx, input.workspaceSelector())
		if err != nil {
			return output(nil, command, nil, err)
		}
		value, err := handler(ctx, request, app, input)
		return output(app, command, value, err)
	}
}

func screenshotHandler(resolver appResolver) func(context.Context, *mcp.CallToolRequest, screenshotInput) (*mcp.CallToolResult, envelopeOutput, error) {
	return func(ctx context.Context, request *mcp.CallToolRequest, input screenshotInput) (*mcp.CallToolResult, envelopeOutput, error) {
		if delegate, ok := resolver.(toolDelegatingResolver); ok {
			return delegate.DelegateTool(ctx, request, "screenshot", input)
		}
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

func devEventNotifier(ctx context.Context, request *mcp.CallToolRequest) func(devtool.DevEvent) {
	return func(event devtool.DevEvent) {
		if request == nil || request.Session == nil || request.Params == nil {
			return
		}
		if progressToken := request.Params.GetProgressToken(); progressToken != nil {
			_ = request.Session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{
				ProgressToken: progressToken,
				Progress:      float64(event.Cursor),
				Message:       event.Message,
			})
		}
	}
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
		"workspace": {Type: "string", MaxLength: new(4096), Description: "Optional, defaults to configured fallback; accepts a catalogued workspace ID or absolute path."},
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
	case "hk_dev_start":
		properties["variant"] = enum("self-hosted", "cloud")
		properties["seed"] = &jsonschema.Schema{Type: "boolean"}
	case "hk_dev_logs":
		properties["cursor"] = &jsonschema.Schema{Type: "integer", Minimum: new(0.0)}
		properties["limit"] = &jsonschema.Schema{Type: "integer", Minimum: new(1.0), Maximum: new(float64(maxRequestedLogLines))}
		properties["follow"] = &jsonschema.Schema{Type: "boolean"}
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
	case "hk_smoke_start":
		properties["variant"] = enum("self-hosted", "cloud")
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
	server.AddResource(resource("Current workspace", "hitkeep-dev://workspace/current", "Current secret-free fallback worktree state."), resourceHandler(resolver))
	server.AddResource(resource("Build variants", "hitkeep-dev://catalog/variants", "Canonical self-hosted and cloud variant catalog for the fallback worktree."), resourceHandler(resolver))
	server.AddResource(resource("QA catalog", "hitkeep-dev://catalog/qa", "Canonical QA profiles and gate catalog for the fallback worktree."), resourceHandler(resolver))
	server.AddResource(resource("Contributing guide", "hitkeep-dev://docs/contributing", "Repository contributor onboarding guidance for the fallback worktree."), resourceHandler(resolver))
	server.AddResourceTemplate(&mcp.ResourceTemplate{Name: "Workspace current state", Description: "Current state for an explicitly selected worktree.", MIMEType: "application/json", URITemplate: "hitkeep-dev://workspaces/{workspace}/workspace/current"}, resourceHandler(resolver))
	server.AddResourceTemplate(&mcp.ResourceTemplate{Name: "Workspace build variants", Description: "Build variants for an explicitly selected worktree.", MIMEType: "application/json", URITemplate: "hitkeep-dev://workspaces/{workspace}/catalog/variants"}, resourceHandler(resolver))
	server.AddResourceTemplate(&mcp.ResourceTemplate{Name: "Workspace QA catalog", Description: "QA catalog for an explicitly selected worktree.", MIMEType: "application/json", URITemplate: "hitkeep-dev://workspaces/{workspace}/catalog/qa"}, resourceHandler(resolver))
	server.AddResourceTemplate(&mcp.ResourceTemplate{Name: "Workspace contributing guide", Description: "Contributor guidance for an explicitly selected worktree.", MIMEType: "application/json", URITemplate: "hitkeep-dev://workspaces/{workspace}/docs/contributing"}, resourceHandler(resolver))
	server.AddResourceTemplate(&mcp.ResourceTemplate{Name: "Run summary", Description: "One asynchronous hk run summary.", MIMEType: "application/json", URITemplate: "hitkeep-dev://runs/{run_id}/summary"}, resourceHandler(resolver))
	server.AddResourceTemplate(&mcp.ResourceTemplate{Name: "Run logs", Description: "Bounded and redacted run log tail.", MIMEType: "application/json", URITemplate: "hitkeep-dev://runs/{run_id}/logs/{gate}"}, resourceHandler(resolver))
	server.AddResourceTemplate(&mcp.ResourceTemplate{Name: "Workspace run summary", Description: "One asynchronous hk run summary for an explicitly selected worktree.", MIMEType: "application/json", URITemplate: "hitkeep-dev://workspaces/{workspace}/runs/{run_id}/summary"}, resourceHandler(resolver))
	server.AddResourceTemplate(&mcp.ResourceTemplate{Name: "Workspace run logs", Description: "Bounded and redacted run log tail for an explicitly selected worktree.", MIMEType: "application/json", URITemplate: "hitkeep-dev://workspaces/{workspace}/runs/{run_id}/logs/{gate}"}, resourceHandler(resolver))
}

func resource(name, uri, description string) *mcp.Resource {
	return &mcp.Resource{Name: name, URI: uri, Description: description, MIMEType: "application/json"}
}

func resourceHandler(resolver appResolver) mcp.ResourceHandler {
	return func(ctx context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		selector, uri, err := workspaceResourceURI(request.Params.URI)
		if err != nil {
			return nil, err
		}
		if delegate, ok := resolver.(resourceDelegatingResolver); ok {
			return delegate.DelegateResource(ctx, request)
		}
		app, err := resolver.Resolve(ctx, selector)
		if err != nil {
			return nil, err
		}
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
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: request.Params.URI, MIMEType: "application/json", Text: string(raw)}}}, nil
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
