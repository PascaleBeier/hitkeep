package opportunities

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	hitai "hitkeep/internal/ai"
	"hitkeep/internal/api"
	"hitkeep/internal/database"
)

func TestDefaultDetectorCatalogSuppressesTrackingSetupOpportunityWithoutEvidence(t *testing.T) {
	siteID := uuid.New()
	teamID := uuid.New()
	generatedAt := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)

	opportunities, err := NewDefaultDetectorCatalog().Detect(DetectorInput{
		TeamID:      teamID,
		SiteID:      siteID,
		GeneratedAt: generatedAt,
	})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if findOpportunityByType(opportunities, "opportunities.types.tracking_setup") != nil {
		t.Fatalf("expected no-evidence tracking setup suggestion to be suppressed, got %#v", opportunities)
	}
}

func TestDefaultDetectorCatalogGeneratesCheckoutOpportunityFromDropoff(t *testing.T) {
	siteID := uuid.New()
	teamID := uuid.New()
	generatedAt := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)

	input := DetectorInput{
		TeamID:      teamID,
		SiteID:      siteID,
		GeneratedAt: generatedAt,
		Ecommerce: &api.EcommerceSummary{
			CheckoutStarts:         120,
			Orders:                 28,
			AverageOrderValue:      95,
			CheckoutConversionRate: 23.3,
			Currency:               "EUR",
		},
	}
	assertCatalogFixture(t, input, "conversion", "opportunities.types.checkout_conversion", "conversion_rate", "conversion_rate", DetectorCategoryConversion)

	opportunities, err := NewDefaultDetectorCatalog().Detect(input)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	checkout := findOpportunityByType(opportunities, "opportunities.types.checkout_conversion")
	if checkout == nil {
		t.Fatalf("expected checkout opportunity, got %#v", opportunities)
	}
	if checkout.ScoreBreakdown.Total != checkout.Score || checkout.ScoreBreakdown.EvidenceFit == 0 {
		t.Fatalf("expected persisted checkout score breakdown, got %#v", checkout.ScoreBreakdown)
	}
}

func TestDefaultDetectorCatalogSuppressesCheckoutOpportunityForTinySample(t *testing.T) {
	catalog := NewDefaultDetectorCatalog()
	opportunities, err := catalog.Detect(DetectorInput{
		TeamID:      uuid.New(),
		SiteID:      uuid.New(),
		GeneratedAt: time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC),
		Ecommerce: &api.EcommerceSummary{
			CheckoutStarts:         12,
			Orders:                 2,
			AverageOrderValue:      95,
			CheckoutConversionRate: 16.7,
			Currency:               "EUR",
		},
	})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	for _, opportunity := range opportunities {
		if opportunity.TypeKey == "opportunities.types.checkout_conversion" {
			t.Fatalf("expected tiny checkout sample to be suppressed, got %#v", opportunity)
		}
	}
}

func TestDefaultDetectorCatalogGeneratesTrafficOpportunityFromSource(t *testing.T) {
	siteID := uuid.New()
	teamID := uuid.New()
	generatedAt := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)

	assertCatalogFixture(t, DetectorInput{
		TeamID:      teamID,
		SiteID:      siteID,
		GeneratedAt: generatedAt,
		Stats: &api.SiteStats{
			TotalPageviews: 640,
			UniqueSessions: 310,
			TopUTMSources:  []api.MetricStat{{Name: "paid-search", Value: 188}},
		},
	}, "traffic", "opportunities.types.traffic_quality", "source", "top_source", DetectorCategoryTrafficQuality)
}

func TestDefaultDetectorCatalogUsesSourceSpecificTrafficEvidence(t *testing.T) {
	opportunities, err := NewDefaultDetectorCatalog().Detect(DetectorInput{
		TeamID:      uuid.New(),
		SiteID:      uuid.New(),
		GeneratedAt: time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC),
		Stats: &api.SiteStats{
			TotalPageviews: 2000,
			UniqueSessions: 900,
			TopUTMSources:  []api.MetricStat{{Name: "openalternative", Value: 240}},
			TopReferrers:   []api.MetricStat{{Name: "example.com", Value: 800}},
		},
	})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	opportunity := findOpportunityByType(opportunities, "opportunities.types.traffic_quality")
	if opportunity == nil {
		t.Fatalf("expected traffic opportunity, got %#v", opportunities)
	}
	if opportunity.CopyParams["source"] != "openalternative" || opportunity.CopyParams["source_hits"] != 240 || opportunity.CopyParams["total_pageviews"] != 2000 {
		t.Fatalf("expected source-specific traffic params, got %#v", opportunity.CopyParams)
	}
	if opportunity.ImpactValue != "240" {
		t.Fatalf("expected source-specific impact value, got %q", opportunity.ImpactValue)
	}
	for _, evidenceID := range []string{"top_source", "source_hits", "total_pageviews", "sessions"} {
		if !hasEvidenceID(opportunity.Evidence, evidenceID) {
			t.Fatalf("expected evidence %q in %#v", evidenceID, opportunity.Evidence)
		}
		if !slices.Contains(opportunity.CitedEvidenceIDs, evidenceID) {
			t.Fatalf("expected cited evidence %q in %#v", evidenceID, opportunity.CitedEvidenceIDs)
		}
	}
}

func TestDefaultDetectorCatalogSuppressesTrafficOpportunityForWeakSourceShare(t *testing.T) {
	opportunities, err := NewDefaultDetectorCatalog().Detect(DetectorInput{
		TeamID:      uuid.New(),
		SiteID:      uuid.New(),
		GeneratedAt: time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC),
		Stats: &api.SiteStats{
			TotalPageviews: 5000,
			UniqueSessions: 3200,
			TopUTMSources:  []api.MetricStat{{Name: "openalternative", Value: 120}},
		},
	})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if findOpportunityByType(opportunities, "opportunities.types.traffic_quality") != nil {
		t.Fatalf("expected weak source share to suppress traffic opportunity, got %#v", opportunities)
	}
}

