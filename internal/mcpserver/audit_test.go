package mcpserver

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPPublishedSurfaceAudit(t *testing.T) {
	store, _, token := setupMCPStore(t)
	conf := testMCPConfig(t, "")
	handler := NewHandler(conf, store, nil, nil, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	session := connectMCPClient(t, ts.URL+conf.MCPPath, token)
	defer session.Close()

	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	wantTools := map[string]bool{
		"hitkeep_list_sites":                true,
		"hitkeep_get_site_overview":         true,
		"hitkeep_get_event_names":           true,
		"hitkeep_get_event_breakdown":       true,
		"hitkeep_get_ecommerce":             true,
		"hitkeep_get_ai_visibility":         true,
		"hitkeep_get_opportunities":         true,
		"hitkeep_get_search_console_status": true,
		"hitkeep_get_search_console":        true,
		"hitkeep_search_docs":               true,
		"hitkeep_get_doc":                   true,
		"hitkeep_get_api_reference":         true,
		"hitkeep_get_mcp_help":              true,
	}
	if len(tools.Tools) != len(wantTools) {
		t.Fatalf("expected %d tools, got %d: %v", len(wantTools), len(tools.Tools), toolNames(tools.Tools))
	}
	for _, tool := range tools.Tools {
		if !wantTools[tool.Name] {
			t.Fatalf("unexpected MCP tool %q", tool.Name)
		}
		if strings.TrimSpace(tool.Title) == "" || strings.TrimSpace(tool.Description) == "" {
			t.Fatalf("tool %q must have title and description", tool.Name)
		}
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Fatalf("tool %q must be marked read-only", tool.Name)
		}
		openWorld := tool.Annotations.OpenWorldHint != nil && *tool.Annotations.OpenWorldHint
		if strings.Contains(tool.Name, "_doc") || strings.Contains(tool.Name, "_api_reference") || strings.Contains(tool.Name, "search_docs") {
			if !openWorld {
				t.Fatalf("docs tool %q should declare open-world docs fetching", tool.Name)
			}
		} else if openWorld {
			t.Fatalf("analytics/local tool %q should not declare open-world behavior", tool.Name)
		}
		if isForbiddenToolName(tool.Name) {
			t.Fatalf("tool %q violates read-only aggregate-only surface policy", tool.Name)
		}
	}

	resources, err := session.ListResources(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	for _, uri := range []string{helpMCPURI, helpMetricsURI, docsLLMSURI} {
		if !hasResource(resources.Resources, uri) {
			t.Fatalf("expected resource %s", uri)
		}
	}

	templates, err := session.ListResourceTemplates(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListResourceTemplates: %v", err)
	}
	if len(templates.ResourceTemplates) != 1 || templates.ResourceTemplates[0].URITemplate != "hitkeep://docs/{+path}" {
		t.Fatalf("unexpected resource templates: %+v", templates.ResourceTemplates)
	}
}

func TestMCPDocsDisabledSurfaceAudit(t *testing.T) {
	store, _, token := setupMCPStore(t)
	conf := testMCPConfig(t, "")
	conf.MCPDocsEnabled = false
	handler := NewHandler(conf, store, nil, nil, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	session := connectMCPClient(t, ts.URL+conf.MCPPath, token)
	defer session.Close()

	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, tool := range tools.Tools {
		if strings.Contains(tool.Name, "_doc") || strings.Contains(tool.Name, "_api_reference") || strings.Contains(tool.Name, "search_docs") {
			t.Fatalf("docs-disabled server should not expose docs tool %q", tool.Name)
		}
	}

	resources, err := session.ListResources(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	if hasResource(resources.Resources, docsLLMSURI) {
		t.Fatalf("docs-disabled server should not expose docs llms resource")
	}

	templates, err := session.ListResourceTemplates(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListResourceTemplates: %v", err)
	}
	if len(templates.ResourceTemplates) != 0 {
		t.Fatalf("docs-disabled server should not expose docs templates: %+v", templates.ResourceTemplates)
	}
}

func isForbiddenToolName(name string) bool {
	for _, part := range []string{
		"create",
		"delete",
		"update",
		"mutate",
		"write",
		"export_hits",
		"raw_hits",
		"billing",
		"takeout",
		"exclusion",
		"goal_mutation",
	} {
		if strings.Contains(name, part) {
			return true
		}
	}
	return false
}

func toolNames(tools []*mcp.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}
