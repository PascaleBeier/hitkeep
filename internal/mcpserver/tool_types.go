package mcpserver

import "hitkeep/internal/api"

const (
	defaultRangeDays = 30
	defaultLimit     = 10
	maxLimit         = 50
)

type rangeInput struct {
	From string `json:"from,omitempty" jsonschema:"Optional RFC3339 start timestamp. Defaults to 30 days before to."`
	To   string `json:"to,omitempty" jsonschema:"Optional RFC3339 end timestamp. Defaults to now."`
}

type siteRangeInput struct {
	SiteID string `json:"site_id" jsonschema:"HitKeep site UUID."`
	rangeInput
}

type filterInput struct {
	Type  string `json:"type" jsonschema:"Filter type: path, hostname, referrer, referrer_host, device, country, city, provider, asn, browser, language, utm_campaign, utm_content, utm_medium, utm_source, or utm_term."`
	Value string `json:"value" jsonschema:"Filter value."`
}

type siteOverviewInput struct {
	SiteID      string        `json:"site_id" jsonschema:"HitKeep site UUID."`
	From        string        `json:"from,omitempty" jsonschema:"Optional RFC3339 start timestamp. Defaults to 30 days before to."`
	To          string        `json:"to,omitempty" jsonschema:"Optional RFC3339 end timestamp. Defaults to now."`
	CompareFrom string        `json:"compare_from,omitempty" jsonschema:"Optional RFC3339 comparison start timestamp."`
	CompareTo   string        `json:"compare_to,omitempty" jsonschema:"Optional RFC3339 comparison end timestamp."`
	Filters     []filterInput `json:"filters,omitempty" jsonschema:"Optional analytics filters."`
}

type eventBreakdownInput struct {
	SiteID      string `json:"site_id" jsonschema:"HitKeep site UUID."`
	EventName   string `json:"event_name" jsonschema:"Event name to inspect."`
	PropertyKey string `json:"property_key" jsonschema:"Event property key to break down."`
	Limit       int    `json:"limit,omitempty" jsonschema:"Maximum rows to return. Defaults to 10 and is capped at 50."`
	rangeInput
}

type ecommerceInput struct {
	SiteID   string        `json:"site_id" jsonschema:"HitKeep site UUID."`
	ItemID   string        `json:"item_id,omitempty" jsonschema:"Optional ecommerce item id filter."`
	ItemName string        `json:"item_name,omitempty" jsonschema:"Optional ecommerce item name filter."`
	Limit    int           `json:"limit,omitempty" jsonschema:"Maximum rows to return. Defaults to 10 and is capped at 50."`
	Filters  []filterInput `json:"filters,omitempty" jsonschema:"Optional analytics filters."`
	rangeInput
}

type webVitalsInput struct {
	SiteID             string `json:"site_id" jsonschema:"HitKeep site UUID."`
	Metric             string `json:"metric,omitempty" jsonschema:"Optional Web Vital metric filter: LCP, INP, CLS, FCP, or TTFB. Defaults to all metrics for summary and LCP for page or dimension breakdowns."`
	Path               string `json:"path,omitempty" jsonschema:"Optional normalized page path filter."`
	Rating             string `json:"rating,omitempty" jsonschema:"Optional rating filter: good, needs_improvement, or poor."`
	IncludePages       bool   `json:"include_pages,omitempty" jsonschema:"Whether to include aggregate page breakdown rows."`
	BreakdownDimension string `json:"breakdown_dimension,omitempty" jsonschema:"Optional aggregate visitor context breakdown: browser, country, language, device, city, provider, or asn."`
	Limit              int    `json:"limit,omitempty" jsonschema:"Maximum page or breakdown rows to return. Defaults to 10 and is capped at 50."`
	rangeInput
}

