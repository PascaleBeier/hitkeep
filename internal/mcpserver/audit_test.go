package mcpserver

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	json "hitkeep/internal/jsonapi"
)

func TestMCPPublishedSurfaceAudit(t *testing.T) {
	store, _, token := setupMCPStore(t)
	conf := testMCPConfig(t, "")
	handler := NewHandler(conf, store, nil, nil, testMCPLogger())
	ts := httptest.NewServer(handler)
	defer ts.Close()

	session := connectMCPClient(t, ts.URL+conf.MCPPath, token)
	defer session.Close()
	init := session.InitializeResult()
	if init == nil || init.ProtocolVersion != "2026-07-28" {
		t.Fatalf("expected sessionless 2026-07-28 negotiation, got %+v", init)
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
		"hitkeep_get_web_vitals":            true,
		"hitkeep_get_ai_visibility":         true,
		"hitkeep_get_opportunities":         true,
		"hitkeep_get_funnel_stats":          true,
		"hitkeep_get_qr_campaigns":          true,
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
	if tools.TTLMs != int((5*time.Minute)/time.Millisecond) || tools.CacheScope != "private" {
		t.Fatalf("tools/list cache = ttl %d scope %q, want 300000/private", tools.TTLMs, tools.CacheScope)
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
		if !tool.Annotations.IdempotentHint {
			t.Fatalf("tool %q must be marked idempotent", tool.Name)
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
	if resources.TTLMs != int((5*time.Minute)/time.Millisecond) || resources.CacheScope != "private" {
		t.Fatalf("resources/list cache = ttl %d scope %q, want 300000/private", resources.TTLMs, resources.CacheScope)
	}

	templates, err := session.ListResourceTemplates(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListResourceTemplates: %v", err)
	}
	if len(templates.ResourceTemplates) != 1 || templates.ResourceTemplates[0].URITemplate != "hitkeep://docs/{+path}" {
		t.Fatalf("unexpected resource templates: %+v", templates.ResourceTemplates)
	}
	if templates.TTLMs != int((5*time.Minute)/time.Millisecond) || templates.CacheScope != "private" {
		t.Fatalf("resources/templates cache = ttl %d scope %q, want 300000/private", templates.TTLMs, templates.CacheScope)
	}

	help, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: helpMCPURI})
	if err != nil {
		t.Fatalf("ReadResource help: %v", err)
	}
	if help.TTLMs != int(time.Hour/time.Millisecond) || help.CacheScope != "private" {
		t.Fatalf("help resource cache = ttl %d scope %q, want 3600000/private", help.TTLMs, help.CacheScope)
	}
}

func TestMCPDocsDisabledSurfaceAudit(t *testing.T) {
	store, _, token := setupMCPStore(t)
	conf := testMCPConfig(t, "")
	conf.MCPDocsEnabled = false
	handler := NewHandler(conf, store, nil, nil, testMCPLogger())
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
