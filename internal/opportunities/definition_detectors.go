package opportunities

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
)

func detectCheckoutOpportunity(input DetectorInput, definition OpportunityDefinition) (*database.OpportunityInput, bool) {
	if input.Ecommerce == nil || input.Ecommerce.CheckoutStarts <= 0 || input.Ecommerce.CheckoutConversionRate >= 55 {
		return nil, false
	}
	score, ok := scoreCheckoutOpportunity(checkoutScoringInput{
		CheckoutStarts:         input.Ecommerce.CheckoutStarts,
		Orders:                 input.Ecommerce.Orders,
		CheckoutConversionRate: input.Ecommerce.CheckoutConversionRate,
		AverageOrderValue:      input.Ecommerce.AverageOrderValue,
	})
	if !ok {
		return nil, false
	}
	opportunity := checkoutOpportunity(definition, input, input.Ecommerce, score, input.GeneratedAt)
	return &opportunity, true
}

func detectAIVisibilityOpportunity(input DetectorInput, definition OpportunityDefinition) (*database.OpportunityInput, bool) {
	if input.AIVisibility == nil || input.AIVisibility.TotalRequests <= 0 {
		return nil, false
	}
	opportunity := aiVisibilityOpportunity(definition, input, input.AIVisibility, input.GeneratedAt)
	return &opportunity, true
}

func detectTrafficQualityOpportunity(input DetectorInput, definition OpportunityDefinition) (*database.OpportunityInput, bool) {
	if input.Stats == nil || input.Stats.TotalPageviews <= 0 {
		return nil, false
	}
	source, ok := trafficOpportunitySource(input.Stats)
	if !ok {
		return nil, false
	}
	opportunity := trafficQualityOpportunity(definition, input, source, input.GeneratedAt)
	return &opportunity, true
}

func detectWebVitalsOpportunity(input DetectorInput, definition OpportunityDefinition) (*database.OpportunityInput, bool) {
	candidate, ok := webVitalsOpportunityCandidate(input.WebVitals)
	if !ok {
		return nil, false
	}
	score, ok := scoreWebVitalsOpportunity(webVitalsScoringInput{
		Samples:                 candidate.Samples,
		PoorSamples:             candidate.PoorSamples,
		NeedsImprovementSamples: candidate.NeedsImprovementSamples,
		Rating:                  candidate.Rating,
		HasPageEvidence:         candidate.Path != "",
	})
	if !ok {
		return nil, false
	}
	opportunity := webVitalsOpportunity(definition, input, candidate, score, input.GeneratedAt)
	return &opportunity, true
}

func detectSearchVisibilityOpportunity(input DetectorInput, definition OpportunityDefinition) (*database.OpportunityInput, bool) {
	if input.SearchConsole == nil || input.SearchConsole.Impressions <= 0 {
		return nil, false
	}
	score, estimatedClicks, ok := scoreSearchVisibilityOpportunity(searchVisibilityScoringInput{
		Impressions:     input.SearchConsole.Impressions,
		Clicks:          input.SearchConsole.Clicks,
		CTR:             input.SearchConsole.CTR,
		AveragePosition: input.SearchConsole.AveragePosition,
	})
	if !ok {
		return nil, false
	}
	opportunity := searchVisibilityOpportunity(definition, input, input.SearchConsole, score, estimatedClicks, input.GeneratedAt)
	return &opportunity, true
}

func detectConversionSignalOpportunity(input DetectorInput, definition OpportunityDefinition) (*database.OpportunityInput, bool) {
	if input.Stats == nil || input.Stats.TotalPageviews < 500 || len(input.EventNames) == 0 {
		return nil, false
	}
	if input.Ecommerce != nil && (input.Ecommerce.CheckoutStarts > 0 || input.Ecommerce.Orders > 0) {
		return nil, false
	}
	if hasKnownConversionEvent(input.EventNames) {
		return nil, false
	}
	eventNames := genericEventNames(input.EventNames)
	if len(eventNames) == 0 {
		return nil, false
	}
	opportunity := conversionSignalOpportunity(definition, input, input.Stats, eventNames, input.GeneratedAt)
	return &opportunity, true
}

func detectSetupGoalSuggestionOpportunity(input DetectorInput, definition OpportunityDefinition) (*database.OpportunityInput, bool) {
	candidate, ok := setupGoalSuggestionCandidate(input.SetupEvidence)
	if !ok {
		return nil, false
	}
	opportunity := setupGoalSuggestionOpportunity(definition, input, candidate, input.GeneratedAt)
	return &opportunity, true
}

