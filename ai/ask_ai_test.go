package ai

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	goaisdk "github.com/zendev-sh/goai"
	"github.com/zendev-sh/goai/provider"
)

func TestRunAskAIStreamingGenerationAnswersInPlainText(t *testing.T) {
	var streamed strings.Builder
	calls := 0
	service := &Service{
		conf: Config{Enabled: true, Provider: "openai", Model: "gpt-test", Timeout: time.Second},
		model: &fakeLanguageModel{
			id: "gpt-test",
			streamFn: func(_ context.Context, params provider.GenerateParams) (*provider.StreamResult, error) {
				calls++
				if calls == 1 {
					// Text before a tool call is draft, not the answer.
					return providerStreamFromChunks(
						provider.StreamChunk{Type: provider.ChunkText, Text: "Let me chart that. "},
						provider.StreamChunk{Type: provider.ChunkToolCall, ToolCallID: "c1", ToolName: "show_chart", ToolInput: `{"type":"table","title":"Pages","rows":[{"path":"/pricing","pageviews":30}]}`},
						provider.StreamChunk{Type: provider.ChunkFinish, FinishReason: provider.FinishToolCalls},
					), nil
				}
				return providerStreamFromChunks(
					provider.StreamChunk{Type: provider.ChunkText, Text: "**/pricing** led with 30 pageviews."},
					provider.StreamChunk{Type: provider.ChunkFinish, FinishReason: provider.FinishStop, Usage: provider.Usage{InputTokens: 6, OutputTokens: 4}},
				), nil
			},
		},
	}
	overview := goaisdk.NewTool("hitkeep_get_site_overview", "overview", func(context.Context, struct{}) (string, error) {
		return `{"evidence_id":"hitkeep_get_site_overview","data":{"total_pageviews":58}}`, nil
	})

	generation := service.runAskAIStreamingGeneration(context.Background(), AskAIRequest{
		SiteID: uuid.New(), Query: "Which page led?",
		From: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC),
		Tools:      []goaisdk.Tool{overview},
		Snapshot:   map[string]string{"hitkeep_get_site_overview": `{}`, "hitkeep_not_offered": `{}`},
		ToolTitles: map[string]string{"hitkeep_get_site_overview": "Site overview"},
	}, func(delta AskAIStreamDelta) error {
		streamed.WriteString(delta.TextDelta)
		return nil
	})

	if generation.Err != nil {
		t.Fatalf("generation failed: %v", generation.Err)
	}
	out := generation.Output
	if out.AnswerMarkdown != "**/pricing** led with 30 pageviews." {
		t.Fatalf("answer = %q, want only the final step", out.AnswerMarkdown)
	}
	if !strings.Contains(streamed.String(), "Let me chart that.") {
		t.Fatalf("draft text was not streamed: %q", streamed.String())
	}
	// The snapshot ran and is the only evidence: the chart tool is not a source.
	if len(out.Citations) != 1 || out.Citations[0] != (AskAICitation{Label: "Site overview", ToolCallID: "hitkeep_get_site_overview"}) {
		t.Fatalf("citations = %+v", out.Citations)
	}
	if len(out.Charts) != 1 || out.Charts[0].Title != "Pages" || len(out.Actions) != 0 {
		t.Fatalf("charts = %+v, actions = %+v", out.Charts, out.Actions)
	}
}

func TestAskAIOutputToolsRejectInvalidInputAndCap(t *testing.T) {
	run := newAskAIRun(Config{}, AskAIRequest{SiteID: uuid.New()}, nil, func() {})
	tools := run.outputTools()
	chart, action := tools[0], tools[1]

	if _, err := chart.Execute(context.Background(), []byte(`{"type":"pie","title":"Share","rows":[]}`)); err == nil {
		t.Fatal("unsupported chart type accepted")
	}
	if _, err := action.Execute(context.Background(), []byte(`{"type":"navigate","label":"Open","target":"https://evil.example"}`)); err == nil {
		t.Fatal("off-dashboard navigation accepted")
	}
	for i := range askAIMaxCharts + 1 {
		_, err := chart.Execute(context.Background(), []byte(`{"type":"table","title":"T","rows":[]}`))
		if (err != nil) != (i == askAIMaxCharts) {
			t.Fatalf("chart %d: err = %v", i, err)
		}
	}
}

func TestValidateAskAIAction(t *testing.T) {
	siteID := uuid.New()
	for target, want := range map[string]string{
		"/dashboard":                         "/dashboard",
		"/ai-agents":                         "/ai-agents",
		"/ai-visibility":                     "/ai-visibility", // the dashboard redirects it to /ai-agents
		"https://example.com/phish":          "",
		"/dashboard?raw=Traffic%20increased": "",
		"/events#latest":                     "",
		"/ai-agents/unknown-tab":             "",
		"/ai-agents-secret":                  "",
	} {
		action, err := validateAskAIAction(AskAIAction{Type: "navigate", Label: "Open", Target: target}, siteID)
		if (err == nil) != (want != "") || (err == nil && action.Target != want) {
			t.Errorf("navigate %q = %q, %v; want %q", target, action.Target, err, want)
		}
	}
	action, err := validateAskAIAction(AskAIAction{Type: "download_export", Label: "Export", Target: "ignored", Format: "csv"}, siteID)
	if err != nil || action.Target != "/api/sites/"+siteID.String()+"/takeout?format=csv" {
		t.Fatalf("export = %+v, %v", action, err)
	}
}

func TestAskAIMessagesReplayHistoryAsTurns(t *testing.T) {
	messages, err := askAIMessages(AskAIRequest{
		Query: "And last month?",
		History: []AskAIMessage{
			{Role: "assistant", Content: "Welcome."},
			{Role: "user", Content: "How is traffic?"},
			{Role: "assistant", Content: "Traffic is stable."},
		},
	}, `{"evidence_id":"hitkeep_get_site_overview"}`)
	if err != nil {
		t.Fatal(err)
	}
	roles := make([]provider.Role, 0, len(messages))
	for _, message := range messages {
		roles = append(roles, message.Role)
	}
	// The leading assistant turn is dropped: providers expect a user turn first.
	want := []provider.Role{provider.RoleUser, provider.RoleAssistant, provider.RoleUser}
	if len(roles) != len(want) || roles[0] != want[0] || roles[1] != want[1] || roles[2] != want[2] {
		t.Fatalf("roles = %v, want %v", roles, want)
	}
	if last := messages[len(messages)-1].Content[0].Text; !strings.Contains(last, "Analytics already read") {
		t.Fatalf("snapshot missing from the question: %q", last)
	}
}

func TestPromptCachingOnlyForProvidersWithCacheMarkers(t *testing.T) {
	for providerName, want := range map[string]int{"anthropic": 1, "Bedrock": 1, "openai": 0, "cohere": 0, "openai-compatible": 0} {
		if got := len(promptCachingOptions(Config{Provider: providerName})); got != want {
			t.Errorf("%s: %d caching options, want %d", providerName, got, want)
		}
	}
}
