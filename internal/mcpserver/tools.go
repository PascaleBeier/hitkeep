package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"hitkeep/internal/api"
)

func (s *service) registerTools(server *mcp.Server) {
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(false)}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "hitkeep_list_sites",
		Title:       "List HitKeep Sites",
		Description: "List HitKeep sites visible to the bearer API client.",
		Annotations: readOnly,
	}, s.listSites)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "hitkeep_get_site_overview",
		Title:       "Get HitKeep Site Overview",
		Description: "Read aggregate traffic KPIs, top dimensions, goals, chart data, and optional comparison for one site.",
		Annotations: readOnly,
	}, s.getSiteOverview)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "hitkeep_get_event_names",
		Title:       "Get HitKeep Event Names",
		Description: "List event names tracked for one site in a date range.",
		Annotations: readOnly,
	}, s.getEventNames)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "hitkeep_get_event_breakdown",
		Title:       "Get HitKeep Event Breakdown",
		Description: "Read a count breakdown for one event property.",
		Annotations: readOnly,
	}, s.getEventBreakdown)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "hitkeep_get_ecommerce",
		Title:       "Get HitKeep Ecommerce Analytics",
		Description: "Read ecommerce summary, top products, and source stats for one site.",
		Annotations: readOnly,
	}, s.getEcommerce)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "hitkeep_get_ai_visibility",
		Title:       "Get HitKeep AI Visibility",
		Description: "Read AI crawler fetch overview, timeseries, and optional fetch-to-visit correlation for one site.",
		Annotations: readOnly,
	}, s.getAIVisibility)
	if s.docs != nil {
		docsReadOnly := &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(true)}
		mcp.AddTool(server, &mcp.Tool{
			Name:        "hitkeep_search_docs",
			Title:       "Search HitKeep Docs",
			Description: "Search official HitKeep documentation using the LLM catalog and cached markdown pages.",
			Annotations: docsReadOnly,
		}, s.searchDocs)
		mcp.AddTool(server, &mcp.Tool{
			Name:        "hitkeep_get_doc",
			Title:       "Get HitKeep Doc",
			Description: "Fetch an official HitKeep docs page as markdown.",
			Annotations: docsReadOnly,
		}, s.getDoc)
		mcp.AddTool(server, &mcp.Tool{
			Name:        "hitkeep_get_api_reference",
			Title:       "Get HitKeep API Reference",
			Description: "Fetch an official HitKeep REST API reference page as markdown by path or operation slug.",
			Annotations: docsReadOnly,
		}, s.getAPIReference)
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "hitkeep_get_mcp_help",
		Title:       "Get HitKeep MCP Help",
		Description: "Return local MCP usage guidance, token setup, privacy boundaries, date ranges, and filter syntax.",
		Annotations: readOnly,
	}, s.getMCPHelp)
}

func (s *service) listSites(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, listSitesOutput, error) {
	authz, err := apiAuth(ctx)
	if err != nil {
		return nil, listSitesOutput{}, err
	}

	var sites []api.Site
	switch {
	case authz.UserID != uuid.Nil:
		sites, err = s.store.GetSites(ctx, authz.UserID)
	case authz.TenantID != uuid.Nil:
		sites, err = s.store.ListSitesForTenant(ctx, authz.TenantID)
	default:
		return nil, listSitesOutput{}, errors.New("unauthorized")
	}
	if err != nil {
		return nil, listSitesOutput{}, err
	}

	filtered := make([]api.Site, 0, len(sites))
	for _, site := range sites {
		if _, ok := authz.SiteRoles[site.ID]; ok {
			filtered = append(filtered, site)
		}
	}
	return nil, listSitesOutput{Sites: toMCPSites(filtered)}, nil
}

