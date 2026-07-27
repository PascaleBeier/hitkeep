package askai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	hitai "hitkeep/internal/ai"
	"hitkeep/internal/api"
	authcore "hitkeep/internal/auth"
	"hitkeep/internal/config"
	"hitkeep/internal/controlstore"
	"hitkeep/internal/database"
	"hitkeep/internal/server/shared"
	"hitkeep/internal/testutil"
)

func TestAskAIRejectsDisabledFeatureWithSafeStatus(t *testing.T) {
	mux, store, siteID, userID, ai, _ := setupAskAIHandlerTestEnv(t, &config.Config{
		JWTSecret:    "ask-ai-test-secret",
		PublicURL:    "http://ask-ai.test",
		AIEnabled:    true,
		AskAIEnabled: false,
		AIProvider:   "bedrock",
		AIModel:      "amazon.nova-lite-v1:0",
	})

	rec := requestAskAI(t, mux, siteID, askAISessionCookie(t, userID), "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected disabled feature status %d, got %d: %s", http.StatusConflict, rec.Code, rec.Body.String())
	}
	var status api.AskAIStatus
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status.Enabled || status.Available || status.Status != "disabled" {
		t.Fatalf("expected disabled safe status, got %+v", status)
	}
	if ai.calls != 0 {
		t.Fatalf("expected AI not to be called, got %d calls", ai.calls)
	}

	teamID := requireSiteTeamID(t, store, siteID)
	entry := requireTeamAuditEntry(t, store, teamID, askAIAuditActionRequested)
	if entry.Outcome != "failure" || !strings.Contains(entry.Details, "status=disabled") {
		t.Fatalf("expected disabled request audit, got %+v", entry)
	}
	responseEntry := requireTeamAuditEntry(t, store, teamID, askAIAuditActionResponded)
	if responseEntry.Outcome != "failure" || !strings.Contains(responseEntry.Details, "status=disabled") {
		t.Fatalf("expected disabled response audit, got %+v", responseEntry)
	}
}

func TestAskAIRejectsAPIClients(t *testing.T) {
	mux, store, siteID, userID, ai, _ := setupAskAIHandlerTestEnv(t, askAITestConfig())
	_, token, err := store.CreateAPIClient(context.Background(), userID, "Ask AI API Client", "test", authcore.InstanceUser, map[uuid.UUID]authcore.SiteRole{
		siteID: authcore.SiteViewer,
	}, nil)
	if err != nil {
		t.Fatalf("CreateAPIClient: %v", err)
	}

	rec := requestAskAI(t, mux, siteID, nil, token)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected API client status %d, got %d: %s", http.StatusForbidden, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "dashboard session") {
		t.Fatalf("expected dashboard-session error, got %q", rec.Body.String())
	}
	if ai.calls != 0 {
		t.Fatalf("expected AI not to be called, got %d calls", ai.calls)
	}

	teamID := requireSiteTeamID(t, store, siteID)
	entry := requireTeamAuditEntry(t, store, teamID, askAIAuditActionRequested)
	if entry.Outcome != "denied" || !strings.Contains(entry.Details, "status=dashboard_session_required") {
		t.Fatalf("expected denied API client audit, got %+v", entry)
	}
	responseEntry := requireTeamAuditEntry(t, store, teamID, askAIAuditActionResponded)
	if responseEntry.Outcome != "denied" || !strings.Contains(responseEntry.Details, "status=dashboard_session_required") {
		t.Fatalf("expected denied API client response audit, got %+v", responseEntry)
	}
}

func TestAskAIHistoryListsSafeRunsForHumanSession(t *testing.T) {
	mux, store, siteID, userID, _, _ := setupAskAIHandlerTestEnv(t, askAITestConfig())
	teamID := requireSiteTeamID(t, store, siteID)
	runID := uuid.New()
	if _, err := store.AppendAIRun(context.Background(), controlstore.AIRunParams{
		ID:              runID,
		TeamID:          teamID,
		SiteID:          siteID,
		ActorID:         userID,
		ActorType:       "user",
		Feature:         "ask_ai",
		Provider:        "bedrock",
		Model:           "openai.gpt-oss-120b-1:0",
		TemplateVersion: "ask-ai-v1",
		EvidenceIDs:     []string{"input_context"},
		InputHash:       "history-input-hash",
		OutputHash:      "history-output-hash",
		OutputJSON: `{
			"version":"ask-ai-output-summary-v1",
			"answer_sha256":"answer-hash",
			"answer_chars":42,
			"citations":[{"tool_call_id":"input_context","label_sha256":"label-hash","label_chars":8}],
			"charts":[{"type":"bar","title_sha256":"title-hash","title_chars":7,"row_count":4,"series_count":1,"series":[{"key_sha256":"key-hash","key_chars":6,"label_sha256":"series-label-hash","label_chars":6}]}],
			"actions":[{"type":"navigate","format":"","label_sha256":"action-label-hash","label_chars":11,"target_sha256":"target-hash","target_chars":7}]
		}`,
		InputTokens:     11,
		OutputTokens:    12,
		TotalTokens:     23,
		ToolCallCount:   1,
		LifecycleEvents: []controlstore.AILifecycleEvent{{Type: "tool_call_start", ToolName: "hitkeep_get_site_overview", Status: "started", Timestamp: time.Now().UTC()}},
		Status:          "success",
		CreatedAt:       time.Now().UTC(),
	}); err != nil {
		t.Fatalf("append ask ai run: %v", err)
	}

	rec := requestAskAIHistory(t, mux, siteID, askAISessionCookie(t, userID), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	var response api.AskAIHistoryResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode history response: %v", err)
	}
	if response.Total != 1 || len(response.Entries) != 1 || response.Entries[0].RunID != runID.String() {
		t.Fatalf("unexpected history response: %+v", response)
	}
	entry := response.Entries[0]
	if entry.AnswerChars != 42 || entry.CitationCount != 1 || entry.ChartCount != 1 || entry.ActionCount != 1 {
		t.Fatalf("expected safe output summary counts, got %+v", entry)
	}
	if strings.Join(entry.ChartTypes, ",") != "bar" || strings.Join(entry.ActionTypes, ",") != "navigate" || strings.Join(entry.ToolNames, ",") != "hitkeep_get_site_overview" {
		t.Fatalf("expected summarized chart/action/tool details, got %+v", entry)
	}
	assertNoRawAskAIContent(t, rec.Body.String())

	audit := requireTeamAuditEntry(t, store, teamID, askAIAuditActionHistoryViewed)
	if audit.Outcome != "success" || !strings.Contains(audit.Details, "status=success") || !strings.Contains(audit.Details, "total=1") {
		t.Fatalf("expected successful history audit, got %+v", audit)
	}
}

