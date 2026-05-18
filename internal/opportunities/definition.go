package opportunities

import (
	"time"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
)

type OpportunityDefinition struct {
	Key                 string
	Kind                string
	Category            DetectorCategory
	TypeKey             string
	MessageKeys         DetectorMessageKeys
	AllowedParams       []string
	ActionTypes         []string
	IdentityEvidenceIDs []string
	RequiredSignals     []OpportunitySignal
	OptionalSignals     []OpportunitySignal
	RouteIcon           string
	Detect              OpportunityDetectFunc
}

type OpportunityDetectFunc func(DetectorInput, OpportunityDefinition) (*database.OpportunityInput, bool)

type OpportunityRecipe struct {
	CopyParams       map[string]any
	ImpactValue      string
	Confidence       string
	Score            int
	ScoreBreakdown   api.OpportunityScoreBreakdown
	RouteParams      map[string]any
	Evidence         []api.OpportunityEvidence
	CitedEvidenceIDs []string
}

func (d OpportunityDefinition) Contract() DetectorContract {
	return DetectorContract{
		Category:            d.Category,
		TypeKey:             d.TypeKey,
		MessageKeys:         d.MessageKeys,
		AllowedParams:       append([]string(nil), d.AllowedParams...),
		ActionTypes:         append([]string(nil), d.ActionTypes...),
		IdentityEvidenceIDs: append([]string(nil), d.IdentityEvidenceIDs...),
		RequiredSignals:     append([]OpportunitySignal(nil), d.RequiredSignals...),
		OptionalSignals:     append([]OpportunitySignal(nil), d.OptionalSignals...),
	}
}

func DefaultOpportunityDefinitions() []OpportunityDefinition {
	return copyOpportunityDefinitions([]OpportunityDefinition{
		checkoutOpportunityDefinition,
		aiVisibilityOpportunityDefinition,
		trafficQualityOpportunityDefinition,
		webVitalsOpportunityDefinition,
		searchVisibilityOpportunityDefinition,
		setupGoalSuggestionOpportunityDefinition,
		setupFunnelSuggestionOpportunityDefinition,
		conversionSignalOpportunityDefinition,
	})
}

func copyOpportunityDefinition(definition OpportunityDefinition) OpportunityDefinition {
	definition.AllowedParams = append([]string(nil), definition.AllowedParams...)
	definition.ActionTypes = append([]string(nil), definition.ActionTypes...)
	definition.IdentityEvidenceIDs = append([]string(nil), definition.IdentityEvidenceIDs...)
	definition.RequiredSignals = append([]OpportunitySignal(nil), definition.RequiredSignals...)
	definition.OptionalSignals = append([]OpportunitySignal(nil), definition.OptionalSignals...)
	return definition
}

func copyOpportunityDefinitions(definitions []OpportunityDefinition) []OpportunityDefinition {
	out := make([]OpportunityDefinition, len(definitions))
	for i, definition := range definitions {
		out[i] = copyOpportunityDefinition(definition)
	}
	return out
}

var checkoutOpportunityDefinition = OpportunityDefinition{
	Key:      "checkout-conversion",
	Kind:     "conversion",
	Category: DetectorCategoryConversion,
	TypeKey:  "opportunities.types.checkout_conversion",
	MessageKeys: DetectorMessageKeys{
		Title:       "opportunities.catalog.checkout_conversion.title",
		Summary:     "opportunities.catalog.checkout_conversion.summary",
		Action:      "opportunities.catalog.checkout_conversion.action",
		Digest:      "opportunities.catalog.checkout_conversion.digest",
		ImpactLabel: "opportunities.impact.checkout_starts",
		RouteLabel:  "opportunities.routes.checkout",
	},
	AllowedParams:       []string{"checkout_starts", "orders", "conversion_rate", "path", "top_city", "top_provider", "top_asn"},
	ActionTypes:         []string{"optimize_checkout"},
	IdentityEvidenceIDs: []string{"conversion_rate"},
	RequiredSignals:     []OpportunitySignal{OpportunitySignalEcommerce},
	RouteIcon:           "pi pi-shopping-cart",
	Detect:              detectCheckoutOpportunity,
}

var aiVisibilityOpportunityDefinition = OpportunityDefinition{
	Key:      "ai-visibility",
	Kind:     "ai",
	Category: DetectorCategoryAIVisibility,
	TypeKey:  "opportunities.types.ai_visibility",
	MessageKeys: DetectorMessageKeys{
		Title:       "opportunities.catalog.ai_visibility.title",
		Summary:     "opportunities.catalog.ai_visibility.summary",
		Action:      "opportunities.catalog.ai_visibility.action",
		Digest:      "opportunities.catalog.ai_visibility.digest",
		ImpactLabel: "opportunities.impact.ai_touched_pages",
		RouteLabel:  "opportunities.routes.path",
	},
	AllowedParams:       []string{"requests", "unique_paths", "top_path", "path", "ai_referrals", "top_path_pageviews", "top_city", "top_provider", "top_asn"},
	ActionTypes:         []string{"improve_content"},
	IdentityEvidenceIDs: []string{"top_ai_path"},
	RequiredSignals:     []OpportunitySignal{OpportunitySignalAIVisibility},
	OptionalSignals:     []OpportunitySignal{OpportunitySignalSiteStats},
	RouteIcon:           "pi pi-sparkles",
	Detect:              detectAIVisibilityOpportunity,
}

