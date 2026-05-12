package opportunities

import (
	"context"
	"fmt"
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
	catalog := s.detectorCatalog()

	signals, err := loadOpportunitySignals(ctx, s.Shared, input, catalog.DetectionSignals())
	if err != nil {
		return nil, nil, "detector_failed", err
	}
	candidates, err := detectOpportunityCandidates(catalog, input, signals)
	if err != nil {
		return nil, nil, "detector_failed", err
	}
	candidates, err = s.suppressDuplicateCandidates(ctx, input.Site.ID, catalog, candidates)
	if err != nil {
		return nil, nil, "unavailable", err
	}
	candidates = rankOpportunityInputs(candidates)
	runID, aiStatus := s.decorateCandidatesWithAI(ctx, input, catalog, candidates)
	stored, err := s.saveGeneratedOpportunities(ctx, candidates, input.Audit, aiStatus)
	if err != nil {
		return nil, runID, aiStatus, err
	}
	return RankOpportunities(stored), runID, aiStatus, nil
}

type opportunitySignals struct {
	Stats         *api.SiteStats
	Ecommerce     *api.EcommerceSummary
	AIVisibility  *api.AIFetchOverview
	SearchConsole *api.SearchConsoleOverview
	EventNames    []string
	SetupEvidence *SetupEvidenceSnapshot
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

func loadOpportunitySignals(ctx context.Context, shared *database.Store, input GenerateInput, required []OpportunitySignal) (opportunitySignals, error) {
	signals := opportunitySignals{}
	for _, signal := range required {
		switch signal {
		case OpportunitySignalSiteStats:
			if signals.Stats != nil {
				continue
			}
			stats, err := input.Store.GetSiteStats(ctx, api.AnalyticsParams{SiteID: input.Site.ID, Start: input.From, End: input.To})
			if err != nil {
				return opportunitySignals{}, fmt.Errorf("load site stats: %w", err)
			}
			signals.Stats = stats
		case OpportunitySignalEcommerce:
			if signals.Ecommerce != nil {
				continue
			}
			ecommerce, err := input.Store.GetEcommerceSummary(ctx, api.EcommerceParams{SiteID: input.Site.ID, Start: input.From, End: input.To})
			if err != nil {
				return opportunitySignals{}, fmt.Errorf("load ecommerce summary: %w", err)
			}
			signals.Ecommerce = ecommerce
		case OpportunitySignalAIVisibility:
			if signals.AIVisibility != nil {
				continue
			}
			aiVisibility, err := input.Store.GetAIFetchOverview(ctx, api.AIFetchQueryParams{SiteID: input.Site.ID, Start: input.From, End: input.To})
			if err != nil {
				return opportunitySignals{}, fmt.Errorf("load ai visibility: %w", err)
			}
			signals.AIVisibility = aiVisibility
		case OpportunitySignalSearchConsole:
			if signals.SearchConsole != nil {
				continue
			}
			searchConsole, err := loadSearchConsoleSignal(ctx, shared, input)
			if err != nil {
				return opportunitySignals{}, err
			}
			signals.SearchConsole = searchConsole
		case OpportunitySignalEvents:
			if signals.EventNames != nil {
				continue
			}
			eventNames, err := input.Store.GetEventNames(ctx, api.EventNamesParams{SiteID: input.Site.ID, Start: input.From, End: input.To})
			if err != nil {
				return opportunitySignals{}, fmt.Errorf("load event names: %w", err)
			}
			signals.EventNames = eventNames
		case OpportunitySignalSetupEvidence:
			if signals.SetupEvidence != nil {
				continue
			}
			snapshot, err := buildSetupEvidenceSnapshot(ctx, setupEvidenceSnapshotInput{
				SharedStore:    shared,
				AnalyticsStore: input.Store,
				SiteID:         input.Site.ID,
				From:           input.From,
				To:             input.To,
			})
			if err != nil {
				return opportunitySignals{}, fmt.Errorf("load setup evidence: %w", err)
			}
			signals.SetupEvidence = snapshot
		default:
			return opportunitySignals{}, fmt.Errorf("unknown opportunity signal %q", signal)
		}
	}
	return signals, nil
}

func loadSearchConsoleSignal(ctx context.Context, shared *database.Store, input GenerateInput) (*api.SearchConsoleOverview, error) {
	mapping, err := shared.GetGoogleSearchConsoleSiteMappingForTeam(ctx, input.Site.ID, input.TeamID)
	if err != nil {
		return nil, fmt.Errorf("load search console mapping: %w", err)
	}
	if mapping == nil || strings.TrimSpace(mapping.PropertyURI) == "" {
		return nil, nil
	}
	overview, err := input.Store.GetSearchConsoleOverview(ctx, api.SearchConsoleReportParams{
		SiteID:      input.Site.ID,
		PropertyURI: mapping.PropertyURI,
		Start:       input.From,
		End:         input.To,
	})
	if err != nil {
		return nil, fmt.Errorf("load search console overview: %w", err)
	}
	return &overview, nil
}

func detectOpportunityCandidates(catalog DetectorCatalog, input GenerateInput, signals opportunitySignals) ([]database.OpportunityInput, error) {
	return catalog.Detect(DetectorInput{
		TeamID:        input.TeamID,
		SiteID:        input.Site.ID,
		Stats:         signals.Stats,
		Ecommerce:     signals.Ecommerce,
		AIVisibility:  signals.AIVisibility,
		SearchConsole: signals.SearchConsole,
		EventNames:    append([]string(nil), signals.EventNames...),
		SetupEvidence: signals.SetupEvidence,
		GeneratedAt:   time.Now().UTC(),
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
	if definition, ok := catalog.DefinitionFor(opportunity.TypeKey); ok {
		if definition.Kind == "" {
			definition.Kind = opportunity.Kind
		}
		if definition.RouteIcon == "" {
			definition.RouteIcon = opportunity.RouteIcon
		}
		return definition, true
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

func (s Service) suppressDuplicateCandidates(ctx context.Context, siteID uuid.UUID, catalog DetectorCatalog, candidates []database.OpportunityInput) ([]database.OpportunityInput, error) {
	if len(candidates) == 0 {
		return candidates, nil
	}
	filtered := make([]database.OpportunityInput, 0, len(candidates))
	for _, candidate := range candidates {
		duplicate, err := s.isDuplicateCandidate(ctx, siteID, catalog, candidate)
		if err != nil {
			return nil, err
		}
		if duplicate {
			continue
		}
		filtered = append(filtered, candidate)
	}
	return filtered, nil
}

func (s Service) isDuplicateCandidate(ctx context.Context, siteID uuid.UUID, catalog DetectorCatalog, candidate database.OpportunityInput) (bool, error) {
	if candidate.ID == uuid.Nil {
		return false, nil
	}
	definition, ok := opportunityDefinitionFor(catalog, candidate)
	if !ok || len(definition.IdentityEvidenceIDs) == 0 {
		return false, nil
	}
	existing, err := s.Shared.GetOpportunity(ctx, siteID, candidate.ID)
	if err != nil {
		return false, fmt.Errorf("load existing opportunity for dedupe: %w", err)
	}
	if existing == nil || existing.TypeKey != candidate.TypeKey {
		return false, nil
	}
	if !sameOpportunityRoute(existing.RouteParams, candidate.RouteParams) {
		return false, nil
	}
	return sameOpportunityEvidenceIdentity(existing.Evidence, candidate.Evidence, definition.IdentityEvidenceIDs), nil
}

func sameOpportunityRoute(existing, candidate map[string]any) bool {
	existingJSON, existingErr := safeJSON(normalizeOpportunityRouteParams(existing))
	candidateJSON, candidateErr := safeJSON(normalizeOpportunityRouteParams(candidate))
	return existingErr == nil && candidateErr == nil && existingJSON == candidateJSON
}

func normalizeOpportunityRouteParams(params map[string]any) map[string]any {
	if len(params) == 0 {
		return map[string]any{}
	}
	return params
}

func sameOpportunityEvidenceIdentity(existing, candidate []api.OpportunityEvidence, identityIDs []string) bool {
	existingValues := opportunityEvidenceValues(existing)
	candidateValues := opportunityEvidenceValues(candidate)
	compared := false
	for _, id := range identityIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		compared = true
		existingValue, existingOK := existingValues[id]
		candidateValue, candidateOK := candidateValues[id]
		if !existingOK || !candidateOK || existingValue != candidateValue {
			return false
		}
	}
	return compared
}

func opportunityEvidenceValues(evidence []api.OpportunityEvidence) map[string]string {
	values := make(map[string]string, len(evidence))
	for _, item := range evidence {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		values[id] = strings.TrimSpace(item.Value)
	}
	return values
}

func applyProposal(opportunity *database.OpportunityInput, proposal hitai.OpportunityCandidateProposal, runID uuid.UUID) {
	opportunity.TitleKey = proposal.TitleKey
	opportunity.SummaryKey = proposal.SummaryKey
	opportunity.ActionKey = proposal.ActionKey
	opportunity.DigestKey = proposal.DigestKey
	opportunity.CitedEvidenceIDs = append([]string(nil), proposal.CitedEvidenceIDs...)
	opportunity.AIRunID = runID
}

func actorType(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "user"
	}
	return value
}