func detectSetupFunnelSuggestionOpportunity(input DetectorInput, definition OpportunityDefinition) (*database.OpportunityInput, bool) {
	candidate, ok := setupFunnelSuggestionCandidate(input.SetupEvidence)
	if !ok {
		return nil, false
	}
	opportunity := setupFunnelSuggestionOpportunity(definition, input, candidate, input.GeneratedAt)
	return &opportunity, true
}

func checkoutOpportunity(definition OpportunityDefinition, input DetectorInput, ecommerce *api.EcommerceSummary, score opportunityScoreBreakdown, generatedAt time.Time) database.OpportunityInput {
	copyParams := map[string]any{
		"checkout_starts": ecommerce.CheckoutStarts,
		"orders":          ecommerce.Orders,
		"conversion_rate": fmt.Sprintf("%.1f%%", ecommerce.CheckoutConversionRate),
	}
	evidence := []api.OpportunityEvidence{
		{ID: "checkout_starts", LabelKey: "opportunities.evidence.checkout_starts", Value: fmt.Sprintf("%d", ecommerce.CheckoutStarts)},
		{ID: "orders", LabelKey: "opportunities.evidence.orders", Value: fmt.Sprintf("%d", ecommerce.Orders)},
		{ID: "conversion_rate", LabelKey: "opportunities.evidence.checkout_conversion_rate", Value: fmt.Sprintf("%.1f%%", ecommerce.CheckoutConversionRate)},
	}
	citedEvidenceIDs := []string{"checkout_starts", "orders", "conversion_rate"}
	appendGeoEvidence := func(id, labelKey, value string) {
		value = strings.TrimSpace(value)
		if value == "" || value == "(Unknown)" {
			return
		}
		copyParams[id] = value
		evidence = append(evidence, api.OpportunityEvidence{ID: id, LabelKey: labelKey, Value: value})
		citedEvidenceIDs = append(citedEvidenceIDs, id)
	}
	appendGeoEvidence("top_city", "opportunities.evidence.top_city", topMetricName(ecommerce.TopCities, ""))
	appendGeoEvidence("top_provider", "opportunities.evidence.top_provider", topMetricName(ecommerce.TopProviders, ""))
	appendGeoEvidence("top_asn", "opportunities.evidence.top_asn", topMetricName(ecommerce.TopASNs, ""))

	return definition.BuildOpportunity(withGeneratedAt(input, generatedAt), OpportunityRecipe{
		CopyParams:       copyParams,
		ImpactValue:      fmt.Sprintf("%d", ecommerce.CheckoutStarts),
		Confidence:       score.Confidence,
		Score:            score.Total,
		ScoreBreakdown:   opportunityScoreAPI(score),
		RouteParams:      map[string]any{"path": "/checkout"},
		Evidence:         evidence,
		CitedEvidenceIDs: citedEvidenceIDs,
	})
}

func opportunityScoreAPI(score opportunityScoreBreakdown) api.OpportunityScoreBreakdown {
	return api.OpportunityScoreBreakdown{
		Sample:        score.Sample,
		Impact:        score.Impact,
		Urgency:       score.Urgency,
		Effort:        score.Effort,
		Actionability: score.Actionability,
		EvidenceFit:   score.EvidenceFit,
		Freshness:     score.Freshness,
		Total:         score.Total,
	}
}

