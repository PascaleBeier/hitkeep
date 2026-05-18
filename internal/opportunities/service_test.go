package opportunities

import (
	"context"
	"errors"
	"fmt"
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

func TestGeneratePersistsGoalSetupSuggestionFromTrackedConversionEvent(t *testing.T) {
	shared, site, teamID, actorID := setupOpportunityServiceTestStore(t)
	ctx := context.Background()
	from := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 30)
	sessionID := uuid.New()
	isUnique := true
	for i := range minSetupGoalEventCount {
		timestamp := from.Add(time.Duration(i+1) * time.Hour)
		requireNoError(t, shared.CreateHit(ctx, &api.Hit{
			SiteID:    site.ID,
			SessionID: sessionID,
			PageID:    uuid.New(),
			Path:      "/pricing",
			Timestamp: timestamp,
			IsUnique:  &isUnique,
		}), "create hit")
		requireNoError(t, shared.CreateEvent(ctx, &api.Event{
			SiteID:    site.ID,
			SessionID: sessionID,
			Name:      "demo_request",
			Timestamp: timestamp.Add(5 * time.Minute),
		}), "create event")
	}

	opportunities, _, aiStatus, err := (Service{Shared: shared}).Generate(ctx, GenerateInput{
		Store:           shared,
		Site:            site,
		TeamID:          teamID,
		ActorID:         actorID,
		EffectiveUserID: actorID,
		ActorType:       "user",
		From:            from,
		To:              to,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if aiStatus != "disabled" {
		t.Fatalf("expected disabled AI status, got %q", aiStatus)
	}
	suggestion := findAPIOpportunityByType(opportunities, "opportunities.types.setup_goal_suggestion")
	if suggestion == nil {
		t.Fatalf("expected setup goal suggestion, got %#v", opportunities)
	}
	if suggestion.CopyParams["event_name"] != "demo_request" || suggestion.ImpactValue != "3" {
		t.Fatalf("expected demo request setup suggestion, got %#v", suggestion)
	}
}

func TestGeneratePersistsFunnelSetupSuggestionFromTrackedPageAndConversionEvent(t *testing.T) {
	shared, site, teamID, actorID := setupOpportunityServiceTestStore(t)
	ctx := context.Background()
	from := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 30)
	sessionID := uuid.New()
	isUnique := true
	for i := range minSetupFunnelStartPageviews {
		timestamp := from.Add(time.Duration(i+1) * time.Minute)
		requireNoError(t, shared.CreateHit(ctx, &api.Hit{
			SiteID:    site.ID,
			SessionID: sessionID,
			PageID:    uuid.New(),
			Path:      "/pricing",
			Timestamp: timestamp,
			IsUnique:  &isUnique,
		}), "create hit")
	}
	for i := range minSetupGoalEventCount {
		requireNoError(t, shared.CreateEvent(ctx, &api.Event{
			SiteID:    site.ID,
			SessionID: sessionID,
			Name:      "demo_request",
			Timestamp: from.Add(time.Duration(i+1) * time.Hour),
		}), "create event")
	}

	opportunities, _, _, err := (Service{Shared: shared}).Generate(ctx, GenerateInput{
		Store:           shared,
		Site:            site,
		TeamID:          teamID,
		ActorID:         actorID,
		EffectiveUserID: actorID,
		ActorType:       "user",
		From:            from,
		To:              to,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	suggestion := findAPIOpportunityByType(opportunities, "opportunities.types.setup_funnel_suggestion")
	if suggestion == nil {
		t.Fatalf("expected setup funnel suggestion, got %#v", opportunities)
	}
	if suggestion.CopyParams["start_path"] != "/pricing" || suggestion.CopyParams["conversion_event"] != "demo_request" {
		t.Fatalf("expected pricing to demo funnel suggestion, got %#v", suggestion.CopyParams)
	}
}

func TestGenerateSuppressesNoDataSetupOpportunityBeforeAI(t *testing.T) {
	shared, site, teamID, actorID := setupOpportunityServiceTestStore(t)
	ai := &countingEchoOpportunityAI{}
	from := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 30)

	opportunities, runID, aiStatus, err := (Service{Shared: shared, AI: ai}).Generate(context.Background(), GenerateInput{
		Store:           shared,
		Site:            site,
		TeamID:          teamID,
		ActorID:         actorID,
		EffectiveUserID: actorID,
		ActorType:       "user",
		From:            from,
		To:              to,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(opportunities) != 0 {
		t.Fatalf("expected no opportunities for a zero-signal site, got %#v", opportunities)
	}
	if runID != nil {
		t.Fatalf("expected no AI run for a zero-signal site, got %s", *runID)
	}
	if aiStatus != "no_opportunities" {
		t.Fatalf("expected no_opportunities AI status, got %q", aiStatus)
	}
	if ai.calls != 0 {
		t.Fatalf("expected AI not to run for zero-signal setup, got %d calls", ai.calls)
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
		Category: DetectorCategoryTraffic,
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
		ActionTypes:   []string{"optimize_checkout", "investigate"},
	}
	output := database.OpportunityInput{
		ID:              uuid.New(),
		TeamID:          teamID,
		SiteID:          site.ID,
		Kind:            "traffic",
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
	if ai.last.DetectorInput.Category != string(DetectorCategoryTraffic) {
		t.Fatalf("expected active catalog category, got %#v", ai.last.DetectorInput)
	}
	if strings.Join(ai.last.DetectorInput.AllowedParams, ",") != "custom_signal" {
		t.Fatalf("expected active catalog allowed params, got %#v", ai.last.DetectorInput.AllowedParams)
	}
	if strings.Join(ai.last.DetectorInput.AllowedActionTypes, ",") != "optimize_checkout,investigate" {
		t.Fatalf("expected active catalog action types, got %#v", ai.last.DetectorInput.AllowedActionTypes)
	}
}

func TestGenerateLoadsOnlySignalsRequiredByActiveCatalog(t *testing.T) {
	shared, site, teamID, actorID := setupOpportunityServiceTestStore(t)
	contract := fixtureDetectorContract("one")
	contract.Category = DetectorCategoryTrafficQuality
	contract.RequiredSignals = []OpportunitySignal{OpportunitySignalSiteStats}
	detector := signalInspectingDetector{
		contract: contract,
		output:   fixtureOpportunity(teamID, site.ID, "one"),
	}
	service := Service{
		Shared:  shared,
		AI:      nil,
		Catalog: NewDetectorCatalog(&detector),
	}

	_, _, _, err := service.Generate(context.Background(), GenerateInput{
		TeamID:    teamID,
		Site:      site,
		Store:     shared,
		ActorID:   actorID,
		ActorType: "user",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if detector.seen.Stats == nil {
		t.Fatalf("expected required site stats to be loaded")
	}
	if detector.seen.Ecommerce != nil || detector.seen.AIVisibility != nil {
		t.Fatalf("expected unrequested signals to stay nil, got ecommerce=%#v ai=%#v", detector.seen.Ecommerce, detector.seen.AIVisibility)
	}
}

func TestGenerateLoadsOptionalSupportingSignalsForActiveCatalog(t *testing.T) {
	shared, site, teamID, actorID := setupOpportunityServiceTestStore(t)
	contract := fixtureDetectorContract("one")
	contract.Category = DetectorCategoryAIVisibility
	contract.RequiredSignals = []OpportunitySignal{OpportunitySignalAIVisibility}
	contract.OptionalSignals = []OpportunitySignal{OpportunitySignalSiteStats}
	detector := signalInspectingDetector{
		contract: contract,
		output:   fixtureOpportunity(teamID, site.ID, "one"),
	}
	service := Service{
		Shared:  shared,
		AI:      nil,
		Catalog: NewDetectorCatalog(&detector),
	}

	_, _, _, err := service.Generate(context.Background(), GenerateInput{
		TeamID:    teamID,
		Site:      site,
		Store:     shared,
		ActorID:   actorID,
		ActorType: "user",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if detector.seen.AIVisibility == nil {
		t.Fatalf("expected required AI visibility signal to be loaded")
	}
	if detector.seen.Stats == nil {
		t.Fatalf("expected optional site stats signal to be loaded for supporting evidence")
	}
	if detector.seen.Ecommerce != nil {
		t.Fatalf("expected unrelated ecommerce signal to stay nil")
	}
}

func TestGenerateLoadsSearchConsoleSignalThroughMappedProperty(t *testing.T) {
	shared, site, teamID, actorID := setupOpportunityServiceTestStore(t)
	from := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)
	propertyURI := "sc-domain:example.com"
	if err := shared.UpsertGoogleSearchConsoleSiteMapping(context.Background(), database.GoogleSearchConsoleSiteMappingInput{
		SiteID:      site.ID,
		TeamID:      teamID,
		PropertyURI: propertyURI,
		MappedBy:    actorID,
		MappedAt:    from,
	}); err != nil {
		t.Fatalf("upsert search console mapping: %v", err)
	}
	if err := shared.UpsertSearchConsoleFact(context.Background(), database.SearchConsoleFactInput{
		SiteID:          site.ID,
		PropertyURI:     propertyURI,
		Date:            time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC),
		Clicks:          54,
		Impressions:     4200,
		CTR:             0.0129,
		Position:        8.4,
		AggregationType: "by_property",
		DataState:       "final",
	}); err != nil {
		t.Fatalf("upsert search console fact: %v", err)
	}
	contract := fixtureDetectorContract("one")
	contract.Category = DetectorCategorySearchVisibility
	contract.RequiredSignals = []OpportunitySignal{OpportunitySignalSearchConsole}
	detector := signalInspectingDetector{
		contract: contract,
		output:   fixtureOpportunity(teamID, site.ID, "one"),
	}
	service := Service{
		Shared:  shared,
		AI:      nil,
		Catalog: NewDetectorCatalog(&detector),
	}

	_, _, _, err := service.Generate(context.Background(), GenerateInput{
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
	if detector.seen.SearchConsole == nil {
		t.Fatalf("expected required Search Console signal to be loaded")
	}
	if detector.seen.SearchConsole.Impressions != 4200 || detector.seen.SearchConsole.Clicks != 54 {
		t.Fatalf("expected Search Console overview to use mapped property, got %#v", detector.seen.SearchConsole)
	}
}

func TestGenerateLoadsEventNamesSignalForActiveCatalog(t *testing.T) {
	shared, site, teamID, actorID := setupOpportunityServiceTestStore(t)
	from := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)
	for _, eventName := range []string{"external_link_click", "download"} {
		if err := shared.CreateEvent(context.Background(), &api.Event{
			SiteID:     site.ID,
			SessionID:  uuid.New(),
			Name:       eventName,
			Properties: map[string]any{},
			Timestamp:  from.Add(24 * time.Hour),
		}); err != nil {
			t.Fatalf("create event %q: %v", eventName, err)
		}
	}
	contract := fixtureDetectorContract("one")
	contract.Category = DetectorCategorySetupQuality
	contract.RequiredSignals = []OpportunitySignal{OpportunitySignalEvents}
	detector := signalInspectingDetector{
		contract: contract,
		output:   fixtureOpportunity(teamID, site.ID, "one"),
	}
	service := Service{
		Shared:  shared,
		AI:      nil,
		Catalog: NewDetectorCatalog(&detector),
	}

	_, _, _, err := service.Generate(context.Background(), GenerateInput{
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
	if strings.Join(detector.seen.EventNames, ",") != "download,external_link_click" {
		t.Fatalf("expected event names signal to be loaded, got %#v", detector.seen.EventNames)
	}
}

func TestGenerateLoadsWebVitalsSignalForActiveCatalog(t *testing.T) {
	shared, site, teamID, actorID := setupOpportunityServiceTestStore(t)
	from := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)
	seedOpportunityWebVitals(t, context.Background(), shared, site.ID, from, minWebVitalsSamples)
	contract := fixtureDetectorContract("one")
	contract.Category = DetectorCategoryPerformance
	contract.RequiredSignals = []OpportunitySignal{OpportunitySignalWebVitals}
	detector := signalInspectingDetector{
		contract: contract,
		output:   fixtureOpportunity(teamID, site.ID, "one"),
	}
	service := Service{
		Shared:  shared,
		AI:      nil,
		Catalog: NewDetectorCatalog(&detector),
	}

	_, _, _, err := service.Generate(context.Background(), GenerateInput{
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
	requireWebVitalsSummaryCount(t, detector.seen.WebVitals, 1)
	assertPoorLCPWebVitalsSignal(t, detector.seen.WebVitals)
}

func TestGeneratePersistsWebVitalsOpportunityFromStoredSamples(t *testing.T) {
	shared, site, teamID, actorID := setupOpportunityServiceTestStore(t)
	ctx := context.Background()
	from := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)
	seedOpportunityWebVitals(t, ctx, shared, site.ID, from, minWebVitalsSamples+10)

	opportunities, _, aiStatus, err := (Service{Shared: shared}).Generate(ctx, GenerateInput{
		Store:           shared,
		Site:            site,
		TeamID:          teamID,
		ActorID:         actorID,
		EffectiveUserID: actorID,
		ActorType:       "user",
		From:            from,
		To:              to,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if aiStatus != "disabled" {
		t.Fatalf("expected disabled AI status, got %q", aiStatus)
	}
	opportunity := findAPIOpportunityByType(opportunities, "opportunities.types.web_vitals_performance")
	if opportunity == nil {
		t.Fatalf("expected web vitals opportunity, got %#v", opportunities)
	}
	assertPersistedWebVitalsOpportunity(t, *opportunity)
}

func seedOpportunityWebVitals(t *testing.T, ctx context.Context, store *database.Store, siteID uuid.UUID, from time.Time, samples int) {
	t.Helper()
	city := "Berlin"
	provider := "Hetzner Online GmbH"
	asnOrg := "Hetzner Online GmbH"
	asn := 24940
	for i := range samples {
		sessionID := uuid.New()
		pageID := uuid.New()
		requireNoError(t, store.CreateHit(ctx, &api.Hit{
			SiteID:    siteID,
			SessionID: sessionID,
			PageID:    pageID,
			Timestamp: from.Add(time.Duration(i)*time.Hour + 30*time.Minute),
			Path:      "/pricing",
			City:      &city,
			Provider:  &provider,
			ASN:       &asn,
			ASNOrg:    &asnOrg,
		}), "create web vital hit context")
		requireNoError(t, store.CreateWebVital(ctx, &api.WebVital{
			SiteID:    siteID,
			SessionID: sessionID,
			PageID:    pageID,
			Metric:    api.WebVitalLCP,
			Value:     4200 + float64(i),
			Path:      "/pricing",
			Timestamp: from.Add(time.Duration(i+1) * time.Hour),
		}), "create web vital")
	}
}

func assertPoorLCPWebVitalsSignal(t *testing.T, snapshot *WebVitalsEvidenceSnapshot) {
	t.Helper()
	summary := snapshot.Summary[0]
	if summary.Metric != api.WebVitalLCP {
		t.Fatalf("expected LCP summary, got %#v", summary)
	}
	if summary.Rating != api.WebVitalRatingPoor {
		t.Fatalf("expected poor LCP summary, got %#v", summary)
	}
	pages := snapshot.Pages[api.WebVitalLCP]
	if len(pages) != 1 {
		t.Fatalf("expected one LCP page breakdown, got %#v", pages)
	}
	if pages[0].Path != "/pricing" {
		t.Fatalf("expected pricing page breakdown, got %#v", pages[0])
	}
	dimensions := snapshot.Dimensions[api.WebVitalLCP]
	if len(dimensions.TopCities) != 1 || dimensions.TopCities[0].Name != "Berlin" {
		t.Fatalf("expected Berlin web vitals city evidence, got %#v", dimensions.TopCities)
	}
	if len(dimensions.TopProviders) != 1 || dimensions.TopProviders[0].Name != "Hetzner Online GmbH" {
		t.Fatalf("expected Hetzner web vitals provider evidence, got %#v", dimensions.TopProviders)
	}
	if len(dimensions.TopASNs) != 1 || dimensions.TopASNs[0].Name != "AS24940 Hetzner Online GmbH" {
		t.Fatalf("expected Hetzner web vitals ASN evidence, got %#v", dimensions.TopASNs)
	}
}

func requireWebVitalsSummaryCount(t *testing.T, snapshot *WebVitalsEvidenceSnapshot, expected int) {
	t.Helper()
	if snapshot == nil {
		t.Fatal("expected web vitals signal to be loaded")
	}
	if len(snapshot.Summary) != expected {
		t.Fatalf("expected %d web vitals summary rows, got %#v", expected, snapshot.Summary)
	}
}

func assertPersistedWebVitalsOpportunity(t *testing.T, opportunity api.Opportunity) {
	t.Helper()
	if opportunity.CopyParams["metric"] != "LCP" {
		t.Fatalf("expected LCP copy param, got %#v", opportunity.CopyParams)
	}
	if opportunity.CopyParams["path"] != "/pricing" {
		t.Fatalf("expected pricing copy param, got %#v", opportunity.CopyParams)
	}
	if opportunity.CopyParams["top_city"] != "Berlin" || opportunity.CopyParams["top_provider"] != "Hetzner Online GmbH" || opportunity.CopyParams["top_asn"] != "AS24940 Hetzner Online GmbH" {
		t.Fatalf("expected web vitals geo/network copy params, got %#v", opportunity.CopyParams)
	}
	for _, evidenceID := range []string{"top_city", "top_provider", "top_asn"} {
		if !containsString(opportunity.CitedEvidenceIDs, evidenceID) {
			t.Fatalf("expected cited evidence %q, got %#v", evidenceID, opportunity.CitedEvidenceIDs)
		}
	}
	if opportunity.ScoreBreakdown.EvidenceFit < 95 {
		t.Fatalf("expected strong evidence fit, got %#v", opportunity.ScoreBreakdown)
	}
	if opportunity.Confidence != "medium" {
		t.Fatalf("expected medium confidence, got %q", opportunity.Confidence)
	}
}

func TestGenerateRejectsUnknownRequiredSignal(t *testing.T) {
	shared, site, teamID, actorID := setupOpportunityServiceTestStore(t)
	contract := fixtureDetectorContract("one")
	contract.RequiredSignals = []OpportunitySignal{"unknown_signal"}
	detector := signalInspectingDetector{
		contract: contract,
		output:   fixtureOpportunity(teamID, site.ID, "one"),
	}
	service := Service{
		Shared:  shared,
		AI:      nil,
		Catalog: NewDetectorCatalog(&detector),
	}

	opportunities, runID, status, err := service.Generate(context.Background(), GenerateInput{
		TeamID:    teamID,
		Site:      site,
		Store:     shared,
		ActorID:   actorID,
		ActorType: "user",
	})
	if err == nil || !strings.Contains(err.Error(), "unknown opportunity signal") {
		t.Fatalf("expected unknown signal error, got %v", err)
	}
	if status != "detector_failed" || runID != nil || len(opportunities) != 0 {
		t.Fatalf("expected detector_failed without output, got status=%q runID=%v opportunities=%#v", status, runID, opportunities)
	}
	if detector.seen.SiteID != uuid.Nil {
		t.Fatalf("expected detector not to run when required signals are invalid")
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

func TestGenerateSuppressesSavedOpportunityWithSameIdentityEvidence(t *testing.T) {
	shared, site, teamID, actorID := setupOpportunityServiceTestStore(t)
	opportunityID := uuid.New()
	detector := &sequenceDetector{
		contract: identityDetectorContract("semantic"),
		outputs: []database.OpportunityInput{
			identityOpportunity(teamID, site.ID, opportunityID, "openalternative.co", 709),
			identityOpportunity(teamID, site.ID, opportunityID, "openalternative.co", 985),
		},
	}
	detector.outputs[0].RouteParams = nil
	detector.outputs[1].RouteParams = nil
	ai := &countingEchoOpportunityAI{}
	service := Service{
		Shared:  shared,
		AI:      ai,
		Catalog: NewDetectorCatalog(detector),
	}

	first, firstRunID, firstStatus, err := service.Generate(context.Background(), GenerateInput{
		TeamID:    teamID,
		Site:      site,
		Store:     shared,
		ActorID:   actorID,
		ActorType: "user",
	})
	if err != nil {
		t.Fatalf("first Generate: %v", err)
	}
	if firstStatus != "success" || firstRunID == nil || len(first) != 1 || ai.calls != 1 {
		t.Fatalf("expected first generation to persist with AI, status=%q run=%v len=%d aiCalls=%d", firstStatus, firstRunID, len(first), ai.calls)
	}

	second, secondRunID, secondStatus, err := service.Generate(context.Background(), GenerateInput{
		TeamID:    teamID,
		Site:      site,
		Store:     shared,
		ActorID:   actorID,
		ActorType: "user",
	})
	if err != nil {
		t.Fatalf("second Generate: %v", err)
	}
	if secondStatus != "no_opportunities" || secondRunID != nil || len(second) != 0 {
		t.Fatalf("expected duplicate semantic opportunity to be suppressed, status=%q run=%v len=%d", secondStatus, secondRunID, len(second))
	}
	if ai.calls != 1 {
		t.Fatalf("expected duplicate suppression before AI call, got %d calls", ai.calls)
	}
}

func TestGenerateReturnsRankedActionableOpportunities(t *testing.T) {
	shared, site, teamID, actorID := setupOpportunityServiceTestStore(t)
	low := fixtureOpportunity(teamID, site.ID, "low")
	low.Score = 62
	low.ScoreBreakdown = api.OpportunityScoreBreakdown{Impact: 40, Actionability: 45, EvidenceFit: 50, Total: 62}
	high := fixtureOpportunity(teamID, site.ID, "high")
	high.Score = 86
	high.ScoreBreakdown = api.OpportunityScoreBreakdown{Impact: 80, Actionability: 92, EvidenceFit: 90, Total: 86}
	done := fixtureOpportunity(teamID, site.ID, "done")
	done.Score = 99
	done.Status = "done"
	done.ScoreBreakdown = api.OpportunityScoreBreakdown{Impact: 99, Actionability: 99, EvidenceFit: 99, Total: 99}

	service := Service{
		Shared: shared,
		AI:     &countingEchoOpportunityAI{},
		Catalog: NewDetectorCatalog(
			fakeDetector{contract: fixtureDetectorContract("low"), output: low},
			fakeDetector{contract: fixtureDetectorContract("done"), output: done},
			fakeDetector{contract: fixtureDetectorContract("high"), output: high},
		),
	}

	opportunities, _, status, err := service.Generate(context.Background(), GenerateInput{
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
		t.Fatalf("expected success, got %q", status)
	}
	gotIDs := opportunityIDs(opportunities)
	wantIDs := []uuid.UUID{high.ID, low.ID, done.ID}
	if !sameUUIDs(gotIDs, wantIDs) {
		t.Fatalf("expected ranked opportunities %v, got %v", wantIDs, gotIDs)
	}
}

func TestGenerateRegeneratesSavedOpportunityWhenIdentityEvidenceChanges(t *testing.T) {
	shared, site, teamID, actorID := setupOpportunityServiceTestStore(t)
	opportunityID := uuid.New()
	detector := &sequenceDetector{
		contract: identityDetectorContract("semantic"),
		outputs: []database.OpportunityInput{
			identityOpportunity(teamID, site.ID, opportunityID, "openalternative.co", 709),
			identityOpportunity(teamID, site.ID, opportunityID, "pascalebeier", 985),
		},
	}
	ai := &countingEchoOpportunityAI{}
	service := Service{
		Shared:  shared,
		AI:      ai,
		Catalog: NewDetectorCatalog(detector),
	}

	_, _, _, err := service.Generate(context.Background(), GenerateInput{
		TeamID:    teamID,
		Site:      site,
		Store:     shared,
		ActorID:   actorID,
		ActorType: "user",
	})
	if err != nil {
		t.Fatalf("first Generate: %v", err)
	}
	second, secondRunID, secondStatus, err := service.Generate(context.Background(), GenerateInput{
		TeamID:    teamID,
		Site:      site,
		Store:     shared,
		ActorID:   actorID,
		ActorType: "user",
	})
	if err != nil {
		t.Fatalf("second Generate: %v", err)
	}
	if secondStatus != "success" || secondRunID == nil || len(second) != 1 {
		t.Fatalf("expected changed identity evidence to regenerate, status=%q run=%v len=%d", secondStatus, secondRunID, len(second))
	}
	if ai.calls != 2 {
		t.Fatalf("expected changed identity to call AI twice, got %d calls", ai.calls)
	}
	if second[0].CopyParams["source"] != "pascalebeier" {
		t.Fatalf("expected regenerated opportunity to persist changed identity, got %#v", second[0].CopyParams)
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

func findAPIOpportunityByType(opportunities []api.Opportunity, typeKey string) *api.Opportunity {
	for i := range opportunities {
		if opportunities[i].TypeKey == typeKey {
			return &opportunities[i]
		}
	}
	return nil
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

type countingEchoOpportunityAI struct {
	calls int
	last  hitai.OpportunityRequest
}

func (f *countingEchoOpportunityAI) GenerateOpportunityProposal(_ context.Context, req hitai.OpportunityRequest) (hitai.OpportunityProposalResult, error) {
	f.calls++
	f.last = req
	citations := make([]string, 0, len(req.DetectorInput.Evidence))
	for _, evidence := range req.DetectorInput.Evidence {
		citations = append(citations, evidence.ID)
	}
	runID := uuid.New()
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
			CitedEvidenceIDs: citations,
		},
	}, nil
}

func (*countingEchoOpportunityAI) Configured() bool { return true }
func (*countingEchoOpportunityAI) Enabled() bool    { return true }
func (*countingEchoOpportunityAI) Provider() string { return "test" }
func (*countingEchoOpportunityAI) Model() string    { return "test-model" }

type noOpportunityDetector struct {
	contract DetectorContract
}

func (d noOpportunityDetector) Contract() DetectorContract {
	return d.contract
}

func (noOpportunityDetector) Detect(DetectorInput) (*database.OpportunityInput, bool) {
	return nil, false
}

type signalInspectingDetector struct {
	contract DetectorContract
	output   database.OpportunityInput
	seen     DetectorInput
}

func (d *signalInspectingDetector) Contract() DetectorContract {
	return d.contract
}

func (d *signalInspectingDetector) Detect(input DetectorInput) (*database.OpportunityInput, bool) {
	d.seen = input
	return &d.output, true
}

type sequenceDetector struct {
	contract DetectorContract
	outputs  []database.OpportunityInput
	calls    int
}

func (d *sequenceDetector) Contract() DetectorContract {
	return d.contract
}

func (d *sequenceDetector) Detect(DetectorInput) (*database.OpportunityInput, bool) {
	if d.calls >= len(d.outputs) {
		return nil, false
	}
	output := d.outputs[d.calls]
	d.calls++
	return &output, true
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

func identityDetectorContract(name string) DetectorContract {
	contract := fixtureDetectorContract(name)
	contract.AllowedParams = []string{"source", "pageviews"}
	contract.IdentityEvidenceIDs = []string{"top_source"}
	return contract
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

func identityOpportunity(teamID, siteID, id uuid.UUID, source string, pageviews int) database.OpportunityInput {
	contract := identityDetectorContract("semantic")
	return database.OpportunityInput{
		ID:              id,
		TeamID:          teamID,
		SiteID:          siteID,
		Kind:            "traffic",
		TypeKey:         contract.TypeKey,
		TitleKey:        contract.MessageKeys.Title,
		SummaryKey:      contract.MessageKeys.Summary,
		ActionKey:       contract.MessageKeys.Action,
		DigestKey:       contract.MessageKeys.Digest,
		CopyParams:      map[string]any{"source": source, "pageviews": pageviews},
		ImpactValue:     fmt.Sprintf("%d", pageviews),
		ImpactLabelKey:  contract.MessageKeys.ImpactLabel,
		Confidence:      "high",
		Score:           80,
		Status:          "new",
		RouteLabelKey:   contract.MessageKeys.RouteLabel,
		RouteParams:     map[string]any{"source": source},
		RouteIcon:       "pi pi-compass",
		DetectorVersion: detectorVersion,
		Evidence: []api.OpportunityEvidence{
			{ID: "pageviews", LabelKey: "opportunities.fixture.pageviews", Value: fmt.Sprintf("%d", pageviews)},
			{ID: "top_source", LabelKey: "opportunities.fixture.top_source", Value: source},
		},
		CitedEvidenceIDs: []string{"pageviews", "top_source"},
		GeneratedAt:      time.Now().UTC(),
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

func opportunityIDs(opportunities []api.Opportunity) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(opportunities))
	for _, opportunity := range opportunities {
		out = append(out, opportunity.ID)
	}
	return out
}

func sameUUIDs(a, b []uuid.UUID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
