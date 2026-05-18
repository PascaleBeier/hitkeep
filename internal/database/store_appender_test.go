package database

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

func setupAppenderStore(t *testing.T) (*Store, uuid.UUID, *api.Site) {
	t.Helper()

	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	userID, err := store.CreateUser(context.Background(), "appender@example.com", "hashed")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(context.Background(), userID, "appender.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	return store, userID, site
}

func TestCreateHitsBulk(t *testing.T) {
	store, _, site := setupAppenderStore(t)
	ctx := context.Background()

	referrer := "https://example.com"
	hostname := "appender.example.com"
	language := "de-DE"
	country := "DE"
	viewport := 1440
	unique := true

	hits := []*api.Hit{
		{
			SiteID:        site.ID,
			SessionID:     uuid.New(),
			PageID:        uuid.New(),
			Timestamp:     time.Now().Add(-2 * time.Minute),
			Path:          "/pricing",
			Hostname:      &hostname,
			Referrer:      &referrer,
			ViewportWidth: &viewport,
			Language:      &language,
			CountryCode:   &country,
			IsUnique:      &unique,
		},
		{
			SiteID:    site.ID,
			SessionID: uuid.New(),
			PageID:    uuid.New(),
			Path:      "/signup",
		},
	}

	if err := store.CreateHitsBulk(ctx, hits); err != nil {
		t.Fatalf("CreateHitsBulk: %v", err)
	}
	if hits[1].Timestamp.IsZero() {
		t.Fatalf("expected bulk insert to assign zero timestamp")
	}

	result, err := store.GetHits(ctx, api.HitQueryParams{
		SiteID: site.ID,
		Start:  time.Now().Add(-1 * time.Hour),
		End:    time.Now().Add(1 * time.Hour),
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("GetHits: %v", err)
	}

	if result.Total != 2 {
		t.Fatalf("expected 2 hits, got %d", result.Total)
	}

	paths := map[string]api.Hit{}
	for _, hit := range result.Data {
		paths[hit.Path] = hit
	}

	if got, ok := paths["/pricing"]; !ok {
		t.Fatalf("expected /pricing hit in %+v", result.Data)
	} else {
		if got.Language == nil || *got.Language != language {
			t.Fatalf("expected language %q, got %+v", language, got.Language)
		}
		if got.Hostname == nil || *got.Hostname != hostname {
			t.Fatalf("expected hostname %q, got %+v", hostname, got.Hostname)
		}
		if got.CountryCode == nil || *got.CountryCode != country {
			t.Fatalf("expected country %q, got %+v", country, got.CountryCode)
		}
	}
}

func TestCreateHitPersistsGeoNetworkMetadata(t *testing.T) {
	store, _, site := setupAppenderStore(t)
	ctx := context.Background()

	region := "California"
	city := "Mountain View"
	provider := "Google LLC"
	asnOrg := "Google LLC"
	asn := 15169

	hit := &api.Hit{
		SiteID:    site.ID,
		SessionID: uuid.New(),
		PageID:    uuid.New(),
		Timestamp: time.Now().UTC(),
		Path:      "/geo",
		Region:    &region,
		City:      &city,
		Provider:  &provider,
		ASN:       &asn,
		ASNOrg:    &asnOrg,
	}

	if err := store.CreateHit(ctx, hit); err != nil {
		t.Fatalf("CreateHit: %v", err)
	}

	result, err := store.GetHits(ctx, api.HitQueryParams{
		SiteID: site.ID,
		Start:  hit.Timestamp.Add(-time.Minute),
		End:    hit.Timestamp.Add(time.Minute),
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("GetHits: %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("expected 1 hit, got %d", result.Total)
	}

	got := result.Data[0]
	if got.Region == nil || *got.Region != region {
		t.Fatalf("expected region %q, got %+v", region, got.Region)
	}
	if got.City == nil || *got.City != city {
		t.Fatalf("expected city %q, got %+v", city, got.City)
	}
	if got.Provider == nil || *got.Provider != provider {
		t.Fatalf("expected provider %q, got %+v", provider, got.Provider)
	}
	if got.ASN == nil || *got.ASN != asn {
		t.Fatalf("expected ASN %d, got %+v", asn, got.ASN)
	}
	if got.ASNOrg == nil || *got.ASNOrg != asnOrg {
		t.Fatalf("expected ASN org %q, got %+v", asnOrg, got.ASNOrg)
	}
}

func TestCreateHitsBulkWithLegacyHostnameColumnOrder(t *testing.T) {
	ctx := context.Background()

	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if _, err := store.DB().ExecContext(ctx, `
		CREATE TABLE hits (
			id              UUID        PRIMARY KEY,
			site_id         UUID        NOT NULL,
			session_id      UUID        NOT NULL,
			page_id         UUID        NOT NULL,
			timestamp       TIMESTAMPTZ NOT NULL,
			path            VARCHAR     NOT NULL,
			referrer        VARCHAR,
			user_agent      VARCHAR,
			viewport_width  INT,
			viewport_height INT,
			screen_width    INT,
			screen_height   INT,
			language        VARCHAR,
			is_unique       BOOLEAN,
			country_code    VARCHAR,
			region          VARCHAR,
			city            VARCHAR,
			provider        VARCHAR,
			asn             INTEGER,
			asn_org         VARCHAR,
			utm_source      VARCHAR,
			utm_medium      VARCHAR,
			utm_campaign    VARCHAR,
			utm_term        VARCHAR,
			utm_content     VARCHAR,
			hostname        VARCHAR
		)
	`); err != nil {
		t.Fatalf("create legacy hits table: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx, `
		CREATE TABLE rollup_dirty_buckets (
			site_id     UUID        NOT NULL,
			rollup_type VARCHAR     NOT NULL,
			bucket_unit VARCHAR     NOT NULL,
			bucket      TIMESTAMPTZ NOT NULL,
			updated_at  TIMESTAMPTZ NOT NULL,
			PRIMARY KEY (site_id, rollup_type, bucket_unit, bucket)
		)
	`); err != nil {
		t.Fatalf("create rollup_dirty_buckets table: %v", err)
	}

	siteID := uuid.New()
	sessionID := uuid.New()
	pageID := uuid.New()
	hostname := "legacy.example.com"
	referrer := "https://ref.example.com/post"
	userAgent := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)"
	language := "de-DE"
	country := "DE"
	viewportWidth := 1440
	viewportHeight := 900
	screenWidth := 1728
	screenHeight := 1117
	isUnique := true

	hit := &api.Hit{
		SiteID:         siteID,
		SessionID:      sessionID,
		PageID:         pageID,
		Timestamp:      time.Now().UTC(),
		Path:           "/legacy-order",
		Hostname:       &hostname,
		Referrer:       &referrer,
		UserAgent:      &userAgent,
		ViewportWidth:  &viewportWidth,
		ViewportHeight: &viewportHeight,
		ScreenWidth:    &screenWidth,
		ScreenHeight:   &screenHeight,
		Language:       &language,
		CountryCode:    &country,
		IsUnique:       &isUnique,
	}

	if err := store.CreateHit(ctx, hit); err != nil {
		t.Fatalf("CreateHit with legacy schema: %v", err)
	}

	var (
		gotHostname       sql.NullString
		gotReferrer       sql.NullString
		gotUserAgent      sql.NullString
		gotViewportWidth  sql.NullInt32
		gotViewportHeight sql.NullInt32
		gotScreenWidth    sql.NullInt32
		gotScreenHeight   sql.NullInt32
	)
	if err := store.DB().QueryRowContext(ctx, `
		SELECT hostname, referrer, user_agent, viewport_width, viewport_height, screen_width, screen_height
		FROM hits
		WHERE site_id = ?
	`, siteID).Scan(
		&gotHostname,
		&gotReferrer,
		&gotUserAgent,
		&gotViewportWidth,
		&gotViewportHeight,
		&gotScreenWidth,
		&gotScreenHeight,
	); err != nil {
		t.Fatalf("load stored hit: %v", err)
	}

	if gotHostname.String != hostname {
		t.Fatalf("expected hostname %q, got %+v", hostname, gotHostname)
	}
	if gotReferrer.String != referrer {
		t.Fatalf("expected referrer %q, got %+v", referrer, gotReferrer)
	}
	if gotUserAgent.String != userAgent {
		t.Fatalf("expected user agent %q, got %+v", userAgent, gotUserAgent)
	}
	if gotViewportWidth.Int32 != int32(viewportWidth) {
		t.Fatalf("expected viewport width %d, got %+v", viewportWidth, gotViewportWidth)
	}
	if gotViewportHeight.Int32 != int32(viewportHeight) {
		t.Fatalf("expected viewport height %d, got %+v", viewportHeight, gotViewportHeight)
	}
	if gotScreenWidth.Int32 != int32(screenWidth) {
		t.Fatalf("expected screen width %d, got %+v", screenWidth, gotScreenWidth)
	}
	if gotScreenHeight.Int32 != int32(screenHeight) {
		t.Fatalf("expected screen height %d, got %+v", screenHeight, gotScreenHeight)
	}
}

func TestCreateEventsBulk(t *testing.T) {
	store, _, site := setupAppenderStore(t)
	ctx := context.Background()

	events := []*api.Event{
		{
			SiteID:     site.ID,
			SessionID:  uuid.New(),
			Name:       "signup",
			Properties: map[string]any{"plan": "pro"},
			Timestamp:  time.Now().Add(-1 * time.Minute),
		},
		{
			SiteID:    site.ID,
			SessionID: uuid.New(),
			Name:      "checkout_started",
		},
	}

	if err := store.CreateEventsBulk(ctx, events); err != nil {
		t.Fatalf("CreateEventsBulk: %v", err)
	}
	if events[1].Timestamp.IsZero() {
		t.Fatalf("expected bulk insert to assign zero timestamp")
	}

	var count int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM events WHERE site_id = ?", site.ID).Scan(&count); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 events, got %d", count)
	}

	var props string
	if err := store.DB().QueryRowContext(ctx, "SELECT CAST(properties AS VARCHAR) FROM events WHERE site_id = ? AND name = ?", site.ID, "signup").Scan(&props); err != nil {
		t.Fatalf("load event properties: %v", err)
	}
	if !strings.Contains(props, "pro") {
		t.Fatalf("expected marshaled properties to contain plan value, got %q", props)
	}
}

func TestCreateAIFetchesBulk(t *testing.T) {
	store, _, site := setupAppenderStore(t)
	ctx := context.Background()

	hostname := "appender.example.com"
	contentType := "text/html; charset=utf-8"
	responseMs := 123
	bytesServed := int64(4567)
	userAgent := "GPTBot/1.0"

	fetches := []*api.AIFetch{
		{
			SiteID:          site.ID,
			Timestamp:       time.Now().Add(-2 * time.Minute),
			AssistantName:   "GPTBot",
			AssistantFamily: "OpenAI",
			Path:            "/pricing",
			Hostname:        &hostname,
			StatusCode:      200,
			ContentType:     &contentType,
			ResourceType:    "html",
			ResponseMs:      &responseMs,
			BytesServed:     &bytesServed,
			UserAgent:       &userAgent,
		},
		{
			SiteID:          site.ID,
			AssistantName:   "PerplexityBot",
			AssistantFamily: "Perplexity",
			Path:            "/docs/getting-started",
			StatusCode:      404,
			ResourceType:    "html",
		},
	}

	if err := store.CreateAIFetchesBulk(ctx, fetches); err != nil {
		t.Fatalf("CreateAIFetchesBulk: %v", err)
	}
	if fetches[1].Timestamp.IsZero() {
		t.Fatalf("expected bulk insert to assign zero timestamp")
	}
	if fetches[1].ID == uuid.Nil {
		t.Fatalf("expected bulk insert to assign zero id")
	}

	var count int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_fetches WHERE site_id = ?", site.ID).Scan(&count); err != nil {
		t.Fatalf("count ai fetches: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 ai fetches, got %d", count)
	}

	var storedPath string
	if err := store.DB().QueryRowContext(ctx, "SELECT path FROM ai_fetches WHERE site_id = ? AND assistant_name = ?", site.ID, "GPTBot").Scan(&storedPath); err != nil {
		t.Fatalf("load ai fetch path: %v", err)
	}
	if storedPath != "/pricing" {
		t.Fatalf("expected stored path /pricing, got %q", storedPath)
	}
}

func TestCreateWebVitalsBulkAndAggregates(t *testing.T) {
	store, _, site := setupAppenderStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	sessionID := uuid.New()
	pageID := uuid.New()
	navigationType := "navigate"
	userAgent := "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_2) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	language := "de-DE"
	country := "DE"
	city := "Berlin"
	provider := "Deutsche Telekom AG"
	asn := 3320
	asnOrg := "Deutsche Telekom AG"
	viewportWidth := 1440
	if err := store.CreateHit(ctx, &api.Hit{
		SiteID:        site.ID,
		SessionID:     sessionID,
		PageID:        pageID,
		Timestamp:     now.Add(-3*time.Hour - time.Second),
		Path:          "/pricing",
		UserAgent:     &userAgent,
		Language:      &language,
		CountryCode:   &country,
		City:          &city,
		Provider:      &provider,
		ASN:           &asn,
		ASNOrg:        &asnOrg,
		ViewportWidth: &viewportWidth,
	}); err != nil {
		t.Fatalf("CreateHit: %v", err)
	}

	vitals := []*api.WebVital{
		{SiteID: site.ID, SessionID: sessionID, PageID: pageID, Metric: api.WebVitalLCP, Value: 1200, Path: "/pricing?plan=pro#cta", NavigationType: &navigationType, Timestamp: now.Add(-3 * time.Hour), TrackerSource: "hk.js"},
		{SiteID: site.ID, SessionID: uuid.New(), PageID: uuid.New(), Metric: api.WebVitalLCP, Value: 2600, Path: "/pricing", Timestamp: now.Add(-2 * time.Hour)},
		{SiteID: site.ID, SessionID: uuid.New(), PageID: uuid.New(), Metric: api.WebVitalLCP, Value: 5100, Path: "/checkout", Timestamp: now.Add(-1 * time.Hour)},
		{SiteID: site.ID, SessionID: uuid.New(), PageID: uuid.New(), Metric: api.WebVitalCLS, Value: 0.31, Path: "/pricing", Timestamp: now.Add(-1 * time.Hour)},
	}

	if err := store.CreateWebVitalsBulk(ctx, vitals); err != nil {
		t.Fatalf("CreateWebVitalsBulk: %v", err)
	}
	if vitals[0].Rating != api.WebVitalRatingGood || vitals[1].Rating != api.WebVitalRatingNeedsImprovement || vitals[2].Rating != api.WebVitalRatingPoor {
		t.Fatalf("expected server-derived ratings, got %s/%s/%s", vitals[0].Rating, vitals[1].Rating, vitals[2].Rating)
	}

	var storedPath string
	if err := store.DB().QueryRowContext(ctx, "SELECT path FROM web_vitals WHERE id = ?", vitals[0].ID).Scan(&storedPath); err != nil {
		t.Fatalf("load web vital path: %v", err)
	}
	if storedPath != "/pricing?plan=pro#cta" {
		t.Fatalf("store should persist the caller-provided sanitized path verbatim, got %q", storedPath)
	}

	summary, err := store.GetWebVitalsSummary(ctx, api.WebVitalsParams{
		SiteID: site.ID,
		Start:  now.Add(-24 * time.Hour),
		End:    now.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("GetWebVitalsSummary: %v", err)
	}
	if len(summary) != 2 {
		t.Fatalf("expected two metric summaries, got %+v", summary)
	}

	lcp := summaryMetric(t, summary, api.WebVitalLCP)
	if lcp.Samples != 3 || lcp.Good != 1 || lcp.NeedsImprove != 1 || lcp.Poor != 1 {
		t.Fatalf("unexpected LCP rating distribution: %+v", lcp)
	}
	if lcp.P75 <= 2600 {
		t.Fatalf("expected interpolated LCP p75 above the middle sample, got %f", lcp.P75)
	}

	pages, err := store.GetWebVitalsPages(ctx, api.WebVitalsParams{
		SiteID: site.ID,
		Start:  now.Add(-24 * time.Hour),
		End:    now.Add(24 * time.Hour),
		Metric: api.WebVitalLCP,
		Rating: api.WebVitalRatingPoor,
	})
	if err != nil {
		t.Fatalf("GetWebVitalsPages: %v", err)
	}
	if len(pages) != 1 || pages[0].Path != "/checkout" || pages[0].Rating != api.WebVitalRatingPoor {
		t.Fatalf("expected poor checkout page row, got %+v", pages)
	}
	if pages[0].Metrics[api.WebVitalLCP].Samples != 1 {
		t.Fatalf("expected selected page row to include LCP metric cell, got %+v", pages[0].Metrics)
	}

	breakdown, err := store.GetWebVitalsBreakdown(ctx, api.WebVitalsParams{
		SiteID: site.ID,
		Start:  now.Add(-24 * time.Hour),
		End:    now.Add(24 * time.Hour),
		Metric: api.WebVitalLCP,
		Path:   "/pricing?plan=pro#cta",
	}, api.WebVitalDimensionBrowser)
	if err != nil {
		t.Fatalf("GetWebVitalsBreakdown: %v", err)
	}
	if len(breakdown) != 1 || breakdown[0].Name != "Chrome" || breakdown[0].Samples != 1 {
		t.Fatalf("expected Chrome browser breakdown, got %+v", breakdown)
	}

	providerBreakdown, err := store.GetWebVitalsBreakdown(ctx, api.WebVitalsParams{
		SiteID: site.ID,
		Start:  now.Add(-24 * time.Hour),
		End:    now.Add(24 * time.Hour),
		Metric: api.WebVitalLCP,
		Path:   "/pricing?plan=pro#cta",
	}, api.WebVitalDimensionProvider)
	if err != nil {
		t.Fatalf("GetWebVitalsBreakdown provider: %v", err)
	}
	if len(providerBreakdown) != 1 || providerBreakdown[0].Name != "Deutsche Telekom AG" || providerBreakdown[0].Samples != 1 {
		t.Fatalf("expected provider breakdown, got %+v", providerBreakdown)
	}
}

func summaryMetric(t *testing.T, metrics []api.WebVitalSummaryMetric, metric api.WebVitalMetric) api.WebVitalSummaryMetric {
	t.Helper()
	for _, item := range metrics {
		if item.Metric == metric {
			return item
		}
	}
	t.Fatalf("missing metric %s in %+v", metric, metrics)
	return api.WebVitalSummaryMetric{}
}