func TestDefaultDetectorCatalogEnrichesAIVisibilityWithSiteTrafficEvidence(t *testing.T) {
	siteID := uuid.New()
	teamID := uuid.New()
	generatedAt := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)

	opportunities, err := NewDefaultDetectorCatalog().Detect(DetectorInput{
		TeamID:      teamID,
		SiteID:      siteID,
		GeneratedAt: generatedAt,
		AIVisibility: &api.AIFetchOverview{
			TotalRequests: 82,
			UniquePaths:   7,
			TopPaths:      []api.MetricStat{{Name: "/pricing", Value: 51}},
		},
		Stats: &api.SiteStats{
			TotalPageviews: 1210,
			UniqueSessions: 640,
			AISourceVisits: 32,
			TopPages:       []api.MetricStat{{Name: "/pricing", Value: 420}},
		},
	})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	opportunity := findOpportunityByType(opportunities, "opportunities.types.ai_visibility")
	if opportunity == nil {
		t.Fatalf("expected AI visibility opportunity, got %#v", opportunities)
	}
	if opportunity.CopyParams["ai_referrals"] != 32 || opportunity.CopyParams["top_path_pageviews"] != 420 {
		t.Fatalf("expected cross-source params, got %#v", opportunity.CopyParams)
	}
	if !hasEvidenceID(opportunity.Evidence, "ai_referrals") || !hasEvidenceID(opportunity.Evidence, "ai_path_pageviews") {
		t.Fatalf("expected cross-source evidence, got %#v", opportunity.Evidence)
	}
	if !slices.Contains(opportunity.CitedEvidenceIDs, "ai_referrals") || !slices.Contains(opportunity.CitedEvidenceIDs, "ai_path_pageviews") {
		t.Fatalf("expected cross-source citations, got %#v", opportunity.CitedEvidenceIDs)
	}
	if opportunity.ScoreBreakdown.EvidenceFit < 98 || opportunity.Score <= 92 {
		t.Fatalf("expected corroborated AI visibility scoring, got score=%d breakdown=%#v", opportunity.Score, opportunity.ScoreBreakdown)
	}
}

func TestDefaultDetectorCatalogSuppressesTrafficOpportunityForUnspecifiedSource(t *testing.T) {
	opportunities, err := NewDefaultDetectorCatalog().Detect(DetectorInput{
		TeamID:      uuid.New(),
		SiteID:      uuid.New(),
		GeneratedAt: time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC),
		Stats: &api.SiteStats{
			TotalPageviews: 1800,
			UniqueSessions: 900,
			TopUTMSources:  []api.MetricStat{{Name: "(Unspecified)", Value: 1800}},
		},
	})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if findOpportunityByType(opportunities, "opportunities.types.traffic_quality") != nil {
		t.Fatalf("expected unspecified source to suppress traffic opportunity, got %#v", opportunities)
	}
}

func TestDefaultDetectorCatalogGeneratesConversionSignalOpportunityFromGenericEvents(t *testing.T) {
	siteID := uuid.New()
	teamID := uuid.New()
	generatedAt := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)

	input := DetectorInput{
		TeamID:      teamID,
		SiteID:      siteID,
		GeneratedAt: generatedAt,
		Stats: &api.SiteStats{
			TotalPageviews: 1638,
			UniqueSessions: 1049,
			TopUTMSources:  []api.MetricStat{{Name: "(Unspecified)", Value: 1638}},
		},
		EventNames: []string{"external_link_click", "download"},
		Ecommerce:  &api.EcommerceSummary{},
	}
	assertCatalogFixture(t, input, "setup", "opportunities.types.conversion_signal", "event_names", "event_names", DetectorCategorySetupQuality)

	opportunities, err := NewDefaultDetectorCatalog().Detect(input)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	signal := findOpportunityByType(opportunities, "opportunities.types.conversion_signal")
	if signal == nil {
		t.Fatalf("expected conversion signal opportunity, got %#v", opportunities)
	}
	if signal.CopyParams["event_count"] != 2 || signal.CopyParams["event_names"] != "external_link_click, download" {
		t.Fatalf("expected generic event params, got %#v", signal.CopyParams)
	}
	if signal.Score < 60 || signal.Confidence != "high" {
		t.Fatalf("expected useful conversion signal score, got score=%d confidence=%q", signal.Score, signal.Confidence)
	}
}

func TestDefaultDetectorCatalogSuppressesConversionSignalWhenConversionEventExists(t *testing.T) {
	opportunities, err := NewDefaultDetectorCatalog().Detect(DetectorInput{
		TeamID:      uuid.New(),
		SiteID:      uuid.New(),
		GeneratedAt: time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC),
		Stats: &api.SiteStats{
			TotalPageviews: 1200,
			UniqueSessions: 700,
		},
		EventNames: []string{"purchase", "download"},
		Ecommerce:  &api.EcommerceSummary{},
	})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if findOpportunityByType(opportunities, "opportunities.types.conversion_signal") != nil {
		t.Fatalf("expected known conversion event to suppress conversion signal opportunity, got %#v", opportunities)
	}
}

func TestDefaultDetectorCatalogGeneratesGoalSetupSuggestionFromConversionEventWithoutGoal(t *testing.T) {
	siteID := uuid.New()
	teamID := uuid.New()
	generatedAt := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)

	input := DetectorInput{
		TeamID:      teamID,
		SiteID:      siteID,
		GeneratedAt: generatedAt,
		SetupEvidence: &SetupEvidenceSnapshot{
			SiteID: siteID,
			Events: []SetupEventEvidence{
				{Name: "demo_request", Count: 18},
				{Name: "download", Count: 42},
			},
			EventNames: []string{"demo_request", "download"},
			TopPages:   []SetupTopPageEvidence{{Path: "/pricing", Pageviews: 640}},
			SetupState: SetupStateEvidence{HasEvents: true, HasConversionEvent: true, HasTraffic: true},
		},
	}
	assertCatalogFixture(t, input, "setup", "opportunities.types.setup_goal_suggestion", "event_name", "suggested_goal_event", DetectorCategorySetupQuality)

	opportunities, err := NewDefaultDetectorCatalog().Detect(input)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	suggestion := findOpportunityByType(opportunities, "opportunities.types.setup_goal_suggestion")
	if suggestion == nil {
		t.Fatalf("expected goal setup suggestion, got %#v", opportunities)
	}
	if suggestion.CopyParams["event_name"] != "demo_request" || suggestion.CopyParams["goal_value"] != "demo_request" {
		t.Fatalf("expected suggested goal params, got %#v", suggestion.CopyParams)
	}
	if suggestion.RouteParams["event_name"] != "demo_request" {
		t.Fatalf("expected event route params, got %#v", suggestion.RouteParams)
	}
	if suggestion.Score < 70 || suggestion.Confidence != "high" {
		t.Fatalf("expected high-confidence setup goal score, got score=%d confidence=%q", suggestion.Score, suggestion.Confidence)
	}
}

