package analyticstools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	goaisdk "github.com/zendev-sh/goai"

	"hitkeep/analyticscatalog"
	"hitkeep/internal/api"
	"hitkeep/internal/database"
	json "hitkeep/internal/jsonapi"
)

type Config struct {
	Analytics     *database.Store
	SiteID        uuid.UUID
	UserID        uuid.UUID
	From          time.Time
	To            time.Time
	Filters       []api.Filter
	BeforeExecute func(context.Context) error
}

type Bridge struct {
	config Config
}

func NewBridge(config Config) Bridge {
	return Bridge{config: config}
}

type SiteOverviewInput struct {
	CompareStart time.Time
	CompareEnd   time.Time
}

type EventBreakdownInput struct {
	EventName   string
	PropertyKey string
	Limit       int
}

type EcommerceInput struct {
	ItemID        string
	ItemName      string
	Limit         int
	IncludeSeries bool
}

type EcommercePayload struct {
	Summary  *api.EcommerceSummary      `json:"summary"`
	Series   []api.EcommerceSeriesPoint `json:"series"`
	Products []api.EcommerceProductStat `json:"products"`
	Sources  []api.EcommerceSourceStat  `json:"sources"`
}

type WebVitalsInput struct {
	Metric             api.WebVitalMetric
	Path               string
	Rating             api.WebVitalRating
	IncludeTimeseries  bool
	IncludePages       bool
	BreakdownDimension api.WebVitalDimension
	Limit              int
}

type WebVitalsPayload struct {
	Metric             api.WebVitalMetric
	Summary            []api.WebVitalSummaryMetric
	Timeseries         []api.WebVitalSeriesPoint
	Pages              []api.WebVitalPageRow
	BreakdownDimension api.WebVitalDimension
	Breakdown          []api.WebVitalDimensionRow
}

type AIVisibilityInput struct {
	AssistantName      string
	AssistantFamily    string
	ResourceType       string
	Path               string
	IncludeTimeseries  bool
	IncludeCorrelation bool
	WindowDays         int
}

type AIVisibilityPayload struct {
	Overview    *api.AIFetchOverview
	Timeseries  []api.AIFetchSeriesPoint
	Correlation *api.AIFetchCorrelationReport
}

func (b Bridge) Tools() []goaisdk.Tool {
	siteOverview := analyticscatalog.MustDefinition(analyticscatalog.ToolSiteOverview)
	eventNames := analyticscatalog.MustDefinition(analyticscatalog.ToolEventNames)
	eventBreakdown := analyticscatalog.MustDefinition(analyticscatalog.ToolEventBreakdown)
	ecommerce := analyticscatalog.MustDefinition(analyticscatalog.ToolEcommerce)
	webVitals := analyticscatalog.MustDefinition(analyticscatalog.ToolWebVitals)
	aiVisibility := analyticscatalog.MustDefinition(analyticscatalog.ToolAIVisibility)
	return []goaisdk.Tool{
		goaisdk.NewTool(siteOverview.Name, siteOverview.AIDescription, b.siteOverview),
		goaisdk.NewTool(eventNames.Name, eventNames.AIDescription, b.eventNames),
		goaisdk.NewTool(eventBreakdown.Name, eventBreakdown.AIDescription, b.eventBreakdown),
		goaisdk.NewTool(ecommerce.Name, ecommerce.AIDescription, b.ecommerce),
		goaisdk.NewTool(webVitals.Name, webVitals.AIDescription, b.webVitals),
		goaisdk.NewTool(aiVisibility.Name, aiVisibility.AIDescription, b.aiVisibility),
	}
}

func (b Bridge) siteOverview(ctx context.Context, _ struct{}) (string, error) {
	stats, err := b.SiteOverviewData(ctx, SiteOverviewInput{})
	if err != nil {
		return "", err
	}
	return toolJSON(analyticscatalog.ToolSiteOverview, b.config.SiteID, b.config.From, b.config.To, stats)
}

