package opportunities

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

func TestSelectDigestPreviewRanksTopActiveOpportunities(t *testing.T) {
	siteID := uuid.New()
	opportunities := []api.Opportunity{
		digestFixtureOpportunity(siteID, "done-high", DetectorCategoryConversion, "opportunities.types.checkout_conversion", "done", 99, time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)),
		digestFixtureOpportunity(siteID, "dismissed-high", DetectorCategoryTraffic, "opportunities.types.traffic_quality", "dismissed", 98, time.Date(2026, 5, 1, 11, 0, 0, 0, time.UTC)),
		digestFixtureOpportunity(siteID, "third", DetectorCategoryTrafficQuality, "opportunities.types.traffic_quality", "saved", 81, time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)),
		digestFixtureOpportunity(siteID, "first", DetectorCategoryConversion, "opportunities.types.checkout_conversion", "new", 94, time.Date(2026, 5, 1, 13, 0, 0, 0, time.UTC)),
		digestFixtureOpportunity(siteID, "fourth", DetectorCategoryAIVisibility, "opportunities.types.ai_visibility", "new", 65, time.Date(2026, 5, 1, 14, 0, 0, 0, time.UTC)),
		digestFixtureOpportunity(siteID, "second", DetectorCategorySearchVisibility, "opportunities.types.search_visibility", "new", 88, time.Date(2026, 5, 1, 15, 0, 0, 0, time.UTC)),
	}

	preview := SelectDigestPreview(DigestSelectionInput{
		Frequency:     api.ReportFrequencyWeekly,
		Opportunities: opportunities,
	})

	if !preview.ShouldSend || preview.Reason != DigestPreviewReasonReady {
		t.Fatalf("expected sendable ready preview, got %#v", preview)
	}
	got := digestItemNames(preview.Items)
	want := []string{"first", "second", "third"}
	if len(got) != len(want) {
		t.Fatalf("expected %d digest items, got %#v", len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected item order %#v, got %#v", want, got)
		}
	}
	for _, item := range preview.Items {
		if item.Status == "done" || item.Status == "dismissed" {
			t.Fatalf("digest preview included non-actionable item: %#v", item)
		}
	}
}

func TestSelectDigestPreviewIncludesAtMostOneSetupWarningWhenMixed(t *testing.T) {
	siteID := uuid.New()
	opportunities := []api.Opportunity{
		digestFixtureOpportunity(siteID, "setup-one", DetectorCategorySetupQuality, "opportunities.types.setup_goal_suggestion", "new", 96, time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)),
		digestFixtureOpportunity(siteID, "setup-two", DetectorCategorySetupQuality, "opportunities.types.setup_funnel_suggestion", "new", 95, time.Date(2026, 5, 1, 11, 0, 0, 0, time.UTC)),
		digestFixtureOpportunity(siteID, "traffic-one", DetectorCategoryConversion, "opportunities.types.checkout_conversion", "new", 84, time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)),
		digestFixtureOpportunity(siteID, "traffic-one", DetectorCategoryTrafficQuality, "opportunities.types.traffic_quality", "new", 82, time.Date(2026, 5, 1, 13, 0, 0, 0, time.UTC)),
	}

	preview := SelectDigestPreview(DigestSelectionInput{
		Frequency:     api.ReportFrequencyDaily,
		Opportunities: opportunities,
	})

	got := digestItemNames(preview.Items)
	want := []string{"setup-one", "traffic-one", "traffic-one"}
	if len(got) != len(want) {
		t.Fatalf("expected mixed digest items %#v, got %#v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected mixed digest items %#v, got %#v", want, got)
		}
	}
	setupCount := 0
	for _, item := range preview.Items {
		if item.Category == string(DetectorCategorySetupQuality) {
			setupCount++
		}
	}
	if setupCount != 1 {
		t.Fatalf("expected one setup warning in mixed digest, got %d items in %#v", setupCount, preview.Items)
	}
}

func TestSelectDigestPreviewAllowsSetupOnlyDigest(t *testing.T) {
	siteID := uuid.New()
	opportunities := []api.Opportunity{
		digestFixtureOpportunity(siteID, "setup-one", DetectorCategorySetupQuality, "opportunities.types.setup_goal_suggestion", "new", 96, time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)),
		digestFixtureOpportunity(siteID, "setup-two", DetectorCategorySetupQuality, "opportunities.types.setup_funnel_suggestion", "new", 95, time.Date(2026, 5, 1, 11, 0, 0, 0, time.UTC)),
		digestFixtureOpportunity(siteID, "setup-three", DetectorCategorySetupQuality, "opportunities.types.conversion_signal", "new", 94, time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)),
	}

	preview := SelectDigestPreview(DigestSelectionInput{
		Frequency:     api.ReportFrequencyWeekly,
		Opportunities: opportunities,
	})

	if got := digestItemNames(preview.Items); len(got) != 3 {
		t.Fatalf("expected setup-only digest to keep setup items, got %#v", got)
	}
}

