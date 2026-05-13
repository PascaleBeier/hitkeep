package mcpserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"slices"
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
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestMCPServerInitializesWithConfiguredPublicHostBehindLoopback(t *testing.T) {
	store, _, token := setupMCPStore(t)
	conf := testMCPConfig(t, "")
	conf.PublicURL = "https://analytics.example.com"
	handler := NewHandler(conf, store, nil, nil, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	req := newMCPInitializeHTTPRequest(t, ts.URL+conf.MCPPath, token)
	req.Host = "analytics.example.com"

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("initialize request: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected public host initialize to succeed, got %d %q", resp.StatusCode, string(body))
	}
}

func TestMCPServerReturnsUnauthorizedForValidPublicHostWithoutBearer(t *testing.T) {
	store, _, _ := setupMCPStore(t)
	conf := testMCPConfig(t, "")
	conf.PublicURL = "https://analytics.example.com"
	handler := NewHandler(conf, store, nil, nil, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	req := newMCPInitializeHTTPRequest(t, ts.URL+conf.MCPPath, "")
	req.Host = "analytics.example.com:443"

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("initialize request: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected valid public host without bearer to get 401, got %d %q", resp.StatusCode, string(body))
	}
}

func TestMCPServerRejectsUnexpectedHostBeforeAuth(t *testing.T) {
	store, _, token := setupMCPStore(t)
	conf := testMCPConfig(t, "")
	conf.PublicURL = "https://analytics.example.com"
	handler := NewHandler(conf, store, nil, nil, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	req := newMCPInitializeHTTPRequest(t, ts.URL+conf.MCPPath, token)
	req.Host = "unexpected.example.com"

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("initialize request: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected unexpected host to get 403, got %d %q", resp.StatusCode, string(body))
	}
}