func aiVisibilityOpportunity(definition OpportunityDefinition, input DetectorInput, aiVisibility *api.AIFetchOverview, generatedAt time.Time) database.OpportunityInput {
	path := topMetricName(aiVisibility.TopPaths, "/")
	support := aiVisibilityTrafficSupport(input.Stats, path)
	score := scoreAIVisibilityOpportunity(aiVisibilityScoringInput{
		Requests:         aiVisibility.TotalRequests,
		UniquePaths:      aiVisibility.UniquePaths,
		AIReferrals:      support.AIReferrals,
		TopPathPageviews: support.TopPathPageviews,
	})
	copyParams := map[string]any{
		"requests":     aiVisibility.TotalRequests,
		"unique_paths": aiVisibility.UniquePaths,
		"top_path":     path,
	}
	evidence := []api.OpportunityEvidence{
		{ID: "ai_requests", LabelKey: "opportunities.evidence.ai_requests", Value: fmt.Sprintf("%d", aiVisibility.TotalRequests)},
		{ID: "ai_paths", LabelKey: "opportunities.evidence.ai_paths", Value: fmt.Sprintf("%d", aiVisibility.UniquePaths)},
		{ID: "top_ai_path", LabelKey: "opportunities.evidence.top_ai_path", Value: path},
	}
	citedEvidenceIDs := []string{"ai_requests", "ai_paths", "top_ai_path"}
	if support.AIReferrals > 0 {
		copyParams["ai_referrals"] = support.AIReferrals
		evidence = append(evidence, api.OpportunityEvidence{ID: "ai_referrals", LabelKey: "opportunities.evidence.ai_referrals", Value: fmt.Sprintf("%d", support.AIReferrals)})
		citedEvidenceIDs = append(citedEvidenceIDs, "ai_referrals")
	}
	if support.TopPathPageviews > 0 {
		copyParams["top_path_pageviews"] = support.TopPathPageviews
		evidence = append(evidence, api.OpportunityEvidence{ID: "ai_path_pageviews", LabelKey: "opportunities.evidence.ai_path_pageviews", Value: fmt.Sprintf("%d", support.TopPathPageviews)})
		citedEvidenceIDs = append(citedEvidenceIDs, "ai_path_pageviews")
	}
	appendGeoEvidence := func(id, labelKey, value string) {
		value = strings.TrimSpace(value)
		if value == "" || value == "(Unknown)" {
			return
		}
		copyParams[id] = value
		evidence = append(evidence, api.OpportunityEvidence{ID: id, LabelKey: labelKey, Value: value})
		citedEvidenceIDs = append(citedEvidenceIDs, id)
	}
	appendGeoEvidence("top_city", "opportunities.evidence.top_city", support.TopCity)
	appendGeoEvidence("top_provider", "opportunities.evidence.top_provider", support.TopProvider)
	appendGeoEvidence("top_asn", "opportunities.evidence.top_asn", support.TopASN)
	return definition.BuildOpportunity(withGeneratedAt(input, generatedAt), OpportunityRecipe{
		CopyParams:       copyParams,
		ImpactValue:      fmt.Sprintf("+%d", maxInt64(1, aiVisibility.UniquePaths)),
		Confidence:       score.Confidence,
		Score:            score.Total,
		ScoreBreakdown:   opportunityScoreAPI(score),
		RouteParams:      map[string]any{"path": path},
		Evidence:         evidence,
		CitedEvidenceIDs: citedEvidenceIDs,
	})
}

type aiVisibilityTrafficSupportEvidence struct {
	AIReferrals      int
	TopPathPageviews int
	TopCity          string
	TopProvider      string
	TopASN           string
}

func aiVisibilityTrafficSupport(stats *api.SiteStats, path string) aiVisibilityTrafficSupportEvidence {
	if stats == nil {
		return aiVisibilityTrafficSupportEvidence{}
	}
	return aiVisibilityTrafficSupportEvidence{
		AIReferrals:      stats.AISourceVisits,
		TopPathPageviews: metricValueForName(stats.TopPages, path),
		TopCity:          topMetricName(stats.TopCities, ""),
		TopProvider:      topMetricName(stats.TopProviders, ""),
		TopASN:           topMetricName(stats.TopASNs, ""),
	}
}

func metricValueForName(items []api.MetricStat, name string) int {
	normalized := strings.TrimSpace(name)
	for _, item := range items {
		if strings.TrimSpace(item.Name) == normalized {
			return item.Value
		}
	}
	return 0
}

type trafficSourceEvidence struct {
	Name           string
	Hits           int
	TotalPageviews int
	Sessions       int
	TopCity        string
	TopProvider    string
	TopASN         string
}

type webVitalsOpportunityEvidence struct {
	Metric                  api.WebVitalMetric
	P75                     float64
	Rating                  api.WebVitalRating
	Samples                 int64
	PoorSamples             int64
	NeedsImprovementSamples int64
	Path                    string
	PageP75                 float64
	PageRating              api.WebVitalRating
	PageSamples             int64
	TopCity                 string
	TopProvider             string
	TopASN                  string
}

func webVitalsOpportunityCandidate(snapshot *WebVitalsEvidenceSnapshot) (webVitalsOpportunityEvidence, bool) {
	if !webVitalsSnapshotHasSummary(snapshot) {
		return webVitalsOpportunityEvidence{}, false
	}
	best, ok := topWebVitalsOpportunityCandidate(snapshot)
	if !webVitalsCandidateActionable(best, ok) {
		return webVitalsOpportunityEvidence{}, false
	}
	return withDefaultWebVitalsPath(best), true
}

func webVitalsSnapshotHasSummary(snapshot *WebVitalsEvidenceSnapshot) bool {
	return snapshot != nil && len(snapshot.Summary) > 0
}

func webVitalsCandidateActionable(candidate webVitalsOpportunityEvidence, ok bool) bool {
	return ok && candidate.Rating != api.WebVitalRatingGood
}

