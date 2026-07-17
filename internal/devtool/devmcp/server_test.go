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
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"hitkeep/internal/devtool"
)

type directCentralResolver struct{ fallback string }

func (resolver directCentralResolver) Resolve(ctx context.Context, session *mcp.ServerSession, selector string) (*devtool.App, error) {
	return newCentralAppResolver(resolver.fallback, nil).Resolve(ctx, session, selector)
}

func newDirectCentralServer(fallback, version string) *mcp.Server {
	return newServer(directCentralResolver{fallback: fallback}, version)
}

func TestCentralDeveloperMCPRoutesByClientRoots(t *testing.T) {
	ctx := context.Background()
	firstRoot := testRepository(t)
	secondRoot := testRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := newDirectCentralServer("", "test").Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	client.AddRoots(&mcp.Root{URI: fileURI(firstRoot), Name: "first"})
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	assertWorkspace := func(root string) {
		t.Helper()
		app, appErr := devtool.NewApp(root)
		if appErr != nil {
			t.Fatal(appErr)
		}
		result, callErr := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "hk_workspace_status", Arguments: map[string]any{}})
		if callErr != nil {
			t.Fatal(callErr)
		}
		structured := result.StructuredContent.(map[string]any)
		if result.IsError || structured["workspace_id"] != app.WorkspaceID() {
			t.Fatalf("central server routed to the wrong workspace: %#v", structured)
		}
	}

	assertWorkspace(firstRoot)
	client.RemoveRoots(fileURI(firstRoot))
	client.AddRoots(&mcp.Root{URI: fileURI(secondRoot), Name: "second"})
	assertWorkspace(secondRoot)
}

func TestCentralDeveloperMCPRequiresSelectorForAmbiguousRoots(t *testing.T) {
	ctx := context.Background()
	firstRoot := testRepository(t)
	secondRoot := testRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	firstApp, err := devtool.NewApp(firstRoot)
	if err != nil {
		t.Fatal(err)
	}

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := newDirectCentralServer("", "test").Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	client.AddRoots(
		&mcp.Root{URI: fileURI(firstRoot), Name: "first"},
		&mcp.Root{URI: fileURI(secondRoot), Name: "second"},
	)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	ambiguous, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "hk_workspace_status", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	structured := ambiguous.StructuredContent.(map[string]any)
	if !ambiguous.IsError || !strings.Contains(structured["error"].(string), "multiple HitKeep workspaces") {
		t.Fatalf("ambiguous workspace roots were accepted: %#v", structured)
	}

	selected, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "hk_workspace_status",
		Arguments: map[string]any{"workspace": firstApp.WorkspaceID()},
	})
	if err != nil {
		t.Fatal(err)
	}
	selectedEnvelope := selected.StructuredContent.(map[string]any)
	if selected.IsError || selectedEnvelope["workspace_id"] != firstApp.WorkspaceID() {
		t.Fatalf("explicit workspace selector did not disambiguate roots: %#v", selectedEnvelope)
	}
}

func TestCentralDeveloperMCPRejectsAmbiguousRootNames(t *testing.T) {
	ctx := context.Background()
	firstRoot := testRepository(t)
	secondRoot := testRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := newDirectCentralServer("", "test").Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	client.AddRoots(
		&mcp.Root{URI: fileURI(firstRoot), Name: "workspace"},
		&mcp.Root{URI: fileURI(secondRoot), Name: "workspace"},
	)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "hk_workspace_status", Arguments: map[string]any{"workspace": "workspace"},
	})
	if err != nil {
		t.Fatal(err)
	}
	envelope := result.StructuredContent.(map[string]any)
	if !result.IsError || !strings.Contains(envelope["error"].(string), "ambiguous") {
		t.Fatalf("duplicate client-root names were accepted: %#v", envelope)
	}
}

func TestCentralAppResolverCachesAndExpiresWorkspaceResolution(t *testing.T) {
	root := testRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := devtool.NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	loads := 0
	resolver := newCentralAppResolver("", nil)
	resolver.now = func() time.Time { return now }
	resolver.loadApp = func(string) (*devtool.App, error) {
		loads++
		return app, nil
	}
	for range 2 {
		resolved, resolveErr := resolver.appForPath(root)
		if resolveErr != nil || resolved != app {
			t.Fatalf("resolve cached app: app=%p err=%v", resolved, resolveErr)
		}
	}
	if loads != 1 {
		t.Fatalf("workspace app loaded %d times, want 1", loads)
	}
	now = now.Add(centralAppCacheTTL)
	if _, err := resolver.appForPath(root); err != nil {
		t.Fatal(err)
	}
	if loads != 2 {
		t.Fatalf("expired workspace app loaded %d times, want 2", loads)
	}
}

