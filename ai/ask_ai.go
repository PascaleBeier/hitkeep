package ai

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	goaisdk "github.com/zendev-sh/goai"
	"github.com/zendev-sh/goai/provider"

	json "hitkeep/jsonapi"
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

// GenerateAskAI returns the finished answer of a generation nobody streams.
func (s *Service) GenerateAskAI(ctx context.Context, req AskAIRequest) (AskAIResult, error) {
	return s.StreamAskAI(ctx, req, nil)
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

func (s *Service) runAskAIStreamingGeneration(ctx context.Context, req AskAIRequest, sink AskAIStreamSink) askAIGeneration {
	timeoutCtx, cancel := context.WithTimeout(ctx, s.conf.Timeout)
	defer cancel()
	run := &askAIRun{conf: s.conf, sink: sink, cancel: cancel, started: time.Now(), toolCalls: map[string]bool{}}
	messages, err := askAIMessages(req)
	if err != nil {
		return run.failed(err)
	}

	options := []goaisdk.Option{
		goaisdk.WithSystem(askAISystemPrompt(req.SkillText)),
		goaisdk.WithMessages(messages...),
		goaisdk.WithTools(req.Tools...),
		goaisdk.WithMaxSteps(5),
		goaisdk.WithMaxOutputTokens(1800),
		goaisdk.WithOnRequest(run.onRequest),
		goaisdk.WithOnResponse(run.onResponse),
		goaisdk.WithOnToolCallStart(run.onToolCallStart),
		goaisdk.WithOnToolCall(run.onToolCall),
	}
	options = append(options, temperatureOptions(s.model, 0.2)...)
	options = append(options, mantleAskAIToolOptions(s.conf, req.Tools)...)
	options = append(options, promptCachingOptions(s.conf)...)

	stream, err := goaisdk.StreamText(timeoutCtx, s.model, options...)
	if err != nil {
		return run.failed(err)
	}

	var raw strings.Builder
	extractor := askAIAnswerDeltaExtractor{}
	textStream := stream.TextStream()
read:
	for {
		select {
		case chunk, ok := <-textStream:
			if !ok {
				break read
			}
			raw.WriteString(chunk)
			if delta := extractor.append(chunk); delta != "" {
				if err := run.emit(AskAIStreamDelta{Type: AskAIStreamDeltaAnswer, Status: "streaming", TextDelta: delta}); err != nil {
					return run.failed(err)
				}
			}
		case <-timeoutCtx.Done():
			return run.failed(cmp.Or(run.sinkError(), timeoutCtx.Err()))
		}
	}

	if err := run.sinkError(); err != nil {
		return run.failed(err)
	}
	result := stream.Result()
	if err := run.sinkError(); err != nil {
		return run.failed(err)
	}
	if err := stream.Err(); err != nil {
		return run.failed(err)
	}
	rawText := raw.String()
	if result != nil && strings.TrimSpace(result.Text) != "" {
		rawText = result.Text
	}
	usage, lifecycle, evidenceIDs := run.snapshot()
	if usage.TotalTokens == 0 && result != nil {
		usage = Usage{
			InputTokens:   result.TotalUsage.InputTokens,
			OutputTokens:  result.TotalUsage.OutputTokens,
			TotalTokens:   totalTokens(result.TotalUsage),
			ToolCallCount: usage.ToolCallCount,
		}
	}
	output, err := decodeAskAIOutputText(rawText)
	if err != nil && extractor.complete {
		// The answer streamed intact but the rest of the object did not:
		// keep the answer the user already read instead of failing the run.
		output, err = AskAIOutput{AnswerMarkdown: extractor.emitted}, nil
	}
	if err == nil {
		output, err = ValidateAskAIOutput(output, req, evidenceIDs)
	}
	return askAIGeneration{Output: output, Usage: usage, EvidenceIDs: evidenceIDs, LifecycleEvents: lifecycle, Latency: time.Since(run.started), Err: err}
}

// askAIRun records one generation. GoAI runs tool calls concurrently, so
// every hook takes the lock.
type askAIRun struct {
	conf    Config
	sink    AskAIStreamSink
	cancel  context.CancelFunc
	started time.Time

	mu        sync.Mutex
	usage     Usage
	lifecycle []LifecycleEvent
	toolCalls map[string]bool
	sinkErr   error
}

func (r *askAIRun) record(event LifecycleEvent) {
	event.Provider = cmp.Or(event.Provider, r.conf.Provider)
	event.Model = cmp.Or(event.Model, r.conf.Model)
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lifecycle = append(r.lifecycle, event)
}

func (r *askAIRun) emit(delta AskAIStreamDelta) error {
	if r.sink == nil {
		return nil
	}
	return r.sink(delta)
}

// emitProgress reports tool progress; a failed write stops the generation.
func (r *askAIRun) emitProgress(delta AskAIStreamDelta) {
	err := r.emit(delta)
	if err == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sinkErr == nil {
		r.sinkErr = err
		r.cancel()
	}
}

func (r *askAIRun) sinkError() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sinkErr
}

func (r *askAIRun) onRequest(info goaisdk.RequestInfo) {
	r.record(LifecycleEvent{Type: "request_start", Model: info.Model, MessageCount: info.MessageCount, ToolCount: info.ToolCount, Status: "started", Timestamp: info.Timestamp})
}

func (r *askAIRun) onResponse(info goaisdk.ResponseInfo) {
	r.mu.Lock()
	r.usage.InputTokens += info.Usage.InputTokens
	r.usage.OutputTokens += info.Usage.OutputTokens
	r.usage.TotalTokens += totalTokens(info.Usage)
	r.mu.Unlock()
	status, category := "success", ""
	if info.Error != nil {
		status, category = "failure", ClassifyError(info.Error)
	}
	r.record(LifecycleEvent{Type: "request_finish", Status: status, StatusCode: info.StatusCode, ErrorCategory: category, LatencyMS: info.Latency.Milliseconds()})
}

