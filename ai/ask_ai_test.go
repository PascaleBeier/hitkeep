package ai

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zendev-sh/goai/provider"
)

func TestRunAskAIStreamingGenerationKeepsStreamedAnswerWhenObjectBreaks(t *testing.T) {
	service := &Service{
		conf: Config{Enabled: true, Provider: "openai", Model: "gpt-test", Timeout: time.Second},
		model: &fakeLanguageModel{
			id: "gpt-test",
			streamFn: func(context.Context, provider.GenerateParams) (*provider.StreamResult, error) {
				// The model finishes the answer, then emits a broken citations array.
				return providerStreamFromChunks(
					provider.StreamChunk{Type: provider.ChunkText, Text: `{"answer_markdown":"Traffic is stable.","citations":[{"label":`},
					provider.StreamChunk{Type: provider.ChunkFinish, FinishReason: provider.FinishStop, Usage: provider.Usage{InputTokens: 6, OutputTokens: 4}},
				), nil
			},
		},
	}

	generation := service.runAskAIStreamingGeneration(context.Background(), AskAIRequest{
		SiteID: uuid.New(), Query: "What changed?",
		From: time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC),
	}, nil)

	if generation.Err != nil {
		t.Fatalf("expected the streamed answer to survive, got %v", generation.Err)
	}
	if generation.Output.AnswerMarkdown != "Traffic is stable." || len(generation.Output.Citations) != 0 {
		t.Fatalf("output = %+v", generation.Output)
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
	})
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
}

func TestPromptCachingOnlyForProvidersWithCacheMarkers(t *testing.T) {
	for providerName, want := range map[string]int{"anthropic": 1, "Bedrock": 1, "openai": 0, "cohere": 0, "openai-compatible": 0} {
		if got := len(promptCachingOptions(Config{Provider: providerName})); got != want {
			t.Errorf("%s: %d caching options, want %d", providerName, got, want)
		}
	}
}
