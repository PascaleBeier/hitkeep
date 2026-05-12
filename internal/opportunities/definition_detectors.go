package opportunities

import (
	"fmt"
	"math"
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
	if _, ok := trafficOpportunitySource(input.Stats); !ok {
		return nil, false
	}
	opportunity := trafficQualityOpportunity(definition, input, input.Stats, input.GeneratedAt)
	return &opportunity, true
}

func detectSearchVisibilityOpportunity(input DetectorInput, definition OpportunityDefinition) (*database.OpportunityInput, bool) {
	if input.SearchConsole == nil || input.SearchConsole.Impressions <= 0 {
		return nil, false
	}
	score, clickUpside, ok := scoreSearchVisibilityOpportunity(searchVisibilityScoringInput{
		Impressions:     input.SearchConsole.Impressions,
		Clicks:          input.SearchConsole.Clicks,
		CTR:             input.SearchConsole.CTR,
		AveragePosition: input.SearchConsole.AveragePosition,
	})
	if !ok {
		return nil, false
	}
	opportunity := searchVisibilityOpportunity(definition, input, input.SearchConsole, score, clickUpside, input.GeneratedAt)
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

func detectTrackingSetupOpportunity(input DetectorInput, definition OpportunityDefinition) (*database.OpportunityInput, bool) {
	if hasOpportunitySignal(input) {
		return nil, false
	}
	opportunity := trackingSetupOpportunity(definition, input, input.GeneratedAt)
	return &opportunity, true
}

func hasOpportunitySignal(input DetectorInput) bool {
	return (input.Ecommerce != nil && input.Ecommerce.CheckoutStarts > 0) ||
		(input.AIVisibility != nil && input.AIVisibility.TotalRequests > 0) ||
		(input.SearchConsole != nil && input.SearchConsole.Impressions > 0) ||
		(input.Stats != nil && input.Stats.TotalPageviews > 0)
}

func checkoutOpportunity(definition OpportunityDefinition, input DetectorInput, ecommerce *api.EcommerceSummary, score opportunityScoreBreakdown, generatedAt time.Time) database.OpportunityInput {
	missedOrders := math.Max(1, float64(ecommerce.CheckoutStarts-ecommerce.Orders)*0.08)
	upside := missedOrders * math.Max(ecommerce.AverageOrderValue, 80)
	return definition.BuildOpportunity(withGeneratedAt(input, generatedAt), OpportunityRecipe{
		CopyParams: map[string]any{
			"checkout_starts": ecommerce.CheckoutStarts,
			"orders":          ecommerce.Orders,
			"conversion_rate": fmt.Sprintf("%.1f%%", ecommerce.CheckoutConversionRate),
			"monthly_upside":  formatMoney(upside, ecommerce.Currency),
			"currency":        ecommerce.Currency,
		},
		ImpactValue:    formatMoney(upside, ecommerce.Currency),
		MonthlyUpside:  upside,
		Confidence:     score.Confidence,
		Score:          score.Total,
		ScoreBreakdown: opportunityScoreAPI(score),
		RouteParams:    map[string]any{"path": "/checkout"},
		Evidence: []api.OpportunityEvidence{
			{ID: "checkout_starts", LabelKey: "opportunities.evidence.checkout_starts", Value: fmt.Sprintf("%d", ecommerce.CheckoutStarts)},
			{ID: "orders", LabelKey: "opportunities.evidence.orders", Value: fmt.Sprintf("%d", ecommerce.Orders)},
			{ID: "conversion_rate", LabelKey: "opportunities.evidence.checkout_conversion_rate", Value: fmt.Sprintf("%.1f%%", ecommerce.CheckoutConversionRate)},
		},
		CitedEvidenceIDs: []string{"checkout_starts", "orders", "conversion_rate"},
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
	return definition.BuildOpportunity(withGeneratedAt(input, generatedAt), OpportunityRecipe{
		CopyParams:       copyParams,
		ImpactValue:      fmt.Sprintf("+%d", maxInt64(1, aiVisibility.UniquePaths)),
		MonthlyUpside:    float64(aiVisibility.TotalRequests) * 8,
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
}

func aiVisibilityTrafficSupport(stats *api.SiteStats, path string) aiVisibilityTrafficSupportEvidence {
	if stats == nil {
		return aiVisibilityTrafficSupportEvidence{}
	}
	return aiVisibilityTrafficSupportEvidence{
		AIReferrals:      stats.AISourceVisits,
		TopPathPageviews: metricValueForName(stats.TopPages, path),
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

func trafficQualityOpportunity(definition OpportunityDefinition, input DetectorInput, stats *api.SiteStats, generatedAt time.Time) database.OpportunityInput {
	source, _ := trafficOpportunitySource(stats)
	return definition.BuildOpportunity(withGeneratedAt(input, generatedAt), OpportunityRecipe{
		CopyParams: map[string]any{
			"source":    source,
			"pageviews": stats.TotalPageviews,
			"sessions":  stats.UniqueSessions,
		},
		ImpactValue:   fmt.Sprintf("%d", stats.TotalPageviews),
		MonthlyUpside: float64(stats.TotalPageviews) * 0.7,
		Confidence:    confidence(stats.TotalPageviews >= 500),
		Score:         clampScore(55 + stats.TotalPageviews/100),
		RouteParams:   map[string]any{"source": source},
		Evidence: []api.OpportunityEvidence{
			{ID: "pageviews", LabelKey: "opportunities.evidence.pageviews", Value: fmt.Sprintf("%d", stats.TotalPageviews)},
			{ID: "sessions", LabelKey: "opportunities.evidence.sessions", Value: fmt.Sprintf("%d", stats.UniqueSessions)},
			{ID: "top_source", LabelKey: "opportunities.evidence.top_source", Value: source},
		},
		CitedEvidenceIDs: []string{"pageviews", "sessions", "top_source"},
	})
}

func trafficOpportunitySource(stats *api.SiteStats) (string, bool) {
	source := topActionableMetricName(stats.TopUTMSources)
	if source == "" {
		source = topActionableMetricName(stats.TopReferrers)
	}
	if source == "" {
		return "", false
	}
	return source, true
}

func topActionableMetricName(items []api.MetricStat) string {
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		if isActionableTrafficSource(name) {
			return name
		}
	}
	return ""
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
	clickUpside int,
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
			"estimated_clicks": clickUpside,
		},
		ImpactValue:    fmt.Sprintf("+%d", clickUpside),
		MonthlyUpside:  float64(clickUpside),
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
		ImpactValue:   fmt.Sprintf("%d", stats.TotalPageviews),
		MonthlyUpside: float64(stats.TotalPageviews) * 0.4,
		Confidence:    confidence(stats.UniqueSessions >= 500),
		Score:         score,
		RouteParams:   map[string]any{},
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
		MonthlyUpside:  0,
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
		MonthlyUpside:  0,
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

func trackingSetupOpportunity(definition OpportunityDefinition, input DetectorInput, generatedAt time.Time) database.OpportunityInput {
	return definition.BuildOpportunity(withGeneratedAt(input, generatedAt), OpportunityRecipe{
		CopyParams:    map[string]any{"pageviews": 0, "events": 0},
		ImpactValue:   "0",
		MonthlyUpside: 0,
		Confidence:    "medium",
		Score:         45,
		RouteParams:   map[string]any{"asset": "hk.js"},
		Evidence: []api.OpportunityEvidence{
			{ID: "pageviews", LabelKey: "opportunities.evidence.pageviews", Value: "0"},
			{ID: "events", LabelKey: "opportunities.evidence.tracked_events", Value: "0"},
		},
		CitedEvidenceIDs: []string{"pageviews", "events"},
	})
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