func TestCentralDeveloperMCPReusesChildAcrossToolsAndResources(t *testing.T) {
	ctx := context.Background()
	root := testRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	connector := &testWorkspaceMCPConnector{}
	t.Cleanup(connector.Close)
	resolver := newCentralAppResolver("", connector.Connect)
	t.Cleanup(resolver.Close)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := newServer(resolver, "test").Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "outer-client", Version: "test"}, nil)
	client.AddRoots(&mcp.Root{URI: fileURI(root), Name: "workspace"})
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	for range 2 {
		result, callErr := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "hk_dev_status", Arguments: map[string]any{}})
		if callErr != nil || result.IsError {
			t.Fatalf("delegated status: result=%#v err=%v", result, callErr)
		}
	}
	resource, err := clientSession.ReadResource(ctx, &mcp.ReadResourceParams{URI: "hitkeep-dev://catalog/variants"})
	if err != nil || len(resource.Contents) != 1 {
		t.Fatalf("delegated resource: result=%#v err=%v", resource, err)
	}
	if got := connector.connects.Load(); got != 1 {
		t.Fatalf("workspace MCP connections = %d, want 1", got)
	}
}

func TestCentralDeveloperMCPConcurrentCallsShareConnection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	root := testRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	connector := &testWorkspaceMCPConnector{delay: 50 * time.Millisecond}
	t.Cleanup(connector.Close)
	resolver := newCentralAppResolver("", connector.Connect)
	t.Cleanup(resolver.Close)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := newServer(resolver, "test").Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "outer-client", Version: "test"}, nil)
	client.AddRoots(&mcp.Root{URI: fileURI(root), Name: "workspace"})
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	const callCount = 8
	errorsChannel := make(chan error, callCount)
	var calls sync.WaitGroup
	for range callCount {
		calls.Go(func() {
			result, callErr := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "hk_dev_status", Arguments: map[string]any{}})
			if callErr != nil {
				errorsChannel <- callErr
				return
			}
			if result.IsError {
				errorsChannel <- fmt.Errorf("tool error: %#v", result.StructuredContent)
			}
		})
	}
	calls.Wait()
	close(errorsChannel)
	for callErr := range errorsChannel {
		t.Error(callErr)
	}
	if got := connector.connects.Load(); got != 1 {
		t.Fatalf("concurrent workspace MCP connections = %d, want 1", got)
	}
}

func TestCentralDeveloperMCPReconnectsAfterConnectFailure(t *testing.T) {
	ctx := context.Background()
	root := testRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	connector := &testWorkspaceMCPConnector{failFirst: 1}
	t.Cleanup(connector.Close)
	resolver := newCentralAppResolver("", connector.Connect)
	t.Cleanup(resolver.Close)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := newServer(resolver, "test").Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "outer-client", Version: "test"}, nil)
	client.AddRoots(&mcp.Root{URI: fileURI(root), Name: "workspace"})
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	failed, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "hk_dev_status", Arguments: map[string]any{}})
	if err != nil || !failed.IsError {
		t.Fatalf("initial connection failure: result=%#v err=%v", failed, err)
	}
	passed, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "hk_dev_status", Arguments: map[string]any{}})
	if err != nil || passed.IsError {
		t.Fatalf("reconnected status: result=%#v err=%v", passed, err)
	}
	if got := connector.connects.Load(); got != 2 {
		t.Fatalf("workspace MCP connections = %d, want 2", got)
	}
}

func TestCentralDeveloperMCPExpiresIdleConnection(t *testing.T) {
	ctx := context.Background()
	root := testRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	connector := &testWorkspaceMCPConnector{}
	t.Cleanup(connector.Close)
	resolver := newCentralAppResolver("", connector.Connect)
	resolver.childIdleTTL = 20 * time.Millisecond
	t.Cleanup(resolver.Close)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := newServer(resolver, "test").Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "outer-client", Version: "test"}, nil)
	client.AddRoots(&mcp.Root{URI: fileURI(root), Name: "workspace"})
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "hk_dev_status", Arguments: map[string]any{}})
	if err != nil || result.IsError {
		t.Fatalf("initial status: result=%#v err=%v", result, err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		resolver.childMu.Lock()
		remaining := len(resolver.children)
		resolver.childMu.Unlock()
		if remaining == 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	resolver.childMu.Lock()
	remaining := len(resolver.children)
	resolver.childMu.Unlock()
	if remaining != 0 {
		t.Fatal("idle workspace MCP connection was not evicted")
	}
	result, err = clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "hk_dev_status", Arguments: map[string]any{}})
	if err != nil || result.IsError {
		t.Fatalf("status after idle eviction: result=%#v err=%v", result, err)
	}
	if got := connector.connects.Load(); got != 2 {
		t.Fatalf("workspace MCP connections after idle eviction = %d, want 2", got)
	}
}

