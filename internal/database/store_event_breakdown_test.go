package database

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

func setupEventBreakdownStore(t *testing.T) (*Store, uuid.UUID) {
	t.Helper()
	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	userID, err := store.CreateUser(context.Background(), "evt@example.com", "hashed")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return store, userID
}

func TestGetEventNames(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "events.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	now := time.Now().UTC()

	events := []struct {
		name  string
		props map[string]any
	}{
		{"purchase_completed", map[string]any{"plan": "pro", "billing": "annual"}},
		{"trial_started", map[string]any{"plan": "starter"}},
	}

	for _, e := range events {
		if err := store.CreateEvent(ctx, &api.Event{
			SiteID:     site.ID,
			SessionID:  uuid.New(),
			Name:       e.name,
			Properties: e.props,
			Timestamp:  now.AddDate(0, 0, -1),
		}); err != nil {
			t.Fatalf("create event %s: %v", e.name, err)
		}
	}

	params := api.EventNamesParams{
		SiteID: site.ID,
		Start:  now.AddDate(0, 0, -7),
		End:    now,
	}

	names, err := store.GetEventNames(ctx, params)
	if err != nil {
		t.Fatalf("GetEventNames: %v", err)
	}

	if len(names) != 2 {
		t.Fatalf("expected 2 event names, got %d: %v", len(names), names)
	}

	sort.Strings(names)
	if names[0] != "purchase_completed" {
		t.Errorf("expected names[0] = purchase_completed, got %s", names[0])
	}
	if names[1] != "trial_started" {
		t.Errorf("expected names[1] = trial_started, got %s", names[1])
	}
}

func TestEventQueriesIncludeImportedAggregates(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "imported-events.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	importID := uuid.New()
	day := time.Date(2026, 4, 8, 0, 0, 0, 0, time.UTC)
	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO imported_event_daily (site_id, import_id, date, event_name, path, link_url, visitors, events, source_file)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, site.ID, importID, day, "outbound_click", "", "https://example.com", 1, 3, "imported_custom_events.csv"); err != nil {
		t.Fatalf("insert imported event: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO imported_event_properties_daily (site_id, import_id, date, event_name, property_key, property_value, visitors, events, source_file)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, site.ID, importID, day, "outbound_click", "url", "https://example.com", 1, 3, "imported_custom_events.csv"); err != nil {
		t.Fatalf("insert imported event property: %v", err)
	}

	names, err := store.GetEventNames(ctx, api.EventNamesParams{
		SiteID: site.ID,
		Start:  day.AddDate(0, 0, -1),
		End:    day.AddDate(0, 0, 1),
	})
	if err != nil {
		t.Fatalf("GetEventNames: %v", err)
	}
	if len(names) != 1 || names[0] != "outbound_click" {
		t.Fatalf("expected imported event name, got %#v", names)
	}

	keys, err := store.GetEventPropertyKeys(ctx, api.EventNamesParams{
		SiteID: site.ID,
		Start:  day.AddDate(0, 0, -1),
		End:    day.AddDate(0, 0, 1),
	}, "outbound_click")
	if err != nil {
		t.Fatalf("GetEventPropertyKeys: %v", err)
	}
	if len(keys) != 1 || keys[0] != "url" {
		t.Fatalf("expected imported property key, got %#v", keys)
	}

	series, err := store.GetEventTimeseries(ctx, api.EventTimeseriesParams{
		SiteID:    site.ID,
		Start:     day.AddDate(0, 0, -1),
		End:       day.AddDate(0, 0, 2),
		EventName: "outbound_click",
	})
	if err != nil {
		t.Fatalf("GetEventTimeseries: %v", err)
	}
	var total int
	for _, point := range series {
		total += point.Count
	}
	if total != 3 {
		t.Fatalf("expected imported event count 3, got %d (%#v)", total, series)
	}
}