func TestMCPRequestHostUsesForwardedHostFromTrustedProxy(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8080/mcp", nil)
	req.Host = "127.0.0.1:8080"
	req.RemoteAddr = "10.0.0.5:44321"
	req.Header.Set("X-Forwarded-Host", "analytics.example.com")

	host, allowLoopbackHost := mcpRequestHost(req, []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")})
	if host != "analytics.example.com" {
		t.Fatalf("expected trusted forwarded host, got %q", host)
	}
	if allowLoopbackHost {
		t.Fatal("expected trusted proxy forwarded host to disable loopback host allowance")
	}
}

func TestMCPRequestHostIgnoresForwardedHostFromUntrustedProxy(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8080/mcp", nil)
	req.Host = "127.0.0.1:8080"
	req.RemoteAddr = "198.51.100.10:44321"
	req.Header.Set("X-Forwarded-Host", "analytics.example.com")

	host, allowLoopbackHost := mcpRequestHost(req, []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")})
	if host != "127.0.0.1:8080" {
		t.Fatalf("expected direct request host, got %q", host)
	}
	if allowLoopbackHost {
		t.Fatal("expected untrusted external remote to disable loopback host allowance")
	}
}

func TestMCPRequestHostParsesStandardForwardedHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8080/mcp", nil)
	req.Host = "127.0.0.1:8080"
	req.RemoteAddr = "10.0.0.5:44321"
	req.Header.Set("Forwarded", `for=203.0.113.10;proto=https;host="analytics.example.com:443"`)

	host, allowLoopbackHost := mcpRequestHost(req, []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")})
	if host != "analytics.example.com:443" {
		t.Fatalf("expected standard Forwarded host, got %q", host)
	}
	if allowLoopbackHost {
		t.Fatal("expected trusted proxy forwarded host to disable loopback host allowance")
	}
}

func TestMCPRequestHostIgnoresForwardedHostWhenTrustedProxiesTrustAll(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://unexpected.example.com/mcp", nil)
	req.Host = "unexpected.example.com"
	req.RemoteAddr = "198.51.100.10:44321"
	req.Header.Set("X-Forwarded-Host", "analytics.example.com")

	host, allowLoopbackHost := mcpRequestHost(req, []netip.Prefix{
		netip.MustParsePrefix("0.0.0.0/0"),
		netip.MustParsePrefix("::/0"),
	})
	if host != "unexpected.example.com" {
		t.Fatalf("expected direct request host for trust-all proxies, got %q", host)
	}
	if allowLoopbackHost {
		t.Fatal("expected external trust-all remote to disable loopback host allowance")
	}
}

func TestMCPRequestHostDisablesLoopbackAllowanceForProxyWithoutForwardedHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8080/mcp", nil)
	req.Host = "127.0.0.1:8080"
	req.RemoteAddr = "10.0.0.5:44321"

	host, allowLoopbackHost := mcpRequestHost(req, []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")})
	if host != "127.0.0.1:8080" {
		t.Fatalf("expected backend request host, got %q", host)
	}
	if allowLoopbackHost {
		t.Fatal("expected proxy request without forwarded host to disable loopback host allowance")
	}
}

func TestMCPHostValidationNormalizesPublicURLAndLoopbackHosts(t *testing.T) {
	tests := []struct {
		name              string
		publicURL         string
		requestHost       string
		allowLoopbackHost bool
		want              bool
	}{
		{
			name:        "https public URL accepts omitted default port",
			publicURL:   "https://analytics.example.com",
			requestHost: "analytics.example.com",
			want:        true,
		},
		{
			name:        "https public URL accepts explicit default port",
			publicURL:   "https://analytics.example.com",
			requestHost: "analytics.example.com:443",
			want:        true,
		},
		{
			name:        "public host comparison ignores case and trailing dot",
			publicURL:   "https://analytics.example.com",
			requestHost: "ANALYTICS.EXAMPLE.COM.",
			want:        true,
		},
		{
			name:        "https public URL rejects wrong default port",
			publicURL:   "https://analytics.example.com",
			requestHost: "analytics.example.com:80",
			want:        false,
		},
		{
			name:        "http public URL accepts explicit default port",
			publicURL:   "http://analytics.example.com",
			requestHost: "analytics.example.com:80",
			want:        true,
		},
		{
			name:        "non-default public URL port requires request port",
			publicURL:   "http://analytics.example.com:8080",
			requestHost: "analytics.example.com",
			want:        false,
		},
		{
			name:        "non-default public URL port accepts matching request port",
			publicURL:   "http://analytics.example.com:8080",
			requestHost: "analytics.example.com:8080",
			want:        true,
		},
		{
			name:              "localhost stays accepted for development",
			publicURL:         "https://analytics.example.com",
			requestHost:       "localhost:8080",
			allowLoopbackHost: true,
			want:              true,
		},
		{
			name:              "ipv4 loopback stays accepted for development",
			publicURL:         "https://analytics.example.com",
			requestHost:       "127.0.0.1:8080",
			allowLoopbackHost: true,
			want:              true,
		},
		{
			name:              "ipv6 loopback stays accepted for development",
			publicURL:         "https://analytics.example.com",
			requestHost:       "[::1]:8080",
			allowLoopbackHost: true,
			want:              true,
		},
		{
			name:        "loopback host is rejected without local request allowance",
			publicURL:   "https://analytics.example.com",
			requestHost: "127.0.0.1:8080",
			want:        false,
		},
		{
			name:        "localhost suffix is not loopback",
			publicURL:   "https://analytics.example.com",
			requestHost: "localhost.evil.example",
			want:        false,
		},
		{
			name:        "unexpected public host is rejected",
			publicURL:   "https://analytics.example.com",
			requestHost: "other.example.com",
			want:        false,
		},
		{
			name:        "malformed trailing bracket is rejected",
			publicURL:   "https://analytics.example.com",
			requestHost: "analytics.example.com]",
			want:        false,
		},
		{
			name:        "malformed leading bracket is rejected",
			publicURL:   "https://analytics.example.com",
			requestHost: "[analytics.example.com",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isAllowedMCPHost(tt.requestHost, tt.publicURL, tt.allowLoopbackHost); got != tt.want {
				t.Fatalf("isAllowedMCPHost(%q, %q) = %v, want %v", tt.requestHost, tt.publicURL, got, tt.want)
			}
		})
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
	req.RemoteAddr = "127.0.0.1:12345"
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
	req.RemoteAddr = "127.0.0.1:12345"
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

func TestMCPOpportunitiesReturnsSafeFinalData(t *testing.T) {
	store, site, token := setupMCPStore(t)
	ctx := context.Background()
	seedMCPOpportunity(t, ctx, store, site)

	conf := testMCPConfig(t, "")
	handler := NewHandler(conf, store, nil, nil, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	session := connectMCPClient(t, ts.URL+conf.MCPPath, token)
	defer session.Close()

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "hitkeep_get_opportunities",
		Arguments: map[string]any{
			"site_id": site.ID.String(),
			"status":  "new",
			"limit":   10,
		},
	})
	requireSuccessfulMCPTool(t, res, err)
	raw := marshalMCPStructuredContent(t, res)
	requireNoMCPOpportunitySecrets(t, raw)
	requireMCPOpportunityOutput(t, raw, site.ID)
}

func TestMCPOpportunitiesReturnsEmptyArrayWhenNoRowsExist(t *testing.T) {
	store, site, token := setupMCPStore(t)
	conf := testMCPConfig(t, "")
	handler := NewHandler(conf, store, nil, nil, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	session := connectMCPClient(t, ts.URL+conf.MCPPath, token)
	defer session.Close()

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "hitkeep_get_opportunities",
		Arguments: map[string]any{
			"site_id": site.ID.String(),
			"status":  "all",
			"limit":   10,
		},
	})
	requireSuccessfulMCPTool(t, res, err)
	raw := marshalMCPStructuredContent(t, res)
	var output map[string]any
	if err := json.Unmarshal([]byte(raw), &output); err != nil {
		t.Fatalf("unmarshal opportunities output: %v", err)
	}
	opportunities, ok := output["opportunities"].([]any)
	if !ok {
		t.Fatalf("expected opportunities to be an array, got %#v in %s", output["opportunities"], raw)
	}
	if len(opportunities) != 0 {
		t.Fatalf("expected no opportunities, got %#v", opportunities)
	}
}

func TestMCPOpportunitiesExposeOnlyCitedEvidence(t *testing.T) {
	store, site, token := setupMCPStore(t)
	ctx := context.Background()
	teamID, err := store.GetSiteTenantID(ctx, site.ID)
	if err != nil {
		t.Fatalf("GetSiteTenantID: %v", err)
	}
	input := mcpOpportunityInput(teamID, site.ID, uuid.New())
	input.Evidence = append(input.Evidence, api.OpportunityEvidence{
		ID:       "uncited_internal_signal",
		LabelKey: "opportunities.evidence.fixture",
		Value:    "do-not-show-in-mcp",
	})
	input.CitedEvidenceIDs = []string{"checkout_starts"}
	if _, err := store.UpsertOpportunities(ctx, []database.OpportunityInput{input}); err != nil {
		t.Fatalf("UpsertOpportunities: %v", err)
	}

	conf := testMCPConfig(t, "")
	handler := NewHandler(conf, store, nil, nil, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	session := connectMCPClient(t, ts.URL+conf.MCPPath, token)
	defer session.Close()

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "hitkeep_get_opportunities",
		Arguments: map[string]any{
			"site_id": site.ID.String(),
		},
	})
	requireSuccessfulMCPTool(t, res, err)
	raw := marshalMCPStructuredContent(t, res)
	if strings.Contains(raw, "uncited_internal_signal") || strings.Contains(raw, "do-not-show-in-mcp") {
		t.Fatalf("MCP Opportunities leaked uncited evidence: %s", raw)
	}

	var output opportunitiesOutput
	if err := json.Unmarshal([]byte(raw), &output); err != nil {
		t.Fatalf("unmarshal opportunities output: %v", err)
	}
	if len(output.Opportunities) != 1 {
		t.Fatalf("expected one opportunity, got %+v", output.Opportunities)
	}
	if len(output.Opportunities[0].Evidence) != 1 || output.Opportunities[0].Evidence[0].ID != "checkout_starts" {
		t.Fatalf("expected only cited evidence in MCP output, got %+v", output.Opportunities[0].Evidence)
	}
}

