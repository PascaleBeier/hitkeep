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
	DetectorCategoryRevenue          DetectorCategory = "revenue"
	DetectorCategoryTrafficQuality   DetectorCategory = "traffic_quality"
	DetectorCategoryAIVisibility     DetectorCategory = "ai_visibility"
	DetectorCategorySearchVisibility DetectorCategory = "search_visibility"
	DetectorCategorySetupQuality     DetectorCategory = "setup_quality"
)

type DetectorInput struct {
	TeamID       uuid.UUID
	SiteID       uuid.UUID
	Stats        *api.SiteStats
	Ecommerce    *api.EcommerceSummary
	AIVisibility *api.AIFetchOverview
	GeneratedAt  time.Time
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
	Category      DetectorCategory
	TypeKey       string
	MessageKeys   DetectorMessageKeys
	AllowedParams []string
}

type Detector interface {
	Contract() DetectorContract
	Detect(DetectorInput) (*database.OpportunityInput, bool)
}

type DetectorCatalog struct {
	detectors []Detector
}

func NewDefaultDetectorCatalog() DetectorCatalog {
	return NewDetectorCatalog(
		checkoutDetector{},
		aiVisibilityDetector{},
		trafficQualityDetector{},
		trackingSetupDetector{},
	)
}

func NewDetectorCatalog(detectors ...Detector) DetectorCatalog {
	return DetectorCatalog{detectors: detectors}
}

func SupportedDetectorCategories() []DetectorCategory {
	return []DetectorCategory{
		DetectorCategoryConversion,
		DetectorCategoryRevenue,
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

func (c DetectorCatalog) ContractFor(typeKey string) (DetectorContract, bool) {
	for _, detector := range c.detectors {
		contract := detector.Contract()
		if contract.TypeKey == typeKey {
			return contract, true
		}
	}
	return DetectorContract{}, false
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

type checkoutDetector struct{}

func (checkoutDetector) Contract() DetectorContract {
	return checkoutOpportunityDefinition.Contract()
}

func (d checkoutDetector) Detect(input DetectorInput) (*database.OpportunityInput, bool) {
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
	opportunity := checkoutOpportunity(input, input.Ecommerce, score, input.GeneratedAt)
	return &opportunity, true
}

type aiVisibilityDetector struct{}

func (aiVisibilityDetector) Contract() DetectorContract {
	return aiVisibilityOpportunityDefinition.Contract()
}

func (d aiVisibilityDetector) Detect(input DetectorInput) (*database.OpportunityInput, bool) {
	if input.AIVisibility == nil || input.AIVisibility.TotalRequests <= 0 {
		return nil, false
	}
	opportunity := aiVisibilityOpportunity(input, input.AIVisibility, input.GeneratedAt)
	return &opportunity, true
}

type trafficQualityDetector struct{}

func (trafficQualityDetector) Contract() DetectorContract {
	return trafficQualityOpportunityDefinition.Contract()
}

func (d trafficQualityDetector) Detect(input DetectorInput) (*database.OpportunityInput, bool) {
	if input.Stats == nil || input.Stats.TotalPageviews <= 0 {
		return nil, false
	}
	opportunity := trafficQualityOpportunity(input, input.Stats, input.GeneratedAt)
	return &opportunity, true
}

type trackingSetupDetector struct{}

func (trackingSetupDetector) Contract() DetectorContract {
	return trackingSetupOpportunityDefinition.Contract()
}

func (d trackingSetupDetector) Detect(input DetectorInput) (*database.OpportunityInput, bool) {
	if hasOpportunitySignal(input) {
		return nil, false
	}
	opportunity := trackingSetupOpportunity(input, input.GeneratedAt)
	return &opportunity, true
}

func hasOpportunitySignal(input DetectorInput) bool {
	return (input.Ecommerce != nil && input.Ecommerce.CheckoutStarts > 0) ||
		(input.AIVisibility != nil && input.AIVisibility.TotalRequests > 0) ||
		(input.Stats != nil && input.Stats.TotalPageviews > 0)
}