func (s *service) getSiteOverview(ctx context.Context, _ *mcp.CallToolRequest, input siteOverviewInput) (*mcp.CallToolResult, siteOverviewOutput, error) {
	siteID, start, end, err := s.parseSiteRange(input.SiteID, rangeInput{From: input.From, To: input.To})
	if err != nil {
		return nil, siteOverviewOutput{}, err
	}
	authz, err := s.requireSiteView(ctx, siteID)
	if err != nil {
		return nil, siteOverviewOutput{}, err
	}
	filters, err := parseFilters(input.Filters)
	if err != nil {
		return nil, siteOverviewOutput{}, err
	}

	params := api.AnalyticsParams{
		SiteID:  siteID,
		UserID:  authz.UserID,
		Start:   start,
		End:     end,
		Filters: filters,
	}
	if input.CompareFrom != "" || input.CompareTo != "" {
		compareStart, compareEnd, err := parseExplicitRange(input.CompareFrom, input.CompareTo, s.conf.MCPMaxRangeDays)
		if err != nil {
			return nil, siteOverviewOutput{}, err
		}
		params.CompareStart = compareStart
		params.CompareEnd = compareEnd
	}

	analyticsStore, err := s.analyticsStore(ctx, siteID)
	if err != nil {
		return nil, siteOverviewOutput{}, err
	}
	stats, err := analyticsStore.GetSiteStats(ctx, params)
	if err != nil {
		return nil, siteOverviewOutput{}, err
	}
	return nil, siteOverviewOutput{SiteID: siteID.String(), From: formatMCPTime(start), To: formatMCPTime(end), Stats: toMCPSiteStats(stats)}, nil
}

func (s *service) getEventNames(ctx context.Context, _ *mcp.CallToolRequest, input siteRangeInput) (*mcp.CallToolResult, eventNamesOutput, error) {
	siteID, start, end, err := s.parseSiteRange(input.SiteID, input.rangeInput)
	if err != nil {
		return nil, eventNamesOutput{}, err
	}
	if _, err := s.requireSiteView(ctx, siteID); err != nil {
		return nil, eventNamesOutput{}, err
	}
	analyticsStore, err := s.analyticsStore(ctx, siteID)
	if err != nil {
		return nil, eventNamesOutput{}, err
	}
	names, err := analyticsStore.GetEventNames(ctx, api.EventNamesParams{SiteID: siteID, Start: start, End: end})
	if err != nil {
		return nil, eventNamesOutput{}, err
	}
	return nil, eventNamesOutput{SiteID: siteID.String(), From: formatMCPTime(start), To: formatMCPTime(end), Names: names}, nil
}

func (s *service) getEventBreakdown(ctx context.Context, _ *mcp.CallToolRequest, input eventBreakdownInput) (*mcp.CallToolResult, eventBreakdownOutput, error) {
	siteID, start, end, err := s.parseSiteRange(input.SiteID, input.rangeInput)
	if err != nil {
		return nil, eventBreakdownOutput{}, err
	}
	if _, err := s.requireSiteView(ctx, siteID); err != nil {
		return nil, eventBreakdownOutput{}, err
	}
	eventName := strings.TrimSpace(input.EventName)
	propertyKey := strings.TrimSpace(input.PropertyKey)
	if eventName == "" || propertyKey == "" {
		return nil, eventBreakdownOutput{}, errors.New("event_name and property_key are required")
	}

	analyticsStore, err := s.analyticsStore(ctx, siteID)
	if err != nil {
		return nil, eventBreakdownOutput{}, err
	}
	breakdown, err := analyticsStore.GetEventPropertyBreakdown(ctx, api.EventBreakdownParams{
		SiteID: siteID, Start: start, End: end, EventName: eventName, PropertyKey: propertyKey,
	})
	if err != nil {
		return nil, eventBreakdownOutput{}, err
	}
	breakdown = limitSlice(breakdown, normalizeLimit(input.Limit))
	return nil, eventBreakdownOutput{SiteID: siteID.String(), From: formatMCPTime(start), To: formatMCPTime(end), Breakdown: breakdown}, nil
}

