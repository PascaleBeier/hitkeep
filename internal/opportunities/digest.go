package opportunities

import (
	"context"
	"fmt"
	"maps"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

const (
	DigestPreviewReasonReady                = "ready"
	DigestPreviewReasonNoOpportunities      = "no_opportunities"
	DigestPreviewReasonUnsupportedFrequency = "unsupported_frequency"
)

const defaultDigestItemLimit = 3
const defaultDigestMinimumScore = 50

type DigestSelectionInput struct {
	Frequency     api.ReportFrequency
	Opportunities []api.Opportunity
	Limit         int
	MinimumScore  int
}

type DigestPreviewForSiteInput struct {
	SiteID       uuid.UUID
	Frequency    api.ReportFrequency
	Limit        int
	MinimumScore int
}

type OpportunityLister interface {
	ListOpportunities(context.Context, uuid.UUID) ([]api.Opportunity, error)
}

type DigestPreview struct {
	Frequency  api.ReportFrequency `json:"frequency"`
	ShouldSend bool                `json:"should_send"`
	Reason     string              `json:"reason"`
	Items      []DigestItem        `json:"items"`
}

type DigestItem struct {
	ID               string                        `json:"id"`
	SiteID           string                        `json:"site_id"`
	Kind             string                        `json:"kind"`
	TypeKey          string                        `json:"type_key"`
	Category         string                        `json:"category"`
	TitleKey         string                        `json:"title_key"`
	ActionKey        string                        `json:"action_key"`
	DigestKey        string                        `json:"digest_key"`
	CopyParams       map[string]any                `json:"copy_params"`
	ImpactValue      string                        `json:"impact_value"`
	ImpactLabelKey   string                        `json:"impact_label_key"`
	Confidence       string                        `json:"confidence"`
	Score            int                           `json:"score"`
	ScoreBreakdown   api.OpportunityScoreBreakdown `json:"score_breakdown"`
	Status           string                        `json:"status"`
	RouteLabelKey    string                        `json:"route_label_key"`
	RouteParams      map[string]any                `json:"route_params"`
	RouteIcon        string                        `json:"route_icon"`
	Evidence         []api.OpportunityEvidence     `json:"evidence"`
	CitedEvidenceIDs []string                      `json:"cited_evidence_ids"`
}

func SelectDigestPreview(input DigestSelectionInput) DigestPreview {
	preview := DigestPreview{
		Frequency: input.Frequency,
		Reason:    DigestPreviewReasonNoOpportunities,
	}
	if !digestFrequencySupported(input.Frequency) {
		preview.Reason = DigestPreviewReasonUnsupportedFrequency
		return preview
	}
	limit := input.Limit
	if limit <= 0 {
		limit = defaultDigestItemLimit
	}
	minimumScore := input.MinimumScore
	if minimumScore <= 0 {
		minimumScore = defaultDigestMinimumScore
	}
	items := digestActionableOpportunities(input.Opportunities, minimumScore)
	if len(items) == 0 {
		return preview
	}
	items = RankOpportunities(items)
	items = digestLimitSetupWarnings(items, limit)
	preview.Items = make([]DigestItem, 0, len(items))
	for _, opportunity := range items {
		preview.Items = append(preview.Items, digestItemFromOpportunity(opportunity))
	}
	preview.ShouldSend = true
	preview.Reason = DigestPreviewReasonReady
	return preview
}

func SelectDigestPreviewForSite(ctx context.Context, lister OpportunityLister, input DigestPreviewForSiteInput) (DigestPreview, error) {
	if lister == nil {
		return DigestPreview{}, fmt.Errorf("opportunity lister is required")
	}
	opportunities, err := lister.ListOpportunities(ctx, input.SiteID)
	if err != nil {
		return DigestPreview{}, fmt.Errorf("list digest opportunities: %w", err)
	}
	return SelectDigestPreview(DigestSelectionInput{
		Frequency:     input.Frequency,
		Opportunities: opportunities,
		Limit:         input.Limit,
		MinimumScore:  input.MinimumScore,
	}), nil
}

func digestFrequencySupported(freq api.ReportFrequency) bool {
	return freq == api.ReportFrequencyDaily || freq == api.ReportFrequencyWeekly
}

func digestActionableOpportunities(opportunities []api.Opportunity, minimumScore int) []api.Opportunity {
	out := make([]api.Opportunity, 0, len(opportunities))
	for _, opportunity := range opportunities {
		if opportunityStatusActionable(opportunity.Status) && opportunity.Score >= minimumScore {
			out = append(out, opportunity)
		}
	}
	return out
}

func digestLimitSetupWarnings(opportunities []api.Opportunity, limit int) []api.Opportunity {
	if len(opportunities) <= limit && digestOnlySetupOpportunities(opportunities) {
		return opportunities
	}
	if digestOnlySetupOpportunities(opportunities) {
		return opportunities[:min(limit, len(opportunities))]
	}
	out := make([]api.Opportunity, 0, limit)
	includedSetup := false
	for _, opportunity := range opportunities {
		if len(out) >= limit {
			break
		}
		if isSetupOpportunity(opportunity) {
			if includedSetup {
				continue
			}
			includedSetup = true
		}
		out = append(out, opportunity)
	}
	return out
}

func digestOnlySetupOpportunities(opportunities []api.Opportunity) bool {
	if len(opportunities) == 0 {
		return false
	}
	for _, opportunity := range opportunities {
		if !isSetupOpportunity(opportunity) {
			return false
		}
	}
	return true
}

func digestItemFromOpportunity(opportunity api.Opportunity) DigestItem {
	return DigestItem{
		ID:               opportunity.ID.String(),
		SiteID:           opportunity.SiteID.String(),
		Kind:             opportunity.Kind,
		TypeKey:          opportunity.TypeKey,
		Category:         digestCategory(opportunity),
		TitleKey:         opportunity.TitleKey,
		ActionKey:        opportunity.ActionKey,
		DigestKey:        opportunity.DigestKey,
		CopyParams:       copyMap(opportunity.CopyParams),
		ImpactValue:      opportunity.ImpactValue,
		ImpactLabelKey:   opportunity.ImpactLabelKey,
		Confidence:       opportunity.Confidence,
		Score:            opportunity.Score,
		ScoreBreakdown:   opportunity.ScoreBreakdown,
		Status:           opportunity.Status,
		RouteLabelKey:    opportunity.RouteLabelKey,
		RouteParams:      copyMap(opportunity.RouteParams),
		RouteIcon:        opportunity.RouteIcon,
		Evidence:         append([]api.OpportunityEvidence(nil), opportunity.Evidence...),
		CitedEvidenceIDs: append([]string(nil), opportunity.CitedEvidenceIDs...),
	}
}

func digestCategory(opportunity api.Opportunity) string {
	if definition, ok := digestDefinitionForType(opportunity.TypeKey); ok {
		return string(definition.Category)
	}
	return ""
}

func digestDefinitionForType(typeKey string) (OpportunityDefinition, bool) {
	for _, definition := range DefaultOpportunityDefinitions() {
		if definition.TypeKey == typeKey {
			return definition, true
		}
	}
	return OpportunityDefinition{}, false
}

func copyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	maps.Copy(out, in)
	return out
}

func isSetupOpportunity(opportunity api.Opportunity) bool {
	return digestCategory(opportunity) == string(DetectorCategorySetupQuality)
}
