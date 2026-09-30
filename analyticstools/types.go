package analyticstools

import (
	"time"

	"hitkeep/api"
	"hitkeep/database"
)

// Output types spell IDs and times as strings: the MCP SDK derives output
// schemas from Go types, and uuid.UUID would otherwise publish as a byte array.

type SiteOverviewOutput struct {
	SiteID string     `json:"site_id"`
	From   string     `json:"from"`
	To     string     `json:"to"`
	Stats  *siteStats `json:"stats"`
}

type siteStats struct {
	LiveVisitors int `json:"live_visitors"`

	TotalPageviews      int                         `json:"total_pageviews"`
	UniqueSessions      int                         `json:"unique_sessions"`
	BounceRate          float64                     `json:"bounce_rate"`
	AvgSessionDuration  float64                     `json:"avg_session_duration"`
	PagesPerSession     float64                     `json:"pages_per_session"`
	ChartData           []chartDataPoint            `json:"chart_data"`
	TopPages            []api.MetricStat            `json:"top_pages"`
	TopLandingPages     []api.MetricStat            `json:"top_landing_pages"`
	TopExitPages        []api.MetricStat            `json:"top_exit_pages"`
	TopReferrers        []api.MetricStat            `json:"top_referrers"`
	TopDevices          []api.MetricStat            `json:"top_devices"`
	TopCountries        []api.MetricStat            `json:"top_countries"`
	TopCities           []api.MetricStat            `json:"top_cities"`
	TopProviders        []api.MetricStat            `json:"top_providers"`
	TopASNs             []api.MetricStat            `json:"top_asns"`
	TopBrowsers         []api.MetricStat            `json:"top_browsers"`
	TopAIBots           []api.MetricStat            `json:"top_ai_bots"`
	TopAIBotCategories  []api.MetricStat            `json:"top_ai_bot_categories"`
	TopAIBotsByCategory map[string][]api.MetricStat `json:"top_ai_bots_by_category,omitempty"`
	TopAISources        []api.MetricStat            `json:"top_ai_sources"`
	TopLanguages        []api.MetricStat            `json:"top_languages"`
	TopUTMCampaigns     []api.MetricStat            `json:"top_utm_campaigns"`
	TopUTMContents      []api.MetricStat            `json:"top_utm_contents"`
	TopUTMMediums       []api.MetricStat            `json:"top_utm_mediums"`
	TopUTMSources       []api.MetricStat            `json:"top_utm_sources"`
	TopUTMTerms         []api.MetricStat            `json:"top_utm_terms"`
	AIBotHits           int                         `json:"ai_bot_hits"`
	AISourceVisits      int                         `json:"ai_source_visits"`
	UTMCampaignHits     int                         `json:"utm_campaign_hits"`
	UTMContentHits      int                         `json:"utm_content_hits"`
	UTMMediumHits       int                         `json:"utm_medium_hits"`
	UTMSourceHits       int                         `json:"utm_source_hits"`
	UTMTermHits         int                         `json:"utm_term_hits"`
	Goals               []goalStats                 `json:"goals"`
	Funnels             []funnel                    `json:"funnels"`
	Comparison          *comparisonStats            `json:"comparison,omitempty"`
}

type comparisonStats struct {
	TotalPageviews     int              `json:"total_pageviews"`
	UniqueSessions     int              `json:"unique_sessions"`
	BounceRate         float64          `json:"bounce_rate"`
	AvgSessionDuration float64          `json:"avg_session_duration"`
	PagesPerSession    float64          `json:"pages_per_session"`
	ChartData          []chartDataPoint `json:"chart_data"`
	AIBotHits          int              `json:"ai_bot_hits"`
	AISourceVisits     int              `json:"ai_source_visits"`
	UTMCampaignHits    int              `json:"utm_campaign_hits"`
	UTMContentHits     int              `json:"utm_content_hits"`
	UTMMediumHits      int              `json:"utm_medium_hits"`
	UTMSourceHits      int              `json:"utm_source_hits"`
	UTMTermHits        int              `json:"utm_term_hits"`
	Goals              []goalStats      `json:"goals"`
	TotalConversions   int              `json:"total_conversions"`
}

type chartDataPoint struct {
	Time      string `json:"time"`
	Pageviews int    `json:"pageviews"`
	Visitors  int    `json:"visitors"`
}

