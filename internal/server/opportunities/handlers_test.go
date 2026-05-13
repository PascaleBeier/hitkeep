package opportunities

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	hitai "hitkeep/internal/ai"
	"hitkeep/internal/api"
	authcore "hitkeep/internal/auth"
	"hitkeep/internal/database"
	"hitkeep/internal/server/shared"
)

func setupOpportunityHandlerTestEnv(t *testing.T) (*database.Store, *shared.Context, uuid.UUID, uuid.UUID) {
	t.Helper()

	store := database.NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	userID, err := store.CreateUser(context.Background(), "opportunities@example.com", "hashed")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	site, err := store.CreateSite(context.Background(), userID, "opportunities.example.com")
	if err != nil {
		t.Fatalf("CreateSite: %v", err)
	}
	teamID, err := store.GetSiteTenantID(context.Background(), site.ID)
	if err != nil {
		t.Fatalf("GetSiteTenantID: %v", err)
	}

	ctx := &shared.Context{Store: store}

	return store, ctx, site.ID, teamID
}

func TestHandleListOpportunitiesReturnsLocalizationContract(t *testing.T) {
	store, ctx, siteID, teamID := setupOpportunityHandlerTestEnv(t)
	opportunityID := uuid.New()

	_, err := store.UpsertOpportunities(context.Background(), []database.OpportunityInput{{
		ID:               opportunityID,
		TeamID:           teamID,
		SiteID:           siteID,
		Kind:             "conversion",
		TypeKey:          "opportunities.types.checkout_conversion",
		TitleKey:         "opportunities.catalog.checkout_conversion.title",
		SummaryKey:       "opportunities.catalog.checkout_conversion.summary",
		ActionKey:        "opportunities.catalog.checkout_conversion.action",
		DigestKey:        "opportunities.catalog.checkout_conversion.digest",
		CopyParams:       map[string]any{"conversion_rate": "42%"},
		ImpactValue:      "$900",
		ImpactLabelKey:   "opportunities.impact.checkout_starts",
		Confidence:       "medium",
		Score:            84,
		ScoreBreakdown:   api.OpportunityScoreBreakdown{Sample: 82, Impact: 70, Urgency: 55, EvidenceFit: 99, Total: 84},
		Status:           "new",
		RouteLabelKey:    "opportunities.routes.checkout",
		RouteParams:      map[string]any{"path": "/checkout"},
		RouteIcon:        "pi pi-filter",
		DetectorVersion:  "opportunities-v1",
		Evidence:         []api.OpportunityEvidence{{ID: "checkout-rate", LabelKey: "opportunities.evidence.checkout_conversion_rate", Value: "42%"}},
		CitedEvidenceIDs: []string{"checkout-rate"},
		GeneratedAt:      time.Now().UTC(),
	}})
	if err != nil {
		t.Fatalf("upsert opportunities: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+siteID.String()+"/opportunities", nil)
	req.SetPathValue("id", siteID.String())
	rec := httptest.NewRecorder()
	h := &handler{ctx: ctx}
	h.handleList().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body map[string][]map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	opportunities := body["opportunities"]
	if len(opportunities) != 1 {
		t.Fatalf("expected one opportunity, got %#v", body)
	}
	opportunity := opportunities[0]
	for _, forbidden := range []string{"title", "summary", "next_action", "impact_label", "route_label", "plan", "team_id", "ai_run_id"} {
		if _, ok := opportunity[forbidden]; ok {
			t.Fatalf("response leaked prose field %q: %#v", forbidden, opportunity)
		}
	}
	for _, required := range []string{"type_key", "title_key", "summary_key", "action_key", "copy_params", "impact_label_key", "route_label_key", "score_breakdown", "cited_evidence_ids"} {
		if _, ok := opportunity[required]; !ok {
			t.Fatalf("response missing localization field %q: %#v", required, opportunity)
		}
	}
	scoreBreakdown, ok := opportunity["score_breakdown"].(map[string]any)
	if !ok || scoreBreakdown["total"] != float64(84) || scoreBreakdown["evidence_fit"] != float64(99) {
		t.Fatalf("expected safe score breakdown, got %#v", opportunity["score_breakdown"])
	}
	evidence, ok := opportunity["evidence"].([]any)
	if !ok || len(evidence) != 1 {
		t.Fatalf("expected one evidence item, got %#v", opportunity["evidence"])
	}
	evidenceItem, ok := evidence[0].(map[string]any)
	if !ok {
		t.Fatalf("expected evidence object, got %#v", evidence[0])
	}
	if _, ok := evidenceItem["label"]; ok {
		t.Fatalf("evidence leaked prose label: %#v", evidenceItem)
	}
	if _, ok := evidenceItem["label_key"]; !ok {
		t.Fatalf("evidence missing label_key: %#v", evidenceItem)
	}
}

func TestHandleListOpportunitiesReturnsEmptyArrayWhenNoRowsExist(t *testing.T) {
	_, ctx, siteID, _ := setupOpportunityHandlerTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+siteID.String()+"/opportunities", nil)
	req.SetPathValue("id", siteID.String())
	rec := httptest.NewRecorder()
	h := &handler{ctx: ctx}
	h.handleList().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := requireJSONMap(t, rec.Body.Bytes())
	opportunities, ok := body["opportunities"].([]any)
	if !ok {
		t.Fatalf("expected opportunities to be an array, got %#v in %s", body["opportunities"], rec.Body.String())
	}
	if len(opportunities) != 0 {
		t.Fatalf("expected no opportunities, got %#v", opportunities)
	}
}

func TestHandleListOpportunitiesRanksActionableWorkFirst(t *testing.T) {
	store, ctx, siteID, teamID := setupOpportunityHandlerTestEnv(t)
	lowActiveID := uuid.New()
	highActiveID := uuid.New()
	doneID := uuid.New()

	_, err := store.UpsertOpportunities(context.Background(), []database.OpportunityInput{
		listRankingOpportunity(teamID, siteID, lowActiveID, "new", 60, api.OpportunityScoreBreakdown{Impact: 55, Actionability: 70, EvidenceFit: 70, Total: 60}),
		listRankingOpportunity(teamID, siteID, doneID, "done", 99, api.OpportunityScoreBreakdown{Impact: 99, Actionability: 99, EvidenceFit: 99, Total: 99}),
		listRankingOpportunity(teamID, siteID, highActiveID, "saved", 80, api.OpportunityScoreBreakdown{Impact: 75, Actionability: 90, EvidenceFit: 90, Total: 80}),
	})
	if err != nil {
		t.Fatalf("upsert opportunities: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+siteID.String()+"/opportunities", nil)
	req.SetPathValue("id", siteID.String())
	rec := httptest.NewRecorder()
	h := &handler{ctx: ctx}
	h.handleList().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body api.OpportunityListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	got := listedOpportunityIDs(body.Opportunities)
	want := []uuid.UUID{highActiveID, lowActiveID, doneID}
	if !sameListedUUIDs(got, want) {
		t.Fatalf("expected ranked opportunities %v, got %v", want, got)
	}
}

func TestHandleDigestPreviewReturnsSafeLocalizationContract(t *testing.T) {
	store, ctx, siteID, teamID := setupOpportunityHandlerTestEnv(t)
	lowActiveID := uuid.New()
	highActiveID := uuid.New()
	doneID := uuid.New()

	_, err := store.UpsertOpportunities(context.Background(), []database.OpportunityInput{
		listRankingOpportunity(teamID, siteID, lowActiveID, "new", 60, api.OpportunityScoreBreakdown{Impact: 55, Actionability: 70, EvidenceFit: 70, Total: 60}),
		listRankingOpportunity(teamID, siteID, doneID, "done", 99, api.OpportunityScoreBreakdown{Impact: 99, Actionability: 99, EvidenceFit: 99, Total: 99}),
		listRankingOpportunity(teamID, siteID, highActiveID, "saved", 88, api.OpportunityScoreBreakdown{Impact: 86, Actionability: 90, EvidenceFit: 90, Total: 88}),
	})
	if err != nil {
		t.Fatalf("upsert opportunities: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+siteID.String()+"/opportunities/digest-preview?frequency=weekly", nil)
	req.SetPathValue("id", siteID.String())
	rec := httptest.NewRecorder()
	h := &handler{ctx: ctx}
	h.handleDigestPreview().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	body := requireJSONMap(t, rec.Body.Bytes())
	requireDigestPreviewMetadata(t, body, "weekly", true, "ready")
	items := requireJSONArrayLen(t, body["items"], 2)
	first := requireJSONObject(t, items[0])
	if first["id"] != highActiveID.String() {
		t.Fatalf("expected high-scoring saved opportunity first, got %#v", first["id"])
	}
	requireFieldsPresent(t, first, "digest item", "title_key", "action_key", "digest_key", "copy_params", "impact_label_key", "score_breakdown", "evidence", "cited_evidence_ids")
	requireFieldsAbsent(t, first, "digest preview", "title", "summary", "digest", "action", "team_id", "ai_run_id", "raw_prompt", "raw_provider_response")
	requireSafeDigestEvidence(t, first)
}

func TestHandleDigestPreviewRejectsUnsupportedFrequency(t *testing.T) {
	_, ctx, siteID, _ := setupOpportunityHandlerTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+siteID.String()+"/opportunities/digest-preview?frequency=monthly", nil)
	req.SetPathValue("id", siteID.String())
	rec := httptest.NewRecorder()
	h := &handler{ctx: ctx}

	h.handleDigestPreview().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Unsupported opportunity digest frequency") {
		t.Fatalf("expected stable unsupported-frequency error, got %q", rec.Body.String())
	}
}

func TestHandleDigestPreviewDefaultsToWeeklyNoSendState(t *testing.T) {
	_, ctx, siteID, _ := setupOpportunityHandlerTestEnv(t)
	req := httptest.NewRequest(http.MethodGet, "/api/sites/"+siteID.String()+"/opportunities/digest-preview", nil)
	req.SetPathValue("id", siteID.String())
	rec := httptest.NewRecorder()
	h := &handler{ctx: ctx}

	h.handleDigestPreview().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := requireJSONMap(t, rec.Body.Bytes())
	requireDigestPreviewMetadata(t, body, "weekly", false, "no_opportunities")
	items, ok := body["items"].([]any)
	if !ok {
		t.Fatalf("expected digest preview items to be an array, got %#v in %s", body["items"], rec.Body.String())
	}
	if len(items) != 0 {
		t.Fatalf("expected no preview items, got %#v", items)
	}
}

func TestHandleGenerateRejectsInvalidRanges(t *testing.T) {
	_, ctx, siteID, _ := setupOpportunityHandlerTestEnv(t)
	h := &handler{ctx: ctx}

	tests := []struct {
		name  string
		query string
	}{
		{name: "invalid from", query: "?from=not-a-date&to=2026-05-09T00:00:00Z"},
		{name: "to before from", query: "?from=2026-05-10T00:00:00Z&to=2026-05-09T00:00:00Z"},
		{name: "range too large", query: "?from=2024-01-01T00:00:00Z&to=2026-05-09T00:00:00Z"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/sites/"+siteID.String()+"/opportunities/generate"+tc.query, nil)
			req.SetPathValue("id", siteID.String())
			rec := httptest.NewRecorder()

			h.handleGenerate().ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func listRankingOpportunity(teamID, siteID, id uuid.UUID, status string, score int, breakdown api.OpportunityScoreBreakdown) database.OpportunityInput {
	return database.OpportunityInput{
		ID:               id,
		TeamID:           teamID,
		SiteID:           siteID,
		Kind:             "conversion",
		TypeKey:          "opportunities.types.checkout_conversion",
		TitleKey:         "opportunities.catalog.checkout_conversion.title",
		SummaryKey:       "opportunities.catalog.checkout_conversion.summary",
		ActionKey:        "opportunities.catalog.checkout_conversion.action",
		DigestKey:        "opportunities.catalog.checkout_conversion.digest",
		CopyParams:       map[string]any{"conversion_rate": "42%"},
		ImpactValue:      "$900",
		ImpactLabelKey:   "opportunities.impact.checkout_starts",
		Confidence:       "medium",
		Score:            score,
		ScoreBreakdown:   breakdown,
		Status:           status,
		RouteLabelKey:    "opportunities.routes.checkout",
		RouteParams:      map[string]any{"path": "/checkout"},
		RouteIcon:        "pi pi-filter",
		DetectorVersion:  "opportunities-v1",
		Evidence:         []api.OpportunityEvidence{{ID: "checkout-rate", LabelKey: "opportunities.evidence.checkout_conversion_rate", Value: "42%"}},
		CitedEvidenceIDs: []string{"checkout-rate"},
		GeneratedAt:      time.Now().UTC(),
	}
}

func listedOpportunityIDs(opportunities []api.Opportunity) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(opportunities))
	for _, opportunity := range opportunities {
		out = append(out, opportunity.ID)
	}
	return out
}

func sameListedUUIDs(a, b []uuid.UUID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func requireJSONMap(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}

func requireDigestPreviewMetadata(t *testing.T, body map[string]any, frequency string, shouldSend bool, reason string) {
	t.Helper()
	if body["frequency"] != frequency || body["should_send"] != shouldSend || body["reason"] != reason {
		t.Fatalf("unexpected digest preview metadata: %#v", body)
	}
}

func requireJSONArrayLen(t *testing.T, value any, want int) []any {
	t.Helper()
	items, ok := value.([]any)
	if !ok || len(items) != want {
		t.Fatalf("expected %d array items, got %#v", want, value)
	}
	return items
}

func requireJSONObject(t *testing.T, value any) map[string]any {
	t.Helper()
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected JSON object, got %#v", value)
	}
	return object
}

func requireFieldsPresent(t *testing.T, object map[string]any, label string, fields ...string) {
	t.Helper()
	for _, field := range fields {
		if _, ok := object[field]; !ok {
			t.Fatalf("%s missing field %q: %#v", label, field, object)
		}
	}
}

func requireFieldsAbsent(t *testing.T, object map[string]any, label string, fields ...string) {
	t.Helper()
	for _, field := range fields {
		if _, ok := object[field]; ok {
			t.Fatalf("%s leaked forbidden field %q: %#v", label, field, object)
		}
	}
}

func requireSafeDigestEvidence(t *testing.T, item map[string]any) {
	t.Helper()
	evidence := requireJSONArrayLen(t, item["evidence"], 1)
	evidenceItem := requireJSONObject(t, evidence[0])
	requireFieldsAbsent(t, evidenceItem, "digest evidence", "label")
	requireFieldsPresent(t, evidenceItem, "digest evidence", "label_key")
}

func TestOpportunityRoutesEnforceSitePermissions(t *testing.T) {
	store, ctx, siteID, teamID := setupOpportunityHandlerTestEnv(t)
	viewerClient, viewerToken, err := store.CreateTeamAPIClient(context.Background(), teamID, "viewer", "", map[uuid.UUID]authcore.SiteRole{
		siteID: authcore.SiteViewer,
	}, nil)
	if err != nil {
		t.Fatalf("CreateTeamAPIClient viewer: %v", err)
	}
	if viewerClient == nil {
		t.Fatal("expected viewer api client")
	}
	adminClient, adminToken, err := store.CreateTeamAPIClient(context.Background(), teamID, "admin", "", map[uuid.UUID]authcore.SiteRole{
		siteID: authcore.SiteAdmin,
	}, nil)
	if err != nil {
		t.Fatalf("CreateTeamAPIClient admin: %v", err)
	}
	if adminClient == nil {
		t.Fatal("expected admin api client")
	}

	mux := http.NewServeMux()
	Register(mux, ctx)

	listReq := httptest.NewRequest(http.MethodGet, "/api/sites/"+siteID.String()+"/opportunities", nil)
	listReq.Header.Set("Authorization", "Bearer "+viewerToken)
	listRec := httptest.NewRecorder()
	mux.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected viewer list 200, got %d: %s", listRec.Code, listRec.Body.String())
	}

	previewReq := httptest.NewRequest(http.MethodGet, "/api/sites/"+siteID.String()+"/opportunities/digest-preview?frequency=weekly", nil)
	previewReq.Header.Set("Authorization", "Bearer "+viewerToken)
	previewRec := httptest.NewRecorder()
	mux.ServeHTTP(previewRec, previewReq)
	if previewRec.Code != http.StatusOK {
		t.Fatalf("expected viewer digest preview 200, got %d: %s", previewRec.Code, previewRec.Body.String())
	}

	viewerGenerateReq := httptest.NewRequest(http.MethodPost, "/api/sites/"+siteID.String()+"/opportunities/generate", nil)
	viewerGenerateReq.Header.Set("Authorization", "Bearer "+viewerToken)
	viewerGenerateRec := httptest.NewRecorder()
	mux.ServeHTTP(viewerGenerateRec, viewerGenerateReq)
	if viewerGenerateRec.Code != http.StatusForbidden {
		t.Fatalf("expected viewer generate 403, got %d: %s", viewerGenerateRec.Code, viewerGenerateRec.Body.String())
	}

	adminGenerateReq := httptest.NewRequest(http.MethodPost, "/api/sites/"+siteID.String()+"/opportunities/generate", nil)
	adminGenerateReq.Header.Set("Authorization", "Bearer "+adminToken)
	adminGenerateRec := httptest.NewRecorder()
	mux.ServeHTTP(adminGenerateRec, adminGenerateReq)
	if adminGenerateRec.Code != http.StatusOK {
		t.Fatalf("expected admin generate 200, got %d: %s", adminGenerateRec.Code, adminGenerateRec.Body.String())
	}
}

func TestGenerateAttributesAPIClientActor(t *testing.T) {
	store, ctx, siteID, teamID := setupOpportunityHandlerTestEnv(t)
	seedSetupSuggestionEvidence(t, store, siteID)
	client, token, err := store.CreateTeamAPIClient(context.Background(), teamID, "team opportunities", "", map[uuid.UUID]authcore.SiteRole{
		siteID: authcore.SiteAdmin,
	}, nil)
	if err != nil {
		t.Fatalf("CreateTeamAPIClient: %v", err)
	}
	if client == nil {
		t.Fatal("expected api client")
	}

	ai := &recordingOpportunityAI{}
	ctx.AI = ai
	mux := http.NewServeMux()
	Register(mux, ctx)

	req := httptest.NewRequest(http.MethodPost, "/api/sites/"+siteID.String()+"/opportunities/generate", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected generate 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if ai.last.ActorType != "api_client" || ai.last.ActorID != client.ID {
		t.Fatalf("expected AI actor api_client/%s, got %s/%s", client.ID, ai.last.ActorType, ai.last.ActorID)
	}
	if ai.toolErr != nil {
		t.Fatalf("expected API client AI tool bridge to authorize with carried permissions: %v", ai.toolErr)
	}

	entries, total, err := store.ListTeamAuditEntries(context.Background(), teamID, "opportunities.generated", 5, 0)
	if err != nil {
		t.Fatalf("ListTeamAuditEntries: %v", err)
	}
	if total != 1 || len(entries) != 1 {
		t.Fatalf("expected one audit entry, got total=%d entries=%d", total, len(entries))
	}
	if entries[0].ActorRoleSnapshot != "api_client" {
		t.Fatalf("expected api_client audit actor role, got %#v", entries[0])
	}
	if !strings.Contains(entries[0].Details, "actor_type=api_client") || !strings.Contains(entries[0].Details, "api_client_id="+client.ID.String()) {
		t.Fatalf("expected api client audit details, got %q", entries[0].Details)
	}
}

func seedSetupSuggestionEvidence(t *testing.T, store *database.Store, siteID uuid.UUID) {
	t.Helper()

	sessionID := uuid.New()
	timestamp := time.Now().UTC().Add(-2 * time.Hour)
	isUnique := true
	for i := range 120 {
		if err := store.CreateHit(context.Background(), &api.Hit{
			SiteID:    siteID,
			SessionID: sessionID,
			PageID:    uuid.New(),
			Path:      "/pricing",
			Timestamp: timestamp.Add(time.Duration(i) * time.Minute),
			IsUnique:  &isUnique,
		}); err != nil {
			t.Fatalf("CreateHit: %v", err)
		}
	}
	for i := range 20 {
		if err := store.CreateEvent(context.Background(), &api.Event{
			SiteID:    siteID,
			SessionID: sessionID,
			Name:      "demo_request",
			Timestamp: timestamp.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("CreateEvent: %v", err)
		}
	}
}

type recordingOpportunityAI struct {
	last    hitai.OpportunityRequest
	toolErr error
}

func (a *recordingOpportunityAI) GenerateOpportunityProposal(_ context.Context, req hitai.OpportunityRequest) (hitai.OpportunityProposalResult, error) {
	a.last = req
	if len(req.Tools) > 0 {
		_, a.toolErr = req.Tools[0].Execute(context.Background(), nil)
	}
	cited := make([]string, 0, len(req.EvidenceSnapshot.Evidence))
	for _, evidence := range req.EvidenceSnapshot.Evidence {
		cited = append(cited, evidence.ID)
	}
	actionType := "optimize_checkout"
	if len(req.DetectorInput.AllowedActionTypes) > 0 {
		actionType = req.DetectorInput.AllowedActionTypes[0]
	}
	return hitai.OpportunityProposalResult{
		RunID: uuid.New(),
		Proposal: hitai.OpportunityCandidateProposal{
			TypeKey:          req.DetectorInput.TypeKey,
			Category:         req.DetectorInput.Category,
			ActionType:       actionType,
			Effort:           "medium",
			TitleKey:         req.DetectorInput.MessageKeys.Title,
			SummaryKey:       req.DetectorInput.MessageKeys.Summary,
			ActionKey:        req.DetectorInput.MessageKeys.Action,
			DigestKey:        req.DetectorInput.MessageKeys.Digest,
			CopyParams:       req.DetectorInput.CopyParams,
			CitedEvidenceIDs: cited,
		},
	}, nil
}

func (a *recordingOpportunityAI) Configured() bool { return true }
func (a *recordingOpportunityAI) Enabled() bool    { return true }
func (a *recordingOpportunityAI) Provider() string { return "test" }
func (a *recordingOpportunityAI) Model() string    { return "test-model" }