func TestMCPOpportunitiesReturnsRankedFinalData(t *testing.T) {
	store, site, token := setupMCPStore(t)
	ctx := context.Background()
	teamID, err := store.GetSiteTenantID(ctx, site.ID)
	if err != nil {
		t.Fatalf("GetSiteTenantID: %v", err)
	}
	runID := uuid.New()
	lowActive := mcpOpportunityInput(teamID, site.ID, runID)
	lowActive.ID = uuid.New()
	lowActive.Status = "new"
	lowActive.Score = 60
	lowActive.ScoreBreakdown = api.OpportunityScoreBreakdown{Impact: 55, Actionability: 70, EvidenceFit: 70, Total: 60}
	highActive := mcpOpportunityInput(teamID, site.ID, runID)
	highActive.ID = uuid.New()
	highActive.Status = "saved"
	highActive.Score = 88
	highActive.ScoreBreakdown = api.OpportunityScoreBreakdown{Impact: 85, Actionability: 90, EvidenceFit: 90, Total: 88}
	done := mcpOpportunityInput(teamID, site.ID, runID)
	done.ID = uuid.New()
	done.Status = "done"
	done.Score = 99
	done.ScoreBreakdown = api.OpportunityScoreBreakdown{Impact: 99, Actionability: 99, EvidenceFit: 99, Total: 99}
	if _, err := store.UpsertOpportunities(ctx, []database.OpportunityInput{lowActive, done, highActive}); err != nil {
		t.Fatalf("UpsertOpportunities: %v", err)
	}

	conf := testMCPConfig(t, "")
	handler := NewHandler(conf, store, nil, nil, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	session := connectMCPClient(t, ts.URL+conf.MCPPath, token)
	defer session.Close()

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "hitkeep_get_opportunities",
		Arguments: map[string]any{
			"site_id": site.ID.String(),
			"status":  "all",
			"limit":   10,
		},
	})
	requireSuccessfulMCPTool(t, res, err)
	raw := marshalMCPStructuredContent(t, res)
	var output opportunitiesOutput
	if err := json.Unmarshal([]byte(raw), &output); err != nil {
		t.Fatalf("unmarshal opportunities output: %v", err)
	}
	got := mcpOpportunityIDs(output.Opportunities)
	want := []string{highActive.ID.String(), lowActive.ID.String(), done.ID.String()}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("expected ranked opportunities %v, got %v", want, got)
	}
}

