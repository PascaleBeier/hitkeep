package database

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

func TestRecordActivityBuildsTrackingStatus(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	userID, err := store.CreateUser(ctx, "activity@example.com", "hashed")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	hostname := "www.example.com"
	now := time.Now().UTC().Truncate(time.Second)
	longVersion := strings.Repeat("v", 80)
	if err := store.RecordHitActivity(ctx, []*api.Hit{{
		SiteID:         site.ID,
		Timestamp:      now,
		Hostname:       &hostname,
		TrackerSource:  " wordpress ",
		TrackerVersion: longVersion,
	}}); err != nil {
		t.Fatalf("record hit activity: %v", err)
	}
	if err := store.RecordEventActivity(ctx, []*api.Event{{
		SiteID:         site.ID,
		Name:           "outbound_click",
		Timestamp:      now.Add(time.Minute),
		TrackerSource:  "wordpress",
		TrackerVersion: "2.3.0",
	}}); err != nil {
		t.Fatalf("record event activity: %v", err)
	}

	status, err := store.GetSiteTrackingStatus(ctx, site.ID, now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("get tracking status: %v", err)
	}
	if status == nil {
		t.Fatal("expected tracking status")
	}
	if status.Status != api.TrackingStatusLive {
		t.Fatalf("expected live status, got %q", status.Status)
	}
	if status.LastHostname != hostname {
		t.Fatalf("expected hostname %q, got %q", hostname, status.LastHostname)
	}
	if status.TrackerSource != "wordpress" {
		t.Fatalf("expected trimmed tracker source, got %q", status.TrackerSource)
	}
	if len(status.TrackerVersion) > 64 {
		t.Fatalf("expected tracker version to be length-limited, got %d", len(status.TrackerVersion))
	}
	if status.LastAutomaticEventName != "outbound_click" {
		t.Fatalf("expected automatic event, got %q", status.LastAutomaticEventName)
	}
}