func TestDefaultDetectorCatalogSuppressesGoalSetupSuggestionWhenMatchingGoalExists(t *testing.T) {
	siteID := uuid.New()
	opportunities, err := NewDefaultDetectorCatalog().Detect(DetectorInput{
		TeamID:      uuid.New(),
		SiteID:      siteID,
		GeneratedAt: time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC),
		SetupEvidence: &SetupEvidenceSnapshot{
			SiteID: siteID,
			Goals: []SetupGoalEvidence{
				{Name: "Demo request", Type: "event", Value: "demo_request"},
			},
			Events: []SetupEventEvidence{{Name: "demo_request", Count: 18}},
		},
	})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if findOpportunityByType(opportunities, "opportunities.types.setup_goal_suggestion") != nil {
		t.Fatalf("expected matching goal to suppress setup suggestion, got %#v", opportunities)
	}
}

func TestDefaultDetectorCatalogSuppressesGoalSetupSuggestionForWeakEventSample(t *testing.T) {
	siteID := uuid.New()
	opportunities, err := NewDefaultDetectorCatalog().Detect(DetectorInput{
		TeamID:      uuid.New(),
		SiteID:      siteID,
		GeneratedAt: time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC),
		SetupEvidence: &SetupEvidenceSnapshot{
			SiteID: siteID,
			Events: []SetupEventEvidence{{Name: "demo_request", Count: 1}},
		},
	})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if findOpportunityByType(opportunities, "opportunities.types.setup_goal_suggestion") != nil {
		t.Fatalf("expected weak event sample to suppress setup suggestion, got %#v", opportunities)
	}
}

func TestDefaultDetectorCatalogGeneratesFunnelSetupSuggestionFromObservedPageAndConversionEvent(t *testing.T) {
	siteID := uuid.New()
	teamID := uuid.New()
	generatedAt := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)

	input := DetectorInput{
		TeamID:      teamID,
		SiteID:      siteID,
		GeneratedAt: generatedAt,
		SetupEvidence: &SetupEvidenceSnapshot{
			SiteID: siteID,
			Events: []SetupEventEvidence{
				{Name: "demo_request", Count: 18},
			},
			EventNames: []string{"demo_request"},
			TopPages:   []SetupTopPageEvidence{{Path: "/pricing", Pageviews: 640}},
			SetupState: SetupStateEvidence{HasEvents: true, HasConversionEvent: true, HasTraffic: true},
		},
	}
	opportunities, err := NewDefaultDetectorCatalog().Detect(input)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	suggestion := findOpportunityByType(opportunities, "opportunities.types.setup_funnel_suggestion")
	if suggestion == nil {
		t.Fatalf("expected funnel setup suggestion, got %#v", opportunities)
	}
	assertCatalogOpportunity(t, NewDefaultDetectorCatalog(), *suggestion, "setup", "opportunities.types.setup_funnel_suggestion", "start_path", "suggested_funnel_start", DetectorCategorySetupQuality)
	if suggestion.CopyParams["start_path"] != "/pricing" || suggestion.CopyParams["conversion_event"] != "demo_request" {
		t.Fatalf("expected suggested funnel params, got %#v", suggestion.CopyParams)
	}
	if suggestion.CopyParams["step_count"] != 2 || suggestion.RouteParams["start_path"] != "/pricing" {
		t.Fatalf("expected two-step funnel route params, copy=%#v route=%#v", suggestion.CopyParams, suggestion.RouteParams)
	}
	if suggestion.Score < 70 || suggestion.Confidence != "high" {
		t.Fatalf("expected high-confidence setup funnel score, got score=%d confidence=%q", suggestion.Score, suggestion.Confidence)
	}
}

func TestDefaultDetectorCatalogSuppressesFunnelSetupSuggestionWhenMatchingFunnelExists(t *testing.T) {
	siteID := uuid.New()
	opportunities, err := NewDefaultDetectorCatalog().Detect(DetectorInput{
		TeamID:      uuid.New(),
		SiteID:      siteID,
		GeneratedAt: time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC),
		SetupEvidence: &SetupEvidenceSnapshot{
			SiteID: siteID,
			Funnels: []SetupFunnelEvidence{{
				Name: "Demo funnel",
				Steps: []SetupFunnelStepEvidence{
					{Type: "path", Value: "/pricing"},
					{Type: "event", Value: "demo_request"},
				},
			}},
			Events:   []SetupEventEvidence{{Name: "demo_request", Count: 18}},
			TopPages: []SetupTopPageEvidence{{Path: "/pricing", Pageviews: 640}},
		},
	})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if findOpportunityByType(opportunities, "opportunities.types.setup_funnel_suggestion") != nil {
		t.Fatalf("expected matching funnel to suppress setup suggestion, got %#v", opportunities)
	}
}

func TestDefaultDetectorCatalogSuppressesFunnelSetupSuggestionWithoutObservedPageStep(t *testing.T) {
	siteID := uuid.New()
	opportunities, err := NewDefaultDetectorCatalog().Detect(DetectorInput{
		TeamID:      uuid.New(),
		SiteID:      siteID,
		GeneratedAt: time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC),
		SetupEvidence: &SetupEvidenceSnapshot{
			SiteID: siteID,
			Events: []SetupEventEvidence{{Name: "demo_request", Count: 18}},
		},
	})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if findOpportunityByType(opportunities, "opportunities.types.setup_funnel_suggestion") != nil {
		t.Fatalf("expected missing page evidence to suppress setup suggestion, got %#v", opportunities)
	}
}

func TestDefaultDetectorCatalogGeneratesSearchVisibilityOpportunityFromSearchConsole(t *testing.T) {
	siteID := uuid.New()
	teamID := uuid.New()
	generatedAt := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)

	input := DetectorInput{
		TeamID:      teamID,
		SiteID:      siteID,
		GeneratedAt: generatedAt,
		SearchConsole: &api.SearchConsoleOverview{
			Clicks:          54,
			Impressions:     4200,
			CTR:             0.0129,
			AveragePosition: 8.4,
		},
	}
	assertCatalogFixture(t, input, "search", "opportunities.types.search_visibility", "estimated_clicks", "search_ctr", DetectorCategorySearchVisibility)

	opportunities, err := NewDefaultDetectorCatalog().Detect(input)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	search := findOpportunityByType(opportunities, "opportunities.types.search_visibility")
	if search == nil {
		t.Fatalf("expected search visibility opportunity, got %#v", opportunities)
	}
	if search.ImpactValue != "+156" {
		t.Fatalf("expected deterministic click impact, got value=%q", search.ImpactValue)
	}
	if search.ScoreBreakdown.Total != 78 || search.ScoreBreakdown.EvidenceFit != 96 {
		t.Fatalf("expected search visibility score breakdown, got %#v", search.ScoreBreakdown)
	}
}

