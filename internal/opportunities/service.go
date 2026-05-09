package opportunities

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"

	hitai "hitkeep/internal/ai"
	"hitkeep/internal/api"
	"hitkeep/internal/auth"
	"hitkeep/internal/database"
)

const detectorVersion = "opportunities-detectors-v1"

type Service struct {
	Shared  *database.Store
	AI      hitai.Client
	Catalog DetectorCatalog
}

type GenerateInput struct {
	TeamID                uuid.UUID
	Site                  api.Site
	Store                 *database.Store
	Audit                 *database.AuditEntryParams
	From                  time.Time
	To                    time.Time
	ActorID               uuid.UUID
	ActorType             string
	APIClientAuth         *database.APIClientAuth
	EffectiveUserID       uuid.UUID
	EffectiveInstanceRole auth.InstanceRole
	EffectiveSiteRole     auth.SiteRole
	SchedulerScope        SchedulerScope
}

type SchedulerScope struct {
	TeamID uuid.UUID
	SiteID uuid.UUID
}

func (s Service) Generate(ctx context.Context, input GenerateInput) ([]api.Opportunity, *uuid.UUID, string, error) {
	if s.Shared == nil {
		return nil, nil, "unavailable", fmt.Errorf("shared store is required")
	}
	if input.Store == nil {
		return nil, nil, "unavailable", fmt.Errorf("analytics store is required")
	}
	if actorType(input.ActorType) == "ai_scheduler" {
		if err := input.SchedulerScope.authorize(input.TeamID, input.Site.ID); err != nil {
			return nil, nil, "access_denied", err
		}
	}
	input = normalizeGenerateWindow(input)

	signals, err := loadOpportunitySignals(ctx, input)
	if err != nil {
		return nil, nil, "detector_failed", err
	}
	catalog := s.detectorCatalog()
	candidates, err := detectOpportunityCandidates(catalog, input, signals)
	if err != nil {
		return nil, nil, "detector_failed", err
	}
	runID, aiStatus := s.decorateCandidatesWithAI(ctx, input, catalog, candidates)
	stored, err := s.saveGeneratedOpportunities(ctx, candidates, input.Audit, aiStatus)
	if err != nil {
		return nil, runID, aiStatus, err
	}
	return stored, runID, aiStatus, nil
}

type opportunitySignals struct {
	Stats        *api.SiteStats
	Ecommerce    *api.EcommerceSummary
	AIVisibility *api.AIFetchOverview
}

func normalizeGenerateWindow(input GenerateInput) GenerateInput {
	if input.To.IsZero() {
		input.To = time.Now().UTC()
	}
	if input.From.IsZero() {
		input.From = input.To.AddDate(0, 0, -30)
	}
	return input
}

func loadOpportunitySignals(ctx context.Context, input GenerateInput) (opportunitySignals, error) {
	stats, err := input.Store.GetSiteStats(ctx, api.AnalyticsParams{SiteID: input.Site.ID, Start: input.From, End: input.To})
	if err != nil {
		return opportunitySignals{}, fmt.Errorf("load site stats: %w", err)
	}
	ecommerce, err := input.Store.GetEcommerceSummary(ctx, api.EcommerceParams{SiteID: input.Site.ID, Start: input.From, End: input.To})
	if err != nil {
		return opportunitySignals{}, fmt.Errorf("load ecommerce summary: %w", err)
	}
	aiVisibility, err := input.Store.GetAIFetchOverview(ctx, api.AIFetchQueryParams{SiteID: input.Site.ID, Start: input.From, End: input.To})
	if err != nil {
		return opportunitySignals{}, fmt.Errorf("load ai visibility: %w", err)
	}
	return opportunitySignals{Stats: stats, Ecommerce: ecommerce, AIVisibility: aiVisibility}, nil
}

func detectOpportunityCandidates(catalog DetectorCatalog, input GenerateInput, signals opportunitySignals) ([]database.OpportunityInput, error) {
	return catalog.Detect(DetectorInput{
		TeamID:       input.TeamID,
		SiteID:       input.Site.ID,
		Stats:        signals.Stats,
		Ecommerce:    signals.Ecommerce,
		AIVisibility: signals.AIVisibility,
		GeneratedAt:  time.Now().UTC(),
	})
}