func (b Bridge) SiteOverviewData(ctx context.Context, input SiteOverviewInput) (*api.SiteStats, error) {
	if err := b.ready(ctx); err != nil {
		return nil, err
	}
	params := api.AnalyticsParams{
		SiteID:  b.config.SiteID,
		UserID:  b.config.UserID,
		Start:   b.config.From,
		End:     b.config.To,
		Filters: b.config.Filters,
	}
	if !input.CompareStart.IsZero() || !input.CompareEnd.IsZero() {
		params.CompareStart = input.CompareStart
		params.CompareEnd = input.CompareEnd
	}
	return b.config.Analytics.GetSiteStats(ctx, params)
}

func (b Bridge) eventNames(ctx context.Context, _ struct{}) (string, error) {
	names, err := b.EventNamesData(ctx)
	if err != nil {
		return "", err
	}
	return toolJSON(analyticscatalog.ToolEventNames, b.config.SiteID, b.config.From, b.config.To, map[string]any{"names": names})
}

func (b Bridge) EventNamesData(ctx context.Context) ([]string, error) {
	if err := b.ready(ctx); err != nil {
		return nil, err
	}
	return b.config.Analytics.GetEventNames(ctx, api.EventNamesParams{SiteID: b.config.SiteID, Start: b.config.From, End: b.config.To})
}

func (b Bridge) eventBreakdown(ctx context.Context, input struct {
	EventName   string `json:"event_name" jsonschema:"description=Tracked event name to inspect."`
	PropertyKey string `json:"property_key" jsonschema:"description=Event property key to group by."`
	Limit       int    `json:"limit" jsonschema:"description=Maximum rows to return, default 10 and maximum 25."`
}) (string, error) {
	breakdown, eventName, propertyKey, err := b.EventBreakdownData(ctx, EventBreakdownInput{
		EventName:   input.EventName,
		PropertyKey: input.PropertyKey,
		Limit:       normalizeLimit(input.Limit, 10, 25),
	})
	if err != nil {
		return "", err
	}
	return toolJSON(analyticscatalog.ToolEventBreakdown, b.config.SiteID, b.config.From, b.config.To, map[string]any{
		"event_name": eventName, "property_key": propertyKey, "breakdown": breakdown,
	})
}

func (b Bridge) EventBreakdownData(ctx context.Context, input EventBreakdownInput) ([]api.MetricStat, string, string, error) {
	if err := b.ready(ctx); err != nil {
		return nil, "", "", err
	}
	eventName := strings.TrimSpace(input.EventName)
	propertyKey := strings.TrimSpace(input.PropertyKey)
	if eventName == "" || propertyKey == "" {
		return nil, "", "", fmt.Errorf("event_name and property_key are required")
	}
	breakdown, err := b.config.Analytics.GetEventPropertyBreakdown(ctx, api.EventBreakdownParams{
		SiteID: b.config.SiteID, Start: b.config.From, End: b.config.To, EventName: eventName, PropertyKey: propertyKey,
	})
	if err != nil {
		return nil, "", "", err
	}
	if input.Limit > 0 {
		breakdown = limitMetricStats(breakdown, input.Limit)
	}
	return breakdown, eventName, propertyKey, nil
}

func (b Bridge) ecommerce(ctx context.Context, _ struct{}) (string, error) {
	payload, err := b.EcommerceData(ctx, EcommerceInput{Limit: 10, IncludeSeries: true})
	if err != nil {
		return "", err
	}
	return toolJSON(analyticscatalog.ToolEcommerce, b.config.SiteID, b.config.From, b.config.To, payload)
}

func (b Bridge) EcommerceData(ctx context.Context, input EcommerceInput) (EcommercePayload, error) {
	if err := b.ready(ctx); err != nil {
		return EcommercePayload{}, err
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 10
	}
	params := api.EcommerceParams{
		SiteID: b.config.SiteID, Start: b.config.From, End: b.config.To, Filters: b.config.Filters,
		ItemID: strings.TrimSpace(input.ItemID), ItemName: strings.TrimSpace(input.ItemName), Limit: limit,
	}
	summary, err := b.config.Analytics.GetEcommerceSummary(ctx, params)
	if err != nil {
		return EcommercePayload{}, err
	}
	var series []api.EcommerceSeriesPoint
	if input.IncludeSeries {
		series, err = b.config.Analytics.GetEcommerceTimeSeries(ctx, params)
		if err != nil {
			return EcommercePayload{}, err
		}
	}
	products, err := b.config.Analytics.GetEcommerceTopProducts(ctx, params)
	if err != nil {
		return EcommercePayload{}, err
	}
	sources, err := b.config.Analytics.GetEcommerceSources(ctx, params)
	if err != nil {
		return EcommercePayload{}, err
	}
	return EcommercePayload{Summary: summary, Series: series, Products: products, Sources: sources}, nil
}