func assertCatalogFixture(
	t *testing.T,
	input DetectorInput,
	wantKind string,
	wantTypeKey string,
	wantParam string,
	wantEvidenceID string,
	wantCategory DetectorCategory,
) {
	t.Helper()
	catalog := NewDefaultDetectorCatalog()
	opportunities, err := catalog.Detect(input)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if len(opportunities) == 0 {
		t.Fatalf("expected at least one opportunity")
	}
	assertCatalogOpportunity(t, catalog, opportunities[0], wantKind, wantTypeKey, wantParam, wantEvidenceID, wantCategory)
}

func TestDetectorCatalogRejectsUndeclaredKeysAndParams(t *testing.T) {
	catalog := NewDetectorCatalog(fakeDetector{
		contract: DetectorContract{
			Category: DetectorCategoryTraffic,
			TypeKey:  "opportunities.types.fixture",
			MessageKeys: DetectorMessageKeys{
				Title:       "opportunities.fixture.title",
				Summary:     "opportunities.fixture.summary",
				Action:      "opportunities.fixture.action",
				Digest:      "opportunities.fixture.digest",
				ImpactLabel: "opportunities.fixture.impact",
				RouteLabel:  "opportunities.fixture.route",
			},
			AllowedParams: []string{"allowed"},
		},
		output: database.OpportunityInput{
			ID:               uuid.New(),
			TeamID:           uuid.New(),
			SiteID:           uuid.New(),
			Kind:             "traffic",
			TypeKey:          "opportunities.types.fixture",
			TitleKey:         "opportunities.fixture.title",
			SummaryKey:       "opportunities.fixture.summary",
			ActionKey:        "opportunities.fixture.action",
			DigestKey:        "opportunities.fixture.digest",
			CopyParams:       map[string]any{"allowed": "yes", "invented": "no"},
			ImpactValue:      "1",
			ImpactLabelKey:   "opportunities.fixture.impact",
			Confidence:       "medium",
			Status:           "new",
			RouteLabelKey:    "opportunities.fixture.route",
			RouteParams:      map[string]any{},
			DetectorVersion:  detectorVersion,
			Evidence:         []api.OpportunityEvidence{{ID: "evidence", LabelKey: "opportunities.fixture.evidence", Value: "1"}},
			CitedEvidenceIDs: []string{"evidence"},
			GeneratedAt:      time.Now().UTC(),
		},
	})

	_, err := catalog.Detect(DetectorInput{TeamID: uuid.New(), SiteID: uuid.New(), GeneratedAt: time.Now().UTC()})
	if err == nil {
		t.Fatalf("expected detector contract violation")
	}
	if !strings.Contains(err.Error(), "invented") {
		t.Fatalf("expected error to mention invented param, got %v", err)
	}
}

func TestDetectorCatalogRejectsFullTextMessageKeys(t *testing.T) {
	catalog := NewDetectorCatalog(fakeDetector{
		contract: DetectorContract{
			Category: DetectorCategoryTraffic,
			TypeKey:  "opportunities.types.fixture",
			MessageKeys: DetectorMessageKeys{
				Title:       "Fix checkout",
				Summary:     "opportunities.fixture.summary",
				Action:      "opportunities.fixture.action",
				Digest:      "opportunities.fixture.digest",
				ImpactLabel: "opportunities.fixture.impact",
				RouteLabel:  "opportunities.fixture.route",
			},
			AllowedParams: []string{"allowed"},
		},
		output: database.OpportunityInput{
			ID:               uuid.New(),
			TeamID:           uuid.New(),
			SiteID:           uuid.New(),
			Kind:             "traffic",
			TypeKey:          "opportunities.types.fixture",
			TitleKey:         "Fix checkout",
			SummaryKey:       "opportunities.fixture.summary",
			ActionKey:        "opportunities.fixture.action",
			DigestKey:        "opportunities.fixture.digest",
			CopyParams:       map[string]any{"allowed": "yes"},
			ImpactValue:      "1",
			ImpactLabelKey:   "opportunities.fixture.impact",
			Confidence:       "medium",
			Status:           "new",
			RouteLabelKey:    "opportunities.fixture.route",
			RouteParams:      map[string]any{},
			DetectorVersion:  detectorVersion,
			Evidence:         []api.OpportunityEvidence{{ID: "evidence", LabelKey: "opportunities.fixture.evidence", Value: "1"}},
			CitedEvidenceIDs: []string{"evidence"},
			GeneratedAt:      time.Now().UTC(),
		},
	})

	_, err := catalog.Detect(DetectorInput{TeamID: uuid.New(), SiteID: uuid.New(), GeneratedAt: time.Now().UTC()})
	if err == nil {
		t.Fatalf("expected detector contract violation")
	}
	if !strings.Contains(err.Error(), "translation key") {
		t.Fatalf("expected translation key error, got %v", err)
	}
}

func TestDetectorCatalogRejectsFullTextEvidenceLabels(t *testing.T) {
	output := validFixtureOpportunity()
	output.Evidence = []api.OpportunityEvidence{{ID: "evidence", LabelKey: "Current conversion rate", Value: "42%"}}
	catalog := NewDetectorCatalog(fakeDetector{
		contract: validFixtureContract(),
		output:   output,
	})

	_, err := catalog.Detect(DetectorInput{TeamID: uuid.New(), SiteID: uuid.New(), GeneratedAt: time.Now().UTC()})
	if err == nil {
		t.Fatalf("expected detector contract violation")
	}
	if !strings.Contains(err.Error(), "evidence label") || !strings.Contains(err.Error(), "translation key") {
		t.Fatalf("expected evidence translation key error, got %v", err)
	}
}