func TestCentralDeveloperMCPShutdownClosesActiveFollow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	root := testRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := devtool.NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	writeActiveDevFixture(t, ctx, app, "generation-pool-shutdown")
	connector := &testWorkspaceMCPConnector{}
	t.Cleanup(connector.Close)
	resolver := newCentralAppResolver("", connector.Connect)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := newServer(resolver, "test").Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "outer-client", Version: "test"}, nil)
	client.AddRoots(&mcp.Root{URI: fileURI(root), Name: "workspace"})
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	callDone := make(chan struct{})
	go func() {
		defer close(callDone)
		_, _ = clientSession.CallTool(ctx, &mcp.CallToolParams{
			Name: "hk_dev_logs", Arguments: map[string]any{"follow": true},
		})
	}()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		resolver.childMu.Lock()
		active := 0
		for _, child := range resolver.children {
			active += child.activeCalls
		}
		resolver.childMu.Unlock()
		if active == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	resolver.childMu.Lock()
	active := 0
	for _, child := range resolver.children {
		active += child.activeCalls
	}
	resolver.childMu.Unlock()
	if active != 1 {
		t.Fatalf("active follow calls = %d, want 1", active)
	}
	closeDone := make(chan struct{})
	go func() {
		resolver.Close()
		close(closeDone)
	}()
	select {
	case <-closeDone:
	case <-time.After(time.Second):
		t.Fatal("workspace MCP pool shutdown blocked on active follow")
	}
	select {
	case <-callDone:
	case <-time.After(time.Second):
		t.Fatal("active follow survived workspace MCP pool shutdown")
	}
	status, err := app.DevStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.State != devtool.DevStateReady {
		t.Fatalf("pool shutdown changed development state: %+v", status)
	}
	// Let the in-memory child handler finish before its server-session cleanup.
	// The production command transport is terminated by lifetime cancellation.
	writeDevEventFixture(t, context.Background(), app, "generation-pool-shutdown", "test cleanup")
}

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

func TestCentralDeveloperMCPIgnoresNonHitKeepClientRoots(t *testing.T) {
	ctx := context.Background()
	hitkeepRoot := testRepository(t)
	unrelatedRoot := t.TempDir()
	if output, err := exec.Command("git", "init", "--quiet", unrelatedRoot).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	wantApp, err := devtool.NewApp(hitkeepRoot)
	if err != nil {
		t.Fatal(err)
	}

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := newDirectCentralServer("", "test").Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	client.AddRoots(
		&mcp.Root{URI: fileURI(hitkeepRoot), Name: "hitkeep"},
		&mcp.Root{URI: fileURI(unrelatedRoot), Name: "unrelated"},
	)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "hk_workspace_status", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	envelope := result.StructuredContent.(map[string]any)
	if result.IsError || envelope["workspace_id"] != wantApp.WorkspaceID() {
		t.Fatalf("non-HitKeep client root affected routing: %#v", envelope)
	}
}

func TestCentralDeveloperMCPRejectsUnadvertisedWorkspaceSelector(t *testing.T) {
	ctx := context.Background()
	hiddenHitKeepRoot := testRepository(t)
	unrelatedRoot := t.TempDir()
	if output, err := exec.Command("git", "init", "--quiet", unrelatedRoot).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := newDirectCentralServer("", "test").Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	client.AddRoots(&mcp.Root{URI: fileURI(unrelatedRoot), Name: "unrelated"})
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "hk_workspace_status",
		Arguments: map[string]any{"workspace": hiddenHitKeepRoot},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatalf("unadvertised workspace selector was accepted: %#v", result.StructuredContent)
	}
}