func TestGetEventNamesEmpty(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "no-events.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	now := time.Now().UTC()
	params := api.EventNamesParams{
		SiteID: site.ID,
		Start:  now.AddDate(0, 0, -7),
		End:    now,
	}

	names, err := store.GetEventNames(ctx, params)
	if err != nil {
		t.Fatalf("GetEventNames: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected 0 event names, got %d: %v", len(names), names)
	}
}

func TestGetEventNamesOutsideRange(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "range-events.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	now := time.Now().UTC()

	// Insert an event that is older than the query window
	if err := store.CreateEvent(ctx, &api.Event{
		SiteID:     site.ID,
		SessionID:  uuid.New(),
		Name:       "old_event",
		Properties: map[string]any{"key": "value"},
		Timestamp:  now.AddDate(0, 0, -30),
	}); err != nil {
		t.Fatalf("create event: %v", err)
	}

	params := api.EventNamesParams{
		SiteID: site.ID,
		Start:  now.AddDate(0, 0, -7),
		End:    now,
	}

	names, err := store.GetEventNames(ctx, params)
	if err != nil {
		t.Fatalf("GetEventNames: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected 0 names for out-of-range event, got %d: %v", len(names), names)
	}
}

func TestGetEventPropertyKeys(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "propkeys.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	now := time.Now().UTC()

	if err := store.CreateEvent(ctx, &api.Event{
		SiteID:     site.ID,
		SessionID:  uuid.New(),
		Name:       "purchase_completed",
		Properties: map[string]any{"plan": "pro", "billing": "annual"},
		Timestamp:  now.AddDate(0, 0, -1),
	}); err != nil {
		t.Fatalf("create event: %v", err)
	}

	params := api.EventNamesParams{
		SiteID: site.ID,
		Start:  now.AddDate(0, 0, -7),
		End:    now,
	}

	keys, err := store.GetEventPropertyKeys(ctx, params, "purchase_completed")
	if err != nil {
		t.Fatalf("GetEventPropertyKeys: %v", err)
	}

	if len(keys) != 2 {
		t.Fatalf("expected 2 property keys, got %d: %v", len(keys), keys)
	}

	sort.Strings(keys)
	if keys[0] != "billing" {
		t.Errorf("expected keys[0] = billing, got %s", keys[0])
	}
	if keys[1] != "plan" {
		t.Errorf("expected keys[1] = plan, got %s", keys[1])
	}
}

func TestGetEventPropertyKeysUnknownEvent(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "propkeys-unknown.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	now := time.Now().UTC()
	params := api.EventNamesParams{
		SiteID: site.ID,
		Start:  now.AddDate(0, 0, -7),
		End:    now,
	}

	keys, err := store.GetEventPropertyKeys(ctx, params, "nonexistent_event")
	if err != nil {
		t.Fatalf("GetEventPropertyKeys: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected 0 property keys for unknown event, got %d: %v", len(keys), keys)
	}
}

func TestGetEventPropertyBreakdown(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "breakdown.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	now := time.Now().UTC()

	if err := store.CreateEvent(ctx, &api.Event{
		SiteID:     site.ID,
		SessionID:  uuid.New(),
		Name:       "purchase_completed",
		Properties: map[string]any{"plan": "pro", "billing": "annual"},
		Timestamp:  now.AddDate(0, 0, -1),
	}); err != nil {
		t.Fatalf("create event: %v", err)
	}

	if err := store.CreateEvent(ctx, &api.Event{
		SiteID:     site.ID,
		SessionID:  uuid.New(),
		Name:       "trial_started",
		Properties: map[string]any{"plan": "starter"},
		Timestamp:  now.AddDate(0, 0, -1),
	}); err != nil {
		t.Fatalf("create trial event: %v", err)
	}

	params := api.EventBreakdownParams{
		SiteID:      site.ID,
		Start:       now.AddDate(0, 0, -7),
		End:         now,
		EventName:   "purchase_completed",
		PropertyKey: "plan",
	}

	breakdown, err := store.GetEventPropertyBreakdown(ctx, params)
	if err != nil {
		t.Fatalf("GetEventPropertyBreakdown: %v", err)
	}

	if len(breakdown) != 1 {
		t.Fatalf("expected 1 breakdown entry, got %d: %v", len(breakdown), breakdown)
	}
	if breakdown[0].Name != "pro" {
		t.Errorf("expected breakdown[0].Name = pro, got %s", breakdown[0].Name)
	}
	if breakdown[0].Value != 1 {
		t.Errorf("expected breakdown[0].Value = 1, got %d", breakdown[0].Value)
	}
}

