package devmcp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"hitkeep/internal/devtool"
)

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
		"hk_build_start", "hk_dev_start", "hk_dev_stop", "hk_doctor", "hk_logs_tail", "hk_qa_plan", "hk_qa_start",
		"hk_run_cancel", "hk_run_list", "hk_run_status", "hk_setup_start", "hk_smoke_start", "hk_workspace_handoff", "hk_workspace_list", "hk_workspace_status",
	}
	var got []string
	for _, tool := range listed.Tools {
		got = append(got, tool.Name)
		if tool.Annotations == nil || tool.Annotations.OpenWorldHint == nil || *tool.Annotations.OpenWorldHint {
			t.Fatalf("tool %s is missing closed-world annotations", tool.Name)
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
	return root
}