func fileURI(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
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
		"hk_run_cancel", "hk_run_list", "hk_run_status", "hk_setup_start", "hk_smoke_start", "hk_workspace_handoff", "hk_workspace_list", "hk_workspace_status",
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
	if len(templates.ResourceTemplates) != 2 {
		t.Fatalf("resource templates: got %d, want 2", len(templates.ResourceTemplates))
	}
	read, err := clientSession.ReadResource(ctx, &mcp.ReadResourceParams{URI: "hitkeep-dev://catalog/variants"})
	if err != nil || len(read.Contents) != 1 || read.Contents[0].Text == "" {
		t.Fatalf("read catalog: %#v, %v", read, err)
	}
	prompts, err := clientSession.ListPrompts(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(prompts.Prompts) != 0 {
		t.Fatalf("developer MCP unexpectedly exposes prompts: %d", len(prompts.Prompts))
	}
}

func TestDeveloperMCPDevLogsEmitProgressAndStructuredLogging(t *testing.T) {
	ctx := context.Background()
	root := testRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := devtool.NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := app.Workspace(ctx)
	if err != nil {
		t.Fatal(err)
	}
	generationID := "generation-mcp-events"
	devDir := filepath.Join(workspace.StateDir, "dev")
	if err := os.MkdirAll(filepath.Join(devDir, "events"), 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	session := map[string]any{
		"state": "stopped", "generation_id": generationID, "variant": "self-hosted",
		"owner": "detached", "started_at": now, "stopped_at": now, "updated_at": now,
		"urls": workspace.URLs, "next_event_cursor": 1,
	}
	raw, _ := json.Marshal(session)
	if err := os.WriteFile(filepath.Join(devDir, "session.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	event := devtool.DevEvent{
		Cursor: 0, Timestamp: now, Type: "log", Component: "backend", Level: "info", Message: "backend ready",
	}
	raw, _ = json.Marshal(event)
	if err := os.WriteFile(filepath.Join(devDir, "events", generationID+".ndjson"), append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}

	progress := make(chan *mcp.ProgressNotificationParams, 1)
	logs := make(chan *mcp.LoggingMessageParams, 1)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := NewServer(app, "test").Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, &mcp.ClientOptions{
		ProgressNotificationHandler: func(_ context.Context, request *mcp.ProgressNotificationClientRequest) {
			progress <- request.Params
		},
		LoggingMessageHandler: func(_ context.Context, request *mcp.LoggingMessageRequest) {
			logs <- request.Params
		},
	})
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()
	if err := clientSession.SetLoggingLevel(ctx, &mcp.SetLoggingLevelParams{Level: mcp.LoggingLevel("debug")}); err != nil {
		t.Fatal(err)
	}
	params := &mcp.CallToolParams{Name: "hk_dev_logs", Arguments: map[string]any{"limit": 20}}
	params.SetProgressToken("dev-progress")
	result, err := clientSession.CallTool(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("dev logs failed: %#v", result.StructuredContent)
	}
	select {
	case notification := <-progress:
		if notification.ProgressToken != "dev-progress" || notification.Progress != 0 {
			t.Fatalf("unexpected progress notification: %+v", notification)
		}
	case <-time.After(time.Second):
		t.Fatal("no progress notification received")
	}
	select {
	case notification := <-logs:
		if notification.Logger != "hitkeep.dev.backend" || notification.Level != "info" {
			t.Fatalf("unexpected log notification: %+v", notification)
		}
	case <-time.After(time.Second):
		t.Fatal("no structured log notification received")
	}
	envelope := result.StructuredContent.(map[string]any)
	data := envelope["data"].(map[string]any)
	if len(data["events"].([]any)) != 1 || data["next_cursor"].(float64) != 1 {
		t.Fatalf("fallback result lost bounded events: %#v", data)
	}
}

func TestCentralDeveloperMCPForwardsDevNotifications(t *testing.T) {
	ctx := context.Background()
	root := testRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := devtool.NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	writeDevEventFixture(t, ctx, app, "generation-forwarded", "forward me")

	var childServers []*mcp.ServerSession
	connector := func(connectContext, _ context.Context, childApp *devtool.App, options *mcp.ClientOptions) (*mcp.ClientSession, error) {
		serverTransport, clientTransport := mcp.NewInMemoryTransports()
		serverSession, connectErr := NewServer(childApp, "test-child").Connect(connectContext, serverTransport, nil)
		if connectErr != nil {
			return nil, connectErr
		}
		childServers = append(childServers, serverSession)
		client := mcp.NewClient(&mcp.Implementation{Name: "test-broker", Version: "test"}, options)
		clientSession, connectErr := client.Connect(connectContext, clientTransport, nil)
		if connectErr == nil && options != nil && options.LoggingMessageHandler != nil {
			connectErr = clientSession.SetLoggingLevel(connectContext, &mcp.SetLoggingLevelParams{Level: mcp.LoggingLevel("debug")})
		}
		return clientSession, connectErr
	}
	t.Cleanup(func() {
		for _, session := range childServers {
			_ = session.Close()
		}
	})

	progress := make(chan *mcp.ProgressNotificationParams, 1)
	logs := make(chan *mcp.LoggingMessageParams, 1)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	resolver := newCentralAppResolver("", connector)
	t.Cleanup(resolver.Close)
	serverSession, err := newServer(resolver, "test").Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "outer-client", Version: "test"}, &mcp.ClientOptions{
		ProgressNotificationHandler: func(_ context.Context, request *mcp.ProgressNotificationClientRequest) {
			progress <- request.Params
		},
		LoggingMessageHandler: func(_ context.Context, request *mcp.LoggingMessageRequest) {
			logs <- request.Params
		},
	})
	client.AddRoots(&mcp.Root{URI: fileURI(root), Name: "workspace"})
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()
	if err := clientSession.SetLoggingLevel(ctx, &mcp.SetLoggingLevelParams{Level: mcp.LoggingLevel("debug")}); err != nil {
		t.Fatal(err)
	}
	params := &mcp.CallToolParams{Name: "hk_dev_logs", Arguments: map[string]any{"limit": 20}}
	params.SetProgressToken("outer-progress")
	result, err := clientSession.CallTool(ctx, params)
	if err != nil || result.IsError {
		t.Fatalf("delegated dev logs: result=%#v err=%v", result, err)
	}
	select {
	case notification := <-progress:
		if notification.ProgressToken != "outer-progress" || notification.Message != "forward me" {
			t.Fatalf("progress was not forwarded: %+v", notification)
		}
	case <-time.After(time.Second):
		t.Fatal("central broker dropped progress notification")
	}
	select {
	case notification := <-logs:
		if notification.Logger != "hitkeep.dev.backend" {
			t.Fatalf("logging was not forwarded: %+v", notification)
		}
	case <-time.After(time.Second):
		t.Fatal("central broker dropped logging notification")
	}
}

