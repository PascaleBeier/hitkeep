package smokegate

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

func TestEvaluateBlocksReleaseUntilBedrockNovaCompletes(t *testing.T) {
	report := Report{
		GeneratedAt: time.Date(2026, 5, 12, 18, 0, 0, 0, time.UTC),
		Source:      "restored EU backup",
		Provider:    "bedrock",
		Model:       "eu.amazon.nova-2-lite-v1:0",
		Targets: []TargetResult{{
			Domain: "hitkeep.com",
			From:   time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC),
			To:     time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC),
			Opportunities: []api.Opportunity{{
				ID:             uuid.New(),
				Kind:           "traffic",
				TypeKey:        "opportunities.types.traffic_quality",
				CopyParams:     map[string]any{"source": "Open Alternative", "source_hits": 120, "total_pageviews": 1800},
				ImpactValue:    "120",
				ImpactLabelKey: "opportunities.impact.pageviews_to_route",
				Evidence: []api.OpportunityEvidence{
					{ID: "source_hits", LabelKey: "opportunities.evidence.source_hits", Value: "120"},
					{ID: "total_pageviews", LabelKey: "opportunities.evidence.total_pageviews", Value: "1800"},
				},
				CitedEvidenceIDs: []string{"source_hits"},
				Confidence:       "medium",
				Score:            72,
				TitleKey:         "opportunities.catalog.traffic_quality.title",
				SummaryKey:       "opportunities.catalog.traffic_quality.summary",
				ActionKey:        "opportunities.catalog.traffic_quality.action",
				DigestKey:        "opportunities.catalog.traffic_quality.digest",
				DetectorVersion:  "opportunities-detectors-v1",
				Status:           "new",
				RouteLabelKey:    "opportunities.routes.source",
				RouteIcon:        "pi pi-chart-line",
				GeneratedAt:      time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC),
				CreatedAt:        time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC),
				UpdatedAt:        time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC),
			}},
		}},
	}

	verdict := Evaluate(report)
	if verdict.ReleaseReady {
		t.Fatal("expected release readiness to stay blocked without a completed Bedrock Nova run")
	}
	if verdict.BedrockNovaComplete {
		t.Fatal("expected Bedrock Nova completion to be false")
	}
	if verdict.MoneyCopyClean != true || verdict.SourceAttributionClean != true || verdict.CausalClaimsClean != true {
		t.Fatalf("expected deterministic outputs to be clean, got %#v", verdict)
	}
}

func TestEvaluateRequiresHitkeepComAndAdditionalTarget(t *testing.T) {
	report := cleanBedrockNovaReport()

	verdict := Evaluate(report)
	if verdict.ReleaseReady {
		t.Fatal("expected release readiness to stay blocked until the smoke covers hitkeep.com and another target")
	}
	if verdict.TargetCoverageComplete {
		t.Fatal("expected target coverage to be incomplete with only hitkeep.com")
	}
	if !strings.Contains(strings.Join(verdict.Failures, "\n"), "hitkeep.com and at least one additional target") {
		t.Fatalf("expected target coverage blocker, got %#v", verdict.Failures)
	}

	report.Targets = append(report.Targets, TargetResult{Domain: "cloud.hitkeep.eu", From: report.Targets[0].From, To: report.Targets[0].To})
	verdict = Evaluate(report)
	if !verdict.TargetCoverageComplete {
		t.Fatalf("expected target coverage to pass with hitkeep.com and a second target, got %#v", verdict)
	}
	if !verdict.ReleaseReady {
		t.Fatalf("expected otherwise clean report to be release-ready, got %#v", verdict)
	}
}

func TestEvaluateDoesNotCountErroredTargetsTowardCoverage(t *testing.T) {
	report := cleanBedrockNovaReport()
	report.Targets = append(report.Targets, TargetResult{Domain: "cloud.hitkeep.eu", Error: "site not found"})

	verdict := Evaluate(report)
	if verdict.TargetCoverageComplete || verdict.ReleaseReady {
		t.Fatalf("expected errored additional target not to satisfy target coverage, got %#v", verdict)
	}

	report.Targets[0].Error = "analytics store failed"
	report.Targets[1].Error = ""
	verdict = Evaluate(report)
	if verdict.TargetCoverageComplete || verdict.ReleaseReady {
		t.Fatalf("expected errored hitkeep.com target not to satisfy target coverage, got %#v", verdict)
	}
}

