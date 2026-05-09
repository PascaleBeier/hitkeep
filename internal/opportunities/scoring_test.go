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