func withDefaultWebVitalsPath(candidate webVitalsOpportunityEvidence) webVitalsOpportunityEvidence {
	if candidate.Path == "" {
		candidate.Path = "/"
	}
	return candidate
}

func topWebVitalsOpportunityCandidate(snapshot *WebVitalsEvidenceSnapshot) (webVitalsOpportunityEvidence, bool) {
	best := webVitalsOpportunityEvidence{}
	bestScore := -1
	for _, metric := range snapshot.Summary {
		candidate, ok := webVitalsMetricOpportunityCandidate(metric, snapshot.Pages[metric.Metric])
		if !ok {
			continue
		}
		candidate = withWebVitalsDimensionEvidence(candidate, snapshot.Dimensions[metric.Metric])
		score := webVitalsMetricOpportunityScore(candidate)
		if score <= bestScore {
			continue
		}
		bestScore = score
		best = candidate
	}
	if bestScore < 0 {
		return webVitalsOpportunityEvidence{}, false
	}
	return best, true
}

func webVitalsMetricOpportunityCandidate(metric api.WebVitalSummaryMetric, pages []api.WebVitalPageRow) (webVitalsOpportunityEvidence, bool) {
	if metric.Samples < minWebVitalsSamples {
		return webVitalsOpportunityEvidence{}, false
	}
	candidate := webVitalsOpportunityEvidence{
		Metric:                  metric.Metric,
		P75:                     metric.P75,
		Rating:                  metric.Rating,
		Samples:                 metric.Samples,
		PoorSamples:             metric.Poor,
		NeedsImprovementSamples: metric.NeedsImprove,
	}
	return withWebVitalsPageEvidence(candidate, pages), true
}

func withWebVitalsPageEvidence(candidate webVitalsOpportunityEvidence, pages []api.WebVitalPageRow) webVitalsOpportunityEvidence {
	for _, page := range pages {
		if page.Samples <= 0 {
			continue
		}
		candidate.Path = strings.TrimSpace(page.Path)
		candidate.PageP75 = page.P75
		candidate.PageRating = page.Rating
		candidate.PageSamples = page.Samples
		return candidate
	}
	return candidate
}

func withWebVitalsDimensionEvidence(candidate webVitalsOpportunityEvidence, dimensions WebVitalsDimensionEvidence) webVitalsOpportunityEvidence {
	candidate.TopCity = topWebVitalDimensionName(dimensions.TopCities)
	candidate.TopProvider = topWebVitalDimensionName(dimensions.TopProviders)
	candidate.TopASN = topWebVitalDimensionName(dimensions.TopASNs)
	return candidate
}

func topWebVitalDimensionName(rows []api.WebVitalDimensionRow) string {
	for _, row := range rows {
		name := strings.TrimSpace(row.Name)
		if name == "" || name == "(Unknown)" {
			continue
		}
		return name
	}
	return ""
}

func webVitalsMetricOpportunityScore(candidate webVitalsOpportunityEvidence) int {
	return webVitalRatingPriority(candidate.Rating)*1000 +
		int(minInt64(candidate.Samples, 100)) +
		int(candidate.PoorSamples*3) +
		int(candidate.NeedsImprovementSamples)
}

func webVitalRatingPriority(rating api.WebVitalRating) int {
	switch rating {
	case api.WebVitalRatingPoor:
		return 2
	case api.WebVitalRatingNeedsImprovement:
		return 1
	case api.WebVitalRatingGood:
		return 0
	default:
		return 0
	}
}