func TestSupportedDetectorCategoriesIncludeReusableOpportunityFamilies(t *testing.T) {
	categories := SupportedDetectorCategories()
	for _, want := range []DetectorCategory{
		DetectorCategoryConversion,
		DetectorCategoryTraffic,
		DetectorCategoryTrafficQuality,
		DetectorCategoryAIVisibility,
		DetectorCategorySearchVisibility,
		DetectorCategorySetupQuality,
	} {
		if !hasCategory(categories, want) {
			t.Fatalf("expected category %q in %#v", want, categories)
		}
	}
}

func TestOpportunityDefinitionBuildsContractAndBaseOpportunity(t *testing.T) {
	siteID := uuid.New()
	teamID := uuid.New()
	generatedAt := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	definition := OpportunityDefinition{
		Key:      "fixture-growth",
		Kind:     "traffic",
		Category: DetectorCategoryTraffic,
		TypeKey:  "opportunities.types.fixture_growth",
		MessageKeys: DetectorMessageKeys{
			Title:       "opportunities.catalog.fixture_growth.title",
			Summary:     "opportunities.catalog.fixture_growth.summary",
			Action:      "opportunities.catalog.fixture_growth.action",
			Digest:      "opportunities.catalog.fixture_growth.digest",
			ImpactLabel: "opportunities.impact.fixture_growth",
			RouteLabel:  "opportunities.routes.fixture_growth",
		},
		AllowedParams:       []string{"source", "path"},
		ActionTypes:         []string{"route_traffic"},
		IdentityEvidenceIDs: []string{"top_source"},
		RouteIcon:           "pi pi-compass",
	}

	contract := definition.Contract()
	if contract.TypeKey != definition.TypeKey || contract.Category != definition.Category {
		t.Fatalf("definition contract lost identity: %#v", contract)
	}
	if strings.Join(contract.AllowedParams, ",") != "source,path" {
		t.Fatalf("definition contract lost params: %#v", contract.AllowedParams)
	}
	if strings.Join(contract.ActionTypes, ",") != "route_traffic" {
		t.Fatalf("definition contract lost action types: %#v", contract.ActionTypes)
	}
	if strings.Join(contract.IdentityEvidenceIDs, ",") != "top_source" {
		t.Fatalf("definition contract lost identity evidence IDs: %#v", contract.IdentityEvidenceIDs)
	}

	base := definition.BaseOpportunity(DetectorInput{TeamID: teamID, SiteID: siteID}, generatedAt)
	got := opportunityDefinitionProjection{
		ID:              base.ID,
		TeamID:          base.TeamID,
		SiteID:          base.SiteID,
		Kind:            base.Kind,
		TypeKey:         base.TypeKey,
		TitleKey:        base.TitleKey,
		ImpactLabelKey:  base.ImpactLabelKey,
		RouteLabelKey:   base.RouteLabelKey,
		RouteIcon:       base.RouteIcon,
		DetectorVersion: base.DetectorVersion,
		Status:          base.Status,
		GeneratedAt:     base.GeneratedAt,
	}
	want := opportunityDefinitionProjection{
		ID:              stableOpportunityID(siteID, "fixture-growth"),
		TeamID:          teamID,
		SiteID:          siteID,
		Kind:            "traffic",
		TypeKey:         definition.TypeKey,
		TitleKey:        definition.MessageKeys.Title,
		ImpactLabelKey:  definition.MessageKeys.ImpactLabel,
		RouteLabelKey:   definition.MessageKeys.RouteLabel,
		RouteIcon:       "pi pi-compass",
		DetectorVersion: detectorVersion,
		Status:          "new",
		GeneratedAt:     generatedAt,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("definition base opportunity metadata mismatch:\ngot  %#v\nwant %#v", got, want)
	}
}

func TestOpportunityDefinitionBuildsOpportunityFromRecipe(t *testing.T) {
	siteID := uuid.New()
	teamID := uuid.New()
	generatedAt := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	definition := OpportunityDefinition{
		Key:      "fixture-route",
		Kind:     "traffic",
		Category: DetectorCategoryTraffic,
		TypeKey:  "opportunities.types.fixture_route",
		MessageKeys: DetectorMessageKeys{
			Title:       "opportunities.catalog.fixture_route.title",
			Summary:     "opportunities.catalog.fixture_route.summary",
			Action:      "opportunities.catalog.fixture_route.action",
			Digest:      "opportunities.catalog.fixture_route.digest",
			ImpactLabel: "opportunities.impact.fixture_route",
			RouteLabel:  "opportunities.routes.fixture_route",
		},
		AllowedParams: []string{"source", "sessions"},
		RouteIcon:     "pi pi-compass",
	}
	recipe := OpportunityRecipe{
		CopyParams:     map[string]any{"source": "newsletter", "sessions": 240},
		ImpactValue:    "240",
		Confidence:     "medium",
		Score:          72,
		ScoreBreakdown: api.OpportunityScoreBreakdown{Total: 72, EvidenceFit: 95},
		RouteParams:    map[string]any{"source": "newsletter"},
		Evidence: []api.OpportunityEvidence{
			{ID: "sessions", LabelKey: "opportunities.evidence.sessions", Value: "240"},
		},
		CitedEvidenceIDs: []string{"sessions"},
	}

	opportunity := definition.BuildOpportunity(DetectorInput{
		TeamID:      teamID,
		SiteID:      siteID,
		GeneratedAt: generatedAt,
	}, recipe)

	if opportunity.ID != stableOpportunityID(siteID, definition.Key) || opportunity.TeamID != teamID || opportunity.SiteID != siteID {
		t.Fatalf("definition recipe lost identity: %#v", opportunity)
	}
	if opportunity.TypeKey != definition.TypeKey || opportunity.Kind != definition.Kind || opportunity.RouteIcon != definition.RouteIcon {
		t.Fatalf("definition recipe lost metadata: %#v", opportunity)
	}
	if opportunity.CopyParams["sessions"] != 240 || opportunity.RouteParams["source"] != "newsletter" {
		t.Fatalf("definition recipe lost params: copy=%#v route=%#v", opportunity.CopyParams, opportunity.RouteParams)
	}
	if opportunity.Score != 72 || opportunity.ScoreBreakdown.Total != 72 || opportunity.ScoreBreakdown.EvidenceFit != 95 {
		t.Fatalf("definition recipe lost score: %#v", opportunity)
	}
	if !hasEvidenceID(opportunity.Evidence, "sessions") || strings.Join(opportunity.CitedEvidenceIDs, ",") != "sessions" {
		t.Fatalf("definition recipe lost evidence: evidence=%#v cited=%#v", opportunity.Evidence, opportunity.CitedEvidenceIDs)
	}
}

