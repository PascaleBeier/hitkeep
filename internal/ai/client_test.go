package ai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	goaisdk "github.com/zendev-sh/goai"
	"github.com/zendev-sh/goai/provider"
)

type recordingRecorder struct {
	mu    sync.Mutex
	usage BudgetUsage
	runs  []RunRecord
}

func (r *recordingRecorder) RecordAIRun(_ context.Context, run RunRecord) (uuid.UUID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := uuid.Must(uuid.NewV7())
	if run.ID != uuid.Nil {
		id = run.ID
		for i := range r.runs {
			if r.runs[i].ID == id {
				run.ID = id
				r.runs[i] = run
				return id, nil
			}
		}
	}
	run.ID = id
	r.runs = append(r.runs, run)
	return id, nil
}

func (r *recordingRecorder) ReserveAIRun(_ context.Context, run RunRecord, _ time.Time, requestLimit, tokenLimit int) (uuid.UUID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	requests := r.usage.Requests + len(r.runs)
	tokens := r.usage.Tokens
	for _, existing := range r.runs {
		tokens += existing.Usage.TotalTokens
	}
	if requestLimit > 0 && requests >= requestLimit {
		id := uuid.Must(uuid.NewV7())
		run.ID = id
		run.Status = "failure"
		run.ErrorCategory = "budget_exhausted"
		r.runs = append(r.runs, run)
		return uuid.Nil, ErrBudgetExhausted
	}
	if tokenLimit > 0 && tokens >= tokenLimit {
		id := uuid.Must(uuid.NewV7())
		run.ID = id
		run.Status = "failure"
		run.ErrorCategory = "budget_exhausted"
		r.runs = append(r.runs, run)
		return uuid.Nil, ErrBudgetExhausted
	}
	id := uuid.Must(uuid.NewV7())
	run.ID = id
	r.runs = append(r.runs, run)
	return id, nil
}

func (r *recordingRecorder) GetAIUsageSince(context.Context, time.Time) (BudgetUsage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.usage, nil
}

func TestRunLedgerReserveRecordsBudgetExhaustedWithoutGeneration(t *testing.T) {
	recorder := &recordingRecorder{usage: BudgetUsage{Requests: 1}}
	ledger := newRunLedger(Config{
		Provider:            "openai",
		Model:               "gpt-test",
		RequestLimit:        1,
		TokenLimit:          100,
		BudgetWindowMinutes: 60,
	}, recorder)

	runID, err := ledger.reserve(context.Background(), OpportunityRequest{
		TeamID:        uuid.New(),
		SiteID:        uuid.New(),
		ActorID:       uuid.New(),
		ActorType:     "user",
		DetectorInput: structuredDetectorInput(),
	})

	if !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("expected ErrBudgetExhausted, got %v", err)
	}
	if runID != uuid.Nil {
		t.Fatalf("expected no reserved run id on exhausted budget, got %s", runID)
	}
	run := requireOneRecordedRun(t, recorder)
	if run.Status != "failure" || run.ErrorCategory != "budget_exhausted" {
		t.Fatalf("expected safe budget audit run, got status=%q category=%q", run.Status, run.ErrorCategory)
	}
	if run.OutputJSON != "{}" {
		t.Fatalf("expected empty output json, got %q", run.OutputJSON)
	}
}

func TestRunLedgerFinalizeGenerationRecordsSafeFailure(t *testing.T) {
	recorder := &recordingRecorder{}
	ledger := newRunLedger(Config{
		Provider:            "openai",
		Model:               "gpt-test",
		RequestLimit:        10,
		TokenLimit:          100,
		BudgetWindowMinutes: 60,
	}, recorder)
	req := OpportunityRequest{
		TeamID:        uuid.New(),
		SiteID:        uuid.New(),
		ActorID:       uuid.New(),
		ActorType:     "user",
		DetectorInput: structuredDetectorInput(),
	}
	reservedRunID, err := ledger.reserve(context.Background(), req)
	if err != nil {
		t.Fatalf("reserve run: %v", err)
	}

	_, err = ledger.finalizeGeneration(context.Background(), reservedRunID, req, opportunityGeneration{
		Err:     ErrInvalidOutput,
		Latency: 25 * time.Millisecond,
		Usage:   Usage{InputTokens: 7, OutputTokens: 3, TotalTokens: 10},
		LifecycleEvents: []LifecycleEvent{{
			Type:          "request_finish",
			Status:        "failure",
			ErrorCategory: "invalid_output",
		}},
	})

	if !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("expected ErrInvalidOutput, got %v", err)
	}
	run := requireOneRecordedRun(t, recorder)
	if run.ID != reservedRunID {
		t.Fatalf("expected reserved run %s to be finalized, got %s", reservedRunID, run.ID)
	}
	if run.Status != "failure" || run.ErrorCategory != "invalid_output" {
		t.Fatalf("expected safe failure status, got status=%q category=%q", run.Status, run.ErrorCategory)
	}
	if run.OutputJSON != "{}" || run.OutputHash == "" {
		t.Fatalf("expected no customer output payload for failed run, got output=%q hash=%q", run.OutputJSON, run.OutputHash)
	}
	if run.Usage.TotalTokens != 10 || len(run.LifecycleEvents) != 1 {
		t.Fatalf("expected usage and lifecycle metadata to be retained, got usage=%+v events=%+v", run.Usage, run.LifecycleEvents)
	}
}

