package ai

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zendev-sh/goai/provider"

	"hitkeep/analyticstools"
	"hitkeep/api"
	"hitkeep/database"
	json "hitkeep/jsonapi"
	"hitkeep/skills"
)

// The Ask AI eval runs questions through the real shared tools on a seeded
// store. TestAskAIEvalGolden scripts the model to pin the contract in CI.
// TestLiveAskAIEval asks real models questions with known answers and checks
// the scores against a committed baseline; it runs only locally.

var (
	evalFrom = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	evalTo   = time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
)

// seedEvalStore records August and September traffic with distinctive totals:
//
//	August:    37 pageviews (/pricing 25, /blog 12)
//	September: 58 pageviews (/pricing 30, /blog 20, /docs 8),
//	           18 of them referred by news.ycombinator.com, 40 from Germany,
//	           and a team note about the pricing page redesign.
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
	hn, germany, us := "https://news.ycombinator.com/item?id=1", "DE", "US"
	visits := 0
	hit := func(month time.Month, path string, count int, referrer *string) {
		for i := range count {
			country := &us
			if month == time.September && visits%58 < 40 {
				country = &germany
			}
			visits++
			day := time.Date(2026, month, 1+i%28, 12, 0, 0, 0, time.UTC)
			if err := store.CreateHit(ctx, &api.Hit{
				SiteID: site.ID, SessionID: uuid.New(), PageID: uuid.New(), Timestamp: day,
				Path: path, Referrer: referrer, CountryCode: country,
			}); err != nil {
				t.Fatalf("create hit: %v", err)
			}
		}
	}
	hit(time.August, "/pricing", 25, nil)
	hit(time.August, "/blog", 12, nil)
	visits = 0
	hit(time.September, "/pricing", 30, nil)
	hit(time.September, "/blog", 18, &hn)
	hit(time.September, "/blog", 2, nil)
	hit(time.September, "/docs", 8, nil)
	if _, err := store.CreateAnnotation(ctx, site.ID, api.AnnotationInput{
		StartsAt: time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC), Body: "Launched the pricing page redesign",
	}, userID); err != nil {
		t.Fatalf("create annotation: %v", err)
	}
	return store, site
}

func evalRequest(store *database.Store, site *api.Site, query string, history []AskAIMessage) AskAIRequest {
	aiTools := analyticstools.GoAI(analyticstools.Scope{
		SiteID: site.ID,
		Resolve: func(context.Context, uuid.UUID) (analyticstools.Site, error) {
			return analyticstools.Site{ID: site.ID, Control: store, Analytics: store}, nil
		},
		From: evalFrom, To: evalTo, MaxRangeDays: 366,
	}, analyticstools.Analytics()...)
	return normalizeAskAIRequest(AskAIRequest{
		SiteID: site.ID, SiteDomain: site.Domain, Query: query, History: history,
		From: evalFrom, To: evalTo, Route: "/dashboard", Tools: aiTools,
		SkillText:  skills.EmbeddedAnalyticsProcedurePack(),
		Snapshot:   analyticstools.Snapshot(evalFrom, evalTo),
		ToolTitles: analyticstools.Titles(analyticstools.Analytics()...),
	})
}

type goldenCase struct {
	name    string
	query   string
	history []AskAIMessage
	// script returns the model's chunks per step; nil ends the script.
	script func(step int) []provider.StreamChunk
	check  func(t *testing.T, generation askAIGeneration, seen string)
}

func toolCallStep(name, input string) []provider.StreamChunk {
	return []provider.StreamChunk{
		{Type: provider.ChunkToolCall, ToolCallID: "call-" + name, ToolName: name, ToolInput: input},
		{Type: provider.ChunkFinish, FinishReason: provider.FinishToolCalls, Usage: provider.Usage{InputTokens: 10, OutputTokens: 5}},
	}
}

func answerStep(answer string) []provider.StreamChunk {
	return []provider.StreamChunk{
		{Type: provider.ChunkText, Text: answer},
		{Type: provider.ChunkFinish, FinishReason: provider.FinishStop, Usage: provider.Usage{InputTokens: 20, OutputTokens: 15}},
	}
}