func TestGetEventPropertyBreakdownMultipleValues(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "breakdown-multi.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	now := time.Now().UTC()

	// Two "pro" events and one "starter" event for the same event name
	for range 2 {
		if err := store.CreateEvent(ctx, &api.Event{
			SiteID:     site.ID,
			SessionID:  uuid.New(),
			Name:       "purchase_completed",
			Properties: map[string]any{"plan": "pro", "billing": "annual"},
			Timestamp:  now.AddDate(0, 0, -1),
		}); err != nil {
			t.Fatalf("create pro event: %v", err)
		}
	}

	if err := store.CreateEvent(ctx, &api.Event{
		SiteID:     site.ID,
		SessionID:  uuid.New(),
		Name:       "purchase_completed",
		Properties: map[string]any{"plan": "starter", "billing": "monthly"},
		Timestamp:  now.AddDate(0, 0, -1),
	}); err != nil {
		t.Fatalf("create starter event: %v", err)
	}

	params := api.EventBreakdownParams{
		SiteID:      site.ID,
		Start:       now.AddDate(0, 0, -7),
		End:         now,
		EventName:   "purchase_completed",
		PropertyKey: "plan",
	}

	breakdown, err := store.GetEventPropertyBreakdown(ctx, params)
	if err != nil {
		t.Fatalf("GetEventPropertyBreakdown: %v", err)
	}

	if len(breakdown) != 2 {
		t.Fatalf("expected 2 breakdown entries, got %d: %v", len(breakdown), breakdown)
	}

	// Results are ordered by count DESC, so "pro" (2) comes before "starter" (1).
	if breakdown[0].Name != "pro" {
		t.Errorf("expected breakdown[0].Name = pro, got %s", breakdown[0].Name)
	}
	if breakdown[0].Value != 2 {
		t.Errorf("expected breakdown[0].Value = 2, got %d", breakdown[0].Value)
	}
	if breakdown[1].Name != "starter" {
		t.Errorf("expected breakdown[1].Name = starter, got %s", breakdown[1].Name)
	}
	if breakdown[1].Value != 1 {
		t.Errorf("expected breakdown[1].Value = 1, got %d", breakdown[1].Value)
	}
}

func TestGetEventAudience(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "audience.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	now := time.Now().UTC()
	sessionWithEvent := uuid.New()
	sessionWithoutEvent := uuid.New()

	// A hit that shares sessionWithEvent — should appear in the audience
	if err := store.CreateHit(ctx, &api.Hit{
		SiteID:    site.ID,
		SessionID: sessionWithEvent,
		PageID:    uuid.New(),
		Timestamp: now.AddDate(0, 0, -1),
		Path:      "/signup",
	}); err != nil {
		t.Fatalf("create hit: %v", err)
	}

	// The event for that session
	if err := store.CreateEvent(ctx, &api.Event{
		SiteID:     site.ID,
		SessionID:  sessionWithEvent,
		Name:       "purchase",
		Properties: map[string]any{},
		Timestamp:  now.AddDate(0, 0, -1),
	}); err != nil {
		t.Fatalf("create event: %v", err)
	}

	// A hit with NO matching event — must not appear in audience
	if err := store.CreateHit(ctx, &api.Hit{
		SiteID:    site.ID,
		SessionID: sessionWithoutEvent,
		PageID:    uuid.New(),
		Timestamp: now.AddDate(0, 0, -1),
		Path:      "/other-page",
	}); err != nil {
		t.Fatalf("create non-event hit: %v", err)
	}

	params := api.EventAudienceParams{
		SiteID:    site.ID,
		Start:     now.AddDate(0, 0, -7),
		End:       now,
		EventName: "purchase",
	}

	audience, err := store.GetEventAudience(ctx, params)
	if err != nil {
		t.Fatalf("GetEventAudience: %v", err)
	}

	if len(audience.TopPages) != 1 {
		t.Fatalf("expected 1 top page, got %d: %v", len(audience.TopPages), audience.TopPages)
	}
	if audience.TopPages[0].Name != "/signup" {
		t.Errorf("expected top page = /signup, got %s", audience.TopPages[0].Name)
	}
	for _, p := range audience.TopPages {
		if p.Name == "/other-page" {
			t.Errorf("non-event page /other-page should not appear in audience")
		}
	}
}

