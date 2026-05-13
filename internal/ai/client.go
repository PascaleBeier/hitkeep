package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	goaisdk "github.com/zendev-sh/goai"
	"github.com/zendev-sh/goai/provider"
)

const OpportunityTemplateVersion = "opportunities-v1"

var (
	ErrDisabled        = errors.New("ai disabled")
	ErrNotConfigured   = errors.New("ai not configured")
	ErrBudgetExhausted = errors.New("ai budget exhausted")
	ErrInvalidOutput   = errors.New("ai output failed validation")
	ErrAccessDenied    = errors.New("access denied")
)

type Config struct {
	Enabled             bool
	Provider            string
	Model               string
	BaseURL             string
	Region              string
	APIKey              string
	Timeout             time.Duration
	RequestLimit        int
	TokenLimit          int
	BudgetWindowMinutes int
	ConfigMode          string
}

type Usage struct {
	InputTokens   int
	OutputTokens  int
	TotalTokens   int
	ToolCallCount int
}

type LifecycleEvent struct {
	Type          string    `json:"type"`
	Provider      string    `json:"provider,omitempty"`
	Model         string    `json:"model,omitempty"`
	ToolName      string    `json:"tool_name,omitempty"`
	Step          int       `json:"step,omitempty"`
	Status        string    `json:"status,omitempty"`
	StatusCode    int       `json:"status_code,omitempty"`
	ErrorCategory string    `json:"error_category,omitempty"`
	LatencyMS     int64     `json:"latency_ms,omitempty"`
	MessageCount  int       `json:"message_count,omitempty"`
	ToolCount     int       `json:"tool_count,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
}

type RunRecord struct {
	ID              uuid.UUID
	TeamID          uuid.UUID
	SiteID          uuid.UUID
	ActorID         uuid.UUID
	ActorType       string
	Feature         string
	Provider        string
	Model           string
	TemplateVersion string
	EvidenceIDs     []string
	InputHash       string
	OutputHash      string
	OutputJSON      string
	Usage           Usage
	LifecycleEvents []LifecycleEvent
	Status          string
	ErrorCategory   string
	Latency         time.Duration
	CreatedAt       time.Time
}

type BudgetUsage struct {
	Requests int
	Tokens   int
}

type RunRecorder interface {
	RecordAIRun(context.Context, RunRecord) (uuid.UUID, error)
	ReserveAIRun(context.Context, RunRecord, time.Time, int, int) (uuid.UUID, error)
	GetAIUsageSince(context.Context, time.Time) (BudgetUsage, error)
}

type OpportunityRequest struct {
	TeamID           uuid.UUID
	SiteID           uuid.UUID
	ActorID          uuid.UUID
	ActorType        string
	DetectorInput    OpportunityDetectorInput
	EvidenceSnapshot OpportunityEvidenceSnapshot
	Tools            []goaisdk.Tool
}

type OpportunityCatalogRequest struct {
	TeamID    uuid.UUID
	SiteID    uuid.UUID
	ActorID   uuid.UUID
	ActorType string
	Catalog   OpportunityCandidateCatalog
	Tools     []goaisdk.Tool
}

type OpportunityEvidenceSnapshot struct {
	SiteDomain string                     `json:"site_domain"`
	From       time.Time                  `json:"from"`
	To         time.Time                  `json:"to"`
	Evidence   []Evidence                 `json:"evidence"`
	Context    map[string]json.RawMessage `json:"context,omitempty"`
}

type OpportunityDetectorInput struct {
	SiteDomain         string                     `json:"site_domain"`
	From               time.Time                  `json:"from"`
	To                 time.Time                  `json:"to"`
	TypeKey            string                     `json:"type_key"`
	Category           string                     `json:"category"`
	MessageKeys        OpportunityMessageKeys     `json:"message_keys"`
	AllowedParams      []string                   `json:"allowed_params"`
	AllowedActionTypes []string                   `json:"allowed_action_types,omitempty"`
	CopyParams         map[string]any             `json:"copy_params,omitempty"`
	Evidence           []Evidence                 `json:"evidence"`
	ImpactValue        string                     `json:"impact_value"`
	Confidence         string                     `json:"confidence"`
	Kind               string                     `json:"kind"`
	RouteParams        map[string]any             `json:"route_params,omitempty"`
	ToolContext        map[string]json.RawMessage `json:"tool_context,omitempty"`
}

type OpportunityMessageKeys struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Action  string `json:"action"`
	Digest  string `json:"digest"`
}

type Evidence struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Value  string `json:"value"`
	Detail string `json:"detail,omitempty"`
}

type OpportunityCandidateProposal struct {
	TypeKey          string         `json:"type_key" jsonschema:"description=The allowed opportunity type key."`
	Category         string         `json:"category" jsonschema:"description=The allowed opportunity category."`
	ActionType       string         `json:"action_type" jsonschema:"description=Stable action enum for the proposed next action."`
	Effort           string         `json:"effort" jsonschema:"description=Estimated implementation effort: low, medium, or high."`
	TitleKey         string         `json:"title_key" jsonschema:"description=One of the detector's allowed title message keys."`
	SummaryKey       string         `json:"summary_key" jsonschema:"description=One of the detector's allowed summary message keys."`
	ActionKey        string         `json:"action_key" jsonschema:"description=One of the detector's allowed action message keys."`
	DigestKey        string         `json:"digest_key" jsonschema:"description=One of the detector's allowed digest message keys."`
	CopyParams       map[string]any `json:"copy_params" jsonschema:"description=Interpolation params using only detector-allowed param names."`
	CitedEvidenceIDs []string       `json:"cited_evidence_ids" jsonschema:"description=Evidence ids used by every claim."`
}

type OpportunityCandidateCatalog struct {
	Candidates        []OpportunityDetectorInput  `json:"candidates"`
	EvidenceSnapshot  OpportunityEvidenceSnapshot `json:"evidence_snapshot"`
	AllowedCategories []string                    `json:"allowed_categories,omitempty"`
}

type OpportunityProposalResult struct {
	RunID    uuid.UUID
	Proposal OpportunityCandidateProposal
	Usage    Usage
}

type Client interface {
	GenerateOpportunityProposal(context.Context, OpportunityRequest) (OpportunityProposalResult, error)
	Configured() bool
	Enabled() bool
	Provider() string
	Model() string
}

type Service struct {
	conf     Config
	model    provider.LanguageModel
	recorder RunRecorder
}

func NewService(conf Config, recorder RunRecorder) (*Service, error) {
	conf.Provider = normalizeProvider(conf.Provider)
	conf.Model = strings.TrimSpace(conf.Model)
	conf.BaseURL = strings.TrimSpace(conf.BaseURL)
	conf.Region = strings.TrimSpace(conf.Region)
	conf.ConfigMode = strings.TrimSpace(conf.ConfigMode)
	if conf.ConfigMode == "" {
		conf.ConfigMode = "self_hosted"
	}
	if conf.Timeout <= 0 {
		conf.Timeout = 30 * time.Second
	}
	if conf.BudgetWindowMinutes <= 0 {
		conf.BudgetWindowMinutes = 1440
	}
	svc := &Service{conf: conf, recorder: recorder}
	if !conf.Enabled {
		return svc, nil
	}
	if err := ValidateConfig(conf); err != nil {
		return svc, err
	}
	model, err := buildModel(conf)
	if err != nil {
		return svc, err
	}
	svc.model = model
	return svc, nil
}

func ValidateConfig(conf Config) error {
	if strings.TrimSpace(conf.Model) == "" {
		return fmt.Errorf("%w: model is required", ErrNotConfigured)
	}
	providerKey := normalizeProvider(conf.Provider)
	if modelBuilders[providerKey] == nil {
		return fmt.Errorf("%w: unsupported provider %q", ErrNotConfigured, conf.Provider)
	}
	if providerRequiresBaseURL[providerKey] && strings.TrimSpace(conf.BaseURL) == "" {
		return fmt.Errorf("%w: base url is required for provider %q", ErrNotConfigured, conf.Provider)
	}
	if providerRequiresAPIKey[providerKey] && strings.TrimSpace(conf.APIKey) == "" {
		return fmt.Errorf("%w: api key is required for provider %q", ErrNotConfigured, conf.Provider)
	}
	return nil
}

func (s *Service) Enabled() bool {
	return s != nil && s.conf.Enabled
}

func (s *Service) Configured() bool {
	return s != nil && s.conf.Enabled && s.model != nil && s.conf.Provider != "" && s.conf.Model != ""
}

func (s *Service) Provider() string {
	if s == nil {
		return ""
	}
	return s.conf.Provider
}

func (s *Service) Model() string {
	if s == nil {
		return ""
	}
	return s.conf.Model
}

func (s *Service) GenerateOpportunityProposal(ctx context.Context, req OpportunityRequest) (OpportunityProposalResult, error) {
	if s == nil || !s.conf.Enabled {
		return OpportunityProposalResult{}, ErrDisabled
	}
	ledger := newRunLedger(s.conf, s.recorder)
	if !s.Configured() {
		if err := ledger.recordNotConfigured(ctx, req); err != nil {
			return OpportunityProposalResult{}, err
		}
		return OpportunityProposalResult{}, ErrNotConfigured
	}
	reservedRunID, err := ledger.reserve(ctx, req)
	if err != nil {
		return OpportunityProposalResult{}, err
	}

	generation := s.runOpportunityGeneration(ctx, req)
	runID, err := ledger.finalizeGeneration(ctx, reservedRunID, req, generation)
	if err != nil {
		return OpportunityProposalResult{}, err
	}
	return OpportunityProposalResult{RunID: runID, Proposal: generation.Output, Usage: generation.Usage}, nil
}

func (s *Service) GenerateOpportunityCatalogCandidateProposal(ctx context.Context, req OpportunityCatalogRequest) (OpportunityProposalResult, error) {
	if s == nil || !s.conf.Enabled {
		return OpportunityProposalResult{}, ErrDisabled
	}
	auditReq := catalogOpportunityRequest(req)
	ledger := newRunLedger(s.conf, s.recorder)
	if !s.Configured() {
		if err := ledger.recordNotConfigured(ctx, auditReq); err != nil {
			return OpportunityProposalResult{}, err
		}
		return OpportunityProposalResult{}, ErrNotConfigured
	}
	reservedRunID, err := ledger.reserve(ctx, auditReq)
	if err != nil {
		return OpportunityProposalResult{}, err
	}

	generation := s.runOpportunityCatalogGeneration(ctx, req)
	runID, err := ledger.finalizeGeneration(ctx, reservedRunID, auditReq, generation)
	if err != nil {
		return OpportunityProposalResult{}, err
	}
	return OpportunityProposalResult{RunID: runID, Proposal: generation.Output, Usage: generation.Usage}, nil
}

type opportunityGeneration struct {
	Output          OpportunityCandidateProposal
	Usage           Usage
	LifecycleEvents []LifecycleEvent
	Latency         time.Duration
	Err             error
}

func (s *Service) runOpportunityGeneration(ctx context.Context, req OpportunityRequest) opportunityGeneration {
	return s.runOpportunityProposalGeneration(
		ctx,
		opportunityGenerationInput(req),
		opportunityProposalSchema(req.DetectorInput),
		req.Tools,
		strictOpportunityCandidateProposalResult,
		func(proposal OpportunityCandidateProposal) error {
			return ValidateOpportunityCandidateProposal(proposal, opportunityProposalValidationInput(req))
		},
	)
}

func (s *Service) runOpportunityCatalogGeneration(ctx context.Context, req OpportunityCatalogRequest) opportunityGeneration {
	return s.runOpportunityProposalGeneration(
		ctx,
		opportunityCatalogGenerationInput(req),
		opportunityCatalogProposalSchema(req.Catalog),
		req.Tools,
		strictOpportunityCatalogCandidateProposalResult,
		func(proposal OpportunityCandidateProposal) error {
			_, err := ValidateOpportunityCatalogCandidateProposal(proposal, req.Catalog)
			return err
		},
	)
}

func (s *Service) runOpportunityProposalGeneration(ctx context.Context, promptInput any, schema json.RawMessage, tools []goaisdk.Tool, decodeResult func(*goaisdk.ObjectResult[OpportunityCandidateProposal]) (OpportunityCandidateProposal, error), validate func(OpportunityCandidateProposal) error) opportunityGeneration {
	inputJSON, err := json.Marshal(promptInput)
	if err != nil {
		return opportunityGeneration{Err: fmt.Errorf("encode opportunity prompt input: %w", err)}
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, s.conf.Timeout)
	defer cancel()

	usage := Usage{}
	lifecycleEvents := []LifecycleEvent{}
	appendLifecycle := func(event LifecycleEvent) {
		if event.Provider == "" {
			event.Provider = s.conf.Provider
		}
		if event.Model == "" {
			event.Model = s.conf.Model
		}
		if event.Timestamp.IsZero() {
			event.Timestamp = time.Now().UTC()
		}
		lifecycleEvents = append(lifecycleEvents, event)
	}
	started := time.Now()
	var output OpportunityCandidateProposal
	result, err := goaisdk.GenerateObject[OpportunityCandidateProposal](timeoutCtx, s.model,
		goaisdk.WithSystem(opportunitySystemPrompt()),
		goaisdk.WithPrompt(opportunityPrompt(string(inputJSON))),
		goaisdk.WithExplicitSchema(schema),
		goaisdk.WithTools(tools...),
		goaisdk.WithMaxSteps(3),
		goaisdk.WithMaxOutputTokens(900),
		goaisdk.WithTemperature(0.2),
		goaisdk.WithOnRequest(func(info goaisdk.RequestInfo) {
			appendLifecycle(LifecycleEvent{
				Type:         "request_start",
				Model:        info.Model,
				MessageCount: info.MessageCount,
				ToolCount:    info.ToolCount,
				Status:       "started",
				Timestamp:    info.Timestamp,
			})
		}),
		goaisdk.WithOnResponse(func(info goaisdk.ResponseInfo) {
			usage.InputTokens += info.Usage.InputTokens
			usage.OutputTokens += info.Usage.OutputTokens
			usage.TotalTokens += totalTokens(info.Usage)
			status := "success"
			category := ""
			if info.Error != nil {
				status = "failure"
				category = ClassifyError(info.Error)
			}
			appendLifecycle(LifecycleEvent{
				Type:          "request_finish",
				Status:        status,
				StatusCode:    info.StatusCode,
				ErrorCategory: category,
				LatencyMS:     info.Latency.Milliseconds(),
			})
		}),
		goaisdk.WithOnToolCallStart(func(info goaisdk.ToolCallStartInfo) {
			appendLifecycle(LifecycleEvent{
				Type:     "tool_call_start",
				ToolName: info.ToolName,
				Step:     info.Step,
				Status:   "started",
			})
		}),
		goaisdk.WithOnToolCall(func(info goaisdk.ToolCallInfo) {
			usage.ToolCallCount++
			status := "success"
			category := ""
			if info.Error != nil {
				status = "failure"
				category = ClassifyError(info.Error)
			}
			appendLifecycle(LifecycleEvent{
				Type:          "tool_call_finish",
				ToolName:      info.ToolName,
				Step:          info.Step,
				Status:        status,
				ErrorCategory: category,
				LatencyMS:     info.Duration.Milliseconds(),
			})
		}),
	)
	latency := time.Since(started)
	output, usage, err = finalizeOpportunityGeneration(result, usage, err, decodeResult, validate)
	return opportunityGeneration{Output: output, Usage: usage, LifecycleEvents: lifecycleEvents, Latency: latency, Err: err}
}

type opportunityGenerationPromptInput struct {
	CandidateContract OpportunityDetectorInput    `json:"candidate_contract"`
	EvidenceSnapshot  OpportunityEvidenceSnapshot `json:"evidence_snapshot"`
}

type opportunityCatalogGenerationPromptInput struct {
	CandidateCatalog  []OpportunityDetectorInput  `json:"candidate_catalog"`
	EvidenceSnapshot  OpportunityEvidenceSnapshot `json:"evidence_snapshot"`
	AllowedCategories []string                    `json:"allowed_categories,omitempty"`
}

func opportunityGenerationInput(req OpportunityRequest) opportunityGenerationPromptInput {
	candidate := req.DetectorInput
	candidate.Evidence = nil
	return opportunityGenerationPromptInput{
		CandidateContract: candidate,
		EvidenceSnapshot:  opportunityAuditInput(req),
	}
}

func opportunityCatalogGenerationInput(req OpportunityCatalogRequest) opportunityCatalogGenerationPromptInput {
	activeCandidates := catalogSchemaCandidates(req.Catalog)
	candidates := make([]OpportunityDetectorInput, 0, len(activeCandidates))
	for _, candidate := range activeCandidates {
		candidate.Evidence = nil
		candidates = append(candidates, candidate)
	}
	return opportunityCatalogGenerationPromptInput{
		CandidateCatalog:  candidates,
		EvidenceSnapshot:  req.Catalog.EvidenceSnapshot,
		AllowedCategories: append([]string(nil), req.Catalog.AllowedCategories...),
	}
}

func catalogOpportunityRequest(req OpportunityCatalogRequest) OpportunityRequest {
	return OpportunityRequest{
		TeamID:           req.TeamID,
		SiteID:           req.SiteID,
		ActorID:          req.ActorID,
		ActorType:        req.ActorType,
		EvidenceSnapshot: req.Catalog.EvidenceSnapshot,
	}
}

func opportunityProposalValidationInput(req OpportunityRequest) OpportunityDetectorInput {
	input := req.DetectorInput
	if snapshot := opportunityAuditInput(req); len(snapshot.Evidence) > 0 {
		input.Evidence = snapshot.Evidence
	}
	return input
}

func finalizeOpportunityGeneration(result *goaisdk.ObjectResult[OpportunityCandidateProposal], usage Usage, err error, decodeResult func(*goaisdk.ObjectResult[OpportunityCandidateProposal]) (OpportunityCandidateProposal, error), validate func(OpportunityCandidateProposal) error) (OpportunityCandidateProposal, Usage, error) {
	var output OpportunityCandidateProposal
	if err != nil {
		return output, usage, err
	}
	if result == nil {
		return output, usage, fmt.Errorf("%w: missing provider result", ErrInvalidOutput)
	}
	output, err = decodeResult(result)
	if err == nil {
		err = validate(output)
	}
	return output, finalizedUsage(result, usage), err
}

func finalizedUsage(result *goaisdk.ObjectResult[OpportunityCandidateProposal], usage Usage) Usage {
	if usage.TotalTokens > 0 {
		return usage
	}
	return Usage{
		InputTokens:   result.Usage.InputTokens,
		OutputTokens:  result.Usage.OutputTokens,
		TotalTokens:   totalTokens(result.Usage),
		ToolCallCount: usage.ToolCallCount,
	}
}

func strictOpportunityCandidateProposalResult(result *goaisdk.ObjectResult[OpportunityCandidateProposal]) (OpportunityCandidateProposal, error) {
	return strictOpportunityProposalResult(result, decodeOpportunityCandidateProposalJSON)
}

func strictOpportunityCatalogCandidateProposalResult(result *goaisdk.ObjectResult[OpportunityCandidateProposal]) (OpportunityCandidateProposal, error) {
	return strictOpportunityProposalResult(result, decodeOpportunityCatalogCandidateProposalJSON)
}

func strictOpportunityProposalResult(result *goaisdk.ObjectResult[OpportunityCandidateProposal], decode func([]byte) (OpportunityCandidateProposal, error)) (OpportunityCandidateProposal, error) {
	for i := len(result.Steps) - 1; i >= 0; i-- {
		if text := strings.TrimSpace(result.Steps[i].Text); text != "" {
			return decode([]byte(text))
		}
	}
	return result.Object, nil
}

func opportunitySystemPrompt() string {
	return "You propose analytics opportunities by returning structured candidate metadata, message keys, interpolation params, and cited evidence ids only. Do not write customer-facing prose. Do not make money claims, revenue-upside claims, or financial promises. Do not make causal claims. Do not infer source-specific traffic from total pageviews; source claims require source-specific evidence. Use only allowed keys and params from the candidate contract. Every claim must be supported by cited_evidence_ids. Return the requested JSON object only."
}

func opportunityPrompt(input string) string {
	return "Propose an evidence-backed opportunity candidate from this contract and evidence snapshot. You may call the available read-only tools to verify aggregate context, but the final object must contain only allowed metadata, message keys, interpolation params, and cited evidence ids. Do not make money claims. Do not make causal claims. Do not infer source-specific traffic from total pageviews. Preserve the opportunity intent and do not add unsupported claims:\n\n" + input
}

func mustRawJSON(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return raw
}

func totalTokens(usage provider.Usage) int {
	if usage.TotalTokens > 0 {
		return usage.TotalTokens
	}
	return usage.InputTokens + usage.OutputTokens + usage.ReasoningTokens + usage.CacheReadTokens + usage.CacheWriteTokens
}

func evidenceIDs(evidence []Evidence) []string {
	ids := make([]string, 0, len(evidence))
	for _, item := range evidence {
		if id := strings.TrimSpace(item.ID); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func HashAny(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return HashString("")
	}
	return HashString(string(raw))
}

func HashString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func mustJSON(value any) string {
	if value == nil {
		return "{}"
	}
	raw, err := json.Marshal(value)
	if err != nil || !json.Valid(raw) {
		return "{}"
	}
	return string(raw)
}

func ClassifyError(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, ErrDisabled):
		return "disabled"
	case errors.Is(err, ErrNotConfigured):
		return "not_configured"
	case errors.Is(err, ErrBudgetExhausted):
		return "budget_exhausted"
	case errors.Is(err, ErrInvalidOutput):
		return "invalid_output"
	case errors.Is(err, ErrAccessDenied):
		return "access_denied"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	default:
		msg := strings.ToLower(err.Error())
		switch {
		case strings.Contains(msg, "access denied"):
			return "access_denied"
		case strings.Contains(msg, "unauthorized"), strings.Contains(msg, "forbidden"), strings.Contains(msg, "api key"), strings.Contains(msg, "credential"):
			return "auth_failed"
		case strings.Contains(msg, "rate limit"), strings.Contains(msg, "429"):
			return "rate_limited"
		case strings.Contains(msg, "timeout"):
			return "timeout"
		default:
			return "provider_error"
		}
	}
}