func TestValidateOpportunityCandidateProposalRejectsInventedEvidence(t *testing.T) {
	err := ValidateOpportunityCandidateProposal(OpportunityCandidateProposal{
		TypeKey:          "opportunities.types.checkout_conversion",
		Category:         "conversion",
		ActionType:       "optimize_checkout",
		Effort:           "medium",
		TitleKey:         "opportunities.catalog.checkout_conversion.title",
		SummaryKey:       "opportunities.catalog.checkout_conversion.summary",
		ActionKey:        "opportunities.catalog.checkout_conversion.action",
		DigestKey:        "opportunities.catalog.checkout_conversion.digest",
		CopyParams:       map[string]any{"conversion_rate": "42%"},
		CitedEvidenceIDs: []string{"unknown"},
	}, structuredDetectorInput())

	if !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("expected ErrInvalidOutput, got %v", err)
	}
}

func TestValidateOpportunityCatalogCandidateProposalRejectsUnsupportedTypeAndInventedEvidence(t *testing.T) {
	catalog := structuredOpportunityCandidateCatalog()
	proposal := validCatalogCandidateProposal()
	proposal.TypeKey = "opportunities.types.unsupported"

	_, err := ValidateOpportunityCatalogCandidateProposal(proposal, catalog)
	if !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("expected ErrInvalidOutput for unsupported catalog type, got %v", err)
	}

	proposal = validCatalogCandidateProposal()
	proposal.CitedEvidenceIDs = []string{"invented"}
	_, err = ValidateOpportunityCatalogCandidateProposal(proposal, catalog)
	if !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("expected ErrInvalidOutput for invented evidence, got %v", err)
	}
}

func TestValidateOpportunityCatalogCandidateProposalAcceptsAllowedCatalogParams(t *testing.T) {
	catalog := structuredOpportunityCandidateCatalog()
	proposal := validCatalogCandidateProposal()

	contract, err := ValidateOpportunityCatalogCandidateProposal(proposal, catalog)
	if err != nil {
		t.Fatalf("ValidateOpportunityCatalogCandidateProposal: %v", err)
	}
	if contract.TypeKey != proposal.TypeKey || contract.Category != proposal.Category {
		t.Fatalf("expected matching catalog contract, got %#v", contract)
	}
}

func TestGenerateOpportunityCatalogCandidateProposalUsesCatalogSchemaAndValidation(t *testing.T) {
	recorder := &recordingRecorder{}
	var capturedParams provider.GenerateParams
	service := &Service{
		conf: Config{
			Enabled:  true,
			Provider: "openai",
			Model:    "gpt-test",
			Timeout:  time.Second,
		},
		recorder: recorder,
		model: &fakeLanguageModel{
			id: "gpt-test",
			generateFn: func(_ context.Context, params provider.GenerateParams) (*provider.GenerateResult, error) {
				capturedParams = params
				return &provider.GenerateResult{
					Text:         validCatalogCandidateProviderJSON(),
					FinishReason: provider.FinishStop,
					Usage:        provider.Usage{InputTokens: 20, OutputTokens: 12},
				}, nil
			},
		},
	}

	result, err := service.GenerateOpportunityCatalogCandidateProposal(context.Background(), OpportunityCatalogRequest{
		TeamID:    uuid.New(),
		SiteID:    uuid.New(),
		ActorID:   uuid.New(),
		ActorType: "user",
		Catalog:   structuredOpportunityCandidateCatalog(),
	})
	if err != nil {
		t.Fatalf("GenerateOpportunityCatalogCandidateProposal: %v", err)
	}
	if result.Proposal.TypeKey != "opportunities.types.setup_goal_suggestion" {
		t.Fatalf("expected catalog-selected type, got %+v", result.Proposal)
	}
	if capturedParams.ResponseFormat == nil || !strings.Contains(string(capturedParams.ResponseFormat.Schema), "opportunities.types.setup_goal_suggestion") {
		t.Fatalf("expected catalog schema in response format, got %+v", capturedParams.ResponseFormat)
	}
	run := requireOneRecordedRun(t, recorder)
	if got := strings.Join(run.EvidenceIDs, ","); got != "suggested_goal_event,suggested_goal_event_count" {
		t.Fatalf("expected catalog evidence in audit run, got %#v", run.EvidenceIDs)
	}
}

func TestDecodeOpportunityCandidateProposalRejectsFreeformProseFields(t *testing.T) {
	_, err := decodeOpportunityCandidateProposalJSON([]byte(`{
		"title": "Fix checkout",
		"summary": "Checkout is weak",
		"next_action": "Rewrite checkout copy",
		"cited_evidence_ids": ["checkout-rate"]
	}`))

	if !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("expected ErrInvalidOutput, got %v", err)
	}
}

func TestDecodeOpportunityCandidateProposalRejectsTrailingProseAfterJSONObject(t *testing.T) {
	_, err := decodeOpportunityCandidateProposalJSON([]byte(validOpportunityProviderJSON() + "\n\nThis is unsupported trailing prose."))

	if !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("expected ErrInvalidOutput for trailing prose, got %v", err)
	}
}

func TestDecodeOpportunityCatalogCandidateProposalRejectsFreeformAndTrailingProse(t *testing.T) {
	_, err := decodeOpportunityCatalogCandidateProposalJSON([]byte(`{"title":"Fix goal setup","cited_evidence_ids":["suggested_goal_event"]}`))
	if !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("expected ErrInvalidOutput for freeform catalog field, got %v", err)
	}

	_, err = decodeOpportunityCatalogCandidateProposalJSON([]byte(validCatalogCandidateProviderJSON() + "\n\nCreate this goal next."))
	if !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("expected ErrInvalidOutput for trailing catalog prose, got %v", err)
	}
}