func TestAskAIHistoryRejectsAPIClients(t *testing.T) {
	mux, store, siteID, userID, _, _ := setupAskAIHandlerTestEnv(t, askAITestConfig())
	_, token, err := store.CreateAPIClient(context.Background(), userID, "Ask AI API Client", "test", authcore.InstanceUser, map[uuid.UUID]authcore.SiteRole{
		siteID: authcore.SiteViewer,
	}, nil)
	if err != nil {
		t.Fatalf("CreateAPIClient: %v", err)
	}

	rec := requestAskAIHistory(t, mux, siteID, nil, token)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected API client status %d, got %d: %s", http.StatusForbidden, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "dashboard session") {
		t.Fatalf("expected dashboard-session error, got %q", rec.Body.String())
	}
	teamID := requireSiteTeamID(t, store, siteID)
	audit := requireTeamAuditEntry(t, store, teamID, askAIAuditActionHistoryViewed)
	if audit.Outcome != "denied" || !strings.Contains(audit.Details, "status=dashboard_session_required") {
		t.Fatalf("expected denied history audit, got %+v", audit)
	}
}

func TestAskAIRejectsAPIClientsBeforeSitePermissionChecks(t *testing.T) {
	mux, store, siteID, userID, ai, _ := setupAskAIHandlerTestEnv(t, askAITestConfig())
	_, token, err := store.CreateAPIClient(context.Background(), userID, "Ask AI API Client", "test", authcore.InstanceUser, nil, nil)
	if err != nil {
		t.Fatalf("CreateAPIClient: %v", err)
	}

	rec := requestAskAI(t, mux, siteID, nil, token)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected API client status %d, got %d: %s", http.StatusForbidden, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "dashboard session") {
		t.Fatalf("expected dashboard-session error, got %q", rec.Body.String())
	}
	if ai.calls != 0 {
		t.Fatalf("expected AI not to be called, got %d calls", ai.calls)
	}

	entry := requireInstanceAuditEntry(t, store, askAIAuditActionRequested)
	if entry.Outcome != "denied" || entry.TargetID != siteID.String() || !strings.Contains(entry.Details, "status=dashboard_session_required") {
		t.Fatalf("expected API client request audit to use dashboard-session boundary before site lookup, got %+v", entry)
	}
	responseEntry := requireInstanceAuditEntry(t, store, askAIAuditActionResponded)
	if responseEntry.Outcome != "denied" || responseEntry.TargetID != siteID.String() || !strings.Contains(responseEntry.Details, "status=dashboard_session_required") {
		t.Fatalf("expected API client response audit to use dashboard-session boundary before site lookup, got %+v", responseEntry)
	}
}