func TestGetEventAudienceWithPropertyFilter(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "audience-prop.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	now := time.Now().UTC()
	sessionPro := uuid.New()
	sessionStarter := uuid.New()

	// Pro session: hit + event with plan=pro
	if err := store.CreateHit(ctx, &api.Hit{
		SiteID:    site.ID,
		SessionID: sessionPro,
		PageID:    uuid.New(),
		Timestamp: now.AddDate(0, 0, -1),
		Path:      "/pricing-pro",
	}); err != nil {
		t.Fatalf("create pro hit: %v", err)
	}
	if err := store.CreateEvent(ctx, &api.Event{
		SiteID:     site.ID,
		SessionID:  sessionPro,
		Name:       "signup",
		Properties: map[string]any{"plan": "pro"},
		Timestamp:  now.AddDate(0, 0, -1),
	}); err != nil {
		t.Fatalf("create pro event: %v", err)
	}

	// Starter session: hit + event with plan=starter
	if err := store.CreateHit(ctx, &api.Hit{
		SiteID:    site.ID,
		SessionID: sessionStarter,
		PageID:    uuid.New(),
		Timestamp: now.AddDate(0, 0, -1),
		Path:      "/pricing-starter",
	}); err != nil {
		t.Fatalf("create starter hit: %v", err)
	}
	if err := store.CreateEvent(ctx, &api.Event{
		SiteID:     site.ID,
		SessionID:  sessionStarter,
		Name:       "signup",
		Properties: map[string]any{"plan": "starter"},
		Timestamp:  now.AddDate(0, 0, -1),
	}); err != nil {
		t.Fatalf("create starter event: %v", err)
	}

	// Filter audience to only plan=pro sessions
	params := api.EventAudienceParams{
		SiteID:        site.ID,
		Start:         now.AddDate(0, 0, -7),
		End:           now,
		EventName:     "signup",
		PropertyKey:   "plan",
		PropertyValue: "pro",
	}

	audience, err := store.GetEventAudience(ctx, params)
	if err != nil {
		t.Fatalf("GetEventAudience: %v", err)
	}

	if len(audience.TopPages) != 1 {
		t.Fatalf("expected 1 top page for plan=pro filter, got %d: %v", len(audience.TopPages), audience.TopPages)
	}
	if audience.TopPages[0].Name != "/pricing-pro" {
		t.Errorf("expected top page = /pricing-pro, got %s", audience.TopPages[0].Name)
	}
	for _, p := range audience.TopPages {
		if p.Name == "/pricing-starter" {
			t.Errorf("/pricing-starter should not appear when filtering to plan=pro")
		}
	}
}

