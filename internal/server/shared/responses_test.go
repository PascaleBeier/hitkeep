package shared

import (
	"context"
	"testing"
	"time"

	"hitkeep/internal/config"
	"hitkeep/internal/database"
)

type askAIUsageStore struct {
	usage database.AIUsageSummary
}

func (s askAIUsageStore) GetAIUsageSince(context.Context, time.Time) (database.AIUsageSummary, error) {
	return s.usage, nil
}

func TestAskAIStatusGating(t *testing.T) {
	tests := []struct {
		name       string
		cfg        *config.Config
		usage      database.AIUsageSummary
		wantStatus string
		available  bool
		enabled    bool
	}{
		{
			name:       "disabled",
			cfg:        &config.Config{},
			wantStatus: "disabled",
		},
		{
			name: "ai enabled ask ai disabled",
			cfg: &config.Config{
				AIEnabled:  true,
				AIProvider: "bedrock",
				AIModel:    "amazon.nova-lite-v1:0",
			},
			wantStatus: "disabled",
		},
		{
			name: "not configured",
			cfg: &config.Config{
				AIEnabled:    true,
				AskAIEnabled: true,
			},
			wantStatus: "not_configured",
			enabled:    true,
		},
		{
			name: "available",
			cfg: &config.Config{
				AIEnabled:             true,
				AskAIEnabled:          true,
				AIProvider:            "bedrock",
				AIModel:               "amazon.nova-lite-v1:0",
				AIRequestLimit:        10,
				AITokenLimit:          1000,
				AIBudgetWindowMinutes: 60,
			},
			wantStatus: "available",
			available:  true,
			enabled:    true,
		},
		{
			name: "budget exhausted",
			cfg: &config.Config{
				AIEnabled:             true,
				AskAIEnabled:          true,
				AIProvider:            "bedrock",
				AIModel:               "amazon.nova-lite-v1:0",
				AIRequestLimit:        1,
				AITokenLimit:          1000,
				AIBudgetWindowMinutes: 60,
			},
			usage:      database.AIUsageSummary{Requests: 1},
			wantStatus: "budget_exhausted",
			enabled:    true,
		},
		{
			name: "mantle instance role missing region",
			cfg: &config.Config{
				AIEnabled:    true,
				AskAIEnabled: true,
				AIProvider:   "openai-compatible",
				AIModel:      "openai.gpt-oss-120b",
				AIBaseURL:    "https://bedrock-mantle.eu-central-1.api.aws/v1",
			},
			wantStatus: "not_configured",
			enabled:    true,
		},
		{
			name: "mantle instance role with region",
			cfg: &config.Config{
				AIEnabled:    true,
				AskAIEnabled: true,
				AIProvider:   "openai-compatible",
				AIModel:      "openai.gpt-oss-120b",
				AIBaseURL:    "https://bedrock-mantle.eu-central-1.api.aws/v1",
				AIRegion:     "eu-central-1",
			},
			wantStatus: "available",
			available:  true,
			enabled:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status := AskAIStatus(context.Background(), tc.cfg, askAIUsageStore{usage: tc.usage})
			if status.Status != tc.wantStatus {
				t.Fatalf("expected status %q, got %#v", tc.wantStatus, status)
			}
			if status.Available != tc.available || status.Enabled != tc.enabled {
				t.Fatalf("unexpected availability: %#v", status)
			}
		})
	}
}
