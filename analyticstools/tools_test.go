package analyticstools

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	goaisdk "github.com/zendev-sh/goai"

	"hitkeep/api"
	"hitkeep/database"
	json "hitkeep/jsonapi"
)

func TestEvidenceToolsNeverIncludeAnnotations(t *testing.T) {
	// Opportunities cites evidence tools: notes explain data but must never become evidence.
	for _, tool := range Evidence() {
		if tool.Name == ToolAnnotations {
			t.Fatal("annotations offered as evidence")
		}
	}
}

func TestBoundScopeReadsOnlyItsSite(t *testing.T) {
	store, siteID, authorID := setupAnnotationsStore(t)
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	for _, input := range []api.AnnotationInput{
		{StartsAt: time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC), Body: "Pricing launch"},
		{StartsAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), Body: "Outside the window"},
	} {
		if _, err := store.CreateAnnotation(context.Background(), siteID, input, authorID); err != nil {
			t.Fatalf("create annotation: %v", err)
		}
	}
	var resolved []uuid.UUID
	scope := Scope{
		SiteID: siteID,
		Resolve: func(_ context.Context, id uuid.UUID) (Site, error) {
			resolved = append(resolved, id)
			return Site{ID: id, Control: store, Analytics: store}, nil
		},
		From: from, To: to,
	}
	tool := goaiTool(t, scope, Annotations)
	if strings.Contains(string(tool.InputSchema), "site_id") {
		t.Fatalf("bound scope exposes site_id: %s", tool.InputSchema)
	}

	// A model that names another site anyway still reads the bound one.
	raw, err := tool.Execute(context.Background(), []byte(`{"site_id":"`+uuid.NewString()+`"}`))
	if err != nil {
		t.Fatalf("execute annotations tool: %v", err)
	}
	if !slices.Equal(resolved, []uuid.UUID{siteID}) {
		t.Fatalf("resolved sites = %v, want only %s", resolved, siteID)
	}
	var body struct {
		EvidenceID string `json:"evidence_id"`
		Data       struct {
			SiteID      string `json:"site_id"`
			Annotations []struct {
				Body string `json:"body"`
			} `json:"annotations"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("decode tool output: %v", err)
	}
	if body.EvidenceID != ToolAnnotations || body.Data.SiteID != siteID.String() {
		t.Fatalf("envelope = %+v", body)
	}
	if len(body.Data.Annotations) != 1 || body.Data.Annotations[0].Body != "Pricing launch" {
		t.Fatalf("notes = %+v, want only the in-range note", body.Data.Annotations)
	}
	if strings.Contains(raw, "created_by") || strings.Contains(raw, authorID.String()) {
		t.Fatalf("tool output exposes the author: %s", raw)
	}
}

func TestScopeRanges(t *testing.T) {
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	scope := Scope{From: from, To: to, MaxRangeDays: 90}

	start, end, err := scope.window(Target{})
	if err != nil || !start.Equal(from) || !end.Equal(to) {
		t.Fatalf("default window = %v..%v (%v), want the scope range", start, end, err)
	}
	start, end, err = scope.window(Target{From: "2026-08-01T00:00:00Z", To: "2026-09-01T00:00:00Z"})
	if err != nil || start.Month() != time.August || end.Month() != time.September {
		t.Fatalf("named window = %v..%v (%v), want the previous period", start, end, err)
	}
	if _, _, err := scope.window(Target{From: "2025-01-01T00:00:00Z", To: "2026-01-01T00:00:00Z"}); err == nil {
		t.Fatal("window beyond MaxRangeDays accepted")
	}

	scope.LockRange = true
	start, end, err = scope.window(Target{From: "2026-08-01T00:00:00Z", To: "2026-09-01T00:00:00Z"})
	if err != nil || !start.Equal(from) || !end.Equal(to) {
		t.Fatalf("locked window = %v..%v (%v), want the scope range", start, end, err)
	}
	schema := string(goaiTool(t, scope, SiteOverview).InputSchema)
	for _, input := range rangeInputs {
		if strings.Contains(schema, `"`+input+`"`) {
			t.Fatalf("locked scope exposes %s: %s", input, schema)
		}
	}
}

func TestGoAIKeepsInternalErrorsFromTheModel(t *testing.T) {
	scope := Scope{SiteID: uuid.New(), Resolve: func(_ context.Context, id uuid.UUID) (Site, error) { return Site{ID: id}, nil }}
	for _, tc := range []struct {
		err       error
		wantShown bool
	}{
		{InvalidInput("event_name is required"), true},
		{fmt.Errorf("query events: %w", errors.New(`duckdb: table "hits" is locked`)), false},
	} {
		tool := Define("probe", "Probe", "Probe.", func(context.Context, Call, Target) (struct{}, error) { return struct{}{}, tc.err })
		_, err := goaiTool(t, scope, tool).Execute(context.Background(), []byte(`{}`))
		if shown := err != nil && err.Error() == tc.err.Error(); shown != tc.wantShown {
			t.Errorf("%v reached the model as %v", tc.err, err)
		}
		if err == nil || strings.Contains(err.Error(), "duckdb") {
			t.Errorf("tool error = %v", err)
		}
	}
}

func TestGoAISchemasAvoidTypeUnions(t *testing.T) {
	for _, tool := range GoAI(Scope{SiteID: uuid.New()}, Analytics()...) {
		if schema := string(tool.InputSchema); strings.Contains(schema, `"type":[`) {
			t.Fatalf("%s schema has a type union some providers reject: %s", tool.Name, schema)
		}
	}
}

func TestSiteStatsKeepsRequestedSections(t *testing.T) {
	stats := &siteStats{
		TopPages:     make([]api.MetricStat, 10),
		TopCountries: make([]api.MetricStat, 10),
		ChartData:    make([]chartDataPoint, 3),
		Goals:        make([]goalStats, 1),
		Comparison:   &comparisonStats{ChartData: make([]chartDataPoint, 3)},
	}
	sections, err := parseSections([]string{"pages"}, overviewSections, overviewSections)
	if err != nil {
		t.Fatal(err)
	}
	stats.keep(sections, 3)
	if len(stats.TopPages) != 3 {
		t.Fatalf("top pages = %d, want capped at 3", len(stats.TopPages))
	}
	if stats.TopCountries != nil || stats.ChartData != nil || stats.Goals != nil || stats.Comparison.ChartData != nil {
		t.Fatalf("unrequested sections kept: %+v", stats)
	}
	if _, err := parseSections([]string{"raw_hits"}, overviewSections, overviewSections); err == nil {
		t.Fatal("unknown section accepted")
	}
}

func goaiTool(t *testing.T, scope Scope, tool Tool) goaisdk.Tool {
	t.Helper()
	tools := GoAI(scope, tool)
	if len(tools) != 1 {
		t.Fatalf("GoAI returned %d tools", len(tools))
	}
	return tools[0]
}

func setupAnnotationsStore(t *testing.T) (*database.Store, uuid.UUID, uuid.UUID) {
	t.Helper()
	store := database.NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	userID, err := store.CreateUser(context.Background(), "notes@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(context.Background(), userID, "notes.example")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	return store, site.ID, userID
}