func (s *service) getEcommerce(ctx context.Context, _ *mcp.CallToolRequest, input ecommerceInput) (*mcp.CallToolResult, ecommerceOutput, error) {
	siteID, start, end, err := s.parseSiteRange(input.SiteID, input.rangeInput)
	if err != nil {
		return nil, ecommerceOutput{}, err
	}
	if _, err := s.requireSiteView(ctx, siteID); err != nil {
		return nil, ecommerceOutput{}, err
	}
	filters, err := parseFilters(input.Filters)
	if err != nil {
		return nil, ecommerceOutput{}, err
	}
	params := api.EcommerceParams{
		SiteID: siteID, Start: start, End: end, Filters: filters,
		ItemID: strings.TrimSpace(input.ItemID), ItemName: strings.TrimSpace(input.ItemName),
		Limit: normalizeLimit(input.Limit),
	}
	analyticsStore, err := s.analyticsStore(ctx, siteID)
	if err != nil {
		return nil, ecommerceOutput{}, err
	}
	summary, err := analyticsStore.GetEcommerceSummary(ctx, params)
	if err != nil {
		return nil, ecommerceOutput{}, err
	}
	products, err := analyticsStore.GetEcommerceTopProducts(ctx, params)
	if err != nil {
		return nil, ecommerceOutput{}, err
	}
	sources, err := analyticsStore.GetEcommerceSources(ctx, params)
	if err != nil {
		return nil, ecommerceOutput{}, err
	}
	return nil, ecommerceOutput{SiteID: siteID.String(), From: formatMCPTime(start), To: formatMCPTime(end), Summary: summary, Products: products, Sources: sources}, nil
}

func (s *service) getAIVisibility(ctx context.Context, _ *mcp.CallToolRequest, input aiVisibilityInput) (*mcp.CallToolResult, aiVisibilityOutput, error) {
	siteID, start, end, err := s.parseSiteRange(input.SiteID, input.rangeInput)
	if err != nil {
		return nil, aiVisibilityOutput{}, err
	}
	if _, err := s.requireSiteView(ctx, siteID); err != nil {
		return nil, aiVisibilityOutput{}, err
	}
	params := api.AIFetchQueryParams{
		SiteID: siteID, Start: start, End: end,
		AssistantName:   strings.TrimSpace(input.AssistantName),
		AssistantFamily: strings.TrimSpace(input.AssistantFamily),
		ResourceType:    strings.TrimSpace(input.ResourceType),
	}
	analyticsStore, err := s.analyticsStore(ctx, siteID)
	if err != nil {
		return nil, aiVisibilityOutput{}, err
	}
	overview, err := analyticsStore.GetAIFetchOverview(ctx, params)
	if err != nil {
		return nil, aiVisibilityOutput{}, err
	}
	timeseries, err := analyticsStore.GetAIFetchTimeseries(ctx, params)
	if err != nil {
		return nil, aiVisibilityOutput{}, err
	}
	output := aiVisibilityOutput{SiteID: siteID.String(), From: formatMCPTime(start), To: formatMCPTime(end), Overview: overview, Timeseries: toMCPAIFetchSeries(timeseries)}
	if input.IncludeCorrelation {
		windowDays := input.WindowDays
		if windowDays <= 0 {
			windowDays = 30
		}
		if windowDays > 90 {
			windowDays = 90
		}
		correlation, err := analyticsStore.GetAIFetchCorrelation(ctx, api.AIFetchCorrelationParams{
			SiteID: siteID, Start: start, End: end,
			AssistantName: params.AssistantName, AssistantFamily: params.AssistantFamily, ResourceType: params.ResourceType,
			WindowDays: windowDays,
		})
		if err != nil {
			return nil, aiVisibilityOutput{}, err
		}
		output.Correlation = correlation
	}
	return nil, output, nil
}

func (s *service) searchDocs(ctx context.Context, _ *mcp.CallToolRequest, input docQueryInput) (*mcp.CallToolResult, docSearchOutput, error) {
	if s.docs == nil {
		return nil, docSearchOutput{}, errors.New("MCP docs tools are disabled")
	}
	results, err := s.docs.Search(ctx, input.Query, normalizeLimit(input.Limit))
	return nil, docSearchOutput{Results: results}, err
}

func (s *service) getDoc(ctx context.Context, _ *mcp.CallToolRequest, input docPathInput) (*mcp.CallToolResult, docOutput, error) {
	return s.fetchDoc(ctx, input.Path)
}