func (b Bridge) webVitals(ctx context.Context, input struct {
	Metric            string `json:"metric" jsonschema:"description=Optional Web Vital metric: LCP, INP, CLS, FCP, or TTFB."`
	IncludeTimeseries bool   `json:"include_timeseries" jsonschema:"description=Whether to include metric timeseries when metric is set."`
	IncludePages      bool   `json:"include_pages" jsonschema:"description=Whether to include slowest page rows when metric is set."`
}) (string, error) {
	payload, err := b.WebVitalsData(ctx, WebVitalsInput{
		Metric:            api.WebVitalMetric(strings.ToUpper(strings.TrimSpace(input.Metric))),
		IncludeTimeseries: input.IncludeTimeseries,
		IncludePages:      input.IncludePages,
		Limit:             10,
	})
	if err != nil {
		return "", err
	}
	return toolJSON(analyticscatalog.ToolWebVitals, b.config.SiteID, b.config.From, b.config.To, payload)
}

func (b Bridge) WebVitalsData(ctx context.Context, input WebVitalsInput) (WebVitalsPayload, error) {
	if err := b.ready(ctx); err != nil {
		return WebVitalsPayload{}, err
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 10
	}
	metric := input.Metric
	if metric == "" && (input.IncludePages || input.BreakdownDimension != "") {
		metric = api.WebVitalLCP
	}
	if metric != "" {
		if _, err := database.WebVitalRatingForValue(metric, 0); err != nil {
			return WebVitalsPayload{}, fmt.Errorf("invalid metric")
		}
	}
	if input.BreakdownDimension != "" && !isAllowedWebVitalBreakdownDimension(input.BreakdownDimension) {
		return WebVitalsPayload{}, fmt.Errorf("invalid web vital breakdown dimension")
	}
	params := api.WebVitalsParams{
		SiteID: b.config.SiteID, Start: b.config.From, End: b.config.To,
		Metric: metric, Path: strings.TrimSpace(input.Path), Rating: input.Rating, Limit: limit,
	}
	summary, err := b.config.Analytics.GetWebVitalsSummary(ctx, params)
	if err != nil {
		return WebVitalsPayload{}, err
	}
	payload := WebVitalsPayload{Metric: metric, Summary: summary, BreakdownDimension: input.BreakdownDimension}
	if input.IncludeTimeseries && metric != "" {
		timeseries, err := b.config.Analytics.GetWebVitalsTimeseries(ctx, params)
		if err != nil {
			return WebVitalsPayload{}, err
		}
		payload.Timeseries = timeseries
	}
	if input.IncludePages && metric != "" {
		pages, err := b.config.Analytics.GetWebVitalsPages(ctx, params)
		if err != nil {
			return WebVitalsPayload{}, err
		}
		payload.Pages = pages
	}
	if input.BreakdownDimension != "" && metric != "" {
		breakdown, err := b.config.Analytics.GetWebVitalsBreakdown(ctx, params, input.BreakdownDimension)
		if err != nil {
			return WebVitalsPayload{}, err
		}
		payload.Breakdown = breakdown
	}
	return payload, nil
}

func (b Bridge) aiVisibility(ctx context.Context, input struct {
	AssistantName      string `json:"assistant_name" jsonschema:"description=Optional assistant name filter."`
	AssistantFamily    string `json:"assistant_family" jsonschema:"description=Optional assistant family filter."`
	ResourceType       string `json:"resource_type" jsonschema:"description=Optional resource type filter."`
	Path               string `json:"path" jsonschema:"description=Optional path filter."`
	IncludeTimeseries  bool   `json:"include_timeseries" jsonschema:"description=Whether to include AI fetch timeseries."`
	IncludeCorrelation bool   `json:"include_correlation" jsonschema:"description=Whether to include fetch-to-AI-referred-visit correlation."`
}) (string, error) {
	payload, err := b.AIVisibilityData(ctx, AIVisibilityInput{
		AssistantName:      input.AssistantName,
		AssistantFamily:    input.AssistantFamily,
		ResourceType:       input.ResourceType,
		Path:               input.Path,
		IncludeTimeseries:  input.IncludeTimeseries,
		IncludeCorrelation: input.IncludeCorrelation,
		WindowDays:         30,
	})
	if err != nil {
		return "", err
	}
	return toolJSON(analyticscatalog.ToolAIVisibility, b.config.SiteID, b.config.From, b.config.To, payload)
}