func TestEvaluateRejectsOpportunitiesWithMissingCitedEvidence(t *testing.T) {
	report := cleanBedrockNovaReport()
	report.Targets = append(report.Targets, TargetResult{Domain: "cloud.hitkeep.eu", From: report.Targets[0].From, To: report.Targets[0].To})

	verdict := Evaluate(report)
	if !verdict.CitedEvidenceComplete || !verdict.ReleaseReady {
		t.Fatalf("expected clean cited evidence to pass release gate, got %#v", verdict)
	}

	report.Targets[0].Opportunities[0].CitedEvidenceIDs = append(report.Targets[0].Opportunities[0].CitedEvidenceIDs, "missing_source_count")
	verdict = Evaluate(report)
	if verdict.CitedEvidenceComplete || verdict.ReleaseReady {
		t.Fatalf("expected missing cited evidence to block release, got %#v", verdict)
	}
	if !strings.Contains(strings.Join(verdict.Failures, "\n"), "missing cited evidence") {
		t.Fatalf("expected missing cited evidence blocker, got %#v", verdict.Failures)
	}
}

func TestEvaluateRejectsCitedEvidenceWithoutVisibleAggregateValues(t *testing.T) {
	report := cleanBedrockNovaReport()
	report.Targets = append(report.Targets, TargetResult{Domain: "cloud.hitkeep.eu", From: report.Targets[0].From, To: report.Targets[0].To})

	report.Targets[0].Opportunities[0].Evidence = []api.OpportunityEvidence{
		{ID: "source_hits", LabelKey: "opportunities.evidence.source_hits", Value: ""},
		{ID: "total_pageviews", LabelKey: "", Value: "1800"},
	}

	verdict := Evaluate(report)
	if verdict.CitedEvidenceComplete || verdict.ReleaseReady {
		t.Fatalf("expected invisible cited evidence to block release, got %#v", verdict)
	}
	if !strings.Contains(strings.Join(verdict.Failures, "\n"), "missing cited evidence") {
		t.Fatalf("expected cited evidence blocker, got %#v", verdict.Failures)
	}
}

func TestEvaluateRejectsAndRedactsPrivacyLeakMarkers(t *testing.T) {
	report := cleanBedrockNovaReport()
	report.Targets = append(report.Targets, TargetResult{Domain: "cloud.hitkeep.eu", From: report.Targets[0].From, To: report.Targets[0].To})
	report.Targets[1].Error = "provider returned user_agent curl/8.0 for ip_address 203.0.113.7"
	report.AIRuns[0].OutputJSON = `{"raw_prompt":"full prompt","provider_response":{"ip_address":"203.0.113.7","user_agent":"curl/8.0"}}`

	verdict := Evaluate(report)
	if verdict.PrivacyClean || verdict.ReleaseReady {
		t.Fatalf("expected privacy leak markers to block release, got %#v", verdict)
	}
	if !strings.Contains(strings.Join(verdict.Failures, "\n"), "privacy-sensitive smoke report content") {
		t.Fatalf("expected privacy blocker, got %#v", verdict.Failures)
	}

	markdown := RenderMarkdown(report)
	for _, forbidden := range []string{"raw_prompt", "provider_response", "ip_address", "user_agent", "203.0.113.7", "curl/8.0"} {
		if strings.Contains(markdown, forbidden) {
			t.Fatalf("report leaked forbidden token %q:\n%s", forbidden, markdown)
		}
	}
	if !strings.Contains(markdown, "privacy_redacted") {
		t.Fatalf("expected report to keep an explicit redaction marker:\n%s", markdown)
	}
}

