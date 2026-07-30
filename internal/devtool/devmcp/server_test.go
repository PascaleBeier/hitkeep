package devmcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"hitkeep/internal/devtool"
	"hitkeep/internal/mcptest"
)

func TestDelegatedEnvelopeRejectsMismatchedResults(t *testing.T) {
	valid := devtool.SuccessEnvelope("dev status", "workspace-id", map[string]any{"state": "ready"})
	if _, err := delegatedEnvelope(valid, "dev status", "workspace-id", false); err != nil {
		t.Fatalf("valid delegated envelope was rejected: %v", err)
	}
	tests := map[string]devtool.Envelope{
		"schema":    valid,
		"command":   valid,
		"workspace": valid,
		"status":    valid,
	}
	broken := tests["schema"]
	broken.SchemaVersion = "hk.dev/v1"
	tests["schema"] = broken
	broken = tests["command"]
	broken.Command = "workspace status"
	tests["command"] = broken
	broken = tests["workspace"]
	broken.WorkspaceID = "other-workspace"
	tests["workspace"] = broken
	broken = tests["status"]
	broken.Status = "error"
	tests["status"] = broken
	for name, envelope := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := delegatedEnvelope(envelope, "dev status", "workspace-id", false); err == nil {
				t.Fatalf("mismatched envelope was accepted: %+v", envelope)
			}
		})
	}
}

func TestWorkspaceResourceURIUnescapesAbsoluteSelector(t *testing.T) {
	raw := "hitkeep-dev://workspaces/" + url.PathEscape("/tmp/hitkeep-worktree") + "/catalog/variants"
	selector, localURI, err := workspaceResourceURI(raw)
	if err != nil {
		t.Fatal(err)
	}
	if selector != "/tmp/hitkeep-worktree" || localURI != "hitkeep-dev://catalog/variants" {
		t.Fatalf("workspace resource URI = selector %q uri %q", selector, localURI)
	}
}

func TestCentralDeveloperMCPUsesConfiguredFallback(t *testing.T) {
	ctx := context.Background()
	root := testRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := devtool.NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	connector := &testWorkspaceMCPConnector{}
	defer connector.Close()
	serverSession, err := newServer(newCentralAppResolver(root, connector.Connect), "test").Connect(ctx, serverTransport, nil)
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
	if init.Capabilities == nil || init.Capabilities.Tools == nil || init.Capabilities.Resources == nil {
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
	if init.Capabilities.Tools.ListChanged || init.Capabilities.Resources.ListChanged || init.Capabilities.Resources.Subscribe {
		t.Fatalf("stateful capabilities advertised: %+v", init.Capabilities)
	}
	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "hk_workspace_status", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	structured := result.StructuredContent.(map[string]any)
	if result.IsError || structured["workspace_id"] != app.WorkspaceID() {
		t.Fatalf("configured fallback routing failed: %#v", structured)
	}
	result, err = clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "hk_workspace_status", Arguments: map[string]any{}})
	if err != nil || result.IsError {
		t.Fatalf("second request failed: %v %#v", err, result)
	}
	if got := connector.connects.Load(); got != 2 {
		t.Fatalf("request-scoped child connections = %d, want 2", got)
	}

	read, err := clientSession.ReadResource(ctx, &mcp.ReadResourceParams{
		URI: "hitkeep-dev://workspaces/" + url.PathEscape(app.WorkspaceID()) + "/catalog/variants",
	})
	if err != nil || len(read.Contents) != 1 || read.Contents[0].Text == "" {
		t.Fatalf("explicit workspace resource failed: %v %#v", err, read)
	}
	if got := connector.connects.Load(); got != 3 {
		t.Fatalf("explicit resource did not use a request-scoped child connection: %d", got)
	}
}

