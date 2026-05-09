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
		AllowedParams: append([]string(nil), d.AllowedParams...),
		CopyParams:    copyOpportunityMap(opportunity.CopyParams),
		Evidence:      aiEvidenceFromOpportunity(opportunity),
		ImpactValue:   opportunity.ImpactValue,
		Confidence:    opportunity.Confidence,
		Kind:          d.Kind,
		RouteParams:   copyOpportunityMap(opportunity.RouteParams),
	}
}

func copyOpportunityMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = copyOpportunityValue(item)
	}
	return out
}

func copyOpportunityValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return copyOpportunityMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = copyOpportunityValue(item)
		}
		return out
	case []string:
		return append([]string(nil), typed...)
	case []int:
		return append([]int(nil), typed...)
	case []float64:
		return append([]float64(nil), typed...)
	case []bool:
		return append([]bool(nil), typed...)
	default:
		return value
	}
}
