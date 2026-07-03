package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	goaisdk "github.com/zendev-sh/goai"
	"github.com/zendev-sh/goai/provider"
)

type AskAIRequest struct {
	TeamID     uuid.UUID
	SiteID     uuid.UUID
	ActorID    uuid.UUID
	ActorType  string
	SiteDomain string
	Query      string
	From       time.Time
	To         time.Time
	Route      string
	Filters    []AskAIFilter
	History    []AskAIMessage
	SkillText  string
	Tools      []goaisdk.Tool
}

type AskAIFilter struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type AskAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type AskAIOutput struct {
	AnswerMarkdown string          `json:"answer_markdown" jsonschema:"description=Concise markdown answer grounded in HitKeep aggregate tool output."`
	Citations      []AskAICitation `json:"citations" jsonschema:"description=Tool evidence references used by the answer."`
	Charts         []AskAIChart    `json:"charts" jsonschema:"description=Optional charts or tables built only from tool output."`
	Actions        []AskAIAction   `json:"actions" jsonschema:"description=Optional dashboard navigation or export actions."`
}

type AskAICitation struct {
	Label      string `json:"label"`
	ToolCallID string `json:"tool_call_id"`
}

type AskAIChart struct {
	Type   string             `json:"type"`
	Title  string             `json:"title"`
	XKey   string             `json:"x_key,omitempty"`
	Series []AskAIChartSeries `json:"series,omitempty"`
	Rows   []map[string]any   `json:"rows"`
}

type AskAIChartSeries struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

type AskAIAction struct {
	Type   string `json:"type"`
	Label  string `json:"label"`
	Target string `json:"target"`
	Format string `json:"format,omitempty"`
}

type AskAIResult struct {
	RunID  uuid.UUID
	Output AskAIOutput
	Usage  Usage
}

const (
	AskAIStreamDeltaAnswer   = "answer"
	AskAIStreamDeltaProgress = "progress"
)

type AskAIStreamDelta struct {
	Type       string
	TextDelta  string
	MessageKey string
	Status     string
	ToolCallID string
	ToolName   string
}

type AskAIStreamSink func(AskAIStreamDelta) error

type askAIGeneration struct {
	Output          AskAIOutput
	Usage           Usage
	EvidenceIDs     []string
	LifecycleEvents []LifecycleEvent
	Latency         time.Duration
	Err             error
}

type askAIPromptInput struct {
	SiteDomain string         `json:"site_domain"`
	From       time.Time      `json:"from"`
	To         time.Time      `json:"to"`
	Route      string         `json:"route,omitempty"`
	Query      string         `json:"query"`
	Filters    []AskAIFilter  `json:"filters,omitempty"`
	History    []AskAIMessage `json:"history,omitempty"`
	ToolNames  []string       `json:"tool_names"`
}

func (s *Service) GenerateAskAI(ctx context.Context, req AskAIRequest) (AskAIResult, error) {
	if s == nil || !s.conf.Enabled {
		return AskAIResult{}, ErrDisabled
	}
	req = normalizeAskAIRequest(req)
	ledger := newRunLedger(s.conf, s.recorder)
	if !s.Configured() {
		if err := ledger.recordAskAINotConfigured(ctx, req); err != nil {
			return AskAIResult{}, err
		}
		return AskAIResult{}, ErrNotConfigured
	}
	reservedRunID, err := ledger.reserveAskAI(ctx, req)
	if err != nil {
		return AskAIResult{}, err
	}
	generation := s.runAskAIGeneration(ctx, req)
	runID, err := ledger.finalizeAskAI(ctx, reservedRunID, req, generation)
	if err != nil {
		return AskAIResult{RunID: runID, Usage: generation.Usage}, err
	}
	return AskAIResult{RunID: runID, Output: generation.Output, Usage: generation.Usage}, nil
}

