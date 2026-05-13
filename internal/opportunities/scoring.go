package opportunities

import "math"

const minCheckoutScoringSample = 30
const minSearchVisibilityImpressions = 1000
const searchVisibilityTargetCTR = 0.05
const minTrafficSourceHits = 50
const minTrafficSourceShare = 0.10
const minSetupGoalEventCount = 3
const minSetupFunnelStartPageviews = 30

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

type searchVisibilityScoringInput struct {
	Impressions     int
	Clicks          int
	CTR             float64
	AveragePosition float64
}

type aiVisibilityScoringInput struct {
	Requests         int64
	UniquePaths      int64
	AIReferrals      int
	TopPathPageviews int
}

func scoreAIVisibilityOpportunity(input aiVisibilityScoringInput) opportunityScoreBreakdown {
	sample := clampScore(int(minInt64(input.Requests, 100)))
	impact := clampScore(int(input.UniquePaths*8) + input.TopPathPageviews/20 + input.AIReferrals)
	evidenceFit := 92
	actionability := 78
	if input.TopPathPageviews > 0 {
		evidenceFit += 4
		actionability += 4
	}
	if input.AIReferrals > 0 {
		evidenceFit += 3
		actionability += 3
	}
	evidenceFit = clampScore(evidenceFit)
	actionability = clampScore(actionability)
	supportBonus := 0
	if input.TopPathPageviews > 0 {
		supportBonus += 4
	}
	if input.AIReferrals > 0 {
		supportBonus += 3
	}
	total := clampScore((sample * 30 / 100) + (impact * 25 / 100) + (actionability * 25 / 100) + (evidenceFit * 20 / 100) + supportBonus)
	return opportunityScoreBreakdown{
		Sample:        sample,
		Impact:        impact,
		Confidence:    confidence(input.Requests >= 20 && (input.TopPathPageviews > 0 || input.AIReferrals > 0)),
		Urgency:       68,
		Effort:        70,
		Actionability: actionability,
		EvidenceFit:   evidenceFit,
		Freshness:     70,
		Total:         total,
	}
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

func scoreSearchVisibilityOpportunity(input searchVisibilityScoringInput) (opportunityScoreBreakdown, int, bool) {
	if input.Impressions < minSearchVisibilityImpressions || input.CTR < 0 || input.CTR >= searchVisibilityTargetCTR {
		return opportunityScoreBreakdown{}, 0, false
	}
	estimatedClicks := int(math.Round(math.Max(0, searchVisibilityTargetCTR-input.CTR) * float64(input.Impressions)))
	if estimatedClicks < 10 {
		return opportunityScoreBreakdown{}, 0, false
	}
	sample := clampScore(input.Impressions / 50)
	impact := clampScore(estimatedClicks / 2)
	urgency := clampScore(int(math.Round((searchVisibilityTargetCTR - input.CTR) * 2000)))
	total := clampScore((sample * 25 / 100) + (impact * 35 / 100) + (urgency * 30 / 100) + 8)
	return opportunityScoreBreakdown{
		Sample:        sample,
		Impact:        impact,
		Confidence:    confidence(sample >= 80),
		Urgency:       urgency,
		Effort:        62,
		Actionability: 82,
		EvidenceFit:   96,
		Freshness:     70,
		Total:         total,
	}, estimatedClicks, true
}

func scoreSetupGoalSuggestion(eventCount int) opportunityScoreBreakdown {
	sample := clampScore(eventCount * 10)
	impact := clampScore(55 + eventCount*2)
	total := clampScore((sample * 25 / 100) + (impact * 25 / 100) + 42)
	return opportunityScoreBreakdown{
		Sample:        sample,
		Impact:        impact,
		Confidence:    confidence(eventCount >= 10),
		Urgency:       72,
		Effort:        88,
		Actionability: 92,
		EvidenceFit:   98,
		Freshness:     70,
		Total:         total,
	}
}

func scoreSetupFunnelSuggestion(pageviews, eventCount int) opportunityScoreBreakdown {
	sample := clampScore((pageviews / 4) + eventCount*5)
	impact := clampScore(55 + pageviews/20 + eventCount)
	total := clampScore((sample * 25 / 100) + (impact * 25 / 100) + 43)
	return opportunityScoreBreakdown{
		Sample:        sample,
		Impact:        impact,
		Confidence:    confidence(pageviews >= 100 && eventCount >= 10),
		Urgency:       76,
		Effort:        82,
		Actionability: 90,
		EvidenceFit:   97,
		Freshness:     70,
		Total:         total,
	}
}
