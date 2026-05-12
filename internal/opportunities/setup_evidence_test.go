package opportunities

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

func TestBuildSetupEvidenceSnapshotIncludesConfiguredAndAggregateSignals(t *testing.T) {
	shared, site, _, _ := setupOpportunityServiceTestStore(t)
	ctx := context.Background()
	from := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 30)

	requireNoError(t, shared.CreateGoal(ctx, &api.Goal{
		SiteID:    site.ID,
		Name:      "Demo request",
		Type:      "event",
		Value:     "demo_request",
		CreatedAt: from.Add(time.Hour),
	}), "create goal")
	requireNoError(t, shared.CreateFunnel(ctx, &api.Funnel{
		SiteID: site.ID,
		Name:   "Trial signup",
		Steps: []api.FunnelStep{
			{Type: "path", Value: "/pricing"},
			{Type: "event", Value: "demo_request"},
		},
		CreatedAt: from.Add(2 * time.Hour),
	}), "create funnel")

	firstSession := uuid.New()
	secondSession := uuid.New()
	isUnique := true
	requireNoError(t, shared.CreateHit(ctx, &api.Hit{
		SiteID:    site.ID,
		SessionID: firstSession,
		PageID:    uuid.New(),
		Path:      "/pricing",
		Timestamp: from.Add(24 * time.Hour),
		IsUnique:  &isUnique,
	}), "create pricing hit")
	requireNoError(t, shared.CreateHit(ctx, &api.Hit{
		SiteID:    site.ID,
		SessionID: secondSession,
		PageID:    uuid.New(),
		Path:      "/checkout",
		Timestamp: from.Add(25 * time.Hour),
		IsUnique:  &isUnique,
	}), "create checkout hit")
	for _, event := range []*api.Event{
		{SiteID: site.ID, SessionID: firstSession, Name: "demo_request", Timestamp: from.Add(24*time.Hour + 10*time.Minute)},
		{SiteID: site.ID, SessionID: firstSession, Name: "begin_checkout", Timestamp: from.Add(24*time.Hour + 20*time.Minute)},
		{
			SiteID:    site.ID,
			SessionID: firstSession,
			Name:      "purchase",
			Timestamp: from.Add(24*time.Hour + 30*time.Minute),
			Properties: map[string]any{
				"transaction_id": "ord_1001",
				"value":          120.0,
				"currency":       "EUR",
				"item_id":        "pro-plan",
				"item_name":      "Pro Plan",
			},
		},
	} {
		requireNoError(t, shared.CreateEvent(ctx, event), "create event "+event.Name)
	}

	snapshot, err := buildSetupEvidenceSnapshot(ctx, setupEvidenceSnapshotInput{
		SharedStore:    shared,
		AnalyticsStore: shared,
		SiteID:         site.ID,
		From:           from,
		To:             to,
	})
	requireNoError(t, err, "build setup evidence snapshot")
	requireSetupSnapshotIdentity(t, snapshot, site.ID, from, to)
	requireSetupGoals(t, snapshot.Goals)
	requireSetupFunnels(t, snapshot.Funnels)
	requireSetupEvents(t, snapshot.EventNames)
	requireSetupEventCounts(t, snapshot.Events)
	requireSetupTopPages(t, snapshot.TopPages)
	requireSetupEcommerce(t, snapshot.Ecommerce)
	requireSetupState(t, snapshot.SetupState)
}

func TestBuildSetupEvidenceSnapshotDoesNotExposeRawRows(t *testing.T) {
	shared, site, _, _ := setupOpportunityServiceTestStore(t)
	ctx := context.Background()
	from := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 30)
	sessionID := uuid.New()
	userAgent := "SensitiveBrowser/7.7 secret-user-agent"
	referrer := "https://example.test/private-path"
	isUnique := true

	requireNoError(t, shared.CreateHit(ctx, &api.Hit{
		SiteID:    site.ID,
		SessionID: sessionID,
		PageID:    uuid.New(),
		Path:      "/pricing",
		Referrer:  &referrer,
		UserAgent: &userAgent,
		Timestamp: from.Add(24 * time.Hour),
		IsUnique:  &isUnique,
	}), "create hit")
	requireNoError(t, shared.CreateEvent(ctx, &api.Event{
		SiteID:    site.ID,
		SessionID: sessionID,
		Name:      "demo_request",
		Properties: map[string]any{
			"email":      "private@example.test",
			"user_agent": userAgent,
		},
		Timestamp: from.Add(24*time.Hour + 10*time.Minute),
	}), "create event")

	snapshot, err := buildSetupEvidenceSnapshot(ctx, setupEvidenceSnapshotInput{
		SharedStore:    shared,
		AnalyticsStore: shared,
		SiteID:         site.ID,
		From:           from,
		To:             to,
	})
	requireNoError(t, err, "build setup evidence snapshot")
	raw, err := json.Marshal(snapshot)
	requireNoError(t, err, "marshal snapshot")
	body := string(raw)
	for _, forbidden := range []string{
		sessionID.String(),
		userAgent,
		referrer,
		"private@example.test",
		"session_id",
		"user_agent",
		"properties",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("snapshot leaked raw row data %q in %s", forbidden, body)
		}
	}
}

