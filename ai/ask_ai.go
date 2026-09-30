package ai

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
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
	// Snapshot maps tool names to inputs run before the model answers.
	Snapshot map[string]string
	// ToolTitles labels the citations the dashboard shows.
	ToolTitles map[string]string
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
	AnswerMarkdown string          `json:"answer_markdown"`
	Citations      []AskAICitation `json:"citations"`
	Charts         []AskAIChart    `json:"charts"`
	Actions        []AskAIAction   `json:"actions"`
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

const (
	askAIMaxSteps = 5
	// Reasoning models spend output tokens on thinking before they answer.
	askAIMaxOutputTokens = 4000
	askAIMaxAnswerChars  = 6000
	askAIMaxCharts       = 3
	askAIMaxActions      = 3
)

func (s *Service) runAskAIStreamingGeneration(ctx context.Context, req AskAIRequest, sink AskAIStreamSink) askAIGeneration {
	timeoutCtx, cancel := context.WithTimeout(ctx, s.conf.Timeout)
	defer cancel()
	run := newAskAIRun(s.conf, req, sink, cancel)
	messages, err := askAIMessages(req, run.readSnapshot(timeoutCtx))
	if err != nil {
		return run.failed(err)
	}

	options := []goaisdk.Option{
		goaisdk.WithSystem(askAISystemPrompt(req.SkillText)),
		goaisdk.WithMessages(messages...),
		goaisdk.WithTools(append(slices.Clone(req.Tools), run.outputTools()...)...),
		goaisdk.WithMaxSteps(askAIMaxSteps),
		goaisdk.WithMaxOutputTokens(askAIMaxOutputTokens),
		goaisdk.WithOnRequest(run.onRequest),
		goaisdk.WithOnResponse(run.onResponse),
		goaisdk.WithOnToolCallStart(run.onToolCallStart),
		goaisdk.WithOnToolCall(run.onToolCall),
		goaisdk.WithOnBeforeStep(func(info goaisdk.BeforeStepInfo) goaisdk.BeforeStepResult {
			if info.Step < askAIMaxSteps {
				return goaisdk.BeforeStepResult{}
			}
			return goaisdk.BeforeStepResult{ExtraMessages: []provider.Message{
				goaisdk.UserMessage("This is your last step: answer now from the analytics you have, without calling more tools."),
			}}
		}),
	}
	options = append(options, temperatureOptions(s.model, 0.2)...)
	options = append(options, promptCachingOptions(s.conf)...)

	stream, err := goaisdk.StreamText(timeoutCtx, s.model, options...)
	if err != nil {
		return run.failed(err)
	}
	textStream := stream.TextStream()
read:
	for {
		select {
		case chunk, ok := <-textStream:
			if !ok {
				break read
			}
			if err := run.emit(AskAIStreamDelta{Type: AskAIStreamDeltaAnswer, Status: "streaming", TextDelta: chunk}); err != nil {
				return run.failed(err)
			}
		case <-timeoutCtx.Done():
			return run.failed(cmp.Or(run.sinkError(), timeoutCtx.Err()))
		}
	}

	result := stream.Result()
	if err := run.sinkError(); err != nil {
		return run.failed(err)
	}
	if err := stream.Err(); err != nil {
		return run.failed(err)
	}
	// The draft also streamed text written before tool calls; the final
	// answer replaces it in the dashboard.
	if result == nil {
		return run.finish("", provider.Usage{})
	}
	return run.finish(run.answer(result.Steps), result.TotalUsage)
}

// askAIRun records one generation. GoAI runs tool calls concurrently, so
// every hook takes the lock.
type askAIRun struct {
	conf    Config
	req     AskAIRequest
	sink    AskAIStreamSink
	cancel  context.CancelFunc
	started time.Time
	// analytics names the tools whose results count as evidence.
	analytics map[string]bool

	mu        sync.Mutex
	usage     Usage
	lifecycle []LifecycleEvent
	evidence  map[string]bool
	charts    []AskAIChart
	actions   []AskAIAction
	sinkErr   error
}

func newAskAIRun(conf Config, req AskAIRequest, sink AskAIStreamSink, cancel context.CancelFunc) *askAIRun {
	analytics := make(map[string]bool, len(req.Tools))
	for _, tool := range req.Tools {
		analytics[tool.Name] = true
	}
	return &askAIRun{conf: conf, req: req, sink: sink, cancel: cancel, started: time.Now(), analytics: analytics, evidence: map[string]bool{}}
}