func seedMCPOpportunity(t *testing.T, ctx context.Context, store *database.Store, site *api.Site) {
	t.Helper()
	teamID, err := store.GetSiteTenantID(ctx, site.ID)
	if err != nil {
		t.Fatalf("GetSiteTenantID: %v", err)
	}
	requireMCPAIRunRejectsRawPayload(t, ctx, store, teamID, site.ID)

	runID, err := store.AppendAIRun(ctx, database.AIRunParams{
		TeamID:          teamID,
		SiteID:          site.ID,
		Feature:         "opportunities",
		Provider:        "openai",
		Model:           "gpt-test",
		TemplateVersion: "opportunities.v1",
		EvidenceIDs:     []string{"checkout_starts"},
		InputHash:       "input-hash",
		OutputHash:      "output-hash",
		OutputJSON:      `{"title_key":"opportunities.catalog.checkout_conversion.title"}`,
		Status:          "success",
		CreatedAt:       time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("AppendAIRun: %v", err)
	}
	_, err = store.UpsertOpportunities(ctx, []database.OpportunityInput{mcpOpportunityInput(teamID, site.ID, runID)})
	if err != nil {
		t.Fatalf("UpsertOpportunities: %v", err)
	}
}

func requireMCPAIRunRejectsRawPayload(t *testing.T, ctx context.Context, store *database.Store, teamID, siteID uuid.UUID) {
	t.Helper()
	_, err := store.AppendAIRun(ctx, database.AIRunParams{
		TeamID:          teamID,
		SiteID:          siteID,
		Feature:         "opportunities",
		Provider:        "openai",
		Model:           "gpt-test",
		TemplateVersion: "opportunities.v1",
		OutputJSON:      `{"raw_provider_response":"do not persist","raw_prompt":"do not persist"}`,
		Status:          "success",
		CreatedAt:       time.Now().UTC(),
	})
	if err == nil || !strings.Contains(err.Error(), "raw") {
		t.Fatalf("expected raw AI payload to be rejected before MCP exposure, got %v", err)
	}
}

func mcpOpportunityInput(teamID, siteID, runID uuid.UUID) database.OpportunityInput {
	return database.OpportunityInput{
		TeamID:          teamID,
		SiteID:          siteID,
		Kind:            "conversion",
		TypeKey:         "opportunities.catalog.checkout_conversion.type",
		TitleKey:        "opportunities.catalog.checkout_conversion.title",
		SummaryKey:      "opportunities.catalog.checkout_conversion.summary",
		ActionKey:       "opportunities.catalog.checkout_conversion.action",
		DigestKey:       "opportunities.catalog.checkout_conversion.digest",
		CopyParams:      map[string]any{"conversion_rate": 1.8, "checkout_starts": 80},
		ImpactValue:     "+12%",
		ImpactLabelKey:  "opportunities.impact.traffic",
		Confidence:      "high",
		Score:           92,
		ScoreBreakdown:  api.OpportunityScoreBreakdown{Sample: 90, Impact: 88, Urgency: 70, EvidenceFit: 99, Total: 92},
		Status:          "new",
		RouteLabelKey:   "opportunities.routes.ecommerce",
		RouteParams:     map[string]any{"tab": "checkout"},
		RouteIcon:       "pi pi-shopping-cart",
		DetectorVersion: "checkout-conversion@v1",
		Evidence: []api.OpportunityEvidence{{
			ID:           "checkout_starts",
			LabelKey:     "opportunities.evidence.checkout_starts",
			Value:        "80",
			DetailKey:    "opportunities.evidence.checkout_starts.detail",
			DetailParams: map[string]any{"completed": 20},
		}},
		CitedEvidenceIDs: []string{"checkout_starts"},
		AIRunID:          runID,
		GeneratedAt:      time.Now().UTC(),
	}
}

func requireNoMCPOpportunitySecrets(t *testing.T, raw string) {
	t.Helper()
	for _, forbidden := range []string{"do not expose", "raw_provider_response", "output_json", "raw_prompt", "provider_error_body", "team_id", "ai_run_id"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("MCP Opportunities leaked forbidden field %q in %s", forbidden, raw)
		}
	}
}

func requireMCPOpportunityOutput(t *testing.T, raw string, siteID uuid.UUID) {
	t.Helper()
	var output opportunitiesOutput
	if err := json.Unmarshal([]byte(raw), &output); err != nil {
		t.Fatalf("unmarshal opportunities output: %v", err)
	}
	if output.SiteID != siteID.String() {
		t.Fatalf("unexpected site id: %+v", output)
	}
	if len(output.Opportunities) != 1 {
		t.Fatalf("expected one opportunity, got %+v", output.Opportunities)
	}
	opp := output.Opportunities[0]
	if opp.TitleKey != "opportunities.catalog.checkout_conversion.title" {
		t.Fatalf("expected localized title key, got %+v", opp)
	}
	if len(opp.CitedEvidenceIDs) != 1 || opp.CitedEvidenceIDs[0] != "checkout_starts" {
		t.Fatalf("expected cited evidence ids, got %+v", opp.CitedEvidenceIDs)
	}
	if opp.CopyParams["conversion_rate"] == nil {
		t.Fatalf("expected copy params for translation interpolation, got %+v", opp.CopyParams)
	}
	if opp.ScoreBreakdown.Total != opp.Score || opp.ScoreBreakdown.EvidenceFit == 0 {
		t.Fatalf("expected safe score breakdown, got %+v", opp.ScoreBreakdown)
	}
}