type aiVisibilityInput struct {
	SiteID             string `json:"site_id" jsonschema:"HitKeep site UUID."`
	AssistantName      string `json:"assistant_name,omitempty" jsonschema:"Optional AI assistant name filter."`
	AssistantFamily    string `json:"assistant_family,omitempty" jsonschema:"Optional AI assistant family filter."`
	ResourceType       string `json:"resource_type,omitempty" jsonschema:"Optional fetched resource type filter."`
	IncludeCorrelation bool   `json:"include_correlation,omitempty" jsonschema:"Whether to include fetch-to-visit correlation details."`
	WindowDays         int    `json:"window_days,omitempty" jsonschema:"Correlation window in days. Defaults to 30 and is capped at 90."`
	rangeInput
}

type opportunitiesInput struct {
	SiteID string `json:"site_id" jsonschema:"HitKeep site UUID."`
	Status string `json:"status,omitempty" jsonschema:"Optional opportunity status filter: new, saved, done, dismissed, or all. Defaults to all statuses."`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum rows to return. Defaults to 10 and is capped at 50."`
}

type searchConsoleStatusInput struct {
	SiteID string `json:"site_id" jsonschema:"HitKeep site UUID."`
}

type searchConsoleInput struct {
	SiteID   string   `json:"site_id" jsonschema:"HitKeep site UUID."`
	Sections []string `json:"sections,omitempty" jsonschema:"Optional sections: overview, series, queries, pages, country, or device. Defaults to overview and series."`
	Page     string   `json:"page,omitempty" jsonschema:"Optional exact Google Search Console page URL filter."`
	Path     string   `json:"path,omitempty" jsonschema:"Optional normalized page path filter."`
	Country  string   `json:"country,omitempty" jsonschema:"Optional country filter. Accepts alpha-2 or alpha-3 country code."`
	Device   string   `json:"device,omitempty" jsonschema:"Optional Google Search Console device filter."`
	Limit    int      `json:"limit,omitempty" jsonschema:"Maximum rows to return. Defaults to 10 and is capped at 50."`
	rangeInput
}

type docQueryInput struct {
	Query string `json:"query" jsonschema:"Search query for official HitKeep docs."`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum rows to return. Defaults to 10 and is capped at 50."`
}

type docPathInput struct {
	Path string `json:"path" jsonschema:"Official HitKeep docs path or URL."`
}

type apiReferenceInput struct {
	PathOrOperation string `json:"path_or_operation" jsonschema:"API docs path, operation slug, or official docs URL."`
}

type listSitesOutput struct {
	Sites []mcpSite `json:"sites"`
}

type siteOverviewOutput struct {
	SiteID string        `json:"site_id"`
	From   string        `json:"from"`
	To     string        `json:"to"`
	Stats  *mcpSiteStats `json:"stats"`
}

type eventNamesOutput struct {
	SiteID string   `json:"site_id"`
	From   string   `json:"from"`
	To     string   `json:"to"`
	Names  []string `json:"names"`
}

type eventBreakdownOutput struct {
	SiteID    string           `json:"site_id"`
	From      string           `json:"from"`
	To        string           `json:"to"`
	Breakdown []api.MetricStat `json:"breakdown"`
}

type ecommerceOutput struct {
	SiteID   string                     `json:"site_id"`
	From     string                     `json:"from"`
	To       string                     `json:"to"`
	Summary  *api.EcommerceSummary      `json:"summary"`
	Products []api.EcommerceProductStat `json:"products"`
	Sources  []api.EcommerceSourceStat  `json:"sources"`
}

type webVitalsOutput struct {
	SiteID             string                      `json:"site_id"`
	From               string                      `json:"from"`
	To                 string                      `json:"to"`
	Metric             api.WebVitalMetric          `json:"metric,omitempty"`
	Summary            []api.WebVitalSummaryMetric `json:"summary"`
	Pages              []api.WebVitalPageRow       `json:"pages,omitempty"`
	BreakdownDimension api.WebVitalDimension       `json:"breakdown_dimension,omitempty"`
	Breakdown          []api.WebVitalDimensionRow  `json:"breakdown,omitempty"`
}