func TestOpportunityCatalogProposalSchemaUsesAllowedCatalogTypesAndParams(t *testing.T) {
	schema := string(opportunityCatalogProposalSchema(structuredOpportunityCandidateCatalog()))
	for _, want := range []string{
		"opportunities.types.setup_goal_suggestion",
		"opportunities.catalog.setup_goal_suggestion.title",
		"event_name",
		"event_count",
	} {
		if !strings.Contains(schema, want) {
			t.Fatalf("expected schema to include %q, got %s", want, schema)
		}
	}
	for _, forbidden := range []string{"raw_prompt", "provider_response", "freeform"} {
		if strings.Contains(schema, forbidden) {
			t.Fatalf("schema leaked forbidden field %q: %s", forbidden, schema)
		}
	}
}

func TestOpportunityCatalogProposalSchemaHonorsAllowedCategories(t *testing.T) {
	catalog := structuredOpportunityCandidateCatalog()
	catalog.AllowedCategories = []string{"setup_quality"}
	catalog.Candidates = append(catalog.Candidates, OpportunityDetectorInput{
		TypeKey:  "opportunities.types.checkout_conversion",
		Category: "conversion",
		MessageKeys: OpportunityMessageKeys{
			Title:   "opportunities.catalog.checkout_conversion.title",
			Summary: "opportunities.catalog.checkout_conversion.summary",
			Action:  "opportunities.catalog.checkout_conversion.action",
			Digest:  "opportunities.catalog.checkout_conversion.digest",
		},
		AllowedParams:      []string{"conversion_rate"},
		AllowedActionTypes: []string{"optimize_checkout"},
	})

	schema := string(opportunityCatalogProposalSchema(catalog))
	if strings.Contains(schema, "opportunities.types.checkout_conversion") || strings.Contains(schema, "conversion_rate") {
		t.Fatalf("schema advertised candidate outside allowed categories: %s", schema)
	}
	if !strings.Contains(schema, "opportunities.types.setup_goal_suggestion") {
		t.Fatalf("schema dropped allowed setup candidate: %s", schema)
	}
}

func TestOpportunityCatalogGenerationInputHonorsAllowedCategories(t *testing.T) {
	catalog := structuredOpportunityCandidateCatalog()
	catalog.AllowedCategories = []string{"setup_quality"}
	catalog.Candidates = append(catalog.Candidates, OpportunityDetectorInput{
		TypeKey:  "opportunities.types.checkout_conversion",
		Category: "conversion",
	})

	input := opportunityCatalogGenerationInput(OpportunityCatalogRequest{Catalog: catalog})
	if len(input.CandidateCatalog) != 1 || input.CandidateCatalog[0].TypeKey != "opportunities.types.setup_goal_suggestion" {
		t.Fatalf("expected prompt catalog to include only allowed candidates, got %#v", input.CandidateCatalog)
	}
}

func TestFinalizeOpportunityGenerationRejectsNilProviderResult(t *testing.T) {
	_, _, err := finalizeOpportunityGeneration(
		nil,
		Usage{},
		nil,
		strictOpportunityCandidateProposalResult,
		func(proposal OpportunityCandidateProposal) error {
			return ValidateOpportunityCandidateProposal(proposal, structuredDetectorInput())
		},
	)

	if !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("expected ErrInvalidOutput for nil provider result, got %v", err)
	}
}

func TestValidateOpportunityCandidateProposalRejectsUnsupportedKeysAndParams(t *testing.T) {
	err := ValidateOpportunityCandidateProposal(OpportunityCandidateProposal{
		TypeKey:          "opportunities.types.checkout_conversion",
		Category:         "conversion",
		ActionType:       "optimize_checkout",
		Effort:           "medium",
		TitleKey:         "opportunities.catalog.checkout_conversion.title",
		SummaryKey:       "opportunities.catalog.checkout_conversion.summary",
		ActionKey:        "opportunities.catalog.checkout_conversion.action",
		DigestKey:        "opportunities.catalog.checkout_conversion.digest",
		CopyParams:       map[string]any{"invented": "nope"},
		CitedEvidenceIDs: []string{"checkout-rate"},
	}, structuredDetectorInput())
	if !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("expected ErrInvalidOutput for unsupported param, got %v", err)
	}

	err = ValidateOpportunityCandidateProposal(OpportunityCandidateProposal{
		TypeKey:          "opportunities.types.checkout_conversion",
		Category:         "conversion",
		ActionType:       "optimize_checkout",
		Effort:           "medium",
		TitleKey:         "opportunities.catalog.other.title",
		SummaryKey:       "opportunities.catalog.checkout_conversion.summary",
		ActionKey:        "opportunities.catalog.checkout_conversion.action",
		DigestKey:        "opportunities.catalog.checkout_conversion.digest",
		CopyParams:       map[string]any{"conversion_rate": "42%"},
		CitedEvidenceIDs: []string{"checkout-rate"},
	}, structuredDetectorInput())
	if !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("expected ErrInvalidOutput for unsupported key, got %v", err)
	}
}