func TestEventQueriesApplyMultipleDimensionFilters(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "events-multi-filter.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	now := time.Now().UTC()
	cases := []struct {
		session uuid.UUID
		path    string
		width   int
	}{
		{session: uuid.New(), path: "/pricing", width: 1200},
		{session: uuid.New(), path: "/pricing", width: 390},
		{session: uuid.New(), path: "/docs", width: 1200},
	}

	for _, tc := range cases {
		width := tc.width
		if err := store.CreateHit(ctx, &api.Hit{
			SiteID:        site.ID,
			SessionID:     tc.session,
			PageID:        uuid.New(),
			Timestamp:     now.Add(-2 * time.Hour),
			Path:          tc.path,
			ViewportWidth: &width,
		}); err != nil {
			t.Fatalf("create hit %s/%d: %v", tc.path, tc.width, err)
		}
		if err := store.CreateEvent(ctx, &api.Event{
			SiteID:     site.ID,
			SessionID:  tc.session,
			Name:       "newsletter_signup",
			Properties: map[string]any{"plan": "pro"},
			Timestamp:  now.Add(-2 * time.Hour),
		}); err != nil {
			t.Fatalf("create event %s/%d: %v", tc.path, tc.width, err)
		}
	}

	filters := []api.Filter{
		{Type: "path", Value: "/pricing"},
		{Type: "device", Value: "Desktop"},
	}
	series, err := store.GetEventTimeseries(ctx, api.EventTimeseriesParams{
		SiteID:    site.ID,
		Start:     now.Add(-24 * time.Hour),
		End:       now,
		EventName: "newsletter_signup",
		Filters:   filters,
	})
	if err != nil {
		t.Fatalf("GetEventTimeseries: %v", err)
	}
	total := 0
	for _, point := range series {
		total += point.Count
	}
	if total != 1 {
		t.Fatalf("expected only the /pricing desktop event to match, got total %d from %+v", total, series)
	}

	audience, err := store.GetEventAudience(ctx, api.EventAudienceParams{
		SiteID:    site.ID,
		Start:     now.Add(-24 * time.Hour),
		End:       now,
		EventName: "newsletter_signup",
		Filters:   filters,
	})
	if err != nil {
		t.Fatalf("GetEventAudience: %v", err)
	}
	if len(audience.TopPages) != 1 || audience.TopPages[0].Name != "/pricing" || audience.TopPages[0].Value != 1 {
		t.Fatalf("expected only /pricing desktop audience row, got %+v", audience.TopPages)
	}
	if len(audience.TopDevices) != 1 || audience.TopDevices[0].Name != "Desktop" || audience.TopDevices[0].Value != 1 {
		t.Fatalf("expected only Desktop audience row, got %+v", audience.TopDevices)
	}
}

func TestGetEventAudienceEmpty(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "audience-empty.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	now := time.Now().UTC()
	params := api.EventAudienceParams{
		SiteID:    site.ID,
		Start:     now.AddDate(0, 0, -7),
		End:       now,
		EventName: "nonexistent",
	}

	audience, err := store.GetEventAudience(ctx, params)
	if err != nil {
		t.Fatalf("GetEventAudience: %v", err)
	}
	if len(audience.TopPages) != 0 || len(audience.TopReferrers) != 0 ||
		len(audience.TopDevices) != 0 || len(audience.TopCountries) != 0 {
		t.Errorf("expected all empty slices for unknown event, got pages=%v referrers=%v",
			audience.TopPages, audience.TopReferrers)
	}
}

func TestGetEventPropertyBreakdownUnknownKey(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := context.Background()

	site, err := store.CreateSite(ctx, userID, "breakdown-unknown.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	now := time.Now().UTC()

	if err := store.CreateEvent(ctx, &api.Event{
		SiteID:     site.ID,
		SessionID:  uuid.New(),
		Name:       "purchase_completed",
		Properties: map[string]any{"plan": "pro"},
		Timestamp:  now.AddDate(0, 0, -1),
	}); err != nil {
		t.Fatalf("create event: %v", err)
	}

	params := api.EventBreakdownParams{
		SiteID:      site.ID,
		Start:       now.AddDate(0, 0, -7),
		End:         now,
		EventName:   "purchase_completed",
		PropertyKey: "nonexistent_key",
	}

	breakdown, err := store.GetEventPropertyBreakdown(ctx, params)
	if err != nil {
		t.Fatalf("GetEventPropertyBreakdown: %v", err)
	}
	if len(breakdown) != 0 {
		t.Errorf("expected 0 breakdown entries for unknown key, got %d: %v", len(breakdown), breakdown)
	}
}