func webVitalsOpportunity(definition OpportunityDefinition, input DetectorInput, candidate webVitalsOpportunityEvidence, score opportunityScoreBreakdown, generatedAt time.Time) database.OpportunityInput {
	p75 := formatWebVitalValue(candidate.Metric, candidate.P75)
	pageP75 := formatWebVitalValue(candidate.Metric, candidate.PageP75)
	rating := formatWebVitalRating(candidate.Rating)
	copyParams := map[string]any{
		"metric":                    string(candidate.Metric),
		"p75":                       p75,
		"rating":                    rating,
		"samples":                   candidate.Samples,
		"poor_samples":              candidate.PoorSamples,
		"needs_improvement_samples": candidate.NeedsImprovementSamples,
		"path":                      candidate.Path,
		"page_p75":                  pageP75,
		"page_samples":              candidate.PageSamples,
	}
	evidence := []api.OpportunityEvidence{
		{ID: "web_vital_metric", LabelKey: "opportunities.evidence.web_vital_metric", Value: string(candidate.Metric)},
		{ID: "web_vital_p75", LabelKey: "opportunities.evidence.web_vital_p75", Value: p75},
		{ID: "web_vital_rating", LabelKey: "opportunities.evidence.web_vital_rating", Value: rating},
		{ID: "web_vital_samples", LabelKey: "opportunities.evidence.web_vital_samples", Value: fmt.Sprintf("%d", candidate.Samples)},
		{ID: "web_vital_poor_samples", LabelKey: "opportunities.evidence.web_vital_poor_samples", Value: fmt.Sprintf("%d", candidate.PoorSamples)},
		{ID: "web_vital_top_page", LabelKey: "opportunities.evidence.web_vital_top_page", Value: candidate.Path},
		{ID: "web_vital_top_page_p75", LabelKey: "opportunities.evidence.web_vital_top_page_p75", Value: pageP75},
	}
	citedEvidenceIDs := []string{"web_vital_metric", "web_vital_p75", "web_vital_rating", "web_vital_samples", "web_vital_top_page", "web_vital_top_page_p75"}
	appendGeoEvidence := func(id, labelKey, value string) {
		value = strings.TrimSpace(value)
		if value == "" || value == "(Unknown)" {
			return
		}
		copyParams[id] = value
		evidence = append(evidence, api.OpportunityEvidence{ID: id, LabelKey: labelKey, Value: value})
		citedEvidenceIDs = append(citedEvidenceIDs, id)
	}
	appendGeoEvidence("top_city", "opportunities.evidence.top_city", candidate.TopCity)
	appendGeoEvidence("top_provider", "opportunities.evidence.top_provider", candidate.TopProvider)
	appendGeoEvidence("top_asn", "opportunities.evidence.top_asn", candidate.TopASN)
	opportunity := definition.BuildOpportunity(withGeneratedAt(input, generatedAt), OpportunityRecipe{
		CopyParams:       copyParams,
		ImpactValue:      fmt.Sprintf("%d", candidate.Samples),
		Confidence:       score.Confidence,
		Score:            score.Total,
		ScoreBreakdown:   opportunityScoreAPI(score),
		RouteParams:      map[string]any{"metric": string(candidate.Metric), "path": candidate.Path},
		Evidence:         evidence,
		CitedEvidenceIDs: citedEvidenceIDs,
	})
	opportunity.ID = stableOpportunityID(input.SiteID, definition.Key+":"+strings.ToLower(string(candidate.Metric)))
	return opportunity
}

func formatWebVitalValue(metric api.WebVitalMetric, value float64) string {
	if metric == api.WebVitalCLS {
		return fmt.Sprintf("%.2f", value)
	}
	return fmt.Sprintf("%.0f ms", value)
}

func formatWebVitalRating(rating api.WebVitalRating) string {
	switch rating {
	case api.WebVitalRatingPoor:
		return "poor"
	case api.WebVitalRatingNeedsImprovement:
		return "needs improvement"
	case api.WebVitalRatingGood:
		return "good"
	default:
		return string(rating)
	}
}

func trafficQualityOpportunity(definition OpportunityDefinition, input DetectorInput, source trafficSourceEvidence, generatedAt time.Time) database.OpportunityInput {
	copyParams := map[string]any{
		"source":          source.Name,
		"source_hits":     source.Hits,
		"total_pageviews": source.TotalPageviews,
		"sessions":        source.Sessions,
	}
	evidence := []api.OpportunityEvidence{
		{ID: "top_source", LabelKey: "opportunities.evidence.top_source", Value: source.Name},
		{ID: "source_hits", LabelKey: "opportunities.evidence.source_hits", Value: fmt.Sprintf("%d", source.Hits)},
		{ID: "total_pageviews", LabelKey: "opportunities.evidence.total_pageviews", Value: fmt.Sprintf("%d", source.TotalPageviews)},
		{ID: "sessions", LabelKey: "opportunities.evidence.sessions", Value: fmt.Sprintf("%d", source.Sessions)},
	}
	citedEvidenceIDs := []string{"top_source", "source_hits", "total_pageviews", "sessions"}
	appendGeoEvidence := func(id, labelKey, value string) {
		value = strings.TrimSpace(value)
		if value == "" || value == "(Unknown)" {
			return
		}
		copyParams[id] = value
		evidence = append(evidence, api.OpportunityEvidence{ID: id, LabelKey: labelKey, Value: value})
		citedEvidenceIDs = append(citedEvidenceIDs, id)
	}
	appendGeoEvidence("top_city", "opportunities.evidence.top_city", source.TopCity)
	appendGeoEvidence("top_provider", "opportunities.evidence.top_provider", source.TopProvider)
	appendGeoEvidence("top_asn", "opportunities.evidence.top_asn", source.TopASN)
	return definition.BuildOpportunity(withGeneratedAt(input, generatedAt), OpportunityRecipe{
		CopyParams:       copyParams,
		ImpactValue:      fmt.Sprintf("%d", source.Hits),
		Confidence:       confidence(source.Hits >= 200),
		Score:            clampScore(55 + source.Hits/4),
		RouteParams:      map[string]any{"source": source.Name},
		Evidence:         evidence,
		CitedEvidenceIDs: citedEvidenceIDs,
	})
}

