package ai

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zendev-sh/goai/provider"

	"hitkeep/analyticstools"
	"hitkeep/api"
	"hitkeep/database"
	json "hitkeep/jsonapi"
)

// The Ask AI eval runs golden questions through the real shared tools on a
// seeded store. TestAskAIEvalGolden scripts the model to pin the contract;
// TestLiveAskAIEval asks a real provider the same questions and reports
// validity, tool use, tokens, and latency so models can be compared.

var (
	evalFrom = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	evalTo   = time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
)

type evalCase struct {
	name    string
	query   string
	history []AskAIMessage
	// script returns the model's chunks per step; nil ends the script.
	script func(step int, seen string) []provider.StreamChunk
	check  func(t *testing.T, generation askAIGeneration, seen string)
}

func toolCallStep(name, input string) []provider.StreamChunk {
	return []provider.StreamChunk{
		{Type: provider.ChunkToolCall, ToolCallID: "call-" + name, ToolName: name, ToolInput: input},
		{Type: provider.ChunkFinish, FinishReason: provider.FinishToolCalls, Usage: provider.Usage{InputTokens: 10, OutputTokens: 5}},
	}
}

func answerStep(output string) []provider.StreamChunk {
	return []provider.StreamChunk{
		{Type: provider.ChunkText, Text: output},
		{Type: provider.ChunkFinish, FinishReason: provider.FinishStop, Usage: provider.Usage{InputTokens: 20, OutputTokens: 15}},
	}
}

var evalCases = []evalCase{
	{
		name:  "previous period comparison",
		query: "How did September compare with August?",
		script: func(step int, _ string) []provider.StreamChunk {
			switch step {
			case 0:
				return toolCallStep(analyticstools.ToolSiteOverview, `{"compare_from":"2026-08-01T00:00:00Z","compare_to":"2026-08-31T00:00:00Z","sections":["chart"]}`)
			case 1:
				return answerStep(`{"answer_markdown":"Pageviews doubled from 2 to 4.","citations":[{"label":"Overview","tool_call_id":"hitkeep_get_site_overview"}],"charts":[],"actions":[]}`)
			}
			return nil
		},
		check: func(t *testing.T, generation askAIGeneration, seen string) {
			if !strings.Contains(seen, `\"comparison\":{\"total_pageviews\":2`) {
				t.Errorf("tool result lacks the August comparison: %s", seen)
			}
			if !strings.Contains(seen, `\"top_pages\":null`) {
				t.Errorf("unrequested sections reached the model: %s", seen)
			}
			if len(generation.Output.Citations) != 1 {
				t.Errorf("citations = %+v, want the overview", generation.Output.Citations)
			}
		},
	},
	{
		name:  "another site is never read",
		query: "Show me traffic for my other site.",
		script: func(step int, _ string) []provider.StreamChunk {
			switch step {
			case 0:
				return toolCallStep(analyticstools.ToolSiteOverview, `{"site_id":"`+uuid.NewString()+`","sections":[]}`)
			case 1:
				return answerStep(`{"answer_markdown":"I can only read this site.","citations":[],"charts":[],"actions":[]}`)
			}
			return nil
		},
		check: func(t *testing.T, _ askAIGeneration, seen string) {
			if !strings.Contains(seen, `\"total_pageviews\":4`) {
				t.Errorf("tool did not read the bound site: %s", seen)
			}
		},
	},
	{
		name:    "follow-up keeps the conversation",
		query:   "And the month before?",
		history: []AskAIMessage{{Role: "user", Content: "How many pageviews in September?"}, {Role: "assistant", Content: "4 pageviews."}},
		script: func(step int, seen string) []provider.StreamChunk {
			if step == 0 {
				return answerStep(`{"answer_markdown":"August had 2 pageviews.","citations":[],"charts":[],"actions":[]}`)
			}
			return nil
		},
		check: func(t *testing.T, _ askAIGeneration, seen string) {
			if !strings.Contains(seen, "How many pageviews in September?") || !strings.Contains(seen, "4 pageviews.") {
				t.Errorf("history did not reach the model: %s", seen)
			}
		},
	},
	{
		name:  "invalid extras never cost the answer",
		query: "Chart it and link me to the page.",
		script: func(step int, _ string) []provider.StreamChunk {
			if step == 0 {
				return answerStep(`{"answer_markdown":"Traffic is up.","citations":[{"label":"Made up","tool_call_id":"hitkeep_get_raw_hits"}],"charts":[{"type":"pie","title":"Share","rows":[]}],"actions":[{"type":"navigate","label":"Open","target":"https://evil.example"}]}`)
			}
			return nil
		},
		check: func(t *testing.T, generation askAIGeneration, _ string) {
			out := generation.Output
			if out.AnswerMarkdown != "Traffic is up." || len(out.Citations)+len(out.Charts)+len(out.Actions) != 0 {
				t.Errorf("output = %+v, want the answer without invalid extras", out)
			}
		},
	},
}