func TestCentralDeveloperMCPRejectsUncataloguedWorkspace(t *testing.T) {
	ctx := context.Background()
	fallback := testRepository(t)
	other := testRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	connector := &testWorkspaceMCPConnector{}
	defer connector.Close()
	serverSession, err := newServer(newCentralAppResolver(fallback, connector.Connect), "test").Connect(ctx, serverTransport, nil)
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
		Name: "hk_workspace_status", Arguments: map[string]any{"workspace": other},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.StructuredContent.(map[string]any)["error"].(string), "configured fallback clone") {
		t.Fatalf("uncatalogued workspace was accepted: %#v", result.StructuredContent)
	}
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
		"hk_build_start", "hk_dev_logs", "hk_dev_start", "hk_dev_status", "hk_dev_stop", "hk_doctor", "hk_logs_tail", "hk_qa_plan", "hk_qa_start",
		"hk_run_cancel", "hk_run_list", "hk_run_status", "hk_screenshot", "hk_setup_start", "hk_smoke_start", "hk_workspace_handoff", "hk_workspace_list", "hk_workspace_status",
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

	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "hk_workspace_status", Arguments: map[string]any{}})
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

	resources, err := clientSession.ListResources(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources.Resources) != 4 {
		t.Fatalf("resources: got %d, want 4", len(resources.Resources))
	}
	templates, err := clientSession.ListResourceTemplates(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(templates.ResourceTemplates) != 8 {
		t.Fatalf("resource templates: got %d, want 8", len(templates.ResourceTemplates))
	}
	if listed.TTLMs != int(mcpListCacheTTL/time.Millisecond) || listed.CacheScope != "private" {
		t.Fatalf("tools/list cache = ttl %d scope %q, want 300000/private", listed.TTLMs, listed.CacheScope)
	}
	if resources.TTLMs != int(mcpListCacheTTL/time.Millisecond) || resources.CacheScope != "private" {
		t.Fatalf("resources/list cache = ttl %d scope %q, want 300000/private", resources.TTLMs, resources.CacheScope)
	}
	if templates.TTLMs != int(mcpListCacheTTL/time.Millisecond) || templates.CacheScope != "private" {
		t.Fatalf("resources/templates cache = ttl %d scope %q, want 300000/private", templates.TTLMs, templates.CacheScope)
	}
	read, err := clientSession.ReadResource(ctx, &mcp.ReadResourceParams{URI: "hitkeep-dev://catalog/variants"})
	if err != nil || len(read.Contents) != 1 || read.Contents[0].Text == "" {
		t.Fatalf("read catalog: %#v, %v", read, err)
	}
	if read.TTLMs != 0 || read.CacheScope != "private" {
		t.Fatalf("mutable catalog resource cache = ttl %d scope %q, want 0/private", read.TTLMs, read.CacheScope)
	}
	prompts, err := clientSession.ListPrompts(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(prompts.Prompts) != 0 {
		t.Fatalf("developer MCP unexpectedly exposes prompts: %d", len(prompts.Prompts))
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
	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "hk_logs_tail", Arguments: map[string]any{"run_id": "not-present", "limit": 1000}})
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
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module hitkeep\n\ngo 1.26.5\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

type testWorkspaceMCPConnector struct {
	connects  atomic.Int32
	failFirst int32
	delay     time.Duration
	mu        sync.Mutex
	servers   []*mcp.ServerSession
}

func (connector *testWorkspaceMCPConnector) Connect(connectContext, lifetimeContext context.Context, app *devtool.App, options *mcp.ClientOptions) (*mcp.ClientSession, error) {
	connectionNumber := connector.connects.Add(1)
	if connector.delay > 0 {
		timer := time.NewTimer(connector.delay)
		defer timer.Stop()
		select {
		case <-connectContext.Done():
			return nil, connectContext.Err()
		case <-lifetimeContext.Done():
			return nil, lifetimeContext.Err()
		case <-timer.C:
		}
	}
	if connectionNumber <= connector.failFirst {
		return nil, errors.New("injected workspace MCP connection failure")
	}
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := NewServer(app, "test-child").Connect(connectContext, serverTransport, nil)
	if err != nil {
		return nil, err
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-broker", Version: "test"}, options)
	clientSession, err := client.Connect(connectContext, clientTransport, nil)
	if err != nil {
		_ = serverSession.Close()
		return nil, err
	}
	connector.mu.Lock()
	connector.servers = append(connector.servers, serverSession)
	connector.mu.Unlock()
	return clientSession, nil
}

func (connector *testWorkspaceMCPConnector) Close() {
	connector.mu.Lock()
	servers := slices.Clone(connector.servers)
	connector.servers = nil
	connector.mu.Unlock()
	for _, server := range servers {
		_ = server.Close()
	}
}