func (s Service) decorateCandidatesWithAI(ctx context.Context, input GenerateInput, catalog DetectorCatalog, candidates []database.OpportunityInput) (*uuid.UUID, string) {
	aiStatus := "disabled"
	if s.AI == nil || !s.AI.Enabled() {
		return nil, aiStatus
	}
	aiStatus = "not_configured"
	if !s.AI.Configured() {
		return nil, aiStatus
	}
	if len(candidates) == 0 {
		return nil, "no_opportunities"
	}

	aiStatus = "success"
	var runID *uuid.UUID
	bridge := NewToolBridge(newOpportunityToolBridgeConfig(s.Shared, input))
	for i := range candidates {
		result, err := s.generateCandidateProposal(ctx, input, catalog, bridge, candidates[i])
		if err != nil {
			aiStatus = hitai.ClassifyError(err)
			continue
		}
		if runID == nil {
			id := result.RunID
			runID = &id
		}
		applyProposal(&candidates[i], result.Proposal, result.RunID)
	}
	return runID, aiStatus
}

func (s Service) generateCandidateProposal(ctx context.Context, input GenerateInput, catalog DetectorCatalog, bridge ToolBridge, candidate database.OpportunityInput) (hitai.OpportunityProposalResult, error) {
	copyContract, err := opportunityAICopyContract(input, candidate, catalog)
	if err != nil {
		return hitai.OpportunityProposalResult{}, err
	}
	result, err := s.AI.GenerateOpportunityProposal(ctx, hitai.OpportunityRequest{
		TeamID:           input.TeamID,
		SiteID:           input.Site.ID,
		ActorID:          input.ActorID,
		ActorType:        actorType(input.ActorType),
		DetectorInput:    copyContract,
		EvidenceSnapshot: opportunityEvidenceSnapshot(input, candidate),
		Tools:            bridge.Tools(),
	})
	if err != nil {
		return hitai.OpportunityProposalResult{}, err
	}
	if err := hitai.ValidateOpportunityCandidateProposal(result.Proposal, copyContract); err != nil {
		return hitai.OpportunityProposalResult{}, err
	}
	return result, nil
}

func newOpportunityToolBridgeConfig(shared *database.Store, input GenerateInput) ToolBridgeConfig {
	return ToolBridgeConfig{
		Shared:                shared,
		Analytics:             input.Store,
		TeamID:                input.TeamID,
		SiteID:                input.Site.ID,
		ActorID:               input.ActorID,
		ActorType:             actorType(input.ActorType),
		APIClientAuth:         input.APIClientAuth,
		EffectiveUserID:       input.EffectiveUserID,
		EffectiveInstanceRole: input.EffectiveInstanceRole,
		EffectiveSiteRole:     input.EffectiveSiteRole,
		SchedulerTeamID:       input.SchedulerScope.TeamID,
		SchedulerSiteID:       input.SchedulerScope.SiteID,
		From:                  input.From,
		To:                    input.To,
	}
}

func (s Service) saveGeneratedOpportunities(ctx context.Context, candidates []database.OpportunityInput, audit *database.AuditEntryParams, aiStatus string) ([]api.Opportunity, error) {
	upserts := append([]database.OpportunityInput(nil), candidates...)
	if audit == nil {
		return s.Shared.UpsertOpportunities(ctx, upserts)
	}
	annotateOpportunityAudit(audit, aiStatus)
	return s.Shared.UpsertOpportunitiesWithAudit(ctx, upserts, *audit)
}

func (s SchedulerScope) authorize(teamID, siteID uuid.UUID) error {
	if s.TeamID == uuid.Nil || s.SiteID == uuid.Nil {
		return fmt.Errorf("access denied")
	}
	if s.TeamID != teamID || s.SiteID != siteID {
		return fmt.Errorf("access denied")
	}
	return nil
}

func annotateOpportunityAudit(audit *database.AuditEntryParams, aiStatus string) {
	if audit == nil {
		return
	}
	aiStatus = strings.TrimSpace(aiStatus)
	if aiStatus == "" {
		aiStatus = "unknown"
	}
	audit.Details = appendAuditDetail(audit.Details, "ai_status="+aiStatus)
	if aiStatus != "success" && aiStatus != "disabled" && aiStatus != "not_configured" && aiStatus != "no_opportunities" {
		audit.Outcome = "degraded"
	}
}

func appendAuditDetail(details, addition string) string {
	details = strings.TrimSpace(details)
	addition = strings.TrimSpace(addition)
	if details == "" {
		return addition
	}
	if addition == "" || strings.Contains(details, addition) {
		return details
	}
	return details + "; " + addition
}

func (s Service) detectorCatalog() DetectorCatalog {
	if len(s.Catalog.detectors) > 0 {
		return s.Catalog
	}
	return NewDefaultDetectorCatalog()
}