var trafficQualityOpportunityDefinition = OpportunityDefinition{
	Key:      "traffic-quality",
	Kind:     "traffic",
	Category: DetectorCategoryTrafficQuality,
	TypeKey:  "opportunities.types.traffic_quality",
	MessageKeys: DetectorMessageKeys{
		Title:       "opportunities.catalog.traffic_quality.title",
		Summary:     "opportunities.catalog.traffic_quality.summary",
		Action:      "opportunities.catalog.traffic_quality.action",
		Digest:      "opportunities.catalog.traffic_quality.digest",
		ImpactLabel: "opportunities.impact.pageviews_to_route",
		RouteLabel:  "opportunities.routes.source",
	},
	AllowedParams:       []string{"source", "source_hits", "total_pageviews", "sessions", "top_city", "top_provider", "top_asn"},
	ActionTypes:         []string{"route_traffic", "improve_content"},
	IdentityEvidenceIDs: []string{"top_source"},
	RequiredSignals:     []OpportunitySignal{OpportunitySignalSiteStats},
	RouteIcon:           "pi pi-chart-line",
	Detect:              detectTrafficQualityOpportunity,
}

var webVitalsOpportunityDefinition = OpportunityDefinition{
	Key:      "web-vitals-performance",
	Kind:     "performance",
	Category: DetectorCategoryPerformance,
	TypeKey:  "opportunities.types.web_vitals_performance",
	MessageKeys: DetectorMessageKeys{
		Title:       "opportunities.catalog.web_vitals_performance.title",
		Summary:     "opportunities.catalog.web_vitals_performance.summary",
		Action:      "opportunities.catalog.web_vitals_performance.action",
		Digest:      "opportunities.catalog.web_vitals_performance.digest",
		ImpactLabel: "opportunities.impact.web_vitals_samples",
		RouteLabel:  "opportunities.routes.web_vitals",
	},
	AllowedParams: []string{
		"metric",
		"p75",
		"rating",
		"samples",
		"poor_samples",
		"needs_improvement_samples",
		"path",
		"page_p75",
		"page_samples",
		"top_city",
		"top_provider",
		"top_asn",
	},
	ActionTypes:         []string{"improve_performance"},
	IdentityEvidenceIDs: []string{"web_vital_metric", "web_vital_top_page"},
	RequiredSignals:     []OpportunitySignal{OpportunitySignalWebVitals},
	RouteIcon:           "pi pi-gauge",
	Detect:              detectWebVitalsOpportunity,
}

var searchVisibilityOpportunityDefinition = OpportunityDefinition{
	Key:      "search-visibility",
	Kind:     "search",
	Category: DetectorCategorySearchVisibility,
	TypeKey:  "opportunities.types.search_visibility",
	MessageKeys: DetectorMessageKeys{
		Title:       "opportunities.catalog.search_visibility.title",
		Summary:     "opportunities.catalog.search_visibility.summary",
		Action:      "opportunities.catalog.search_visibility.action",
		Digest:      "opportunities.catalog.search_visibility.digest",
		ImpactLabel: "opportunities.impact.estimated_search_clicks",
		RouteLabel:  "opportunities.routes.search_console",
	},
	AllowedParams:       []string{"impressions", "clicks", "ctr", "average_position", "estimated_clicks"},
	ActionTypes:         []string{"improve_content"},
	IdentityEvidenceIDs: []string{"search_ctr", "search_position"},
	RequiredSignals:     []OpportunitySignal{OpportunitySignalSearchConsole},
	RouteIcon:           "pi pi-search",
	Detect:              detectSearchVisibilityOpportunity,
}

var conversionSignalOpportunityDefinition = OpportunityDefinition{
	Key:      "conversion-signal",
	Kind:     "setup",
	Category: DetectorCategorySetupQuality,
	TypeKey:  "opportunities.types.conversion_signal",
	MessageKeys: DetectorMessageKeys{
		Title:       "opportunities.catalog.conversion_signal.title",
		Summary:     "opportunities.catalog.conversion_signal.summary",
		Action:      "opportunities.catalog.conversion_signal.action",
		Digest:      "opportunities.catalog.conversion_signal.digest",
		ImpactLabel: "opportunities.impact.conversion_signal_coverage",
		RouteLabel:  "opportunities.routes.events",
	},
	AllowedParams:       []string{"pageviews", "sessions", "event_count", "event_names"},
	ActionTypes:         []string{"define_conversion_signal", "fix_tracking"},
	IdentityEvidenceIDs: []string{"event_names"},
	RequiredSignals:     []OpportunitySignal{OpportunitySignalSiteStats, OpportunitySignalEcommerce, OpportunitySignalEvents},
	RouteIcon:           "pi pi-bullseye",
	Detect:              detectConversionSignalOpportunity,
}

