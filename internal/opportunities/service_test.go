package opportunities

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	hitai "hitkeep/internal/ai"
	"hitkeep/internal/api"
	"hitkeep/internal/database"
)

func TestGeneratePersistsValidatedAIProposal(t *testing.T) {
	shared, site, teamID, actorID := setupOpportunityServiceTestStore(t)
	runID := uuid.New()
	catalog := NewDetectorCatalog(fakeDetector{
		contract: DetectorContract{
			Category: DetectorCategoryConversion,
			TypeKey:  "opportunities.types.fixture",
			MessageKeys: DetectorMessageKeys{
				Title:       "opportunities.fixture.title",
				Summary:     "opportunities.fixture.summary",
				Action:      "opportunities.fixture.action",
				Digest:      "opportunities.fixture.digest",
				ImpactLabel: "opportunities.fixture.impact",
				RouteLabel:  "opportunities.fixture.route",
			},
			AllowedParams: []string{"allowed", "kept"},
		},
		output: database.OpportunityInput{
			ID:         uuid.New(),
			TeamID:     teamID,
			SiteID:     site.ID,
			Kind:       "conversion",
			TypeKey:    "opportunities.types.fixture",
			TitleKey:   "opportunities.fixture.title",
			SummaryKey: "opportunities.fixture.summary",
			ActionKey:  "opportunities.fixture.action",
			DigestKey:  "opportunities.fixture.digest",
			CopyParams: map[string]any{
				"allowed": "detector value",
				"kept":    "detector fallback",
			},
			ImpactValue:     "EUR 900",
			ImpactLabelKey:  "opportunities.fixture.impact",
			MonthlyUpside:   900,
			Confidence:      "medium",
			Score:           80,
			Status:          "new",
			RouteLabelKey:   "opportunities.fixture.route",
			RouteParams:     map[string]any{"allowed": "route"},
			RouteIcon:       "pi pi-compass",
			DetectorVersion: detectorVersion,
			Evidence: []api.OpportunityEvidence{
				{ID: "primary", LabelKey: "opportunities.fixture.primary", Value: "42%"},
				{ID: "secondary", LabelKey: "opportunities.fixture.secondary", Value: "17"},
			},
			CitedEvidenceIDs: []string{"primary"},
			GeneratedAt:      time.Now().UTC(),
		},
	})
	service := Service{
		Shared: shared,
		AI: fakeOpportunityAI{
			runID: runID,
			proposal: hitai.OpportunityCandidateProposal{
				TypeKey:          "opportunities.types.fixture",
				Category:         "conversion",
				ActionType:       "optimize_checkout",
				Effort:           "medium",
				TitleKey:         "opportunities.fixture.title",
				SummaryKey:       "opportunities.fixture.summary",
				ActionKey:        "opportunities.fixture.action",
				DigestKey:        "opportunities.fixture.digest",
				CopyParams:       map[string]any{"allowed": "detector value", "kept": "detector fallback"},
				CitedEvidenceIDs: []string{"secondary"},
			},
		},
		Catalog: catalog,
	}

	opportunities, gotRunID, status, err := service.Generate(context.Background(), GenerateInput{
		TeamID:    teamID,
		Site:      site,
		Store:     shared,
		From:      time.Now().UTC().AddDate(0, 0, -30),
		To:        time.Now().UTC(),
		ActorID:   actorID,
		ActorType: "user",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if status != "success" {
		t.Fatalf("expected AI success, got %q", status)
	}
	if gotRunID == nil || *gotRunID != runID {
		t.Fatalf("expected run ID %s, got %v", runID, gotRunID)
	}
	if len(opportunities) != 1 {
		t.Fatalf("expected one opportunity, got %d", len(opportunities))
	}
	opportunity := opportunities[0]
	if opportunity.AIRunID == nil || *opportunity.AIRunID != runID {
		t.Fatalf("expected opportunity AI run ID %s, got %v", runID, opportunity.AIRunID)
	}
	if opportunity.CopyParams["allowed"] != "detector value" {
		t.Fatalf("expected detector proposal param to be retained, got %#v", opportunity.CopyParams)
	}
	if opportunity.CopyParams["kept"] != "detector fallback" {
		t.Fatalf("expected detector fallback param to be retained, got %#v", opportunity.CopyParams)
	}
	if len(opportunity.CitedEvidenceIDs) != 1 || opportunity.CitedEvidenceIDs[0] != "secondary" {
		t.Fatalf("expected AI citations to be persisted, got %#v", opportunity.CitedEvidenceIDs)
	}
}

func TestGeneratePassesEvidenceSnapshotToAI(t *testing.T) {
	shared, site, teamID, actorID := setupOpportunityServiceTestStore(t)
	ai := &recordingOpportunityAI{runID: uuid.New(), proposal: fixtureAIProposal("one")}
	service := Service{
		Shared:  shared,
		AI:      ai,
		Catalog: NewDetectorCatalog(fakeDetector{contract: fixtureDetectorContract("one"), output: fixtureOpportunity(teamID, site.ID, "one")}),
	}
	from := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC)

	_, _, status, err := service.Generate(context.Background(), GenerateInput{
		TeamID:    teamID,
		Site:      site,
		Store:     shared,
		From:      from,
		To:        to,
		ActorID:   actorID,
		ActorType: "user",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if status != "success" {
		t.Fatalf("expected success, got %q", status)
	}
	if ai.last.EvidenceSnapshot.SiteDomain != site.Domain {
		t.Fatalf("expected snapshot site domain %q, got %q", site.Domain, ai.last.EvidenceSnapshot.SiteDomain)
	}
	if !ai.last.EvidenceSnapshot.From.Equal(from) || !ai.last.EvidenceSnapshot.To.Equal(to) {
		t.Fatalf("expected snapshot window %s-%s, got %s-%s", from, to, ai.last.EvidenceSnapshot.From, ai.last.EvidenceSnapshot.To)
	}
	if got := evidenceIDList(ai.last.EvidenceSnapshot.Evidence); strings.Join(got, ",") != "primary,secondary" {
		t.Fatalf("expected candidate evidence in snapshot, got %#v", got)
	}
}

func TestGenerateUsesActiveCatalogContractForDefaultTypeKey(t *testing.T) {
	shared, site, teamID, actorID := setupOpportunityServiceTestStore(t)
	contract := DetectorContract{
		Category: DetectorCategoryRevenue,
		TypeKey:  "opportunities.types.checkout_conversion",
		MessageKeys: DetectorMessageKeys{
			Title:       "opportunities.fixture.custom_checkout.title",
			Summary:     "opportunities.fixture.custom_checkout.summary",
			Action:      "opportunities.fixture.custom_checkout.action",
			Digest:      "opportunities.fixture.custom_checkout.digest",
			ImpactLabel: "opportunities.fixture.custom_checkout.impact",
			RouteLabel:  "opportunities.fixture.custom_checkout.route",
		},
		AllowedParams: []string{"custom_signal"},
	}
	output := database.OpportunityInput{
		ID:              uuid.New(),
		TeamID:          teamID,
		SiteID:          site.ID,
		Kind:            "revenue",
		TypeKey:         contract.TypeKey,
		TitleKey:        contract.MessageKeys.Title,
		SummaryKey:      contract.MessageKeys.Summary,
		ActionKey:       contract.MessageKeys.Action,
		DigestKey:       contract.MessageKeys.Digest,
		CopyParams:      map[string]any{"custom_signal": "42"},
		ImpactValue:     "EUR 900",
		ImpactLabelKey:  contract.MessageKeys.ImpactLabel,
		Confidence:      "medium",
		Status:          "new",
		RouteLabelKey:   contract.MessageKeys.RouteLabel,
		RouteParams:     map[string]any{"custom_signal": "route"},
		RouteIcon:       "pi pi-bolt",
		DetectorVersion: detectorVersion,
		Evidence:        []api.OpportunityEvidence{{ID: "custom-evidence", LabelKey: "opportunities.fixture.custom_checkout.evidence", Value: "42"}},
		CitedEvidenceIDs: []string{
			"custom-evidence",
		},
		GeneratedAt: time.Now().UTC(),
	}
	ai := &echoOpportunityAI{runID: uuid.New(), citedEvidenceIDs: []string{"custom-evidence"}}
	service := Service{
		Shared:  shared,
		AI:      ai,
		Catalog: NewDetectorCatalog(fakeDetector{contract: contract, output: output}),
	}

	_, _, status, err := service.Generate(context.Background(), GenerateInput{
		TeamID:    teamID,
		Site:      site,
		Store:     shared,
		ActorID:   actorID,
		ActorType: "user",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if status != "success" {
		t.Fatalf("expected active catalog contract to drive AI validation, got status %q", status)
	}
	if ai.last.DetectorInput.Category != string(DetectorCategoryRevenue) {
		t.Fatalf("expected active catalog category, got %#v", ai.last.DetectorInput)
	}
	if strings.Join(ai.last.DetectorInput.AllowedParams, ",") != "custom_signal" {
		t.Fatalf("expected active catalog allowed params, got %#v", ai.last.DetectorInput.AllowedParams)
	}
}

func TestGenerateReportsNoOpportunitiesWhenAIIsConfiguredAndNoCandidatesExist(t *testing.T) {
	shared, site, teamID, actorID := setupOpportunityServiceTestStore(t)
	service := Service{
		Shared:  shared,
		AI:      fakeOpportunityAI{runID: uuid.New()},
		Catalog: NewDetectorCatalog(noOpportunityDetector{contract: fixtureDetectorContract("quiet")}),
	}

	opportunities, runID, status, err := service.Generate(context.Background(), GenerateInput{
		TeamID:    teamID,
		Site:      site,
		Store:     shared,
		ActorID:   actorID,
		ActorType: "user",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(opportunities) != 0 || runID != nil {
		t.Fatalf("expected no generated opportunities or AI run, got opportunities=%d run=%v", len(opportunities), runID)
	}
	if status != "no_opportunities" {
		t.Fatalf("expected no_opportunities status, got %q", status)
	}
}

func TestGenerateRejectsUnsupportedAIProposalAndPersistsEvidenceBackedFallback(t *testing.T) {
	shared, site, teamID, actorID := setupOpportunityServiceTestStore(t)
	runID := uuid.New()
	catalog := NewDetectorCatalog(fakeDetector{
		contract: fixtureDetectorContract("one"),
		output:   fixtureOpportunity(teamID, site.ID, "one"),
	})
	service := Service{
		Shared: shared,
		AI: fakeOpportunityAI{
			runID: runID,
			proposal: hitai.OpportunityCandidateProposal{
				TypeKey:          "opportunities.fixture.one.type",
				Category:         "conversion",
				ActionType:       "optimize_checkout",
				Effort:           "medium",
				TitleKey:         "opportunities.fixture.one.title",
				SummaryKey:       "opportunities.fixture.one.summary",
				ActionKey:        "opportunities.fixture.one.action",
				DigestKey:        "opportunities.fixture.one.digest",
				CopyParams:       map[string]any{"allowed": "detector one"},
				CitedEvidenceIDs: []string{"invented-evidence"},
			},
		},
		Catalog: catalog,
	}

	opportunities, gotRunID, status, err := service.Generate(context.Background(), GenerateInput{
		TeamID:    teamID,
		Site:      site,
		Store:     shared,
		From:      time.Now().UTC().AddDate(0, 0, -30),
		To:        time.Now().UTC(),
		ActorID:   actorID,
		ActorType: "user",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if status != "invalid_output" {
		t.Fatalf("expected invalid_output status, got %q", status)
	}
	if gotRunID != nil {
		t.Fatalf("expected no accepted AI run ID, got %v", gotRunID)
	}
	if len(opportunities) != 1 {
		t.Fatalf("expected deterministic fallback opportunity, got %d", len(opportunities))
	}
	opportunity := opportunities[0]
	if opportunity.AIRunID != nil {
		t.Fatalf("expected invalid AI run to stay off customer-visible opportunity, got %v", opportunity.AIRunID)
	}
	if len(opportunity.CitedEvidenceIDs) != 1 || opportunity.CitedEvidenceIDs[0] != "primary" {
		t.Fatalf("expected detector citations to remain, got %#v", opportunity.CitedEvidenceIDs)
	}
}

func TestGenerateRejectsAIProposalThatChangesEvidenceBoundParams(t *testing.T) {
	shared, site, teamID, actorID := setupOpportunityServiceTestStore(t)
	catalog := NewDetectorCatalog(fakeDetector{
		contract: fixtureDetectorContract("one"),
		output:   fixtureOpportunity(teamID, site.ID, "one"),
	})
	service := Service{
		Shared: shared,
		AI: fakeOpportunityAI{
			runID: uuid.New(),
			proposal: hitai.OpportunityCandidateProposal{
				TypeKey:          "opportunities.fixture.one.type",
				Category:         "conversion",
				ActionType:       "optimize_checkout",
				Effort:           "medium",
				TitleKey:         "opportunities.fixture.one.title",
				SummaryKey:       "opportunities.fixture.one.summary",
				ActionKey:        "opportunities.fixture.one.action",
				DigestKey:        "opportunities.fixture.one.digest",
				CopyParams:       map[string]any{"allowed": "AI changed the evidence"},
				CitedEvidenceIDs: []string{"secondary"},
			},
		},
		Catalog: catalog,
	}

	opportunities, gotRunID, status, err := service.Generate(context.Background(), GenerateInput{
		TeamID:    teamID,
		Site:      site,
		Store:     shared,
		From:      time.Now().UTC().AddDate(0, 0, -30),
		To:        time.Now().UTC(),
		ActorID:   actorID,
		ActorType: "user",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if status != "invalid_output" {
		t.Fatalf("expected invalid_output status, got %q", status)
	}
	if gotRunID != nil {
		t.Fatalf("expected no accepted AI run ID, got %v", gotRunID)
	}
	if len(opportunities) != 1 {
		t.Fatalf("expected deterministic fallback opportunity, got %d", len(opportunities))
	}
	if opportunities[0].AIRunID != nil {
		t.Fatalf("expected invalid AI run to stay off customer-visible opportunity, got %v", opportunities[0].AIRunID)
	}
	if opportunities[0].CopyParams["allowed"] != "detector one" {
		t.Fatalf("expected detector params to remain authoritative, got %#v", opportunities[0].CopyParams)
	}
}

func TestGenerateRejectsAIProposalWithUnsupportedMetadata(t *testing.T) {
	shared, site, teamID, actorID := setupOpportunityServiceTestStore(t)
	proposal := fixtureAIProposal("one")
	proposal.Category = "search_visibility"
	service := Service{
		Shared: shared,
		AI: fakeOpportunityAI{
			runID:    uuid.New(),
			proposal: proposal,
		},
		Catalog: NewDetectorCatalog(fakeDetector{contract: fixtureDetectorContract("one"), output: fixtureOpportunity(teamID, site.ID, "one")}),
	}

	opportunities, gotRunID, status, err := service.Generate(context.Background(), GenerateInput{
		TeamID:    teamID,
		Site:      site,
		Store:     shared,
		From:      time.Now().UTC().AddDate(0, 0, -30),
		To:        time.Now().UTC(),
		ActorID:   actorID,
		ActorType: "user",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if status != "invalid_output" {
		t.Fatalf("expected invalid_output status, got %q", status)
	}
	if gotRunID != nil {
		t.Fatalf("expected no accepted AI run ID, got %v", gotRunID)
	}
	if len(opportunities) != 1 || opportunities[0].AIRunID != nil {
		t.Fatalf("expected deterministic fallback without AI run, got %#v", opportunities)
	}
}

func TestGenerateAppliesAIProposalToEveryCandidate(t *testing.T) {
	shared, site, teamID, actorID := setupOpportunityServiceTestStore(t)
	firstRunID := uuid.New()
	secondRunID := uuid.New()
	catalog := NewDetectorCatalog(
		fakeDetector{
			contract: fixtureDetectorContract("one"),
			output:   fixtureOpportunity(teamID, site.ID, "one"),
		},
		fakeDetector{
			contract: fixtureDetectorContract("two"),
			output:   fixtureOpportunity(teamID, site.ID, "two"),
		},
	)
	service := Service{
		Shared: shared,
		AI: &sequenceOpportunityAI{
			runIDs: []uuid.UUID{firstRunID, secondRunID},
		},
		Catalog: catalog,
	}

	opportunities, gotRunID, status, err := service.Generate(context.Background(), GenerateInput{
		TeamID:    teamID,
		Site:      site,
		Store:     shared,
		From:      time.Now().UTC().AddDate(0, 0, -30),
		To:        time.Now().UTC(),
		ActorID:   actorID,
		ActorType: "user",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if status != "success" {
		t.Fatalf("expected AI success, got %q", status)
	}
	if gotRunID == nil || *gotRunID != firstRunID {
		t.Fatalf("expected first run ID %s, got %v", firstRunID, gotRunID)
	}
	if len(opportunities) != 2 {
		t.Fatalf("expected two opportunities, got %d", len(opportunities))
	}
	seenRunIDs := map[uuid.UUID]bool{}
	for _, opportunity := range opportunities {
		if opportunity.AIRunID == nil {
			t.Fatalf("expected AI run ID on every opportunity: %#v", opportunity)
		}
		seenRunIDs[*opportunity.AIRunID] = true
		if value, ok := opportunity.CopyParams["allowed"].(string); !ok || !strings.HasPrefix(value, "detector ") {
			t.Fatalf("expected detector proposal params to be retained, got %#v", opportunity.CopyParams)
		}
		if len(opportunity.CitedEvidenceIDs) != 1 || opportunity.CitedEvidenceIDs[0] != "secondary" {
			t.Fatalf("expected AI citations on every opportunity, got %#v", opportunity.CitedEvidenceIDs)
		}
	}
	if !seenRunIDs[firstRunID] || !seenRunIDs[secondRunID] {
		t.Fatalf("expected both AI run IDs, got %#v", seenRunIDs)
	}
}

func TestGenerateAuditRecordsSafeAIStatus(t *testing.T) {
	shared, site, teamID, actorID := setupOpportunityServiceTestStore(t)
	catalog := NewDetectorCatalog(fakeDetector{
		contract: fixtureDetectorContract("one"),
		output:   fixtureOpportunity(teamID, site.ID, "one"),
	})
	service := Service{
		Shared:  shared,
		AI:      fakeOpportunityAI{err: hitai.ErrBudgetExhausted},
		Catalog: catalog,
	}

	opportunities, _, status, err := service.Generate(context.Background(), GenerateInput{
		TeamID:    teamID,
		Site:      site,
		Store:     shared,
		Audit:     &database.AuditEntryParams{ActorID: actorID, TeamID: teamID, Action: "opportunities.generated", TargetType: "site", TargetID: site.ID.String(), Outcome: "success", Details: "generated opportunities"},
		From:      time.Now().UTC().AddDate(0, 0, -30),
		To:        time.Now().UTC(),
		ActorID:   actorID,
		ActorType: "user",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if status != "budget_exhausted" {
		t.Fatalf("expected budget exhausted AI status, got %q", status)
	}
	if len(opportunities) != 1 {
		t.Fatalf("expected deterministic fallback opportunity to persist, got %d", len(opportunities))
	}

	entries, _, err := shared.ListInstanceAuditEntries(context.Background(), database.InstanceAuditFilter{Action: "opportunities.generated", Limit: 10})
	if err != nil {
		t.Fatalf("list audit entries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one audit entry, got %#v", entries)
	}
	if entries[0].Outcome != "degraded" || !strings.Contains(entries[0].Details, "ai_status=budget_exhausted") {
		t.Fatalf("expected degraded audit with safe AI status, got outcome=%q details=%q", entries[0].Outcome, entries[0].Details)
	}
}

func TestGenerateRejectsSchedulerWithoutExplicitScope(t *testing.T) {
	shared, site, teamID, _ := setupOpportunityServiceTestStore(t)
	service := Service{
		Shared:  shared,
		AI:      fakeOpportunityAI{runID: uuid.New()},
		Catalog: NewDetectorCatalog(fakeDetector{contract: fixtureDetectorContract("one"), output: fixtureOpportunity(teamID, site.ID, "one")}),
	}

	opportunities, runID, status, err := service.Generate(context.Background(), GenerateInput{
		TeamID:    teamID,
		Site:      site,
		Store:     shared,
		ActorType: "ai_scheduler",
	})

	if err == nil || !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("expected scheduler scope denial, got err=%v", err)
	}
	if status != "access_denied" {
		t.Fatalf("expected access_denied status, got %q", status)
	}
	if runID != nil || len(opportunities) != 0 {
		t.Fatalf("expected no AI run or opportunities on denied scheduler, got runID=%v opportunities=%#v", runID, opportunities)
	}
}

func TestGenerateRejectsSchedulerWithMismatchedScope(t *testing.T) {
	shared, site, teamID, _ := setupOpportunityServiceTestStore(t)
	service := Service{
		Shared:  shared,
		AI:      fakeOpportunityAI{runID: uuid.New()},
		Catalog: NewDetectorCatalog(fakeDetector{contract: fixtureDetectorContract("one"), output: fixtureOpportunity(teamID, site.ID, "one")}),
	}

	_, _, status, err := service.Generate(context.Background(), GenerateInput{
		TeamID:    teamID,
		Site:      site,
		Store:     shared,
		ActorType: "ai_scheduler",
		SchedulerScope: SchedulerScope{
			TeamID: teamID,
			SiteID: uuid.New(),
		},
	})

	if err == nil || !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("expected scheduler scope denial, got %v", err)
	}
	if status != "access_denied" {
		t.Fatalf("expected access_denied status, got %q", status)
	}
}

func TestGenerateAllowsExplicitlyScopedScheduler(t *testing.T) {
	shared, site, teamID, _ := setupOpportunityServiceTestStore(t)
	runID := uuid.New()
	service := Service{
		Shared:  shared,
		AI:      fakeOpportunityAI{runID: runID, proposal: fixtureAIProposal("one")},
		Catalog: NewDetectorCatalog(fakeDetector{contract: fixtureDetectorContract("one"), output: fixtureOpportunity(teamID, site.ID, "one")}),
	}

	opportunities, gotRunID, status, err := service.Generate(context.Background(), GenerateInput{
		TeamID:    teamID,
		Site:      site,
		Store:     shared,
		ActorType: "ai_scheduler",
		SchedulerScope: SchedulerScope{
			TeamID: teamID,
			SiteID: site.ID,
		},
	})

	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if status != "success" {
		t.Fatalf("expected success, got %q", status)
	}
	if gotRunID == nil || *gotRunID != runID {
		t.Fatalf("expected run ID %s, got %v", runID, gotRunID)
	}
	if len(opportunities) != 1 || opportunities[0].AIRunID == nil || *opportunities[0].AIRunID != runID {
		t.Fatalf("expected scheduler opportunity with AI run %s, got %#v", runID, opportunities)
	}
}

func setupOpportunityServiceTestStore(t *testing.T) (*database.Store, api.Site, uuid.UUID, uuid.UUID) {
	t.Helper()
	store := database.NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	userID, err := store.CreateUser(context.Background(), "opportunity-service@example.com", "hashed")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(context.Background(), userID, "opportunity-service.example")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	teamID, err := store.GetSiteTenantID(context.Background(), site.ID)
	if err != nil {
		t.Fatalf("get site tenant: %v", err)
	}
	return store, *site, teamID, userID
}

type fakeOpportunityAI struct {
	runID    uuid.UUID
	proposal hitai.OpportunityCandidateProposal
	err      error
}

func (f fakeOpportunityAI) GenerateOpportunityProposal(context.Context, hitai.OpportunityRequest) (hitai.OpportunityProposalResult, error) {
	if f.err != nil {
		return hitai.OpportunityProposalResult{}, f.err
	}
	return hitai.OpportunityProposalResult{RunID: f.runID, Proposal: f.proposal}, nil
}

func (fakeOpportunityAI) Configured() bool { return true }
func (fakeOpportunityAI) Enabled() bool    { return true }
func (fakeOpportunityAI) Provider() string { return "test" }
func (fakeOpportunityAI) Model() string    { return "test-model" }

type recordingOpportunityAI struct {
	runID    uuid.UUID
	proposal hitai.OpportunityCandidateProposal
	last     hitai.OpportunityRequest
}

func (f *recordingOpportunityAI) GenerateOpportunityProposal(_ context.Context, req hitai.OpportunityRequest) (hitai.OpportunityProposalResult, error) {
	f.last = req
	return hitai.OpportunityProposalResult{RunID: f.runID, Proposal: f.proposal}, nil
}

func (*recordingOpportunityAI) Configured() bool { return true }
func (*recordingOpportunityAI) Enabled() bool    { return true }
func (*recordingOpportunityAI) Provider() string { return "test" }
func (*recordingOpportunityAI) Model() string    { return "test-model" }

type echoOpportunityAI struct {
	runID            uuid.UUID
	citedEvidenceIDs []string
	last             hitai.OpportunityRequest
}

func (f *echoOpportunityAI) GenerateOpportunityProposal(_ context.Context, req hitai.OpportunityRequest) (hitai.OpportunityProposalResult, error) {
	f.last = req
	citations := append([]string(nil), f.citedEvidenceIDs...)
	if len(citations) == 0 {
		for _, evidence := range req.DetectorInput.Evidence {
			citations = append(citations, evidence.ID)
		}
	}
	return hitai.OpportunityProposalResult{
		RunID: f.runID,
		Proposal: hitai.OpportunityCandidateProposal{
			TypeKey:          req.DetectorInput.TypeKey,
			Category:         req.DetectorInput.Category,
			ActionType:       "optimize_checkout",
			Effort:           "medium",
			TitleKey:         req.DetectorInput.MessageKeys.Title,
			SummaryKey:       req.DetectorInput.MessageKeys.Summary,
			ActionKey:        req.DetectorInput.MessageKeys.Action,
			DigestKey:        req.DetectorInput.MessageKeys.Digest,
			CopyParams:       req.DetectorInput.CopyParams,
			CitedEvidenceIDs: citations,
		},
	}, nil
}

func (*echoOpportunityAI) Configured() bool { return true }
func (*echoOpportunityAI) Enabled() bool    { return true }
func (*echoOpportunityAI) Provider() string { return "test" }
func (*echoOpportunityAI) Model() string    { return "test-model" }

type sequenceOpportunityAI struct {
	runIDs []uuid.UUID
	calls  int
}

func (f *sequenceOpportunityAI) GenerateOpportunityProposal(_ context.Context, req hitai.OpportunityRequest) (hitai.OpportunityProposalResult, error) {
	if f.calls >= len(f.runIDs) {
		return hitai.OpportunityProposalResult{}, errors.New("unexpected sequence call")
	}
	runID := f.runIDs[f.calls]
	f.calls++
	return hitai.OpportunityProposalResult{
		RunID: runID,
		Proposal: hitai.OpportunityCandidateProposal{
			TypeKey:          req.DetectorInput.TypeKey,
			Category:         req.DetectorInput.Category,
			ActionType:       "optimize_checkout",
			Effort:           "medium",
			TitleKey:         req.DetectorInput.MessageKeys.Title,
			SummaryKey:       req.DetectorInput.MessageKeys.Summary,
			ActionKey:        req.DetectorInput.MessageKeys.Action,
			DigestKey:        req.DetectorInput.MessageKeys.Digest,
			CopyParams:       req.DetectorInput.CopyParams,
			CitedEvidenceIDs: []string{"secondary"},
		},
	}, nil
}

func (*sequenceOpportunityAI) Configured() bool { return true }
func (*sequenceOpportunityAI) Enabled() bool    { return true }
func (*sequenceOpportunityAI) Provider() string { return "test" }
func (*sequenceOpportunityAI) Model() string    { return "test-model" }

type noOpportunityDetector struct {
	contract DetectorContract
}

func (d noOpportunityDetector) Contract() DetectorContract {
	return d.contract
}

func (noOpportunityDetector) Detect(DetectorInput) (*database.OpportunityInput, bool) {
	return nil, false
}

func fixtureDetectorContract(name string) DetectorContract {
	prefix := "opportunities.fixture." + name
	return DetectorContract{
		Category: DetectorCategoryConversion,
		TypeKey:  prefix + ".type",
		MessageKeys: DetectorMessageKeys{
			Title:       prefix + ".title",
			Summary:     prefix + ".summary",
			Action:      prefix + ".action",
			Digest:      prefix + ".digest",
			ImpactLabel: prefix + ".impact",
			RouteLabel:  prefix + ".route",
		},
		AllowedParams: []string{"allowed"},
	}
}

func fixtureAIProposal(name string) hitai.OpportunityCandidateProposal {
	contract := fixtureDetectorContract(name)
	return hitai.OpportunityCandidateProposal{
		TypeKey:          contract.TypeKey,
		Category:         string(contract.Category),
		ActionType:       "optimize_checkout",
		Effort:           "medium",
		TitleKey:         contract.MessageKeys.Title,
		SummaryKey:       contract.MessageKeys.Summary,
		ActionKey:        contract.MessageKeys.Action,
		DigestKey:        contract.MessageKeys.Digest,
		CopyParams:       map[string]any{"allowed": "detector " + name},
		CitedEvidenceIDs: []string{"secondary"},
	}
}

func fixtureOpportunity(teamID, siteID uuid.UUID, name string) database.OpportunityInput {
	contract := fixtureDetectorContract(name)
	return database.OpportunityInput{
		ID:              uuid.New(),
		TeamID:          teamID,
		SiteID:          siteID,
		Kind:            "conversion",
		TypeKey:         contract.TypeKey,
		TitleKey:        contract.MessageKeys.Title,
		SummaryKey:      contract.MessageKeys.Summary,
		ActionKey:       contract.MessageKeys.Action,
		DigestKey:       contract.MessageKeys.Digest,
		CopyParams:      map[string]any{"allowed": "detector " + name},
		ImpactValue:     "EUR 900",
		ImpactLabelKey:  contract.MessageKeys.ImpactLabel,
		MonthlyUpside:   900,
		Confidence:      "medium",
		Score:           80,
		Status:          "new",
		RouteLabelKey:   contract.MessageKeys.RouteLabel,
		RouteParams:     map[string]any{"allowed": "route"},
		RouteIcon:       "pi pi-compass",
		DetectorVersion: detectorVersion,
		Evidence: []api.OpportunityEvidence{
			{ID: "primary", LabelKey: "opportunities.fixture.primary", Value: "42%"},
			{ID: "secondary", LabelKey: "opportunities.fixture.secondary", Value: "17"},
		},
		CitedEvidenceIDs: []string{"primary"},
		GeneratedAt:      time.Now().UTC(),
	}
}

func evidenceIDList(evidence []hitai.Evidence) []string {
	out := make([]string, 0, len(evidence))
	for _, item := range evidence {
		out = append(out, item.ID)
	}
	return out
}
