package opportunities

import (
	"cmp"
	"maps"
	"slices"
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
	slices.SortStableFunc(out, func(left, right api.Opportunity) int {
		return compareOpportunityRanks(rankOpportunity(left), rankOpportunity(right))
	})
	return out
}

func rankOpportunityInputs(opportunities []database.OpportunityInput) []database.OpportunityInput {
	out := make([]database.OpportunityInput, len(opportunities))
	for i, opportunity := range opportunities {
		out[i] = normalizeRankedOpportunityInput(opportunity)
	}
	slices.SortStableFunc(out, func(left, right database.OpportunityInput) int {
		return compareOpportunityRanks(rankOpportunityInput(left), rankOpportunityInput(right))
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

func compareOpportunityRanks(left, right opportunityRank) int {
	if leftActionable, rightActionable := opportunityStatusActionable(left.status), opportunityStatusActionable(right.status); leftActionable != rightActionable {
		if leftActionable {
			return -1
		}
		return 1
	}
	if left.score != right.score {
		return cmp.Compare(right.score, left.score)
	}
	if left.impact != right.impact {
		return cmp.Compare(right.impact, left.impact)
	}
	if left.actionability != right.actionability {
		return cmp.Compare(right.actionability, left.actionability)
	}
	if left.evidenceFit != right.evidenceFit {
		return cmp.Compare(right.evidenceFit, left.evidenceFit)
	}
	if left.urgency != right.urgency {
		return cmp.Compare(right.urgency, left.urgency)
	}
	if !left.updatedAt.Equal(right.updatedAt) {
		return right.updatedAt.Compare(left.updatedAt)
	}
	if !left.generatedAt.Equal(right.generatedAt) {
		return right.generatedAt.Compare(left.generatedAt)
	}
	return cmp.Compare(left.id.String(), right.id.String())
}

func opportunityStatusActionable(status string) bool {
	return status == "new" || status == "saved"
}
