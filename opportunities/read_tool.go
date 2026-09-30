package opportunities

import (
	"context"
	"strings"
	"time"

	"hitkeep/analyticstools"
	"hitkeep/api"
)

const ToolGetOpportunities = "hitkeep_get_opportunities"

type readInput struct {
	analyticstools.SiteInput
	Status string `json:"status,omitempty" jsonschema:"Optional opportunity status filter: new, saved, done, dismissed, or all. Defaults to all statuses."`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum rows to return. Defaults to 10 and is capped at 50."`
}

type ReadOutput struct {
	SiteID        string             `json:"site_id"`
	Opportunities []SavedOpportunity `json:"opportunities"`
}

type SavedOpportunity struct {
	ID               string                        `json:"id"`
	SiteID           string                        `json:"site_id"`
	Kind             string                        `json:"kind"`
	TypeKey          string                        `json:"type_key"`
	TitleKey         string                        `json:"title_key"`
	SummaryKey       string                        `json:"summary_key"`
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
	DetectorVersion  string                        `json:"detector_version"`
	Evidence         []api.OpportunityEvidence     `json:"evidence"`
	CitedEvidenceIDs []string                      `json:"cited_evidence_ids"`
	GeneratedAt      string                        `json:"generated_at"`
	CreatedAt        string                        `json:"created_at"`
	UpdatedAt        string                        `json:"updated_at"`
}

// ReadTool reads saved, ranked Opportunities. It is never offered to the
// Opportunities run itself: saved recommendations are not evidence.
var ReadTool = analyticstools.Define(ToolGetOpportunities, "Get HitKeep Opportunities",
	"Read saved Opportunities recommendations for one site, ranked by impact and urgency, as localization keys, params, and cited evidence. Never returns raw prompts or provider payloads.",
	func(ctx context.Context, call analyticstools.Call, in readInput) (ReadOutput, error) {
		status, ok := statusFilters[strings.ToLower(strings.TrimSpace(in.Status))]
		if !ok {
			return ReadOutput{}, analyticstools.InvalidInput("invalid opportunity status %q", in.Status)
		}
		limit := in.Limit
		if limit <= 0 {
			limit = 10
		}
		limit = min(limit, 50)
		saved, err := call.Control.ListOpportunities(ctx, call.ID)
		if err != nil {
			return ReadOutput{}, err
		}
		out := make([]SavedOpportunity, 0, min(len(saved), limit))
		for _, o := range RankOpportunities(saved) {
			if status != "" && o.Status != status {
				continue
			}
			out = append(out, toReadOpportunity(o))
			if len(out) == limit {
				break
			}
		}
		return ReadOutput{SiteID: call.ID.String(), Opportunities: out}, nil
	})

var statusFilters = map[string]string{
	"":          "",
	"all":       "",
	"new":       "new",
	"saved":     "saved",
	"done":      "done",
	"dismissed": "dismissed",
}

func toReadOpportunity(o api.Opportunity) SavedOpportunity {
	format := func(ts time.Time) string { return ts.UTC().Format(time.RFC3339) }
	return SavedOpportunity{
		ID:               o.ID.String(),
		SiteID:           o.SiteID.String(),
		Kind:             o.Kind,
		TypeKey:          o.TypeKey,
		TitleKey:         o.TitleKey,
		SummaryKey:       o.SummaryKey,
		ActionKey:        o.ActionKey,
		DigestKey:        o.DigestKey,
		CopyParams:       o.CopyParams,
		ImpactValue:      o.ImpactValue,
		ImpactLabelKey:   o.ImpactLabelKey,
		Confidence:       o.Confidence,
		Score:            o.Score,
		ScoreBreakdown:   o.ScoreBreakdown,
		Status:           o.Status,
		RouteLabelKey:    o.RouteLabelKey,
		RouteParams:      o.RouteParams,
		RouteIcon:        o.RouteIcon,
		DetectorVersion:  o.DetectorVersion,
		Evidence:         citedEvidence(o.Evidence, o.CitedEvidenceIDs),
		CitedEvidenceIDs: o.CitedEvidenceIDs,
		GeneratedAt:      format(o.GeneratedAt),
		CreatedAt:        format(o.CreatedAt),
		UpdatedAt:        format(o.UpdatedAt),
	}
}

func citedEvidence(evidence []api.OpportunityEvidence, citedIDs []string) []api.OpportunityEvidence {
	cited := make(map[string]bool, len(citedIDs))
	for _, id := range citedIDs {
		if strings.TrimSpace(id) != "" {
			cited[id] = true
		}
	}
	out := make([]api.OpportunityEvidence, 0, len(evidence))
	for _, item := range evidence {
		if cited[item.ID] {
			out = append(out, item)
		}
	}
	return out
}
