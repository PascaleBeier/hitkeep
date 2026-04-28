package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"hitkeep/internal/api"
	authcore "hitkeep/internal/auth"
	"hitkeep/internal/config"
	"hitkeep/internal/database"
)

func TestMCPServerRequiresBearerToken(t *testing.T) {
	store, _, _ := setupMCPStore(t)
	conf := testMCPConfig(t, "")
	handler := NewHandler(conf, store, nil, nil, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	req := httptest.NewRequest(http.MethodPost, ts.URL+conf.MCPPath, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestMCPServerRejectsMalformedAndRevokedBearerToken(t *testing.T) {
	store, _, token := setupMCPStore(t)
	conf := testMCPConfig(t, "")
	handler := NewHandler(conf, store, nil, nil, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	req := httptest.NewRequest(http.MethodPost, ts.URL+conf.MCPPath, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Token "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected malformed auth to get 401, got %d", rec.Code)
	}

	authz, err := store.GetAPIClientAuth(context.Background(), token)
	if err != nil {
		t.Fatalf("GetAPIClientAuth: %v", err)
	}
	if authz == nil {
		t.Fatalf("expected token to resolve before revocation")
	}
	if _, err := store.UpdateAPIClient(context.Background(), authz.UserID, authz.ClientID, "revoked", "", authz.InstanceRole, authz.SiteRoles, nil, true); err != nil {
		t.Fatalf("UpdateAPIClient revoke: %v", err)
	}

	req = httptest.NewRequest(http.MethodPost, ts.URL+conf.MCPPath, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected revoked token to get 401, got %d", rec.Code)
	}
}

func TestMCPToolsListAndSiteOverview(t *testing.T) {
	store, site, token := setupMCPStore(t)
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
	if !hasTool(tools.Tools, "hitkeep_get_site_overview") {
		t.Fatalf("expected hitkeep_get_site_overview in tool list")
	}

	sites, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "hitkeep_list_sites"})
	if err != nil {
		t.Fatalf("CallTool list sites: %v", err)
	}
	if sites.IsError {
		t.Fatalf("expected list sites success, got %+v", sites.Content)
	}

	from := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	to := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "hitkeep_get_site_overview",
		Arguments: map[string]any{
			"site_id": site.ID.String(),
			"from":    from,
			"to":      to,
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected successful tool call, got %+v", res.Content)
	}

	var raw []byte
	raw, err = json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var output siteOverviewOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if output.Stats == nil || output.Stats.TotalPageviews != 1 {
		t.Fatalf("expected one pageview, got %+v", output.Stats)
	}
	if len(output.Stats.Goals) != 1 || output.Stats.Goals[0].GoalID == "" {
		t.Fatalf("expected string goal id in overview output, got %+v", output.Stats.Goals)
	}
}

func TestMCPToolDeniesUnscopedSite(t *testing.T) {
	store, _, token := setupMCPStore(t)
	ctx := context.Background()
	otherUserID, err := store.CreateUser(ctx, "other@mcp.test", "hash")
	if err != nil {
		t.Fatalf("CreateUser other: %v", err)
	}
	otherSite, err := store.CreateSite(ctx, otherUserID, "other-mcp.example.com")
	if err != nil {
		t.Fatalf("CreateSite other: %v", err)
	}

	conf := testMCPConfig(t, "")
	handler := NewHandler(conf, store, nil, nil, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	session := connectMCPClient(t, ts.URL+conf.MCPPath, token)
	defer session.Close()

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "hitkeep_get_site_overview",
		Arguments: map[string]any{
			"site_id": otherSite.ID.String(),
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected tool error for unscoped site")
	}
}

func TestMCPToolRejectsRangeBeyondConfiguredLimit(t *testing.T) {
	store, site, token := setupMCPStore(t)
	conf := testMCPConfig(t, "")
	conf.MCPMaxRangeDays = 1
	handler := NewHandler(conf, store, nil, nil, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	session := connectMCPClient(t, ts.URL+conf.MCPPath, token)
	defer session.Close()

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "hitkeep_get_event_names",
		Arguments: map[string]any{
			"site_id": site.ID.String(),
			"from":    time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339),
			"to":      time.Now().UTC().Format(time.RFC3339),
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected range limit tool error")
	}
}

func TestMCPToolRejectsComparisonRangeBeyondConfiguredLimit(t *testing.T) {
	store, site, token := setupMCPStore(t)
	conf := testMCPConfig(t, "")
	conf.MCPMaxRangeDays = 1
	handler := NewHandler(conf, store, nil, nil, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	session := connectMCPClient(t, ts.URL+conf.MCPPath, token)
	defer session.Close()

	now := time.Now().UTC()
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "hitkeep_get_site_overview",
		Arguments: map[string]any{
			"site_id":      site.ID.String(),
			"from":         now.Add(-time.Hour).Format(time.RFC3339),
			"to":           now.Format(time.RFC3339),
			"compare_from": now.Add(-48 * time.Hour).Format(time.RFC3339),
			"compare_to":   now.Format(time.RFC3339),
		},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected comparison range limit tool error")
	}
}

func TestMCPResourcesListAndReadHelp(t *testing.T) {
	store, _, token := setupMCPStore(t)
	conf := testMCPConfig(t, "")
	handler := NewHandler(conf, store, nil, nil, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	session := connectMCPClient(t, ts.URL+conf.MCPPath, token)
	defer session.Close()

	resources, err := session.ListResources(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	if !hasResource(resources.Resources, helpMCPURI) {
		t.Fatalf("expected %s in resource list", helpMCPURI)
	}

	templates, err := session.ListResourceTemplates(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListResourceTemplates: %v", err)
	}
	if len(templates.ResourceTemplates) == 0 {
		t.Fatalf("expected docs resource template")
	}

	help, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: helpMCPURI})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(help.Contents) != 1 || !strings.Contains(help.Contents[0].Text, "HitKeep MCP") {
		t.Fatalf("expected MCP help markdown, got %+v", help.Contents)
	}
}

func TestMCPDocsToolsFetchMarkdown(t *testing.T) {
	var sawMarkdownAccept bool
	docsTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Accept"), "text/markdown") {
			sawMarkdownAccept = true
		}
		switch r.URL.Path {
		case "/llms.txt":
			w.Header().Set("Content-Type", "text/markdown")
			_, _ = w.Write([]byte("- [API Clients](/guides/security/api-clients/): Create scoped tokens\n"))
		case "/guides/security/api-clients/":
			w.Header().Set("Content-Type", "text/markdown")
			_, _ = w.Write([]byte("# API Clients\n\nUse bearer tokens for integrations.\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer docsTS.Close()

	store, _, token := setupMCPStore(t)
	conf := testMCPConfig(t, docsTS.URL)
	handler := NewHandler(conf, store, nil, nil, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	session := connectMCPClient(t, ts.URL+conf.MCPPath, token)
	defer session.Close()

	search, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "hitkeep_search_docs",
		Arguments: map[string]any{"query": "api tokens"},
	})
	if err != nil {
		t.Fatalf("search docs: %v", err)
	}
	if search.IsError {
		t.Fatalf("expected docs search success")
	}

	doc, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "hitkeep_get_doc",
		Arguments: map[string]any{"path": "/guides/security/api-clients/"},
	})
	if err != nil {
		t.Fatalf("get doc: %v", err)
	}
	var raw []byte
	raw, err = json.Marshal(doc.StructuredContent)
	if err != nil {
		t.Fatalf("marshal doc output: %v", err)
	}
	var output docOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		t.Fatalf("unmarshal doc output: %v", err)
	}
	if !strings.Contains(output.Markdown, "# API Clients") {
		t.Fatalf("expected markdown doc, got %q", output.Markdown)
	}
	if !sawMarkdownAccept {
		t.Fatalf("expected docs client to request text/markdown")
	}
}

