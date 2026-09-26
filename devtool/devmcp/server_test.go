package devmcp

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"hitkeep/internal/devtool"
	json "hitkeep/jsonapi"
	"hitkeep/mcptest"
)

func TestCentralDeveloperMCPUsesConfiguredFallback(t *testing.T) {
	ctx := context.Background()
	root := testRepository(t)
	if _, err := os.Stat(filepath.Join(root, "hk")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("test repository unexpectedly has an hk launcher: %v", err)
	}
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := devtool.NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := newServer(newCentralAppResolver(root), "test").Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()
	init := clientSession.InitializeResult()
	if init == nil || init.ProtocolVersion != "2026-07-28" {
		t.Fatalf("expected 2026-07-28 stateless negotiation, got %+v", init)
	}
	if init.Capabilities == nil || init.Capabilities.Tools == nil || init.Capabilities.Resources != nil {
		t.Fatalf("unexpected advertised capabilities: %+v", init.Capabilities)
	}
	var capabilityFields map[string]json.RawMessage
	capabilityJSON, err := json.Marshal(init.Capabilities)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(capabilityJSON, &capabilityFields); err != nil {
		t.Fatal(err)
	}
	if _, advertised := capabilityFields["logging"]; advertised {
		t.Fatalf("deprecated logging capability advertised: %s", capabilityJSON)
	}
	if init.Capabilities.Tools.ListChanged {
		t.Fatalf("stateful capabilities advertised: %+v", init.Capabilities)
	}
	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "hk_context", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	structured := result.StructuredContent.(map[string]any)
	if result.IsError || structured["workspace_id"] != app.WorkspaceID() {
		t.Fatalf("configured fallback routing failed: %#v", structured)
	}
	result, err = clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "hk_context", Arguments: map[string]any{}})
	if err != nil || result.IsError {
		t.Fatalf("second request failed: %v %#v", err, result)
	}

}

func TestCentralDeveloperMCPForwardsJSONProgressToken(t *testing.T) {
	ctx := t.Context()
	root := testRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	server := NewCentralServer(root, "test")
	addTestProgressTool(server, newCentralAppResolver(root))
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()

	progress := make(chan any, 1)
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, &mcp.ClientOptions{
		ProgressNotificationHandler: func(_ context.Context, notification *mcp.ProgressNotificationClientRequest) {
			progress <- notification.Params.ProgressToken
		},
	})
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	const wantToken = 1.25
	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Meta:      mcp.Meta{"progressToken": wantToken},
		Name:      "test_progress",
		Arguments: map[string]any{},
	})
	if err != nil || result.IsError {
		t.Fatalf("numeric progress-token request failed: %v %#v", err, result)
	}
	select {
	case got := <-progress:
		if got != wantToken {
			t.Fatalf("progress token = %#v (%T), want %#v (%T)", got, got, wantToken, wantToken)
		}
	case <-time.After(time.Second):
		t.Fatal("request-aware handler emitted no progress notification")
	}
}

func TestCentralDeveloperMCPRejectsUncataloguedWorkspace(t *testing.T) {
	ctx := context.Background()
	fallback := testRepository(t)
	other := testRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := newServer(newCentralAppResolver(fallback), "test").Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()
	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "hk_context", Arguments: map[string]any{"workspace": other},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.StructuredContent.(map[string]any)["error"].(string), "configured fallback clone") {
		t.Fatalf("uncatalogued workspace was accepted: %#v", result.StructuredContent)
	}
}

func TestCentralDeveloperMCPMatchesLocalEnvelope(t *testing.T) {
	root := testRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := devtool.NewApp(root)
	if err != nil {
		t.Fatal(err)
	}

	local := contextEnvelope(t, NewServer(app, "test"))
	central := contextEnvelope(t, NewCentralServer(root, "test"))
	for _, field := range []string{"schema_version", "command", "status", "workspace_id"} {
		if central[field] != local[field] {
			t.Fatalf("central envelope %s = %#v, local = %#v", field, central[field], local[field])
		}
	}
}

