package smokegate

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

type Report struct {
	GeneratedAt time.Time
	Source      string
	Provider    string
	Model       string
	AIRuns      []AIRun
	Targets     []TargetResult
}

type AIRun struct {
	ID            uuid.UUID
	Provider      string
	Model         string
	Status        string
	ErrorCategory string
	OutputJSON    string
	TotalTokens   int
	ToolCalls     int
	EvidenceIDs   []string
}

type TargetResult struct {
	Domain        string
	From          time.Time
	To            time.Time
	Status        string
	Error         string
	Opportunities []api.Opportunity
}

type Verdict struct {
	ReleaseReady           bool
	BedrockNovaComplete    bool
	TargetCoverageComplete bool
	CitedEvidenceComplete  bool
	PrivacyClean           bool
	MoneyCopyClean         bool
	SourceAttributionClean bool
	CausalClaimsClean      bool
	Failures               []string
}

var privacyLeakMarkers = []string{
	"raw_prompt",
	"prompt_text",
	"provider_response",
	"raw_provider",
	"ip_address",
	"user_agent",
	"visitor_id",
	"session_id",
}

func Evaluate(report Report) Verdict {
	verdict := Verdict{
		BedrockNovaComplete:    hasSuccessfulBedrockNovaRun(report),
		TargetCoverageComplete: hasRequiredTargetCoverage(report),
		CitedEvidenceComplete:  hasCompleteCitedEvidence(report),
		PrivacyClean:           hasNoPrivacyLeakMarkers(report),
		MoneyCopyClean:         hasNoMoneyCopy(report),
		SourceAttributionClean: hasNoSourceTotalAttribution(report),
		CausalClaimsClean:      hasNoCausalClaims(report),
	}
	if !verdict.BedrockNovaComplete {
		verdict.Failures = append(verdict.Failures, "Bedrock Nova 2 Lite smoke did not complete successfully")
	}
	if !verdict.TargetCoverageComplete {
		verdict.Failures = append(verdict.Failures, "smoke report must cover hitkeep.com and at least one additional target")
	}
	if !verdict.CitedEvidenceComplete {
		verdict.Failures = append(verdict.Failures, "accepted opportunity has missing cited evidence")
	}
	if !verdict.PrivacyClean {
		verdict.Failures = append(verdict.Failures, "privacy-sensitive smoke report content detected")
	}
	if !verdict.MoneyCopyClean {
		verdict.Failures = append(verdict.Failures, "money/upside opportunity contract detected")
	}
	if !verdict.SourceAttributionClean {
		verdict.Failures = append(verdict.Failures, "traffic source claim is not backed by source-specific counts")
	}
	if !verdict.CausalClaimsClean {
		verdict.Failures = append(verdict.Failures, "unsupported causal wording detected")
	}
	verdict.ReleaseReady = len(verdict.Failures) == 0
	return verdict
}