func TestDocsClientBlocksOtherOriginsAndCachesMarkdown(t *testing.T) {
	requests := 0
	docsTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if !strings.Contains(r.Header.Get("Accept"), "text/markdown") {
			t.Errorf("expected Accept header to include text/markdown, got %q", r.Header.Get("Accept"))
		}
		if r.URL.Path != "/guides/integrations/mcp/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/markdown")
		_, _ = w.Write([]byte("# MCP Integration\n\nUse the official server.\n"))
	}))
	defer docsTS.Close()

	client := newDocsClient(docsTS.URL, time.Hour)
	for i := range 2 {
		page, err := client.GetMarkdown(context.Background(), "/guides/integrations/mcp")
		if err != nil {
			t.Fatalf("GetMarkdown attempt %d: %v", i+1, err)
		}
		if page.Path != "/guides/integrations/mcp/" || !strings.Contains(page.Markdown, "# MCP Integration") {
			t.Fatalf("unexpected page: %+v", page)
		}
	}
	if requests != 1 {
		t.Fatalf("expected cached second docs fetch, got %d requests", requests)
	}
	if _, err := client.GetMarkdown(context.Background(), "https://example.com/guides/integrations/mcp/"); err == nil {
		t.Fatalf("expected other docs origin to be rejected")
	}
	if _, err := client.GetMarkdown(context.Background(), "/guides/%2e%2e/secret"); err == nil {
		t.Fatalf("expected encoded parent traversal to be rejected")
	}
}