func TestAskAIHumanSessionUsesScopedToolsAndSkills(t *testing.T) {
	mux, store, siteID, userID, ai, _ := setupAskAIHandlerTestEnv(t, askAITestConfig())
	ai.output = &hitai.AskAIOutput{
		AnswerMarkdown: "Traffic increased.",
		Charts: []hitai.AskAIChart{{
			Type:   "line",
			Title:  "Traffic",
			XKey:   "date",
			Series: []hitai.AskAIChartSeries{{Key: "visits_total", Label: "Visits"}},
			Rows:   []map[string]any{{"date": "2026-06-01", "visits_total": float64(42)}},
		}},
		Actions: []hitai.AskAIAction{{Type: "navigate", Label: "Open events", Target: "/events"}},
	}

	rec := requestAskAI(t, mux, siteID, askAISessionCookie(t, userID), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if ai.calls != 1 {
		t.Fatalf("expected one AI call, got %d", ai.calls)
	}
	if ai.last.ActorType != "user" || ai.last.ActorID != userID {
		t.Fatalf("expected human actor %s, got %s/%s", userID, ai.last.ActorType, ai.last.ActorID)
	}
	if ai.last.SiteID != siteID || ai.last.TeamID == uuid.Nil {
		t.Fatalf("expected site/team scoping, got site=%s team=%s", ai.last.SiteID, ai.last.TeamID)
	}
	if len(ai.last.Tools) == 0 {
		t.Fatal("expected read-only aggregate tools to be supplied")
	}
	if !strings.Contains(ai.last.SkillText, "HitKeep") {
		t.Fatal("expected public HitKeep skills to be supplied")
	}

	var response api.AskAIResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.AnswerMarkdown == "" || len(response.Actions) != 1 || response.Actions[0].Target != "/events" {
		t.Fatalf("unexpected Ask AI response: %+v", response)
	}

	teamID := requireSiteTeamID(t, store, siteID)
	requested := requireTeamAuditEntry(t, store, teamID, askAIAuditActionRequested)
	if requested.Outcome != "success" || !strings.Contains(requested.Details, "status=accepted") || !strings.Contains(requested.Details, "request_sha256=") || !strings.Contains(requested.Details, "query_sha256=") {
		t.Fatalf("expected accepted request audit with hashes, got %+v", requested)
	}
	assertNoRawAskAIContent(t, requested.Details)

	responded := requireTeamAuditEntry(t, store, teamID, askAIAuditActionResponded)
	if responded.Outcome != "success" || !strings.Contains(responded.Details, "status=success") || !strings.Contains(responded.Details, "run_id="+response.RunID) || !strings.Contains(responded.Details, "answer_sha256=") || !strings.Contains(responded.Details, "action_types=navigate") {
		t.Fatalf("expected successful response audit with hashes, got %+v", responded)
	}
	assertNoRawAskAIContent(t, responded.Details)

	instanceEntry := requireInstanceAuditEntry(t, store, askAIAuditActionResponded)
	if instanceEntry.TeamID == nil || *instanceEntry.TeamID != teamID || instanceEntry.TargetID != siteID.String() {
		t.Fatalf("expected instance audit to include team/site context, got %+v", instanceEntry)
	}
	metadataJSON := requireInstanceAuditMetadataJSON(t, store, askAIAuditActionResponded)
	assertNoRawAskAIContent(t, metadataJSON)
	assertAskAIAuditMetadataHashesChartSeriesKeys(t, metadataJSON)
}

func TestAskAIWithoutExplicitRangeUsesObservedSiteDataBounds(t *testing.T) {
	mux, _, siteID, userID, ai, tenantStores := setupAskAIHandlerTestEnv(t, askAITestConfig())
	firstHit := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	lastHit := time.Date(2026, 6, 12, 18, 30, 0, 0, time.UTC)
	analytics, _, err := tenantStores.ResolveSiteStore(t.Context(), siteID)
	if err != nil {
		t.Fatalf("resolve analytics: %v", err)
	}
	seedAskAIHit(t, analytics, siteID, firstHit)
	seedAskAIHit(t, analytics, siteID, lastHit)

	rec := requestAskAIWithBody(t, mux, siteID, askAISessionCookie(t, userID), "", `{
		"query": "What changed in traffic?",
		"route": "/dashboard"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if ai.calls != 1 {
		t.Fatalf("expected one AI call, got %d", ai.calls)
	}
	if !ai.last.From.Equal(firstHit) || !ai.last.To.Equal(lastHit) {
		t.Fatalf("expected Ask AI range to use hit bounds %s-%s, got %s-%s", firstHit, lastHit, ai.last.From, ai.last.To)
	}
}

func TestAskAINonDefaultSiteUsesTenantAnalyticsBounds(t *testing.T) {
	ctx := context.Background()
	control, manager := testutil.NewControlAndTenantStores(t)
	userID, err := control.CreateUser(ctx, "ask-ai-tenant@example.com", "hash")
	if err != nil {
		_ = control.Close()
		t.Fatal(err)
	}
	tenant, err := control.CreateTenant(ctx, userID, "Ask AI Tenant", "")
	if err != nil {
		_ = control.Close()
		t.Fatal(err)
	}
	if err := control.SetActiveTenantID(ctx, userID, tenant.ID); err != nil {
		_ = control.Close()
		t.Fatal(err)
	}
	site, err := control.CreateSite(ctx, userID, "ask-ai-tenant.example.test")
	if err != nil {
		_ = control.Close()
		t.Fatal(err)
	}
	if err := manager.SyncSite(ctx, site.ID); err != nil {
		t.Fatal(err)
	}
	tenantStore, _, err := manager.ResolveSiteStore(ctx, site.ID)
	if err != nil {
		t.Fatal(err)
	}
	firstHit := time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC)
	lastHit := time.Date(2026, 6, 12, 18, 30, 0, 0, time.UTC)
	seedAskAIHit(t, tenantStore, site.ID, firstHit)
	seedAskAIHit(t, tenantStore, site.ID, lastHit)
	ai := &recordingAskAIClient{}
	cfg := askAITestConfig()
	appCtx := &shared.Context{Store: control, TenantStores: manager, Config: cfg, AI: ai}
	mux := http.NewServeMux()
	Register(mux, appCtx)
	defer func() {
		_ = manager.Close()
		_ = control.Close()
	}()
	rec := requestAskAIWithBody(t, mux, site.ID, askAISessionCookie(t, userID), "", `{"query":"What changed in traffic?","route":"/dashboard"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if !ai.last.From.Equal(firstHit) || !ai.last.To.Equal(lastHit) {
		t.Fatalf("expected non-default Ask AI range from tenant hits %s-%s, got %s-%s", firstHit, lastHit, ai.last.From, ai.last.To)
	}
}

func TestAskAIStreamingEmitsProgressBeforeFinalResponse(t *testing.T) {
	mux, store, siteID, userID, ai, _ := setupAskAIHandlerTestEnv(t, askAITestConfig())
	releaseAI := make(chan struct{})
	calledAI := make(chan struct{})
	ai.block = releaseAI
	ai.called = calledAI
	ai.streamDeltas = []hitai.AskAIStreamDelta{
		{Type: hitai.AskAIStreamDeltaProgress, Status: "tool_call_start", MessageKey: "askAi.progress.readingAnalytics", ToolCallID: "tool-1", ToolName: "hitkeep_get_event_names"},
		{Type: hitai.AskAIStreamDeltaAnswer, TextDelta: "Traffic "},
		{Type: hitai.AskAIStreamDeltaAnswer, TextDelta: "increased."},
	}

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	req := newAskAIHTTPTestRequest(t, http.MethodPost, server.URL+"/api/sites/"+siteID.String()+"/ask-ai/events", askAISessionCookie(t, userID), "")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("stream Ask AI request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected streaming status %d, got %d: %s", http.StatusOK, resp.StatusCode, string(body))
	}
	if contentType := resp.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "text/event-stream") {
		t.Fatalf("expected text/event-stream content type, got %q", contentType)
	}

	reader := bufio.NewReader(resp.Body)
	first := readAskAISSEEvent(t, reader)
	if first.Event != "progress" || !strings.Contains(first.Data, `"status":"accepted"`) {
		t.Fatalf("expected accepted progress event before AI completed, got %+v", first)
	}
	second := readAskAISSEEvent(t, reader)
	if second.Event != "progress" || !strings.Contains(second.Data, `"status":"generating"`) {
		t.Fatalf("expected generating progress event before AI completed, got %+v", second)
	}
	select {
	case <-calledAI:
	case <-time.After(time.Second):
		t.Fatal("expected streaming handler to start AI generation after progress events")
	}

	close(releaseAI)
	toolProgress := readAskAISSEEvent(t, reader)
	if toolProgress.Event != "progress" || !strings.Contains(toolProgress.Data, `"tool_call_id":"tool-1"`) || !strings.Contains(toolProgress.Data, `"tool_name":"hitkeep_get_event_names"`) {
		t.Fatalf("expected tool progress metadata before answer delta, got %+v", toolProgress)
	}
	delta := readAskAISSEEvent(t, reader)
	if delta.Event != "delta" || !strings.Contains(delta.Data, `"delta_markdown":"Traffic "`) {
		t.Fatalf("expected first streamed answer delta before final response, got %+v", delta)
	}
	delta = readAskAISSEEvent(t, reader)
	if delta.Event != "delta" || !strings.Contains(delta.Data, `"delta_markdown":"increased."`) {
		t.Fatalf("expected second streamed answer delta before final response, got %+v", delta)
	}
	final := readAskAISSEEvent(t, reader)
	if final.Event != "final" || !strings.Contains(final.Data, `"answer_markdown":"Traffic increased."`) {
		t.Fatalf("expected final Ask AI response event, got %+v", final)
	}

	teamID := requireSiteTeamID(t, store, siteID)
	requested := requireTeamAuditEntry(t, store, teamID, askAIAuditActionRequested)
	if requested.Outcome != "success" || !strings.Contains(requested.Details, "status=accepted") {
		t.Fatalf("expected accepted streaming request audit, got %+v", requested)
	}
	responded := requireTeamAuditEntry(t, store, teamID, askAIAuditActionResponded)
	if responded.Outcome != "success" || !strings.Contains(responded.Details, "status=success") {
		t.Fatalf("expected successful streaming response audit, got %+v", responded)
	}
}

func TestAskAIStreamingAuditsTerminalResponseAfterRequestContextCancel(t *testing.T) {
	mux, store, siteID, userID, ai, _ := setupAskAIHandlerTestEnv(t, askAITestConfig())
	failedRunID := uuid.New()
	ctx, cancel := context.WithCancel(context.Background())
	ai.cancelBeforeReturn = cancel
	ai.err = context.Canceled
	ai.runIDOnError = failedRunID

	req := newAskAIHTTPTestRequest(t, http.MethodPost, "/api/sites/"+siteID.String()+"/ask-ai/events", askAISessionCookie(t, userID), "").WithContext(ctx)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	teamID := requireSiteTeamID(t, store, siteID)
	requested := requireTeamAuditEntry(t, store, teamID, askAIAuditActionRequested)
	if requested.Outcome != "success" || !strings.Contains(requested.Details, "status=accepted") {
		t.Fatalf("expected accepted streaming request audit, got %+v", requested)
	}
	responded := requireTeamAuditEntry(t, store, teamID, askAIAuditActionResponded)
	if responded.Outcome != "failure" || !strings.Contains(responded.Details, "status=canceled") || !strings.Contains(responded.Details, "run_id="+failedRunID.String()) {
		t.Fatalf("expected terminal canceled response audit after request cancellation, got %+v", responded)
	}
	assertAskAIAuditMetadataRunID(t, requireInstanceAuditMetadataJSON(t, store, askAIAuditActionResponded), failedRunID)
}

func TestAskAIStreamingAuditsTerminalResponseWhenProgressWriteFails(t *testing.T) {
	mux, store, siteID, userID, ai, _ := setupAskAIHandlerTestEnv(t, askAITestConfig())

	req := newAskAIHTTPTestRequest(t, http.MethodPost, "/api/sites/"+siteID.String()+"/ask-ai/events", askAISessionCookie(t, userID), "")
	rec := &failingAskAIStreamResponseWriter{header: http.Header{}}
	mux.ServeHTTP(rec, req)

	if ai.calls != 0 {
		t.Fatalf("expected AI not to be called after progress write failure, got %d calls", ai.calls)
	}
	teamID := requireSiteTeamID(t, store, siteID)
	requested := requireTeamAuditEntry(t, store, teamID, askAIAuditActionRequested)
	if requested.Outcome != "success" || !strings.Contains(requested.Details, "status=accepted") {
		t.Fatalf("expected accepted streaming request audit, got %+v", requested)
	}
	responded := requireTeamAuditEntry(t, store, teamID, askAIAuditActionResponded)
	if responded.Outcome != "failure" || !strings.Contains(responded.Details, "status=stream_write_failed") {
		t.Fatalf("expected stream write failure response audit, got %+v", responded)
	}
}

func TestAskAIStreamingAuditsTerminalResponseWhenFinalWriteFails(t *testing.T) {
	mux, store, siteID, userID, ai, _ := setupAskAIHandlerTestEnv(t, askAITestConfig())

	req := newAskAIHTTPTestRequest(t, http.MethodPost, "/api/sites/"+siteID.String()+"/ask-ai/events", askAISessionCookie(t, userID), "")
	rec := &eventuallyFailingAskAIStreamResponseWriter{header: http.Header{}, failOnWrite: 3}
	mux.ServeHTTP(rec, req)

	if ai.calls != 1 {
		t.Fatalf("expected AI to be called before final stream write failure, got %d calls", ai.calls)
	}
	teamID := requireSiteTeamID(t, store, siteID)
	requested := requireTeamAuditEntry(t, store, teamID, askAIAuditActionRequested)
	if requested.Outcome != "success" || !strings.Contains(requested.Details, "status=accepted") {
		t.Fatalf("expected accepted streaming request audit, got %+v", requested)
	}
	respondedEntries := requireTeamAuditEntries(t, store, teamID, askAIAuditActionResponded, 2)
	var sawGenerated, sawDeliveryFailure bool
	for _, responded := range respondedEntries {
		assertNoRawAskAIContent(t, responded.Details)
		switch {
		case responded.Outcome == "success" && strings.Contains(responded.Details, "status=success"):
			sawGenerated = true
		case responded.Outcome == "failure" && strings.Contains(responded.Details, "status=stream_write_failed"):
			sawDeliveryFailure = true
		}
	}
	if !sawGenerated || !sawDeliveryFailure {
		t.Fatalf("expected generated-response and delivery-failure audits, got %+v", respondedEntries)
	}
}

func TestAskAIAuditsProviderFailures(t *testing.T) {
	mux, store, siteID, userID, ai, _ := setupAskAIHandlerTestEnv(t, askAITestConfig())
	failedRunID := uuid.New()
	ai.err = hitai.ErrInvalidOutput
	ai.runIDOnError = failedRunID

	rec := requestAskAI(t, mux, siteID, askAISessionCookie(t, userID), "")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadGateway, rec.Code, rec.Body.String())
	}
	if ai.calls != 1 {
		t.Fatalf("expected one AI call, got %d", ai.calls)
	}

	teamID := requireSiteTeamID(t, store, siteID)
	requested := requireTeamAuditEntry(t, store, teamID, askAIAuditActionRequested)
	if requested.Outcome != "success" || !strings.Contains(requested.Details, "status=accepted") {
		t.Fatalf("expected accepted request audit before provider failure, got %+v", requested)
	}
	responded := requireTeamAuditEntry(t, store, teamID, askAIAuditActionResponded)
	if responded.Outcome != "failure" || !strings.Contains(responded.Details, "status=invalid_output") || !strings.Contains(responded.Details, "error_category=invalid_output") || !strings.Contains(responded.Details, "run_id="+failedRunID.String()) {
		t.Fatalf("expected failed response audit, got %+v", responded)
	}
	assertNoRawAskAIContent(t, responded.Details)
	assertAskAIAuditMetadataRunID(t, requireInstanceAuditMetadataJSON(t, store, askAIAuditActionResponded), failedRunID)
}