func TestValidateOpportunityCandidateProposalRejectsRemovedMoneyParamsEvenIfAllowed(t *testing.T) {
	input := structuredDetectorInput()
	input.AllowedParams = append(input.AllowedParams, "monthly_upside", "currency")
	input.CopyParams = map[string]any{
		"conversion_rate": "42%",
		"monthly_upside":  "8500",
		"currency":        "EUR",
	}

	err := ValidateOpportunityCandidateProposal(OpportunityCandidateProposal{
		TypeKey:          "opportunities.types.checkout_conversion",
		Category:         "conversion",
		ActionType:       "optimize_checkout",
		Effort:           "medium",
		TitleKey:         "opportunities.catalog.checkout_conversion.title",
		SummaryKey:       "opportunities.catalog.checkout_conversion.summary",
		ActionKey:        "opportunities.catalog.checkout_conversion.action",
		DigestKey:        "opportunities.catalog.checkout_conversion.digest",
		CopyParams:       map[string]any{"conversion_rate": "42%", "monthly_upside": "8500", "currency": "EUR"},
		CitedEvidenceIDs: []string{"checkout-rate"},
	}, input)
	if !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("expected ErrInvalidOutput for removed money params, got %v", err)
	}
}

func TestOpportunityPromptsForbidMoneyCausalAndSourceTotalClaims(t *testing.T) {
	prompt := opportunitySystemPrompt() + "\n" + opportunityPrompt("{}")
	for _, want := range []string{
		"Do not make money claims",
		"Do not make causal claims",
		"Do not infer source-specific traffic from total pageviews",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected prompt guardrail %q in %s", want, prompt)
		}
	}
}

func TestValidateOpportunityCandidateProposalRejectsChangedDetectorParams(t *testing.T) {
	err := ValidateOpportunityCandidateProposal(OpportunityCandidateProposal{
		TypeKey:          "opportunities.types.checkout_conversion",
		Category:         "conversion",
		ActionType:       "optimize_checkout",
		Effort:           "medium",
		TitleKey:         "opportunities.catalog.checkout_conversion.title",
		SummaryKey:       "opportunities.catalog.checkout_conversion.summary",
		ActionKey:        "opportunities.catalog.checkout_conversion.action",
		DigestKey:        "opportunities.catalog.checkout_conversion.digest",
		CopyParams:       map[string]any{"conversion_rate": "99%"},
		CitedEvidenceIDs: []string{"checkout-rate"},
	}, structuredDetectorInput())

	if !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("expected ErrInvalidOutput for changed detector param, got %v", err)
	}
}

func TestValidateOpportunityCandidateProposalRejectsActionTypeOutsideDetectorContract(t *testing.T) {
	input := structuredDetectorInput()
	input.TypeKey = "opportunities.types.conversion_signal"
	input.Category = "setup_quality"
	input.AllowedActionTypes = []string{"define_conversion_signal"}

	err := ValidateOpportunityCandidateProposal(OpportunityCandidateProposal{
		TypeKey:          "opportunities.types.conversion_signal",
		Category:         "setup_quality",
		ActionType:       "optimize_checkout",
		Effort:           "medium",
		TitleKey:         input.MessageKeys.Title,
		SummaryKey:       input.MessageKeys.Summary,
		ActionKey:        input.MessageKeys.Action,
		DigestKey:        input.MessageKeys.Digest,
		CopyParams:       map[string]any{"conversion_rate": "42%"},
		CitedEvidenceIDs: []string{"checkout-rate"},
	}, input)

	if !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("expected ErrInvalidOutput for unsupported detector action type, got %v", err)
	}
}

func TestClassifyErrorReturnsAccessDeniedForLocalPermissionFailures(t *testing.T) {
	if got := ClassifyError(ErrAccessDenied); got != "access_denied" {
		t.Fatalf("expected access_denied for ErrAccessDenied, got %q", got)
	}
	if got := ClassifyError(errors.New("access denied")); got != "access_denied" {
		t.Fatalf("expected access_denied for stable access denied error, got %q", got)
	}
}

func TestGenerateOpportunityProposalBlocksBeforeProviderWhenBudgetExhausted(t *testing.T) {
	recorder := &recordingRecorder{usage: BudgetUsage{Requests: 1, Tokens: 0}}
	service, err := NewService(Config{
		Enabled:             true,
		Provider:            "openai",
		Model:               "gpt-test",
		APIKey:              "test-key",
		RequestLimit:        1,
		TokenLimit:          100,
		BudgetWindowMinutes: 60,
	}, recorder)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = service.GenerateOpportunityProposal(context.Background(), OpportunityRequest{
		TeamID:    uuid.New(),
		SiteID:    uuid.New(),
		ActorID:   uuid.New(),
		ActorType: "user",
		DetectorInput: OpportunityDetectorInput{
			SiteDomain:  "example.test",
			TypeKey:     "opportunities.types.checkout_conversion",
			MessageKeys: structuredDetectorInput().MessageKeys,
			AllowedParams: []string{
				"conversion_rate",
			},
			Evidence:    []Evidence{{ID: "checkout-rate", Label: "opportunities.evidence.checkout_conversion_rate", Value: "42%"}},
			ImpactValue: "$900",
			Confidence:  "medium",
			Kind:        "conversion",
		},
	})
	if !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("expected ErrBudgetExhausted, got %v", err)
	}
	if len(recorder.runs) != 1 {
		t.Fatalf("expected one safe audit run, got %d", len(recorder.runs))
	}
	run := recorder.runs[0]
	if run.Status != "failure" || run.ErrorCategory != "budget_exhausted" {
		t.Fatalf("expected budget exhausted audit, got status=%q category=%q", run.Status, run.ErrorCategory)
	}
	if run.OutputJSON != "{}" {
		t.Fatalf("expected empty output json for blocked call, got %q", run.OutputJSON)
	}
}