func TestOpportunityDefinitionRecipeReturnsDefensiveValues(t *testing.T) {
	definition := checkoutOpportunityDefinition
	recipe := OpportunityRecipe{
		CopyParams: map[string]any{
			"conversion_rate": "42%",
			"segments":        []any{"mobile", "paid"},
		},
		RouteParams: map[string]any{"path": "/checkout"},
		Evidence: []api.OpportunityEvidence{
			{ID: "conversion_rate", LabelKey: "opportunities.evidence.checkout_conversion_rate", Value: "42%"},
		},
		CitedEvidenceIDs: []string{"conversion_rate"},
	}

	opportunity := definition.BuildOpportunity(DetectorInput{TeamID: uuid.New(), SiteID: uuid.New()}, recipe)
	opportunity.CopyParams["conversion_rate"] = "mutated"
	opportunity.CopyParams["segments"].([]any)[0] = "changed"
	opportunity.RouteParams["path"] = "/changed"
	opportunity.Evidence[0].Value = "0%"
	opportunity.CitedEvidenceIDs[0] = "changed"

	if recipe.CopyParams["conversion_rate"] != "42%" || recipe.CopyParams["segments"].([]any)[0] != "mobile" {
		t.Fatalf("definition recipe leaked mutable copy params: %#v", recipe.CopyParams)
	}
	if recipe.RouteParams["path"] != "/checkout" {
		t.Fatalf("definition recipe leaked mutable route params: %#v", recipe.RouteParams)
	}
	if recipe.Evidence[0].Value != "42%" || recipe.CitedEvidenceIDs[0] != "conversion_rate" {
		t.Fatalf("definition recipe leaked mutable evidence: evidence=%#v cited=%#v", recipe.Evidence, recipe.CitedEvidenceIDs)
	}
}

func TestDetectorCatalogGeneratesFromDefinitionDetector(t *testing.T) {
	siteID := uuid.New()
	teamID := uuid.New()
	generatedAt := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	definition := OpportunityDefinition{
		Key:      "fixture-expansion",
		Kind:     "traffic",
		Category: DetectorCategoryTraffic,
		TypeKey:  "opportunities.types.fixture_expansion",
		MessageKeys: DetectorMessageKeys{
			Title:       "opportunities.catalog.fixture_expansion.title",
			Summary:     "opportunities.catalog.fixture_expansion.summary",
			Action:      "opportunities.catalog.fixture_expansion.action",
			Digest:      "opportunities.catalog.fixture_expansion.digest",
			ImpactLabel: "opportunities.impact.fixture_expansion",
			RouteLabel:  "opportunities.routes.fixture_expansion",
		},
		AllowedParams: []string{"source", "sessions"},
		RouteIcon:     "pi pi-compass",
		Detect: func(input DetectorInput, definition OpportunityDefinition) (*database.OpportunityInput, bool) {
			if input.Stats == nil || input.Stats.UniqueSessions < 100 {
				return nil, false
			}
			opportunity := definition.BuildOpportunity(input, OpportunityRecipe{
				CopyParams:       map[string]any{"source": "newsletter", "sessions": input.Stats.UniqueSessions},
				ImpactValue:      "240",
				Confidence:       "medium",
				Score:            72,
				RouteParams:      map[string]any{"source": "newsletter"},
				Evidence:         []api.OpportunityEvidence{{ID: "sessions", LabelKey: "opportunities.evidence.sessions", Value: "240"}},
				CitedEvidenceIDs: []string{"sessions"},
			})
			return &opportunity, true
		},
	}
	catalog := NewDetectorCatalogFromDefinitions(definition)

	opportunities, err := catalog.Detect(DetectorInput{
		TeamID:      teamID,
		SiteID:      siteID,
		GeneratedAt: generatedAt,
		Stats:       &api.SiteStats{UniqueSessions: 240},
	})
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if len(opportunities) != 1 {
		t.Fatalf("expected one definition-backed opportunity, got %d", len(opportunities))
	}
	got := opportunities[0]
	if got.ID != stableOpportunityID(siteID, definition.Key) || got.TypeKey != definition.TypeKey || got.RouteIcon != definition.RouteIcon {
		t.Fatalf("definition detector lost base opportunity metadata: %#v", got)
	}
	if got.CopyParams["sessions"] != 240 || got.RouteParams["source"] != "newsletter" {
		t.Fatalf("definition detector lost params: copy=%#v route=%#v", got.CopyParams, got.RouteParams)
	}
	if definitionForType, ok := catalog.DefinitionFor(definition.TypeKey); !ok || definitionForType.TypeKey != definition.TypeKey {
		t.Fatalf("expected catalog to expose the backing definition, got %#v ok=%v", definitionForType, ok)
	}
}

func TestDetectorCatalogDefinitionForReturnsDefensiveDefinition(t *testing.T) {
	definition := aiVisibilityOpportunityDefinition
	catalog := NewDetectorCatalogFromDefinitions(definition)

	got, ok := catalog.DefinitionFor(definition.TypeKey)
	if !ok {
		t.Fatalf("expected catalog definition for %q", definition.TypeKey)
	}
	got.AllowedParams[0] = "mutated"
	got.RequiredSignals[0] = "mutated"
	got.OptionalSignals[0] = "mutated"

	fresh, ok := catalog.DefinitionFor(definition.TypeKey)
	if !ok {
		t.Fatalf("expected fresh catalog definition for %q", definition.TypeKey)
	}
	if fresh.AllowedParams[0] == "mutated" {
		t.Fatalf("catalog definition leaked mutable allowed params")
	}
	if fresh.RequiredSignals[0] == "mutated" {
		t.Fatalf("catalog definition leaked mutable required signals")
	}
	if fresh.OptionalSignals[0] == "mutated" {
		t.Fatalf("catalog definition leaked mutable optional signals")
	}
}