func TestCentralDeveloperMCPMultiplexesConcurrentProgress(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	root := testRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := devtool.NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	writeDevEventFixture(t, ctx, app, "generation-progress-multiplex", "multiplex me")
	connector := &testWorkspaceMCPConnector{delay: 25 * time.Millisecond}
	t.Cleanup(connector.Close)
	resolver := newCentralAppResolver("", connector.Connect)
	t.Cleanup(resolver.Close)

	progress := make(chan *mcp.ProgressNotificationParams, 2)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := newServer(resolver, "test").Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "outer-client", Version: "test"}, &mcp.ClientOptions{
		ProgressNotificationHandler: func(_ context.Context, request *mcp.ProgressNotificationClientRequest) {
			progress <- request.Params
		},
	})
	client.AddRoots(&mcp.Root{URI: fileURI(root), Name: "workspace"})
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	var calls sync.WaitGroup
	callErrors := make(chan error, 2)
	for _, token := range []string{"outer-one", "outer-two"} {
		calls.Go(func() {
			params := &mcp.CallToolParams{Name: "hk_dev_logs", Arguments: map[string]any{"limit": 20}}
			params.SetProgressToken(token)
			result, callErr := clientSession.CallTool(ctx, params)
			if callErr != nil {
				callErrors <- callErr
				return
			}
			if result.IsError {
				callErrors <- fmt.Errorf("tool error: %#v", result.StructuredContent)
			}
		})
	}
	calls.Wait()
	close(callErrors)
	for callErr := range callErrors {
		t.Fatal(callErr)
	}
	seen := map[any]bool{}
	for range 2 {
		select {
		case notification := <-progress:
			seen[notification.ProgressToken] = true
		case <-ctx.Done():
			t.Fatal("timed out waiting for multiplexed progress")
		}
	}
	if !seen["outer-one"] || !seen["outer-two"] {
		t.Fatalf("progress tokens were crossed or lost: %v", seen)
	}
	if got := connector.connects.Load(); got != 1 {
		t.Fatalf("concurrent progress used %d child connections, want 1", got)
	}
}

