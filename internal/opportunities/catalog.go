package opportunities

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
)

type DetectorCategory string

const (
	DetectorCategoryConversion       DetectorCategory = "conversion"
	DetectorCategoryTraffic          DetectorCategory = "traffic"
	DetectorCategoryTrafficQuality   DetectorCategory = "traffic_quality"
	DetectorCategoryAIVisibility     DetectorCategory = "ai_visibility"
	DetectorCategorySearchVisibility DetectorCategory = "search_visibility"
	DetectorCategorySetupQuality     DetectorCategory = "setup_quality"
)

type OpportunitySignal string

const (
	OpportunitySignalSiteStats     OpportunitySignal = "site_stats"
	OpportunitySignalEcommerce     OpportunitySignal = "ecommerce"
	OpportunitySignalAIVisibility  OpportunitySignal = "ai_visibility"
	OpportunitySignalSearchConsole OpportunitySignal = "search_console"
	OpportunitySignalEvents        OpportunitySignal = "events"
	OpportunitySignalSetupEvidence OpportunitySignal = "setup_evidence"
)

type DetectorInput struct {
	TeamID        uuid.UUID
	SiteID        uuid.UUID
	Stats         *api.SiteStats
	Ecommerce     *api.EcommerceSummary
	AIVisibility  *api.AIFetchOverview
	SearchConsole *api.SearchConsoleOverview
	EventNames    []string
	SetupEvidence *SetupEvidenceSnapshot
	GeneratedAt   time.Time
}

type DetectorMessageKeys struct {
	Title       string
	Summary     string
	Action      string
	Digest      string
	ImpactLabel string
	RouteLabel  string
}

type DetectorContract struct {
	Category            DetectorCategory
	TypeKey             string
	MessageKeys         DetectorMessageKeys
	AllowedParams       []string
	ActionTypes         []string
	IdentityEvidenceIDs []string
	RequiredSignals     []OpportunitySignal
	OptionalSignals     []OpportunitySignal
}

type Detector interface {
	Contract() DetectorContract
	Detect(DetectorInput) (*database.OpportunityInput, bool)
}

type definitionBackedDetector interface {
	Detector
	Definition() OpportunityDefinition
}

type DetectorCatalog struct {
	detectors []Detector
}

func NewDefaultDetectorCatalog() DetectorCatalog {
	return NewDetectorCatalogFromDefinitions(DefaultOpportunityDefinitions()...)
}

func NewDetectorCatalog(detectors ...Detector) DetectorCatalog {
	return DetectorCatalog{detectors: detectors}
}

func NewDetectorCatalogFromDefinitions(definitions ...OpportunityDefinition) DetectorCatalog {
	definitions = copyOpportunityDefinitions(definitions)
	detectors := make([]Detector, 0, len(definitions))
	for _, definition := range definitions {
		detectors = append(detectors, definition.Detector())
	}
	return NewDetectorCatalog(detectors...)
}

func SupportedDetectorCategories() []DetectorCategory {
	return []DetectorCategory{
		DetectorCategoryConversion,
		DetectorCategoryTraffic,
		DetectorCategoryTrafficQuality,
		DetectorCategoryAIVisibility,
		DetectorCategorySearchVisibility,
		DetectorCategorySetupQuality,
	}
}

func (c DetectorCatalog) Detect(input DetectorInput) ([]database.OpportunityInput, error) {
	if input.GeneratedAt.IsZero() {
		input.GeneratedAt = time.Now().UTC()
	}
	out := []database.OpportunityInput{}
	for _, detector := range c.detectors {
		opportunity, ok := detector.Detect(input)
		if !ok {
			continue
		}
		if err := validateDetectorOutput(detector.Contract(), *opportunity); err != nil {
			return nil, err
		}
		out = append(out, *opportunity)
	}
	return out, nil
}

func (c DetectorCatalog) Contracts() []DetectorContract {
	contracts := make([]DetectorContract, 0, len(c.detectors))
	for _, detector := range c.detectors {
		contracts = append(contracts, detector.Contract())
	}
	return contracts
}

func (c DetectorCatalog) RequiredSignals() []OpportunitySignal {
	return c.signals(func(contract DetectorContract) []OpportunitySignal {
		return contract.RequiredSignals
	})
}

func (c DetectorCatalog) DetectionSignals() []OpportunitySignal {
	seen := map[OpportunitySignal]bool{}
	signals := []OpportunitySignal{}
	for _, detector := range c.detectors {
		contract := detector.Contract()
		for _, signal := range append(append([]OpportunitySignal(nil), contract.RequiredSignals...), contract.OptionalSignals...) {
			signals = appendSignalOnce(signals, seen, signal)
		}
	}
	return signals
}