func TestSelectDigestPreviewReturnsNoSendForEmptyLowQualityOrUnsupportedFrequency(t *testing.T) {
	siteID := uuid.New()

	empty := SelectDigestPreview(DigestSelectionInput{Frequency: api.ReportFrequencyWeekly})
	if empty.ShouldSend || empty.Reason != DigestPreviewReasonNoOpportunities {
		t.Fatalf("expected no-send empty preview, got %#v", empty)
	}

	lowQuality := SelectDigestPreview(DigestSelectionInput{
		Frequency: api.ReportFrequencyWeekly,
		Opportunities: []api.Opportunity{
			digestFixtureOpportunity(siteID, "too-weak", DetectorCategoryTraffic, "opportunities.types.traffic_quality", "new", 39, time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)),
		},
	})
	if lowQuality.ShouldSend || lowQuality.Reason != DigestPreviewReasonNoOpportunities {
		t.Fatalf("expected no-send low-quality preview, got %#v", lowQuality)
	}

	unsupported := SelectDigestPreview(DigestSelectionInput{
		Frequency:     api.ReportFrequencyMonthly,
		Opportunities: []api.Opportunity{digestFixtureOpportunity(siteID, "strong", DetectorCategoryTraffic, "opportunities.types.traffic_quality", "new", 90, time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC))},
	})
	if unsupported.ShouldSend || unsupported.Reason != DigestPreviewReasonUnsupportedFrequency {
		t.Fatalf("expected unsupported monthly preview, got %#v", unsupported)
	}
}

func TestSelectDigestPreviewForSiteLoadsSavedOpportunities(t *testing.T) {
	siteID := uuid.New()
	lister := &fakeOpportunityLister{
		opportunities: []api.Opportunity{
			digestFixtureOpportunity(siteID, "saved", DetectorCategoryTraffic, "opportunities.types.traffic_quality", "new", 90, time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)),
		},
	}

	preview, err := SelectDigestPreviewForSite(context.Background(), lister, DigestPreviewForSiteInput{
		SiteID:    siteID,
		Frequency: api.ReportFrequencyWeekly,
	})
	if err != nil {
		t.Fatalf("SelectDigestPreviewForSite: %v", err)
	}
	if lister.siteID != siteID {
		t.Fatalf("expected lister to load site %s, got %s", siteID, lister.siteID)
	}
	if !preview.ShouldSend || len(preview.Items) != 1 {
		t.Fatalf("expected sendable saved-opportunity preview, got %#v", preview)
	}

	lister.err = errors.New("database unavailable")
	_, err = SelectDigestPreviewForSite(context.Background(), lister, DigestPreviewForSiteInput{
		SiteID:    siteID,
		Frequency: api.ReportFrequencyWeekly,
	})
	if err == nil {
		t.Fatal("expected lister error")
	}
}

func digestFixtureOpportunity(siteID uuid.UUID, name string, category DetectorCategory, typeKey, status string, score int, updatedAt time.Time) api.Opportunity {
	return api.Opportunity{
		ID:          uuid.New(),
		SiteID:      siteID,
		Kind:        "fixture",
		TypeKey:     typeKey,
		TitleKey:    "opportunities.catalog." + name + ".title",
		SummaryKey:  "opportunities.catalog." + name + ".summary",
		ActionKey:   "opportunities.catalog." + name + ".action",
		DigestKey:   "opportunities.catalog." + name + ".digest",
		CopyParams:  map[string]any{"name": name},
		ImpactValue: "high",
		Confidence:  "high",
		Score:       score,
		ScoreBreakdown: api.OpportunityScoreBreakdown{
			Impact:        score,
			Urgency:       score - 10,
			Actionability: score - 5,
			EvidenceFit:   score,
			Total:         score,
		},
		Status:           status,
		RouteLabelKey:    "opportunities.routes.fixture",
		RouteParams:      map[string]any{"name": name},
		RouteIcon:        "pi pi-bolt",
		Evidence:         []api.OpportunityEvidence{{ID: "evidence_" + name, LabelKey: "opportunities.evidence.fixture", Value: name}},
		CitedEvidenceIDs: []string{"evidence_" + name},
		GeneratedAt:      updatedAt.Add(-time.Hour),
		UpdatedAt:        updatedAt,
	}
}

type fakeOpportunityLister struct {
	siteID        uuid.UUID
	opportunities []api.Opportunity
	err           error
}

func (f *fakeOpportunityLister) ListOpportunities(_ context.Context, siteID uuid.UUID) ([]api.Opportunity, error) {
	f.siteID = siteID
	if f.err != nil {
		return nil, f.err
	}
	return append([]api.Opportunity(nil), f.opportunities...), nil
}

func digestItemNames(items []DigestItem) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		if name, _ := item.CopyParams["name"].(string); name != "" {
			names = append(names, name)
		}
	}
	return names
}