func TestGenerateOpportunityProposalReservesBudgetBeforeProviderCall(t *testing.T) {
	recorder := &recordingRecorder{}
	var calls int32
	service := &Service{
		conf: Config{
			Enabled:             true,
			Provider:            "openai",
			Model:               "gpt-test",
			Timeout:             time.Second,
			RequestLimit:        1,
			TokenLimit:          100,
			BudgetWindowMinutes: 60,
		},
		recorder: recorder,
		model: &fakeLanguageModel{
			id: "gpt-test",
			generateFn: func(context.Context, provider.GenerateParams) (*provider.GenerateResult, error) {
				atomic.AddInt32(&calls, 1)
				return &provider.GenerateResult{
					Text:         validOpportunityProviderJSON(),
					FinishReason: provider.FinishStop,
					Usage:        provider.Usage{InputTokens: 10, OutputTokens: 5},
				}, nil
			},
		},
	}

	first, err := service.GenerateOpportunityProposal(context.Background(), OpportunityRequest{
		TeamID:        uuid.New(),
		SiteID:        uuid.New(),
		ActorID:       uuid.New(),
		ActorType:     "user",
		DetectorInput: structuredDetectorInput(),
	})
	if err != nil {
		t.Fatalf("first GenerateOpportunityProposal: %v", err)
	}
	if first.RunID == uuid.Nil {
		t.Fatal("expected first run id")
	}
	_, err = service.GenerateOpportunityProposal(context.Background(), OpportunityRequest{
		TeamID:        uuid.New(),
		SiteID:        uuid.New(),
		ActorID:       uuid.New(),
		ActorType:     "user",
		DetectorInput: structuredDetectorInput(),
	})
	if !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("expected second call to be budget exhausted, got %v", err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected only one provider call after reservation, got %d", calls)
	}
	if len(recorder.runs) != 2 || recorder.runs[0].Status != "success" || recorder.runs[0].ID != first.RunID || recorder.runs[1].ErrorCategory != "budget_exhausted" {
		t.Fatalf("expected finalized reservation plus safe budget-exhausted run, got %+v", recorder.runs)
	}
}

func TestGenerateOpportunityProposalReturnsStructuredCandidateMetadata(t *testing.T) {
	recorder := &recordingRecorder{}
	service := &Service{
		conf: Config{
			Enabled:  true,
			Provider: "openai",
			Model:    "gpt-test",
			Timeout:  time.Second,
		},
		recorder: recorder,
		model: &fakeLanguageModel{
			id: "gpt-test",
			generateFn: func(context.Context, provider.GenerateParams) (*provider.GenerateResult, error) {
				return &provider.GenerateResult{
					Text:         validOpportunityProviderJSON(),
					FinishReason: provider.FinishStop,
					Usage:        provider.Usage{InputTokens: 10, OutputTokens: 5},
				}, nil
			},
		},
	}

	result, err := service.GenerateOpportunityProposal(context.Background(), OpportunityRequest{
		TeamID:        uuid.New(),
		SiteID:        uuid.New(),
		ActorID:       uuid.New(),
		ActorType:     "user",
		DetectorInput: structuredDetectorInput(),
	})
	if err != nil {
		t.Fatalf("GenerateOpportunityProposal: %v", err)
	}
	if result.Proposal.TypeKey != "opportunities.types.checkout_conversion" ||
		result.Proposal.Category != "conversion" ||
		result.Proposal.ActionType != "optimize_checkout" ||
		result.Proposal.Effort != "medium" {
		t.Fatalf("expected structured proposal metadata, got %+v", result.Proposal)
	}
}

func TestRunLedgerRecordsEvidenceSnapshotAsAuditableInput(t *testing.T) {
	recorder := &recordingRecorder{}
	legacyInput := structuredDetectorInput()
	snapshot := OpportunityEvidenceSnapshot{
		SiteDomain: "shop.example",
		From:       time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		To:         time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC),
		Evidence: []Evidence{
			{ID: "snapshot-checkout-rate", Label: "opportunities.evidence.checkout_conversion_rate", Value: "42%"},
			{ID: "snapshot-orders", Label: "opportunities.evidence.orders", Value: "28"},
		},
	}
	service := &Service{
		conf: Config{
			Enabled:  true,
			Provider: "openai",
			Model:    "gpt-test",
			Timeout:  time.Second,
		},
		recorder: recorder,
		model: &fakeLanguageModel{
			id: "gpt-test",
			generateFn: func(context.Context, provider.GenerateParams) (*provider.GenerateResult, error) {
				return &provider.GenerateResult{
					Text:         validOpportunityProviderJSONWithEvidence("snapshot-checkout-rate"),
					FinishReason: provider.FinishStop,
					Usage:        provider.Usage{InputTokens: 10, OutputTokens: 5},
				}, nil
			},
		},
	}

	_, err := service.GenerateOpportunityProposal(context.Background(), OpportunityRequest{
		TeamID:           uuid.New(),
		SiteID:           uuid.New(),
		ActorID:          uuid.New(),
		ActorType:        "user",
		DetectorInput:    legacyInput,
		EvidenceSnapshot: snapshot,
	})
	if err != nil {
		t.Fatalf("GenerateOpportunityProposal: %v", err)
	}

	run := requireOneRecordedRun(t, recorder)
	if got := strings.Join(run.EvidenceIDs, ","); got != "snapshot-checkout-rate,snapshot-orders" {
		t.Fatalf("expected snapshot evidence IDs, got %#v", run.EvidenceIDs)
	}
	if run.InputHash != HashAny(snapshot) {
		t.Fatalf("expected input hash from evidence snapshot")
	}
	if run.InputHash == HashAny(legacyInput) {
		t.Fatalf("expected snapshot hash to replace detector input hash")
	}
}