var goldenCases = []goldenCase{
	{
		name:  "previous period comparison",
		query: "How did September compare with August?",
		script: func(step int) []provider.StreamChunk {
			switch step {
			case 0:
				return toolCallStep(analyticstools.ToolSiteOverview, `{"compare_from":"2026-08-01T00:00:00Z","compare_to":"2026-08-31T00:00:00Z","sections":["chart"]}`)
			case 1:
				return answerStep("Pageviews rose from 37 to 58.")
			}
			return nil
		},
		check: func(t *testing.T, generation askAIGeneration, seen string) {
			if !strings.Contains(seen, `\"compare_from\":\"2026-08-01T00:00:00Z\"`) || !strings.Contains(seen, `\"comparison\":{\"total_pageviews\":37`) {
				t.Errorf("tool result lacks the August comparison: %s", seen)
			}
			if !strings.Contains(seen, `\"top_pages\":null`) {
				t.Errorf("unrequested sections reached the model: %s", seen)
			}
			if got := generation.Output.Citations; len(got) != 2 || got[1].ToolCallID != analyticstools.ToolSiteOverview || got[1].Label != "Get HitKeep Site Overview" {
				t.Errorf("citations = %+v, want the snapshot tools with titles", got)
			}
		},
	},
	{
		name:  "another site is never read",
		query: "Show me traffic for my other site.",
		script: func(step int) []provider.StreamChunk {
			switch step {
			case 0:
				return toolCallStep(analyticstools.ToolSiteOverview, `{"site_id":"`+uuid.NewString()+`","sections":[]}`)
			case 1:
				return answerStep("I can only read this site.")
			}
			return nil
		},
		check: func(t *testing.T, _ askAIGeneration, seen string) {
			if !strings.Contains(seen, `\"total_pageviews\":58`) {
				t.Errorf("tool did not read the bound site: %s", seen)
			}
		},
	},
	{
		name:    "follow-up keeps the conversation",
		query:   "And the month before?",
		history: []AskAIMessage{{Role: "user", Content: "How many pageviews in September?"}, {Role: "assistant", Content: "58 pageviews."}},
		script: func(step int) []provider.StreamChunk {
			if step == 0 {
				return answerStep("August had 37 pageviews.")
			}
			return nil
		},
		check: func(t *testing.T, _ askAIGeneration, seen string) {
			if !strings.Contains(seen, "How many pageviews in September?") || !strings.Contains(seen, "58 pageviews.") {
				t.Errorf("history did not reach the model: %s", seen)
			}
			// The snapshot reached the model before it called any tool.
			if !strings.Contains(seen, `\"total_pageviews\":58`) || !strings.Contains(seen, "pricing page redesign") {
				t.Errorf("snapshot did not reach the model: %s", seen)
			}
		},
	},
	{
		name:  "invalid charts are corrected, never fatal",
		query: "Chart the top pages.",
		script: func(step int) []provider.StreamChunk {
			switch step {
			case 0:
				return toolCallStep("show_chart", `{"type":"pie","title":"Share","columns":[],"rows":[]}`)
			case 1:
				return toolCallStep("show_chart", `{"type":"table","title":"Top pages","columns":["path","pageviews"],"rows":[["/pricing","30"]]}`)
			case 2:
				return answerStep("/pricing led September.")
			}
			return nil
		},
		check: func(t *testing.T, generation askAIGeneration, seen string) {
			if !strings.Contains(seen, "unsupported chart type") {
				t.Errorf("the chart error did not reach the model: %s", seen)
			}
			out := generation.Output
			if out.AnswerMarkdown != "/pricing led September." || len(out.Charts) != 1 || out.Charts[0].Title != "Top pages" {
				t.Errorf("output = %+v, want the answer and the corrected chart", out)
			}
		},
	},
}

func TestAskAIEvalGolden(t *testing.T) {
	store, site := seedEvalStore(t)
	for _, tc := range goldenCases {
		t.Run(tc.name, func(t *testing.T) {
			model := &scriptedStreamModel{script: tc.script}
			service := &Service{conf: Config{Enabled: true, Provider: "openai", Model: "eval", Timeout: 10 * time.Second}, model: model}
			generation := service.runAskAIStreamingGeneration(context.Background(), evalRequest(store, site, tc.query, tc.history), nil)
			if generation.Err != nil {
				t.Fatalf("generation failed: %v", generation.Err)
			}
			tc.check(t, generation, model.seen.String())
		})
	}
}

// liveCase passes when the answer is valid, mentions one term from every
// group in want, and mentions nothing in forbid. The request range is
// September 2026.
type liveCase struct {
	name   string
	query  string
	want   [][]string
	forbid []string
}

var liveCases = []liveCase{
	{name: "range total", query: "How many pageviews did the site get in this period?", want: [][]string{{"58"}}},
	{name: "previous period", query: "How did pageviews compare with the previous month, August 2026?", want: [][]string{{"58"}, {"37"}}},
	{name: "top page", query: "Which page got the most pageviews in this period?", want: [][]string{{"/pricing"}}},
	{name: "filtered page", query: "How many pageviews did /blog get in this period?", want: [][]string{{"20"}}},
	{name: "top referrer", query: "Which external referrer sent the most traffic in this period?", want: [][]string{{"ycombinator"}}},
	{name: "top country", query: "Which country sent the most visitors in this period?", want: [][]string{{"Germany", "DE"}}},
	{name: "team notes", query: "Did the team note anything this month that could explain the traffic?", want: [][]string{{"redesign"}}},
	{name: "off-topic refusal", query: "Write a Python function that sorts a list.", forbid: []string{"def "}},
}

func (c liveCase) passes(generation askAIGeneration) bool {
	if generation.Err != nil {
		return false
	}
	answer := generation.Output.AnswerMarkdown
	for _, group := range c.want {
		if !mentionsAny(answer, group) {
			return false
		}
	}
	return !mentionsAny(answer, c.forbid)
}