func (c DetectorCatalog) signals(selectSignals func(DetectorContract) []OpportunitySignal) []OpportunitySignal {
	seen := map[OpportunitySignal]bool{}
	signals := []OpportunitySignal{}
	for _, detector := range c.detectors {
		for _, signal := range selectSignals(detector.Contract()) {
			signals = appendSignalOnce(signals, seen, signal)
		}
	}
	return signals
}

func appendSignalOnce(signals []OpportunitySignal, seen map[OpportunitySignal]bool, signal OpportunitySignal) []OpportunitySignal {
	if seen[signal] {
		return signals
	}
	seen[signal] = true
	return append(signals, signal)
}

func (c DetectorCatalog) ContractFor(typeKey string) (DetectorContract, bool) {
	for _, detector := range c.detectors {
		contract := detector.Contract()
		if contract.TypeKey == typeKey {
			return contract, true
		}
	}
	return DetectorContract{}, false
}

func (c DetectorCatalog) DefinitionFor(typeKey string) (OpportunityDefinition, bool) {
	for _, detector := range c.detectors {
		contract := detector.Contract()
		if contract.TypeKey != typeKey {
			continue
		}
		if definitionDetector, ok := detector.(definitionBackedDetector); ok {
			return definitionDetector.Definition(), true
		}
		return OpportunityDefinition{
			Kind:                "",
			Category:            contract.Category,
			TypeKey:             contract.TypeKey,
			MessageKeys:         contract.MessageKeys,
			AllowedParams:       append([]string(nil), contract.AllowedParams...),
			ActionTypes:         append([]string(nil), contract.ActionTypes...),
			IdentityEvidenceIDs: append([]string(nil), contract.IdentityEvidenceIDs...),
			RequiredSignals:     append([]OpportunitySignal(nil), contract.RequiredSignals...),
			OptionalSignals:     append([]OpportunitySignal(nil), contract.OptionalSignals...),
		}, true
	}
	return OpportunityDefinition{}, false
}

func validateDetectorOutput(contract DetectorContract, opportunity database.OpportunityInput) error {
	if err := validateDetectorContract(contract); err != nil {
		return err
	}
	if opportunity.TypeKey != contract.TypeKey {
		return fmt.Errorf("detector contract violation: type key %q is not declared", opportunity.TypeKey)
	}
	if !declaresMessageKeys(contract.MessageKeys, opportunity) {
		return fmt.Errorf("detector contract violation: undeclared message key for %q", opportunity.TypeKey)
	}
	if err := validateParams(contract, opportunity.CopyParams); err != nil {
		return err
	}
	if err := validateParams(contract, opportunity.RouteParams); err != nil {
		return err
	}
	return validateCitations(contract.TypeKey, opportunity.Evidence, opportunity.CitedEvidenceIDs)
}

func validateDetectorContract(contract DetectorContract) error {
	keys := []struct {
		field string
		key   string
	}{
		{field: "type", key: contract.TypeKey},
		{field: "title", key: contract.MessageKeys.Title},
		{field: "summary", key: contract.MessageKeys.Summary},
		{field: "action", key: contract.MessageKeys.Action},
		{field: "digest", key: contract.MessageKeys.Digest},
		{field: "impact_label", key: contract.MessageKeys.ImpactLabel},
		{field: "route_label", key: contract.MessageKeys.RouteLabel},
	}
	for _, item := range keys {
		if !isTranslationKey(item.key) {
			return fmt.Errorf("detector contract violation: %s must be a translation key", item.field)
		}
	}
	return nil
}

func declaresMessageKeys(keys DetectorMessageKeys, opportunity database.OpportunityInput) bool {
	return opportunity.TitleKey == keys.Title &&
		opportunity.SummaryKey == keys.Summary &&
		opportunity.ActionKey == keys.Action &&
		opportunity.DigestKey == keys.Digest &&
		opportunity.ImpactLabelKey == keys.ImpactLabel &&
		opportunity.RouteLabelKey == keys.RouteLabel
}

func validateParams(contract DetectorContract, params map[string]any) error {
	allowedParams := stringSet(contract.AllowedParams)
	for param := range params {
		if !allowedParams[param] {
			return fmt.Errorf("detector contract violation: undeclared param %q for %q", param, contract.TypeKey)
		}
	}
	return nil
}

func validateCitations(typeKey string, evidence []api.OpportunityEvidence, citedEvidenceIDs []string) error {
	evidenceIDs := map[string]bool{}
	for _, item := range evidence {
		if !isTranslationKey(item.LabelKey) {
			return fmt.Errorf("detector contract violation: evidence label must be a translation key for %q", typeKey)
		}
		evidenceIDs[item.ID] = true
	}
	for _, id := range citedEvidenceIDs {
		if !evidenceIDs[id] {
			return fmt.Errorf("detector contract violation: cited evidence %q is missing for %q", id, typeKey)
		}
	}
	return nil
}

func isTranslationKey(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed != "" && trimmed == value && strings.Contains(value, ".") && !strings.ContainsAny(value, " \t\r\n")
}

func stringSet(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}