func TestAskAIRejectsUnscopedAPIClientsBeforeSiteLookup(t *testing.T) {
	mux, store, _, userID, ai, _ := setupAskAIHandlerTestEnv(t, askAITestConfig())
	_, token, err := store.CreateAPIClient(context.Background(), userID, "Ask AI API Client", "test", authcore.InstanceUser, nil, nil)
	if err != nil {
		t.Fatalf("CreateAPIClient: %v", err)
	}
	missingSiteID := uuid.New()

	rec := requestAskAI(t, mux, missingSiteID, nil, token)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected API client status %d, got %d: %s", http.StatusForbidden, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "dashboard session") {
		t.Fatalf("expected dashboard-session error, got %q", rec.Body.String())
	}
	if ai.calls != 0 {
		t.Fatalf("expected AI not to be called, got %d calls", ai.calls)
	}

	requested := requireInstanceAuditEntry(t, store, askAIAuditActionRequested)
	if requested.Outcome != "denied" || requested.TargetID != missingSiteID.String() || !strings.Contains(requested.Details, "status=dashboard_session_required") {
		t.Fatalf("expected denied API client request audit before site lookup, got %+v", requested)
	}
	responded := requireInstanceAuditEntry(t, store, askAIAuditActionResponded)
	if responded.Outcome != "denied" || responded.TargetID != missingSiteID.String() || !strings.Contains(responded.Details, "status=dashboard_session_required") {
		t.Fatalf("expected denied API client response audit before site lookup, got %+v", responded)
	}
}