func TestEvaluateRejectsMoneyCopyAndSourceTotalAttribution(t *testing.T) {
	report := Report{
		Provider: "bedrock",
		Model:    "eu.amazon.nova-2-lite-v1:0",
		AIRuns:   []AIRun{{Provider: "bedrock", Model: "eu.amazon.nova-2-lite-v1:0", Status: "success"}},
		Targets: []TargetResult{{
			Domain: "hitkeep.com",
			Opportunities: []api.Opportunity{{
				Kind:             "revenue",
				TypeKey:          "opportunities.types.traffic_quality",
				CopyParams:       map[string]any{"source": "Open Alternative", "source_hits": 1800, "total_pageviews": 1800, "monthly_upside": "8500"},
				ImpactLabelKey:   "opportunities.impact.estimated_monthly_upside",
				CitedEvidenceIDs: []string{"source_hits", "total_pageviews"},
				Evidence:         []api.OpportunityEvidence{{ID: "source_hits", LabelKey: "opportunities.evidence.source_hits", Value: "1800"}},
				TitleKey:         "opportunities.catalog.traffic_quality.title",
				SummaryKey:       "opportunities.catalog.traffic_quality.summary",
				ActionKey:        "opportunities.catalog.traffic_quality.action",
				DigestKey:        "opportunities.catalog.traffic_quality.digest",
				DetectorVersion:  "opportunities-detectors-v1",
				Status:           "new",
				RouteLabelKey:    "opportunities.routes.source",
				RouteIcon:        "pi pi-chart-line",
			}},
		}},
	}

	verdict := Evaluate(report)
	if verdict.MoneyCopyClean || verdict.SourceAttributionClean || verdict.ReleaseReady {
		t.Fatalf("expected unsafe report to fail release gate, got %#v", verdict)
	}
}

func TestEvaluateRejectsWeakOrImpossibleTrafficSourceEvidence(t *testing.T) {
	tests := map[string]map[string]any{
		"missing source":                  {"source_hits": 120, "total_pageviews": 1800},
		"nil source":                      {"source": nil, "source_hits": 120, "total_pageviews": 1800},
		"zero source hits":                {"source": "Open Alternative", "source_hits": 0, "total_pageviews": 1800},
		"fractional source hits":          {"source": "Open Alternative", "source_hits": 120.5, "total_pageviews": 1800},
		"source hits equal total traffic": {"source": "Open Alternative", "source_hits": 1800, "total_pageviews": 1800},
		"source hits exceed total":        {"source": "Open Alternative", "source_hits": 1900, "total_pageviews": 1800},
		"missing total pageviews":         {"source": "Open Alternative", "source_hits": 120},
	}
	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			report := cleanBedrockNovaReport()
			report.Targets = append(report.Targets, TargetResult{Domain: "cloud.hitkeep.eu", From: report.Targets[0].From, To: report.Targets[0].To})
			report.Targets[0].Opportunities[0].CopyParams = params

			verdict := Evaluate(report)
			if verdict.SourceAttributionClean || verdict.ReleaseReady {
				t.Fatalf("expected weak source evidence to block release, got %#v", verdict)
			}
		})
	}
}

func TestEvaluateRejectsTrafficSourceOpportunitiesWithoutCitedSourceCounts(t *testing.T) {
	report := cleanBedrockNovaReport()
	report.Targets = append(report.Targets, TargetResult{Domain: "cloud.hitkeep.eu", From: report.Targets[0].From, To: report.Targets[0].To})
	report.Targets[0].Opportunities[0].CitedEvidenceIDs = []string{"total_pageviews"}

	verdict := Evaluate(report)
	if verdict.SourceAttributionClean || verdict.ReleaseReady {
		t.Fatalf("expected traffic source opportunity without cited source counts to block release, got %#v", verdict)
	}
	if !strings.Contains(strings.Join(verdict.Failures, "\n"), "source-specific counts") {
		t.Fatalf("expected source-specific count blocker, got %#v", verdict.Failures)
	}
}

func TestEvaluateRejectsTrafficSourceOpportunitiesWithMismatchedSourceCountEvidence(t *testing.T) {
	report := cleanBedrockNovaReport()
	report.Targets = append(report.Targets, TargetResult{Domain: "cloud.hitkeep.eu", From: report.Targets[0].From, To: report.Targets[0].To})
	report.Targets[0].Opportunities[0].Evidence = []api.OpportunityEvidence{
		{ID: "source_hits", LabelKey: "opportunities.evidence.source_hits", Value: "1"},
		{ID: "total_pageviews", LabelKey: "opportunities.evidence.total_pageviews", Value: "1800"},
	}

	verdict := Evaluate(report)
	if verdict.SourceAttributionClean || verdict.ReleaseReady {
		t.Fatalf("expected mismatched source count evidence to block release, got %#v", verdict)
	}
}

func TestEvaluateAcceptsStringifiedTrafficSourceCounts(t *testing.T) {
	report := cleanBedrockNovaReport()
	report.Targets = append(report.Targets, TargetResult{Domain: "cloud.hitkeep.eu", From: report.Targets[0].From, To: report.Targets[0].To})
	report.Targets[0].Opportunities[0].CopyParams = map[string]any{
		"source":          "Open Alternative",
		"source_hits":     "120",
		"total_pageviews": "1800",
	}

	verdict := Evaluate(report)
	if !verdict.SourceAttributionClean || !verdict.ReleaseReady {
		t.Fatalf("expected stringified source evidence counts to pass release gate, got %#v", verdict)
	}
}