// mentionsAny matches whole terms only, so "20" never matches "1,240" and
// "DE" never matches "provides".
func mentionsAny(text string, terms []string) bool {
	for _, term := range terms {
		if regexp.MustCompile(`(?i)(^|[^\pL\pN])` + regexp.QuoteMeta(term) + `($|[^\pL\pN])`).MatchString(text) {
			return true
		}
	}
	return false
}

const evalBaselinePath = "testdata/ask_ai_eval_baseline.json"

type evalScore struct {
	Passed int `json:"passed"`
	Total  int `json:"total"`
}

// TestLiveAskAIEval scores real models on questions with known answers. It is
// local-only: it skips unless a provider is set, and CI sets none.
//
//	HITKEEP_AI_EVAL_PROVIDER=openai-compatible HITKEEP_AI_EVAL_BASE_URL=$CI_BASE_URL \
//	HITKEEP_AI_EVAL_API_KEY=$CI_API_KEY HITKEEP_AI_EVAL_MODELS=gpt-5-nano,claude-haiku-4.5 \
//	go test -run TestLiveAskAIEval -v ./ai
//
// Add HITKEEP_AI_EVAL_UPDATE=1 to record the scores as the new baseline.
// Otherwise a model that scores more than one case below its baseline fails.
func TestLiveAskAIEval(t *testing.T) {
	providerName := os.Getenv("HITKEEP_AI_EVAL_PROVIDER")
	if providerName == "" {
		t.Skip("HITKEEP_AI_EVAL_PROVIDER is not set")
	}
	baseline := map[string]evalScore{}
	if raw, err := os.ReadFile(evalBaselinePath); err == nil {
		if err := json.Unmarshal(raw, &baseline); err != nil {
			t.Fatalf("decode baseline: %v", err)
		}
	}
	store, site := seedEvalStore(t)
	var mu sync.Mutex
	scores := map[string]evalScore{}
	// Parallel model subtests run after this function returns, so the
	// baseline is written once they have all finished.
	t.Cleanup(func() {
		if os.Getenv("HITKEEP_AI_EVAL_UPDATE") == "" {
			return
		}
		maps.Copy(baseline, scores)
		raw, err := json.MarshalIndent(baseline, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(evalBaselinePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(evalBaselinePath, append(raw, '\n'), 0o644); err != nil {
			t.Fatalf("write baseline: %v", err)
		}
	})
	for model := range strings.SplitSeq(os.Getenv("HITKEEP_AI_EVAL_MODELS"), ",") {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		t.Run(model, func(t *testing.T) {
			t.Parallel()
			service, err := NewService(Config{
				Enabled: true, Provider: providerName, Model: model,
				APIKey: os.Getenv("HITKEEP_AI_EVAL_API_KEY"), BaseURL: os.Getenv("HITKEEP_AI_EVAL_BASE_URL"),
				Region: os.Getenv("HITKEEP_AI_EVAL_REGION"), Timeout: 90 * time.Second,
			}, &recordingRecorder{})
			if err != nil {
				t.Fatalf("NewService: %v", err)
			}
			score := evalScore{Total: len(liveCases)}
			for _, tc := range liveCases {
				generation := service.runAskAIStreamingGeneration(context.Background(), evalRequest(store, site, tc.query, nil), nil)
				passed := tc.passes(generation)
				if passed {
					score.Passed++
				}
				t.Logf("%-18s pass=%-5t tools=%d tokens=%-6d latency=%-6s err=%v answer=%q",
					tc.name, passed, generation.Usage.ToolCallCount, generation.Usage.TotalTokens,
					generation.Latency.Round(100*time.Millisecond), generation.Err, truncateUTF8(generation.Output.AnswerMarkdown, 160))
			}
			mu.Lock()
			scores[model] = score
			mu.Unlock()
			t.Logf("score %d/%d (baseline %s)", score.Passed, score.Total, describeBaseline(baseline, model))
			if want, ok := baseline[model]; ok && os.Getenv("HITKEEP_AI_EVAL_UPDATE") == "" && score.Passed < want.Passed-1 {
				t.Errorf("%s scored %d/%d, below its baseline of %d/%d", model, score.Passed, score.Total, want.Passed, want.Total)
			}
		})
	}
}

func describeBaseline(baseline map[string]evalScore, model string) string {
	if score, ok := baseline[model]; ok {
		return fmt.Sprintf("%d/%d", score.Passed, score.Total)
	}
	return "none"
}

// scriptedStreamModel plays one scripted step per request and keeps every
// request it saw, so a case can check what reached the model.
type scriptedStreamModel struct {
	script func(step int) []provider.StreamChunk
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
	chunks := m.script(m.step)
	m.step++
	if chunks == nil {
		chunks = answerStep("Script ended.")
	}
	return providerStreamFromChunks(chunks...), nil
}
