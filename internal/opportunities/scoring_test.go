package opportunities

import "testing"

func TestScoreCheckoutOpportunitySuppressesTinySamples(t *testing.T) {
	score, ok := scoreCheckoutOpportunity(checkoutScoringInput{
		CheckoutStarts:         12,
		Orders:                 2,
		CheckoutConversionRate: 16.7,
		AverageOrderValue:      95,
	})

	if ok {
		t.Fatalf("expected tiny checkout sample to be suppressed, got %+v", score)
	}
}

func TestScoreCheckoutOpportunityProducesDeterministicBreakdown(t *testing.T) {
	score, ok := scoreCheckoutOpportunity(checkoutScoringInput{
		CheckoutStarts:         120,
		Orders:                 28,
		CheckoutConversionRate: 23.3,
		AverageOrderValue:      95,
	})
	if !ok {
		t.Fatal("expected checkout opportunity to be scored")
	}

	if score.Sample != 99 || score.Impact != 87 || score.Urgency != 63 || score.Confidence != "high" || score.EvidenceFit != 99 || score.Total != 79 {
		t.Fatalf("unexpected score breakdown: %+v", score)
	}
}

func TestScoreSearchVisibilityOpportunitySuppressesTinySamples(t *testing.T) {
	score, estimatedClicks, ok := scoreSearchVisibilityOpportunity(searchVisibilityScoringInput{
		Impressions:     420,
		Clicks:          6,
		CTR:             0.014,
		AveragePosition: 9.4,
	})

	if ok {
		t.Fatalf("expected tiny search sample to be suppressed, got score=%+v estimatedClicks=%d", score, estimatedClicks)
	}
}

func TestScoreSearchVisibilityOpportunityAllowsZeroCTRWithStrongImpressions(t *testing.T) {
	score, estimatedClicks, ok := scoreSearchVisibilityOpportunity(searchVisibilityScoringInput{
		Impressions:     4200,
		Clicks:          0,
		CTR:             0,
		AveragePosition: 8.4,
	})
	if !ok {
		t.Fatal("expected zero-CTR search opportunity to be scored when impressions are strong")
	}
	if estimatedClicks != 210 {
		t.Fatalf("expected 210 estimated clicks, got %d", estimatedClicks)
	}
	if score.Total < 85 || score.Confidence != "high" {
		t.Fatalf("expected strong zero-CTR opportunity, got %+v", score)
	}
}

func TestScoreSearchVisibilityOpportunityProducesDeterministicBreakdown(t *testing.T) {
	score, estimatedClicks, ok := scoreSearchVisibilityOpportunity(searchVisibilityScoringInput{
		Impressions:     4200,
		Clicks:          54,
		CTR:             0.0129,
		AveragePosition: 8.4,
	})
	if !ok {
		t.Fatal("expected low-CTR search opportunity to be scored")
	}

	if estimatedClicks != 156 {
		t.Fatalf("expected 156 estimated clicks, got %d", estimatedClicks)
	}
	if score.Sample != 84 || score.Impact != 78 || score.Urgency != 74 || score.Confidence != "high" || score.EvidenceFit != 96 || score.Total != 78 {
		t.Fatalf("unexpected score breakdown: %+v", score)
	}
}