func TestCentralDeveloperMCPConcurrentWorkspaceCallsIsolateCancellation(t *testing.T) {
	ctx := t.Context()
	fallback := testRepository(t)
	other := testWorktree(t, fallback)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))

	fallbackApp, err := devtool.NewApp(fallback)
	if err != nil {
		t.Fatal(err)
	}
	otherApp, err := devtool.NewApp(other)
	if err != nil {
		t.Fatal(err)
	}
	fallbackWorkspace, err := fallbackApp.Workspace(ctx)
	if err != nil {
		t.Fatal(err)
	}
	otherWorkspace, err := otherApp.Workspace(ctx)
	if err != nil {
		t.Fatal(err)
	}

	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	server := NewCentralServer(fallback, "test")
	addTestWorkspaceTool(server, newCentralAppResolver(fallback), entered, release)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	clientSession, err := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	defer close(release)
	canceled, cancel := context.WithCancel(ctx)
	defer cancel()
	first := make(chan testToolCall, 1)
	go func() {
		result, err := clientSession.CallTool(canceled, &mcp.CallToolParams{
			Name: "test_workspace", Arguments: map[string]any{"workspace": fallbackApp.WorkspaceID(), "block": true},
		})
		first <- testToolCall{result: result, err: err}
	}()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("canceled request did not reach its selected workspace handler")
	}

	second, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "test_workspace", Arguments: map[string]any{"workspace": otherApp.WorkspaceID()},
	})
	if err != nil || second.IsError {
		t.Fatalf("parallel selected-workspace request failed: %v %#v", err, second)
	}
	assertWorkspaceToolResult(t, second, otherWorkspace)

	cancel()
	select {
	case call := <-first:
		if !errors.Is(call.err, context.Canceled) {
			t.Fatalf("canceled request error = %v, want context cancellation", call.err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled selected-workspace request did not return")
	}

	if fallbackWorkspace.ID == otherWorkspace.ID || fallbackWorkspace.Root == otherWorkspace.Root || fallbackWorkspace.StateDir == otherWorkspace.StateDir {
		t.Fatalf("test worktrees are not distinct: fallback=%+v other=%+v", fallbackWorkspace, otherWorkspace)
	}
}

type testWorkspaceToolInput struct {
	Workspace string `json:"workspace,omitempty"`
	Block     bool   `json:"block,omitempty"`
}

func (input testWorkspaceToolInput) workspaceSelector() string { return input.Workspace }

type testToolCall struct {
	result *mcp.CallToolResult
	err    error
}

func addTestProgressTool(server *mcp.Server, resolver appResolver) {
	mcp.AddTool(server, &mcp.Tool{Name: "test_progress"}, routedRequestHandler(resolver, "test progress", func(ctx context.Context, request *mcp.CallToolRequest, app *devtool.App, _ workspaceInput) (any, error) {
		if request == nil || request.Params == nil || request.Session == nil {
			return nil, errors.New("missing MCP request session")
		}
		if err := request.Session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{
			ProgressToken: request.Params.GetProgressToken(),
			Progress:      1,
			Total:         1,
		}); err != nil {
			return nil, err
		}
		workspace, err := app.Workspace(ctx)
		if err != nil {
			return nil, err
		}
		return map[string]string{"id": workspace.ID}, nil
	}))
}

func addTestWorkspaceTool(server *mcp.Server, resolver appResolver, entered chan<- struct{}, release <-chan struct{}) {
	mcp.AddTool(server, &mcp.Tool{Name: "test_workspace"}, routedRequestHandler(resolver, "test workspace", func(ctx context.Context, _ *mcp.CallToolRequest, app *devtool.App, input testWorkspaceToolInput) (any, error) {
		if input.Block {
			entered <- struct{}{}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-release:
				return nil, context.Canceled
			}
		}
		workspace, err := app.Workspace(ctx)
		if err != nil {
			return nil, err
		}
		return map[string]string{"id": workspace.ID, "root": workspace.Root, "state_dir": workspace.StateDir}, nil
	}))
}

func assertWorkspaceToolResult(t *testing.T, result *mcp.CallToolResult, want devtool.Workspace) {
	t.Helper()
	envelope, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("workspace tool envelope type = %T", result.StructuredContent)
	}
	if got := envelope["workspace_id"]; got != want.ID {
		t.Fatalf("workspace envelope ID = %#v, want %q", got, want.ID)
	}
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("workspace tool data type = %T", envelope["data"])
	}
	for field, expected := range map[string]string{"id": want.ID, "root": want.Root, "state_dir": want.StateDir} {
		if got := data[field]; got != expected {
			t.Fatalf("workspace tool %s = %#v, want %q", field, got, expected)
		}
	}
}

func testWorktree(t *testing.T, root string) string {
	t.Helper()
	for _, arguments := range [][]string{
		{"-C", root, "add", "--all"},
		{"-C", root, "-c", "user.email=test@example.com", "-c", "user.name=HitKeep Test", "commit", "-m", "test workspace base"},
	} {
		command := exec.CommandContext(t.Context(), "git", arguments...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("prepare test worktree: %v\n%s", err, output)
		}
	}
	worktree := filepath.Join(t.TempDir(), "worktree")
	command := exec.CommandContext(t.Context(), "git", "-C", root, "worktree", "add", "--detach", worktree, "HEAD")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("create test worktree: %v\n%s", err, output)
	}
	return worktree
}

func contextEnvelope(t *testing.T, server *mcp.Server) map[string]any {
	t.Helper()
	ctx := t.Context()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	clientSession, err := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()
	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "hk_context", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("hk_context failed: %#v", result.StructuredContent)
	}
	envelope, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structured result type: %T", result.StructuredContent)
	}
	return envelope
}