func trafficOpportunitySource(stats *api.SiteStats) (trafficSourceEvidence, bool) {
	if stats == nil || stats.TotalPageviews <= 0 {
		return trafficSourceEvidence{}, false
	}
	source, ok := topActionableTrafficSource(stats.TopUTMSources)
	if !ok {
		source, ok = topActionableTrafficSource(stats.TopReferrers)
	}
	if !ok || source.Hits < minTrafficSourceHits {
		return trafficSourceEvidence{}, false
	}
	if float64(source.Hits)/float64(stats.TotalPageviews) < minTrafficSourceShare {
		return trafficSourceEvidence{}, false
	}
	source.TotalPageviews = stats.TotalPageviews
	source.Sessions = stats.UniqueSessions
	// These audience dimensions come from the site-wide stats snapshot, not
	// from a source-filtered query. The evidence labels call this out as
	// overall audience context.
	source.TopCity = topMetricName(stats.TopCities, "")
	source.TopProvider = topMetricName(stats.TopProviders, "")
	source.TopASN = topMetricName(stats.TopASNs, "")
	return source, true
}

func topActionableTrafficSource(items []api.MetricStat) (trafficSourceEvidence, bool) {
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		if isActionableTrafficSource(name) && item.Value > 0 {
			return trafficSourceEvidence{Name: name, Hits: item.Value}, true
		}
	}
	return trafficSourceEvidence{}, false
}

func isActionableTrafficSource(name string) bool {
	normalized := strings.ToLower(strings.TrimSpace(name))
	switch normalized {
	case "", "(unspecified)", "unspecified", "(not set)", "not set", "direct", "(direct)", "none", "(none)":
		return false
	default:
		return true
	}
}

func searchVisibilityOpportunity(
	definition OpportunityDefinition,
	input DetectorInput,
	overview *api.SearchConsoleOverview,
	score opportunityScoreBreakdown,
	estimatedClicks int,
	generatedAt time.Time,
) database.OpportunityInput {
	ctr := formatRatePercent(overview.CTR)
	position := fmt.Sprintf("%.1f", overview.AveragePosition)
	return definition.BuildOpportunity(withGeneratedAt(input, generatedAt), OpportunityRecipe{
		CopyParams: map[string]any{
			"impressions":      overview.Impressions,
			"clicks":           overview.Clicks,
			"ctr":              ctr,
			"average_position": position,
			"estimated_clicks": estimatedClicks,
		},
		ImpactValue:    fmt.Sprintf("+%d", estimatedClicks),
		Confidence:     score.Confidence,
		Score:          score.Total,
		ScoreBreakdown: opportunityScoreAPI(score),
		RouteParams:    map[string]any{},
		Evidence: []api.OpportunityEvidence{
			{ID: "search_impressions", LabelKey: "opportunities.evidence.search_impressions", Value: fmt.Sprintf("%d", overview.Impressions)},
			{ID: "search_ctr", LabelKey: "opportunities.evidence.search_ctr", Value: ctr},
			{ID: "search_position", LabelKey: "opportunities.evidence.search_position", Value: position},
		},
		CitedEvidenceIDs: []string{"search_impressions", "search_ctr", "search_position"},
	})
}

func conversionSignalOpportunity(definition OpportunityDefinition, input DetectorInput, stats *api.SiteStats, eventNames []string, generatedAt time.Time) database.OpportunityInput {
	eventNamesValue := strings.Join(eventNames, ", ")
	score := clampScore(58 + stats.TotalPageviews/250 + len(eventNames)*3)
	return definition.BuildOpportunity(withGeneratedAt(input, generatedAt), OpportunityRecipe{
		CopyParams: map[string]any{
			"pageviews":   stats.TotalPageviews,
			"sessions":    stats.UniqueSessions,
			"event_count": len(eventNames),
			"event_names": eventNamesValue,
		},
		ImpactValue: fmt.Sprintf("%d", stats.TotalPageviews),
		Confidence:  confidence(stats.UniqueSessions >= 500),
		Score:       score,
		RouteParams: map[string]any{},
		Evidence: []api.OpportunityEvidence{
			{ID: "pageviews", LabelKey: "opportunities.evidence.pageviews", Value: fmt.Sprintf("%d", stats.TotalPageviews)},
			{ID: "sessions", LabelKey: "opportunities.evidence.sessions", Value: fmt.Sprintf("%d", stats.UniqueSessions)},
			{ID: "event_names", LabelKey: "opportunities.evidence.event_names", Value: eventNamesValue},
		},
		CitedEvidenceIDs: []string{"pageviews", "sessions", "event_names"},
	})
}