func (s *Service) StreamAskAI(ctx context.Context, req AskAIRequest, sink AskAIStreamSink) (AskAIResult, error) {
	if s == nil || !s.conf.Enabled {
		return AskAIResult{}, ErrDisabled
	}
	req = normalizeAskAIRequest(req)
	ledger := newRunLedger(s.conf, s.recorder)
	if !s.Configured() {
		if err := ledger.recordAskAINotConfigured(ctx, req); err != nil {
			return AskAIResult{}, err
		}
		return AskAIResult{}, ErrNotConfigured
	}
	reservedRunID, err := ledger.reserveAskAI(ctx, req)
	if err != nil {
		return AskAIResult{}, err
	}
	generation := s.runAskAIStreamingGeneration(ctx, req, sink)
	runID, err := ledger.finalizeAskAI(ctx, reservedRunID, req, generation)
	if err != nil {
		return AskAIResult{RunID: runID, Usage: generation.Usage}, err
	}
	return AskAIResult{RunID: runID, Output: generation.Output, Usage: generation.Usage}, nil
}

func (s *Service) runAskAIGeneration(ctx context.Context, req AskAIRequest) askAIGeneration {
	inputJSON, err := json.Marshal(askAIGenerationInput(req))
	if err != nil {
		return askAIGeneration{Err: fmt.Errorf("encode ask ai prompt input: %w", err)}
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, s.conf.Timeout)
	defer cancel()

	usage := Usage{}
	lifecycleEvents := []LifecycleEvent{}
	successfulToolCalls := map[string]bool{}
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
	temperatureOpts := temperatureOptions(s.model, 0.2)
	structuredOutputOpts := mantleStructuredOutputOptions(s.conf)
	toolChoiceOpts := mantleAskAIToolOptions(s.conf, req.Tools)
	options := make([]goaisdk.Option, 0, 10+len(temperatureOpts)+len(structuredOutputOpts)+len(toolChoiceOpts))
	options = append(options,
		goaisdk.WithSystem(askAISystemPrompt(req.SkillText)),
		goaisdk.WithPrompt(askAIPrompt(string(inputJSON))),
		goaisdk.WithExplicitSchema(askAIOutputSchema()),
		goaisdk.WithTools(req.Tools...),
		goaisdk.WithMaxSteps(5),
		goaisdk.WithMaxOutputTokens(1800),
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
			} else if name := strings.TrimSpace(info.ToolName); name != "" {
				successfulToolCalls[name] = true
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
	options = append(options, temperatureOpts...)
	options = append(options, structuredOutputOpts...)
	options = append(options, toolChoiceOpts...)
	result, err := goaisdk.GenerateObject[AskAIOutput](timeoutCtx, s.model, options...)
	latency := time.Since(started)
	evidenceIDs := askAIToolEvidenceIDs(successfulToolCalls)
	output, usage, err := finalizeAskAIGeneration(result, usage, err, req, evidenceIDs)
	return askAIGeneration{Output: output, Usage: usage, EvidenceIDs: evidenceIDs, LifecycleEvents: lifecycleEvents, Latency: latency, Err: err}
}

func (s *Service) runAskAIStreamingGeneration(ctx context.Context, req AskAIRequest, sink AskAIStreamSink) askAIGeneration {
	inputJSON, err := json.Marshal(askAIGenerationInput(req))
	if err != nil {
		return askAIGeneration{Err: fmt.Errorf("encode ask ai prompt input: %w", err)}
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, s.conf.Timeout)
	defer cancel()

	var mu sync.Mutex
	usage := Usage{}
	lifecycleEvents := []LifecycleEvent{}
	successfulToolCalls := map[string]bool{}
	var sinkErr error
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
		mu.Lock()
		defer mu.Unlock()
		lifecycleEvents = append(lifecycleEvents, event)
	}
	addUsage := func(providerUsage provider.Usage) {
		mu.Lock()
		defer mu.Unlock()
		usage.InputTokens += providerUsage.InputTokens
		usage.OutputTokens += providerUsage.OutputTokens
		usage.TotalTokens += totalTokens(providerUsage)
	}
	incrementToolCallUsage := func() {
		mu.Lock()
		defer mu.Unlock()
		usage.ToolCallCount++
	}
	recordSuccessfulToolCall := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		successfulToolCalls[name] = true
	}
	currentSinkErr := func() error {
		mu.Lock()
		defer mu.Unlock()
		return sinkErr
	}
	recordSinkErr := func(err error) {
		if err == nil {
			return
		}
		mu.Lock()
		if sinkErr == nil {
			sinkErr = err
			cancel()
		}
		mu.Unlock()
	}
	emit := func(delta AskAIStreamDelta) error {
		if sink == nil {
			return nil
		}
		return sink(delta)
	}

	started := time.Now()
	failedGeneration := func(err error) askAIGeneration {
		mu.Lock()
		usageSnapshot := usage
		lifecycleSnapshot := slices.Clone(lifecycleEvents)
		mu.Unlock()
		return askAIGeneration{Usage: usageSnapshot, LifecycleEvents: lifecycleSnapshot, Latency: time.Since(started), Err: err}
	}
	generationSnapshot := func() (Usage, []LifecycleEvent, []string) {
		mu.Lock()
		defer mu.Unlock()
		return usage, slices.Clone(lifecycleEvents), askAIToolEvidenceIDs(successfulToolCalls)
	}
	temperatureOpts := temperatureOptions(s.model, 0.2)
	toolChoiceOpts := mantleAskAIToolOptions(s.conf, req.Tools)
	options := make([]goaisdk.Option, 0, 9+len(temperatureOpts)+len(toolChoiceOpts))
	options = append(options,
		goaisdk.WithSystem(askAISystemPrompt(req.SkillText)),
		goaisdk.WithPrompt(askAIStreamingPrompt(string(inputJSON))),
		goaisdk.WithTools(req.Tools...),
		goaisdk.WithMaxSteps(5),
		goaisdk.WithMaxOutputTokens(1800),
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
			addUsage(info.Usage)
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
			if err := emit(AskAIStreamDelta{Type: AskAIStreamDeltaProgress, Status: "tool_call_start", MessageKey: "askAi.progress.readingAnalytics", ToolCallID: info.ToolCallID, ToolName: info.ToolName}); err != nil {
				recordSinkErr(err)
			}
		}),
		goaisdk.WithOnToolCall(func(info goaisdk.ToolCallInfo) {
			incrementToolCallUsage()
			status := "success"
			category := ""
			if info.Error != nil {
				status = "failure"
				category = ClassifyError(info.Error)
			} else {
				recordSuccessfulToolCall(info.ToolName)
			}
			appendLifecycle(LifecycleEvent{
				Type:          "tool_call_finish",
				ToolName:      info.ToolName,
				Step:          info.Step,
				Status:        status,
				ErrorCategory: category,
				LatencyMS:     info.Duration.Milliseconds(),
			})
			if err := emit(AskAIStreamDelta{Type: AskAIStreamDeltaProgress, Status: "tool_call_finish", MessageKey: "askAi.progress.composing", ToolCallID: info.ToolCallID, ToolName: info.ToolName}); err != nil {
				recordSinkErr(err)
			}
		}),
	)
	options = append(options, temperatureOpts...)
	options = append(options, toolChoiceOpts...)

	stream, err := goaisdk.StreamText(timeoutCtx, s.model, options...)
	if err != nil {
		return failedGeneration(err)
	}

	var raw strings.Builder
	extractor := askAIAnswerDeltaExtractor{}
	textStream := stream.TextStream()
	for {
		select {
		case chunk, ok := <-textStream:
			if !ok {
				goto streamDone
			}
			raw.WriteString(chunk)
			if delta := extractor.append(chunk); delta != "" {
				if err := emit(AskAIStreamDelta{Type: AskAIStreamDeltaAnswer, Status: "streaming", TextDelta: delta}); err != nil {
					return failedGeneration(err)
				}
			}
		case <-timeoutCtx.Done():
			if err := currentSinkErr(); err != nil {
				return failedGeneration(err)
			}
			return failedGeneration(timeoutCtx.Err())
		}
	}

