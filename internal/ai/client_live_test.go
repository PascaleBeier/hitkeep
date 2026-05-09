package ai

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestLiveOpenAIOpportunityCandidateProposalSmoke(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY is not set")
	}
	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		model = "gpt-4o-mini"
	}
	recorder := &recordingRecorder{}
	service, err := NewService(Config{
		Enabled:             true,
		Provider:            "openai",
		Model:               model,
		APIKey:              apiKey,
		Timeout:             30 * time.Second,
		RequestLimit:        10,
		TokenLimit:          20000,
		BudgetWindowMinutes: 60,
	}, recorder)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	result, err := service.GenerateOpportunityProposal(ctx, OpportunityRequest{
		TeamID:        uuid.New(),
		SiteID:        uuid.New(),
		ActorID:       uuid.New(),
		ActorType:     "local_smoke",
		DetectorInput: structuredDetectorInput(),
	})
	if err != nil {
		t.Fatalf("GenerateOpportunityProposal: %v", err)
	}
	if result.RunID == uuid.Nil {
		t.Fatal("expected recorded run id")
	}
	if len(result.Proposal.CitedEvidenceIDs) == 0 {
		t.Fatalf("expected cited evidence ids, got %+v", result.Proposal)
	}
	if len(recorder.runs) != 1 || recorder.runs[0].Status != "success" {
		t.Fatalf("expected one successful audit run, got %+v", recorder.runs)
	}
}
