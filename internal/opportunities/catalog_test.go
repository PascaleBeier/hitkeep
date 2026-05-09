package opportunities

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	hitai "hitkeep/internal/ai"
	"hitkeep/internal/api"
	"hitkeep/internal/database"
)

func TestDefaultDetectorCatalogGeneratesSetupOpportunityFromNoSignal(t *testing.T) {
	siteID := uuid.New()
	teamID := uuid.New()
	generatedAt := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)

	assertCatalogFixture(t, DetectorInput{
		TeamID:      teamID,
		SiteID:      siteID,
		GeneratedAt: generatedAt,
	}, "setup", "opportunities.types.tracking_setup", "events", "pageviews", DetectorCategorySetupQuality)
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
	}, "revenue", "opportunities.types.traffic_quality", "source", "top_source", DetectorCategoryTrafficQuality)
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
			Category: DetectorCategoryRevenue,
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
			Kind:             "revenue",
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
			Category: DetectorCategoryRevenue,
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
			Kind:             "revenue",
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
		DetectorCategoryRevenue,
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
		Kind:     "revenue",
		Category: DetectorCategoryRevenue,
		TypeKey:  "opportunities.types.fixture_growth",
		MessageKeys: DetectorMessageKeys{
			Title:       "opportunities.catalog.fixture_growth.title",
			Summary:     "opportunities.catalog.fixture_growth.summary",
			Action:      "opportunities.catalog.fixture_growth.action",
			Digest:      "opportunities.catalog.fixture_growth.digest",
			ImpactLabel: "opportunities.impact.fixture_growth",
			RouteLabel:  "opportunities.routes.fixture_growth",
		},
		AllowedParams: []string{"source", "path"},
		RouteIcon:     "pi pi-compass",
	}

	contract := definition.Contract()
	if contract.TypeKey != definition.TypeKey || contract.Category != definition.Category {
		t.Fatalf("definition contract lost identity: %#v", contract)
	}
	if strings.Join(contract.AllowedParams, ",") != "source,path" {
		t.Fatalf("definition contract lost params: %#v", contract.AllowedParams)
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
		Kind:            "revenue",
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
		AllowedParams: definition.AllowedParams,
		CopyParams:    opportunity.CopyParams,
		Evidence:      []hitai.Evidence{{ID: "conversion_rate", Label: "opportunities.evidence.checkout_conversion_rate", Value: "42%"}},
		ImpactValue:   "EUR 900",
		Confidence:    "medium",
		Kind:          definition.Kind,
		RouteParams:   opportunity.RouteParams,
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

	fresh := DefaultOpportunityDefinitions()
	if fresh[0].AllowedParams[0] == "mutated" {
		t.Fatalf("default opportunity definitions leaked mutable allowed params")
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
		Category: DetectorCategoryRevenue,
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
		Kind:             "revenue",
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
	for _, category := range categories {
		if category == want {
			return true
		}
	}
	return false
}