type setupGoalCandidate struct {
	EventName  string
	EventCount int
}

type setupFunnelCandidate struct {
	StartPath       string
	StartPageviews  int
	ConversionEvent string
	EventCount      int
}

func setupGoalSuggestionCandidate(snapshot *SetupEvidenceSnapshot) (setupGoalCandidate, bool) {
	if snapshot == nil {
		return setupGoalCandidate{}, false
	}
	for _, event := range snapshot.Events {
		eventName := strings.TrimSpace(event.Name)
		if event.Count < minSetupGoalEventCount || !isKnownConversionEvent(eventName) {
			continue
		}
		if setupEvidenceHasGoalForEvent(snapshot.Goals, eventName) {
			continue
		}
		return setupGoalCandidate{EventName: eventName, EventCount: event.Count}, true
	}
	return setupGoalCandidate{}, false
}

func setupFunnelSuggestionCandidate(snapshot *SetupEvidenceSnapshot) (setupFunnelCandidate, bool) {
	if snapshot == nil {
		return setupFunnelCandidate{}, false
	}
	startPath, pageviews, ok := setupFunnelStartPath(snapshot.TopPages)
	if !ok {
		return setupFunnelCandidate{}, false
	}
	conversionEvent, eventCount, ok := setupFunnelConversionEvent(snapshot.Events)
	if !ok {
		return setupFunnelCandidate{}, false
	}
	if setupEvidenceHasFunnelSteps(snapshot.Funnels, startPath, conversionEvent) {
		return setupFunnelCandidate{}, false
	}
	return setupFunnelCandidate{
		StartPath:       startPath,
		StartPageviews:  pageviews,
		ConversionEvent: conversionEvent,
		EventCount:      eventCount,
	}, true
}

func setupFunnelStartPath(topPages []SetupTopPageEvidence) (string, int, bool) {
	for _, page := range topPages {
		path := strings.TrimSpace(page.Path)
		if page.Pageviews < minSetupFunnelStartPageviews || !isLikelyFunnelStartPath(path) {
			continue
		}
		return path, page.Pageviews, true
	}
	return "", 0, false
}

func setupFunnelConversionEvent(events []SetupEventEvidence) (string, int, bool) {
	for _, event := range events {
		name := strings.TrimSpace(event.Name)
		if event.Count < minSetupGoalEventCount || !isKnownConversionEvent(name) {
			continue
		}
		return name, event.Count, true
	}
	return "", 0, false
}

func isLikelyFunnelStartPath(path string) bool {
	normalized := normalizeOpportunityToken(path)
	if normalized == "" || normalized == "/" {
		return false
	}
	for _, part := range []string{"pricing", "signup", "sign-up", "demo", "contact", "checkout", "cart", "trial"} {
		if strings.Contains(normalized, part) {
			return true
		}
	}
	return false
}

func setupEvidenceHasGoalForEvent(goals []SetupGoalEvidence, eventName string) bool {
	normalizedEvent := normalizeOpportunityToken(eventName)
	for _, goal := range goals {
		if normalizeOpportunityToken(goal.Type) != "event" {
			continue
		}
		if normalizeOpportunityToken(goal.Value) == normalizedEvent {
			return true
		}
	}
	return false
}

func setupEvidenceHasFunnelSteps(funnels []SetupFunnelEvidence, startPath, conversionEvent string) bool {
	normalizedStart := normalizeOpportunityToken(startPath)
	normalizedEvent := normalizeOpportunityToken(conversionEvent)
	for _, funnel := range funnels {
		if len(funnel.Steps) < 2 {
			continue
		}
		first := funnel.Steps[0]
		last := funnel.Steps[len(funnel.Steps)-1]
		if normalizeOpportunityToken(first.Type) != "path" || normalizeOpportunityToken(last.Type) != "event" {
			continue
		}
		if normalizeOpportunityToken(first.Value) == normalizedStart && normalizeOpportunityToken(last.Value) == normalizedEvent {
			return true
		}
	}
	return false
}