type goalStats struct {
	GoalID         string  `json:"goal_id"`
	Name           string  `json:"name"`
	Conversions    int     `json:"conversions"`
	ConversionRate float64 `json:"conversion_rate"`
}

type funnelStep struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type funnel struct {
	ID        string       `json:"id"`
	SiteID    string       `json:"site_id"`
	Name      string       `json:"name"`
	Steps     []funnelStep `json:"steps"`
	CreatedAt string       `json:"created_at"`
}

type EventNamesOutput struct {
	SiteID string   `json:"site_id"`
	From   string   `json:"from"`
	To     string   `json:"to"`
	Names  []string `json:"names"`
}

type EventBreakdownOutput struct {
	SiteID    string           `json:"site_id"`
	From      string           `json:"from"`
	To        string           `json:"to"`
	Breakdown []api.MetricStat `json:"breakdown"`
}

type AnnotationsOutput struct {
	SiteID      string       `json:"site_id"`
	From        string       `json:"from"`
	To          string       `json:"to"`
	Annotations []annotation `json:"annotations"`
}

type annotation struct {
	ID       string `json:"id"`
	StartsAt string `json:"starts_at"`
	EndsAt   string `json:"ends_at,omitempty"`
	Body     string `json:"body"`
}

type EcommerceOutput struct {
	SiteID   string                     `json:"site_id"`
	From     string                     `json:"from"`
	To       string                     `json:"to"`
	Summary  *api.EcommerceSummary      `json:"summary"`
	Series   []api.EcommerceSeriesPoint `json:"series,omitempty"`
	Products []api.EcommerceProductStat `json:"products"`
	Sources  []api.EcommerceSourceStat  `json:"sources"`
}

type WebVitalsOutput struct {
	SiteID             string                      `json:"site_id"`
	From               string                      `json:"from"`
	To                 string                      `json:"to"`
	Metric             api.WebVitalMetric          `json:"metric,omitempty"`
	Summary            []api.WebVitalSummaryMetric `json:"summary"`
	Timeseries         []api.WebVitalSeriesPoint   `json:"timeseries,omitempty"`
	Pages              []api.WebVitalPageRow       `json:"pages,omitempty"`
	BreakdownDimension api.WebVitalDimension       `json:"breakdown_dimension,omitempty"`
	Breakdown          []api.WebVitalDimensionRow  `json:"breakdown,omitempty"`
}

type AiVisibilityOutput struct {
	SiteID      string                        `json:"site_id"`
	From        string                        `json:"from"`
	To          string                        `json:"to"`
	Overview    *api.AIFetchOverview          `json:"overview"`
	Timeseries  []aiFetchSeriesPoint          `json:"timeseries,omitempty"`
	Correlation *api.AIFetchCorrelationReport `json:"correlation,omitempty"`
}

type aiFetchSeriesPoint struct {
	Time  string `json:"time"`
	Count int    `json:"count"`
}

type FunnelStatsOutput struct {
	SiteID string           `json:"site_id"`
	From   string           `json:"from"`
	To     string           `json:"to"`
	Stats  *api.FunnelStats `json:"stats"`
}

type QrCampaignsOutput struct {
	SiteID    string       `json:"site_id"`
	From      string       `json:"from"`
	To        string       `json:"to"`
	Campaigns []qrCampaign `json:"campaigns"`
}

type qrCampaign struct {
	ID         string                      `json:"id"`
	Name       string                      `json:"name"`
	CreatedAt  string                      `json:"created_at"`
	OpenCount  int                         `json:"open_count"`
	Pageviews  int                         `json:"pageviews"`
	Visitors   int                         `json:"visitors"`
	Timeseries []api.QRCodeOpenSeriesPoint `json:"timeseries,omitempty"`
}

type SearchConsoleStatusOutput struct {
	SiteID                  string                   `json:"site_id"`
	TeamID                  string                   `json:"team_id,omitempty"`
	Mapped                  bool                     `json:"mapped"`
	PropertyURI             string                   `json:"property_uri,omitempty"`
	PropertyPermissionLevel string                   `json:"property_permission_level,omitempty"`
	SyncStatus              *searchConsoleSyncStatus `json:"sync_status,omitempty"`
	DataAvailable           bool                     `json:"data_available"`
	AvailableFrom           string                   `json:"available_from,omitempty"`
	AvailableTo             string                   `json:"available_to,omitempty"`
	NeedsAttention          bool                     `json:"needs_attention"`
	Reason                  string                   `json:"reason"`
}

