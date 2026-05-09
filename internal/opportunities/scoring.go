package opportunities

import "math"

const minCheckoutScoringSample = 30

type opportunityScoreBreakdown struct {
	Sample        int
	Impact        int
	Confidence    string
	Urgency       int
	Effort        int
	Actionability int
	EvidenceFit   int
	Freshness     int
	Total         int
}

type checkoutScoringInput struct {
	CheckoutStarts         int
	Orders                 int
	CheckoutConversionRate float64
	AverageOrderValue      float64
}

func scoreCheckoutOpportunity(input checkoutScoringInput) (opportunityScoreBreakdown, bool) {
	if input.CheckoutStarts < minCheckoutScoringSample {
		return opportunityScoreBreakdown{}, false
	}
	leakedOrders := math.Max(0, float64(input.CheckoutStarts-input.Orders))
	if leakedOrders < 1 {
		return opportunityScoreBreakdown{}, false
	}
	sample := clampScore((input.CheckoutStarts * 100) / 120)
	impact := clampScore(int(math.Min(leakedOrders*math.Max(input.AverageOrderValue, 80)/100, 100)))
	urgency := clampScore(int(math.Max(0, 55-input.CheckoutConversionRate) * 2))
	total := clampScore((sample * 25 / 100) + (impact * 35 / 100) + (urgency * 40 / 100))
	return opportunityScoreBreakdown{
		Sample:        sample,
		Impact:        impact,
		Confidence:    confidence(sample >= 80),
		Urgency:       urgency,
		Effort:        70,
		Actionability: 85,
		EvidenceFit:   99,
		Freshness:     50,
		Total:         total,
	}, true
}