func checkoutOpportunity(input DetectorInput, ecommerce *api.EcommerceSummary, score opportunityScoreBreakdown, generatedAt time.Time) database.OpportunityInput {
	missedOrders := math.Max(1, float64(ecommerce.CheckoutStarts-ecommerce.Orders)*0.08)
	upside := missedOrders * math.Max(ecommerce.AverageOrderValue, 80)
	opportunity := checkoutOpportunityDefinition.BaseOpportunity(input, generatedAt)
	opportunity.CopyParams = map[string]any{
		"checkout_starts": ecommerce.CheckoutStarts,
		"orders":          ecommerce.Orders,
		"conversion_rate": fmt.Sprintf("%.1f%%", ecommerce.CheckoutConversionRate),
		"monthly_upside":  formatMoney(upside, ecommerce.Currency),
		"currency":        ecommerce.Currency,
	}
	opportunity.ImpactValue = formatMoney(upside, ecommerce.Currency)
	opportunity.MonthlyUpside = upside
	opportunity.Confidence = score.Confidence
	opportunity.Score = score.Total
	opportunity.ScoreBreakdown = opportunityScoreAPI(score)
	opportunity.RouteParams = map[string]any{"path": "/checkout"}
	opportunity.Evidence = []api.OpportunityEvidence{
		{ID: "checkout_starts", LabelKey: "opportunities.evidence.checkout_starts", Value: fmt.Sprintf("%d", ecommerce.CheckoutStarts)},
		{ID: "orders", LabelKey: "opportunities.evidence.orders", Value: fmt.Sprintf("%d", ecommerce.Orders)},
		{ID: "conversion_rate", LabelKey: "opportunities.evidence.checkout_conversion_rate", Value: fmt.Sprintf("%.1f%%", ecommerce.CheckoutConversionRate)},
	}
	opportunity.CitedEvidenceIDs = []string{"checkout_starts", "orders", "conversion_rate"}
	return opportunity
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

func aiVisibilityOpportunity(input DetectorInput, aiVisibility *api.AIFetchOverview, generatedAt time.Time) database.OpportunityInput {
	path := topMetricName(aiVisibility.TopPaths, "/")
	opportunity := aiVisibilityOpportunityDefinition.BaseOpportunity(input, generatedAt)
	opportunity.CopyParams = map[string]any{
		"requests":     aiVisibility.TotalRequests,
		"unique_paths": aiVisibility.UniquePaths,
		"top_path":     path,
	}
	opportunity.ImpactValue = fmt.Sprintf("+%d", maxInt64(1, aiVisibility.UniquePaths))
	opportunity.MonthlyUpside = float64(aiVisibility.TotalRequests) * 8
	opportunity.Confidence = confidence(aiVisibility.TotalRequests >= 20)
	opportunity.Score = clampScore(62 + int(minInt64(aiVisibility.TotalRequests, 30)))
	opportunity.RouteParams = map[string]any{"path": path}
	opportunity.Evidence = []api.OpportunityEvidence{
		{ID: "ai_requests", LabelKey: "opportunities.evidence.ai_requests", Value: fmt.Sprintf("%d", aiVisibility.TotalRequests)},
		{ID: "ai_paths", LabelKey: "opportunities.evidence.ai_paths", Value: fmt.Sprintf("%d", aiVisibility.UniquePaths)},
		{ID: "top_ai_path", LabelKey: "opportunities.evidence.top_ai_path", Value: path},
	}
	opportunity.CitedEvidenceIDs = []string{"ai_requests", "ai_paths", "top_ai_path"}
	return opportunity
}

func trafficQualityOpportunity(input DetectorInput, stats *api.SiteStats, generatedAt time.Time) database.OpportunityInput {
	source := topMetricName(stats.TopUTMSources, topMetricName(stats.TopReferrers, "direct"))
	opportunity := trafficQualityOpportunityDefinition.BaseOpportunity(input, generatedAt)
	opportunity.CopyParams = map[string]any{
		"source":    source,
		"pageviews": stats.TotalPageviews,
		"sessions":  stats.UniqueSessions,
	}
	opportunity.ImpactValue = fmt.Sprintf("%d", stats.TotalPageviews)
	opportunity.MonthlyUpside = float64(stats.TotalPageviews) * 0.7
	opportunity.Confidence = confidence(stats.TotalPageviews >= 500)
	opportunity.Score = clampScore(55 + stats.TotalPageviews/100)
	opportunity.RouteParams = map[string]any{"source": source}
	opportunity.Evidence = []api.OpportunityEvidence{
		{ID: "pageviews", LabelKey: "opportunities.evidence.pageviews", Value: fmt.Sprintf("%d", stats.TotalPageviews)},
		{ID: "sessions", LabelKey: "opportunities.evidence.sessions", Value: fmt.Sprintf("%d", stats.UniqueSessions)},
		{ID: "top_source", LabelKey: "opportunities.evidence.top_source", Value: source},
	}
	opportunity.CitedEvidenceIDs = []string{"pageviews", "sessions", "top_source"}
	return opportunity
}

func trackingSetupOpportunity(input DetectorInput, generatedAt time.Time) database.OpportunityInput {
	opportunity := trackingSetupOpportunityDefinition.BaseOpportunity(input, generatedAt)
	opportunity.CopyParams = map[string]any{"pageviews": 0, "events": 0}
	opportunity.ImpactValue = "0"
	opportunity.MonthlyUpside = 0
	opportunity.Confidence = "medium"
	opportunity.Score = 45
	opportunity.RouteParams = map[string]any{"asset": "hk.js"}
	opportunity.Evidence = []api.OpportunityEvidence{
		{ID: "pageviews", LabelKey: "opportunities.evidence.pageviews", Value: "0"},
		{ID: "events", LabelKey: "opportunities.evidence.tracked_events", Value: "0"},
	}
	opportunity.CitedEvidenceIDs = []string{"pageviews", "events"}
	return opportunity
}

func opportunityAICopyContract(input GenerateInput, opportunity database.OpportunityInput, catalog DetectorCatalog) (hitai.OpportunityDetectorInput, error) {
	definition, ok := opportunityDefinitionFor(catalog, opportunity)
	if !ok {
		return hitai.OpportunityDetectorInput{}, fmt.Errorf("%w: unsupported opportunity type", hitai.ErrInvalidOutput)
	}
	return definition.AICopyContract(OpportunityCopyContext{
		SiteDomain: input.Site.Domain,
		From:       input.From,
		To:         input.To,
	}, opportunity), nil
}

func opportunityDefinitionFor(catalog DetectorCatalog, opportunity database.OpportunityInput) (OpportunityDefinition, bool) {
	if contract, ok := catalog.ContractFor(opportunity.TypeKey); ok {
		return OpportunityDefinition{
			Kind:          opportunity.Kind,
			Category:      contract.Category,
			TypeKey:       contract.TypeKey,
			MessageKeys:   contract.MessageKeys,
			AllowedParams: contract.AllowedParams,
			RouteIcon:     opportunity.RouteIcon,
		}, true
	}
	for _, definition := range DefaultOpportunityDefinitions() {
		if definition.TypeKey == opportunity.TypeKey {
			return definition, true
		}
	}
	return OpportunityDefinition{}, false
}

func opportunityEvidenceSnapshot(input GenerateInput, opportunity database.OpportunityInput) hitai.OpportunityEvidenceSnapshot {
	return hitai.OpportunityEvidenceSnapshot{
		SiteDomain: input.Site.Domain,
		From:       input.From,
		To:         input.To,
		Evidence:   aiEvidenceFromOpportunity(opportunity),
	}
}

func aiEvidenceFromOpportunity(opportunity database.OpportunityInput) []hitai.Evidence {
	evidence := make([]hitai.Evidence, 0, len(opportunity.Evidence))
	for _, item := range opportunity.Evidence {
		evidence = append(evidence, hitai.Evidence{
			ID:     item.ID,
			Label:  item.LabelKey,
			Value:  item.Value,
			Detail: item.DetailKey,
		})
	}
	return evidence
}

func applyProposal(opportunity *database.OpportunityInput, proposal hitai.OpportunityCandidateProposal, runID uuid.UUID) {
	opportunity.TitleKey = proposal.TitleKey
	opportunity.SummaryKey = proposal.SummaryKey
	opportunity.ActionKey = proposal.ActionKey
	opportunity.DigestKey = proposal.DigestKey
	opportunity.CitedEvidenceIDs = append([]string(nil), proposal.CitedEvidenceIDs...)
	opportunity.AIRunID = runID
}

func safeJSON(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func stableOpportunityID(siteID uuid.UUID, key string) uuid.UUID {
	return uuid.NewSHA1(siteID, []byte("hitkeep:opportunity:"+key))
}

func actorType(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "user"
	}
	return value
}

func confidence(high bool) string {
	if high {
		return "high"
	}
	return "medium"
}

func topMetricName(items []api.MetricStat, fallback string) string {
	for _, item := range items {
		if strings.TrimSpace(item.Name) != "" {
			return item.Name
		}
	}
	return fallback
}

func formatMoney(value float64, currency string) string {
	if currency == "" {
		currency = "USD"
	}
	if value >= 1000 {
		return fmt.Sprintf("%s %.1fk", currency, value/1000)
	}
	return fmt.Sprintf("%s %.0f", currency, value)
}

func clampScore(score int) int {
	if score < 1 {
		return 1
	}
	if score > 99 {
		return 99
	}
	return score
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