func mcpOpportunityIDs(opportunities []mcpOpportunity) []string {
	out := make([]string, 0, len(opportunities))
	for _, opportunity := range opportunities {
		out = append(out, opportunity.ID)
	}
	return out
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

func TestMCPSearchConsoleStatusReportsMappedSite(t *testing.T) {
	store, site, token := setupMCPStore(t)
	teamID := seedSearchConsoleMapping(t, store, site)
	seedSearchConsoleSyncState(t, store, site.ID, teamID, "succeeded")

	conf := testMCPConfig(t, "")
	handler := NewHandler(conf, store, nil, nil, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	session := connectMCPClient(t, ts.URL+conf.MCPPath, token)
	defer session.Close()

	raw, output := callSearchConsoleStatus(t, session, site.ID)
	requireNoSearchConsoleSecrets(t, raw)
	requireMappedSearchConsoleStatus(t, output, site, teamID, "ready")
}

func TestMCPSearchConsoleReturnsOverviewAndSeriesFromImportedFacts(t *testing.T) {
	store, tenantStores, site, token := setupMCPTenantSearchConsoleStore(t)
	defer tenantStores.Close()
	seedSearchConsoleMapping(t, store, site)
	sharedFact := searchConsoleFact(site, 99, 990)
	if err := store.UpsertSearchConsoleFact(context.Background(), sharedFact); err != nil {
		t.Fatalf("seed shared Search Console fact: %v", err)
	}
	tenantStore, _, err := tenantStores.ResolveSiteStore(context.Background(), site.ID)
	if err != nil {
		t.Fatalf("ResolveSiteStore: %v", err)
	}
	tenantFact := sharedFact
	tenantFact.Query = "tenant scoped"
	tenantFact.Clicks = 4
	tenantFact.Impressions = 40
	tenantFact.CTR = 0.1
	if err := tenantStore.UpsertSearchConsoleFact(context.Background(), tenantFact); err != nil {
		t.Fatalf("seed tenant Search Console fact: %v", err)
	}

	conf := testMCPConfig(t, "")
	handler := NewHandler(conf, store, tenantStores, nil, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	session := connectMCPClient(t, ts.URL+conf.MCPPath, token)
	defer session.Close()

	output := callSearchConsoleReport(t, session, map[string]any{
		"site_id": site.ID.String(),
		"from":    "2026-05-01T00:00:00Z",
		"to":      "2026-05-03T00:00:00Z",
	})
	requireTenantSearchConsoleDefaultReport(t, output)
}

func TestMCPSearchConsoleReturnsExplicitSectionsWithFiltersAndCappedLimit(t *testing.T) {
	store, tenantStores, site, token := setupMCPTenantSearchConsoleStore(t)
	defer tenantStores.Close()
	seedSearchConsoleMapping(t, store, site)
	tenantStore, _, err := tenantStores.ResolveSiteStore(context.Background(), site.ID)
	if err != nil {
		t.Fatalf("ResolveSiteStore: %v", err)
	}
	for i := range 60 {
		fact := searchConsoleFact(site, i+1, (i+1)*10)
		fact.Query = "filtered query " + strconv.Itoa(i)
		fact.Page = "https://" + site.Domain + "/landing?utm=test"
		fact.Country = "US"
		fact.Device = "desktop"
		if err := tenantStore.UpsertSearchConsoleFact(context.Background(), fact); err != nil {
			t.Fatalf("seed tenant Search Console fact %d: %v", i, err)
		}
	}

	conf := testMCPConfig(t, "")
	handler := NewHandler(conf, store, tenantStores, nil, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	session := connectMCPClient(t, ts.URL+conf.MCPPath, token)
	defer session.Close()

	output := callSearchConsoleReport(t, session, map[string]any{
		"site_id":  site.ID.String(),
		"from":     "2026-05-01T00:00:00Z",
		"to":       "2026-05-03T00:00:00Z",
		"sections": []string{"queries", "pages", "country", "device"},
		"path":     "/landing",
		"country":  "US",
		"device":   "desktop",
		"limit":    99,
	})
	requireExplicitSearchConsoleSections(t, output)
}

func TestMCPSearchConsoleDeniesUnscopedSite(t *testing.T) {
	store, _, token := setupMCPStore(t)
	ctx := context.Background()
	otherUserID, err := store.CreateUser(ctx, "other-gsc@mcp.test", "hash")
	if err != nil {
		t.Fatalf("CreateUser other: %v", err)
	}
	otherSite, err := store.CreateSite(ctx, otherUserID, "other-gsc-mcp.example.com")
	if err != nil {
		t.Fatalf("CreateSite other: %v", err)
	}
	seedSearchConsoleMapping(t, store, otherSite)

	conf := testMCPConfig(t, "")
	handler := NewHandler(conf, store, nil, nil, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	session := connectMCPClient(t, ts.URL+conf.MCPPath, token)
	defer session.Close()

	for _, name := range []string{"hitkeep_get_search_console_status", "hitkeep_get_search_console"} {
		res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
			Name:      name,
			Arguments: map[string]any{"site_id": otherSite.ID.String()},
		})
		if err != nil {
			t.Fatalf("CallTool %s: %v", name, err)
		}
		if !res.IsError {
			t.Fatalf("expected tool error for unscoped site with %s", name)
		}
	}
}

func TestMCPSearchConsoleReportRequiresMappedSite(t *testing.T) {
	store, site, token := setupMCPStore(t)
	conf := testMCPConfig(t, "")
	handler := NewHandler(conf, store, nil, nil, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	session := connectMCPClient(t, ts.URL+conf.MCPPath, token)
	defer session.Close()

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "hitkeep_get_search_console",
		Arguments: map[string]any{"site_id": site.ID.String()},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected unmapped site tool error")
	}
	if !strings.Contains(mcpToolErrorText(res), "Search Console property is not mapped") {
		t.Fatalf("expected clear unmapped error, got %+v", res.Content)
	}
}

func TestMCPSearchConsoleRejectsRangeBeyondConfiguredLimit(t *testing.T) {
	store, site, token := setupMCPStore(t)
	seedSearchConsoleMapping(t, store, site)
	conf := testMCPConfig(t, "")
	conf.MCPMaxRangeDays = 1
	handler := NewHandler(conf, store, nil, nil, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	session := connectMCPClient(t, ts.URL+conf.MCPPath, token)
	defer session.Close()

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "hitkeep_get_search_console",
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

func TestMCPSearchConsoleReturnsImportedDataWithSyncWarnings(t *testing.T) {
	store, tenantStores, site, token := setupMCPTenantSearchConsoleStore(t)
	defer tenantStores.Close()
	teamID := seedSearchConsoleMapping(t, store, site)
	importedStart := time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)
	importedEnd := time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)
	lastAttempt := time.Date(2026, 5, 4, 9, 0, 0, 0, time.UTC)
	if err := store.UpsertGoogleSearchConsoleSyncState(context.Background(), database.GoogleSearchConsoleSyncStateInput{
		SiteID:            site.ID,
		TeamID:            teamID,
		State:             "needs_attention",
		ImportedStartDate: &importedStart,
		ImportedEndDate:   &importedEnd,
		LastAttemptAt:     &lastAttempt,
		LastErrorCategory: "authorization_revoked",
	}); err != nil {
		t.Fatalf("UpsertGoogleSearchConsoleSyncState: %v", err)
	}
	tenantStore, _, err := tenantStores.ResolveSiteStore(context.Background(), site.ID)
	if err != nil {
		t.Fatalf("ResolveSiteStore: %v", err)
	}
	if err := tenantStore.UpsertSearchConsoleFact(context.Background(), searchConsoleFact(site, 7, 70)); err != nil {
		t.Fatalf("seed tenant Search Console fact: %v", err)
	}

	conf := testMCPConfig(t, "")
	handler := NewHandler(conf, store, tenantStores, nil, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	session := connectMCPClient(t, ts.URL+conf.MCPPath, token)
	defer session.Close()

	output := callSearchConsoleReport(t, session, map[string]any{
		"site_id":  site.ID.String(),
		"from":     "2026-05-01T00:00:00Z",
		"to":       "2026-05-04T00:00:00Z",
		"sections": []string{"overview"},
	})
	requireSearchConsoleSyncWarnings(t, output)
}

func TestMCPSearchConsoleWarnsWhenSyncFailedButImportedDataExists(t *testing.T) {
	store, tenantStores, site, token := setupMCPTenantSearchConsoleStore(t)
	defer tenantStores.Close()
	teamID := seedSearchConsoleMapping(t, store, site)
	importedStart := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	importedEnd := time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)
	lastAttempt := time.Date(2026, 5, 4, 9, 0, 0, 0, time.UTC)
	if err := store.UpsertGoogleSearchConsoleSyncState(context.Background(), database.GoogleSearchConsoleSyncStateInput{
		SiteID:            site.ID,
		TeamID:            teamID,
		State:             "failed",
		ImportedStartDate: &importedStart,
		ImportedEndDate:   &importedEnd,
		LastAttemptAt:     &lastAttempt,
		LastErrorCategory: "quota_exceeded",
	}); err != nil {
		t.Fatalf("UpsertGoogleSearchConsoleSyncState: %v", err)
	}
	tenantStore, _, err := tenantStores.ResolveSiteStore(context.Background(), site.ID)
	if err != nil {
		t.Fatalf("ResolveSiteStore: %v", err)
	}
	if err := tenantStore.UpsertSearchConsoleFact(context.Background(), searchConsoleFact(site, 7, 70)); err != nil {
		t.Fatalf("seed tenant Search Console fact: %v", err)
	}

	conf := testMCPConfig(t, "")
	handler := NewHandler(conf, store, tenantStores, nil, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	session := connectMCPClient(t, ts.URL+conf.MCPPath, token)
	defer session.Close()

	_, status := callSearchConsoleStatus(t, session, site.ID)
	if status.Reason != "failed" || status.NeedsAttention {
		t.Fatalf("expected failed status without needs-attention flag, got %+v", status)
	}

	output := callSearchConsoleReport(t, session, map[string]any{
		"site_id":  site.ID.String(),
		"from":     "2026-05-01T00:00:00Z",
		"to":       "2026-05-04T00:00:00Z",
		"sections": []string{"overview"},
	})
	if output.Overview == nil || output.Overview.Clicks != 7 {
		t.Fatalf("expected imported data despite failed sync, got %+v", output.Overview)
	}
	if !hasWarning(output.Warnings, "search_console_sync_failed") {
		t.Fatalf("expected failed sync warning in %+v", output.Warnings)
	}
	if output.SyncStatus == nil || output.SyncStatus.State != "failed" || output.SyncStatus.LastErrorCategory != "quota_exceeded" {
		t.Fatalf("expected failed sync status, got %+v", output.SyncStatus)
	}
}

func TestMCPSearchConsoleReturnsEmptyWarningsArrayWhenHealthy(t *testing.T) {
	store, tenantStores, site, token := setupMCPTenantSearchConsoleStore(t)
	defer tenantStores.Close()
	teamID := seedSearchConsoleMapping(t, store, site)
	seedSearchConsoleSyncState(t, store, site.ID, teamID, "succeeded")
	tenantStore, _, err := tenantStores.ResolveSiteStore(context.Background(), site.ID)
	if err != nil {
		t.Fatalf("ResolveSiteStore: %v", err)
	}
	if err := tenantStore.UpsertSearchConsoleFact(context.Background(), searchConsoleFact(site, 7, 70)); err != nil {
		t.Fatalf("seed tenant Search Console fact: %v", err)
	}

	conf := testMCPConfig(t, "")
	handler := NewHandler(conf, store, tenantStores, nil, nil)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	session := connectMCPClient(t, ts.URL+conf.MCPPath, token)
	defer session.Close()

	raw, output := callSearchConsoleReportRaw(t, session, map[string]any{
		"site_id":  site.ID.String(),
		"from":     "2026-04-02T00:00:00Z",
		"to":       "2026-04-03T00:00:00Z",
		"sections": []string{"overview"},
	})
	if len(output.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %+v", output.Warnings)
	}
	if !strings.Contains(raw, `"warnings":[]`) {
		t.Fatalf("expected empty warnings array in raw output, got %s", raw)
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
	if !strings.Contains(help.Contents[0].Text, "Search Console") || !strings.Contains(help.Contents[0].Text, "imported") {
		t.Fatalf("expected Search Console imported-data guidance, got %q", help.Contents[0].Text)
	}
	if !strings.Contains(help.Contents[0].Text, "Opportunities") || !strings.Contains(help.Contents[0].Text, "localization keys") {
		t.Fatalf("expected Opportunities safe-data guidance, got %q", help.Contents[0].Text)
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

func setupMCPTenantSearchConsoleStore(t *testing.T) (*database.Store, *database.TenantStoreManager, *api.Site, string) {
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
	userID, err := store.CreateUser(ctx, "tenant-mcp@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	team, err := store.CreateTenant(ctx, userID, "MCP Tenant", "")
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	if err := store.SetActiveTenantID(ctx, userID, team.ID); err != nil {
		t.Fatalf("SetActiveTenantID: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "tenant-mcp.example.com")
	if err != nil {
		t.Fatalf("CreateSite: %v", err)
	}
	_, token, err := store.CreateAPIClient(ctx, userID, "mcp-tenant-reader", "", authcore.InstanceUser, map[uuid.UUID]authcore.SiteRole{
		site.ID: authcore.SiteViewer,
	}, nil)
	if err != nil {
		t.Fatalf("CreateAPIClient: %v", err)
	}
	return store, database.NewTenantStoreManager(store, t.TempDir()), site, token
}

func callSearchConsoleStatus(t *testing.T, session *mcp.ClientSession, siteID uuid.UUID) (string, searchConsoleStatusOutput) {
	t.Helper()
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "hitkeep_get_search_console_status",
		Arguments: map[string]any{"site_id": siteID.String()},
	})
	requireSuccessfulMCPTool(t, res, err)
	raw := marshalMCPStructuredContent(t, res)
	var output searchConsoleStatusOutput
	if err := json.Unmarshal([]byte(raw), &output); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	return raw, output
}

func callSearchConsoleReport(t *testing.T, session *mcp.ClientSession, args map[string]any) searchConsoleOutput {
	t.Helper()
	_, output := callSearchConsoleReportRaw(t, session, args)
	return output
}

func callSearchConsoleReportRaw(t *testing.T, session *mcp.ClientSession, args map[string]any) (string, searchConsoleOutput) {
	t.Helper()
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "hitkeep_get_search_console", Arguments: args})
	requireSuccessfulMCPTool(t, res, err)
	raw := marshalMCPStructuredContent(t, res)
	var output searchConsoleOutput
	if err := json.Unmarshal([]byte(raw), &output); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	return raw, output
}

func requireSuccessfulMCPTool(t *testing.T, res *mcp.CallToolResult, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected Search Console tool success, got %+v", res.Content)
	}
}

func marshalMCPStructuredContent(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	return string(raw)
}

func requireNoSearchConsoleSecrets(t *testing.T, rawJSON string) {
	t.Helper()
	for _, forbidden := range []string{"access_token", "refresh_token", "client_secret", "google_account"} {
		if strings.Contains(rawJSON, forbidden) {
			t.Fatalf("Search Console status leaked forbidden field %q in %s", forbidden, rawJSON)
		}
	}
}

func requireMappedSearchConsoleStatus(t *testing.T, output searchConsoleStatusOutput, site *api.Site, teamID uuid.UUID, reason string) {
	t.Helper()
	if output.SiteID != site.ID.String() || output.TeamID != teamID.String() {
		t.Fatalf("unexpected site/team in status: %+v", output)
	}
	requireMappedSearchConsoleProperty(t, output, site)
	requireSearchConsoleAvailability(t, output, reason)
}

func requireMappedSearchConsoleProperty(t *testing.T, output searchConsoleStatusOutput, site *api.Site) {
	t.Helper()
	if !output.Mapped || output.PropertyURI != "sc-domain:"+site.Domain || output.PropertyPermissionLevel != "siteOwner" {
		t.Fatalf("expected mapped property details, got %+v", output)
	}
}

func requireSearchConsoleAvailability(t *testing.T, output searchConsoleStatusOutput, reason string) {
	t.Helper()
	if output.SyncStatus == nil || output.SyncStatus.State != "succeeded" {
		t.Fatalf("expected succeeded sync status, got %+v", output.SyncStatus)
	}
	if !output.DataAvailable || output.AvailableFrom != "2026-04-01" || output.AvailableTo != "2026-05-01" {
		t.Fatalf("expected available imported date range, got %+v", output)
	}
	if output.NeedsAttention || output.Reason != reason {
		t.Fatalf("expected ready status, got %+v", output)
	}
}

func requireTenantSearchConsoleDefaultReport(t *testing.T, output searchConsoleOutput) {
	t.Helper()
	if output.PropertyURI != "sc-domain:tenant-mcp.example.com" {
		t.Fatalf("expected property uri, got %+v", output)
	}
	if output.Overview == nil || output.Overview.Clicks != 4 || output.Overview.Impressions != 40 {
		t.Fatalf("expected tenant-scoped overview, got %+v", output.Overview)
	}
	requireTenantSearchConsoleSeries(t, output)
}

func requireTenantSearchConsoleSeries(t *testing.T, output searchConsoleOutput) {
	t.Helper()
	if output.Series == nil || len(output.Series.Series) != 1 || output.Series.Series[0].Clicks != 4 || output.Series.Series[0].Date != "2026-05-02" {
		t.Fatalf("expected tenant-scoped series, got %+v", output.Series)
	}
	if output.Queries != nil || output.Pages != nil || output.Country != nil || output.Device != nil {
		t.Fatalf("default Search Console report should omit dimensions, got %+v", output)
	}
}

func requireExplicitSearchConsoleSections(t *testing.T, output searchConsoleOutput) {
	t.Helper()
	if output.Overview != nil || output.Series != nil {
		t.Fatalf("explicit dimensions should omit unrequested overview/series, got %+v", output)
	}
	requireDimensionRows(t, output.Queries, 50, "")
	requireDimensionRows(t, output.Pages, 1, "https://tenant-mcp.example.com/landing?utm=test")
	requireDimensionRows(t, output.Country, 1, "USA")
	requireDimensionRows(t, output.Device, 1, "DESKTOP")
}

func requireDimensionRows(t *testing.T, rows *api.SearchConsoleDimensionResponse, count int, value string) {
	t.Helper()
	if rows == nil || len(rows.Rows) != count {
		t.Fatalf("expected %d dimension rows, got %+v", count, rows)
	}
	if value != "" && rows.Rows[0].Value != value {
		t.Fatalf("expected dimension value %q, got %+v", value, rows.Rows[0])
	}
}

func requireSearchConsoleSyncWarnings(t *testing.T, output searchConsoleOutput) {
	t.Helper()
	if output.Overview == nil || output.Overview.Clicks != 7 {
		t.Fatalf("expected imported data despite sync warning, got %+v", output.Overview)
	}
	for _, warning := range []string{"search_console_sync_needs_attention", "requested_range_starts_before_imported_data", "requested_range_ends_after_imported_data"} {
		if !hasWarning(output.Warnings, warning) {
			t.Fatalf("expected warning %q in %+v", warning, output.Warnings)
		}
	}
	if output.SyncStatus == nil || output.SyncStatus.State != "needs_attention" || output.SyncStatus.LastErrorCategory != "authorization_revoked" {
		t.Fatalf("expected needs-attention sync status, got %+v", output.SyncStatus)
	}
}

func seedSearchConsoleMapping(t *testing.T, store *database.Store, site *api.Site) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	teamID, err := store.GetSiteTenantID(ctx, site.ID)
	if err != nil {
		t.Fatalf("GetSiteTenantID: %v", err)
	}
	propertyURI := "sc-domain:" + site.Domain
	if err := store.UpsertGoogleSearchConsoleProperty(ctx, database.GoogleSearchConsolePropertyInput{
		TeamID:          teamID,
		URI:             propertyURI,
		PermissionLevel: "siteOwner",
		SeenAt:          time.Now().UTC(),
	}); err != nil {
		t.Fatalf("UpsertGoogleSearchConsoleProperty: %v", err)
	}
	if err := store.UpsertGoogleSearchConsoleSiteMapping(ctx, database.GoogleSearchConsoleSiteMappingInput{
		SiteID:      site.ID,
		TeamID:      teamID,
		PropertyURI: propertyURI,
		MappedBy:    site.UserID,
		MappedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("UpsertGoogleSearchConsoleSiteMapping: %v", err)
	}
	return teamID
}

func seedSearchConsoleSyncState(t *testing.T, store *database.Store, siteID, teamID uuid.UUID, state string) {
	t.Helper()
	importedStart := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	importedEnd := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	lastSuccess := time.Date(2026, 5, 3, 9, 30, 0, 0, time.UTC)
	if err := store.UpsertGoogleSearchConsoleSyncState(context.Background(), database.GoogleSearchConsoleSyncStateInput{
		SiteID:            siteID,
		TeamID:            teamID,
		State:             state,
		ImportedStartDate: &importedStart,
		ImportedEndDate:   &importedEnd,
		LastSuccessAt:     &lastSuccess,
		LastAttemptAt:     &lastSuccess,
	}); err != nil {
		t.Fatalf("UpsertGoogleSearchConsoleSyncState: %v", err)
	}
}

func searchConsoleFact(site *api.Site, clicks, impressions int) database.SearchConsoleFactInput {
	return database.SearchConsoleFactInput{
		SiteID:          site.ID,
		PropertyURI:     "sc-domain:" + site.Domain,
		Date:            time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC),
		Query:           "hitkeep analytics",
		Page:            "https://" + site.Domain + "/",
		Country:         "US",
		Device:          "DESKTOP",
		Clicks:          clicks,
		Impressions:     impressions,
		CTR:             float64(clicks) / float64(impressions),
		Position:        3.5,
		AggregationType: "auto",
		DataState:       "final",
		ImportedAt:      time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC),
	}
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

func newMCPInitializeHTTPRequest(t *testing.T, endpoint, token string) *http.Request {
	t.Helper()
	body := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test-client","version":"test"}}}`)
	req, err := http.NewRequest(http.MethodPost, endpoint, body)
	if err != nil {
		t.Fatalf("new initialize request: %v", err)
	}
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
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

func hasWarning(warnings []string, want string) bool {
	return slices.Contains(warnings, want)
}

func mcpToolErrorText(res *mcp.CallToolResult) string {
	if res == nil {
		return ""
	}
	raw, err := json.Marshal(res.Content)
	if err != nil {
		return ""
	}
	return string(raw)
}
