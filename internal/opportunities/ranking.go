package opportunities

import (
	"maps"
	"sort"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
)

type opportunityRank struct {
	id            uuid.UUID
	status        string
	score         int
	impact        int
	urgency       int
	actionability int
	evidenceFit   int
	generatedAt   time.Time
	updatedAt     time.Time
}

func RankOpportunities(opportunities []api.Opportunity) []api.Opportunity {
	out := make([]api.Opportunity, len(opportunities))
	for i, opportunity := range opportunities {
		out[i] = normalizeRankedOpportunity(opportunity)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return ranksBefore(rankOpportunity(out[i]), rankOpportunity(out[j]))
	})
	return out
}

func rankOpportunityInputs(opportunities []database.OpportunityInput) []database.OpportunityInput {
	out := make([]database.OpportunityInput, len(opportunities))
	for i, opportunity := range opportunities {
		out[i] = normalizeRankedOpportunityInput(opportunity)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return ranksBefore(rankOpportunityInput(out[i]), rankOpportunityInput(out[j]))
	})
	return out
}

func normalizeRankedOpportunity(opportunity api.Opportunity) api.Opportunity {
	if opportunity.CopyParams == nil {
		opportunity.CopyParams = map[string]any{}
	} else {
		opportunity.CopyParams = maps.Clone(opportunity.CopyParams)
	}
	if opportunity.RouteParams == nil {
		opportunity.RouteParams = map[string]any{}
	} else {
		opportunity.RouteParams = maps.Clone(opportunity.RouteParams)
	}
	if opportunity.Evidence == nil {
		opportunity.Evidence = []api.OpportunityEvidence{}
	} else {
		opportunity.Evidence = append([]api.OpportunityEvidence(nil), opportunity.Evidence...)
	}
	if opportunity.CitedEvidenceIDs == nil {
		opportunity.CitedEvidenceIDs = []string{}
	} else {
		opportunity.CitedEvidenceIDs = append([]string(nil), opportunity.CitedEvidenceIDs...)
	}
	return opportunity
}

func normalizeRankedOpportunityInput(opportunity database.OpportunityInput) database.OpportunityInput {
	if opportunity.CopyParams == nil {
		opportunity.CopyParams = map[string]any{}
	} else {
		opportunity.CopyParams = maps.Clone(opportunity.CopyParams)
	}
	if opportunity.RouteParams == nil {
		opportunity.RouteParams = map[string]any{}
	} else {
		opportunity.RouteParams = maps.Clone(opportunity.RouteParams)
	}
	if opportunity.Evidence == nil {
		opportunity.Evidence = []api.OpportunityEvidence{}
	} else {
		opportunity.Evidence = append([]api.OpportunityEvidence(nil), opportunity.Evidence...)
	}
	if opportunity.CitedEvidenceIDs == nil {
		opportunity.CitedEvidenceIDs = []string{}
	} else {
		opportunity.CitedEvidenceIDs = append([]string(nil), opportunity.CitedEvidenceIDs...)
	}
	return opportunity
}

func rankOpportunity(opportunity api.Opportunity) opportunityRank {
	return opportunityRank{
		id:            opportunity.ID,
		status:        opportunity.Status,
		score:         opportunity.Score,
		impact:        opportunity.ScoreBreakdown.Impact,
		urgency:       opportunity.ScoreBreakdown.Urgency,
		actionability: opportunity.ScoreBreakdown.Actionability,
		evidenceFit:   opportunity.ScoreBreakdown.EvidenceFit,
		generatedAt:   opportunity.GeneratedAt,
		updatedAt:     opportunity.UpdatedAt,
	}
}

func rankOpportunityInput(opportunity database.OpportunityInput) opportunityRank {
	return opportunityRank{
		id:            opportunity.ID,
		status:        opportunity.Status,
		score:         opportunity.Score,
		impact:        opportunity.ScoreBreakdown.Impact,
		urgency:       opportunity.ScoreBreakdown.Urgency,
		actionability: opportunity.ScoreBreakdown.Actionability,
		evidenceFit:   opportunity.ScoreBreakdown.EvidenceFit,
		generatedAt:   opportunity.GeneratedAt,
	}
}

func ranksBefore(left, right opportunityRank) bool {
	if leftActionable, rightActionable := opportunityStatusActionable(left.status), opportunityStatusActionable(right.status); leftActionable != rightActionable {
		return leftActionable
	}
	if left.score != right.score {
		return left.score > right.score
	}
	if left.impact != right.impact {
		return left.impact > right.impact
	}
	if left.actionability != right.actionability {
		return left.actionability > right.actionability
	}
	if left.evidenceFit != right.evidenceFit {
		return left.evidenceFit > right.evidenceFit
	}
	if left.urgency != right.urgency {
		return left.urgency > right.urgency
	}
	if !left.updatedAt.Equal(right.updatedAt) {
		return left.updatedAt.After(right.updatedAt)
	}
	if !left.generatedAt.Equal(right.generatedAt) {
		return left.generatedAt.After(right.generatedAt)
	}
	return left.id.String() < right.id.String()
}

func opportunityStatusActionable(status string) bool {
	return status == "new" || status == "saved"
}
