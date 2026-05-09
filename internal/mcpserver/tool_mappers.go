package mcpserver

import (
	"time"

	"hitkeep/internal/api"
)

func formatMCPTime(ts time.Time) string {
	return ts.UTC().Format(time.RFC3339)
}

func formatMCPDate(ts time.Time) string {
	return ts.UTC().Format(time.DateOnly)
}

func toMCPSites(sites []api.Site) []mcpSite {
	out := make([]mcpSite, 0, len(sites))
	for _, site := range sites {
		out = append(out, mcpSite{
			ID:                site.ID.String(),
			UserID:            site.UserID.String(),
			Domain:            site.Domain,
			OwnerEmail:        site.OwnerEmail,
			DataRetentionDays: site.DataRetentionDays,
			CreatedAt:         formatMCPTime(site.CreatedAt),
		})
	}
	return out
}

func toMCPSiteStats(stats *api.SiteStats) *mcpSiteStats {
	if stats == nil {
		return nil
	}
	return &mcpSiteStats{
		LiveVisitors:       stats.LiveVisitors,
		TotalPageviews:     stats.TotalPageviews,
		UniqueSessions:     stats.UniqueSessions,
		BounceRate:         stats.BounceRate,
		AvgSessionDuration: stats.AvgSessionDuration,
		PagesPerSession:    stats.PagesPerSession,
		ChartData:          toMCPChartData(stats.ChartData),
		TopPages:           stats.TopPages,
		TopLandingPages:    stats.TopLandingPages,
		TopExitPages:       stats.TopExitPages,
		TopReferrers:       stats.TopReferrers,
		TopDevices:         stats.TopDevices,
		TopCountries:       stats.TopCountries,
		TopBrowsers:        stats.TopBrowsers,
		TopAIBots:          stats.TopAIBots,
		TopAISources:       stats.TopAISources,
		TopLanguages:       stats.TopLanguages,
		TopUTMCampaigns:    stats.TopUTMCampaigns,
		TopUTMContents:     stats.TopUTMContents,
		TopUTMMediums:      stats.TopUTMMediums,
		TopUTMSources:      stats.TopUTMSources,
		TopUTMTerms:        stats.TopUTMTerms,
		AIBotHits:          stats.AIBotHits,
		AISourceVisits:     stats.AISourceVisits,
		UTMCampaignHits:    stats.UTMCampaignHits,
		UTMContentHits:     stats.UTMContentHits,
		UTMMediumHits:      stats.UTMMediumHits,
		UTMSourceHits:      stats.UTMSourceHits,
		UTMTermHits:        stats.UTMTermHits,
		Goals:              toMCPGoals(stats.Goals),
		Comparison:         toMCPComparisonStats(stats.Comparison),
	}
}

func toMCPComparisonStats(stats *api.ComparisonStats) *mcpComparisonStats {
	if stats == nil {
		return nil
	}
	return &mcpComparisonStats{
		TotalPageviews:     stats.TotalPageviews,
		UniqueSessions:     stats.UniqueSessions,
		BounceRate:         stats.BounceRate,
		AvgSessionDuration: stats.AvgSessionDuration,
		PagesPerSession:    stats.PagesPerSession,
		ChartData:          toMCPChartData(stats.ChartData),
		UTMCampaignHits:    stats.UTMCampaignHits,
		UTMContentHits:     stats.UTMContentHits,
		UTMMediumHits:      stats.UTMMediumHits,
		UTMSourceHits:      stats.UTMSourceHits,
		UTMTermHits:        stats.UTMTermHits,
		Goals:              toMCPGoals(stats.Goals),
		TotalConversions:   stats.TotalConversions,
	}
}

func toMCPChartData(points []api.ChartDataPoint) []mcpChartDataPoint {
	out := make([]mcpChartDataPoint, 0, len(points))
	for _, point := range points {
		out = append(out, mcpChartDataPoint{
			Time:      formatMCPTime(point.Time),
			Pageviews: point.Pageviews,
			Visitors:  point.Visitors,
		})
	}
	return out
}

func toMCPGoals(goals []api.GoalStats) []mcpGoalStats {
	out := make([]mcpGoalStats, 0, len(goals))
	for _, goal := range goals {
		out = append(out, mcpGoalStats{
			GoalID:         goal.GoalID.String(),
			Name:           goal.Name,
			Conversions:    goal.Conversions,
			ConversionRate: goal.ConversionRate,
		})
	}
	return out
}

func toMCPAIFetchSeries(points []api.AIFetchSeriesPoint) []mcpAIFetchSeriesPoint {
	out := make([]mcpAIFetchSeriesPoint, 0, len(points))
	for _, point := range points {
		out = append(out, mcpAIFetchSeriesPoint{
			Time:  formatMCPTime(point.Time),
			Count: point.Count,
		})
	}
	return out
}

func toMCPSearchConsoleSeries(series api.SearchConsoleSeriesResponse) *mcpSearchConsoleSeriesResponse {
	out := &mcpSearchConsoleSeriesResponse{
		DataSource: series.DataSource,
		Series:     make([]mcpSearchConsoleMetricPoint, 0, len(series.Series)),
	}
	for _, point := range series.Series {
		out.Series = append(out.Series, mcpSearchConsoleMetricPoint{
			Date:            formatMCPDate(time.Time(point.Date)),
			Clicks:          point.Clicks,
			Impressions:     point.Impressions,
			CTR:             point.CTR,
			AveragePosition: point.AveragePosition,
		})
	}
	return out
}

func isAllowedFilter(filterType string) bool {
	switch filterType {
	case "path", "hostname", "referrer", "referrer_host", "device", "country", "browser", "language", "utm_campaign", "utm_content", "utm_medium", "utm_source", "utm_term":
		return true
	default:
		return false
	}
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

func limitSlice[T any](input []T, limit int) []T {
	if len(input) <= limit {
		return input
	}
	return input[:limit]
}

func mcpHelpMarkdown() string {
	return `# HitKeep MCP

HitKeep's MCP server is read-only. It exposes aggregate analytics and official HitKeep documentation to MCP clients.

## Authentication

Create a scoped personal or team API client in HitKeep, then connect with:

` + "```http" + `
Authorization: Bearer <hitkeep-api-client-token>
` + "```" + `

Team API clients are recommended for shared assistants and automations.

## Analytics Scope

Analytics tools require a ` + "`site_id`" + ` that the API client can view. The server returns aggregate KPIs, event summaries, ecommerce summaries, AI visibility reports, and saved Opportunities recommendations. It does not expose raw hit exports or write/admin actions.

Date inputs use RFC3339 timestamps. If omitted, tools default to the last 30 days. Filters support path, hostname, referrer, referrer_host, device, country, browser, language, and UTM fields.

Opportunities are returned as final customer-visible records with localization keys, interpolation params, cited evidence, detector metadata, and status. MCP does not expose raw prompts, raw provider payloads, unrestricted tool calls, or visitor-level rows.

Search Console tools read imported Google Search Console facts stored in HitKeep. They do not call Google live, refresh OAuth credentials, or trigger syncs. Use ` + "`hitkeep_get_search_console_status`" + ` to check whether a site is mapped, synced, stale, failed, or needs attention before interpreting empty reports. ` + "`hitkeep_get_search_console`" + ` returns overview and series by default. Query, page, country, and device rows are aggregate imported provider data and are returned only when explicitly requested. Report warnings flag missing imported data, failed or needs-attention syncs, and requested ranges outside the imported date range.

## Docs Scope

Docs tools fetch official HitKeep docs as markdown using HTTP content negotiation. Docs requests are limited to the configured HitKeep docs origin.
`
}