func TestRunLedgerFallsBackToDetectorEvidenceWhenSnapshotHasNoEvidence(t *testing.T) {
	req := OpportunityRequest{
		DetectorInput: structuredDetectorInput(),
		EvidenceSnapshot: OpportunityEvidenceSnapshot{
			SiteDomain: "empty-snapshot.example",
		},
	}

	input := opportunityAuditInput(req)

	if got := strings.Join(evidenceIDs(input.Evidence), ","); got != "checkout-rate" {
		t.Fatalf("expected detector evidence fallback, got %#v", input.Evidence)
	}
}

func TestOpportunityGenerationInputKeepsEvidenceOnlyInSnapshot(t *testing.T) {
	req := OpportunityRequest{
		DetectorInput: structuredDetectorInput(),
		EvidenceSnapshot: OpportunityEvidenceSnapshot{
			SiteDomain: "snapshot.example",
			Evidence:   []Evidence{{ID: "snapshot-evidence", Label: "opportunities.evidence.orders", Value: "28"}},
		},
	}

	input := opportunityGenerationInput(req)

	if len(input.CandidateContract.Evidence) != 0 {
		t.Fatalf("expected candidate contract evidence to be omitted from prompt, got %#v", input.CandidateContract.Evidence)
	}
	if got := strings.Join(evidenceIDs(input.EvidenceSnapshot.Evidence), ","); got != "snapshot-evidence" {
		t.Fatalf("expected snapshot evidence in prompt, got %#v", input.EvidenceSnapshot.Evidence)
	}
}

func TestNewServiceKeepsEnabledUnconfiguredState(t *testing.T) {
	recorder := &recordingRecorder{}
	service, err := NewService(Config{
		Enabled:             true,
		Provider:            "unsupported-test-provider",
		Model:               "gpt-test",
		RequestLimit:        10,
		TokenLimit:          100,
		BudgetWindowMinutes: 60,
	}, recorder)
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
	if service == nil || !service.Enabled() || service.Configured() {
		t.Fatalf("expected enabled unconfigured service, got %#v", service)
	}

	_, err = service.GenerateOpportunityProposal(context.Background(), OpportunityRequest{
		TeamID:        uuid.New(),
		SiteID:        uuid.New(),
		ActorID:       uuid.New(),
		ActorType:     "user",
		DetectorInput: structuredDetectorInput(),
	})
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured on generation, got %v", err)
	}
	run := requireOneRecordedRun(t, recorder)
	if run.Status != "failure" || run.ErrorCategory != "not_configured" {
		t.Fatalf("expected not_configured audit run, got status=%q category=%q", run.Status, run.ErrorCategory)
	}
}

func TestValidateConfigRequiresAPIKeyForHostedKeyProviders(t *testing.T) {
	err := ValidateConfig(Config{Provider: "openai", Model: "gpt-test"})
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured for missing OpenAI API key, got %v", err)
	}
	err = ValidateConfig(Config{Provider: "openai-compatible", Model: "gpt-test", BaseURL: "https://gateway.example/v1"})
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured for missing gateway API key, got %v", err)
	}
	if err := ValidateConfig(Config{Provider: "ollama", Model: "llama3"}); err != nil {
		t.Fatalf("expected local keyless provider config to be valid, got %v", err)
	}
}

func TestValidateConfigRequiresExplicitBaseURLForGatewayProviders(t *testing.T) {
	for _, provider := range []string{"openai-compatible", "compat", "gateway", "bifrost", "litellm"} {
		t.Run(provider, func(t *testing.T) {
			err := ValidateConfig(Config{Provider: provider, Model: "gpt-test", APIKey: "provider-secret"})
			if !errors.Is(err, ErrNotConfigured) {
				t.Fatalf("expected ErrNotConfigured for missing gateway base URL, got %v", err)
			}
		})
	}

	if err := ValidateConfig(Config{
		Provider: "openai-compatible",
		Model:    "gpt-test",
		BaseURL:  "https://gateway.example/v1",
		APIKey:   "provider-secret",
	}); err != nil {
		t.Fatalf("expected explicit gateway config to be valid, got %v", err)
	}
}

func TestGenerateOpportunityProposalRejectsProviderOutputWithFreeformProseFields(t *testing.T) {
	recorder := &recordingRecorder{}
	service := &Service{
		conf: Config{
			Enabled:  true,
			Provider: "openai",
			Model:    "gpt-test",
			Timeout:  time.Second,
		},
		recorder: recorder,
		model: &fakeLanguageModel{
			id: "gpt-test",
			generateFn: func(context.Context, provider.GenerateParams) (*provider.GenerateResult, error) {
				return &provider.GenerateResult{
					Text: `{
						"title": "Rewrite checkout",
						"title_key": "opportunities.catalog.checkout_conversion.title",
						"summary_key": "opportunities.catalog.checkout_conversion.summary",
						"action_key": "opportunities.catalog.checkout_conversion.action",
						"digest_key": "opportunities.catalog.checkout_conversion.digest",
						"copy_params": {"conversion_rate": "42%"},
						"cited_evidence_ids": ["checkout-rate"]
					}`,
					FinishReason: provider.FinishStop,
					Usage:        provider.Usage{InputTokens: 19, OutputTokens: 11},
				}, nil
			},
		},
	}

	_, err := service.GenerateOpportunityProposal(context.Background(), OpportunityRequest{
		TeamID:        uuid.New(),
		SiteID:        uuid.New(),
		ActorID:       uuid.New(),
		ActorType:     "user",
		DetectorInput: structuredDetectorInput(),
	})
	if !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("expected ErrInvalidOutput, got %v", err)
	}
	if len(recorder.runs) != 1 {
		t.Fatalf("expected one audit run, got %d", len(recorder.runs))
	}
	run := recorder.runs[0]
	if run.Status != "failure" || run.ErrorCategory != "invalid_output" {
		t.Fatalf("expected invalid output audit, got status=%q category=%q", run.Status, run.ErrorCategory)
	}
	if stringsContainsAny(run.OutputJSON, "Rewrite checkout", `"title":`, "raw") {
		t.Fatalf("provider prose/raw output leaked into audit output json: %s", run.OutputJSON)
	}
}