streamDone:
	if err := currentSinkErr(); err != nil {
		return failedGeneration(err)
	}
	result := stream.Result()
	if err := currentSinkErr(); err != nil {
		return failedGeneration(err)
	}
	if err := stream.Err(); err != nil {
		return failedGeneration(err)
	}
	rawText := raw.String()
	if result != nil && strings.TrimSpace(result.Text) != "" {
		rawText = result.Text
	}
	usageSnapshot, lifecycleSnapshot, evidenceIDs := generationSnapshot()
	output, usage, err := finalizeAskAIStreamingGeneration(rawText, result, usageSnapshot, req, evidenceIDs)
	return askAIGeneration{Output: output, Usage: usage, EvidenceIDs: evidenceIDs, LifecycleEvents: lifecycleSnapshot, Latency: time.Since(started), Err: err}
}

func finalizeAskAIGeneration(result *goaisdk.ObjectResult[AskAIOutput], usage Usage, err error, req AskAIRequest, evidenceIDs []string) (AskAIOutput, Usage, error) {
	var output AskAIOutput
	if err != nil {
		return output, usage, err
	}
	if result == nil {
		return output, usage, fmt.Errorf("%w: missing provider result", ErrInvalidOutput)
	}
	output, err = strictAskAIResult(result)
	if err == nil {
		output, err = ValidateAskAIOutput(output, req, evidenceIDs)
	}
	if usage.TotalTokens == 0 {
		usage = Usage{
			InputTokens:   result.Usage.InputTokens,
			OutputTokens:  result.Usage.OutputTokens,
			TotalTokens:   totalTokens(result.Usage),
			ToolCallCount: usage.ToolCallCount,
		}
	}
	return output, usage, err
}