func TestDeveloperMCPContract(t *testing.T) {
	ctx := context.Background()
	root := testRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := devtool.NewApp(root)
	if err != nil {
		t.Fatal(err)
	}

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := NewServer(app, "test").Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	listed, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"hk_context", "hk_dev_start", "hk_dev_status", "hk_dev_stop", "hk_doctor", "hk_qa_plan", "hk_run_cancel", "hk_run_start", "hk_run_status", "hk_screenshot",
	}
	var got []string
	for _, tool := range listed.Tools {
		got = append(got, tool.Name)
		if tool.Annotations == nil || tool.Annotations.OpenWorldHint == nil || *tool.Annotations.OpenWorldHint {
			t.Fatalf("tool %s is missing closed-world annotations", tool.Name)
		}
		if tool.Name == "hk_dev_start" && !tool.Annotations.IdempotentHint {
			t.Fatal("development start is not marked idempotent")
		}
		// Some MCP clients (Claude Code) reject boolean-form property schemas,
		// which jsonschema-go emits for `any`-typed fields.
		mcptest.RequireObjectFormPropertySchemas(t, tool.Name, "input", tool.InputSchema)
		mcptest.RequireObjectFormPropertySchemas(t, tool.Name, "output", tool.OutputSchema)
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("tool contract mismatch\nwant %v\n got %v", want, got)
	}
	for _, forbidden := range []string{"shell", "exec", "git", "clean", "publish", "release", "deploy", "credential", "delete"} {
		for _, name := range got {
			if name == forbidden || name == "hk_"+forbidden {
				t.Fatalf("forbidden operation exposed: %s", name)
			}
		}
	}

	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "hk_context", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structured result type: %T", result.StructuredContent)
	}
	if structured["schema_version"] != devtool.SchemaVersion || structured["status"] != "ok" {
		t.Fatalf("unexpected structured envelope: %#v", structured)
	}

	if listed.TTLMs != int(mcpListCacheTTL/time.Millisecond) || listed.CacheScope != "public" {
		t.Fatalf("tools/list cache = ttl %d scope %q, want 24h/public", listed.TTLMs, listed.CacheScope)
	}
	prompts, err := clientSession.ListPrompts(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(prompts.Prompts) != 0 {
		t.Fatalf("developer MCP unexpectedly exposes prompts: %d", len(prompts.Prompts))
	}
	resources, err := clientSession.ListResources(ctx, nil)
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	if len(resources.Resources) != 0 {
		t.Fatalf("developer MCP unexpectedly exposes resources: %+v", resources.Resources)
	}
	templates, err := clientSession.ListResourceTemplates(ctx, nil)
	if err != nil {
		t.Fatalf("ListResourceTemplates: %v", err)
	}
	if len(templates.ResourceTemplates) != 0 {
		t.Fatalf("developer MCP unexpectedly exposes resource templates: %+v", templates.ResourceTemplates)
	}
}

func TestScreenshotResultUsesFileResourceLinks(t *testing.T) {
	result := &mcp.CallToolResult{}
	appendScreenshotResourceLinks(result, devtool.ScreenshotResult{Artifacts: []devtool.ScreenshotArtifact{{
		Route: "/admin/status", Path: "/tmp/hitkeep visual/status.png", MIMEType: "image/png", Width: 1440, Height: 1024, Bytes: 42,
	}}})
	if len(result.Content) != 1 {
		t.Fatalf("screenshot content count = %d, want 1", len(result.Content))
	}
	link, ok := result.Content[0].(*mcp.ResourceLink)
	if !ok {
		t.Fatalf("screenshot content type = %T, want resource link", result.Content[0])
	}
	if link.URI != "file:///tmp/hitkeep%20visual/status.png" || link.MIMEType != "image/png" || link.Size == nil || *link.Size != 42 {
		t.Fatalf("unexpected screenshot resource link: %+v", link)
	}
}

func TestDeveloperMCPRejectsOversizedLogs(t *testing.T) {
	ctx := context.Background()
	root := testRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := devtool.NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := NewServer(app, "test").Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	clientSession, err := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()
	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "hk_run_status", Arguments: map[string]any{"run_id": "not-present", "limit": 1000}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatalf("oversized log request was accepted: %#v", result)
	}
}

func TestDeveloperMCPMarksApplicationErrorsAsToolErrors(t *testing.T) {
	ctx := context.Background()
	root := testRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := devtool.NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := NewServer(app, "test").Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	clientSession, err := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "hk_run_status", Arguments: map[string]any{"run_id": "20260714T000000-missing"}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatalf("application error has IsError=false: %#v", result)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok || structured["status"] != "error" || structured["error"] == "" {
		t.Fatalf("application error lost structured envelope: %#v", result.StructuredContent)
	}
}

func testRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if output, err := exec.Command("git", "init", "--quiet", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	if err := os.WriteFile(filepath.Join(root, "CONTRIBUTING.md"), []byte("# Contributing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module hitkeep\n\ngo 1.27.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}
