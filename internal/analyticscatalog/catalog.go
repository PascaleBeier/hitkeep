package analyticscatalog

const (
	ToolSiteOverview   = "hitkeep_get_site_overview"
	ToolEventNames     = "hitkeep_get_event_names"
	ToolEventBreakdown = "hitkeep_get_event_breakdown"
	ToolEcommerce      = "hitkeep_get_ecommerce"
	ToolWebVitals      = "hitkeep_get_web_vitals"
	ToolAIVisibility   = "hitkeep_get_ai_visibility"
)

type ToolDefinition struct {
	Name           string
	Title          string
	MCPDescription string
	AIDescription  string
}

var ReadOnlyAggregateTools = []ToolDefinition{
	{
		Name:           ToolSiteOverview,
		Title:          "Get HitKeep Site Overview",
		MCPDescription: "Read aggregate traffic KPIs, top dimensions, goals, chart data, and optional comparison for one site.",
		AIDescription:  "Read aggregate traffic KPIs, chart data, top dimensions, goals, funnels, and optional filters for the scoped site.",
	},
	{
		Name:           ToolEventNames,
		Title:          "Get HitKeep Event Names",
		MCPDescription: "List event names tracked for one site in a date range.",
		AIDescription:  "List tracked event names for the scoped site and date range.",
	},
	{
		Name:           ToolEventBreakdown,
		Title:          "Get HitKeep Event Breakdown",
		MCPDescription: "Read a count breakdown for one event property.",
		AIDescription:  "Read a count breakdown for one event property on the scoped site.",
	},
	{
		Name:           ToolEcommerce,
		Title:          "Get HitKeep Ecommerce Analytics",
		MCPDescription: "Read ecommerce summary, top products, source stats, and aggregate city, provider, and ASN breakdowns for one site.",
		AIDescription:  "Read aggregate ecommerce summary, timeseries, products, and sources for the scoped site.",
	},
	{
		Name:           ToolWebVitals,
		Title:          "Get HitKeep Web Vitals",
		MCPDescription: "Read aggregate Web Vitals p75, sample counts, rating counts, and optional page or visitor-context breakdowns for one site.",
		AIDescription:  "Read aggregate Web Vitals summary and optional metric details for the scoped site.",
	},
	{
		Name:           ToolAIVisibility,
		Title:          "Get HitKeep AI Visibility",
		MCPDescription: "Read AI crawler fetch overview, timeseries, and optional fetch-to-visit correlation for one site.",
		AIDescription:  "Read AI crawler fetch overview and optional correlation for the scoped site.",
	},
}

func Definition(name string) (ToolDefinition, bool) {
	for _, definition := range ReadOnlyAggregateTools {
		if definition.Name == name {
			return definition, true
		}
	}
	return ToolDefinition{}, false
}

func MustDefinition(name string) ToolDefinition {
	definition, ok := Definition(name)
	if !ok {
		panic("unknown analytics catalog tool: " + name)
	}
	return definition
}