func strictAskAIResult(result *goaisdk.ObjectResult[AskAIOutput]) (AskAIOutput, error) {
	for i := len(result.Steps) - 1; i >= 0; i-- {
		if text := strings.TrimSpace(result.Steps[i].Text); text != "" {
			return decodeAskAIOutputText(text)
		}
	}
	return result.Object, nil
}

func finalizeAskAIStreamingGeneration(rawText string, result *goaisdk.TextResult, usage Usage, req AskAIRequest, evidenceIDs []string) (AskAIOutput, Usage, error) {
	output, err := decodeAskAIOutputText(rawText)
	if err == nil {
		output, err = ValidateAskAIOutput(output, req, evidenceIDs)
	}
	if usage.TotalTokens == 0 && result != nil {
		usage = Usage{
			InputTokens:   result.TotalUsage.InputTokens,
			OutputTokens:  result.TotalUsage.OutputTokens,
			TotalTokens:   totalTokens(result.TotalUsage),
			ToolCallCount: usage.ToolCallCount,
		}
	}
	return output, usage, err
}

func decodeAskAIOutputText(text string) (AskAIOutput, error) {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "\ufeff")
	if strings.HasPrefix(text, "```") {
		text = strings.TrimSpace(strings.TrimPrefix(text, "```json"))
		text = strings.TrimSpace(strings.TrimPrefix(text, "```"))
		text = strings.TrimSpace(strings.TrimSuffix(text, "```"))
	}
	if start := strings.Index(text, "{"); start > 0 {
		text = text[start:]
	}
	if end := strings.LastIndex(text, "}"); end >= 0 && end < len(text)-1 {
		text = text[:end+1]
	}
	var output AskAIOutput
	if err := json.Unmarshal([]byte(text), &output); err != nil {
		return AskAIOutput{}, fmt.Errorf("%w: decode ask ai output: %v", ErrInvalidOutput, err)
	}
	return output, nil
}

func normalizeAskAIRequest(req AskAIRequest) AskAIRequest {
	req.Query = strings.TrimSpace(req.Query)
	req.Route = normalizeDashboardRouteContext(req.Route)
	req.ActorType = strings.TrimSpace(req.ActorType)
	if req.ActorType == "" {
		req.ActorType = "user"
	}
	if req.To.IsZero() {
		req.To = time.Now().UTC()
	}
	if req.From.IsZero() || !req.From.Before(req.To) {
		req.From = req.To.AddDate(0, 0, -30)
	}
	req.History = trimAskAIHistory(req.History, 8)
	return req
}

func askAIGenerationInput(req AskAIRequest) askAIPromptInput {
	return askAIPromptInput{
		SiteDomain: req.SiteDomain,
		From:       req.From,
		To:         req.To,
		Route:      req.Route,
		Query:      req.Query,
		Filters:    req.Filters,
		History:    req.History,
		ToolNames:  askAIToolNames(req.Tools),
	}
}