func TestRenderMarkdownIncludesSanitizedVerdictAndEvidence(t *testing.T) {
	report := Report{
		GeneratedAt: time.Date(2026, 5, 12, 18, 0, 0, 0, time.UTC),
		Source:      "restored EU backup",
		Provider:    "bedrock",
		Model:       "eu.amazon.nova-2-lite-v1:0",
		AIRuns:      []AIRun{{Provider: "bedrock", Model: "eu.amazon.nova-2-lite-v1:0", Status: "invalid_output", ErrorCategory: "invalid_output", OutputJSON: `{"type_key":"opportunities.types.traffic_quality"}`}},
		Targets: []TargetResult{{
			Domain: "hitkeep.com",
			From:   time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC),
			To:     time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC),
			Opportunities: []api.Opportunity{{
				Kind:           "traffic",
				TypeKey:        "opportunities.types.traffic_quality",
				CopyParams:     map[string]any{"source": "Open Alternative", "source_hits": 120, "total_pageviews": 1800},
				ImpactValue:    "120",
				ImpactLabelKey: "opportunities.impact.pageviews_to_route",
				Evidence: []api.OpportunityEvidence{
					{ID: "source_hits", LabelKey: "opportunities.evidence.source_hits", Value: "120"},
					{ID: "total_pageviews", LabelKey: "opportunities.evidence.total_pageviews", Value: "1800"},
				},
				CitedEvidenceIDs: []string{"source_hits"},
				Confidence:       "medium",
				Score:            72,
			}},
		}},
	}

	markdown := RenderMarkdown(report)
	for _, want := range []string{"# Opportunities Release-Hardening Smoke Report", "Bedrock Nova complete", "Target coverage complete", "Cited evidence complete", "Privacy clean", "Open Alternative", "source_hits", "invalid_output"} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("expected report to contain %q:\n%s", want, markdown)
		}
	}
	for _, forbidden := range []string{"raw_prompt", "provider_response", "user_agent", "ip_address"} {
		if strings.Contains(markdown, forbidden) {
			t.Fatalf("report leaked forbidden token %q:\n%s", forbidden, markdown)
		}
	}
}

func cleanBedrockNovaReport() Report {
	return Report{
		GeneratedAt: time.Date(2026, 5, 12, 18, 0, 0, 0, time.UTC),
		Source:      "restored EU backup",
		Provider:    "bedrock",
		Model:       "eu.amazon.nova-2-lite-v1:0",
		AIRuns:      []AIRun{{Provider: "bedrock", Model: "eu.amazon.nova-2-lite-v1:0", Status: "success"}},
		Targets: []TargetResult{{
			Domain: "hitkeep.com",
			From:   time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC),
			To:     time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC),
			Opportunities: []api.Opportunity{{
				ID:             uuid.New(),
				Kind:           "traffic",
				TypeKey:        "opportunities.types.traffic_quality",
				CopyParams:     map[string]any{"source": "Open Alternative", "source_hits": 120, "total_pageviews": 1800},
				ImpactValue:    "120",
				ImpactLabelKey: "opportunities.impact.pageviews_to_route",
				Evidence: []api.OpportunityEvidence{
					{ID: "source_hits", LabelKey: "opportunities.evidence.source_hits", Value: "120"},
					{ID: "total_pageviews", LabelKey: "opportunities.evidence.total_pageviews", Value: "1800"},
				},
				CitedEvidenceIDs: []string{"source_hits", "total_pageviews"},
				Confidence:       "medium",
				Score:            72,
				TitleKey:         "opportunities.catalog.traffic_quality.title",
				SummaryKey:       "opportunities.catalog.traffic_quality.summary",
				ActionKey:        "opportunities.catalog.traffic_quality.action",
				DigestKey:        "opportunities.catalog.traffic_quality.digest",
				DetectorVersion:  "opportunities-detectors-v1",
				Status:           "new",
				RouteLabelKey:    "opportunities.routes.source",
				RouteIcon:        "pi pi-chart-line",
				GeneratedAt:      time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC),
				CreatedAt:        time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC),
				UpdatedAt:        time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC),
			}},
		}},
	}
}