// readSnapshot runs the request's snapshot calls before the model answers,
// so every model starts from real numbers instead of guessing.
func (r *askAIRun) readSnapshot(ctx context.Context) string {
	var snapshot strings.Builder
	for _, name := range slices.Sorted(maps.Keys(r.req.Snapshot)) {
		i := slices.IndexFunc(r.req.Tools, func(tool goaisdk.Tool) bool { return tool.Name == name })
		if i < 0 {
			continue
		}
		callID := "snapshot-" + name
		r.onToolCallStart(goaisdk.ToolCallStartInfo{ToolCallID: callID, ToolName: name})
		started := time.Now()
		out, err := r.req.Tools[i].Execute(ctx, json.RawMessage(r.req.Snapshot[name]))
		r.onToolCall(goaisdk.ToolCallInfo{ToolCallID: callID, ToolName: name, Duration: time.Since(started), Error: err})
		if err == nil {
			snapshot.WriteString(out)
			snapshot.WriteString("\n")
		}
	}
	return snapshot.String()
}

// answer is the text written after the last analytics tool call: text before
// it is preamble, and chart or action calls must not cut the answer off.
func (r *askAIRun) answer(steps []goaisdk.StepResult) string {
	var parts []string
	for _, step := range steps {
		if slices.ContainsFunc(step.ToolCalls, func(call provider.ToolCall) bool { return r.analytics[call.Name] }) {
			parts = parts[:0]
			continue
		}
		if text := strings.TrimSpace(step.Text); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n")
}

// outputTools let the model attach charts and dashboard actions to its
// answer. They exist only for the dashboard, never for MCP.
func (r *askAIRun) outputTools() []goaisdk.Tool {
	return []goaisdk.Tool{
		goaisdk.NewTool("show_chart",
			"Show a chart below the answer, with values copied from tool results. type is line, bar, or table. columns names each value in a row; line and bar charts also need x_key (one of the columns) and series (the columns to plot). At most 3 charts, 120 rows, and 12 columns.",
			func(_ context.Context, in askAIChartInput) (string, error) {
				chart, err := in.chart()
				if err == nil {
					chart, err = validateAskAIChart(chart)
				}
				if err != nil {
					return "", err
				}
				return "The chart is shown below the answer.", r.collect(func() bool {
					return appendCapped(&r.charts, chart, askAIMaxCharts)
				})
			}),
		goaisdk.NewTool("suggest_action",
			"Offer a button below the answer: type navigate with a dashboard path target such as /dashboard, /events, /goals, or /ai-agents, or type download_export with format xlsx, json, csv, or ndjson for the site export. At most 3 actions.",
			func(_ context.Context, action AskAIAction) (string, error) {
				action, err := validateAskAIAction(action, r.req.SiteID)
				if err != nil {
					return "", err
				}
				return "The action is shown below the answer.", r.collect(func() bool {
					return appendCapped(&r.actions, action, askAIMaxActions)
				})
			}),
	}
}

// askAIChartInput keeps rows as plain string cells: nested free-form objects
// are rejected by some providers' tool schemas, such as Gemini's.
type askAIChartInput struct {
	Type    string             `json:"type"`
	Title   string             `json:"title"`
	XKey    string             `json:"x_key,omitempty"`
	Series  []AskAIChartSeries `json:"series,omitempty"`
	Columns []string           `json:"columns"`
	Rows    [][]string         `json:"rows"`
}

func (in askAIChartInput) chart() (AskAIChart, error) {
	rows := make([]map[string]any, 0, len(in.Rows))
	for _, cells := range in.Rows {
		if len(cells) != len(in.Columns) {
			return AskAIChart{}, errors.New("each row needs one value per column")
		}
		row := make(map[string]any, len(cells))
		for i, cell := range cells {
			row[in.Columns[i]] = cell
			if n, err := strconv.ParseFloat(cell, 64); err == nil && !math.IsNaN(n) && !math.IsInf(n, 0) {
				row[in.Columns[i]] = n
			}
		}
		rows = append(rows, row)
	}
	return AskAIChart{Type: in.Type, Title: in.Title, XKey: in.XKey, Series: in.Series, Rows: rows}, nil
}

func (r *askAIRun) collect(add func() bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !add() {
		return errors.New("limit reached; answer without it")
	}
	return nil
}

func appendCapped[T any](items *[]T, item T, limit int) bool {
	if len(*items) >= limit {
		return false
	}
	*items = append(*items, item)
	return true
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
	if r.analytics[info.ToolName] {
		r.emitProgress(AskAIStreamDelta{Type: AskAIStreamDeltaProgress, Status: "tool_call_start", MessageKey: "askAi.progress.readingAnalytics", ToolCallID: info.ToolCallID, ToolName: info.ToolName})
	}
}

func (r *askAIRun) onToolCall(info goaisdk.ToolCallInfo) {
	status, category := "success", ""
	r.mu.Lock()
	r.usage.ToolCallCount++
	if info.Error != nil {
		status, category = "failure", ClassifyError(info.Error)
	} else if r.analytics[info.ToolName] {
		r.evidence[info.ToolName] = true
	}
	r.mu.Unlock()
	r.record(LifecycleEvent{Type: "tool_call_finish", ToolName: info.ToolName, Step: info.Step, Status: status, ErrorCategory: category, LatencyMS: info.Duration.Milliseconds()})
	if r.analytics[info.ToolName] {
		r.emitProgress(AskAIStreamDelta{Type: AskAIStreamDeltaProgress, Status: "tool_call_finish", MessageKey: "askAi.progress.composing", ToolCallID: info.ToolCallID, ToolName: info.ToolName})
	}
}

func (r *askAIRun) snapshot() (Usage, []LifecycleEvent, []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.usage, slices.Clone(r.lifecycle), slices.Sorted(maps.Keys(r.evidence))
}

func (r *askAIRun) failed(err error) askAIGeneration {
	usage, lifecycle, _ := r.snapshot()
	return askAIGeneration{Usage: usage, LifecycleEvents: lifecycle, Latency: time.Since(r.started), Err: err}
}

// finish assembles the dashboard answer. Citations come from the analytics
// tools that actually ran, so the model can never cite invented evidence.
func (r *askAIRun) finish(answer string, fallback provider.Usage) askAIGeneration {
	usage, lifecycle, evidence := r.snapshot()
	if usage.TotalTokens == 0 {
		usage = Usage{InputTokens: fallback.InputTokens, OutputTokens: fallback.OutputTokens, TotalTokens: totalTokens(fallback), ToolCallCount: usage.ToolCallCount}
	}
	generation := askAIGeneration{Usage: usage, EvidenceIDs: evidence, LifecycleEvents: lifecycle, Latency: time.Since(r.started)}
	answer = truncateUTF8(strings.TrimSpace(answer), askAIMaxAnswerChars)
	if answer == "" {
		generation.Err = fmt.Errorf("%w: the model returned no answer", ErrInvalidOutput)
		return generation
	}
	citations := make([]AskAICitation, 0, len(evidence))
	for _, id := range evidence {
		citations = append(citations, AskAICitation{Label: cmp.Or(r.req.ToolTitles[id], id), ToolCallID: id})
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	generation.Output = AskAIOutput{
		AnswerMarkdown: answer,
		Citations:      citations,
		Charts:         append([]AskAIChart{}, r.charts...),
		Actions:        append([]AskAIAction{}, r.actions...),
	}
	return generation
}

// askAIMessages replays the drawer conversation as real turns, then asks the
// current question with its scope and the analytics already read.
func askAIMessages(req AskAIRequest, snapshot string) ([]provider.Message, error) {
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
	prompt := "Answer this dashboard question for the scoped HitKeep site and date range.\n\nInput:\n\n" + string(inputJSON)
	if snapshot != "" {
		prompt += "\n\nAnalytics already read for this question. Start from it and call tools only for what it lacks:\n\n" + snapshot
	}
	return append(messages, goaisdk.UserMessage(prompt)), nil
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
	base := `You are HitKeep Ask AI, a privacy-first dashboard assistant. Answer only questions about the scoped HitKeep site's analytics, tracking setup, or use of HitKeep features. Code is in scope when it helps implement or diagnose HitKeep tracking or an analytics integration. Interpret short follow-ups in the context of earlier HitKeep analytics questions. If a user asks for something unrelated, including a general programming tutorial, do not answer that task; briefly say you can help with HitKeep analytics or tracking and invite a relevant question. Start from the analytics already read for the question and call the read-only aggregate tools for anything it lacks. Tools default to the dashboard date range and filters in the input; pass from and to, or compare_from and compare_to on the site overview, to read other periods such as the previous one. Request only the site overview sections the question needs. For comparisons, call the site overview with compare_from and compare_to set to the exact periods the question names, such as a calendar month. Check annotations before explaining a spike or drop, and use the docs tools, when offered, for product questions instead of guessing. Write the answer as concise markdown that leads with the verdict. Never invent numbers: state only values from analytics you were given or read, and say so when the data cannot answer the question. Do not request or expose raw hit rows, visitor identities, IP addresses, credentials, billing mutations, site administration, goal/funnel mutation, or dashboard cookies. To show a chart or table, call show_chart with rows copied from tool results. To point to a dashboard page or the site export, call suggest_action. The dashboard lists the analytics you read as sources, so do not add a sources section.`
	skillText = strings.TrimSpace(skillText)
	if skillText == "" {
		return base
	}
	skillText = truncateUTF8(skillText, 18000)
	return base + "\n\nPublic HitKeep skill guidance:\n\n" + skillText
}

// truncateUTF8 cuts s to at most n bytes without splitting a character.
func truncateUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
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