func (b Bridge) AIVisibilityData(ctx context.Context, input AIVisibilityInput) (AIVisibilityPayload, error) {
	if err := b.ready(ctx); err != nil {
		return AIVisibilityPayload{}, err
	}
	params := api.AIFetchQueryParams{
		SiteID: b.config.SiteID, Start: b.config.From, End: b.config.To,
		AssistantName: strings.TrimSpace(input.AssistantName), AssistantFamily: strings.TrimSpace(input.AssistantFamily),
		ResourceType: strings.TrimSpace(input.ResourceType), Path: strings.TrimSpace(input.Path),
	}
	overview, err := b.config.Analytics.GetAIFetchOverview(ctx, params)
	if err != nil {
		return AIVisibilityPayload{}, err
	}
	payload := AIVisibilityPayload{Overview: overview}
	if input.IncludeTimeseries {
		timeseries, err := b.config.Analytics.GetAIFetchTimeseries(ctx, params)
		if err != nil {
			return AIVisibilityPayload{}, err
		}
		payload.Timeseries = timeseries
	}
	if input.IncludeCorrelation {
		windowDays := input.WindowDays
		if windowDays <= 0 {
			windowDays = 30
		}
		if windowDays > 90 {
			windowDays = 90
		}
		correlation, err := b.config.Analytics.GetAIFetchCorrelation(ctx, api.AIFetchCorrelationParams{
			SiteID: params.SiteID, Start: params.Start, End: params.End, AssistantName: params.AssistantName,
			AssistantFamily: params.AssistantFamily, ResourceType: params.ResourceType, Path: params.Path, WindowDays: windowDays,
		})
		if err != nil {
			return AIVisibilityPayload{}, err
		}
		payload.Correlation = correlation
	}
	return payload, nil
}

func (b Bridge) ready(ctx context.Context) error {
	if b.config.BeforeExecute != nil {
		if err := b.config.BeforeExecute(ctx); err != nil {
			return err
		}
	}
	if b.config.Analytics == nil {
		return fmt.Errorf("analytics store unavailable")
	}
	if b.config.SiteID == uuid.Nil {
		return fmt.Errorf("site scope is required")
	}
	if b.config.To.IsZero() || b.config.From.IsZero() || !b.config.From.Before(b.config.To) {
		return fmt.Errorf("valid date range is required")
	}
	return nil
}

func isAllowedWebVitalBreakdownDimension(dimension api.WebVitalDimension) bool {
	switch dimension {
	case api.WebVitalDimensionBrowser,
		api.WebVitalDimensionCountry,
		api.WebVitalDimensionLanguage,
		api.WebVitalDimensionDevice,
		api.WebVitalDimensionCity,
		api.WebVitalDimensionProvider,
		api.WebVitalDimensionASN:
		return true
	default:
		return false
	}
}

func toolJSON(evidenceID string, siteID uuid.UUID, from, to time.Time, data any) (string, error) {
	raw, err := json.Marshal(map[string]any{
		"evidence_id": evidenceID,
		"site_id":     siteID.String(),
		"from":        from.UTC().Format(time.RFC3339),
		"to":          to.UTC().Format(time.RFC3339),
		"data":        data,
	})
	if err != nil || !json.Valid(raw) {
		return "", fmt.Errorf("encode tool result")
	}
	return string(raw), nil
}

func normalizeLimit(value, fallback, maxValue int) int {
	if value <= 0 {
		value = fallback
	}
	if value > maxValue {
		value = maxValue
	}
	return value
}

func limitMetricStats(rows []api.MetricStat, limit int) []api.MetricStat {
	if limit <= 0 || len(rows) <= limit {
		return rows
	}
	return rows[:limit]
}