func TestAskAIRejectsUsersWithoutSiteView(t *testing.T) {
	mux, store, siteID, _, ai, _ := setupAskAIHandlerTestEnv(t, askAITestConfig())
	otherID, err := store.CreateUser(context.Background(), "ask-ai-no-access@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	rec := requestAskAI(t, mux, siteID, askAISessionCookie(t, otherID), "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected no-access status %d, got %d: %s", http.StatusForbidden, rec.Code, rec.Body.String())
	}
	if ai.calls != 0 {
		t.Fatalf("expected AI not to be called, got %d calls", ai.calls)
	}

	teamID := requireSiteTeamID(t, store, siteID)
	entry := requireTeamAuditEntry(t, store, teamID, askAIAuditActionRequested)
	if entry.Outcome != "denied" || !strings.Contains(entry.Details, "status=access_denied") {
		t.Fatalf("expected denied access audit entry, got %+v", entry)
	}
	responseEntry := requireTeamAuditEntry(t, store, teamID, askAIAuditActionResponded)
	if responseEntry.Outcome != "denied" || !strings.Contains(responseEntry.Details, "status=access_denied") {
		t.Fatalf("expected denied access response audit entry, got %+v", responseEntry)
	}
}

func TestAskAIAuditsInvalidRequests(t *testing.T) {
	mux, store, siteID, userID, ai, _ := setupAskAIHandlerTestEnv(t, askAITestConfig())

	req := httptest.NewRequest(http.MethodPost, "/api/sites/"+siteID.String()+"/ask-ai", strings.NewReader(`{"query":""}`))
	req.AddCookie(askAISessionCookie(t, userID))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid request status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
	if ai.calls != 0 {
		t.Fatalf("expected AI not to be called, got %d calls", ai.calls)
	}

	teamID := requireSiteTeamID(t, store, siteID)
	requestEntry := requireTeamAuditEntry(t, store, teamID, askAIAuditActionRequested)
	if requestEntry.Outcome != "failure" || !strings.Contains(requestEntry.Details, "status=invalid_request") {
		t.Fatalf("expected invalid request audit, got %+v", requestEntry)
	}
	responseEntry := requireTeamAuditEntry(t, store, teamID, askAIAuditActionResponded)
	if responseEntry.Outcome != "failure" || !strings.Contains(responseEntry.Details, "status=invalid_request") {
		t.Fatalf("expected invalid response audit, got %+v", responseEntry)
	}
}