func TestListSystemActivationUsesSameDomainNormalizationAsSiteStatus(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	userID, err := store.CreateUser(ctx, "activation@example.com", "hashed")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	hostname := "www.example.com"
	now := time.Now().UTC().Truncate(time.Second)
	if err := store.RecordHitActivity(ctx, []*api.Hit{{
		SiteID:    site.ID,
		Timestamp: now,
		Hostname:  &hostname,
	}}); err != nil {
		t.Fatalf("record hit activity: %v", err)
	}

	live, err := store.ListSystemActivation(ctx, ActivationQuery{
		Status: "live",
		Now:    now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("list live activation rows: %v", err)
	}
	if live.Total != 1 || len(live.Rows) != 1 {
		t.Fatalf("expected one live activation row, got total=%d rows=%d", live.Total, len(live.Rows))
	}
	if live.Rows[0].SiteID != site.ID {
		t.Fatalf("expected site %s, got %s", site.ID, live.Rows[0].SiteID)
	}
	if live.Rows[0].Status != api.TrackingStatusLive {
		t.Fatalf("expected live row, got %q", live.Rows[0].Status)
	}
	if live.Rows[0].HitsLast24h != 1 || live.Rows[0].HitsLast7d != 1 {
		t.Fatalf("expected hourly hit counts, got 24h=%d 7d=%d", live.Rows[0].HitsLast24h, live.Rows[0].HitsLast7d)
	}

	mismatch, err := store.ListSystemActivation(ctx, ActivationQuery{
		Status: "domain_mismatch",
		Now:    now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("list mismatch activation rows: %v", err)
	}
	if mismatch.Total != 0 || len(mismatch.Rows) != 0 {
		t.Fatalf("expected no mismatch rows, got total=%d rows=%d", mismatch.Total, len(mismatch.Rows))
	}
}

func TestListSystemActivationReportsDormantSites(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	userID, err := store.CreateUser(ctx, "dormant@example.com", "hashed")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "dormant.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	hostname := "dormant.example.com"
	hitAt := time.Now().UTC().Add(-8 * 24 * time.Hour).Truncate(time.Second)
	if err := store.RecordHitActivity(ctx, []*api.Hit{{
		SiteID:    site.ID,
		Timestamp: hitAt,
		Hostname:  &hostname,
	}}); err != nil {
		t.Fatalf("record hit activity: %v", err)
	}

	rows, err := store.ListSystemActivation(ctx, ActivationQuery{
		Status: "dormant",
		Now:    hitAt.Add(8 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("list dormant activation rows: %v", err)
	}
	if rows.Total != 1 || len(rows.Rows) != 1 {
		t.Fatalf("expected one dormant row, got total=%d rows=%d", rows.Total, len(rows.Rows))
	}
	if rows.Rows[0].Status != api.TrackingStatusDormant {
		t.Fatalf("expected dormant row, got %q", rows.Rows[0].Status)
	}
}

func TestRecordActivityIgnoresNilAndUnknownSiteIDs(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	if err := store.RecordHitActivity(ctx, []*api.Hit{nil}); err != nil {
		t.Fatalf("record nil hit activity: %v", err)
	}
	if err := store.RecordEventActivity(ctx, []*api.Event{nil}); err != nil {
		t.Fatalf("record nil event activity: %v", err)
	}
	if err := store.RecordHitActivity(ctx, []*api.Hit{{SiteID: uuid.New(), Timestamp: time.Now().UTC()}}); err == nil {
		t.Fatal("expected unknown site activity to fail")
	}
}

func TestGetSiteSetupStateReportsRecordedSignals(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	userID, err := store.CreateUser(ctx, "setup-state@example.com", "hashed")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "setup-state.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	state, err := store.GetSiteSetupState(ctx, site.ID)
	if err != nil {
		t.Fatalf("get setup state: %v", err)
	}
	if state != (api.SiteSetupState{}) {
		t.Fatalf("expected every flag to be false for a fresh site, got %+v", state)
	}

	now := time.Now().UTC()
	if err := store.CreateAIFetch(ctx, &api.AIFetch{
		SiteID:          site.ID,
		Timestamp:       now,
		AssistantName:   "ChatGPT-User",
		AssistantFamily: "openai",
		Path:            "/pricing",
		StatusCode:      200,
		ResourceType:    "page",
	}); err != nil {
		t.Fatalf("create ai fetch: %v", err)
	}
	if err := store.CreateEventsBulk(ctx, []*api.Event{
		{SiteID: site.ID, SessionID: uuid.New(), Name: "assistant.chat_started", Timestamp: now},
		{SiteID: site.ID, SessionID: uuid.New(), Name: "purchase", Timestamp: now},
	}); err != nil {
		t.Fatalf("create events: %v", err)
	}
	if err := store.CreateWebVital(ctx, &api.WebVital{
		SiteID:    site.ID,
		SessionID: uuid.New(),
		PageID:    uuid.New(),
		Metric:    api.WebVitalLCP,
		Value:     1800,
		Path:      "/",
		Timestamp: now,
	}); err != nil {
		t.Fatalf("create web vital: %v", err)
	}

	state, err = store.GetSiteSetupState(ctx, site.ID)
	if err != nil {
		t.Fatalf("get setup state after ingest: %v", err)
	}
	want := api.SiteSetupState{
		HasAIFetches:       true,
		HasChatbotEvents:   true,
		HasCustomEvents:    true,
		HasEcommerceEvents: true,
		HasWebVitals:       true,
	}
	if state != want {
		t.Fatalf("expected every flag to be true, got %+v", state)
	}
}

func TestGetSiteSetupStateSeparatesCustomEventsFromNamedFamilies(t *testing.T) {
	store := setupTenantStore(t)
	ctx := context.Background()

	userID, err := store.CreateUser(ctx, "setup-state-custom@example.com", "hashed")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "setup-state-custom.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	if err := store.CreateEvent(ctx, &api.Event{
		SiteID:    site.ID,
		SessionID: uuid.New(),
		Name:      "newsletter_signup",
		Timestamp: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create event: %v", err)
	}

	state, err := store.GetSiteSetupState(ctx, site.ID)
	if err != nil {
		t.Fatalf("get setup state: %v", err)
	}
	if state != (api.SiteSetupState{HasCustomEvents: true}) {
		t.Fatalf("expected only has_custom_events to be true, got %+v", state)
	}
}