func TestGenerateOpportunityProposalRecordsSafeLifecycleMetadata(t *testing.T) {
	recorder := &recordingRecorder{}
	service := &Service{
		conf: Config{
			Enabled:  true,
			Provider: "openai",
			Model:    "gpt-test",
			Timeout:  time.Second,
		},
		recorder: recorder,
		model:    successfulToolLoopModel(t),
	}

	result, err := service.GenerateOpportunityProposal(context.Background(), OpportunityRequest{
		TeamID:        uuid.New(),
		SiteID:        uuid.New(),
		ActorID:       uuid.New(),
		ActorType:     "user",
		DetectorInput: structuredDetectorInput(),
		Tools: []goaisdk.Tool{{
			Name:        "site_overview",
			Description: "aggregate overview",
			InputSchema: json.RawMessage(`{"type":"object"}`),
			Execute: func(context.Context, json.RawMessage) (string, error) {
				return `{"aggregate":"safe"}`, nil
			},
		}},
	})
	if err != nil {
		t.Fatalf("GenerateOpportunityProposal: %v", err)
	}
	if result.Usage.TotalTokens != 31 || result.Usage.ToolCallCount != 1 {
		t.Fatalf("unexpected usage metadata: %+v", result.Usage)
	}
	assertSuccessfulLifecycleRun(t, recorder)
}

func successfulToolLoopModel(t *testing.T) *scriptedLanguageModel {
	t.Helper()
	return &scriptedLanguageModel{
		t:  t,
		id: "gpt-test",
		results: []*provider.GenerateResult{
			{
				FinishReason: provider.FinishToolCalls,
				ToolCalls: []provider.ToolCall{{
					ID:    "call_1",
					Name:  "site_overview",
					Input: json.RawMessage(`{"include_raw":true}`),
				}},
				Usage: provider.Usage{InputTokens: 7, OutputTokens: 3},
			},
			{
				Text:         validOpportunityProviderJSON(),
				FinishReason: provider.FinishStop,
				Usage:        provider.Usage{InputTokens: 13, OutputTokens: 8},
			},
		},
	}
}

func validOpportunityProviderJSON() string {
	return validOpportunityProviderJSONWithEvidence("checkout-rate")
}

func validOpportunityProviderJSONWithEvidence(evidenceID string) string {
	return `{
		"type_key": "opportunities.types.checkout_conversion",
		"category": "conversion",
		"action_type": "optimize_checkout",
		"effort": "medium",
		"title_key": "opportunities.catalog.checkout_conversion.title",
		"summary_key": "opportunities.catalog.checkout_conversion.summary",
		"action_key": "opportunities.catalog.checkout_conversion.action",
		"digest_key": "opportunities.catalog.checkout_conversion.digest",
		"copy_params": {"conversion_rate": "42%"},
		"cited_evidence_ids": ["` + evidenceID + `"]
	}`
}

func assertSuccessfulLifecycleRun(t *testing.T, recorder *recordingRecorder) {
	t.Helper()
	run := requireOneRecordedRun(t, recorder)
	assertProviderMetadata(t, run)
	assertLifecycleStatus(t, run)
	assertRunUsage(t, run)
	assertRunHashes(t, run)
	assertLifecycleEvents(t, run)
	assertNoAuditPayloadLeak(t, run.OutputJSON)
}

func requireOneRecordedRun(t *testing.T, recorder *recordingRecorder) RunRecord {
	t.Helper()
	if len(recorder.runs) != 1 {
		t.Fatalf("expected one audit run, got %d", len(recorder.runs))
	}
	return recorder.runs[0]
}

func assertProviderMetadata(t *testing.T, run RunRecord) {
	t.Helper()
	if run.Provider != "openai" || run.Model != "gpt-test" || run.TemplateVersion != OpportunityTemplateVersion {
		t.Fatalf("unexpected provider metadata: %+v", run)
	}
}

func assertLifecycleStatus(t *testing.T, run RunRecord) {
	t.Helper()
	if run.Status != "success" || run.ErrorCategory != "" || run.Latency <= 0 {
		t.Fatalf("unexpected lifecycle status: status=%q category=%q latency=%s", run.Status, run.ErrorCategory, run.Latency)
	}
}

func assertRunUsage(t *testing.T, run RunRecord) {
	t.Helper()
	if run.Usage.TotalTokens != 31 || run.Usage.ToolCallCount != 1 {
		t.Fatalf("unexpected run usage: %+v", run.Usage)
	}
}

func assertRunHashes(t *testing.T, run RunRecord) {
	t.Helper()
	if run.InputHash == "" || run.OutputHash == "" {
		t.Fatalf("expected input/output hashes, got input=%q output=%q", run.InputHash, run.OutputHash)
	}
}

func assertLifecycleEvents(t *testing.T, run RunRecord) {
	t.Helper()
	events := map[string]bool{}
	for _, event := range run.LifecycleEvents {
		events[event.Type] = true
		if stringsContainsAny(event.ToolName, "include_raw", "aggregate") || stringsContainsAny(event.ErrorCategory, "include_raw", "aggregate") {
			t.Fatalf("lifecycle event leaked raw payload: %+v", event)
		}
	}
	for _, eventType := range []string{"request_start", "request_finish", "tool_call_start", "tool_call_finish"} {
		if !events[eventType] {
			t.Fatalf("expected lifecycle event %q in %+v", eventType, run.LifecycleEvents)
		}
	}
}