func TestLoadOpportunitySignalsCanProvideSetupEvidenceSnapshot(t *testing.T) {
	shared, site, teamID, actorID := setupOpportunityServiceTestStore(t)
	ctx := context.Background()
	from := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 30)
	requireNoError(t, shared.CreateGoal(ctx, &api.Goal{
		SiteID: site.ID,
		Name:   "Signup",
		Type:   "event",
		Value:  "signup",
	}), "create goal")

	signals, err := loadOpportunitySignals(ctx, shared, GenerateInput{
		Store:           shared,
		Site:            site,
		TeamID:          teamID,
		ActorID:         actorID,
		EffectiveUserID: actorID,
		From:            from,
		To:              to,
	}, []OpportunitySignal{OpportunitySignalSetupEvidence})
	if err != nil {
		t.Fatalf("load opportunity signals: %v", err)
	}
	if signals.SetupEvidence == nil {
		t.Fatal("expected setup evidence signal")
	}
	if len(signals.SetupEvidence.Goals) != 1 || signals.SetupEvidence.Goals[0].Name != "Signup" {
		t.Fatalf("expected setup goal evidence, got %+v", signals.SetupEvidence.Goals)
	}
	if signals.Stats != nil || signals.Ecommerce != nil || signals.EventNames != nil {
		t.Fatalf("expected setup evidence to be the only requested signal, got %+v", signals)
	}
}

func TestSetupEvidenceSnapshotBoundsPromptFacingCollections(t *testing.T) {
	goals := make([]api.Goal, 0, defaultSetupEvidenceGoalLimit+2)
	longName := strings.Repeat("x", defaultSetupEvidenceStringLimit+20)
	for range defaultSetupEvidenceGoalLimit + 2 {
		goals = append(goals, api.Goal{Name: longName, Type: "event", Value: "signup"})
	}
	eventNames := make([]string, 0, defaultSetupEvidenceEventLimit+2)
	for range defaultSetupEvidenceEventLimit + 2 {
		eventNames = append(eventNames, longName)
	}

	goalEvidence := setupGoalEvidence(goals)
	if len(goalEvidence) != defaultSetupEvidenceGoalLimit {
		t.Fatalf("expected %d goals, got %d", defaultSetupEvidenceGoalLimit, len(goalEvidence))
	}
	if got := utf8.RuneCountInString(goalEvidence[0].Name); got != defaultSetupEvidenceStringLimit {
		t.Fatalf("expected truncated goal name to %d runes, got %d", defaultSetupEvidenceStringLimit, got)
	}
	safeEventNames := setupEventNames(eventNames)
	if len(safeEventNames) != defaultSetupEvidenceEventLimit {
		t.Fatalf("expected %d event names, got %d", defaultSetupEvidenceEventLimit, len(safeEventNames))
	}
	if got := utf8.RuneCountInString(safeEventNames[0]); got != defaultSetupEvidenceStringLimit {
		t.Fatalf("expected truncated event name to %d runes, got %d", defaultSetupEvidenceStringLimit, got)
	}
}

func containsString(values []string, want string) bool {
	return slices.Contains(values, want)
}

func requireNoError(t *testing.T, err error, label string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", label, err)
	}
}

func requireSetupSnapshotIdentity(t *testing.T, snapshot *SetupEvidenceSnapshot, siteID uuid.UUID, from, to time.Time) {
	t.Helper()
	if snapshot.SiteID != siteID || !snapshot.From.Equal(from) || !snapshot.To.Equal(to) {
		t.Fatalf("unexpected snapshot identity/window: %+v", snapshot)
	}
}

func requireSetupGoals(t *testing.T, goals []SetupGoalEvidence) {
	t.Helper()
	if len(goals) != 1 || goals[0].Name != "Demo request" || goals[0].Value != "demo_request" {
		t.Fatalf("expected configured goal evidence, got %+v", goals)
	}
}

func requireSetupFunnels(t *testing.T, funnels []SetupFunnelEvidence) {
	t.Helper()
	if len(funnels) != 1 || funnels[0].Name != "Trial signup" || len(funnels[0].Steps) != 2 {
		t.Fatalf("expected configured funnel evidence, got %+v", funnels)
	}
}

func requireSetupEvents(t *testing.T, eventNames []string) {
	t.Helper()
	if !containsString(eventNames, "demo_request") || !containsString(eventNames, "purchase") {
		t.Fatalf("expected aggregate event names, got %+v", eventNames)
	}
}

func requireSetupEventCounts(t *testing.T, events []SetupEventEvidence) {
	t.Helper()
	if !hasSetupEventCount(events, "demo_request", 1) || !hasSetupEventCount(events, "purchase", 1) {
		t.Fatalf("expected aggregate event counts, got %+v", events)
	}
}

func hasSetupEventCount(events []SetupEventEvidence, name string, count int) bool {
	for _, event := range events {
		if event.Name == name && event.Count == count {
			return true
		}
	}
	return false
}

func requireSetupTopPages(t *testing.T, topPages []SetupTopPageEvidence) {
	t.Helper()
	if len(topPages) == 0 || topPages[0].Path == "" {
		t.Fatalf("expected aggregate top pages, got %+v", topPages)
	}
}

func requireSetupEcommerce(t *testing.T, ecommerce SetupEcommerceEvidence) {
	t.Helper()
	if ecommerce.CheckoutStarts != 1 || ecommerce.Orders != 1 || ecommerce.Currency != "EUR" {
		t.Fatalf("expected ecommerce aggregate evidence, got %+v", ecommerce)
	}
}

func requireSetupState(t *testing.T, state SetupStateEvidence) {
	t.Helper()
	if !state.HasGoals || !state.HasFunnels || !state.HasConversionEvent || !state.HasCheckoutSignal || !state.HasOrderSignal {
		t.Fatalf("expected setup state flags, got %+v", state)
	}
}