var setupGoalSuggestionOpportunityDefinition = OpportunityDefinition{
	Key:      "setup-goal-suggestion",
	Kind:     "setup",
	Category: DetectorCategorySetupQuality,
	TypeKey:  "opportunities.types.setup_goal_suggestion",
	MessageKeys: DetectorMessageKeys{
		Title:       "opportunities.catalog.setup_goal_suggestion.title",
		Summary:     "opportunities.catalog.setup_goal_suggestion.summary",
		Action:      "opportunities.catalog.setup_goal_suggestion.action",
		Digest:      "opportunities.catalog.setup_goal_suggestion.digest",
		ImpactLabel: "opportunities.impact.conversion_events_to_measure",
		RouteLabel:  "opportunities.routes.event",
	},
	AllowedParams:       []string{"event_name", "event_count", "goal_type", "goal_value"},
	ActionTypes:         []string{"create_goal"},
	IdentityEvidenceIDs: []string{"suggested_goal_event"},
	RequiredSignals:     []OpportunitySignal{OpportunitySignalSetupEvidence},
	RouteIcon:           "pi pi-bullseye",
	Detect:              detectSetupGoalSuggestionOpportunity,
}

var setupFunnelSuggestionOpportunityDefinition = OpportunityDefinition{
	Key:      "setup-funnel-suggestion",
	Kind:     "setup",
	Category: DetectorCategorySetupQuality,
	TypeKey:  "opportunities.types.setup_funnel_suggestion",
	MessageKeys: DetectorMessageKeys{
		Title:       "opportunities.catalog.setup_funnel_suggestion.title",
		Summary:     "opportunities.catalog.setup_funnel_suggestion.summary",
		Action:      "opportunities.catalog.setup_funnel_suggestion.action",
		Digest:      "opportunities.catalog.setup_funnel_suggestion.digest",
		ImpactLabel: "opportunities.impact.funnel_steps_to_measure",
		RouteLabel:  "opportunities.routes.funnel",
	},
	AllowedParams:       []string{"start_path", "conversion_event", "event_count", "step_count", "funnel_steps"},
	ActionTypes:         []string{"create_funnel"},
	IdentityEvidenceIDs: []string{"suggested_funnel_start", "suggested_funnel_conversion_event"},
	RequiredSignals:     []OpportunitySignal{OpportunitySignalSetupEvidence},
	RouteIcon:           "pi pi-sitemap",
	Detect:              detectSetupFunnelSuggestionOpportunity,
}

func (d OpportunityDefinition) BaseOpportunity(input DetectorInput, generatedAt time.Time) database.OpportunityInput {
	return database.OpportunityInput{
		ID:              stableOpportunityID(input.SiteID, d.Key),
		TeamID:          input.TeamID,
		SiteID:          input.SiteID,
		Kind:            d.Kind,
		TypeKey:         d.TypeKey,
		TitleKey:        d.MessageKeys.Title,
		SummaryKey:      d.MessageKeys.Summary,
		ActionKey:       d.MessageKeys.Action,
		DigestKey:       d.MessageKeys.Digest,
		ImpactLabelKey:  d.MessageKeys.ImpactLabel,
		Status:          "new",
		RouteLabelKey:   d.MessageKeys.RouteLabel,
		RouteIcon:       d.RouteIcon,
		DetectorVersion: detectorVersion,
		GeneratedAt:     generatedAt,
	}
}

func (d OpportunityDefinition) BuildOpportunity(input DetectorInput, recipe OpportunityRecipe) database.OpportunityInput {
	generatedAt := input.GeneratedAt
	if generatedAt.IsZero() {
		generatedAt = time.Now().UTC()
	}
	opportunity := d.BaseOpportunity(input, generatedAt)
	opportunity.CopyParams = copyOpportunityMap(recipe.CopyParams)
	opportunity.ImpactValue = recipe.ImpactValue
	opportunity.Confidence = recipe.Confidence
	opportunity.Score = recipe.Score
	opportunity.ScoreBreakdown = recipe.ScoreBreakdown
	opportunity.RouteParams = copyOpportunityMap(recipe.RouteParams)
	opportunity.Evidence = append([]api.OpportunityEvidence(nil), recipe.Evidence...)
	opportunity.CitedEvidenceIDs = append([]string(nil), recipe.CitedEvidenceIDs...)
	return opportunity
}

func (d OpportunityDefinition) Detector() Detector {
	return definitionDetector{definition: d}
}

type definitionDetector struct {
	definition OpportunityDefinition
}

func (d definitionDetector) Contract() DetectorContract {
	return d.definition.Contract()
}

func (d definitionDetector) Definition() OpportunityDefinition {
	return copyOpportunityDefinition(d.definition)
}

func (d definitionDetector) Detect(input DetectorInput) (*database.OpportunityInput, bool) {
	if d.definition.Detect == nil {
		return nil, false
	}
	return d.definition.Detect(input, d.definition)
}