func assertNoAuditPayloadLeak(t *testing.T, outputJSON string) {
	t.Helper()
	if stringsContainsAny(outputJSON, "include_raw", "site_overview", "aggregate", "system", "prompt") {
		t.Fatalf("audit output leaked prompt/tool/provider payload: %s", outputJSON)
	}
}

type fakeLanguageModel struct {
	id         string
	generateFn func(context.Context, provider.GenerateParams) (*provider.GenerateResult, error)
}

func (m *fakeLanguageModel) ModelID() string { return m.id }

func (m *fakeLanguageModel) DoGenerate(ctx context.Context, params provider.GenerateParams) (*provider.GenerateResult, error) {
	return m.generateFn(ctx, params)
}

func (m *fakeLanguageModel) DoStream(context.Context, provider.GenerateParams) (*provider.StreamResult, error) {
	return nil, errors.New("stream not implemented")
}

type scriptedLanguageModel struct {
	t       *testing.T
	id      string
	calls   int
	results []*provider.GenerateResult
}

func (m *scriptedLanguageModel) ModelID() string { return m.id }

func (m *scriptedLanguageModel) DoGenerate(context.Context, provider.GenerateParams) (*provider.GenerateResult, error) {
	m.t.Helper()
	if m.calls >= len(m.results) {
		m.t.Fatalf("unexpected provider call %d", m.calls+1)
	}
	result := m.results[m.calls]
	m.calls++
	return result, nil
}

func (m *scriptedLanguageModel) DoStream(context.Context, provider.GenerateParams) (*provider.StreamResult, error) {
	return nil, errors.New("stream not implemented")
}

func stringsContainsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func structuredDetectorInput() OpportunityDetectorInput {
	return OpportunityDetectorInput{
		TypeKey:  "opportunities.types.checkout_conversion",
		Category: "conversion",
		MessageKeys: OpportunityMessageKeys{
			Title:   "opportunities.catalog.checkout_conversion.title",
			Summary: "opportunities.catalog.checkout_conversion.summary",
			Action:  "opportunities.catalog.checkout_conversion.action",
			Digest:  "opportunities.catalog.checkout_conversion.digest",
		},
		AllowedParams: []string{"conversion_rate"},
		CopyParams:    map[string]any{"conversion_rate": "42%"},
		Evidence:      []Evidence{{ID: "checkout-rate", Label: "opportunities.evidence.checkout_conversion_rate", Value: "42%"}},
	}
}

func structuredOpportunityCandidateCatalog() OpportunityCandidateCatalog {
	return OpportunityCandidateCatalog{
		Candidates: []OpportunityDetectorInput{
			{
				TypeKey:  "opportunities.types.setup_goal_suggestion",
				Category: "setup_quality",
				MessageKeys: OpportunityMessageKeys{
					Title:   "opportunities.catalog.setup_goal_suggestion.title",
					Summary: "opportunities.catalog.setup_goal_suggestion.summary",
					Action:  "opportunities.catalog.setup_goal_suggestion.action",
					Digest:  "opportunities.catalog.setup_goal_suggestion.digest",
				},
				AllowedParams:      []string{"event_name", "event_count", "goal_type", "goal_value"},
				AllowedActionTypes: []string{"create_goal"},
			},
		},
		EvidenceSnapshot: OpportunityEvidenceSnapshot{
			Evidence: []Evidence{
				{ID: "suggested_goal_event", Label: "opportunities.evidence.suggested_goal_event", Value: "demo_request"},
				{ID: "suggested_goal_event_count", Label: "opportunities.evidence.suggested_goal_event_count", Value: "18"},
			},
		},
	}
}

func validCatalogCandidateProposal() OpportunityCandidateProposal {
	return OpportunityCandidateProposal{
		TypeKey:    "opportunities.types.setup_goal_suggestion",
		Category:   "setup_quality",
		ActionType: "create_goal",
		Effort:     "low",
		TitleKey:   "opportunities.catalog.setup_goal_suggestion.title",
		SummaryKey: "opportunities.catalog.setup_goal_suggestion.summary",
		ActionKey:  "opportunities.catalog.setup_goal_suggestion.action",
		DigestKey:  "opportunities.catalog.setup_goal_suggestion.digest",
		CopyParams: map[string]any{
			"event_name":  "demo_request",
			"event_count": 18,
			"goal_type":   "event",
			"goal_value":  "demo_request",
		},
		CitedEvidenceIDs: []string{"suggested_goal_event", "suggested_goal_event_count"},
	}
}

func validCatalogCandidateProviderJSON() string {
	return `{
		"type_key": "opportunities.types.setup_goal_suggestion",
		"category": "setup_quality",
		"action_type": "create_goal",
		"effort": "low",
		"title_key": "opportunities.catalog.setup_goal_suggestion.title",
		"summary_key": "opportunities.catalog.setup_goal_suggestion.summary",
		"action_key": "opportunities.catalog.setup_goal_suggestion.action",
		"digest_key": "opportunities.catalog.setup_goal_suggestion.digest",
		"copy_params": {"event_name": "demo_request", "event_count": 18, "goal_type": "event", "goal_value": "demo_request"},
		"cited_evidence_ids": ["suggested_goal_event", "suggested_goal_event_count"]
	}`
}