func askAIToolNames(tools []goaisdk.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		if name := strings.TrimSpace(tool.Name); name != "" {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

func askAISystemPrompt(skillText string) string {
	base := `You are HitKeep Ask AI, a privacy-first dashboard assistant. Use the available read-only aggregate tools before making analytics claims. Keep answers concise and evidence-backed. Do not request or expose raw hit rows, visitor identities, IP addresses, credentials, billing mutations, site administration, goal/funnel mutation, or dashboard cookies. Charts and tables must be derived only from tool output. Export actions must only suggest the existing site takeout download. Navigation actions must only point to known dashboard routes. Return the requested JSON object only.`
	skillText = strings.TrimSpace(skillText)
	if skillText == "" {
		return base
	}
	if len(skillText) > 18000 {
		skillText = skillText[:18000]
	}
	return base + "\n\nPublic HitKeep skill guidance:\n\n" + skillText
}

func askAIPrompt(input string) string {
	return "Answer this dashboard question for the scoped HitKeep site and date range. Use aggregate tools as needed, cite tool evidence by tool name, and return only the structured JSON response:\n\n" + input
}

func askAIStreamingPrompt(input string) string {
	return "Answer this dashboard question for the scoped HitKeep site and date range. Use aggregate tools as needed, cite tool evidence by tool name, and return only one JSON object. Put the answer_markdown field first so the dashboard can stream a safe draft answer, then citations, charts, and actions. Do not wrap the JSON in markdown fences. The object must match this JSON schema:\n\n" + string(askAIOutputSchema()) + "\n\nInput:\n\n" + input
}

type askAIAnswerDeltaExtractor struct {
	raw      strings.Builder
	emitted  string
	complete bool
}

func (e *askAIAnswerDeltaExtractor) append(chunk string) string {
	if e.complete || chunk == "" {
		return ""
	}
	e.raw.WriteString(chunk)
	value, complete, ok := extractJSONStringFieldPrefix(e.raw.String(), "answer_markdown")
	if !ok {
		return ""
	}
	e.complete = complete
	if len(value) <= len(e.emitted) || !strings.HasPrefix(value, e.emitted) {
		return ""
	}
	delta := value[len(e.emitted):]
	e.emitted = value
	return delta
}

func extractJSONStringFieldPrefix(input, field string) (string, bool, bool) {
	key := `"` + field + `"`
	idx := strings.Index(input, key)
	if idx < 0 {
		return "", false, false
	}
	pos := idx + len(key)
	for pos < len(input) && isJSONSpace(input[pos]) {
		pos++
	}
	if pos >= len(input) || input[pos] != ':' {
		return "", false, false
	}
	pos++
	for pos < len(input) && isJSONSpace(input[pos]) {
		pos++
	}
	if pos >= len(input) || input[pos] != '"' {
		return "", false, false
	}
	pos++

	var out strings.Builder
	for pos < len(input) {
		ch := input[pos]
		if ch == '"' {
			return out.String(), true, true
		}
		if ch != '\\' {
			out.WriteByte(ch)
			pos++
			continue
		}
		if pos+1 >= len(input) {
			return out.String(), false, true
		}
		escaped := input[pos+1]
		switch escaped {
		case '"', '\\', '/':
			out.WriteByte(escaped)
			pos += 2
		case 'b':
			out.WriteByte('\b')
			pos += 2
		case 'f':
			out.WriteByte('\f')
			pos += 2
		case 'n':
			out.WriteByte('\n')
			pos += 2
		case 'r':
			out.WriteByte('\r')
			pos += 2
		case 't':
			out.WriteByte('\t')
			pos += 2
		case 'u':
			if pos+6 > len(input) {
				return out.String(), false, true
			}
			codepoint, err := strconv.ParseInt(input[pos+2:pos+6], 16, 32)
			if err != nil {
				return out.String(), false, true
			}
			out.WriteRune(rune(codepoint))
			pos += 6
		default:
			return out.String(), false, true
		}
	}
	return out.String(), false, true
}

func isJSONSpace(ch byte) bool {
	return ch == ' ' || ch == '\n' || ch == '\r' || ch == '\t'
}

func ValidateAskAIOutput(output AskAIOutput, req AskAIRequest, evidenceIDs []string) (AskAIOutput, error) {
	output.AnswerMarkdown = strings.TrimSpace(output.AnswerMarkdown)
	if output.AnswerMarkdown == "" {
		return AskAIOutput{}, fmt.Errorf("%w: answer_markdown is required", ErrInvalidOutput)
	}
	if len(output.AnswerMarkdown) > 6000 {
		return AskAIOutput{}, fmt.Errorf("%w: answer_markdown too long", ErrInvalidOutput)
	}
	allowedCitations := askAIAllowedCitationIDs(evidenceIDs)
	citations := make([]AskAICitation, 0, len(output.Citations))
	for _, citation := range output.Citations {
		citation.Label = strings.TrimSpace(citation.Label)
		citation.ToolCallID = strings.TrimSpace(citation.ToolCallID)
		if citation.Label == "" || citation.ToolCallID == "" {
			return AskAIOutput{}, fmt.Errorf("%w: citation label and tool_call_id are required", ErrInvalidOutput)
		}
		if !allowedCitations[citation.ToolCallID] {
			return AskAIOutput{}, fmt.Errorf("%w: unsupported citation %q", ErrInvalidOutput, citation.ToolCallID)
		}
		citations = append(citations, citation)
	}
	output.Citations = citations

	charts := make([]AskAIChart, 0, len(output.Charts))
	for _, chart := range output.Charts {
		normalized, err := validateAskAIChart(chart)
		if err != nil {
			return AskAIOutput{}, err
		}
		charts = append(charts, normalized)
	}
	output.Charts = charts

	actions := make([]AskAIAction, 0, len(output.Actions))
	for _, action := range output.Actions {
		normalized, err := validateAskAIAction(action, req.SiteID)
		if err != nil {
			return AskAIOutput{}, err
		}
		actions = append(actions, normalized)
	}
	output.Actions = actions
	return output, nil
}

func askAIAllowedCitationIDs(evidenceIDs []string) map[string]bool {
	allowed := map[string]bool{"input_context": true}
	for _, id := range evidenceIDs {
		if id = strings.TrimSpace(id); id != "" {
			allowed[id] = true
		}
	}
	return allowed
}

func askAIToolEvidenceIDs(toolCalls map[string]bool) []string {
	if len(toolCalls) == 0 {
		return nil
	}
	ids := make([]string, 0, len(toolCalls))
	for name := range toolCalls {
		if name = strings.TrimSpace(name); name != "" {
			ids = append(ids, name)
		}
	}
	sort.Strings(ids)
	return ids
}

func validateAskAIChart(chart AskAIChart) (AskAIChart, error) {
	chart.Type = strings.TrimSpace(chart.Type)
	chart.Title = strings.TrimSpace(chart.Title)
	chart.XKey = strings.TrimSpace(chart.XKey)
	switch chart.Type {
	case "line", "bar", "table":
	default:
		return AskAIChart{}, fmt.Errorf("%w: unsupported chart type %q", ErrInvalidOutput, chart.Type)
	}
	if chart.Title == "" {
		return AskAIChart{}, fmt.Errorf("%w: chart title is required", ErrInvalidOutput)
	}
	if len(chart.Rows) > 120 {
		return AskAIChart{}, fmt.Errorf("%w: chart row limit exceeded", ErrInvalidOutput)
	}
	if chart.Rows == nil {
		chart.Rows = []map[string]any{}
	}
	for i, row := range chart.Rows {
		if len(row) > 12 {
			return AskAIChart{}, fmt.Errorf("%w: chart row has too many fields", ErrInvalidOutput)
		}
		for key, value := range row {
			cleanKey := strings.TrimSpace(key)
			if cleanKey == "" || cleanKey != key {
				return AskAIChart{}, fmt.Errorf("%w: invalid chart row key", ErrInvalidOutput)
			}
			switch value.(type) {
			case nil, string, float64, bool:
			default:
				return AskAIChart{}, fmt.Errorf("%w: unsupported chart row value at row %d", ErrInvalidOutput, i)
			}
		}
	}
	if chart.Type != "table" {
		if chart.XKey == "" || len(chart.Series) == 0 {
			return AskAIChart{}, fmt.Errorf("%w: chart x_key and series are required", ErrInvalidOutput)
		}
	}
	for i := range chart.Series {
		chart.Series[i].Key = strings.TrimSpace(chart.Series[i].Key)
		chart.Series[i].Label = strings.TrimSpace(chart.Series[i].Label)
		if chart.Series[i].Key == "" || chart.Series[i].Label == "" {
			return AskAIChart{}, fmt.Errorf("%w: chart series key and label are required", ErrInvalidOutput)
		}
	}
	return chart, nil
}

func validateAskAIAction(action AskAIAction, siteID uuid.UUID) (AskAIAction, error) {
	action.Type = strings.TrimSpace(action.Type)
	action.Label = strings.TrimSpace(action.Label)
	action.Target = strings.TrimSpace(action.Target)
	action.Format = strings.ToLower(strings.TrimSpace(action.Format))
	if action.Label == "" {
		return AskAIAction{}, fmt.Errorf("%w: action label is required", ErrInvalidOutput)
	}
	switch action.Type {
	case "navigate":
		route := normalizeDashboardRoute(action.Target)
		if route == "" {
			return AskAIAction{}, fmt.Errorf("%w: unsupported navigation target", ErrInvalidOutput)
		}
		action.Target = route
		action.Format = ""
		return action, nil
	case "download_export":
		if siteID == uuid.Nil {
			return AskAIAction{}, fmt.Errorf("%w: site id is required for export", ErrInvalidOutput)
		}
		if action.Format == "" {
			action.Format = "xlsx"
		}
		switch action.Format {
		case "xlsx", "json", "csv", "ndjson":
		default:
			return AskAIAction{}, fmt.Errorf("%w: unsupported export format", ErrInvalidOutput)
		}
		action.Target = fmt.Sprintf("/api/sites/%s/takeout?format=%s", siteID.String(), action.Format)
		return action, nil
	default:
		return AskAIAction{}, fmt.Errorf("%w: unsupported action type %q", ErrInvalidOutput, action.Type)
	}
}

func normalizeDashboardRoute(raw string) string {
	return normalizeDashboardRoutePath(raw, false)
}

func normalizeDashboardRouteContext(raw string) string {
	return normalizeDashboardRoutePath(raw, true)
}

func normalizeDashboardRoutePath(raw string, allowQueryOrFragment bool) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || strings.HasPrefix(raw, "//") {
		return ""
	}
	if !allowQueryOrFragment && (parsed.RawQuery != "" || parsed.Fragment != "") {
		return ""
	}
	path := parsed.EscapedPath()
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	allowed := []string{
		"/dashboard", "/events", "/ecommerce", "/web-vitals", "/ai-visibility",
		"/opportunities", "/goals", "/funnels", "/utm", "/utm/builder",
		"/utm/qr-codes", "/import-export", "/integration/google-search-console",
	}
	if !slices.Contains(allowed, path) {
		return ""
	}
	return path
}

func trimAskAIHistory(history []AskAIMessage, limit int) []AskAIMessage {
	if len(history) > limit {
		history = history[len(history)-limit:]
	}
	out := make([]AskAIMessage, 0, len(history))
	for _, message := range history {
		role := strings.TrimSpace(message.Role)
		if role != "user" && role != "assistant" {
			continue
		}
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		if len(content) > 1200 {
			content = content[:1200]
		}
		out = append(out, AskAIMessage{Role: role, Content: content})
	}
	return out
}

func askAIOutputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type":"object",
		"additionalProperties":false,
		"required":["answer_markdown","citations","charts","actions"],
		"properties":{
			"answer_markdown":{"type":"string"},
			"citations":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["label","tool_call_id"],"properties":{"label":{"type":"string"},"tool_call_id":{"type":"string"}}}},
			"charts":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["type","title","rows"],"properties":{"type":{"type":"string","enum":["line","bar","table"]},"title":{"type":"string"},"x_key":{"type":"string"},"series":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["key","label"],"properties":{"key":{"type":"string"},"label":{"type":"string"}}}},"rows":{"type":"array","items":{"type":"object","additionalProperties":{"type":["string","number","boolean","null"]}}}}}},
			"actions":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["type","label","target"],"properties":{"type":{"type":"string","enum":["navigate","download_export"]},"label":{"type":"string"},"target":{"type":"string"},"format":{"type":"string","enum":["xlsx","json","csv","ndjson"]}}}}
		}
	}`)
}

func askAIAuditInput(req AskAIRequest) askAIPromptInput {
	input := askAIGenerationInput(req)
	input.History = trimAskAIHistory(input.History, 3)
	return input
}

func askAIEvidenceIDs(output *AskAIOutput) []string {
	if output == nil {
		return nil
	}
	ids := make([]string, 0, len(output.Citations))
	for _, citation := range output.Citations {
		if id := strings.TrimSpace(citation.ToolCallID); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func askAIRunEvidenceIDs(evidenceIDs []string, output *AskAIOutput) []string {
	if len(evidenceIDs) > 0 {
		return append([]string(nil), evidenceIDs...)
	}
	return askAIEvidenceIDs(output)
}

func askAIOutputSummary(output *AskAIOutput) map[string]any {
	if output == nil {
		return map[string]any{"version": "ask-ai-output-summary-v1"}
	}
	answer := strings.TrimSpace(output.AnswerMarkdown)
	return map[string]any{
		"version":       "ask-ai-output-summary-v1",
		"answer_sha256": HashString(answer),
		"answer_chars":  len(answer),
		"citations":     askAICitationSummary(output.Citations),
		"charts":        askAIChartSummary(output.Charts),
		"actions":       askAIActionSummary(output.Actions),
	}
}

func askAICitationSummary(citations []AskAICitation) []map[string]any {
	out := make([]map[string]any, 0, len(citations))
	for _, citation := range citations {
		label := strings.TrimSpace(citation.Label)
		out = append(out, map[string]any{
			"tool_call_id": strings.TrimSpace(citation.ToolCallID),
			"label_sha256": HashString(label),
			"label_chars":  len(label),
		})
	}
	return out
}

func askAIChartSummary(charts []AskAIChart) []map[string]any {
	out := make([]map[string]any, 0, len(charts))
	for _, chart := range charts {
		title := strings.TrimSpace(chart.Title)
		out = append(out, map[string]any{
			"type":         strings.TrimSpace(chart.Type),
			"title_sha256": HashString(title),
			"title_chars":  len(title),
			"row_count":    len(chart.Rows),
			"series_count": len(chart.Series),
			"series":       askAIChartSeriesSummary(chart.Series),
		})
	}
	return out
}

func askAIChartSeriesSummary(series []AskAIChartSeries) []map[string]any {
	out := make([]map[string]any, 0, len(series))
	for _, item := range series {
		key := strings.TrimSpace(item.Key)
		label := strings.TrimSpace(item.Label)
		out = append(out, map[string]any{
			"key_sha256":   HashString(key),
			"key_chars":    len(key),
			"label_sha256": HashString(label),
			"label_chars":  len(label),
		})
	}
	return out
}

func askAIActionSummary(actions []AskAIAction) []map[string]any {
	out := make([]map[string]any, 0, len(actions))
	for _, action := range actions {
		label := strings.TrimSpace(action.Label)
		target := strings.TrimSpace(action.Target)
		out = append(out, map[string]any{
			"type":          strings.TrimSpace(action.Type),
			"format":        strings.TrimSpace(action.Format),
			"label_sha256":  HashString(label),
			"label_chars":   len(label),
			"target_sha256": HashString(target),
			"target_chars":  len(target),
		})
	}
	return out
}

func askAIGenerationAuditFields(generation askAIGeneration) (string, string, *AskAIOutput) {
	if generation.Err != nil {
		if errors.Is(generation.Err, ErrInvalidOutput) {
			return "failure", "invalid_output", nil
		}
		return "failure", ClassifyError(generation.Err), nil
	}
	return "success", "", &generation.Output
}