func setupAskAIHandlerTestEnv(t *testing.T, cfg *config.Config) (*http.ServeMux, *controlstore.Store, uuid.UUID, uuid.UUID, *recordingAskAIClient, *database.TenantStoreManager) {
	t.Helper()
	cfg.AuthSessionMinutes = 15
	store, tenantStores := testutil.NewControlAndTenantStores(t)
	t.Cleanup(func() { _ = store.Close() })

	userID, err := store.CreateUser(context.Background(), "ask-ai-human@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	site, err := store.CreateSite(context.Background(), userID, "ask-ai.example.test")
	if err != nil {
		t.Fatalf("CreateSite: %v", err)
	}

	ai := &recordingAskAIClient{}
	t.Cleanup(func() { _ = tenantStores.Close() })
	if err := tenantStores.SyncSite(context.Background(), site.ID); err != nil {
		t.Fatalf("SyncSite: %v", err)
	}
	appCtx := &shared.Context{Store: store, TenantStores: tenantStores, Config: cfg, AI: ai}
	mux := http.NewServeMux()
	Register(mux, appCtx)
	return mux, store, site.ID, userID, ai, tenantStores
}

func askAITestConfig() *config.Config {
	return &config.Config{
		JWTSecret:          "ask-ai-test-secret",
		PublicURL:          "http://ask-ai.test",
		AIEnabled:          true,
		AskAIEnabled:       true,
		AIProvider:         "bedrock",
		AIModel:            "amazon.nova-lite-v1:0",
		AIRequestLimit:     100,
		AITokenLimit:       100_000,
		AuthSessionMinutes: 15,
	}
}

func askAISessionCookie(t *testing.T, userID uuid.UUID) *http.Cookie {
	t.Helper()
	token, _, err := authcore.GenerateTokenWithDuration("ask-ai-test-secret", "http://ask-ai.test", userID, time.Hour)
	if err != nil {
		t.Fatalf("GenerateTokenWithDuration: %v", err)
	}
	return &http.Cookie{Name: authcore.CookieName, Value: token}
}

func requestAskAI(t *testing.T, mux *http.ServeMux, siteID uuid.UUID, cookie *http.Cookie, apiToken string) *httptest.ResponseRecorder {
	t.Helper()
	req := newAskAIHTTPTestRequest(t, http.MethodPost, "/api/sites/"+siteID.String()+"/ask-ai", cookie, apiToken)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func requestAskAIWithBody(t *testing.T, mux *http.ServeMux, siteID uuid.UUID, cookie *http.Cookie, apiToken string, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := newAskAIHTTPTestRequestWithBody(t, http.MethodPost, "/api/sites/"+siteID.String()+"/ask-ai", cookie, apiToken, body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func requestAskAIHistory(t *testing.T, mux *http.ServeMux, siteID uuid.UUID, cookie *http.Cookie, apiToken string) *httptest.ResponseRecorder {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "/api/sites/"+siteID.String()+"/ask-ai/history?limit=20", nil)
	if err != nil {
		t.Fatalf("new Ask AI history request: %v", err)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if apiToken != "" {
		req.Header.Set("X-Api-Key", apiToken)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func newAskAIHTTPTestRequest(t *testing.T, method, url string, cookie *http.Cookie, apiToken string) *http.Request {
	t.Helper()
	return newAskAIHTTPTestRequestWithBody(t, method, url, cookie, apiToken, `{
			"query": "What changed in traffic?",
			"from": "2026-06-01",
			"to": "2026-06-25",
			"route": "/dashboard",
			"filters": [{ "type": "path", "value": "/" }],
			"history": [{ "role": "user", "content": "previous question" }]
		}`)
}

func newAskAIHTTPTestRequestWithBody(t *testing.T, method, url string, cookie *http.Cookie, apiToken string, rawBody string) *http.Request {
	t.Helper()
	body := bytes.NewReader([]byte(rawBody))
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("new Ask AI test request: %v", err)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if apiToken != "" {
		req.Header.Set("X-Api-Key", apiToken)
	}
	return req
}

func seedAskAIHit(t *testing.T, store *database.Store, siteID uuid.UUID, timestamp time.Time) {
	t.Helper()
	if err := store.CreateHit(context.Background(), &api.Hit{
		SiteID:    siteID,
		SessionID: uuid.New(),
		PageID:    uuid.New(),
		Timestamp: timestamp,
		Path:      "/",
	}); err != nil {
		t.Fatalf("CreateHit: %v", err)
	}
}

type askAISSEEvent struct {
	Event string
	Data  string
}

func readAskAISSEEvent(t *testing.T, reader *bufio.Reader) askAISSEEvent {
	t.Helper()
	var event askAISSEEvent
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read SSE event: %v", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if event.Event == "" && event.Data == "" {
				continue
			}
			return event
		}
		if after, ok := strings.CutPrefix(line, "event: "); ok {
			event.Event = after
			continue
		}
		if after, ok := strings.CutPrefix(line, "data: "); ok {
			if event.Data != "" {
				event.Data += "\n"
			}
			event.Data += after
		}
	}
}

func requireSiteTeamID(t *testing.T, store *controlstore.Store, siteID uuid.UUID) uuid.UUID {
	t.Helper()
	teamID, err := store.GetSiteTenantID(context.Background(), siteID)
	if err != nil {
		t.Fatalf("GetSiteTenantID: %v", err)
	}
	return teamID
}

func requireTeamAuditEntry(t *testing.T, store *controlstore.Store, teamID uuid.UUID, action string) api.TeamAuditEntry {
	t.Helper()
	entries := requireTeamAuditEntries(t, store, teamID, action, 1)
	return entries[0]
}

func requireTeamAuditEntries(t *testing.T, store *controlstore.Store, teamID uuid.UUID, action string, want int) []api.TeamAuditEntry {
	t.Helper()
	entries, total, err := store.ListTeamAuditEntries(context.Background(), teamID, action, 10, 0)
	if err != nil {
		t.Fatalf("ListTeamAuditEntries(%s): %v", action, err)
	}
	if total != want || len(entries) != want {
		t.Fatalf("expected %d %s team audit entries, got total=%d entries=%d", want, action, total, len(entries))
	}
	return entries
}

func requireInstanceAuditEntry(t *testing.T, store *controlstore.Store, action string) api.InstanceAuditEntry {
	t.Helper()
	entries, total, err := store.ListInstanceAuditEntries(context.Background(), database.InstanceAuditFilter{Action: action, Limit: 10})
	if err != nil {
		t.Fatalf("ListInstanceAuditEntries(%s): %v", action, err)
	}
	if total != 1 || len(entries) != 1 {
		t.Fatalf("expected one %s instance audit entry, got total=%d entries=%d", action, total, len(entries))
	}
	return entries[0]
}

func requireInstanceAuditMetadataJSON(t *testing.T, store *controlstore.Store, action string) string {
	t.Helper()
	metadataJSON, err := store.LatestInstanceAuditMetadataJSON(context.Background(), action)
	if err != nil {
		t.Fatalf("read %s audit metadata: %v", action, err)
	}
	return metadataJSON
}

func assertAskAIAuditMetadataHashesChartSeriesKeys(t *testing.T, metadataJSON string) {
	t.Helper()
	var metadata struct {
		Charts []struct {
			SeriesLabels []map[string]any `json:"series_labels"`
		} `json:"charts"`
	}
	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
		t.Fatalf("decode Ask AI audit metadata: %v", err)
	}
	if len(metadata.Charts) != 1 || len(metadata.Charts[0].SeriesLabels) != 1 {
		t.Fatalf("expected one chart series audit summary, got %s", metadataJSON)
	}
	series := metadata.Charts[0].SeriesLabels[0]
	if _, ok := series["key"]; ok {
		t.Fatalf("response audit metadata leaked raw chart series key: %s", metadataJSON)
	}
	if _, ok := series["key_sha256"].(string); !ok {
		t.Fatalf("response audit metadata missing key_sha256: %s", metadataJSON)
	}
	if _, ok := series["key_chars"].(float64); !ok {
		t.Fatalf("response audit metadata missing key_chars: %s", metadataJSON)
	}
}

func assertAskAIAuditMetadataRunID(t *testing.T, metadataJSON string, runID uuid.UUID) {
	t.Helper()
	var metadata map[string]any
	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
		t.Fatalf("decode Ask AI audit metadata: %v", err)
	}
	if got, _ := metadata["run_id"].(string); got != runID.String() {
		t.Fatalf("expected Ask AI audit metadata run_id %s, got %s in %s", runID, got, metadataJSON)
	}
}

func assertNoRawAskAIContent(t *testing.T, details string) {
	t.Helper()
	for _, raw := range []string{"What changed in traffic", "previous question", "Traffic increased", "visits_total"} {
		if strings.Contains(details, raw) {
			t.Fatalf("audit details leaked raw Ask AI content %q in %q", raw, details)
		}
	}
}

type recordingAskAIClient struct {
	mu                 sync.Mutex
	calls              int
	last               hitai.AskAIRequest
	output             *hitai.AskAIOutput
	err                error
	runIDOnError       uuid.UUID
	block              <-chan struct{}
	called             chan struct{}
	cancelBeforeReturn func()
	streamDeltas       []hitai.AskAIStreamDelta
}

func (c *recordingAskAIClient) GenerateAskAI(_ context.Context, req hitai.AskAIRequest) (hitai.AskAIResult, error) {
	c.mu.Lock()
	c.calls++
	c.last = req
	if c.called != nil {
		close(c.called)
		c.called = nil
	}
	block := c.block
	c.mu.Unlock()
	if block != nil {
		<-block
	}
	if c.cancelBeforeReturn != nil {
		c.cancelBeforeReturn()
	}
	if c.err != nil {
		return hitai.AskAIResult{RunID: c.runIDOnError}, c.err
	}
	output := hitai.AskAIOutput{
		AnswerMarkdown: "Traffic increased.",
		Actions:        []hitai.AskAIAction{{Type: "navigate", Label: "Open events", Target: "/events"}},
	}
	if c.output != nil {
		output = *c.output
	}
	return hitai.AskAIResult{
		RunID:  uuid.New(),
		Output: output,
	}, nil
}

func (c *recordingAskAIClient) StreamAskAI(ctx context.Context, req hitai.AskAIRequest, emit hitai.AskAIStreamSink) (hitai.AskAIResult, error) {
	result, err := c.GenerateAskAI(ctx, req)
	if err != nil {
		return result, err
	}
	for _, delta := range c.streamDeltas {
		if err := emit(delta); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (c *recordingAskAIClient) GenerateOpportunityProposal(context.Context, hitai.OpportunityRequest) (hitai.OpportunityProposalResult, error) {
	return hitai.OpportunityProposalResult{}, hitai.ErrDisabled
}

func (c *recordingAskAIClient) Configured() bool { return true }
func (c *recordingAskAIClient) Enabled() bool    { return true }
func (c *recordingAskAIClient) Provider() string { return "bedrock" }
func (c *recordingAskAIClient) Model() string    { return "amazon.nova-lite-v1:0" }

type failingAskAIStreamResponseWriter struct {
	header http.Header
}

func (w *failingAskAIStreamResponseWriter) Header() http.Header {
	return w.header
}

func (w *failingAskAIStreamResponseWriter) Write([]byte) (int, error) {
	return 0, io.ErrClosedPipe
}

func (w *failingAskAIStreamResponseWriter) WriteHeader(int) {}

func (w *failingAskAIStreamResponseWriter) Flush() {}

type eventuallyFailingAskAIStreamResponseWriter struct {
	header      http.Header
	writes      int
	failOnWrite int
}

func (w *eventuallyFailingAskAIStreamResponseWriter) Header() http.Header {
	return w.header
}

func (w *eventuallyFailingAskAIStreamResponseWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes >= w.failOnWrite {
		return 0, io.ErrClosedPipe
	}
	return len(p), nil
}

func (w *eventuallyFailingAskAIStreamResponseWriter) WriteHeader(int) {}

func (w *eventuallyFailingAskAIStreamResponseWriter) Flush() {}