func (r *askAIRun) onToolCallStart(info goaisdk.ToolCallStartInfo) {
	r.record(LifecycleEvent{Type: "tool_call_start", ToolName: info.ToolName, Step: info.Step, Status: "started"})
	r.emitProgress(AskAIStreamDelta{Type: AskAIStreamDeltaProgress, Status: "tool_call_start", MessageKey: "askAi.progress.readingAnalytics", ToolCallID: info.ToolCallID, ToolName: info.ToolName})
}

func (r *askAIRun) onToolCall(info goaisdk.ToolCallInfo) {
	status, category := "success", ""
	r.mu.Lock()
	r.usage.ToolCallCount++
	if info.Error != nil {
		status, category = "failure", ClassifyError(info.Error)
	} else if name := strings.TrimSpace(info.ToolName); name != "" {
		r.toolCalls[name] = true
	}
	r.mu.Unlock()
	r.record(LifecycleEvent{Type: "tool_call_finish", ToolName: info.ToolName, Step: info.Step, Status: status, ErrorCategory: category, LatencyMS: info.Duration.Milliseconds()})
	r.emitProgress(AskAIStreamDelta{Type: AskAIStreamDeltaProgress, Status: "tool_call_finish", MessageKey: "askAi.progress.composing", ToolCallID: info.ToolCallID, ToolName: info.ToolName})
}

func (r *askAIRun) snapshot() (Usage, []LifecycleEvent, []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.usage, slices.Clone(r.lifecycle), slices.Sorted(maps.Keys(r.toolCalls))
}

func (r *askAIRun) failed(err error) askAIGeneration {
	usage, lifecycle, _ := r.snapshot()
	return askAIGeneration{Usage: usage, LifecycleEvents: lifecycle, Latency: time.Since(r.started), Err: err}
}

// askAIMessages replays the drawer conversation as real turns, then asks the
// current question with the scoped context.
func askAIMessages(req AskAIRequest) ([]provider.Message, error) {
	input := askAIGenerationInput(req)
	input.History = nil
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("encode ask ai prompt input: %w", err)
	}
	messages := make([]provider.Message, 0, len(req.History)+1)
	for _, message := range req.History {
		switch {
		case message.Role == "user":
			messages = append(messages, goaisdk.UserMessage(message.Content))
		case len(messages) > 0:
			// Providers expect the conversation to open with a user turn.
			messages = append(messages, goaisdk.AssistantMessage(message.Content))
		}
	}
	return append(messages, goaisdk.UserMessage(askAIStreamingPrompt(string(inputJSON)))), nil
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
	if before, after, ok := strings.CutLast(text, "}"); ok && after != "" {
		text = before + "}"
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
	base := `You are HitKeep Ask AI, a privacy-first dashboard assistant. Answer only questions about the scoped HitKeep site's analytics, tracking setup, or use of HitKeep features. Code is in scope when it helps implement or diagnose HitKeep tracking or an analytics integration. Interpret short follow-ups in the context of earlier HitKeep analytics questions. If a user asks for something unrelated, including a general programming tutorial, do not answer that task; briefly say you can help with HitKeep analytics or tracking and invite a relevant question. Use the available read-only aggregate tools before making analytics claims. Tools default to the dashboard date range and filters in the input; pass from and to, or compare_from and compare_to on the site overview, to read other periods such as the previous one. Request only the site overview sections the question needs. Check annotations before explaining a spike or drop, and use the docs tools, when offered, for product questions instead of guessing. Keep answers concise and evidence-backed. Do not request or expose raw hit rows, visitor identities, IP addresses, credentials, billing mutations, site administration, goal/funnel mutation, or dashboard cookies. Charts and tables must be derived only from tool output. Export actions must only suggest the existing site takeout download. Navigation actions must only point to known dashboard routes. Return the requested JSON object only.`
	skillText = strings.TrimSpace(skillText)
	if skillText == "" {
		return base
	}
	skillText = truncateUTF8(skillText, 18000)
	return base + "\n\nPublic HitKeep skill guidance:\n\n" + skillText
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
	// Optional parts that fail validation are dropped, not fatal: the answer
	// stays useful while unsafe or invented citations, charts, and actions
	// never reach the dashboard.
	allowedCitations := askAIAllowedCitationIDs(evidenceIDs)
	output.Citations = keepValid(output.Citations, func(citation AskAICitation) (AskAICitation, error) {
		citation.Label, citation.ToolCallID = strings.TrimSpace(citation.Label), strings.TrimSpace(citation.ToolCallID)
		if citation.Label == "" || !allowedCitations[citation.ToolCallID] {
			return citation, fmt.Errorf("%w: unsupported citation %q", ErrInvalidOutput, citation.ToolCallID)
		}
		return citation, nil
	})
	output.Charts = keepValid(output.Charts, validateAskAIChart)
	output.Actions = keepValid(output.Actions, func(action AskAIAction) (AskAIAction, error) {
		return validateAskAIAction(action, req.SiteID)
	})
	return output, nil
}

// truncateUTF8 cuts s to at most n bytes without splitting a character.
func truncateUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
}

func keepValid[T any](items []T, validate func(T) (T, error)) []T {
	out := make([]T, 0, len(items))
	for _, item := range items {
		if normalized, err := validate(item); err == nil {
			out = append(out, normalized)
		}
	}
	return out
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
		"/dashboard", "/events", "/ecommerce", "/web-vitals", "/ai-agents", "/ai-visibility",
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
		content = truncateUTF8(content, 1200)
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