type aiVisibilityOutput struct {
	SiteID      string                        `json:"site_id"`
	From        string                        `json:"from"`
	To          string                        `json:"to"`
	Overview    *api.AIFetchOverview          `json:"overview"`
	Timeseries  []mcpAIFetchSeriesPoint       `json:"timeseries"`
	Correlation *api.AIFetchCorrelationReport `json:"correlation,omitempty"`
}

type opportunitiesOutput struct {
	SiteID        string           `json:"site_id"`
	Opportunities []mcpOpportunity `json:"opportunities"`
}

type mcpOpportunity struct {
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

type searchConsoleStatusOutput struct {
	SiteID                  string                      `json:"site_id"`
	TeamID                  string                      `json:"team_id,omitempty"`
	Mapped                  bool                        `json:"mapped"`
	PropertyURI             string                      `json:"property_uri,omitempty"`
	PropertyPermissionLevel string                      `json:"property_permission_level,omitempty"`
	SyncStatus              *mcpSearchConsoleSyncStatus `json:"sync_status,omitempty"`
	DataAvailable           bool                        `json:"data_available"`
	AvailableFrom           string                      `json:"available_from,omitempty"`
	AvailableTo             string                      `json:"available_to,omitempty"`
	NeedsAttention          bool                        `json:"needs_attention"`
	Reason                  string                      `json:"reason"`
}

type searchConsoleOutput struct {
	SiteID      string                              `json:"site_id"`
	From        string                              `json:"from"`
	To          string                              `json:"to"`
	PropertyURI string                              `json:"property_uri,omitempty"`
	SyncStatus  *mcpSearchConsoleSyncStatus         `json:"sync_status,omitempty"`
	Overview    *api.SearchConsoleOverview          `json:"overview,omitempty"`
	Series      *mcpSearchConsoleSeriesResponse     `json:"series,omitempty"`
	Queries     *api.SearchConsoleDimensionResponse `json:"queries,omitempty"`
	Pages       *api.SearchConsoleDimensionResponse `json:"pages,omitempty"`
	Country     *api.SearchConsoleDimensionResponse `json:"country,omitempty"`
	Device      *api.SearchConsoleDimensionResponse `json:"device,omitempty"`
	Warnings    []string                            `json:"warnings"`
}

type mcpSearchConsoleSyncStatus struct {
	State             string `json:"state"`
	ImportedStartDate string `json:"imported_start_date,omitempty"`
	ImportedEndDate   string `json:"imported_end_date,omitempty"`
	LastSuccessAt     string `json:"last_success_at,omitempty"`
	LastAttemptAt     string `json:"last_attempt_at,omitempty"`
	LastErrorCategory string `json:"last_error_category,omitempty"`
	NextRetryAt       string `json:"next_retry_at,omitempty"`
	Manual            bool   `json:"manual"`
}

type mcpSearchConsoleSeriesResponse struct {
	DataSource string                        `json:"data_source"`
	Series     []mcpSearchConsoleMetricPoint `json:"series"`
}

type mcpSearchConsoleMetricPoint struct {
	Date            string  `json:"date"`
	Clicks          int     `json:"clicks"`
	Impressions     int     `json:"impressions"`
	CTR             float64 `json:"ctr"`
	AveragePosition float64 `json:"average_position"`
}

type docSearchOutput struct {
	Results []docSearchResult `json:"results"`
}

type docOutput struct {
	URL      string `json:"url"`
	Path     string `json:"path"`
	Markdown string `json:"markdown"`
}

type mcpSite struct {
	ID                string `json:"id"`
	UserID            string `json:"user_id"`
	Domain            string `json:"domain"`
	OwnerEmail        string `json:"owner_email,omitempty"`
	DataRetentionDays int    `json:"data_retention_days"`
	CreatedAt         string `json:"created_at"`
}

type mcpSiteStats struct {
	LiveVisitors int `json:"live_visitors"`

	TotalPageviews     int                 `json:"total_pageviews"`
	UniqueSessions     int                 `json:"unique_sessions"`
	BounceRate         float64             `json:"bounce_rate"`
	AvgSessionDuration float64             `json:"avg_session_duration"`
	PagesPerSession    float64             `json:"pages_per_session"`
	ChartData          []mcpChartDataPoint `json:"chart_data"`
	TopPages           []api.MetricStat    `json:"top_pages"`
	TopLandingPages    []api.MetricStat    `json:"top_landing_pages"`
	TopExitPages       []api.MetricStat    `json:"top_exit_pages"`
	TopReferrers       []api.MetricStat    `json:"top_referrers"`
	TopDevices         []api.MetricStat    `json:"top_devices"`
	TopCountries       []api.MetricStat    `json:"top_countries"`
	TopCities          []api.MetricStat    `json:"top_cities"`
	TopProviders       []api.MetricStat    `json:"top_providers"`
	TopASNs            []api.MetricStat    `json:"top_asns"`
	TopBrowsers        []api.MetricStat    `json:"top_browsers"`
	TopAIBots          []api.MetricStat    `json:"top_ai_bots"`
	TopAISources       []api.MetricStat    `json:"top_ai_sources"`
	TopLanguages       []api.MetricStat    `json:"top_languages"`
	TopUTMCampaigns    []api.MetricStat    `json:"top_utm_campaigns"`
	TopUTMContents     []api.MetricStat    `json:"top_utm_contents"`
	TopUTMMediums      []api.MetricStat    `json:"top_utm_mediums"`
	TopUTMSources      []api.MetricStat    `json:"top_utm_sources"`
	TopUTMTerms        []api.MetricStat    `json:"top_utm_terms"`
	AIBotHits          int                 `json:"ai_bot_hits"`
	AISourceVisits     int                 `json:"ai_source_visits"`
	UTMCampaignHits    int                 `json:"utm_campaign_hits"`
	UTMContentHits     int                 `json:"utm_content_hits"`
	UTMMediumHits      int                 `json:"utm_medium_hits"`
	UTMSourceHits      int                 `json:"utm_source_hits"`
	UTMTermHits        int                 `json:"utm_term_hits"`
	Goals              []mcpGoalStats      `json:"goals"`
	Comparison         *mcpComparisonStats `json:"comparison,omitempty"`
}

type mcpComparisonStats struct {
	TotalPageviews     int                 `json:"total_pageviews"`
	UniqueSessions     int                 `json:"unique_sessions"`
	BounceRate         float64             `json:"bounce_rate"`
	AvgSessionDuration float64             `json:"avg_session_duration"`
	PagesPerSession    float64             `json:"pages_per_session"`
	ChartData          []mcpChartDataPoint `json:"chart_data"`
	UTMCampaignHits    int                 `json:"utm_campaign_hits"`
	UTMContentHits     int                 `json:"utm_content_hits"`
	UTMMediumHits      int                 `json:"utm_medium_hits"`
	UTMSourceHits      int                 `json:"utm_source_hits"`
	UTMTermHits        int                 `json:"utm_term_hits"`
	Goals              []mcpGoalStats      `json:"goals"`
	TotalConversions   int                 `json:"total_conversions"`
}

type mcpChartDataPoint struct {
	Time      string `json:"time"`
	Pageviews int    `json:"pageviews"`
	Visitors  int    `json:"visitors"`
}

type mcpGoalStats struct {
	GoalID         string  `json:"goal_id"`
	Name           string  `json:"name"`
	Conversions    int     `json:"conversions"`
	ConversionRate float64 `json:"conversion_rate"`
}

type mcpAIFetchSeriesPoint struct {
	Time  string `json:"time"`
	Count int    `json:"count"`
}