type SearchConsoleOutput struct {
	SiteID      string                              `json:"site_id"`
	From        string                              `json:"from"`
	To          string                              `json:"to"`
	PropertyURI string                              `json:"property_uri,omitempty"`
	SyncStatus  *searchConsoleSyncStatus            `json:"sync_status,omitempty"`
	Overview    *api.SearchConsoleOverview          `json:"overview,omitempty"`
	Series      *searchConsoleSeries                `json:"series,omitempty"`
	Queries     *api.SearchConsoleDimensionResponse `json:"queries,omitempty"`
	Pages       *api.SearchConsoleDimensionResponse `json:"pages,omitempty"`
	Country     *api.SearchConsoleDimensionResponse `json:"country,omitempty"`
	Device      *api.SearchConsoleDimensionResponse `json:"device,omitempty"`
	Warnings    []string                            `json:"warnings"`
}

type searchConsoleSyncStatus struct {
	State             string `json:"state"`
	ImportedStartDate string `json:"imported_start_date,omitempty"`
	ImportedEndDate   string `json:"imported_end_date,omitempty"`
	LastSuccessAt     string `json:"last_success_at,omitempty"`
	LastAttemptAt     string `json:"last_attempt_at,omitempty"`
	LastErrorCategory string `json:"last_error_category,omitempty"`
	NextRetryAt       string `json:"next_retry_at,omitempty"`
	Manual            bool   `json:"manual"`
}

type searchConsoleSeries struct {
	DataSource string                     `json:"data_source"`
	Series     []searchConsoleMetricPoint `json:"series"`
}

type searchConsoleMetricPoint struct {
	Date            string  `json:"date"`
	Clicks          int     `json:"clicks"`
	Impressions     int     `json:"impressions"`
	CTR             float64 `json:"ctr"`
	AveragePosition float64 `json:"average_position"`
}

func toSiteStats(stats *api.SiteStats) *siteStats {
	if stats == nil {
		return nil
	}
	return &siteStats{
		LiveVisitors:        stats.LiveVisitors,
		TotalPageviews:      stats.TotalPageviews,
		UniqueSessions:      stats.UniqueSessions,
		BounceRate:          stats.BounceRate,
		AvgSessionDuration:  stats.AvgSessionDuration,
		PagesPerSession:     stats.PagesPerSession,
		ChartData:           toChartData(stats.ChartData),
		TopPages:            stats.TopPages,
		TopLandingPages:     stats.TopLandingPages,
		TopExitPages:        stats.TopExitPages,
		TopReferrers:        stats.TopReferrers,
		TopDevices:          stats.TopDevices,
		TopCountries:        stats.TopCountries,
		TopCities:           stats.TopCities,
		TopProviders:        stats.TopProviders,
		TopASNs:             stats.TopASNs,
		TopBrowsers:         stats.TopBrowsers,
		TopAIBots:           stats.TopAIBots,
		TopAIBotCategories:  stats.TopAIBotCategories,
		TopAIBotsByCategory: stats.TopAIBotsByCategory,
		TopAISources:        stats.TopAISources,
		TopLanguages:        stats.TopLanguages,
		TopUTMCampaigns:     stats.TopUTMCampaigns,
		TopUTMContents:      stats.TopUTMContents,
		TopUTMMediums:       stats.TopUTMMediums,
		TopUTMSources:       stats.TopUTMSources,
		TopUTMTerms:         stats.TopUTMTerms,
		AIBotHits:           stats.AIBotHits,
		AISourceVisits:      stats.AISourceVisits,
		UTMCampaignHits:     stats.UTMCampaignHits,
		UTMContentHits:      stats.UTMContentHits,
		UTMMediumHits:       stats.UTMMediumHits,
		UTMSourceHits:       stats.UTMSourceHits,
		UTMTermHits:         stats.UTMTermHits,
		Goals:               toGoals(stats.Goals),
		Funnels:             toFunnels(stats.Funnels),
		Comparison:          toComparisonStats(stats.Comparison),
	}
}

