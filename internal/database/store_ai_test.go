package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/database/migrations"
)

func setupAIStore(t *testing.T) (*Store, uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	userID, err := store.CreateUser(context.Background(), "ai-store@example.com", "hashed")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(context.Background(), userID, "ai-store.example")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	teamID, err := store.GetSiteTenantID(context.Background(), site.ID)
	if err != nil {
		t.Fatalf("get site tenant: %v", err)
	}
	return store, userID, site.ID, teamID
}

func TestOpportunitySchemaRemovesMoneyContract(t *testing.T) {
	store, _, _, _ := setupAIStore(t)
	assertNoOpportunityColumn(t, store, "monthly_upside")
}

func assertNoOpportunityColumn(t *testing.T, store *Store, column string) {
	t.Helper()
	rows, err := store.DB().QueryContext(context.Background(), "PRAGMA table_info('opportunities')")
	if err != nil {
		t.Fatalf("table info: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notNull bool
		var defaultValue any
		var pk bool
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("scan table info: %v", err)
		}
		if name == column {
			t.Fatalf("opportunities table must not expose %s", column)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read table info: %v", err)
	}
}

func TestRemoveOpportunityMoneyContractMigrationHandlesIndexedLegacyTable(t *testing.T) {
	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()

	if _, err := store.DB().ExecContext(ctx, `
		CREATE TABLE opportunities (
			id UUID PRIMARY KEY,
			team_id UUID NOT NULL,
			site_id UUID NOT NULL,
			kind VARCHAR NOT NULL,
			type_key VARCHAR NOT NULL,
			title_key VARCHAR NOT NULL,
			summary_key VARCHAR NOT NULL,
			action_key VARCHAR NOT NULL,
			digest_key VARCHAR NOT NULL DEFAULT '',
			copy_params_json JSON NOT NULL DEFAULT '{}',
			impact_value VARCHAR NOT NULL,
			impact_label_key VARCHAR NOT NULL,
			monthly_upside DOUBLE NOT NULL DEFAULT 0,
			confidence VARCHAR NOT NULL,
			score BIGINT NOT NULL DEFAULT 0,
			score_breakdown_json JSON NOT NULL DEFAULT '{}',
			status VARCHAR NOT NULL,
			route_label_key VARCHAR NOT NULL DEFAULT '',
			route_params_json JSON NOT NULL DEFAULT '{}',
			route_icon VARCHAR NOT NULL DEFAULT '',
			detector_version VARCHAR NOT NULL DEFAULT '',
			evidence_json JSON NOT NULL DEFAULT '[]',
			cited_evidence_ids_json JSON NOT NULL DEFAULT '[]',
			ai_run_id UUID,
			generated_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX idx_opportunities_site_status_score ON opportunities (site_id, status, score DESC);
		CREATE INDEX idx_opportunities_team_site_updated ON opportunities (team_id, site_id, updated_at DESC);
		INSERT INTO opportunities (
			id, team_id, site_id, kind, type_key, title_key, summary_key, action_key, copy_params_json,
			impact_value, impact_label_key, monthly_upside, confidence, status, generated_at
		) VALUES (
			?, ?, ?, 'revenue', 'opportunities.types.traffic_quality', 'opportunities.catalog.traffic_quality.title',
			'opportunities.catalog.traffic_quality.summary', 'opportunities.catalog.traffic_quality.action',
			'{"monthly_upside":"8500","currency":"EUR","source":"google"}', '$8500',
			'opportunities.impact.estimated_monthly_upside', 8500, 'high', 'new', CURRENT_TIMESTAMP
		);
	`, uuid.New(), uuid.New(), uuid.New()); err != nil {
		t.Fatalf("create legacy opportunity table: %v", err)
	}

	migrationSQL, err := migrations.Fs.ReadFile("2026_05_28_000000_remove_opportunity_money_contract.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx, string(migrationSQL)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}

	assertNoOpportunityColumn(t, store, "monthly_upside")
	var kind, label, params string
	if err := store.DB().QueryRowContext(ctx, `SELECT kind, impact_label_key, CAST(copy_params_json AS VARCHAR) FROM opportunities`).Scan(&kind, &label, &params); err != nil {
		t.Fatalf("read migrated row: %v", err)
	}
	if kind != "traffic" || label != "opportunities.impact.checkout_starts" {
		t.Fatalf("legacy money row was not normalized: kind=%q label=%q", kind, label)
	}
	if strings.Contains(params, "monthly_upside") || strings.Contains(params, "currency") {
		t.Fatalf("legacy money params survived migration: %s", params)
	}
}

func TestAIRunSummaryAndUsage(t *testing.T) {
	store, userID, siteID, teamID := setupAIStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if _, err := store.AppendAIRun(ctx, AIRunParams{
		TeamID:          teamID,
		SiteID:          siteID,
		ActorID:         userID,
		ActorType:       "user",
		Feature:         "opportunities",
		Provider:        "openai",
		Model:           "gpt-test",
		TemplateVersion: "opportunities-v1",
		EvidenceIDs:     []string{"checkout-rate"},
		InputHash:       "input-hash",
		OutputHash:      "output-hash",
		OutputJSON:      `{"title_key":"opportunities.fixture.title"}`,
		InputTokens:     10,
		OutputTokens:    15,
		TotalTokens:     25,
		Status:          "success",
		CreatedAt:       now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("append success run: %v", err)
	}
	if _, err := store.AppendAIRun(ctx, AIRunParams{
		TeamID:        teamID,
		SiteID:        siteID,
		ActorID:       userID,
		ActorType:     "user",
		Feature:       "opportunities",
		Provider:      "openai",
		Model:         "gpt-test",
		OutputJSON:    `{}`,
		TotalTokens:   0,
		Status:        "failure",
		ErrorCategory: "budget_exhausted",
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("append failure run: %v", err)
	}

	usage, err := store.GetAIUsageSince(ctx, now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	if usage.Requests != 1 || usage.Tokens != 25 {
		t.Fatalf("unexpected usage: requests=%d tokens=%d", usage.Requests, usage.Tokens)
	}

	summary, err := store.GetAIRunSummary(ctx)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.LastSuccessAt == nil {
		t.Fatalf("expected last success")
	}
	if summary.LastAttemptAt == nil {
		t.Fatalf("expected last attempt")
	}
	if summary.LastErrorCategory != "budget_exhausted" {
		t.Fatalf("expected last error budget_exhausted, got %q", summary.LastErrorCategory)
	}
}

func TestAIRunSummarySinceFiltersWindow(t *testing.T) {
	store, userID, siteID, teamID := setupAIStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if _, err := store.AppendAIRun(ctx, AIRunParams{
		TeamID:      teamID,
		SiteID:      siteID,
		ActorID:     userID,
		ActorType:   "user",
		Feature:     "opportunities",
		Provider:    "openai",
		Model:       "gpt-test",
		OutputJSON:  `{}`,
		TotalTokens: 25,
		Status:      "success",
		CreatedAt:   now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("append success run: %v", err)
	}
	if _, err := store.AppendAIRun(ctx, AIRunParams{
		TeamID:        teamID,
		SiteID:        siteID,
		ActorID:       userID,
		ActorType:     "user",
		Feature:       "opportunities",
		Provider:      "openai",
		Model:         "gpt-test",
		OutputJSON:    `{}`,
		Status:        "failure",
		ErrorCategory: "budget_exhausted",
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("append failure run: %v", err)
	}

	recentSummary, err := store.GetAIRunSummarySince(ctx, now.Add(-30*time.Second))
	if err != nil {
		t.Fatalf("recent summary: %v", err)
	}
	if recentSummary.LastSuccessAt != nil {
		t.Fatalf("did not expect last success inside recent window")
	}
	if recentSummary.LastAttemptAt == nil {
		t.Fatalf("expected last attempt inside recent window")
	}
	if recentSummary.LastErrorCategory != "budget_exhausted" {
		t.Fatalf("expected recent last error budget_exhausted, got %q", recentSummary.LastErrorCategory)
	}

	emptySummary, err := store.GetAIRunSummarySince(ctx, now.Add(time.Second))
	if err != nil {
		t.Fatalf("empty summary: %v", err)
	}
	if emptySummary.LastSuccessAt != nil || emptySummary.LastAttemptAt != nil || emptySummary.LastErrorCategory != "" {
		t.Fatalf("expected empty summary outside run window, got %#v", emptySummary)
	}
}

func TestAppendAIRunRejectsRawPromptAndProviderPayloadFields(t *testing.T) {
	store, userID, siteID, teamID := setupAIStore(t)
	ctx := context.Background()

	_, err := store.AppendAIRun(ctx, AIRunParams{
		TeamID:          teamID,
		SiteID:          siteID,
		ActorID:         userID,
		ActorType:       "user",
		Feature:         "opportunities",
		Provider:        "openai",
		Model:           "gpt-test",
		TemplateVersion: "opportunities-v1",
		OutputJSON:      `{"title_key":"opportunities.fixture.title","raw_prompt":"tell me secret things"}`,
		Status:          "success",
		CreatedAt:       time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("expected raw prompt output json to be rejected")
	}
	if !strings.Contains(err.Error(), "raw prompt") {
		t.Fatalf("expected raw prompt validation error, got %v", err)
	}

	_, err = store.AppendAIRun(ctx, AIRunParams{
		TeamID:          teamID,
		SiteID:          siteID,
		ActorID:         userID,
		ActorType:       "user",
		Feature:         "opportunities",
		Provider:        "openai",
		Model:           "gpt-test",
		TemplateVersion: "opportunities-v1",
		OutputJSON:      `{"title_key":"opportunities.fixture.title","provider_response":{"raw":"payload"}}`,
		Status:          "success",
		CreatedAt:       time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("expected provider payload output json to be rejected")
	}
	if !strings.Contains(err.Error(), "provider payload") {
		t.Fatalf("expected provider payload validation error, got %v", err)
	}

	_, err = store.AppendAIRun(ctx, AIRunParams{
		TeamID:          teamID,
		SiteID:          siteID,
		ActorID:         userID,
		ActorType:       "user",
		Feature:         "opportunities",
		Provider:        "openai",
		Model:           "gpt-test",
		TemplateVersion: "opportunities-v1",
		OutputJSON:      `{"title_key":"opportunities.fixture.title","provider_error_body":"external provider body"}`,
		Status:          "failure",
		CreatedAt:       time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("expected provider error body output json to be rejected")
	}
	if !strings.Contains(err.Error(), "provider payload") {
		t.Fatalf("expected provider payload validation error, got %v", err)
	}

	_, err = store.AppendAIRun(ctx, AIRunParams{
		TeamID:          teamID,
		SiteID:          siteID,
		ActorID:         userID,
		ActorType:       "user",
		Feature:         "opportunities",
		Provider:        "openai",
		Model:           "gpt-test",
		TemplateVersion: "opportunities-v1",
		OutputJSON:      `{"title_key":"opportunities.fixture.title","api_key":"placeholder-secret"}`,
		Status:          "failure",
		CreatedAt:       time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("expected credential-shaped output json to be rejected")
	}
	if !strings.Contains(err.Error(), "credential") {
		t.Fatalf("expected credential validation error, got %v", err)
	}

	_, err = store.AppendAIRun(ctx, AIRunParams{
		TeamID:          teamID,
		SiteID:          siteID,
		ActorID:         userID,
		ActorType:       "user",
		Feature:         "opportunities",
		Provider:        "openai",
		Model:           "gpt-test",
		TemplateVersion: "opportunities-v1",
		OutputJSON:      `{"title_key":"opportunities.fixture.title","model_note":"sk-placeholder-secret"}`,
		Status:          "failure",
		CreatedAt:       time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("expected sk-shaped output json value to be rejected")
	}
	if !strings.Contains(err.Error(), "raw payload") {
		t.Fatalf("expected raw payload value validation error, got %v", err)
	}
}

func TestAppendAskAIAIRunRequiresSafeOutputSummary(t *testing.T) {
	store, userID, siteID, teamID := setupAIStore(t)
	ctx := context.Background()

	for name, outputJSON := range map[string]string{
		"raw answer":                `{"answer_markdown":"Traffic increased over the selected range.","citations":[],"charts":[],"actions":[]}`,
		"raw chart rows":            `{"version":"ask-ai-output-summary-v1","answer_sha256":"hash","answer_chars":12,"charts":[{"type":"line","rows":[{"date":"2026-06-01","visits":10}]}],"actions":[],"citations":[]}`,
		"unsupported chart type":    `{"version":"ask-ai-output-summary-v1","answer_sha256":"hash","answer_chars":12,"charts":[{"type":"pie","title_sha256":"title-hash","title_chars":6,"row_count":7,"series_count":1,"series":[]}],"actions":[],"citations":[]}`,
		"mismatched series count":   `{"version":"ask-ai-output-summary-v1","answer_sha256":"hash","answer_chars":12,"charts":[{"type":"line","title_sha256":"title-hash","title_chars":6,"row_count":7,"series_count":2,"series":[]}],"actions":[],"citations":[]}`,
		"raw action link":           `{"version":"ask-ai-output-summary-v1","answer_sha256":"hash","answer_chars":12,"charts":[],"actions":[{"type":"navigate","target":"/dashboard","label":"Open dashboard"}],"citations":[]}`,
		"unsupported action type":   `{"version":"ask-ai-output-summary-v1","answer_sha256":"hash","answer_chars":12,"charts":[],"actions":[{"type":"delete_site","format":"","label_sha256":"label-hash","label_chars":4,"target_sha256":"target-hash","target_chars":10}],"citations":[]}`,
		"unsupported export format": `{"version":"ask-ai-output-summary-v1","answer_sha256":"hash","answer_chars":12,"charts":[],"actions":[{"type":"download_export","format":"html","label_sha256":"label-hash","label_chars":4,"target_sha256":"target-hash","target_chars":10}],"citations":[]}`,
		"malformed action summary":  `{"version":"ask-ai-output-summary-v1","answer_sha256":"hash","answer_chars":12,"charts":[],"actions":[{"type":"navigate","format":"","label_sha256":"label-hash","label_chars":"four","target_sha256":"target-hash","target_chars":10}],"citations":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := store.AppendAIRun(ctx, AIRunParams{
				TeamID:          teamID,
				SiteID:          siteID,
				ActorID:         userID,
				ActorType:       "user",
				Feature:         "ask_ai",
				Provider:        "bedrock",
				Model:           "openai.gpt-oss-120b",
				TemplateVersion: "ask-ai-v1",
				OutputJSON:      outputJSON,
				Status:          "success",
				CreatedAt:       time.Now().UTC(),
			})
			if err == nil {
				t.Fatal("expected raw Ask AI output json to be rejected")
			}
			if !strings.Contains(err.Error(), "ask ai") {
				t.Fatalf("expected Ask AI validation error, got %v", err)
			}
		})
	}

	_, err := store.AppendAIRun(ctx, AIRunParams{
		TeamID:          teamID,
		SiteID:          siteID,
		ActorID:         userID,
		ActorType:       "user",
		Feature:         "ask_ai",
		Provider:        "bedrock",
		Model:           "openai.gpt-oss-120b",
		TemplateVersion: "ask-ai-v1",
		EvidenceIDs:     []string{"hitkeep_get_site_overview"},
		OutputHash:      "output-hash",
		OutputJSON: `{
			"version":"ask-ai-output-summary-v1",
			"answer_sha256":"answer-hash",
			"answer_chars":42,
			"citations":[
				{"tool_call_id":"input_context","label_sha256":"context-label-hash","label_chars":13},
				{"tool_call_id":"hitkeep_get_site_overview","label_sha256":"label-hash","label_chars":8}
			],
			"charts":[{"type":"line","title_sha256":"title-hash","title_chars":6,"row_count":7,"series_count":1,"series":[{"key_sha256":"key-hash","key_chars":6,"label_sha256":"label-hash","label_chars":6}]}],
			"actions":[
				{"type":"navigate","format":"","label_sha256":"action-label-hash","label_chars":14,"target_sha256":"target-hash","target_chars":10},
				{"type":"download_export","format":"csv","label_sha256":"export-label-hash","label_chars":15,"target_sha256":"export-target-hash","target_chars":56}
			]
		}`,
		Status:    "success",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("expected safe Ask AI output summary to be accepted: %v", err)
	}
}

func TestListAskAIHistoryReturnsSafeSiteScopedSummaries(t *testing.T) {
	store, userID, siteID, teamID := setupAIStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	otherSite, err := store.CreateSite(ctx, userID, "ask-ai-other.example")
	if err != nil {
		t.Fatalf("create other site: %v", err)
	}
	otherTeamID, err := store.GetSiteTenantID(ctx, otherSite.ID)
	if err != nil {
		t.Fatalf("get other site team: %v", err)
	}
	olderRunID := uuid.New()
	newerRunID := uuid.New()

	if _, err := store.AppendAIRun(ctx, AIRunParams{
		ID:              olderRunID,
		TeamID:          teamID,
		SiteID:          siteID,
		ActorID:         userID,
		ActorType:       "user",
		Feature:         "ask_ai",
		Provider:        "bedrock",
		Model:           "openai.gpt-oss-120b-1:0",
		TemplateVersion: "ask-ai-v1",
		EvidenceIDs:     []string{"hitkeep_get_site_overview"},
		InputHash:       "older-input-hash",
		OutputHash:      "older-output-hash",
		OutputJSON: `{
			"version":"ask-ai-output-summary-v1",
			"answer_sha256":"older-answer-hash",
			"answer_chars":12,
			"citations":[{"tool_call_id":"hitkeep_get_site_overview","label_sha256":"label-hash","label_chars":8}],
			"charts":[],
			"actions":[]
		}`,
		TotalTokens: 33,
		Status:      "success",
		CreatedAt:   now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("append older ask ai run: %v", err)
	}
	if _, err := store.AppendAIRun(ctx, AIRunParams{
		TeamID:          teamID,
		SiteID:          siteID,
		ActorID:         userID,
		ActorType:       "user",
		Feature:         "opportunities",
		Provider:        "bedrock",
		Model:           "openai.gpt-oss-120b-1:0",
		TemplateVersion: "opportunities-v1",
		OutputJSON:      `{"title_key":"opportunities.fixture.title"}`,
		Status:          "success",
		CreatedAt:       now.Add(-30 * time.Minute),
	}); err != nil {
		t.Fatalf("append opportunity run: %v", err)
	}
	if _, err := store.AppendAIRun(ctx, AIRunParams{
		TeamID:          otherTeamID,
		SiteID:          otherSite.ID,
		ActorID:         userID,
		ActorType:       "user",
		Feature:         "ask_ai",
		Provider:        "bedrock",
		Model:           "openai.gpt-oss-120b-1:0",
		TemplateVersion: "ask-ai-v1",
		OutputJSON: `{
			"version":"ask-ai-output-summary-v1",
			"answer_sha256":"other-answer-hash",
			"answer_chars":99,
			"citations":[],
			"charts":[],
			"actions":[]
		}`,
		Status:    "success",
		CreatedAt: now.Add(-10 * time.Minute),
	}); err != nil {
		t.Fatalf("append other site run: %v", err)
	}
	if _, err := store.AppendAIRun(ctx, AIRunParams{
		ID:              newerRunID,
		TeamID:          teamID,
		SiteID:          siteID,
		ActorID:         userID,
		ActorType:       "user",
		Feature:         "ask_ai",
		Provider:        "bedrock",
		Model:           "openai.gpt-oss-120b-1:0",
		TemplateVersion: "ask-ai-v1",
		EvidenceIDs:     []string{"input_context", "hitkeep_get_site_overview"},
		InputHash:       "newer-input-hash",
		OutputHash:      "newer-output-hash",
		OutputJSON: `{
			"version":"ask-ai-output-summary-v1",
			"answer_sha256":"newer-answer-hash",
			"answer_chars":42,
			"citations":[
				{"tool_call_id":"input_context","label_sha256":"context-label-hash","label_chars":13},
				{"tool_call_id":"hitkeep_get_site_overview","label_sha256":"label-hash","label_chars":8}
			],
			"charts":[{"type":"line","title_sha256":"title-hash","title_chars":7,"row_count":7,"series_count":1,"series":[{"key_sha256":"key-hash","key_chars":6,"label_sha256":"series-label-hash","label_chars":6}]}],
			"actions":[
				{"type":"navigate","format":"","label_sha256":"action-label-hash","label_chars":11,"target_sha256":"target-hash","target_chars":7},
				{"type":"download_export","format":"csv","label_sha256":"export-label-hash","label_chars":14,"target_sha256":"export-target-hash","target_chars":54}
			]
		}`,
		InputTokens:     10,
		OutputTokens:    20,
		TotalTokens:     30,
		ToolCallCount:   2,
		LifecycleEvents: []AILifecycleEvent{{Type: "tool_call_start", ToolName: "hitkeep_get_site_overview", Status: "started", Timestamp: now}},
		Status:          "success",
		CreatedAt:       now,
	}); err != nil {
		t.Fatalf("append newer ask ai run: %v", err)
	}

	entries, total, err := store.ListAskAIHistory(ctx, siteID, 10, 0)
	if err != nil {
		t.Fatalf("list ask ai history: %v", err)
	}
	if total != 2 || len(entries) != 2 {
		t.Fatalf("expected two site-scoped Ask AI runs, got total=%d entries=%d", total, len(entries))
	}
	if entries[0].RunID != newerRunID || entries[1].RunID != olderRunID {
		t.Fatalf("expected newest-first Ask AI history, got %#v", entries)
	}
	newer := entries[0]
	if newer.AnswerChars != 42 || newer.CitationCount != 2 || newer.ChartCount != 1 || newer.ActionCount != 2 {
		t.Fatalf("unexpected Ask AI summary counts: %#v", newer)
	}
	if strings.Join(newer.ChartTypes, ",") != "line" {
		t.Fatalf("expected chart types to be summarized, got %#v", newer.ChartTypes)
	}
	if strings.Join(newer.ActionTypes, ",") != "download_export,navigate" {
		t.Fatalf("expected sorted action types, got %#v", newer.ActionTypes)
	}
	if strings.Join(newer.ToolNames, ",") != "hitkeep_get_site_overview" {
		t.Fatalf("expected lifecycle tool names, got %#v", newer.ToolNames)
	}
	if newer.InputHash != "newer-input-hash" || newer.OutputHash != "newer-output-hash" {
		t.Fatalf("expected audit hashes in history, got %#v", newer)
	}
	if strings.Contains(fmt.Sprintf("%#v", entries), "Traffic increased") || strings.Contains(fmt.Sprintf("%#v", entries), "Open events") {
		t.Fatalf("Ask AI history leaked raw prompt/response content: %#v", entries)
	}

	page, pageTotal, err := store.ListAskAIHistory(ctx, siteID, 1, 1)
	if err != nil {
		t.Fatalf("list paginated ask ai history: %v", err)
	}
	if pageTotal != 2 || len(page) != 1 || page[0].RunID != olderRunID {
		t.Fatalf("unexpected paginated history: total=%d page=%#v", pageTotal, page)
	}
}

func TestAppendAIRunRequiresStructuredObjectOutput(t *testing.T) {
	store, userID, siteID, teamID := setupAIStore(t)
	ctx := context.Background()

	_, err := store.AppendAIRun(ctx, AIRunParams{
		TeamID:          teamID,
		SiteID:          siteID,
		ActorID:         userID,
		ActorType:       "user",
		Feature:         "opportunities",
		Provider:        "openai",
		Model:           "gpt-test",
		TemplateVersion: "opportunities-v1",
		OutputJSON:      `"raw provider response body"`,
		Status:          "failure",
		CreatedAt:       time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("expected scalar output json to be rejected")
	}
	if !strings.Contains(err.Error(), "object") {
		t.Fatalf("expected object validation error, got %v", err)
	}
}

func TestAppendOpportunityAIRunRejectsInventedCitedEvidence(t *testing.T) {
	store, userID, siteID, teamID := setupAIStore(t)
	ctx := context.Background()

	_, err := store.AppendAIRun(ctx, AIRunParams{
		TeamID:          teamID,
		SiteID:          siteID,
		ActorID:         userID,
		ActorType:       "user",
		Feature:         "opportunities",
		Provider:        "openai",
		Model:           "gpt-test",
		TemplateVersion: "opportunities-v1",
		EvidenceIDs:     []string{"checkout-rate"},
		OutputJSON:      `{"title_key":"opportunities.fixture.title","cited_evidence_ids":["checkout-rate","invented"]}`,
		Status:          "success",
		CreatedAt:       time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("expected invented cited evidence to be rejected")
	}
	if !strings.Contains(err.Error(), "cited evidence") || !strings.Contains(err.Error(), "invented") {
		t.Fatalf("expected cited evidence validation error, got %v", err)
	}

	if _, err := store.AppendAIRun(ctx, AIRunParams{
		TeamID:          teamID,
		SiteID:          siteID,
		ActorID:         userID,
		ActorType:       "user",
		Feature:         "opportunities",
		Provider:        "openai",
		Model:           "gpt-test",
		TemplateVersion: "opportunities-v1",
		EvidenceIDs:     []string{"checkout-rate"},
		OutputJSON:      `{"title_key":"opportunities.fixture.title","cited_evidence_ids":["checkout-rate"]}`,
		Status:          "success",
		CreatedAt:       time.Now().UTC(),
	}); err != nil {
		t.Fatalf("append valid cited evidence run: %v", err)
	}
}

func TestAppendOpportunityAIRunRejectsMalformedCitedEvidence(t *testing.T) {
	store, userID, siteID, teamID := setupAIStore(t)
	ctx := context.Background()

	_, err := store.AppendAIRun(ctx, AIRunParams{
		TeamID:          teamID,
		SiteID:          siteID,
		ActorID:         userID,
		ActorType:       "user",
		Feature:         "opportunities",
		Provider:        "openai",
		Model:           "gpt-test",
		TemplateVersion: "opportunities-v1",
		EvidenceIDs:     []string{"checkout-rate"},
		OutputJSON:      `{"title_key":"opportunities.fixture.title","cited_evidence_ids":"checkout-rate"}`,
		Status:          "success",
		CreatedAt:       time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("expected malformed cited evidence to be rejected")
	}
	if !strings.Contains(err.Error(), "cited evidence") {
		t.Fatalf("expected cited evidence validation error, got %v", err)
	}
}

func TestAppendAIRunRejectsUnsafeStatusAndErrorCategory(t *testing.T) {
	store, userID, siteID, teamID := setupAIStore(t)
	ctx := context.Background()

	_, err := store.AppendAIRun(ctx, AIRunParams{
		TeamID:          teamID,
		SiteID:          siteID,
		ActorID:         userID,
		ActorType:       "user",
		Feature:         "opportunities",
		Provider:        "openai",
		Model:           "gpt-test",
		TemplateVersion: "opportunities-v1",
		OutputJSON:      `{}`,
		Status:          "provider said request failed",
		CreatedAt:       time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("expected unsafe ai run status to be rejected")
	}
	if !strings.Contains(err.Error(), "status") {
		t.Fatalf("expected status validation error, got %v", err)
	}

	_, err = store.AppendAIRun(ctx, AIRunParams{
		TeamID:          teamID,
		SiteID:          siteID,
		ActorID:         userID,
		ActorType:       "user",
		Feature:         "opportunities",
		Provider:        "openai",
		Model:           "gpt-test",
		TemplateVersion: "opportunities-v1",
		OutputJSON:      `{}`,
		Status:          "failure",
		ErrorCategory:   "provider returned sk-secret raw body",
		CreatedAt:       time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("expected unsafe ai run error category to be rejected")
	}
	if !strings.Contains(err.Error(), "error category") {
		t.Fatalf("expected error category validation error, got %v", err)
	}
}

func TestAppendAIRunRejectsUnsafeLifecycleEventMetadata(t *testing.T) {
	store, userID, siteID, teamID := setupAIStore(t)
	ctx := context.Background()

	_, err := store.AppendAIRun(ctx, AIRunParams{
		TeamID:          teamID,
		SiteID:          siteID,
		ActorID:         userID,
		ActorType:       "user",
		Feature:         "opportunities",
		Provider:        "openai",
		Model:           "gpt-test",
		TemplateVersion: "opportunities-v1",
		OutputJSON:      `{}`,
		Status:          "failure",
		LifecycleEvents: []AILifecycleEvent{{
			Type:          "provider wrote raw response",
			Status:        "provider said no",
			ErrorCategory: "sk-secret raw provider body",
		}},
		CreatedAt: time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("expected unsafe lifecycle event metadata to be rejected")
	}
	if !strings.Contains(err.Error(), "lifecycle event") {
		t.Fatalf("expected lifecycle event validation error, got %v", err)
	}
}

func TestAppendOpportunityAIRunRejectsCustomerProseOutputFields(t *testing.T) {
	store, userID, siteID, teamID := setupAIStore(t)
	ctx := context.Background()

	_, err := store.AppendAIRun(ctx, AIRunParams{
		TeamID:          teamID,
		SiteID:          siteID,
		ActorID:         userID,
		ActorType:       "user",
		Feature:         "opportunities",
		Provider:        "openai",
		Model:           "gpt-test",
		TemplateVersion: "opportunities-v1",
		OutputJSON:      `{"title":"Fix checkout conversion","summary_key":"opportunities.fixture.summary"}`,
		Status:          "success",
		CreatedAt:       time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("expected customer prose output json to be rejected")
	}
	if !strings.Contains(err.Error(), "customer prose") {
		t.Fatalf("expected customer prose validation error, got %v", err)
	}

	_, err = store.AppendAIRun(ctx, AIRunParams{
		TeamID:          teamID,
		SiteID:          siteID,
		ActorID:         userID,
		ActorType:       "user",
		Feature:         "Opportunities",
		Provider:        "openai",
		Model:           "gpt-test",
		TemplateVersion: "opportunities-v1",
		OutputJSON:      `{"summary":"Checkout conversion is leaking traffic","summary_key":"opportunities.fixture.summary"}`,
		Status:          "success",
		CreatedAt:       time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("expected customer prose output json to be rejected for case-insensitive feature")
	}
	if !strings.Contains(err.Error(), "customer prose") {
		t.Fatalf("expected customer prose validation error, got %v", err)
	}
}

func TestReserveAIRunSerializesBudgetAcrossConcurrentCallers(t *testing.T) {
	store, userID, siteID, teamID := setupAIStore(t)
	ctx := context.Background()
	const callers = 8

	var wg sync.WaitGroup
	results := make(chan error, callers)
	for range callers {
		wg.Go(func() {
			_, err := store.ReserveAIRun(ctx, AIRunParams{
				TeamID:          teamID,
				SiteID:          siteID,
				ActorID:         userID,
				ActorType:       "user",
				Feature:         "opportunities",
				Provider:        "openai",
				Model:           "gpt-test",
				TemplateVersion: "opportunities-v1",
				OutputJSON:      `{}`,
				CreatedAt:       time.Now().UTC(),
			}, time.Now().UTC().Add(-time.Hour), 1, 0)
			results <- err
		})
	}
	wg.Wait()
	close(results)

	var reserved, exhausted int
	for err := range results {
		switch {
		case err == nil:
			reserved++
		case errors.Is(err, ErrAIBudgetExhausted):
			exhausted++
		default:
			t.Fatalf("unexpected reservation error: %v", err)
		}
	}
	if reserved != 1 || exhausted != callers-1 {
		t.Fatalf("expected one reservation and %d exhausted calls, got reserved=%d exhausted=%d", callers-1, reserved, exhausted)
	}
}

func TestReserveAIRunBudgetExhaustedAuditDoesNotConsumeBudget(t *testing.T) {
	store, userID, siteID, teamID := setupAIStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	since := now.Add(-time.Hour)

	if _, err := store.AppendAIRun(ctx, AIRunParams{
		TeamID:          teamID,
		SiteID:          siteID,
		ActorID:         userID,
		ActorType:       "user",
		Feature:         "opportunities",
		Provider:        "openai",
		Model:           "gpt-test",
		TemplateVersion: "opportunities-v1",
		OutputJSON:      `{"title_key":"opportunities.fixture.title"}`,
		Status:          "success",
		CreatedAt:       now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("append success run: %v", err)
	}

	_, err := store.ReserveAIRun(ctx, AIRunParams{
		TeamID:          teamID,
		SiteID:          siteID,
		ActorID:         userID,
		ActorType:       "user",
		Feature:         "opportunities",
		Provider:        "openai",
		Model:           "gpt-test",
		TemplateVersion: "opportunities-v1",
		OutputJSON:      `{}`,
		CreatedAt:       now,
	}, since, 1, 0)
	if !errors.Is(err, ErrAIBudgetExhausted) {
		t.Fatalf("expected ErrAIBudgetExhausted, got %v", err)
	}

	usage, err := store.GetAIUsageSince(ctx, since)
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	if usage.Requests != 1 {
		t.Fatalf("expected exhausted audit row not to consume request budget, got %d requests", usage.Requests)
	}
}

func TestReserveAIRunBudgetIsGlobalAcrossTeams(t *testing.T) {
	store, userID, firstSiteID, firstTeamID := setupAIStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	since := now.Add(-time.Hour)

	secondTeam, err := store.CreateTenant(ctx, userID, "Second AI Team", "")
	if err != nil {
		t.Fatalf("create second team: %v", err)
	}
	if err := store.SetActiveTenantID(ctx, userID, secondTeam.ID); err != nil {
		t.Fatalf("set active second team: %v", err)
	}
	secondSite, err := store.CreateSite(ctx, userID, "ai-budget-second.example")
	if err != nil {
		t.Fatalf("create second site: %v", err)
	}

	if _, err := store.AppendAIRun(ctx, AIRunParams{
		TeamID:          firstTeamID,
		SiteID:          firstSiteID,
		ActorID:         userID,
		ActorType:       "user",
		Feature:         "opportunities",
		Provider:        "openai",
		Model:           "gpt-test",
		TemplateVersion: "opportunities-v1",
		OutputJSON:      `{"title_key":"opportunities.fixture.title"}`,
		Status:          "success",
		CreatedAt:       now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("append first team run: %v", err)
	}

	_, err = store.ReserveAIRun(ctx, AIRunParams{
		TeamID:          secondTeam.ID,
		SiteID:          secondSite.ID,
		ActorID:         userID,
		ActorType:       "user",
		Feature:         "opportunities",
		Provider:        "openai",
		Model:           "gpt-test",
		TemplateVersion: "opportunities-v1",
		OutputJSON:      `{}`,
		CreatedAt:       now,
	}, since, 1, 0)
	if !errors.Is(err, ErrAIBudgetExhausted) {
		t.Fatalf("expected global budget to be exhausted by first team usage, got %v", err)
	}
}

func TestOpportunityPersistenceAndStatus(t *testing.T) {
	store, _, siteID, teamID := setupAIStore(t)
	ctx := context.Background()
	opportunityID := uuid.New()

	opportunities, err := store.UpsertOpportunities(ctx, []OpportunityInput{{
		ID:         opportunityID,
		TeamID:     teamID,
		SiteID:     siteID,
		Kind:       "conversion",
		TypeKey:    "opportunities.types.checkout_conversion",
		TitleKey:   "opportunities.catalog.checkout_conversion.title",
		SummaryKey: "opportunities.catalog.checkout_conversion.summary",
		ActionKey:  "opportunities.catalog.checkout_conversion.action",
		DigestKey:  "opportunities.catalog.checkout_conversion.digest",
		CopyParams: map[string]any{
			"conversion_rate": "42%",
		},
		ImpactValue:    "$900",
		ImpactLabelKey: "opportunities.impact.checkout_starts",
		Confidence:     "medium",
		Score:          84,
		ScoreBreakdown: api.OpportunityScoreBreakdown{
			Sample:      82,
			Impact:      70,
			Urgency:     55,
			EvidenceFit: 99,
			Total:       84,
		},
		Status:           "new",
		RouteLabelKey:    "opportunities.routes.checkout",
		RouteParams:      map[string]any{"path": "/checkout"},
		RouteIcon:        "pi pi-filter",
		DetectorVersion:  "opportunities-v1",
		Evidence:         []api.OpportunityEvidence{{ID: "checkout-rate", LabelKey: "opportunities.evidence.checkout_conversion_rate", Value: "42%"}},
		CitedEvidenceIDs: []string{"checkout-rate"},
	}})
	if err != nil {
		t.Fatalf("upsert opportunities: %v", err)
	}
	if len(opportunities) != 1 {
		t.Fatalf("expected one opportunity, got %d", len(opportunities))
	}
	if opportunities[0].Evidence[0].ID != "checkout-rate" ||
		opportunities[0].Evidence[0].LabelKey != "opportunities.evidence.checkout_conversion_rate" ||
		opportunities[0].CopyParams["conversion_rate"] != "42%" ||
		opportunities[0].RouteParams["path"] != "/checkout" ||
		len(opportunities[0].CitedEvidenceIDs) != 1 ||
		opportunities[0].CitedEvidenceIDs[0] != "checkout-rate" ||
		opportunities[0].ScoreBreakdown.Total != 84 ||
		opportunities[0].ScoreBreakdown.EvidenceFit != 99 {
		t.Fatalf("opportunity JSON fields did not round trip: %#v", opportunities[0])
	}

	updated, err := store.UpdateOpportunityStatus(ctx, siteID, opportunityID, "done")
	if err != nil {
		t.Fatalf("update status: %v", err)
	}
	if updated == nil || updated.Status != "done" {
		t.Fatalf("expected updated done opportunity, got %#v", updated)
	}
}

func TestEncodeOpportunityJSONUsesEmptyCollections(t *testing.T) {
	input := OpportunityInput{}
	input.CopyParams = nil
	input.RouteParams = nil
	input.Evidence = nil
	input.CitedEvidenceIDs = nil

	encoded, err := encodeOpportunityJSON(input)
	if err != nil {
		t.Fatalf("encode opportunity JSON: %v", err)
	}
	if encoded.CopyParams != "{}" || encoded.RouteParams != "{}" || encoded.Evidence != "[]" || encoded.CitedEvidenceIDs != "[]" {
		t.Fatalf("expected encoded empty JSON collections, got copy=%s route=%s evidence=%s cited=%s", encoded.CopyParams, encoded.RouteParams, encoded.Evidence, encoded.CitedEvidenceIDs)
	}
}

func TestUpsertOpportunitiesPreservesDismissedStatusForUnchangedEvidence(t *testing.T) {
	for _, status := range []string{"dismissed", "done"} {
		t.Run(status, func(t *testing.T) {
			store, _, siteID, teamID := setupAIStore(t)
			ctx := context.Background()
			opportunityID := uuid.New()
			input := validOpportunityInputWithID(siteID, teamID, opportunityID)

			if _, err := store.UpsertOpportunities(ctx, []OpportunityInput{input}); err != nil {
				t.Fatalf("upsert opportunity: %v", err)
			}
			if _, err := store.UpdateOpportunityStatus(ctx, siteID, opportunityID, status); err != nil {
				t.Fatalf("update opportunity status: %v", err)
			}

			regenerated := input
			regenerated.Status = "new"
			regenerated.Score = input.Score + 5
			opportunities, err := store.UpsertOpportunities(ctx, []OpportunityInput{regenerated})
			if err != nil {
				t.Fatalf("regenerate opportunity: %v", err)
			}

			if len(opportunities) != 1 || opportunities[0].Status != status {
				t.Fatalf("expected unchanged %s opportunity to stay %s, got %#v", status, status, opportunities)
			}
		})
	}
}

func TestUpsertOpportunitiesResurfacesDismissedStatusWhenEvidenceChanges(t *testing.T) {
	store, _, siteID, teamID := setupAIStore(t)
	ctx := context.Background()
	opportunityID := uuid.New()
	input := validOpportunityInputWithID(siteID, teamID, opportunityID)

	if _, err := store.UpsertOpportunities(ctx, []OpportunityInput{input}); err != nil {
		t.Fatalf("upsert opportunity: %v", err)
	}
	if _, err := store.UpdateOpportunityStatus(ctx, siteID, opportunityID, "dismissed"); err != nil {
		t.Fatalf("dismiss opportunity: %v", err)
	}

	changed := input
	changed.Status = "new"
	changed.CopyParams = map[string]any{"conversion_rate": "58%"}
	changed.Evidence = []api.OpportunityEvidence{{ID: "checkout-rate", LabelKey: "opportunities.evidence.checkout_conversion_rate", Value: "58%"}}
	opportunities, err := store.UpsertOpportunities(ctx, []OpportunityInput{changed})
	if err != nil {
		t.Fatalf("regenerate opportunity: %v", err)
	}

	if len(opportunities) != 1 || opportunities[0].Status != "new" {
		t.Fatalf("expected materially changed opportunity to resurface as new, got %#v", opportunities)
	}
}

func TestUpsertOpportunitiesRejectsCrossSiteIDCollision(t *testing.T) {
	store, userID, firstSiteID, firstTeamID := setupAIStore(t)
	ctx := context.Background()
	secondSite, err := store.CreateSite(ctx, userID, "ai-store-second.example")
	if err != nil {
		t.Fatalf("create second site: %v", err)
	}
	secondTeamID, err := store.GetSiteTenantID(ctx, secondSite.ID)
	if err != nil {
		t.Fatalf("second site tenant: %v", err)
	}
	opportunityID := uuid.New()
	first := validOpportunityInputWithID(firstSiteID, firstTeamID, opportunityID)
	first.Score = 70
	if _, err := store.UpsertOpportunities(ctx, []OpportunityInput{first}); err != nil {
		t.Fatalf("upsert first opportunity: %v", err)
	}

	second := validOpportunityInputWithID(secondSite.ID, secondTeamID, opportunityID)
	second.Score = 99
	_, err = store.UpsertOpportunities(ctx, []OpportunityInput{second})
	if err == nil {
		t.Fatal("expected cross-site opportunity id collision to be rejected")
	}
	if !strings.Contains(err.Error(), "opportunity id") || !strings.Contains(err.Error(), "different site") {
		t.Fatalf("expected scoped id collision error, got %v", err)
	}

	stored, err := store.GetOpportunity(ctx, firstSiteID, opportunityID)
	if err != nil {
		t.Fatalf("get original opportunity: %v", err)
	}
	if stored == nil || stored.Score != 70 || stored.SiteID != firstSiteID {
		t.Fatalf("expected original opportunity to remain unchanged, got %#v", stored)
	}
	if other, err := store.GetOpportunity(ctx, secondSite.ID, opportunityID); err != nil || other != nil {
		t.Fatalf("expected no second-site opportunity for colliding id, got opportunity=%#v err=%v", other, err)
	}
}

func TestUpdateOpportunityStatusWithAuditRollsBackWhenAuditFails(t *testing.T) {
	store, _, siteID, teamID := setupAIStore(t)
	ctx := context.Background()
	opportunityID := uuid.New()
	_, err := store.UpsertOpportunities(ctx, []OpportunityInput{validOpportunityInputWithID(siteID, teamID, opportunityID)})
	if err != nil {
		t.Fatalf("upsert opportunities: %v", err)
	}

	updated, err := store.UpdateOpportunityStatusWithAudit(ctx, siteID, opportunityID, "done", AuditEntryParams{})
	if err == nil {
		t.Fatal("expected audit failure")
	}
	if updated != nil {
		t.Fatalf("expected no updated response on rollback, got %#v", updated)
	}
	opportunity, err := store.GetOpportunity(ctx, siteID, opportunityID)
	if err != nil {
		t.Fatalf("get opportunity: %v", err)
	}
	if opportunity == nil || opportunity.Status != "new" {
		t.Fatalf("expected status rollback to new, got %#v", opportunity)
	}
}

func TestUpsertOpportunitiesWithAuditRollsBackWhenAuditFails(t *testing.T) {
	store, _, siteID, teamID := setupAIStore(t)
	ctx := context.Background()
	input := validOpportunityInput(siteID, teamID)

	_, err := store.UpsertOpportunitiesWithAudit(ctx, []OpportunityInput{input}, AuditEntryParams{})
	if err == nil {
		t.Fatal("expected audit failure")
	}
	opportunities, err := store.ListOpportunities(ctx, siteID)
	if err != nil {
		t.Fatalf("list opportunities: %v", err)
	}
	if len(opportunities) != 0 {
		t.Fatalf("expected opportunity insert rollback, got %#v", opportunities)
	}
}

func TestOpportunityPersistenceRejectsFullTextLabelFields(t *testing.T) {
	store, _, siteID, teamID := setupAIStore(t)
	ctx := context.Background()
	input := validOpportunityInput(siteID, teamID)
	input.TitleKey = "Fix checkout conversion"

	_, err := store.UpsertOpportunities(ctx, []OpportunityInput{input})
	if err == nil {
		t.Fatalf("expected opportunity label key validation error")
	}
	if !strings.Contains(err.Error(), "title_key") || !strings.Contains(err.Error(), "translation key") {
		t.Fatalf("expected title_key translation key error, got %v", err)
	}

	opportunities, err := store.ListOpportunities(ctx, siteID)
	if err != nil {
		t.Fatalf("list opportunities: %v", err)
	}
	if len(opportunities) != 0 {
		t.Fatalf("expected invalid opportunity not to persist, got %#v", opportunities)
	}
}

func TestOpportunityPersistenceRejectsFullTextEvidenceKeys(t *testing.T) {
	store, _, siteID, teamID := setupAIStore(t)
	ctx := context.Background()
	input := validOpportunityInput(siteID, teamID)
	input.Evidence = []api.OpportunityEvidence{{
		ID:        "checkout-rate",
		LabelKey:  "Checkout conversion rate",
		Value:     "42%",
		DetailKey: "opportunities.evidence.checkout_conversion_rate_detail",
	}}

	_, err := store.UpsertOpportunities(ctx, []OpportunityInput{input})
	if err == nil {
		t.Fatalf("expected opportunity evidence key validation error")
	}
	if !strings.Contains(err.Error(), "evidence.label_key") || !strings.Contains(err.Error(), "translation key") {
		t.Fatalf("expected evidence label translation key error, got %v", err)
	}
}

func TestOpportunityPersistenceRejectsMissingCitedEvidence(t *testing.T) {
	store, _, siteID, teamID := setupAIStore(t)
	ctx := context.Background()
	input := validOpportunityInput(siteID, teamID)
	input.CitedEvidenceIDs = []string{"checkout-rate", "invented-evidence"}

	_, err := store.UpsertOpportunities(ctx, []OpportunityInput{input})
	if err == nil {
		t.Fatalf("expected missing cited evidence validation error")
	}
	if !strings.Contains(err.Error(), "cited evidence") || !strings.Contains(err.Error(), "invented-evidence") {
		t.Fatalf("expected cited evidence error, got %v", err)
	}
}

func TestOpportunityPersistenceRequiresEvidenceCitations(t *testing.T) {
	store, _, siteID, teamID := setupAIStore(t)
	ctx := context.Background()

	missingCitation := validOpportunityInput(siteID, teamID)
	missingCitation.CitedEvidenceIDs = nil
	if _, err := store.UpsertOpportunities(ctx, []OpportunityInput{missingCitation}); err == nil {
		t.Fatal("expected missing cited evidence ids to be rejected")
	} else if !strings.Contains(err.Error(), "cited evidence") {
		t.Fatalf("expected cited evidence validation error, got %v", err)
	}

	blankEvidenceID := validOpportunityInput(siteID, teamID)
	blankEvidenceID.Evidence = []api.OpportunityEvidence{{ID: "", LabelKey: "opportunities.evidence.checkout_conversion_rate", Value: "42%"}}
	blankEvidenceID.CitedEvidenceIDs = []string{""}
	if _, err := store.UpsertOpportunities(ctx, []OpportunityInput{blankEvidenceID}); err == nil {
		t.Fatal("expected blank evidence id to be rejected")
	} else if !strings.Contains(err.Error(), "evidence.id") {
		t.Fatalf("expected evidence id validation error, got %v", err)
	}
}

func TestOpportunityPersistenceRejectsRawPayloadFieldsInCustomerParams(t *testing.T) {
	store, _, siteID, teamID := setupAIStore(t)
	ctx := context.Background()

	for _, tc := range []struct {
		name    string
		input   func(OpportunityInput) OpportunityInput
		wantErr string
	}{
		{
			name: "copy params",
			input: func(input OpportunityInput) OpportunityInput {
				input.CopyParams = map[string]any{"raw_prompt": "do not persist"}
				return input
			},
			wantErr: "raw",
		},
		{
			name: "route params",
			input: func(input OpportunityInput) OpportunityInput {
				input.RouteParams = map[string]any{"provider_error_body": "do not persist"}
				return input
			},
			wantErr: "raw",
		},
		{
			name: "evidence detail params",
			input: func(input OpportunityInput) OpportunityInput {
				input.Evidence[0].DetailKey = "opportunities.evidence.checkout_conversion_rate_detail"
				input.Evidence[0].DetailParams = map[string]any{"raw_provider_response": "do not persist"}
				return input
			},
			wantErr: "raw",
		},
		{
			name: "copy param value",
			input: func(input OpportunityInput) OpportunityInput {
				input.CopyParams = map[string]any{"note": "raw_provider_response sk-secret do not persist"}
				return input
			},
			wantErr: "raw",
		},
		{
			name: "copy param credential field",
			input: func(input OpportunityInput) OpportunityInput {
				input.CopyParams = map[string]any{"access_token": "placeholder"}
				return input
			},
			wantErr: "credential",
		},
		{
			name: "route param bearer value",
			input: func(input OpportunityInput) OpportunityInput {
				input.RouteParams = map[string]any{"target": "Authorization: Bearer placeholder"}
				return input
			},
			wantErr: "raw",
		},
		{
			name: "impact value credential",
			input: func(input OpportunityInput) OpportunityInput {
				input.ImpactValue = "Bearer sk-secret"
				return input
			},
			wantErr: "raw",
		},
		{
			name: "evidence value raw provider payload",
			input: func(input OpportunityInput) OpportunityInput {
				input.Evidence[0].Value = "raw_provider_response user_agent curl/8.0"
				return input
			},
			wantErr: "raw",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := tc.input(validOpportunityInput(siteID, teamID))
			_, err := store.UpsertOpportunities(ctx, []OpportunityInput{input})
			if err == nil {
				t.Fatal("expected raw payload param field to be rejected")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected %q validation error, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestOpportunityPersistenceAllowsCredentialWordsInSafeCustomerRoutes(t *testing.T) {
	store, _, siteID, teamID := setupAIStore(t)
	ctx := context.Background()
	input := validOpportunityInput(siteID, teamID)
	input.RouteParams = map[string]any{"path": "/docs/api-key-guide"}

	if _, err := store.UpsertOpportunities(ctx, []OpportunityInput{input}); err != nil {
		t.Fatalf("expected safe customer route to be persisted: %v", err)
	}
}

func TestOpportunityPersistenceRejectsInvalidBatchWithoutPartialWrite(t *testing.T) {
	store, _, siteID, teamID := setupAIStore(t)
	ctx := context.Background()
	valid := validOpportunityInput(siteID, teamID)
	valid.ID = uuid.New()
	invalid := validOpportunityInput(siteID, teamID)
	invalid.ID = uuid.New()
	invalid.ActionKey = "Tell customers what to do"

	_, err := store.UpsertOpportunities(ctx, []OpportunityInput{valid, invalid})
	if err == nil {
		t.Fatalf("expected opportunity batch validation error")
	}
	if !strings.Contains(err.Error(), "action_key") || !strings.Contains(err.Error(), "translation key") {
		t.Fatalf("expected action_key translation key error, got %v", err)
	}

	opportunities, err := store.ListOpportunities(ctx, siteID)
	if err != nil {
		t.Fatalf("list opportunities: %v", err)
	}
	if len(opportunities) != 0 {
		t.Fatalf("expected invalid batch not to partially persist, got %#v", opportunities)
	}
}

func validOpportunityInput(siteID, teamID uuid.UUID) OpportunityInput {
	return validOpportunityInputWithID(siteID, teamID, uuid.New())
}

func validOpportunityInputWithID(siteID, teamID, id uuid.UUID) OpportunityInput {
	return OpportunityInput{
		ID:         id,
		TeamID:     teamID,
		SiteID:     siteID,
		Kind:       "conversion",
		TypeKey:    "opportunities.types.checkout_conversion",
		TitleKey:   "opportunities.catalog.checkout_conversion.title",
		SummaryKey: "opportunities.catalog.checkout_conversion.summary",
		ActionKey:  "opportunities.catalog.checkout_conversion.action",
		DigestKey:  "opportunities.catalog.checkout_conversion.digest",
		CopyParams: map[string]any{
			"conversion_rate": "42%",
		},
		ImpactValue:      "$900",
		ImpactLabelKey:   "opportunities.impact.checkout_starts",
		Confidence:       "medium",
		Score:            84,
		Status:           "new",
		RouteLabelKey:    "opportunities.routes.checkout",
		RouteParams:      map[string]any{"path": "/checkout"},
		RouteIcon:        "pi pi-filter",
		DetectorVersion:  "opportunities-v1",
		Evidence:         []api.OpportunityEvidence{{ID: "checkout-rate", LabelKey: "opportunities.evidence.checkout_conversion_rate", Value: "42%"}},
		CitedEvidenceIDs: []string{"checkout-rate"},
	}
}