func RenderMarkdown(report Report) string {
	verdict := Evaluate(report)
	var b strings.Builder
	fmt.Fprintf(&b, "# Opportunities Release-Hardening Smoke Report\n\n")
	fmt.Fprintf(&b, "Generated: `%s`\n", report.GeneratedAt.UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "Source: `%s`\n", report.Source)
	fmt.Fprintf(&b, "Provider: `%s`\n", report.Provider)
	fmt.Fprintf(&b, "Model: `%s`\n\n", report.Model)
	fmt.Fprintf(&b, "This private report contains aggregate opportunity evidence and structured AI outputs only. It must not include raw prompts, raw provider payloads, IP addresses, user agents, or row-level visitor data.\n\n")

	fmt.Fprintf(&b, "## Verdict\n\n")
	fmt.Fprintf(&b, "- Release ready: `%t`\n", verdict.ReleaseReady)
	fmt.Fprintf(&b, "- Bedrock Nova complete: `%t`\n", verdict.BedrockNovaComplete)
	fmt.Fprintf(&b, "- Target coverage complete: `%t`\n", verdict.TargetCoverageComplete)
	fmt.Fprintf(&b, "- Cited evidence complete: `%t`\n", verdict.CitedEvidenceComplete)
	fmt.Fprintf(&b, "- Privacy clean: `%t`\n", verdict.PrivacyClean)
	fmt.Fprintf(&b, "- Money/upside copy clean: `%t`\n", verdict.MoneyCopyClean)
	fmt.Fprintf(&b, "- Source attribution clean: `%t`\n", verdict.SourceAttributionClean)
	fmt.Fprintf(&b, "- Causal claims clean: `%t`\n", verdict.CausalClaimsClean)
	if len(verdict.Failures) > 0 {
		fmt.Fprintf(&b, "- Blockers: `%s`\n", strings.Join(verdict.Failures, "`, `"))
	}
	fmt.Fprintf(&b, "\n")
	if !verdict.PrivacyClean {
		fmt.Fprintf(&b, "Detailed target and AI output sections were redacted as `privacy_redacted` because the smoke report contained privacy-sensitive markers.\n")
		return b.String()
	}

	for _, target := range report.Targets {
		fmt.Fprintf(&b, "## %s\n\n", target.Domain)
		fmt.Fprintf(&b, "- Range: `%s` to `%s`\n", target.From.UTC().Format(time.RFC3339), target.To.UTC().Format(time.RFC3339))
		fmt.Fprintf(&b, "- Status: `%s`\n", target.Status)
		if target.Error != "" {
			fmt.Fprintf(&b, "- Error: `%s`\n", target.Error)
		}
		fmt.Fprintf(&b, "- Accepted opportunities: `%d`\n\n", len(target.Opportunities))
		for _, opportunity := range target.Opportunities {
			renderOpportunity(&b, opportunity)
		}
	}

	if len(report.AIRuns) > 0 {
		fmt.Fprintf(&b, "## AI Runs\n\n")
		for _, run := range report.AIRuns {
			fmt.Fprintf(&b, "- Run `%s`: provider `%s`, model `%s`, status `%s`, error `%s`, tokens `%d`, tools `%d`, evidence `%s`\n",
				run.ID, run.Provider, run.Model, run.Status, dash(run.ErrorCategory), run.TotalTokens, run.ToolCalls, strings.Join(run.EvidenceIDs, ", "))
			if strings.TrimSpace(run.OutputJSON) != "" {
				fmt.Fprintf(&b, "\n```json\n%s\n```\n\n", prettySafeJSON(run.OutputJSON))
			}
		}
	}

	return b.String()
}

func renderOpportunity(b *strings.Builder, opportunity api.Opportunity) {
	fmt.Fprintf(b, "### %s\n\n", opportunity.TypeKey)
	fmt.Fprintf(b, "- Kind: `%s`\n", opportunity.Kind)
	fmt.Fprintf(b, "- Score: `%d`\n", opportunity.Score)
	fmt.Fprintf(b, "- Confidence: `%s`\n", opportunity.Confidence)
	fmt.Fprintf(b, "- Impact: `%s` / `%s`\n", opportunity.ImpactValue, opportunity.ImpactLabelKey)
	fmt.Fprintf(b, "- Copy params: `%s`\n", compactMap(opportunity.CopyParams))
	fmt.Fprintf(b, "- Evidence:\n")
	for _, evidence := range opportunity.Evidence {
		fmt.Fprintf(b, "  - `%s`: `%s` = `%s`\n", evidence.ID, evidence.LabelKey, evidence.Value)
	}
	fmt.Fprintf(b, "- Cited evidence IDs: `%s`\n\n", strings.Join(opportunity.CitedEvidenceIDs, "`, `"))
}

func hasSuccessfulBedrockNovaRun(report Report) bool {
	for _, run := range report.AIRuns {
		if strings.EqualFold(run.Provider, "bedrock") &&
			strings.Contains(strings.ToLower(run.Model), "nova-2-lite") &&
			strings.EqualFold(run.Status, "success") {
			return true
		}
	}
	return false
}

func hasRequiredTargetCoverage(report Report) bool {
	hasHitkeepCom := false
	additionalTargets := 0
	for _, target := range report.Targets {
		if strings.TrimSpace(target.Error) != "" {
			continue
		}
		domain := strings.ToLower(strings.TrimSpace(target.Domain))
		if domain == "" {
			continue
		}
		if domain == "hitkeep.com" {
			hasHitkeepCom = true
			continue
		}
		additionalTargets++
	}
	return hasHitkeepCom && additionalTargets > 0
}