func setupGoalSuggestionOpportunity(definition OpportunityDefinition, input DetectorInput, candidate setupGoalCandidate, generatedAt time.Time) database.OpportunityInput {
	eventCount := strconv.Itoa(candidate.EventCount)
	score := scoreSetupGoalSuggestion(candidate.EventCount)
	opportunity := definition.BuildOpportunity(withGeneratedAt(input, generatedAt), OpportunityRecipe{
		CopyParams: map[string]any{
			"event_name":  candidate.EventName,
			"event_count": candidate.EventCount,
			"goal_type":   "event",
			"goal_value":  candidate.EventName,
		},
		ImpactValue:    eventCount,
		Confidence:     score.Confidence,
		Score:          score.Total,
		ScoreBreakdown: opportunityScoreAPI(score),
		RouteParams:    map[string]any{"event_name": candidate.EventName},
		Evidence: []api.OpportunityEvidence{
			{ID: "suggested_goal_event", LabelKey: "opportunities.evidence.suggested_goal_event", Value: candidate.EventName},
			{ID: "suggested_goal_event_count", LabelKey: "opportunities.evidence.suggested_goal_event_count", Value: eventCount},
		},
		CitedEvidenceIDs: []string{"suggested_goal_event", "suggested_goal_event_count"},
	})
	opportunity.ID = stableOpportunityID(input.SiteID, definition.Key+":"+normalizeOpportunityToken(candidate.EventName))
	return opportunity
}

func setupFunnelSuggestionOpportunity(definition OpportunityDefinition, input DetectorInput, candidate setupFunnelCandidate, generatedAt time.Time) database.OpportunityInput {
	eventCount := strconv.Itoa(candidate.EventCount)
	pageviews := strconv.Itoa(candidate.StartPageviews)
	stepCount := 2
	funnelSteps := candidate.StartPath + " -> " + candidate.ConversionEvent
	score := scoreSetupFunnelSuggestion(candidate.StartPageviews, candidate.EventCount)
	opportunity := definition.BuildOpportunity(withGeneratedAt(input, generatedAt), OpportunityRecipe{
		CopyParams: map[string]any{
			"start_path":       candidate.StartPath,
			"conversion_event": candidate.ConversionEvent,
			"event_count":      candidate.EventCount,
			"step_count":       stepCount,
			"funnel_steps":     funnelSteps,
		},
		ImpactValue:    strconv.Itoa(stepCount),
		Confidence:     score.Confidence,
		Score:          score.Total,
		ScoreBreakdown: opportunityScoreAPI(score),
		RouteParams:    map[string]any{"start_path": candidate.StartPath},
		Evidence: []api.OpportunityEvidence{
			{ID: "suggested_funnel_start", LabelKey: "opportunities.evidence.suggested_funnel_start", Value: candidate.StartPath},
			{ID: "suggested_funnel_start_pageviews", LabelKey: "opportunities.evidence.suggested_funnel_start_pageviews", Value: pageviews},
			{ID: "suggested_funnel_conversion_event", LabelKey: "opportunities.evidence.suggested_funnel_conversion_event", Value: candidate.ConversionEvent},
			{ID: "suggested_funnel_event_count", LabelKey: "opportunities.evidence.suggested_funnel_event_count", Value: eventCount},
		},
		CitedEvidenceIDs: []string{
			"suggested_funnel_start",
			"suggested_funnel_start_pageviews",
			"suggested_funnel_conversion_event",
			"suggested_funnel_event_count",
		},
	})
	identity := normalizeOpportunityToken(candidate.StartPath) + ":" + normalizeOpportunityToken(candidate.ConversionEvent)
	opportunity.ID = stableOpportunityID(input.SiteID, definition.Key+":"+identity)
	return opportunity
}

func normalizeOpportunityToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func withGeneratedAt(input DetectorInput, generatedAt time.Time) DetectorInput {
	input.GeneratedAt = generatedAt
	return input
}

func genericEventNames(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		name := strings.TrimSpace(value)
		if name == "" || isKnownConversionEvent(name) || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
		if len(out) == 3 {
			break
		}
	}
	return out
}

func hasKnownConversionEvent(values []string) bool {
	return slices.ContainsFunc(values, isKnownConversionEvent)
}

func isKnownConversionEvent(name string) bool {
	normalized := strings.ToLower(strings.TrimSpace(name))
	switch normalized {
	case "purchase", "purchase_completed", "order", "order_completed", "checkout_complete", "checkout_completed",
		"conversion", "lead", "signup", "sign_up", "trial_started", "demo_request", "demo_requested":
		return true
	default:
		return false
	}
}