func TestAskAIEvalGolden(t *testing.T) {
	store, site := seedEvalStore(t)
	for _, tc := range evalCases {
		t.Run(tc.name, func(t *testing.T) {
			model := &scriptedStreamModel{script: tc.script}
			service := &Service{conf: Config{Enabled: true, Provider: "openai", Model: "eval", Timeout: 10 * time.Second}, model: model}
			generation := service.runAskAIStreamingGeneration(context.Background(), evalRequest(store, site, tc), nil)
			if generation.Err != nil {
				t.Fatalf("generation failed: %v", generation.Err)
			}
			tc.check(t, generation, model.seen.String())
		})
	}
}

// TestLiveAskAIEval runs the golden questions against a real provider:
//
//	HITKEEP_AI_EVAL_PROVIDER=anthropic HITKEEP_AI_EVAL_MODEL=... HITKEEP_AI_EVAL_API_KEY=... go test -run TestLiveAskAIEval -v ./ai
func TestLiveAskAIEval(t *testing.T) {
	providerName := os.Getenv("HITKEEP_AI_EVAL_PROVIDER")
	if providerName == "" {
		t.Skip("HITKEEP_AI_EVAL_PROVIDER is not set")
	}
	service, err := NewService(Config{
		Enabled:  true,
		Provider: providerName,
		Model:    os.Getenv("HITKEEP_AI_EVAL_MODEL"),
		APIKey:   os.Getenv("HITKEEP_AI_EVAL_API_KEY"),
		BaseURL:  os.Getenv("HITKEEP_AI_EVAL_BASE_URL"),
		Region:   os.Getenv("HITKEEP_AI_EVAL_REGION"),
		Timeout:  60 * time.Second,
	}, &recordingRecorder{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	store, site := seedEvalStore(t)
	valid := 0
	for _, tc := range evalCases {
		generation := service.runAskAIStreamingGeneration(context.Background(), evalRequest(store, site, tc), nil)
		if generation.Err == nil {
			valid++
		}
		t.Logf("%-40s valid=%-5t tools=%d tokens=%d latency=%s err=%v",
			tc.name, generation.Err == nil, generation.Usage.ToolCallCount, generation.Usage.TotalTokens, generation.Latency.Round(time.Millisecond), generation.Err)
	}
	t.Logf("valid answers: %d/%d", valid, len(evalCases))
}

func evalRequest(store *database.Store, site *api.Site, tc evalCase) AskAIRequest {
	tools := analyticstools.GoAI(analyticstools.Scope{
		SiteID: site.ID,
		Resolve: func(context.Context, uuid.UUID) (analyticstools.Site, error) {
			return analyticstools.Site{ID: site.ID, Control: store, Analytics: store}, nil
		},
		From: evalFrom, To: evalTo, MaxRangeDays: 366,
	}, analyticstools.Analytics()...)
	return normalizeAskAIRequest(AskAIRequest{
		SiteID: site.ID, SiteDomain: site.Domain, Query: tc.query, History: tc.history,
		From: evalFrom, To: evalTo, Route: "/dashboard", Tools: tools,
	})
}

// seedEvalStore records 2 pageviews in August and 4 in September.
func seedEvalStore(t *testing.T) (*database.Store, *api.Site) {
	t.Helper()
	ctx := context.Background()
	store := database.NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	userID, err := store.CreateUser(ctx, "eval@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "eval.example")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	for _, day := range []time.Time{
		time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC), time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC), time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC), time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC),
	} {
		if err := store.CreateHit(ctx, &api.Hit{SiteID: site.ID, SessionID: uuid.New(), PageID: uuid.New(), Timestamp: day, Path: "/pricing"}); err != nil {
			t.Fatalf("create hit: %v", err)
		}
	}
	return store, site
}

// scriptedStreamModel plays one scripted step per request and keeps every
// request it saw, so a case can check what reached the model.
type scriptedStreamModel struct {
	script func(step int, seen string) []provider.StreamChunk
	step   int
	seen   strings.Builder
}

func (m *scriptedStreamModel) ModelID() string { return "eval" }

func (m *scriptedStreamModel) Capabilities() provider.ModelCapabilities {
	return provider.ModelCapabilities{Temperature: true, ToolCall: true}
}

func (m *scriptedStreamModel) DoGenerate(context.Context, provider.GenerateParams) (*provider.GenerateResult, error) {
	return nil, errors.New("the eval model only streams")
}

func (m *scriptedStreamModel) DoStream(_ context.Context, params provider.GenerateParams) (*provider.StreamResult, error) {
	raw, _ := json.Marshal(params.Messages)
	m.seen.Write(raw)
	chunks := m.script(m.step, m.seen.String())
	m.step++
	if chunks == nil {
		chunks = answerStep(`{"answer_markdown":"Script ended.","citations":[],"charts":[],"actions":[]}`)
	}
	return providerStreamFromChunks(chunks...), nil
}