func toFunnels(funnels []api.Funnel) []funnel {
	out := make([]funnel, 0, len(funnels))
	for _, f := range funnels {
		steps := make([]funnelStep, 0, len(f.Steps))
		for _, step := range f.Steps {
			steps = append(steps, funnelStep{Type: step.Type, Value: step.Value})
		}
		out = append(out, funnel{
			ID:        f.ID.String(),
			SiteID:    f.SiteID.String(),
			Name:      f.Name,
			Steps:     steps,
			CreatedAt: formatTime(f.CreatedAt),
		})
	}
	return out
}

func toComparisonStats(stats *api.ComparisonStats) *comparisonStats {
	if stats == nil {
		return nil
	}
	return &comparisonStats{
		TotalPageviews:     stats.TotalPageviews,
		UniqueSessions:     stats.UniqueSessions,
		BounceRate:         stats.BounceRate,
		AvgSessionDuration: stats.AvgSessionDuration,
		PagesPerSession:    stats.PagesPerSession,
		ChartData:          toChartData(stats.ChartData),
		AIBotHits:          stats.AIBotHits,
		AISourceVisits:     stats.AISourceVisits,
		UTMCampaignHits:    stats.UTMCampaignHits,
		UTMContentHits:     stats.UTMContentHits,
		UTMMediumHits:      stats.UTMMediumHits,
		UTMSourceHits:      stats.UTMSourceHits,
		UTMTermHits:        stats.UTMTermHits,
		Goals:              toGoals(stats.Goals),
		TotalConversions:   stats.TotalConversions,
	}
}

func toChartData(points []api.ChartDataPoint) []chartDataPoint {
	out := make([]chartDataPoint, 0, len(points))
	for _, point := range points {
		out = append(out, chartDataPoint{Time: formatTime(point.Time), Pageviews: point.Pageviews, Visitors: point.Visitors})
	}
	return out
}

func toGoals(goals []api.GoalStats) []goalStats {
	out := make([]goalStats, 0, len(goals))
	for _, goal := range goals {
		out = append(out, goalStats{GoalID: goal.GoalID.String(), Name: goal.Name, Conversions: goal.Conversions, ConversionRate: goal.ConversionRate})
	}
	return out
}

func toAnnotations(annotations []api.Annotation) []annotation {
	out := make([]annotation, 0, len(annotations))
	for _, a := range annotations {
		note := annotation{ID: a.ID.String(), StartsAt: formatTime(a.StartsAt), Body: a.Body}
		if a.EndsAt != nil {
			note.EndsAt = formatTime(*a.EndsAt)
		}
		out = append(out, note)
	}
	return out
}

func toAIFetchSeries(points []api.AIFetchSeriesPoint) []aiFetchSeriesPoint {
	out := make([]aiFetchSeriesPoint, 0, len(points))
	for _, point := range points {
		out = append(out, aiFetchSeriesPoint{Time: formatTime(point.Time), Count: point.Count})
	}
	return out
}

func toSearchConsoleSeries(series api.SearchConsoleSeriesResponse) *searchConsoleSeries {
	out := &searchConsoleSeries{
		DataSource: series.DataSource,
		Series:     make([]searchConsoleMetricPoint, 0, len(series.Series)),
	}
	for _, point := range series.Series {
		out.Series = append(out.Series, searchConsoleMetricPoint{
			Date:            formatDate(time.Time(point.Date)),
			Clicks:          point.Clicks,
			Impressions:     point.Impressions,
			CTR:             point.CTR,
			AveragePosition: point.AveragePosition,
		})
	}
	return out
}

func toSearchConsoleSyncStatus(state *database.GoogleSearchConsoleSyncState) *searchConsoleSyncStatus {
	if state == nil {
		return nil
	}
	return &searchConsoleSyncStatus{
		State:             state.State,
		ImportedStartDate: formatOptional(state.ImportedStartDate, formatDate),
		ImportedEndDate:   formatOptional(state.ImportedEndDate, formatDate),
		LastSuccessAt:     formatOptional(state.LastSuccessAt, formatTime),
		LastAttemptAt:     formatOptional(state.LastAttemptAt, formatTime),
		LastErrorCategory: state.LastErrorCategory,
		NextRetryAt:       formatOptional(state.NextRetryAt, formatTime),
		Manual:            state.Manual,
	}
}

func formatOptional(ts *time.Time, format func(time.Time) string) string {
	if ts == nil {
		return ""
	}
	return format(*ts)
}