func (s *service) getAPIReference(ctx context.Context, _ *mcp.CallToolRequest, input apiReferenceInput) (*mcp.CallToolResult, docOutput, error) {
	path := strings.TrimSpace(input.PathOrOperation)
	if path == "" {
		return nil, docOutput{}, errors.New("path_or_operation is required")
	}
	if !strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "http://") && !strings.HasPrefix(path, "https://") {
		path = "/api/operations/" + strings.Trim(path, "/") + "/"
	}
	return s.fetchDoc(ctx, path)
}

func (s *service) getMCPHelp(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, docOutput, error) {
	markdown := mcpHelpMarkdown()
	return nil, docOutput{URL: "hitkeep://help/mcp", Path: "hitkeep://help/mcp", Markdown: markdown}, nil
}

func (s *service) fetchDoc(ctx context.Context, path string) (*mcp.CallToolResult, docOutput, error) {
	if s.docs == nil {
		return nil, docOutput{}, errors.New("MCP docs tools are disabled")
	}
	page, err := s.docs.GetMarkdown(ctx, path)
	if err != nil {
		return nil, docOutput{}, err
	}
	return nil, docOutput(page), nil
}

func (s *service) parseSiteRange(rawSiteID string, input rangeInput) (uuid.UUID, time.Time, time.Time, error) {
	siteID, err := uuid.Parse(strings.TrimSpace(rawSiteID))
	if err != nil {
		return uuid.Nil, time.Time{}, time.Time{}, errors.New("invalid site_id")
	}
	start, end, err := parseRange(input, s.conf.MCPMaxRangeDays)
	return siteID, start, end, err
}

func parseRange(input rangeInput, maxDays int) (time.Time, time.Time, error) {
	end := time.Now().UTC()
	start := end.AddDate(0, 0, -defaultRangeDays)
	if strings.TrimSpace(input.To) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(input.To))
		if err != nil {
			return time.Time{}, time.Time{}, errors.New("invalid to timestamp, expected RFC3339")
		}
		end = parsed
	}
	if strings.TrimSpace(input.From) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(input.From))
		if err != nil {
			return time.Time{}, time.Time{}, errors.New("invalid from timestamp, expected RFC3339")
		}
		start = parsed
	} else if strings.TrimSpace(input.To) != "" {
		start = end.AddDate(0, 0, -defaultRangeDays)
	}
	if !end.After(start) {
		return time.Time{}, time.Time{}, errors.New("to must be after from")
	}
	if maxDays <= 0 {
		maxDays = 366
	}
	if end.Sub(start) > time.Duration(maxDays)*24*time.Hour {
		return time.Time{}, time.Time{}, fmt.Errorf("date range exceeds %d days", maxDays)
	}
	return start.UTC(), end.UTC(), nil
}

func parseExplicitRange(from, to string, maxDays int) (time.Time, time.Time, error) {
	if strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" {
		return time.Time{}, time.Time{}, errors.New("compare_from and compare_to are required together")
	}
	start, err := time.Parse(time.RFC3339, strings.TrimSpace(from))
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("invalid compare_from timestamp, expected RFC3339")
	}
	end, err := time.Parse(time.RFC3339, strings.TrimSpace(to))
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("invalid compare_to timestamp, expected RFC3339")
	}
	if !end.After(start) {
		return time.Time{}, time.Time{}, errors.New("compare_to must be after compare_from")
	}
	if maxDays <= 0 {
		maxDays = 366
	}
	if end.Sub(start) > time.Duration(maxDays)*24*time.Hour {
		return time.Time{}, time.Time{}, fmt.Errorf("comparison date range exceeds %d days", maxDays)
	}
	return start.UTC(), end.UTC(), nil
}

func parseFilters(inputs []filterInput) ([]api.Filter, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	filters := make([]api.Filter, 0, len(inputs))
	for _, input := range inputs {
		filterType := strings.ToLower(strings.TrimSpace(input.Type))
		filterValue := strings.TrimSpace(input.Value)
		if filterType == "" || filterValue == "" {
			return nil, errors.New("filter type and value are required together")
		}
		if !isAllowedFilter(filterType) {
			return nil, fmt.Errorf("invalid filter type %q", filterType)
		}
		filters = append(filters, api.Filter{Type: filterType, Value: filterValue})
	}
	return filters, nil
}
