package opportunities

import (
	"time"

	hitai "hitkeep/internal/ai"
	"hitkeep/internal/database"
)

type OpportunityCopyContext struct {
	SiteDomain string
	From       time.Time
	To         time.Time
}

func (d OpportunityDefinition) AICopyContract(ctx OpportunityCopyContext, opportunity database.OpportunityInput) hitai.OpportunityDetectorInput {
	return hitai.OpportunityDetectorInput{
		SiteDomain: ctx.SiteDomain,
		From:       ctx.From,
		To:         ctx.To,
		TypeKey:    d.TypeKey,
		Category:   string(d.Category),
		MessageKeys: hitai.OpportunityMessageKeys{
			Title:   d.MessageKeys.Title,
			Summary: d.MessageKeys.Summary,
			Action:  d.MessageKeys.Action,
			Digest:  d.MessageKeys.Digest,
		},
		AllowedParams:      append([]string(nil), d.AllowedParams...),
		AllowedActionTypes: append([]string(nil), d.ActionTypes...),
		CopyParams:         copyOpportunityMap(opportunity.CopyParams),
		Evidence:           aiEvidenceFromOpportunity(opportunity),
		ImpactValue:        opportunity.ImpactValue,
		Confidence:         opportunity.Confidence,
		Kind:               d.Kind,
		RouteParams:        copyOpportunityMap(opportunity.RouteParams),
	}
}