func TestOpportunityDefinitionBuildsAICopyContract(t *testing.T) {
	from := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 9, 0, 0, 0, 0, time.UTC)
	definition := checkoutOpportunityDefinition
	opportunity := definition.BaseOpportunity(DetectorInput{TeamID: uuid.New(), SiteID: uuid.New()}, to)
	opportunity.CopyParams = map[string]any{"conversion_rate": "42%"}
	opportunity.RouteParams = map[string]any{"path": "/checkout"}
	opportunity.ImpactValue = "EUR 900"
	opportunity.Confidence = "medium"
	opportunity.Evidence = []api.OpportunityEvidence{
		{ID: "conversion_rate", LabelKey: "opportunities.evidence.checkout_conversion_rate", Value: "42%"},
	}

	contract := definition.AICopyContract(OpportunityCopyContext{
		SiteDomain: "example.test",
		From:       from,
		To:         to,
	}, opportunity)

	want := hitai.OpportunityDetectorInput{
		SiteDomain: "example.test",
		From:       from,
		To:         to,
		TypeKey:    definition.TypeKey,
		Category:   string(definition.Category),
		MessageKeys: hitai.OpportunityMessageKeys{
			Title:   definition.MessageKeys.Title,
			Summary: definition.MessageKeys.Summary,
			Action:  definition.MessageKeys.Action,
			Digest:  definition.MessageKeys.Digest,
		},
		AllowedParams:      definition.AllowedParams,
		AllowedActionTypes: definition.ActionTypes,
		CopyParams:         opportunity.CopyParams,
		Evidence:           []hitai.Evidence{{ID: "conversion_rate", Label: "opportunities.evidence.checkout_conversion_rate", Value: "42%"}},
		ImpactValue:        "EUR 900",
		Confidence:         "medium",
		Kind:               definition.Kind,
		RouteParams:        opportunity.RouteParams,
	}
	if !reflect.DeepEqual(contract, want) {
		t.Fatalf("unexpected AI copy contract:\ngot  %#v\nwant %#v", contract, want)
	}
}

func TestOpportunityDefinitionAICopyContractReturnsDefensiveMaps(t *testing.T) {
	definition := checkoutOpportunityDefinition
	opportunity := definition.BaseOpportunity(DetectorInput{TeamID: uuid.New(), SiteID: uuid.New()}, time.Now().UTC())
	opportunity.CopyParams = map[string]any{
		"conversion_rate": "42%",
		"breakdown":       map[string]any{"mobile": "38%"},
	}
	opportunity.RouteParams = map[string]any{
		"path":     "/checkout",
		"segments": []any{"mobile", "paid"},
	}

	contract := definition.AICopyContract(OpportunityCopyContext{}, opportunity)
	contract.CopyParams["conversion_rate"] = "mutated"
	contract.RouteParams["path"] = "/changed"
	contract.CopyParams["breakdown"].(map[string]any)["mobile"] = "mutated"
	contract.RouteParams["segments"].([]any)[0] = "changed"

	if opportunity.CopyParams["conversion_rate"] != "42%" || opportunity.RouteParams["path"] != "/checkout" {
		t.Fatalf("AI copy contract leaked mutable opportunity maps: copy=%#v route=%#v", opportunity.CopyParams, opportunity.RouteParams)
	}
	if opportunity.CopyParams["breakdown"].(map[string]any)["mobile"] != "38%" || opportunity.RouteParams["segments"].([]any)[0] != "mobile" {
		t.Fatalf("AI copy contract leaked nested mutable opportunity values: copy=%#v route=%#v", opportunity.CopyParams, opportunity.RouteParams)
	}
}

func TestDefaultOpportunityDefinitionsBackDefaultCatalog(t *testing.T) {
	definitions := DefaultOpportunityDefinitions()
	catalog := NewDefaultDetectorCatalog()
	contracts := catalog.Contracts()
	if len(definitions) != len(contracts) {
		t.Fatalf("expected one definition per default detector, got definitions=%d contracts=%d", len(definitions), len(contracts))
	}

	for _, definition := range definitions {
		contract, ok := catalog.ContractFor(definition.TypeKey)
		if !ok {
			t.Fatalf("default catalog missing contract for definition %q", definition.TypeKey)
		}
		if !reflect.DeepEqual(contract, definition.Contract()) {
			t.Fatalf("definition and catalog contract drifted for %q: definition=%#v contract=%#v", definition.TypeKey, definition, contract)
		}
	}
}

func TestDetectorCatalogRequiredSignalsDeduplicatesActiveContracts(t *testing.T) {
	catalog := NewDetectorCatalog(
		fakeDetector{contract: DetectorContract{
			Category:        DetectorCategoryTraffic,
			TypeKey:         "opportunities.types.one",
			MessageKeys:     validFixtureContract().MessageKeys,
			RequiredSignals: []OpportunitySignal{OpportunitySignalSiteStats, OpportunitySignalEcommerce},
		}},
		fakeDetector{contract: DetectorContract{
			Category:        DetectorCategoryConversion,
			TypeKey:         "opportunities.types.two",
			MessageKeys:     validFixtureContract().MessageKeys,
			RequiredSignals: []OpportunitySignal{OpportunitySignalEcommerce, OpportunitySignalAIVisibility},
		}},
	)

	got := catalog.RequiredSignals()
	want := []OpportunitySignal{OpportunitySignalSiteStats, OpportunitySignalEcommerce, OpportunitySignalAIVisibility}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected required signals:\ngot  %#v\nwant %#v", got, want)
	}
}

func TestDetectorCatalogDetectionSignalsIncludeOptionalSupportWithoutChangingRequiredSignals(t *testing.T) {
	catalog := NewDetectorCatalog(
		fakeDetector{contract: DetectorContract{
			Category:        DetectorCategoryTraffic,
			TypeKey:         "opportunities.types.one",
			MessageKeys:     validFixtureContract().MessageKeys,
			RequiredSignals: []OpportunitySignal{OpportunitySignalAIVisibility},
			OptionalSignals: []OpportunitySignal{OpportunitySignalSiteStats, OpportunitySignalEcommerce},
		}},
		fakeDetector{contract: DetectorContract{
			Category:        DetectorCategoryConversion,
			TypeKey:         "opportunities.types.two",
			MessageKeys:     validFixtureContract().MessageKeys,
			RequiredSignals: []OpportunitySignal{OpportunitySignalEcommerce},
			OptionalSignals: []OpportunitySignal{OpportunitySignalAIVisibility},
		}},
	)

	if got, want := catalog.RequiredSignals(), []OpportunitySignal{OpportunitySignalAIVisibility, OpportunitySignalEcommerce}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected required signals:\ngot  %#v\nwant %#v", got, want)
	}
	if got, want := catalog.DetectionSignals(), []OpportunitySignal{OpportunitySignalAIVisibility, OpportunitySignalSiteStats, OpportunitySignalEcommerce}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected detection signals:\ngot  %#v\nwant %#v", got, want)
	}
}