func TestDocsClientCoalescesConcurrentFetches(t *testing.T) {
	var requests atomic.Int32
	docsTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "text/markdown")
		_, _ = w.Write([]byte("# MCP Integration\n"))
	}))
	defer docsTS.Close()

	client := newDocsClient(docsTS.URL, time.Hour)
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for range 20 {
		wg.Go(func() {
			_, err := client.GetMarkdown(context.Background(), "/guides/integrations/mcp/")
			errs <- err
		})
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("GetMarkdown: %v", err)
		}
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("expected concurrent requests to coalesce to one fetch, got %d", got)
	}
}

func TestDocsClientCapsCachedPages(t *testing.T) {
	docsTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown")
		_, _ = w.Write([]byte("# " + r.URL.Path + "\n"))
	}))
	defer docsTS.Close()

	client := newDocsClient(docsTS.URL, time.Hour)
	for i := range maxDocCacheEntries + 5 {
		if _, err := client.GetMarkdown(context.Background(), "/guides/page-"+strconv.Itoa(i)+"/"); err != nil {
			t.Fatalf("GetMarkdown page %d: %v", i, err)
		}
	}

	if got := client.pages.Len(); got != maxDocCacheEntries {
		t.Fatalf("expected docs cache to cap at %d entries, got %d", maxDocCacheEntries, got)
	}
}

func setupMCPStore(t *testing.T) (*database.Store, *api.Site, string) {
	t.Helper()
	ctx := context.Background()
	store := database.NewStore(filepath.Join(t.TempDir(), "hitkeep.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	userID, err := store.CreateUser(ctx, "mcp@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "mcp.example.com")
	if err != nil {
		t.Fatalf("CreateSite: %v", err)
	}
	sessionID := uuid.New()
	if err := store.CreateHit(ctx, &api.Hit{
		SiteID:    site.ID,
		SessionID: sessionID,
		PageID:    uuid.New(),
		Timestamp: time.Now().UTC(),
		Path:      "/",
	}); err != nil {
		t.Fatalf("CreateHit: %v", err)
	}
	if err := store.CreateGoal(ctx, &api.Goal{
		SiteID: site.ID,
		Name:   "Homepage Visit",
		Type:   "path",
		Value:  "/",
	}); err != nil {
		t.Fatalf("CreateGoal: %v", err)
	}
	_, token, err := store.CreateAPIClient(ctx, userID, "mcp-reader", "", authcore.InstanceUser, map[uuid.UUID]authcore.SiteRole{
		site.ID: authcore.SiteViewer,
	}, nil)
	if err != nil {
		t.Fatalf("CreateAPIClient: %v", err)
	}
	return store, site, token
}

func testMCPConfig(t *testing.T, docsURL string) *config.Config {
	t.Helper()
	conf := &config.Config{
		ApiRateLimit:        1000,
		ApiBurst:            1000,
		DataPath:            t.TempDir(),
		MCPDocsCacheMinutes: 60,
		MCPDocsEnabled:      true,
		MCPDocsURL:          "https://hitkeep.com",
		MCPEnabled:          true,
		MCPMaxRangeDays:     366,
		MCPPath:             "/mcp",
		TrustedProxies:      "*",
		Version:             "test",
	}
	if docsURL != "" {
		conf.MCPDocsURL = docsURL
	}
	return conf
}

func connectMCPClient(t *testing.T, endpoint, token string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:             endpoint,
		DisableStandaloneSSE: true,
		HTTPClient: &http.Client{Transport: authRoundTripper{
			token: token,
			base:  http.DefaultTransport,
		}},
	}, nil)
	if err != nil {
		t.Fatalf("connect MCP client: %v", err)
	}
	return session
}

type authRoundTripper struct {
	token string
	base  http.RoundTripper
}

func (t authRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(req)
}

func hasTool(tools []*mcp.Tool, name string) bool {
	for _, tool := range tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func hasResource(resources []*mcp.Resource, uri string) bool {
	for _, resource := range resources {
		if resource.URI == uri {
			return true
		}
	}
	return false
}