func TestCentralDeveloperMCPForwardsFollowCancellation(t *testing.T) {
	ctx := context.Background()
	root := testRepository(t)
	t.Setenv("HK_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	app, err := devtool.NewApp(root)
	if err != nil {
		t.Fatal(err)
	}
	writeActiveDevFixture(t, ctx, app, "generation-cancel-follow")

	var childServers []*mcp.ServerSession
	var connections atomic.Int32
	connector := func(connectContext, _ context.Context, childApp *devtool.App, options *mcp.ClientOptions) (*mcp.ClientSession, error) {
		connections.Add(1)
		serverTransport, clientTransport := mcp.NewInMemoryTransports()
		serverSession, connectErr := NewServer(childApp, "test-child").Connect(connectContext, serverTransport, nil)
		if connectErr != nil {
			return nil, connectErr
		}
		childServers = append(childServers, serverSession)
		client := mcp.NewClient(&mcp.Implementation{Name: "test-broker", Version: "test"}, options)
		return client.Connect(connectContext, clientTransport, nil)
	}
	t.Cleanup(func() {
		for _, session := range childServers {
			_ = session.Close()
		}
	})

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	resolver := newCentralAppResolver("", connector)
	t.Cleanup(resolver.Close)
	serverSession, err := newServer(resolver, "test").Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "outer-client", Version: "test"}, nil)
	client.AddRoots(&mcp.Root{URI: fileURI(root), Name: "workspace"})
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	callContext, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	_, callErr := clientSession.CallTool(callContext, &mcp.CallToolParams{
		Name: "hk_dev_logs", Arguments: map[string]any{"follow": true},
	})
	if !errors.Is(callErr, context.DeadlineExceeded) {
		t.Fatalf("delegated follow cancellation error = %v", callErr)
	}
	status, err := app.DevStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != devtool.DevStateReady {
		t.Fatalf("follow cancellation changed development state: %+v", status)
	}
	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "hk_dev_status", Arguments: map[string]any{}})
	if err != nil || result.IsError {
		t.Fatalf("status after follow cancellation: result=%#v err=%v", result, err)
	}
	if got := connections.Load(); got != 1 {
		t.Fatalf("follow cancellation replaced workspace MCP connection: got %d connections", got)
	}
}

func writeDevEventFixture(t *testing.T, ctx context.Context, app *devtool.App, generationID, message string) {
	t.Helper()
	workspace, err := app.Workspace(ctx)
	if err != nil {
		t.Fatal(err)
	}
	devDir := filepath.Join(workspace.StateDir, "dev")
	if err := os.MkdirAll(filepath.Join(devDir, "events"), 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	session := map[string]any{
		"state": "stopped", "generation_id": generationID, "variant": "self-hosted",
		"owner": "detached", "started_at": now, "stopped_at": now, "updated_at": now,
		"urls": workspace.URLs, "next_event_cursor": 1,
	}
	raw, _ := json.Marshal(session)
	if err := os.WriteFile(filepath.Join(devDir, "session.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	event := devtool.DevEvent{
		Cursor: 0, Timestamp: now, Type: "log", Component: "backend", Level: "info", Message: message,
	}
	raw, _ = json.Marshal(event)
	if err := os.WriteFile(filepath.Join(devDir, "events", generationID+".ndjson"), append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeActiveDevFixture(t *testing.T, ctx context.Context, app *devtool.App, generationID string) {
	t.Helper()
	workspace, err := app.Workspace(ctx)
	if err != nil {
		t.Fatal(err)
	}
	devDir := filepath.Join(workspace.StateDir, "dev")
	if err := os.MkdirAll(filepath.Join(devDir, "events"), 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	session := map[string]any{
		"state": "ready", "generation_id": generationID, "variant": "self-hosted",
		"owner": "detached", "started_at": now, "ready_at": now, "updated_at": now,
		"urls": workspace.URLs, "next_event_cursor": 0, "supervisor_pid": os.Getpid(),
	}
	raw, _ := json.Marshal(session)
	if err := os.WriteFile(filepath.Join(devDir, "session.json"), raw, 0o600); err != nil {
		t.Fatal(err)
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
	if options != nil && options.LoggingMessageHandler != nil {
		if err := clientSession.SetLoggingLevel(connectContext, &mcp.SetLoggingLevelParams{Level: mcp.LoggingLevel("debug")}); err != nil {
			_ = clientSession.Close()
			_ = serverSession.Close()
			return nil, err
		}
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