func hasCompleteCitedEvidence(report Report) bool {
	for _, target := range report.Targets {
		for _, opportunity := range target.Opportunities {
			if len(opportunity.CitedEvidenceIDs) == 0 {
				return false
			}
			evidenceIDs := make(map[string]api.OpportunityEvidence, len(opportunity.Evidence))
			for _, evidence := range opportunity.Evidence {
				id := strings.TrimSpace(evidence.ID)
				if id != "" {
					evidenceIDs[id] = evidence
				}
			}
			for _, citedID := range opportunity.CitedEvidenceIDs {
				evidence, ok := evidenceIDs[strings.TrimSpace(citedID)]
				if !ok || !visibleOpportunityEvidence(evidence) {
					return false
				}
			}
		}
	}
	return true
}

func visibleOpportunityEvidence(evidence api.OpportunityEvidence) bool {
	return strings.TrimSpace(evidence.LabelKey) != "" && strings.TrimSpace(evidence.Value) != ""
}

func hasNoPrivacyLeakMarkers(report Report) bool {
	return !reportContainsAny(report, privacyLeakMarkers...)
}

func hasNoMoneyCopy(report Report) bool {
	return !reportContainsAny(report, "monthly_upside", "estimated_monthly_upside", "revenue winners", "make more money", "upside")
}

func hasNoSourceTotalAttribution(report Report) bool {
	for _, target := range report.Targets {
		for _, opportunity := range target.Opportunities {
			if opportunity.TypeKey != "opportunities.types.traffic_quality" {
				continue
			}
			source := stringParam(opportunity.CopyParams, "source")
			sourceHits, ok := positiveIntParam(opportunity.CopyParams, "source_hits")
			if source == "" || !ok {
				return false
			}
			totalPageviews, ok := positiveIntParam(opportunity.CopyParams, "total_pageviews")
			if !ok {
				return false
			}
			if sourceHits >= totalPageviews {
				return false
			}
			if !hasVisibleCitedOpportunityEvidenceValue(opportunity, "source_hits", sourceHits) {
				return false
			}
		}
	}
	return true
}

func hasVisibleCitedOpportunityEvidenceValue(opportunity api.Opportunity, id string, expected int) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	cited := false
	for _, citedID := range opportunity.CitedEvidenceIDs {
		if strings.TrimSpace(citedID) == id {
			cited = true
			break
		}
	}
	if !cited {
		return false
	}
	for _, evidence := range opportunity.Evidence {
		if strings.TrimSpace(evidence.ID) != id || !visibleOpportunityEvidence(evidence) {
			continue
		}
		value, ok := positiveIntString(evidence.Value)
		return ok && value == expected
	}
	return false
}

func stringParam(values map[string]any, key string) string {
	raw, ok := values[key]
	if !ok || raw == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(raw))
}

func positiveIntParam(values map[string]any, key string) (int, bool) {
	raw, ok := values[key]
	if !ok {
		return 0, false
	}
	switch value := raw.(type) {
	case int:
		return value, value > 0
	case int64:
		if value <= 0 || int64(int(value)) != value {
			return 0, false
		}
		return int(value), true
	case float64:
		if value <= 0 || value != float64(int(value)) {
			return 0, false
		}
		return int(value), true
	case string:
		return positiveIntString(value)
	default:
		return 0, false
	}
}

func positiveIntString(value string) (int, bool) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	return parsed, err == nil && parsed > 0
}

func hasNoCausalClaims(report Report) bool {
	return !reportContainsAny(report, "caused by", "because of", "drives ", "driver of", "attributable to")
}

func reportContainsAny(report Report, needles ...string) bool {
	haystack := strings.ToLower(compactMap(map[string]any{
		"provider": report.Provider,
		"model":    report.Model,
		"targets":  report.Targets,
		"ai_runs":  report.AIRuns,
	}))
	for _, needle := range needles {
		if strings.Contains(haystack, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func compactMap(values map[string]any) string {
	if len(values) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", key, values[key]))
	}
	return strings.Join(parts, ", ")
}

func prettyJSON(raw string) string {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return "{}"
	}
	pretty, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(pretty)
}

func prettySafeJSON(raw string) string {
	if containsAny(raw, privacyLeakMarkers...) {
		return `{"redacted":"privacy_redacted"}`
	}
	return prettyJSON(raw)
}

func dash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func containsAny(haystack string, needles ...string) bool {
	normalized := strings.ToLower(haystack)
	for _, needle := range needles {
		if strings.Contains(normalized, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}
