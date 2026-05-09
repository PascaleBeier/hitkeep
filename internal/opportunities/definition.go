package opportunities

import (
	"time"

	"hitkeep/internal/database"
)

type OpportunityDefinition struct {
	Key           string
	Kind          string
	Category      DetectorCategory
	TypeKey       string
	MessageKeys   DetectorMessageKeys
	AllowedParams []string
	RouteIcon     string
}

func (d OpportunityDefinition) Contract() DetectorContract {
	return DetectorContract{
		Category:      d.Category,
		TypeKey:       d.TypeKey,
		MessageKeys:   d.MessageKeys,
		AllowedParams: append([]string(nil), d.AllowedParams...),
	}
}

func DefaultOpportunityDefinitions() []OpportunityDefinition {
	return copyOpportunityDefinitions([]OpportunityDefinition{
		checkoutOpportunityDefinition,
		aiVisibilityOpportunityDefinition,
		trafficQualityOpportunityDefinition,
		trackingSetupOpportunityDefinition,
	})
}

func copyOpportunityDefinitions(definitions []OpportunityDefinition) []OpportunityDefinition {
	out := make([]OpportunityDefinition, len(definitions))
	for i, definition := range definitions {
		out[i] = definition
		out[i].AllowedParams = append([]string(nil), definition.AllowedParams...)
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
		ImpactLabel: "opportunities.impact.estimated_monthly_upside",
		RouteLabel:  "opportunities.routes.checkout",
	},
	AllowedParams: []string{"checkout_starts", "orders", "conversion_rate", "monthly_upside", "currency", "path"},
	RouteIcon:     "pi pi-shopping-cart",
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
	AllowedParams: []string{"requests", "unique_paths", "top_path", "path"},
	RouteIcon:     "pi pi-sparkles",
}

var trafficQualityOpportunityDefinition = OpportunityDefinition{
	Key:      "traffic-quality",
	Kind:     "revenue",
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
	AllowedParams: []string{"source", "pageviews", "sessions"},
	RouteIcon:     "pi pi-chart-line",
}

var trackingSetupOpportunityDefinition = OpportunityDefinition{
	Key:      "tracking-setup",
	Kind:     "setup",
	Category: DetectorCategorySetupQuality,
	TypeKey:  "opportunities.types.tracking_setup",
	MessageKeys: DetectorMessageKeys{
		Title:       "opportunities.catalog.tracking_setup.title",
		Summary:     "opportunities.catalog.tracking_setup.summary",
		Action:      "opportunities.catalog.tracking_setup.action",
		Digest:      "opportunities.catalog.tracking_setup.digest",
		ImpactLabel: "opportunities.impact.tracked_conversion_events",
		RouteLabel:  "opportunities.routes.tracker",
	},
	AllowedParams: []string{"pageviews", "events", "asset"},
	RouteIcon:     "pi pi-code",
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