type opportunityDefinitionProjection struct {
	ID              uuid.UUID
	TeamID          uuid.UUID
	SiteID          uuid.UUID
	Kind            string
	TypeKey         string
	TitleKey        string
	ImpactLabelKey  string
	RouteLabelKey   string
	RouteIcon       string
	DetectorVersion string
	Status          string
	GeneratedAt     time.Time
}

func TestDefaultOpportunityDefinitionsReturnsDefensiveCopies(t *testing.T) {
	definitions := DefaultOpportunityDefinitions()
	if len(definitions) == 0 || len(definitions[0].AllowedParams) == 0 {
		t.Fatalf("expected default definitions with allowed params")
	}

	definitions[0].AllowedParams[0] = "mutated"
	definitions[0].IdentityEvidenceIDs[0] = "mutated"
	definitions[0].RequiredSignals[0] = "mutated"
	definitions[1].OptionalSignals[0] = "mutated"

	fresh := DefaultOpportunityDefinitions()
	if fresh[0].AllowedParams[0] == "mutated" {
		t.Fatalf("default opportunity definitions leaked mutable allowed params")
	}
	if fresh[0].IdentityEvidenceIDs[0] == "mutated" {
		t.Fatalf("default opportunity definitions leaked mutable identity evidence IDs")
	}
	if fresh[0].RequiredSignals[0] == "mutated" {
		t.Fatalf("default opportunity definitions leaked mutable required signals")
	}
	if fresh[1].OptionalSignals[0] == "mutated" {
		t.Fatalf("default opportunity definitions leaked mutable optional signals")
	}
}

type fakeDetector struct {
	contract DetectorContract
	output   database.OpportunityInput
}

func (d fakeDetector) Contract() DetectorContract {
	return d.contract
}

func (d fakeDetector) Detect(DetectorInput) (*database.OpportunityInput, bool) {
	return &d.output, true
}

func validFixtureContract() DetectorContract {
	return DetectorContract{
		Category: DetectorCategoryTraffic,
		TypeKey:  "opportunities.types.fixture",
		MessageKeys: DetectorMessageKeys{
			Title:       "opportunities.fixture.title",
			Summary:     "opportunities.fixture.summary",
			Action:      "opportunities.fixture.action",
			Digest:      "opportunities.fixture.digest",
			ImpactLabel: "opportunities.fixture.impact",
			RouteLabel:  "opportunities.fixture.route",
		},
		AllowedParams: []string{"allowed"},
	}
}

func validFixtureOpportunity() database.OpportunityInput {
	return database.OpportunityInput{
		ID:               uuid.New(),
		TeamID:           uuid.New(),
		SiteID:           uuid.New(),
		Kind:             "traffic",
		TypeKey:          "opportunities.types.fixture",
		TitleKey:         "opportunities.fixture.title",
		SummaryKey:       "opportunities.fixture.summary",
		ActionKey:        "opportunities.fixture.action",
		DigestKey:        "opportunities.fixture.digest",
		CopyParams:       map[string]any{"allowed": "yes"},
		ImpactValue:      "1",
		ImpactLabelKey:   "opportunities.fixture.impact",
		Confidence:       "medium",
		Status:           "new",
		RouteLabelKey:    "opportunities.fixture.route",
		RouteParams:      map[string]any{},
		DetectorVersion:  detectorVersion,
		Evidence:         []api.OpportunityEvidence{{ID: "evidence", LabelKey: "opportunities.fixture.evidence", Value: "1"}},
		CitedEvidenceIDs: []string{"evidence"},
		GeneratedAt:      time.Now().UTC(),
	}
}

func assertMessageKey(t *testing.T, value string) {
	t.Helper()
	if value == "" || strings.Contains(value, " ") || !strings.Contains(value, ".") {
		t.Fatalf("expected translation key, got %q", value)
	}
}

func assertCatalogOpportunity(
	t *testing.T,
	catalog DetectorCatalog,
	opportunity database.OpportunityInput,
	wantKind string,
	wantTypeKey string,
	wantParam string,
	wantEvidenceID string,
	wantCategory DetectorCategory,
) {
	t.Helper()
	if opportunity.Kind != wantKind || opportunity.TypeKey != wantTypeKey {
		t.Fatalf("unexpected opportunity kind/type: %#v", opportunity)
	}
	assertOpportunityKeys(t, opportunity)
	if _, ok := opportunity.CopyParams[wantParam]; !ok {
		t.Fatalf("expected copy param %q in %#v", wantParam, opportunity.CopyParams)
	}
	if !hasEvidenceID(opportunity.Evidence, wantEvidenceID) {
		t.Fatalf("expected evidence %q in %#v", wantEvidenceID, opportunity.Evidence)
	}
	if len(opportunity.CitedEvidenceIDs) == 0 {
		t.Fatalf("expected cited evidence ids")
	}
	contract, ok := catalog.ContractFor(opportunity.TypeKey)
	if !ok {
		t.Fatalf("missing contract for %q", opportunity.TypeKey)
	}
	if contract.Category != wantCategory {
		t.Fatalf("expected category %q, got %q", wantCategory, contract.Category)
	}
}

func assertOpportunityKeys(t *testing.T, opportunity database.OpportunityInput) {
	t.Helper()
	assertMessageKey(t, opportunity.TitleKey)
	assertMessageKey(t, opportunity.SummaryKey)
	assertMessageKey(t, opportunity.ActionKey)
	assertMessageKey(t, opportunity.DigestKey)
	assertMessageKey(t, opportunity.ImpactLabelKey)
	assertMessageKey(t, opportunity.RouteLabelKey)
}

func hasEvidenceID(evidence []api.OpportunityEvidence, id string) bool {
	for _, item := range evidence {
		if item.ID == id && assertEvidenceKey(item.LabelKey) {
			return true
		}
	}
	return false
}

func assertEvidenceKey(value string) bool {
	return value != "" && !strings.Contains(value, " ") && strings.Contains(value, ".")
}

func findOpportunityByType(opportunities []database.OpportunityInput, typeKey string) *database.OpportunityInput {
	for i := range opportunities {
		if opportunities[i].TypeKey == typeKey {
			return &opportunities[i]
		}
	}
	return nil
}

func hasCategory(categories []DetectorCategory, want DetectorCategory) bool {
	return slices.Contains(categories, want)
}
